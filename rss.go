package main

import "encoding/xml"

type RSS struct {
	XMLName     xml.Name `xml:"rss"`
	Title       string   `xml:"channel>title"`
	Link        []string `xml:"channel>link"` // Captures both RSS and Atom link elements.
	Description string   `xml:"channel>description"`
}
