package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"github.com/kennygrant/sanitize"
)

const (
	utcTimestampFmt   = "Mon, 02 Jan 2006 15:04:05" + " GMT"
	localTimestampFmt = "Mon, 02 Jan 2006 15:04:05 MST"
	maxFeedItems      = 5
	characterCount    = 280
)

var (
	logger  = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	counter = make(chan uint)
	history = make(map[string]bool)
)

type Feed struct {
	URL         string     `json:"feedUrl"`
	Website     string     `json:"websiteUrl"`
	Title       string     `json:"feedTitle"`
	Description string     `json:"feedDescription"`
	LastUpdate  string     `json:"whenLastUpdate"`
	Items       []FeedItem `json:"item"`
}

type FeedItem struct {
	Body      string `json:"body"`
	Permalink string `json:"permaLink"`
	PubDate   string `json:"pubDate"`
	Title     string `json:"title"`
	Link      string `json:"link"`
	Id        string `json:"id"`
}

type FetchResult struct {
	Content *xml.Decoder
	URL     string
}

type River struct {
	UpdatedFeeds struct {
		UpdatedFeed []Feed `json:"updatedFeed"`
	} `json:"updatedFeeds"`
	Metadata map[string]string `json:"metadata"`
}

type FeedFetcher struct {
	Ticker <-chan time.Time
	Delay  <-chan time.Time
	URL    string
}

func (self *FeedFetcher) Run(results chan FetchResult) {
	for {
		select {
		case <-self.Delay:
			self.Ticker = time.Tick(time.Minute)
			fetchFeed(self.URL, results)
		case <-self.Ticker:
			fetchFeed(self.URL, results)
		}
	}
}

func fetchFeed(url string, results chan FetchResult) {
	resp, err := http.Get(url)
	if err != nil {
		logger.Fatal(err)
		return
	}
	if resp.StatusCode == 404 {
		logger.Printf("%q returned 404", url)
		return
	}
	var decoder = xml.NewDecoder(resp.Body)
	decoder.CharsetReader = charset.NewReader // Needed for non-UTF-8 encoded feeds.
	results <- FetchResult{decoder, url}
}

func clean(s string) string {
	s = sanitize.HTML(s)
	s = strings.Trim(s, " ")
	if len(s) > characterCount {
		i := characterCount - 1
		c := string(s[i])
		for c != " " {
			i--
			c = string(s[i])
		}
		switch string(s[i-1]) {
		case ".", ",":
			return s[:i-1] + "\u2026"
		default:
			return s[:i] + "\u2026"
		}
	}
	return s
}

func parseFeed(obj FetchResult) Feed {
	var feed = Feed{
		Title:      "Untitled",
		Website:    "http://example.com/",
		URL:        obj.URL,
		LastUpdate: time.Now().UTC().Format(utcTimestampFmt),
		Items:      []FeedItem{},
	}
	for {
		t, _ := obj.Content.Token()
		if t == nil {
			break
		}
		if e, ok := t.(xml.StartElement); ok {
			switch e.Name.Local {
			case "rss":
				var rss = RSS{}
				obj.Content.DecodeElement(&rss, &e)
				feed.Title = rss.Title
				feed.Website = rss.Website()
				feed.Description = rss.Description
				for idx, item := range rss.Items {
					if idx > maxFeedItems-1 {
						break
					}

					// Check if we've seen this item before
					if found := history[item.Guid()]; found {
						break
					}

					history[item.Guid()] = true
					title, body := item.River()

					feed.Items = append(feed.Items, FeedItem{
						Title:     clean(title),
						Body:      clean(body),
						Link:      item.Link,
						Permalink: item.Permalink,
						PubDate:   item.Timestamp(),
						Id:        fmt.Sprintf("%07d", <-counter),
					})
				}
			case "feed":
				var atom = Atom{}
				obj.Content.DecodeElement(&atom, &e)
				feed.Title = atom.Title
				feed.Website = atom.Website()
				feed.Description = atom.Description
			}
		}
	}
	if feed.Title == "" {
		feed.Title = "Untitled"
	}
	return feed
}

func buildRiver(c chan FetchResult) {
	var river = River{
		Metadata: map[string]string{
			"docs":    "http://riverjs.org",
			"version": "3",
		},
	}
	for obj := range c {
		feed := parseFeed(obj)

		if len(feed.Items) == 0 {
			logger.Printf("%q had no new items", obj.URL)
			continue
		}

		logger.Printf("Updating: %v with %v new items", feed.Title, len(feed.Items))

		writer, _ := os.Create("river.js")
		encoder := json.NewEncoder(writer)

		// Update the timestamps
		river.Metadata["whenGMT"] = time.Now().UTC().Format(utcTimestampFmt)
		river.Metadata["whenLocal"] = time.Now().Format(localTimestampFmt)

		river.UpdatedFeeds.UpdatedFeed = append([]Feed{feed}, river.UpdatedFeeds.UpdatedFeed...)

		writer.Write([]byte("onGetRiverStream("))
		encoder.Encode(river)
		writer.Write([]byte(")"))
		writer.Sync()
	}
}

func main() {
	var wg sync.WaitGroup
	results := make(chan FetchResult)

	go buildRiver(results)

	go func() {
		var c uint = 1
		for {
			counter <- c
			c += 1
		}
	}()

	rand.Seed(time.Now().UnixNano())

	for _, url := range feedsActive {
		wg.Add(1)

		delayDuration := time.Second * time.Duration(rand.Intn(60))
		tickerDuration := time.Minute
		logger.Printf("%q will first update in %v and every %v after that", url, delayDuration, tickerDuration)

		fetcher := &FeedFetcher{
			Delay: time.After(delayDuration),
			URL:   url,
		}
		go fetcher.Run(results)
	}

	wg.Wait()
}
