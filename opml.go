package main

import (
	"encoding/xml"
)

type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Body    struct {
		Outlines []struct {
			Text   string `xml:"text,attr"`
			Type   string `xml:"type,attr"`
			XmlUrl string `xml:"xmlUrl,attr"`
		} `xml:"outline"`
	} `xml:"body"`
}
