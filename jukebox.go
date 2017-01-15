package main

import (
	"container/list"
	"errors"
	"log"
	"sync"

	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
	_ "layeh.com/gumble/opus"
)

var ErrVolumeOutsideRange = errors.New("Volume must be set to a value between 0 and 1")
var ErrQueueFull = errors.New("Queue is full")

type Jukebox struct {
	rwMutex        sync.RWMutex
	client         *gumble.Client
	stream         *gumbleffmpeg.Stream
	volume         float32
	playQueue      *list.List
	playChannel    chan bool
	downloadQueue  *list.List
	roomToDownload chan bool
	songToDownload chan bool
}

// NewJukebox returns a new jukebox.
func NewJukebox(client *gumble.Client) *Jukebox {
	jukebox := Jukebox{
		client:         client,
		stream:         nil,
		volume:         1.0,
		playQueue:      list.New(),
		playChannel:    make(chan bool),
		downloadQueue:  list.New(),
		roomToDownload: make(chan bool),
		songToDownload: make(chan bool),
	}
	go jukebox.playThread()
	go jukebox.downloadThread()
	return &jukebox
}

// Waits until songs are added to the play queue, and then plays them until
// completion.
func (jukebox *Jukebox) playThread() {
	for {
		jukebox.rwMutex.Lock()
		if jukebox.playQueue.Len() == 0 {
			jukebox.rwMutex.Unlock()
			_ = <-jukebox.playChannel
			jukebox.rwMutex.Lock()
		}
		song, _ := jukebox.playQueue.Front().Value.(*Song)
		jukebox.rwMutex.Unlock()

		jukebox.playSong(song)

		jukebox.rwMutex.Lock()
		jukebox.playQueue.Remove(jukebox.playQueue.Front())
		if jukebox.playQueue.Len() == config.Queue.MaxSize-1 {
			go func() { jukebox.roomToDownload <- true }()
		}
		jukebox.rwMutex.Unlock()
	}
}

// Plays the given song, blocking until completion.
func (jukebox *Jukebox) playSong(song *Song) {
	source := gumbleffmpeg.SourceFile(*song.songpath)

	jukebox.rwMutex.Lock()
	jukebox.stream = gumbleffmpeg.New(jukebox.client, source)
	jukebox.stream.Volume = jukebox.volume
	jukebox.rwMutex.Unlock()

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
		jukebox.rwMutex.Lock()
		if jukebox.playQueue.Len() >= config.Cache.MaxSize {
			log.Println("Maximum number of downloads reached")
			jukebox.rwMutex.Unlock()
			_ = <-jukebox.roomToDownload
			jukebox.rwMutex.Lock()
		}
		if jukebox.downloadQueue.Len() == 0 {
			log.Println("Nothing to download")
			jukebox.rwMutex.Unlock()
			_ = <-jukebox.songToDownload
			jukebox.rwMutex.Lock()
		}
		song, _ := jukebox.downloadQueue.Front().Value.(*Song)
		jukebox.rwMutex.Unlock()

		err := song.Download()
		if err != nil {
			log.Println(err)
			jukebox.rwMutex.Lock()
			jukebox.downloadQueue.Remove(jukebox.downloadQueue.Front())
			jukebox.rwMutex.Unlock()
			continue
		}

		jukebox.rwMutex.Lock()
		jukebox.downloadQueue.Remove(jukebox.downloadQueue.Front())
		jukebox.playQueue.PushBack(song)
		if jukebox.playQueue.Len() == 1 {
			go func() { jukebox.playChannel <- true }()
		}
		jukebox.rwMutex.Unlock()
	}
}

// Add adds a song to the jukebox's download queue. After the song is
// downloaded, it will be added to the play queue.
func (jukebox *Jukebox) Add(song *Song) error {
	jukebox.rwMutex.Lock()
	defer jukebox.rwMutex.Unlock()
	if jukebox.downloadQueue.Len() >= config.Queue.MaxSize {
		return ErrQueueFull
	}
	jukebox.downloadQueue.PushBack(song)
	if jukebox.downloadQueue.Len() == 1 {
		go func() { jukebox.songToDownload <- true }()
	}
	return nil
}

// Play resumes the jukebox's playback.
func (jukebox *Jukebox) Play() {
	jukebox.rwMutex.RLock()
	defer jukebox.rwMutex.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Play()
	}
}

// Pause pauses the jukebox's playback.
func (jukebox *Jukebox) Pause() {
	jukebox.rwMutex.RLock()
	defer jukebox.rwMutex.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Pause()
	}
}

// Volume sets the volume of the jukebox to the given value.
func (jukebox *Jukebox) Volume(volume float32) error {
	if volume > 1 {
		return ErrVolumeOutsideRange
	}
	jukebox.rwMutex.Lock()
	defer jukebox.rwMutex.Unlock()
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
	return nil
}

// Queue sends a message to the given user containing the list of songs
// currently in the queue.
func (jukebox *Jukebox) Queue() []*Song {
	jukebox.rwMutex.RLock()
	defer jukebox.rwMutex.RUnlock()
	// TODO: Should songs be duplicated?
	songs := make([]*Song, jukebox.playQueue.Len()+jukebox.downloadQueue.Len())
	i := 0
	for elem := jukebox.playQueue.Front(); elem != nil; elem = elem.Next() {
		songs[i] = elem.Value.(*Song)
		i++
	}
	for elem := jukebox.downloadQueue.Front(); elem != nil; elem = elem.Next() {
		songs[i] = elem.Value.(*Song)
		i++
	}
	return songs
}

// Skip skips the current song in the queue.
func (jukebox *Jukebox) Skip() {
	jukebox.rwMutex.RLock()
	defer jukebox.rwMutex.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Stop()
	}
}

// Clear clears all songs in the queue, including the song which is currently
// playing.
func (jukebox *Jukebox) Clear() {
	jukebox.rwMutex.Lock()
	defer jukebox.rwMutex.Unlock()
	jukebox.playQueue = list.New()
	if jukebox.stream != nil {
		jukebox.stream.Stop()
	}
}
