package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleffmpeg"
	"github.com/layeh/gumble/gumbleutil"
	_ "github.com/layeh/gumble/opus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os/exec"
	"regexp"
	"sync"
)

func main() {
	blob, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	config := gumble.NewConfig()
	err = yaml.Unmarshal(blob, &config)
	if err != nil {
		log.Fatal(err)
	}

	yt := regexp.MustCompile("(https?\\:\\/\\/)?(www\\.)?(youtube\\.com|youtu\\.?be)\\/.+")
	client := gumble.NewClient(config)
	client.Attach(gumbleutil.Listener{
		Connect: func(e *gumble.ConnectEvent) {
			e.Client.Attach(gumbleutil.AutoBitrate)
		},
		TextMessage: func(e *gumble.TextMessageEvent) {
			log.Printf("Received message: %s", e.Message)
			link := yt.FindString(e.Message)
			if link == "" {
				log.Println("Could not find YouTube URL")
				return
			}
			file := fmt.Sprintf("audio/%s.mp3", uuid.New())
			log.Printf("File will be saved to: %s", file)
			cmd := exec.Command("youtube-dl", "--extract-audio",
				"--audio-format", "mp3",
				"-o", file,
				link)
			err := cmd.Run()
			if err != nil {
				log.Println(err)
				return
			}
			source := gumbleffmpeg.SourceFile(file)
			stream := gumbleffmpeg.New(e.Client, source)
			err = stream.Play()
			if err != nil {
				log.Println(err)
				return
			}
		},
	})
	err = client.Connect()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}
