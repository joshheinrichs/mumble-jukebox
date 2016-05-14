package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleutil"
	"golang.org/x/net/html"
)

const (
	cmdPrefix string = "/"
	cmdAdd    string = cmdPrefix + "add"
	cmdPlay   string = cmdPrefix + "play"
	cmdPause  string = cmdPrefix + "pause"
	cmdVolume string = cmdPrefix + "volume"
	cmdQueue  string = cmdPrefix + "queue"
	cmdSkip   string = cmdPrefix + "skip"
	cmdClear  string = cmdPrefix + "clear"
	cmdHelp   string = cmdPrefix + "help"
)

var jukebox *Jukebox
var config *Config

// Parses the given string, and executes a jukebox command based upon the
// string's prefix.
func parseMessage(s string, sender *gumble.User) {
	switch {
	case strings.HasPrefix(s, cmdAdd):
		urls := parseURLs(s)
		for _, url := range urls {
			log.Printf("Found url: %s", url)
			song := NewSong(sender, url)
			jukebox.Add(song)
		}
	case strings.HasPrefix(s, cmdPlay):
		jukebox.Play()
	case strings.HasPrefix(s, cmdPause):
		jukebox.Pause()
	case strings.HasPrefix(s, cmdVolume):
		volumeString := strings.TrimPrefix(s, cmdVolume+" ")
		volume64, err := strconv.ParseFloat(volumeString, 32)
		if err != nil {
			log.Println(err)
			return
		}
		err = jukebox.Volume(float32(volume64))
		if err != nil {
			sender.Send(fmt.Sprintf("Error: %s", err.Error()))
		}
	case strings.HasPrefix(s, cmdQueue):
		queue := jukebox.Queue()
		if len(queue) == 0 {
			sender.Send("No songs in queue.")
		} else {
			message := "<table border=\"1\">" +
				"<tr><th>Title</th><th>Duration</th><th>Sender</th><th>URL</th></tr>"
			for _, song := range queue {
				var title, duration, sender, url string
				titlePtr := song.Title()
				if titlePtr != nil {
					title = *titlePtr
				}
				durationPtr := song.Duration()
				if durationPtr != nil {
					duration = durationPtr.String()
				}
				senderPtr := song.Sender()
				if senderPtr != nil {
					sender = senderPtr.Name
				}
				url = song.URL()
				message += fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td><a href=\"%s\">%s</a></td></tr>",
					title, duration, sender, url, url)
			}
			message += "</table>"
			sender.Send(message)
		}
	case strings.HasPrefix(s, cmdSkip):
		jukebox.Skip()
	case strings.HasPrefix(s, cmdClear):
		jukebox.Clear()
	case strings.HasPrefix(s, cmdHelp):
		message := "<table border=\"1\">" +
			"<tr><th>Command</th><th>Description</th></tr>" +
			"<tr><td>" + cmdAdd + " link</td><td>add a song to the queue</td></tr>" +
			"<tr><td>" + cmdPlay + "</td><td> start the player</td></tr>" +
			"<tr><td>" + cmdPause + "</td><td> pause the player</td></tr>" +
			"<tr><td>" + cmdVolume + " value</td><td> sets the volume of the song</td></tr>" +
			"<tr><td>" + cmdQueue + "</td><td> lists the current songs in the queue</td></tr>" +
			"<tr><td>" + cmdSkip + "</td><td> skips the current song in the queue</td></tr>" +
			"<tr><td>" + cmdClear + "</td><td> clears the queue</td></tr>" +
			"</table>"
		sender.Send(message)
	}
}

// Parses the given string, and returns the set of URLs found within it. URLs
// should be follow standard HTML format (i.e. <a href="foo"></a>).
func parseURLs(s string) []string {
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
