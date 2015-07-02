package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
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
	"gopkg.in/yaml.v2"
)

const (
	utcTimestampFmt   = "Mon, 02 Jan 2006 15:04:05" + " GMT"
	localTimestampFmt = "Mon, 02 Jan 2006 15:04:05 MST"
	maxFeedItems      = 5
	characterCount    = 280
	pollInterval      = time.Hour
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
	Output  string
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
	Output string
}

func (self *FeedFetcher) Run(results chan FetchResult) {
	for {
		select {
		case <-self.Delay:
			self.Ticker = time.Tick(pollInterval)
			fetchFeed(self.URL, results, self.Output)
		case <-self.Ticker:
			fetchFeed(self.URL, results, self.Output)
		}
	}
}

func fetchFeed(url string, results chan FetchResult, output string) {
	resp, err := http.Get(url)
	if err != nil {
		logger.Printf("http.Get error: %v", err)
		return
	}
	if resp.StatusCode == 404 {
		logger.Printf("%q returned 404", url)
		return
	}
	var decoder = xml.NewDecoder(resp.Body)
	decoder.CharsetReader = charset.NewReader // Needed for non-UTF-8 encoded feeds.
	results <- FetchResult{decoder, url, output}
}

func clean(s string) string {
	s = sanitize.HTML(s)
	s = strings.Trim(s, " ")
	const ellipsis = "\u2026"
	if len(s) > characterCount {
		i := characterCount - 1
		c := string(s[i])
		for c != " " {
			i--
			c = string(s[i])
		}
		switch string(s[i-1]) {
		case ".", ",":
			return s[:i-1] + ellipsis
		default:
			return s[:i] + ellipsis
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
				for idx, item := range atom.Items {
					if idx > maxFeedItems-1 {
						break
					}

					if found := history[item.Guid()]; found {
						break
					}

					history[item.Guid()] = true
					title, body := item.River()

					feed.Items = append(feed.Items, FeedItem{
						Title:     clean(title),
						Body:      clean(body),
						Link:      item.WebLink(),
						Permalink: item.Id,
						PubDate:   item.Timestamp(),
						Id:        fmt.Sprintf("%07d", <-counter),
					})
				}
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

		writer, _ := os.Create(obj.Output)
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

type FeedConfig struct {
	Output string
	Feeds  []string
}

func main() {
	var wg sync.WaitGroup
	results := make(chan FetchResult)

	rand.Seed(time.Now().UnixNano())

	go buildRiver(results)

	go func() {
		var c uint = 1
		for {
			counter <- c
			c += 1
		}
	}()

	flag.Parse()
	for _, list := range flag.Args() {
		config := FeedConfig{}
		data, err := ioutil.ReadFile(list)
		if err != nil {
			logger.Fatal("couldn't ready feed list: %v", err)
		}
		yaml.Unmarshal(data, &config)

		for _, url := range config.Feeds {
			wg.Add(1)

			delayDuration := time.Minute * time.Duration(rand.Intn(60))
			logger.Printf("%q will first update in %v and every %v after that", url, delayDuration, pollInterval)

			fetcher := &FeedFetcher{
				Delay:  time.After(delayDuration),
				URL:    url,
				Output: config.Output,
			}
			go fetcher.Run(results)
		}
	}

	wg.Wait()
}
