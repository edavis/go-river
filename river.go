package main

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
