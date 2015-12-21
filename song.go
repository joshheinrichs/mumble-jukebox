package main

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/layeh/gumble/gumble"
	"html"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"
)

// Used for grabbing information from the *.info.json file provided by
// youtube-dl
type Info struct {
	Title    *string  `json:"title"`
	Duration *float64 `json:"duration"`
}

type Song struct {
	sender   *gumble.User
	url      string
	filepath *string
	infopath *string
	title    *string
	duration *time.Duration
}

func NewSong(sender *gumble.User, url string) *Song {
	song := Song{
		sender:   sender,
		url:      url,
		title:    nil,
		duration: nil,
	}
	return &song
}

// Downloads
func (song *Song) Download() error {
	id := uuid.New()
	outputpath := fmt.Sprintf("%s/%s.%%(ext)s", config.Filesystem.Directory, id)
	filepath := fmt.Sprintf("%s/%s.mp3", config.Filesystem.Directory, id)
	infopath := fmt.Sprintf("%s/%s.info.json", config.Filesystem.Directory, id)

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
		song.url)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("An error occurred downloading the link:\n%s\n", out)
		return errors.New("Unable to obtain audio from the specified link.")
	}

	song.filepath = &filepath
	song.infopath = &infopath

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

	song.title = info.Title
	if info.Duration != nil {
		var duration time.Duration
		duration = time.Duration(float64(time.Second) * *info.Duration)
		song.duration = &duration
	}

	return nil
}

func (song *Song) Delete() error {
	err := os.Remove(*song.filepath)
	if err != nil {
		return err
	}
	err = os.Remove(*song.infopath)
	if err != nil {
		return err
	}
	return nil
}

func (song *Song) String() string {
	str := ""
	if song.title != nil {
		str += html.EscapeString(fmt.Sprintf("Title: %s", *song.title)) + "<br>"
	}
	if song.duration != nil {
		str += html.EscapeString(fmt.Sprintf("Duration: %s", song.duration.String())) + "<br>"
	}
	if song.sender != nil {
		str += html.EscapeString(fmt.Sprintf("Sender: %s", song.sender.Name)) + "<br>"
	}
	str += fmt.Sprintf("URL: <a href='%s'>%s</a>", html.EscapeString(song.url), html.EscapeString(song.url)) + "<br>"
	return str
}
