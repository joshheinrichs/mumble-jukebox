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
	"strconv"
	"strings"
	"sync"
)

const (
	CMD_PREFIX string = "/"
	CMD_ADD    string = CMD_PREFIX + "add"
	CMD_PLAY   string = CMD_PREFIX + "play"
	CMD_PAUSE  string = CMD_PREFIX + "pause"
	CMD_VOLUME string = CMD_PREFIX + "volume"
	CMD_SKIP   string = CMD_PREFIX + "skip"
	CMD_CLEAR  string = CMD_PREFIX + "clear"
	CMD_HELP   string = CMD_PREFIX + "help"
)

var youtubeRegexp *regexp.Regexp
var soundcloudRegexp *regexp.Regexp

var audioStreamer *AudioStreamer

type AudioStreamer struct {
	lock      sync.RWMutex
	playQueue *list.List
	client    *gumble.Client
	stream    *gumbleffmpeg.Stream
	playing   bool
	volume    float32
}

func NewAudioStreamer(client *gumble.Client) *AudioStreamer {
	audioStreamer := AudioStreamer{
		playQueue: list.New(),
		client:    client,
		stream:    nil,
		playing:   false,
		volume:    1.0,
	}
	return &audioStreamer
}

func (audioStreamer *AudioStreamer) Add(url string) {
	audioStreamer.lock.Lock()
	defer audioStreamer.lock.Unlock()
	if audioStreamer.playing {
		log.Printf("Added url to queue")
		audioStreamer.playQueue.PushBack(url)
	} else {
		log.Printf("Playing url\n")
		audioStreamer.playing = true
		go audioStreamer.playUrl(url)
	}
}

func (audioStreamer *AudioStreamer) Play() {
	audioStreamer.lock.RLock()
	defer audioStreamer.lock.RUnlock()
	audioStreamer.stream.Play()
}

func (audioStreamer *AudioStreamer) Pause() {
	audioStreamer.lock.RLock()
	defer audioStreamer.lock.RUnlock()
	audioStreamer.stream.Pause()
}

func (audioStreamer *AudioStreamer) Volume(volume float32) {
	audioStreamer.lock.Lock()
	defer audioStreamer.lock.Unlock()
	audioStreamer.volume = volume
	if audioStreamer.stream.State() == gumbleffmpeg.StatePlaying {
		audioStreamer.stream.Pause()
		audioStreamer.stream.Volume = volume
		audioStreamer.stream.Play()
	} else {
		audioStreamer.stream.Volume = volume
	}
}

func (audioStreamer *AudioStreamer) Skip() {
	audioStreamer.lock.RLock()
	defer audioStreamer.lock.RUnlock()
	audioStreamer.stream.Stop()
}

func (audioStreamer *AudioStreamer) Clear() {
	audioStreamer.lock.Lock()
	defer audioStreamer.lock.Unlock()
	audioStreamer.playQueue = list.New()
	audioStreamer.stream.Stop()
}

func (audioStreamer *AudioStreamer) Help(sender *gumble.User) {
	message := "Commands:<br>" +
		CMD_ADD + " <link> - add a song to the queue<br>" +
		CMD_PLAY + " - start the player<br>" +
		CMD_PAUSE + " - pause the player<br>" +
		CMD_VOLUME + " <value> - sets the volume of the song<br>" +
		CMD_SKIP + " - skips the current song in the queue<br>" +
		CMD_CLEAR + " - clears the queue<br>" +
		CMD_HELP + " - how did you even find this"
	sender.Send(message)
}

func (audioStreamer *AudioStreamer) playUrl(url string) {

	defer func() {
		audioStreamer.lock.Lock()
		defer audioStreamer.lock.Unlock()
		if audioStreamer.playQueue.Front() == nil {
			log.Printf("Nothing to play\n")
			audioStreamer.playing = false
		} else {
			log.Printf("Playing next url\n")
			value := audioStreamer.playQueue.Remove(audioStreamer.playQueue.Front())
			url, _ := value.(string)
			go audioStreamer.playUrl(url)
		}
	}()

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
	defer os.Remove(file)

	source := gumbleffmpeg.SourceFile(file)
	audioStreamer.stream = gumbleffmpeg.New(audioStreamer.client, source)
	audioStreamer.stream.Volume = audioStreamer.volume
	err = audioStreamer.stream.Play()
	if err != nil {
		log.Println(err)
		return
	}
	audioStreamer.stream.Wait()

	log.Printf("Finished playing song")
}

func parseMessage(s string, sender *gumble.User) {
	switch {
	case strings.HasPrefix(s, CMD_ADD):
		urls := parseUrls(s)
		for _, url := range urls {
			log.Printf("Found url: %s", url)
			if legalUrl(url) {
				audioStreamer.Add(url)
			}
		}
	case strings.HasPrefix(s, CMD_PLAY):
		audioStreamer.Play()
	case strings.HasPrefix(s, CMD_PAUSE):
		audioStreamer.Pause()
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
		audioStreamer.Volume(float32(volume64))
	case strings.HasPrefix(s, CMD_SKIP):
		audioStreamer.Skip()
	case strings.HasPrefix(s, CMD_CLEAR):
		audioStreamer.Clear()
	case strings.HasPrefix(s, CMD_HELP):
		audioStreamer.Help(sender)
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

func legalUrl(s string) bool {
	return youtubeRegexp.MatchString(s) || soundcloudRegexp.MatchString(s)
}

func main() {
	youtubeRegexp = regexp.MustCompile("(https?\\:\\/\\/)?(www\\.)?(youtube\\.com|youtu\\.?be)\\/(.*)")
	soundcloudRegexp = regexp.MustCompile("(https?\\:\\/\\/)?(www\\.)?(soundcloud.com|snd.sc)\\/(.*)")

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
			audioStreamer = NewAudioStreamer(e.Client)
		},
		TextMessage: func(e *gumble.TextMessageEvent) {
			log.Printf("Received message: %s", e.Message)
			parseMessage(e.Message, e.Sender)
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
