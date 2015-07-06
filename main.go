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
	"path"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
	"code.google.com/p/go-uuid/uuid"
	"github.com/kennygrant/sanitize"
	"gopkg.in/yaml.v2"
)

const (
	utcTimestampFmt   = "Mon, 02 Jan 2006 15:04:05" + " GMT"
	localTimestampFmt = "Mon, 02 Jan 2006 15:04:05 MST"
	maxFeedItems      = 5
	characterCount    = 280
	updatedFeedCount  = 300
)

var (
	logger      = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	history     = make(map[string]bool)
	activeFeeds = make(map[string]*FeedFetcher)
)

type River struct {
	UpdatedFeeds struct {
		UpdatedFeed []Feed `json:"updatedFeed"`
	} `json:"updatedFeeds"`
	Metadata map[string]string `json:"metadata"`
}

type FetchResult struct {
	Content *xml.Decoder
	URL     string
}

type FeedFetcher struct {
	Poll   time.Duration
	Ticker *time.Ticker
	Delay  <-chan time.Time
	URL    string
}

func (self *FeedFetcher) Run(results chan FetchResult) {
	for {
		select {
		case <-self.Delay:
			self.Ticker = time.NewTicker(self.Poll)
			fetchFeed(self.URL, results)
		case <-self.Ticker.C:
			fetchFeed(self.URL, results)
		}
	}
}

func fetchFeed(url string, results chan FetchResult) {
	resp, err := http.Get(url)
	if err != nil {
		logger.Printf("http.Get error: %v", err)
		return
	}
	if resp.StatusCode == 404 {
		logger.Printf("%q returned 404", url)
		return
	}
	decoder := xml.NewDecoder(resp.Body)
	decoder.CharsetReader = charset.NewReader // Needed for non-UTF-8 encoded feeds.
	results <- FetchResult{decoder, url}
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
						Id:        uuid.New(),
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
						Id:        uuid.New(),
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

func buildRiver(c chan FetchResult, output string) {
	river := River{
		Metadata: map[string]string{
			"docs":    "http://riverjs.org",
			"version": "3",
			"secs":    "",
		},
	}
	for obj := range c {
		feed := parseFeed(obj)

		if len(feed.Items) == 0 {
			continue
		}

		logger.Printf("Updating: %v with %v new items", feed.Title, len(feed.Items))

		writer, _ := os.Create(output)
		encoder := json.NewEncoder(writer)

		// Update the timestamps
		river.Metadata["whenGMT"] = time.Now().UTC().Format(utcTimestampFmt)
		river.Metadata["whenLocal"] = time.Now().Format(localTimestampFmt)

		river.UpdatedFeeds.UpdatedFeed = append([]Feed{feed}, river.UpdatedFeeds.UpdatedFeed...)

		if len(river.UpdatedFeeds.UpdatedFeed) > updatedFeedCount {
			river.UpdatedFeeds.UpdatedFeed = river.UpdatedFeeds.UpdatedFeed[:updatedFeedCount]
		}

		writer.Write([]byte("onGetRiverStream("))
		encoder.Encode(river)
		writer.Write([]byte(")"))
		writer.Sync()
	}
}

func loadRemoteFeedList(input string) []string {
	feeds := []string{}
	switch path.Ext(input) {
	case ".opml":
		opml := OPML{}
		resp, err := http.Get(input)
		if err != nil {
			logger.Fatal("couldn't load feed list: %v", err)
		}
		decoder := xml.NewDecoder(resp.Body)
		decoder.CharsetReader = charset.NewReader
		decoder.Decode(&opml)
		for _, outline := range opml.Body.Outlines {
			if outline.XmlUrl != "" {
				feeds = append(feeds, outline.XmlUrl)
			}
		}
	case ".txt", ".yaml":
		resp, err := http.Get(input)
		if err != nil {
			logger.Fatal("couldn't load feed list: %v", err)
		}
		data, _ := ioutil.ReadAll(resp.Body)
		yaml.Unmarshal(data, &feeds)
	}
	return feeds
}

func loadLocalFeedList(input string) []string {
	feeds := []string{}
	switch path.Ext(input) {
	case ".opml":
		opml := OPML{}
		fp, err := os.Open(input)
		if err != nil {
			logger.Fatal("couldn't load feed list: %v", err)
		}
		decoder := xml.NewDecoder(fp)
		decoder.CharsetReader = charset.NewReader
		decoder.Decode(&opml)
		for _, outline := range opml.Body.Outlines {
			if outline.XmlUrl != "" {
				feeds = append(feeds, outline.XmlUrl)
			}
		}
	case ".txt", ".yaml":
		data, err := ioutil.ReadFile(input)
		if err != nil {
			logger.Fatal("couldn't load feed list: %v", err)
		}
		yaml.Unmarshal(data, &feeds)
	}
	return feeds
}

func pollFeedList(input string, feedPoll, listPoll time.Duration, results chan FetchResult) {
	ticker := time.Tick(listPoll)
	for {
		select {
		case <-ticker:
			logger.Printf("Polling %q", input)

			var feeds []string
			if startsWith(input, "http://", "https://") {
				feeds = loadRemoteFeedList(input)
			} else {
				feeds = loadLocalFeedList(input)
			}

			// Build a map of the just fetched feeds.
			feedsMap := make(map[string]bool)
			for _, url := range feeds {
				feedsMap[url] = true
			}

			// First, check if any feeds have been removed
			for url := range activeFeeds {
				if !feedsMap[url] {
					logger.Printf("Removing feed: %s", url)
					fetcher := activeFeeds[url]
					fetcher.Ticker.Stop()
					delete(activeFeeds, url)
				}
			}

			// Second, check if any feeds have been added
			for _, url := range feeds {
				// Skip if already present
				if _, found := activeFeeds[url]; found {
					continue
				}

				logger.Printf("Adding feed: %s", url)

				t := new(time.Ticker)
				fetcher := &FeedFetcher{
					Poll:   feedPoll,
					Ticker: t,
					Delay:  time.After(time.Duration(0)), // check immediately
					URL:    url,
				}
				activeFeeds[url] = fetcher
				go fetcher.Run(results)
			}
		}
	}
}

func startsWith(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func main() {
	var wg sync.WaitGroup
	results := make(chan FetchResult)
	rand.Seed(time.Now().UnixNano())

	input := flag.String("input", "", "read feed URLs from this file or URL")
	poll := flag.Duration("poll", time.Hour, "how often to poll feeds")
	listPoll := flag.Duration("listpoll", 15*time.Minute, "how often to check feed list for new feeds")
	output := flag.String("output", "river.js", "write output to this file")
	quickstart := flag.Bool("quickstart", false, "when true, don't space out initial feed reads")
	flag.Parse()

	if *input == "" {
		fmt.Println("no -input given, exiting...")
		return
	}

	go buildRiver(results, *output)

	var feeds []string
	if startsWith(*input, "http://", "https://") {
		feeds = loadRemoteFeedList(*input)
	} else {
		feeds = loadLocalFeedList(*input)
	}
	go pollFeedList(*input, *poll, *listPoll, results)

	logger.Printf("Loading %d feeds from %q and writing to %q", len(feeds), *input, *output)

	for _, url := range feeds {
		wg.Add(1)

		if _, found := activeFeeds[url]; found {
			logger.Printf("%q already added, skipping", url)
			continue
		}

		var delayDuration time.Duration
		if *quickstart {
			delayDuration = time.Duration(0)
			logger.Printf("%q will update every %v", url, *poll)
		} else {
			delayDuration = time.Minute * time.Duration(rand.Intn(60))
			logger.Printf("%q will first update in %v and then every %v", url, delayDuration, *poll)
		}

		t := new(time.Ticker)
		fetcher := &FeedFetcher{
			Poll:   *poll,
			Ticker: t,
			Delay:  time.After(delayDuration),
			URL:    url,
		}
		activeFeeds[url] = fetcher
		go fetcher.Run(results)
	}

	wg.Wait()
}
