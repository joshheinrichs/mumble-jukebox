package main

import (
	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleutil"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"sync"
)

var jukebox *Jukebox

func parseMessage(s string, sender *gumble.User) {
	switch {
	case strings.HasPrefix(s, CMD_ADD):
		urls := parseUrls(s)
		for _, url := range urls {
			log.Printf("Found url: %s", url)
			song := NewSong(sender, url)
			jukebox.Add(song)
		}
	case strings.HasPrefix(s, CMD_PLAY):
		jukebox.Play()
	case strings.HasPrefix(s, CMD_PAUSE):
		jukebox.Pause()
	case strings.HasPrefix(s, CMD_VOLUME):
		volumeString := strings.TrimPrefix(s, CMD_VOLUME+" ")
		volume64, err := strconv.ParseFloat(volumeString, 32)
		if err != nil {
			log.Println(err)
			return
		} else if volume64 > 1.0 {
			log.Println("Tried to set volume to value greater than 1")
			return
		}
		jukebox.Volume(float32(volume64))
	case strings.HasPrefix(s, CMD_QUEUE):
		jukebox.Queue(sender)
	case strings.HasPrefix(s, CMD_SKIP):
		jukebox.Skip()
	case strings.HasPrefix(s, CMD_CLEAR):
		jukebox.Clear()
	case strings.HasPrefix(s, CMD_HELP):
		jukebox.Help(sender)
	}
}

func parseUrls(s string) []string {
	urls := make([]string, 0)
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		log.Fatal(err)
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					urls = append(urls, a.Val)
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return urls
}

func main() {
	var wg sync.WaitGroup
	wg.Add(1)

	blob, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	config := gumble.NewConfig()
	err = yaml.Unmarshal(blob, &config)
	if err != nil {
		log.Fatal(err)
	}

	client := gumble.NewClient(config)
	client.Attach(gumbleutil.Listener{
		Connect: func(e *gumble.ConnectEvent) {
			log.Printf("Sever's maximum bitrate: %d", *e.MaximumBitrate)
			e.Client.Attach(gumbleutil.AutoBitrate)
			jukebox = NewJukebox(e.Client)
		},
		TextMessage: func(e *gumble.TextMessageEvent) {
			log.Printf("Received message: %s", e.Message)
			go parseMessage(e.Message, e.Sender)
		},
		UserChange: func(e *gumble.UserChangeEvent) {
			log.Printf("%s self muted: %t", e.User.Name, e.User.SelfMuted)
		},
		Disconnect: func(e *gumble.DisconnectEvent) {
			// TODO: clean up audio files
			wg.Done()
		},
	})
	err = client.Connect()
	if err != nil {
		log.Fatal(err)
	}

	wg.Wait()
}
