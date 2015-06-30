package main

import (
	_ "encoding/json"
	"encoding/xml"
	"log"
	"net/http"
	"os"
	"time"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

const (
	timestampFmt = "Mon, 02 Jan 2006 15:04:05 GMT"
)

type Feed struct {
	Title       string
	URL         string
	Website     string
	Description string
	LastUpdate  string
	Type        string
}

type FetchResult struct {
	Content *xml.Decoder
	URL     string
}

var logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

func fetchFeed(url string, results chan FetchResult) {
	resp, _ := http.Get(url)
	var decoder = xml.NewDecoder(resp.Body)
	decoder.CharsetReader = charset.NewReader
	results <- FetchResult{decoder, url}
}

func parseFeed(obj FetchResult) Feed {
	var feed = Feed{
		Title:      "Untitled",
		Website:    "http://example.com/",
		URL:        obj.URL,
		LastUpdate: time.Now().UTC().Format(timestampFmt),
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
				// Use the first non-empty link
				for _, link := range rss.Link {
					if link != "" {
						feed.Website = link
						break
					}
				}
				feed.Description = rss.Description
				feed.Type = "rss"
			case "feed":
				var atom = Atom{}
				obj.Content.DecodeElement(&atom, &e)
				feed.Title = atom.Title
				for _, link := range atom.Link {
					if link.Type == "text/html" {
						feed.Website = link.Href
					}
				}
				feed.Description = atom.Description
				feed.Type = "atom"
			}
		}
	}
	return feed
}

func buildRiver(c chan FetchResult) {
	for obj := range c {
		var feed = parseFeed(obj)
		logger.Printf("%#v", feed)
	}
}

func main() {
	var results = make(chan FetchResult)
	go buildRiver(results)

	for {
		for _, url := range feeds {
			go fetchFeed(url, results)
		}
		time.Sleep(time.Minute * 15)
	}
}
