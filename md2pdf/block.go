package md2pdf

import "strings"

// Kind is which markdown construct a [Block] is.
type Kind uint8

const (
	KindParagraph Kind = iota
	KindHeading
	KindList
	KindTable
	KindCode
	KindQuote
	KindRule
	KindImage
)

// CellAlign is a table column's alignment, as the colons of its delimiter row
// spell it.
type CellAlign uint8

const (
	AlignNone   CellAlign = iota // "---": the renderer's to choose
	AlignLeft                    // ":--"
	AlignCenter                  // ":-:"
	AlignRight                   // "--:"
)

// Line is one source line of a paragraph or heading, as prose markup.
type Line struct {
	Markup string
	// Bold is set when every visible character on the line is strong, which is
	// how a source marks a label or a total rather than running text.
	Bold bool
	// Hard is set when the line ended in a hard break rather than a soft one.
	Hard bool
}

// Block is one block-level construct of a markdown source. Which fields it
// uses follows from its Kind.
type Block struct {
	Kind  Kind
	Level int    // KindHeading: 1..6
	Lines []Line // KindParagraph, KindHeading
	// KindQuote: the blocks the quote holds.
	Children []Block
	// KindList: each item's blocks. Start is an ordered list's first number.
	Items   [][]Block
	Ordered bool
	Start   int
	// KindTable: cells as markup, Rows[0] the header. Every row is as wide as
	// Align, which holds each column's alignment.
	Rows  [][]string
	Align []CellAlign
	// KindCode: the source, and Info the fence's info string. KindImage: the
	// picture's path, and Alt its description.
	Src  string
	Info string
	Alt  string
}

// Joined is a paragraph's or heading's lines run together as one passage of
// markup: soft breaks fold into spaces, hard breaks are kept.
func (b *Block) Joined() string {
	var sb strings.Builder
	for i, ln := range b.Lines {
		if i > 0 {
			if b.Lines[i-1].Hard {
				sb.WriteString("<br/>")
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(ln.Markup)
	}
	return sb.String()
}

// Text is what a reader sees of a paragraph or heading: its passage with the
// markup taken out.
func (b *Block) Text() string { return strings.TrimSpace(StripTags(b.Joined())) }

// AllBold reports whether b is a paragraph every line of which is strong.
func (b *Block) AllBold() bool {
	if b.Kind != KindParagraph || len(b.Lines) == 0 {
		return false
	}
	for _, ln := range b.Lines {
		if !ln.Bold {
			return false
		}
	}
	return true
}

// Title is the text of the first top-level heading in bs, or failing one of the
// first heading of any level. It is empty when bs has no heading.
func Title(bs []Block) string {
	first := -1
	for i := range bs {
		if bs[i].Kind != KindHeading {
			continue
		}
		if bs[i].Level == 1 {
			return bs[i].Text()
		}
		if first < 0 {
			first = i
		}
	}
	if first < 0 {
		return ""
	}
	return bs[first].Text()
}
