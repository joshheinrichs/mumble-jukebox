package main

import (
	"container/list"
	"github.com/layeh/gumble/gumble"
	"github.com/layeh/gumble/gumbleffmpeg"
	"github.com/layeh/gumble/gumbleutil"
	_ "github.com/layeh/gumble/opus"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
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
	CMD_QUEUE  string = CMD_PREFIX + "queue"
	CMD_SKIP   string = CMD_PREFIX + "skip"
	CMD_CLEAR  string = CMD_PREFIX + "clear"
	CMD_HELP   string = CMD_PREFIX + "help"
)

var audioStreamer *AudioStreamer

type AudioStreamer struct {
	lock            sync.RWMutex
	client          *gumble.Client
	stream          *gumbleffmpeg.Stream
	volume          float32
	playQueue       *list.List
	playChannel     chan bool
	downloadQueue   *list.List
	downloadChannel chan bool
}

func NewAudioStreamer(client *gumble.Client) *AudioStreamer {
	audioStreamer := AudioStreamer{
		client:          client,
		stream:          nil,
		volume:          1.0,
		playQueue:       list.New(),
		playChannel:     make(chan bool),
		downloadQueue:   list.New(),
		downloadChannel: make(chan bool),
	}
	go audioStreamer.playThread()
	go audioStreamer.downloadThread()
	return &audioStreamer
}

func (audioStreamer *AudioStreamer) playThread() {
	for {
		audioStreamer.lock.Lock()
		if audioStreamer.playQueue.Len() == 0 {
			audioStreamer.lock.Unlock()
			_ = <-audioStreamer.playChannel
			audioStreamer.lock.Lock()
		}
		song, _ := audioStreamer.playQueue.Front().Value.(*Song)
		audioStreamer.lock.Unlock()

		audioStreamer.playSong(song)

		audioStreamer.lock.Lock()
		audioStreamer.playQueue.Remove(audioStreamer.playQueue.Front())
		audioStreamer.lock.Unlock()
	}
}

func (audioStreamer *AudioStreamer) playSong(song *Song) {
	source := gumbleffmpeg.SourceFile(*song.filepath)

	audioStreamer.lock.Lock()
	audioStreamer.stream = gumbleffmpeg.New(audioStreamer.client, source)
	audioStreamer.stream.Volume = audioStreamer.volume
	audioStreamer.lock.Unlock()

	err := audioStreamer.stream.Play()
	if err != nil {
		log.Panic(err)
	}
	audioStreamer.stream.Wait()

	err = song.Delete()
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Finished playing song")
}

func (audioStreamer *AudioStreamer) downloadThread() {
	for {
		audioStreamer.lock.Lock()
		if audioStreamer.downloadQueue.Len() == 0 {
			log.Println("Nothing to download")
			audioStreamer.lock.Unlock()
			_ = <-audioStreamer.downloadChannel
			audioStreamer.lock.Lock()
		}
		song, _ := audioStreamer.downloadQueue.Front().Value.(*Song)
		audioStreamer.lock.Unlock()

		err := song.Download()
		if err != nil {
			log.Println(err)
			continue
		}

		audioStreamer.lock.Lock()
		audioStreamer.downloadQueue.Remove(audioStreamer.downloadQueue.Front())
		audioStreamer.playQueue.PushBack(song)
		if audioStreamer.playQueue.Len() == 1 {
			go func() { audioStreamer.playChannel <- true }()
		}
		audioStreamer.lock.Unlock()
	}
}

func (audioStreamer *AudioStreamer) Add(song *Song) {
	audioStreamer.lock.Lock()
	audioStreamer.downloadQueue.PushBack(song)
	if audioStreamer.downloadQueue.Len() == 1 {
		go func() { audioStreamer.downloadChannel <- true }()
	}
	audioStreamer.lock.Unlock()
}

func (audioStreamer *AudioStreamer) Play() {
	audioStreamer.lock.RLock()
	defer audioStreamer.lock.RUnlock()
	if audioStreamer.stream != nil {
		audioStreamer.stream.Play()
	}
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

func (audioStreamer *AudioStreamer) Queue(sender *gumble.User) {
	audioStreamer.lock.Lock()
	defer audioStreamer.lock.Unlock()
	message := ""
	elem := audioStreamer.playQueue.Front()
	for elem != nil {
		song, _ := elem.Value.(*Song)
		message += song.String() + "<br>"
		elem = elem.Next()
	}
	elem = audioStreamer.downloadQueue.Front()
	for elem != nil {
		song, _ := elem.Value.(*Song)
		message += song.String() + "<br>"
		elem = elem.Next()
	}
	sender.Send(message)
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

func parseMessage(s string, sender *gumble.User) {
	switch {
	case strings.HasPrefix(s, CMD_ADD):
		urls := parseUrls(s)
		for _, url := range urls {
			log.Printf("Found url: %s", url)
			song := NewSong(sender, url)
			audioStreamer.Add(song)
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
	case strings.HasPrefix(s, CMD_QUEUE):
		audioStreamer.Queue(sender)
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
			audioStreamer = NewAudioStreamer(e.Client)
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
