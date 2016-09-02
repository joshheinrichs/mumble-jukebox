package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/layeh/gumble/gumble"
	"github.com/pborman/uuid"
)

var ErrInternal = errors.New("Internal server error")

// Used for grabbing information from the *.info.json file provided by
// youtube-dl
type Info struct {
	Title    *string  `json:"title"`
	Duration *float64 `json:"duration"`
}

type Song struct {
	rwMutex  sync.RWMutex
	sender   *gumble.User
	url      string
	songpath *string
	infopath *string
	title    *string
	duration *time.Duration
}

func NewSong(sender *gumble.User, url string) *Song {
	song := Song{
		sender: sender,
		url:    url,
	}
	return &song
}

func (song *Song) Download() error {
	song.rwMutex.RLock()
	url := song.url
	song.rwMutex.RUnlock()

	id := uuid.New()
	songpath, err := filepath.Abs(fmt.Sprintf("%s/%s.mp3", config.Cache.Directory, id))
	if err != nil {
		log.Printf("%s\n", err)
		return ErrInternal
	}
	infopath, err := filepath.Abs(fmt.Sprintf("%s/%s.mp3.info.json", config.Cache.Directory, id))
	if err != nil {
		log.Printf("%s\n", err)
		return ErrInternal
	}

	log.Printf("File will be saved to: %s\n", songpath)
	log.Printf("Info will be saved to: %s\n", infopath)

	cmd := exec.Command("youtube-dl",
		"--max-filesize", config.Cache.MaxFilesize,
		"--extract-audio",
		"--no-playlist",
		"--write-info-json",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"-o", songpath,
		url)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("An error occurred downloading the link:\n%s\n", out)
		return errors.New("Unable to obtain audio from the specified link.")
	}

	blob, err := ioutil.ReadFile(infopath)
	if err != nil {
		log.Printf("%s\n", err)
		return ErrInternal
	}

	var info Info
	err = json.Unmarshal(blob, &info)
	if err != nil {
		log.Printf("%s\n", err)
		return ErrInternal
	}

	song.rwMutex.Lock()
	song.songpath = &songpath
	song.infopath = &infopath
	song.title = info.Title
	if info.Duration != nil {
		duration := time.Duration(float64(time.Second) * *info.Duration)
		song.duration = &duration
	}
	song.rwMutex.Unlock()

	return nil
}

func (song *Song) Delete() error {
	song.rwMutex.Lock()
	defer song.rwMutex.Unlock()
	if song.songpath != nil {
		if err := os.Remove(*song.songpath); err != nil {
			return err
		}
		song.songpath = nil
	}
	if song.infopath != nil {
		if err := os.Remove(*song.infopath); err != nil {
			return err
		}
		song.infopath = nil
	}
	return nil
}

func (song *Song) Title() *string {
	song.rwMutex.RLock()
	defer song.rwMutex.RUnlock()
	return song.title
}

func (song *Song) Duration() *time.Duration {
	song.rwMutex.RLock()
	defer song.rwMutex.RUnlock()
	return song.duration
}

func (song *Song) Sender() *gumble.User {
	song.rwMutex.RLock()
	defer song.rwMutex.RUnlock()
	return song.sender
}

func (song *Song) URL() string {
	song.rwMutex.RLock()
	defer song.rwMutex.RUnlock()
	return song.url
}
