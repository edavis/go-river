package main

import "encoding/xml"

type Atom struct {
	XMLName     xml.Name   `xml:"feed"`
	Title       string     `xml:"title"`
	Link        []AtomLink `xml:"link"`
	Description string     `xml:"subtitle"`
}

func (self Atom) Website() string {
	for _, link := range self.Link {
		if link.Type == "text/html" {
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
