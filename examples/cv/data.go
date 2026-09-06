package main

import "time"

type Tag uint8

const (
	_tagUndefined Tag = iota
	TagAerospace      // Aerospace
	TagMechanical     // Mechanical
	TagIT             // IT
	TagEmbedded       // Embedded
)

type Heading struct {
	Headline    string
	Description string
	Href        string
}

type PubKind uint8

const (
	_pubUndefined  PubKind = iota
	PubJournal             // Journal article.
	PubProceedings         // Conference proceedings.
	PubBook                // Book.
	PubBookChapter         // Chapter within a book.
)

// Publication is a citation. Authors, venue and volume are fields rather than
// SubEvents so the renderer can format the citation itself, and so the ITBA
// faculty form's refereed/foreign buckets can be regenerated from the data.
type Publication struct {
	Heading // Headline is the title, Href the DOI or landing page.
	// Refereed and Foreign sort a publication into the four buckets the ITBA
	// form asks for: revistas con/sin referato, argentinas/extranjeras.
	Refereed bool
	Foreign  bool
	// Cited marks a publication written by other people, listed because their
	// work builds on this author's open-source libraries. It is rendered with a
	// dagger and a footnote so no reader takes it for authorship.
	Cited    bool
	Kind     PubKind
	Authors  []string
	Venue    string
	Volume   string
	Date     time.Time
	Location string
	Tags     []Tag
}

type Event struct {
	Heading
	// Tags mark which CV variants an event belongs to. An event with no tags
	// is unconditional; one with tags is kept only by variants that want them.
	Tags       []Tag
	Start, End time.Time
	Location   string
	SubEvents  []Event
}
