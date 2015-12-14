package main

import (
	"code.google.com/p/go-uuid/uuid"
	"container/list"
	"fmt"
	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleffmpeg"
	"github.com/layeh/gumble/gumbleutil"
	_ "github.com/layeh/gumble/opus"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

var youtubeRegexp *regexp.Regexp
var soundcloudRegexp *regexp.Regexp

var audioStreamer *AudioStreamer

type AudioStreamer struct {
	urlChan  chan string
	urlQueue *list.List
	client   *gumble.Client
	stream   *gumbleffmpeg.Stream
	playing  bool
	finished chan bool
}

func NewAudioStreamer(client *gumble.Client) *AudioStreamer {
	audioStreamer := AudioStreamer{
		urlChan:  make(chan string),
		urlQueue: list.New(),
		client:   client,
		playing:  false,
		finished: make(chan bool),
	}
	audioStreamer.listen()
	return &audioStreamer
}

func (audioStreamer *AudioStreamer) listen() {
	go func() {
		for {
			select {
			case url := <-audioStreamer.urlChan:
				if audioStreamer.playing {
					log.Printf("Added url to queue")
					audioStreamer.urlQueue.PushBack(url)
				} else {
					log.Printf("Playing url\n")
					audioStreamer.playing = true
					go audioStreamer.playUrl(url)
				}
			case _ = <-audioStreamer.finished:
				if audioStreamer.urlQueue.Front() == nil {
					log.Printf("Nothing to play\n")
					audioStreamer.playing = false
				} else {
					log.Printf("Playing next url\n")
					value := audioStreamer.urlQueue.Remove(audioStreamer.urlQueue.Front())
					url, _ := value.(string)
					audioStreamer.playing = true
					go audioStreamer.playUrl(url)
				}
			}
		}
	}()
}

func (audioStreamer *AudioStreamer) playUrl(url string) {
	file := fmt.Sprintf("audio/%s.mp3", uuid.New())
	log.Printf("File will be saved to: %s", file)
	cmd := exec.Command("youtube-dl",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"-o", file,
		url)
	err := cmd.Run()
	if err != nil {
		log.Println(err)
		return
	}
	source := gumbleffmpeg.SourceFile(file)
	stream := gumbleffmpeg.New(audioStreamer.client, source)
	err = stream.Play()
	if err != nil {
		log.Println(err)
		return
	}
	stream.Wait()
	log.Printf("Finished playing song")
	audioStreamer.finished <- true
	os.Remove(file)
}

func (audioStreamer *AudioStreamer) AddUrl(url string) {
	audioStreamer.urlChan <- url
}

func findUrls(s string) []string {
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

func legalUrl(s string) bool {
	return youtubeRegexp.MatchString(s) || soundcloudRegexp.MatchString(s)
}

func main() {
	youtubeRegexp = regexp.MustCompile("(https?\\:\\/\\/)?(www\\.)?(youtube\\.com|youtu\\.?be)\\/(.*)")
	soundcloudRegexp = regexp.MustCompile("(https?\\:\\/\\/)?(www\\.)?(soundcloud.com|snd.sc)\\/(.*)")

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
			audioStreamer = NewAudioStreamer(e.Client)
		},
		TextMessage: func(e *gumble.TextMessageEvent) {
			log.Printf("Received message: %s", e.Message)
			urls := findUrls(e.Message)
			for _, url := range urls {
				log.Printf("Found url: %s", url)
				if legalUrl(url) {
					audioStreamer.AddUrl(url)
				}
			}
		},
		UserChange: func(e *gumble.UserChangeEvent) {
			log.Printf("%s self muted: %t", e.User.Name, e.User.SelfMuted)
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
