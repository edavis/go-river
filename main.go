package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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

func fetchFeed(url string, results chan FetchResult) {
	resp, _ := http.Get(url)
	var decoder = xml.NewDecoder(resp.Body)
	decoder.CharsetReader = charset.NewReader // Needed for non-UTF-8 encoded feeds.
	results <- FetchResult{decoder, url}
}

func clean(s string) string {
	s = sanitize.HTML(s)
	s = strings.Trim(s, " ")
	if len(s) > characterCount {
		// non-breaking space and ellipsis
		return s[:characterCount] + "\u00a0\u2026"
	}
	return s
}

func parseFeed(obj FetchResult) Feed {
	var feed = Feed{
		Title:      "Untitled",
		Website:    "http://example.com/",
		URL:        obj.URL,
		LastUpdate: time.Now().UTC().Format(utcTimestampFmt),
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
		writer, _ := os.Create("river.js")
		var encoder = json.NewEncoder(writer)

		// Update the timestamps
		river.Metadata["whenGMT"] = time.Now().UTC().Format(utcTimestampFmt)
		river.Metadata["whenLocal"] = time.Now().Format(localTimestampFmt)

		var feed = parseFeed(obj)
		logger.Printf("updating %q", feed.Title)

		river.UpdatedFeeds.UpdatedFeed = append(river.UpdatedFeeds.UpdatedFeed, feed)
		writer.Write([]byte("onGetRiverStream("))
		encoder.Encode(river)
		writer.Write([]byte(")"))
		writer.Sync()
	}
}

func main() {
	var results = make(chan FetchResult)
	go buildRiver(results)

	go func() {
		var c uint = 1
		for {
			counter <- c
			c += 1
		}
	}()

	for {
		for _, url := range feeds {
			go fetchFeed(url, results)
		}
		time.Sleep(time.Minute * 15)
	}
}
