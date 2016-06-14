package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/layeh/gumble/gumble"
	"github.com/pborman/uuid"
)

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
	filepath *string
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
	outputpath := fmt.Sprintf("%s/%s.%%(ext)s", config.Cache.Directory, id)
	filepath := fmt.Sprintf("%s/%s.mp3", config.Cache.Directory, id)
	infopath := fmt.Sprintf("%s/%s.info.json", config.Cache.Directory, id)

	log.Printf("Output path: %s\n", outputpath)
	log.Printf("File will be saved to: %s\n", filepath)
	log.Printf("Info will be saved to: %s\n", infopath)

	cmd := exec.Command("youtube-dl",
		"--extract-audio",
		"--no-playlist",
		"--write-info-json",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"-o", outputpath,
		url)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("An error occurred downloading the link:\n%s\n", out)
		return errors.New("Unable to obtain audio from the specified link.")
	}

	blob, err := ioutil.ReadFile(infopath)
	if err != nil {
		log.Printf("%s\n", err)
		return errors.New("Internal server error.")
	}

	var info Info
	err = json.Unmarshal(blob, &info)
	if err != nil {
		log.Printf("%s\n", err)
		return errors.New("Internal server error.")
	}

	song.rwMutex.Lock()
	song.filepath = &filepath
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
	if song.filepath != nil {
		err := os.Remove(*song.filepath)
		if err != nil {
			return err
		}
		song.filepath = nil
	}
	if song.filepath != nil {
		err := os.Remove(*song.infopath)
		if err != nil {
			return err
		}
		song.filepath = nil
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
