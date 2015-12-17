package main

import (
	"container/list"
	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleffmpeg"
	_ "github.com/layeh/gumble/opus"
	"log"
	"sync"
)

const (
	CMD_PREFIX string = "/"
	CMD_ADD    string = CMD_PREFIX + "add"
	CMD_PLAY   string = CMD_PREFIX + "play"
	CMD_PAUSE  string = CMD_PREFIX + "pause"
	CMD_VOLUME string = CMD_PREFIX + "volume"
	CMD_QUEUE  string = CMD_PREFIX + "queue"
	CMD_SKIP   string = CMD_PREFIX + "skip"
	CMD_CLEAR  string = CMD_PREFIX + "clear"
	CMD_HELP   string = CMD_PREFIX + "help"
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

func (jukebox *Jukebox) Add(song *Song) {
	jukebox.lock.Lock()
	jukebox.downloadQueue.PushBack(song)
	if jukebox.downloadQueue.Len() == 1 {
		go func() { jukebox.downloadChannel <- true }()
	}
	jukebox.lock.Unlock()
}

func (jukebox *Jukebox) Play() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Play()
	}
}

func (jukebox *Jukebox) Pause() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	jukebox.stream.Pause()
}

func (jukebox *Jukebox) Volume(volume float32) {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	jukebox.volume = volume
	if jukebox.stream.State() == gumbleffmpeg.StatePlaying {
		jukebox.stream.Pause()
		jukebox.stream.Volume = volume
		jukebox.stream.Play()
	} else {
		jukebox.stream.Volume = volume
	}
}

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

func (jukebox *Jukebox) Skip() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	jukebox.stream.Stop()
}

func (jukebox *Jukebox) Clear() {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	jukebox.playQueue = list.New()
	jukebox.stream.Stop()
}

func (jukebox *Jukebox) Help(sender *gumble.User) {
	message := "Commands:<br>" +
		CMD_ADD + " <link> - add a song to the queue<br>" +
		CMD_PLAY + " - start the player<br>" +
		CMD_PAUSE + " - pause the player<br>" +
		CMD_VOLUME + " <value> - sets the volume of the song<br>" +
		CMD_QUEUE + " - lists the current songs in the queue<br>" +
		CMD_SKIP + " - skips the current song in the queue<br>" +
		CMD_CLEAR + " - clears the queue<br>" +
		CMD_HELP + " - how did you even find this"
	sender.Send(message)
}
