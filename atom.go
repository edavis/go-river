package main

import (
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"time"
)

type Atom struct {
	XMLName     xml.Name   `xml:"feed"`
	Title       string     `xml:"title"`
	Link        []AtomLink `xml:"link"`
	Description string     `xml:"subtitle"`
	Items       []AtomItem `xml:"entry"`
}

func (self Atom) Website() string {
	for _, link := range self.Link {
		if link.Type == "text/html" || link.Rel == "" {
			return link.Href
		}
	}
	return "http://example.com"
}

type AtomLink struct {
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// http://www.atomenabled.org/developers/syndication/
type AtomItem struct {
	XMLName xml.Name   `xml:"entry"`
	Id      string     `xml:"id"`
	Title   string     `xml:"title"`
	Updated string     `xml:"updated"`
	Link    []AtomLink `xml:"link"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
}

func (self AtomItem) WebLink() string {
	for _, link := range self.Link {
		if link.Type == "text/html" || link.Rel == "" {
			return link.Href
		}
	}
	return "http://example.com"
}

func (self AtomItem) Guid() string {
	if self.Id != "" {
		return self.Id
	} else {
		h := sha1.New()
		h.Write([]byte(self.Title))
		h.Write([]byte(self.WebLink()))
		return fmt.Sprintf("%x", h.Sum(nil))
	}
}

func (self AtomItem) River() (string, string) {
	var body string
	switch {
	case self.Summary != "":
		body = self.Summary
	case self.Content != "":
		body = self.Content
	}

	switch {
	case self.Title != "" && body != "":
		return self.Title, body
	case self.Title == "" && body != "":
		return body, ""
	case self.Title != "":
		return self.Title, ""
	default:
		return "", ""
	}
}

func (self AtomItem) Timestamp() string {
	if self.Updated != "" {
		formats := []string{
			"2006-01-02T15:04:05-07:00",
			"2006-01-02T15:04:05-0700",
		}
		for _, format := range formats {
			if parsed, err := time.Parse(format, self.Updated); err == nil {
				return parsed.Format(utcTimestampFmt)
			}
			continue
		}
	}
	return time.Now().UTC().Format(utcTimestampFmt)
}
