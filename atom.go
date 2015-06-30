package main

import "encoding/xml"

type Atom struct {
	XMLName     xml.Name   `xml:"feed"`
	Title       string     `xml:"title"`
	Link        []AtomLink `xml:"link"`
	Description string     `xml:"subtitle"`
}

type AtomLink struct {
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}
