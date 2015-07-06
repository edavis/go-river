package main

import (
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"time"
)

type RSS struct {
	XMLName     xml.Name  `xml:"rss"`
	Title       string    `xml:"channel>title"`
	Link        []string  `xml:"channel>link"` // Captures both RSS and Atom link elements.
	Description string    `xml:"channel>description"`
	Items       []RSSItem `xml:"channel>item"`
}

func (self RSS) Website() string {
	for _, link := range self.Link {
		if link != "" {
			return link
		}
	}
	return "http://example.com"
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	DcDate      string `xml:"http://purl.org/dc/elements/1.1/ date"`
	Permalink   string `xml:"guid"`
}

func (self RSSItem) Timestamp() string {
	var s string
	switch {
	case self.PubDate != "":
		s = self.PubDate
	case self.DcDate != "":
		s = self.DcDate
	}

	if s != "" {
		formats := []string{
			"Mon, 02 Jan 2006 15:04:05 UTC",
			"Mon, 02 Jan 2006 15:04:05 MST",
			"Mon, 02 Jan 2006 15:04:05 -0700",
			"Mon, 02 Jan 2006 15:04:05 -07:00",
			"Mon, 2 Jan 2006 15:04:05 UTC",
			"Mon, 2 Jan 2006 15:04:05 MST",
			"Mon, 2 Jan 2006 15:04:05 -0700",
			"Mon, 2 Jan 2006 15:04:05 -07:00",
			"02 Jan 2006 15:04:05 UTC",
			"02 Jan 2006 15:04:05 MST",
			"02 Jan 2006 15:04:05 -0700",
			"02 Jan 2006 15:04:05 -07:00",
			"2 Jan 2006 15:04:05 UTC",
			"2 Jan 2006 15:04:05 MST",
			"2 Jan 2006 15:04:05 -0700",
			"2 Jan 2006 15:04:05 -07:00",
			"2006-01-02T15:04:05-07:00",
			"2006-01-02T15:04:05-0700",
		}
		for _, format := range formats {
			if parsed, err := time.Parse(format, s); err == nil {
				return parsed.Format(utcTimestampFmt)
			}
			continue
		}
	}
	return time.Now().UTC().Format(utcTimestampFmt)
}

func (self RSSItem) River() (string, string) {
	if self.Title != "" && self.Description != "" {
		return self.Title, self.Description
	} else if self.Title == "" && self.Description != "" {
		return self.Description, ""
	} else if self.Title != "" {
		return self.Title, ""
	} else {
		return "", ""
	}
}

func (self RSSItem) Guid() string {
	if self.Permalink != "" {
		return self.Permalink
	} else {
		h := sha1.New()
		h.Write([]byte(self.Title))
		h.Write([]byte(self.Link))
		return fmt.Sprintf("%x", h.Sum(nil))
	}
}
