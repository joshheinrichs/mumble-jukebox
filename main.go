package main

import (
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleutil"
	"golang.org/x/net/html"
)

var jukebox *Jukebox
var config *Config

// Parses the given string, and executes a jukebox command based upon the
// string's prefix.
func parseMessage(s string, sender *gumble.User) {
	switch {
	case strings.HasPrefix(s, CmdAdd):
		urls := parseUrls(s)
		for _, url := range urls {
			log.Printf("Found url: %s", url)
			song := NewSong(sender, url)
			jukebox.Add(song)
		}
	case strings.HasPrefix(s, CmdPlay):
		jukebox.Play()
	case strings.HasPrefix(s, CmdPause):
		jukebox.Pause()
	case strings.HasPrefix(s, CmdVolume):
		volumeString := strings.TrimPrefix(s, CmdVolume+" ")
		volume64, err := strconv.ParseFloat(volumeString, 32)
		if err != nil {
			log.Println(err)
			return
		} else if volume64 > 1.0 {
			log.Println("Tried to set volume to value greater than 1")
			return
		}
		jukebox.Volume(float32(volume64))
	case strings.HasPrefix(s, CmdQueue):
		jukebox.Queue(sender)
	case strings.HasPrefix(s, CmdSkip):
		jukebox.Skip()
	case strings.HasPrefix(s, CmdClear):
		jukebox.Clear()
	case strings.HasPrefix(s, CmdHelp):
		jukebox.Help(sender)
	}
}

// Parses the given string, and returns the set of URLs found within it. URLs
// should be follow standard HTML format (i.e. <a href="foo"></a>).
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
	var err error
	config, err = ReadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	client := gumble.NewClient(config.Mumble)
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
		ChannelChange: func(e *gumble.ChannelChangeEvent) {
			log.Printf("Changed to Channel: %s\n", e.Channel.Name)
		},
		ACL: func(e *gumble.ACLEvent) {
			log.Printf("Got ACL for: %s", e.ACL.Channel.Name)
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

	wg.Add(1)
	wg.Wait()
}
