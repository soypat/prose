// Package prose is a themed document layer over github.com/soypat/piudf: a
// palette, a type scale and the furniture a long document needs — running
// heads, folios, sections, lists, notes, code blocks and tables — so that a
// generator holds its content and not its typography.
//
// A [Kit] makes elements and a [Doc] accumulates them. Every element is a
// piudoc.Drawer, so anything this package does not name can still be built with
// piudoc directly and passed to [Doc.Add].
package prose

import (
	"image/color"

	canvas "github.com/soypat/piudf/piupage"
)

// The families a theme sets text in. They are fixed names rather than a
// caller's choice: which faces back them is [Fonts]' business, and a style free
// to name any family would let a theme and its fonts disagree silently. Neither
// may carry a '-', which piudoc reads as a weight suffix.
const (
	FamilyText = "Text"
	FamilyMono = "Mono"
)

// Weights of the text family, as a Style.Font spells them.
const (
	textRegular = FamilyText
	textBold    = FamilyText + "-Bold"
)

// Palette is a document's colors. Ink is never pure black: on a white page a
// hair of warmth reads as deliberate where #000 reads as a default nobody chose.
type Palette struct {
	Ink    color.Color // headings and emphasis
	Body   color.Color // running text
	Muted  color.Color // labels, folios, running heads
	Hair   color.Color // rules
	Accent color.Color // links, section numbers, list markers
	Band   color.Color // tinted panels
	CodeBg color.Color // the ground under a code block
	Paper  color.Color // text set on a dark panel
	Link   color.Color // a link reversed out of a dark panel
}

// Scale is one style per role the document has. A zero style is filled in from
// the palette, so a caller may override three and leave the rest.
type Scale struct {
	Eyebrow  Style // small label above a title
	Title    Style // the masthead
	Lead     Style // opening statement, set larger than body
	Body     Style
	Small    Style
	H2       Style // section heading
	H3       Style // run-in heading
	Sub      Style // subsection heading
	Cell     Style // table body
	CellHead Style // table header
	Code     Style
	Marker   Style // list marker
	Figure   Style // a large number in a figures band
	Caption  Style // its label beneath
	Margin   Style // a label in the margin, e.g. a year beside an entry
	Panel    Style // text reversed out of a dark panel
}

// Rhythm is the vertical spacing that separates a document's parts.
type Rhythm struct {
	SectionBefore float64 // above a section's rule
	SectionRule   float64 // between that rule and the heading
	SectionAfter  float64 // between the heading and what it introduces
	SubBefore     float64 // above a subsection heading
	PanelPad      Padding // around a note's or a panel's text
	CodePad       Padding // around a code block
	CodeRule      float64 // width of the accent rule down its left edge
}

// Theme is a document's whole visual vocabulary. Restyling for another brand
// touches this value, not the story.
type Theme struct {
	Page     PageSize
	Margins  Margins
	Palette  Palette
	Scale    Scale
	Bullet   BulletStyle // marker and indent for lists
	Pad      Padding     // default cell padding
	Hairline float64     // width of a rule between rows
	Rhythm   Rhythm
}

// DefaultPalette is a near-black ink over white with a single accent, and three
// greys that do nothing but separate.
func DefaultPalette() Palette {
	return Palette{
		Ink:    canvas.HexColor("#12161a"),
		Body:   canvas.HexColor("#272d34"),
		Muted:  canvas.HexColor("#6a7480"),
		Hair:   canvas.HexColor("#dbe1e8"),
		Accent: canvas.HexColor("#1a5fb4"),
		Band:   canvas.HexColor("#f1f5fa"),
		CodeBg: canvas.HexColor("#f5f7f9"),
		Paper:  canvas.HexColor("#ffffff"),
		Link:   canvas.HexColor("#8fc0f5"),
	}
}

// DefaultTheme is A4 with 62pt side margins, which leaves a 471pt measure — at
// the body size a little over 90 characters a line, the range where a reader's
// eye still tracks from one line to the next reliably.
func DefaultTheme() Theme {
	t := Theme{
		Page:     SizeA4(),
		Margins:  Margins{Left: 62, Right: 62, Top: 58, Bottom: 64},
		Palette:  DefaultPalette(),
		Pad:      Padding{Right: 14, Top: 7, Bottom: 7},
		Hairline: 0.5,
		Rhythm: Rhythm{
			SectionBefore: 20, SectionRule: 11, SectionAfter: 9, SubBefore: 8,
			PanelPad: Padding{Left: 16, Right: 16, Top: 13, Bottom: 13},
			CodePad:  PadAll(11),
			CodeRule: 2.2,
		},
	}
	t.fill()
	return t
}

// fill resolves the zero styles in the scale from the palette. A style counts
// as unset when its size is zero, which no drawable style has.
func (t *Theme) fill() {
	p := t.Palette
	body := Style{Font: textRegular, Color: p.Body, Link: LinkStyle{Color: p.Accent}}
	set := func(dst *Style, src Style) {
		if dst.Size == 0 {
			*dst = src
		}
	}
	s := &t.Scale
	set(&s.Eyebrow, Style{Font: textBold, Size: 8, Leading: 11, Color: p.Accent})
	set(&s.Title, Style{Font: textBold, Size: 29, Leading: 33, Color: p.Ink})
	set(&s.Lead, with(body, 12, 18, func(st *Style) { st.Align, st.SpaceAfter = Justify, 9 }))
	set(&s.Body, with(body, 9.8, 14.6, func(st *Style) { st.Align, st.SpaceAfter = Justify, 9 }))
	set(&s.Small, with(body, 8.4, 12.4, func(st *Style) { st.Color = p.Muted }))
	set(&s.H2, Style{Font: textBold, Size: 15, Leading: 19, Color: p.Ink})
	set(&s.H3, Style{Font: textBold, Size: 10.2, Leading: 15, Color: p.Ink, SpaceAfter: 2})
	set(&s.Sub, Style{Font: textBold, Size: 11, Leading: 16, Color: p.Ink, SpaceAfter: 4})
	set(&s.Cell, with(body, 9.2, 13.4, nil))
	set(&s.CellHead, Style{Font: textBold, Size: 7.8, Leading: 11, Color: p.Muted})
	set(&s.Code, Style{Font: FamilyMono, Size: 8.1, Leading: 12.4, Color: p.Ink})
	set(&s.Marker, Style{Font: textBold, Size: 9.8, Leading: 14.6, Color: p.Accent})
	set(&s.Figure, Style{Font: textBold, Size: 15.5, Leading: 18, Color: p.Accent})
	set(&s.Caption, Style{Font: textRegular, Size: 7.8, Leading: 10.4, Color: p.Muted})
	set(&s.Margin, Style{Font: textBold, Size: 9.4, Leading: 13.4, Color: p.Accent})
	set(&s.Panel, with(body, 10.4, 15.6, func(st *Style) {
		st.Align, st.Color, st.Link.Color = Justify, p.Paper, p.Link
	}))

	// A link with no color of its own inherits the span it sits in, which in a
	// heading set in Ink leaves it indistinguishable from the words beside it.
	// Every style that has not named a link color links in the accent; Panel,
	// which reverses out of a dark ground, has already named its own.
	for _, st := range []*Style{
		&s.Eyebrow, &s.Title, &s.Lead, &s.Body, &s.Small, &s.H2, &s.H3, &s.Sub,
		&s.Cell, &s.CellHead, &s.Code, &s.Marker, &s.Figure, &s.Caption,
		&s.Margin, &s.Panel,
	} {
		if st.Link.Color == nil {
			st.Link.Color = p.Accent
		}
	}

	if t.Bullet.Marker == "" {
		// An en dash in the accent, so a list reads as a list without a heavy
		// glyph in the margin.
		t.Bullet = BulletStyle{Marker: "–", Style: s.Marker, Indent: 15}
	}
}

// with derives a style at a size and leading, optionally amended.
func with(base Style, size, leading float64, amend func(*Style)) Style {
	base.Size, base.Leading = size, leading
	if amend != nil {
		amend(&base)
	}
	return base
}

// Measure is the text width the frame flows into.
func (t Theme) Measure() float64 {
	return t.Page.W - t.Margins.Left - t.Margins.Right
}

// hex renders a color as "#rrggbb", which is how markup names one.
func hex(c color.Color) string {
	const digits = "0123456789abcdef"
	if c == nil {
		return "#000000"
	}
	r, g, b, _ := c.RGBA()
	out := []byte("#______")
	for i, n := range [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)} {
		out[1+2*i], out[2+2*i] = digits[n>>4], digits[n&0xf]
	}
	return string(out)
}
