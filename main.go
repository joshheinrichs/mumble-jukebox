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

var jukebox *Jukebox

type Jukebox struct {
	lock            sync.RWMutex
	client          *gumble.Client
	stream          *gumbleffmpeg.Stream
	volume          float32
	playQueue       *list.List
	playChannel     chan bool
	downloadQueue   *list.List
	downloadChannel chan bool
}

func NewJukebox(client *gumble.Client) *Jukebox {
	jukebox := Jukebox{
		client:          client,
		stream:          nil,
		volume:          1.0,
		playQueue:       list.New(),
		playChannel:     make(chan bool),
		downloadQueue:   list.New(),
		downloadChannel: make(chan bool),
	}
	go jukebox.playThread()
	go jukebox.downloadThread()
	return &jukebox
}

func (jukebox *Jukebox) playThread() {
	for {
		jukebox.lock.Lock()
		if jukebox.playQueue.Len() == 0 {
			jukebox.lock.Unlock()
			_ = <-jukebox.playChannel
			jukebox.lock.Lock()
		}
		song, _ := jukebox.playQueue.Front().Value.(*Song)
		jukebox.lock.Unlock()

		jukebox.playSong(song)

		jukebox.lock.Lock()
		jukebox.playQueue.Remove(jukebox.playQueue.Front())
		jukebox.lock.Unlock()
	}
}

func (jukebox *Jukebox) playSong(song *Song) {
	source := gumbleffmpeg.SourceFile(*song.filepath)

	jukebox.lock.Lock()
	jukebox.stream = gumbleffmpeg.New(jukebox.client, source)
	jukebox.stream.Volume = jukebox.volume
	jukebox.lock.Unlock()

	err := jukebox.stream.Play()
	if err != nil {
		log.Panic(err)
	}
	jukebox.stream.Wait()

	err = song.Delete()
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Finished playing song")
}

func (jukebox *Jukebox) downloadThread() {
	for {
		jukebox.lock.Lock()
		if jukebox.downloadQueue.Len() == 0 {
			log.Println("Nothing to download")
			jukebox.lock.Unlock()
			_ = <-jukebox.downloadChannel
			jukebox.lock.Lock()
		}
		song, _ := jukebox.downloadQueue.Front().Value.(*Song)
		jukebox.lock.Unlock()

		err := song.Download()
		if err != nil {
			log.Println(err)
			jukebox.lock.Lock()
			jukebox.downloadQueue.Remove(jukebox.downloadQueue.Front())
			jukebox.lock.Unlock()
			continue
		}

		jukebox.lock.Lock()
		jukebox.downloadQueue.Remove(jukebox.downloadQueue.Front())
		jukebox.playQueue.PushBack(song)
		if jukebox.playQueue.Len() == 1 {
			go func() { jukebox.playChannel <- true }()
		}
		jukebox.lock.Unlock()
	}
}

func (jukebox *Jukebox) Add(song *Song) {
	jukebox.lock.Lock()
	jukebox.downloadQueue.PushBack(song)
	if jukebox.downloadQueue.Len() == 1 {
		go func() { jukebox.downloadChannel <- true }()
	}
	jukebox.lock.Unlock()
}

func (jukebox *Jukebox) Play() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	if jukebox.stream != nil {
		jukebox.stream.Play()
	}
}

func (jukebox *Jukebox) Pause() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	jukebox.stream.Pause()
}

func (jukebox *Jukebox) Volume(volume float32) {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	jukebox.volume = volume
	if jukebox.stream.State() == gumbleffmpeg.StatePlaying {
		jukebox.stream.Pause()
		jukebox.stream.Volume = volume
		jukebox.stream.Play()
	} else {
		jukebox.stream.Volume = volume
	}
}

func (jukebox *Jukebox) Queue(sender *gumble.User) {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	message := ""
	elem := jukebox.playQueue.Front()
	for elem != nil {
		song, _ := elem.Value.(*Song)
		message += song.String() + "<br>"
		elem = elem.Next()
	}
	elem = jukebox.downloadQueue.Front()
	for elem != nil {
		song, _ := elem.Value.(*Song)
		message += song.String() + "<br>"
		elem = elem.Next()
	}
	sender.Send(message)
}

func (jukebox *Jukebox) Skip() {
	jukebox.lock.RLock()
	defer jukebox.lock.RUnlock()
	jukebox.stream.Stop()
}

func (jukebox *Jukebox) Clear() {
	jukebox.lock.Lock()
	defer jukebox.lock.Unlock()
	jukebox.playQueue = list.New()
	jukebox.stream.Stop()
}

func (jukebox *Jukebox) Help(sender *gumble.User) {
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
