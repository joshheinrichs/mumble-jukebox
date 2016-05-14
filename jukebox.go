package main

import (
	"container/list"
	"log"
	"sync"

	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleffmpeg"
	_ "github.com/layeh/gumble/opus"
)

const (
	CmdPrefix string = "/"
	CmdAdd    string = CmdPrefix + "add"
	CmdPlay   string = CmdPrefix + "play"
	CmdPause  string = CmdPrefix + "pause"
	CmdVolume string = CmdPrefix + "volume"
	CmdQueue  string = CmdPrefix + "queue"
	CmdSkip   string = CmdPrefix + "skip"
	CmdClear  string = CmdPrefix + "clear"
	CmdHelp   string = CmdPrefix + "help"
)

type Jukebox struct {
	lock            sync.RWMutex
	client          *gumble.Client
	stream          *gumbleffmpeg.Stream
	volume          float32
	playQueue       *list.List
	playChannel     chan bool
	downloadQueue   *list.List
	downloadChannel chan bool
}

// NewJukebox returns a new jukebox.
func NewJukebox(client *gumble.Client) *Jukebox {
	jukebox := Jukebox{
		client:          client,
		stream:          nil,
		volume:          1.0,
		playQueue:       list.New(),
		playChannel:     make(chan bool),
		downloadQueue:   list.New(),
		downloadChannel: make(chan bool),
	}
	go jukebox.playThread()
	go jukebox.downloadThread()
	return &jukebox
}

// Waits until songs are added to the play queue, and then plays them until
// completion.
func (jukebox *Jukebox) playThread() {
	for {
		jukebox.lock.Lock()
		if jukebox.playQueue.Len() == 0 {
			jukebox.lock.Unlock()
			_ = <-jukebox.playChannel
			jukebox.lock.Lock()
		}
		song, _ := jukebox.playQueue.Front().Value.(*Song)
		jukebox.lock.Unlock()

		jukebox.playSong(song)

		jukebox.lock.Lock()
		jukebox.playQueue.Remove(jukebox.playQueue.Front())
		jukebox.lock.Unlock()
	}
}

// Plays the given song, blocking until completion.
func (jukebox *Jukebox) playSong(song *Song) {
	source := gumbleffmpeg.SourceFile(*song.filepath)

	jukebox.lock.Lock()
	jukebox.stream = gumbleffmpeg.New(jukebox.client, source)
	jukebox.stream.Volume = jukebox.volume
	jukebox.lock.Unlock()

	err := jukebox.stream.Play()
	if err != nil {
		log.Panic(err)
	}
	jukebox.stream.Wait()

	err = song.Delete()
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Finished playing song")
}

// Waits until songs are added to the download queue, and then downloads and
// adds them to the play queue.
func (jukebox *Jukebox) downloadThread() {
	for {
		jukebox.lock.Lock()
		if jukebox.downloadQueue.Len() == 0 {
			log.Println("Nothing to download")
			jukebox.lock.Unlock()
			_ = <-jukebox.downloadChannel
			jukebox.lock.Lock()
		}
		song, _ := jukebox.downloadQueue.Front().Value.(*Song)
		jukebox.lock.Unlock()

		err := song.Download()
		if err != nil {
			log.Println(err)
			jukebox.lock.Lock()
			jukebox.downloadQueue.Remove(jukebox.downloadQueue.Front())
			jukebox.lock.Unlock()
			continue
		}

		jukebox.lock.Lock()
		jukebox.downloadQueue.Remove(jukebox.downloadQueue.Front())
		jukebox.playQueue.PushBack(song)
		if jukebox.playQueue.Len() == 1 {
			go func() { jukebox.playChannel <- true }()
		}
		jukebox.lock.Unlock()
	}
}

// Add adds a song to the jukebox's download queue. After the song is
// downloaded, it will be added to the play queue.
func (jukebox *Jukebox) Add(song *Song) {
	jukebox.lock.Lock()
	jukebox.downloadQueue.PushBack(song)
	if jukebox.downloadQueue.Len() == 1 {
		go func() { jukebox.downloadChannel <- true }()
	}
	jukebox.lock.Unlock()
}

// Play resumes the jukebox's playback.
func (jukebox *Jukebox) Play() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Play()
	}
}

// Pause pauses the jukebox's playback.
func (jukebox *Jukebox) Pause() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Pause()
	}
}

// Volume sets the volume of the jukebox to the given value.
func (jukebox *Jukebox) Volume(volume float32) {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	jukebox.volume = volume
	if jukebox.stream != nil {
		if jukebox.stream.State() == gumbleffmpeg.StatePlaying {
			jukebox.stream.Pause()
			jukebox.stream.Volume = volume
			jukebox.stream.Play()
		} else {
			jukebox.stream.Volume = volume
		}
	}
}

// Queue sends a message to the given user containing the list of songs
// currently in the queue.
func (jukebox *Jukebox) Queue(sender *gumble.User) {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	message := ""
	elem := jukebox.playQueue.Front()
	for elem != nil {
		song, _ := elem.Value.(*Song)
		message += song.String() + "<br>"
		elem = elem.Next()
	}
	elem = jukebox.downloadQueue.Front()
	for elem != nil {
		song, _ := elem.Value.(*Song)
		message += song.String() + "<br>"
		elem = elem.Next()
	}
	sender.Send(message)
}

// Skip skips the current song in the queue.
func (jukebox *Jukebox) Skip() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Stop()
	}
}

// Clear clears all songs in the queue, including the song which is currently
// playing.
func (jukebox *Jukebox) Clear() {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	jukebox.playQueue = list.New()
	if jukebox.stream != nil {
		jukebox.stream.Stop()
	}
}

// Help sends a message to the given user containing a list of jukebox commands.
func (jukebox *Jukebox) Help(sender *gumble.User) {
	message := "Commands:<br>" +
		CmdAdd + " <link> - add a song to the queue<br>" +
		CmdPlay + " - start the player<br>" +
		CmdPause + " - pause the player<br>" +
		CmdVolume + " <value> - sets the volume of the song<br>" +
		CmdQueue + " - lists the current songs in the queue<br>" +
		CmdSkip + " - skips the current song in the queue<br>" +
		CmdClear + " - clears the queue<br>" +
		CmdHelp + " - how did you even find this"
	sender.Send(message)
}
