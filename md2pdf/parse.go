package md2pdf

import (
	"bytes"
	"errors"
	"html"
	"io"
	"strings"

	"github.com/soypat/ohimark"
)

// ParseOptions are the choices [Parse] makes on the renderer's behalf, because
// they end up in the blocks' markup.
type ParseOptions struct {
	// CodeColor is the colour a code span is set in, as "#rrggbb", the markup
	// dialect having no monospace of its own. Empty leaves code spans unmarked.
	CodeColor string
}

// Parse reads markdown into blocks. name is what errors call the source.
//
// The markdown is read whole before anything is set, because how a block is
// set depends on its neighbours: a heading is held with what follows it, a
// total belongs to the table above it, and a bold label owns the blocks under
// it. ohimark's event stream says what a construct is; blocks say where it sits.
func Parse(name string, src []byte, opt ParseOptions) ([]Block, error) {
	m := &mdReader{src: src, codeColor: opt.CodeColor}
	if err := m.p.Reset(name, bytes.NewReader(src), make([]byte, 4096)); err != nil {
		return nil, err
	}
	n, err := m.p.Next()
	if err != nil {
		return nil, err
	}
	if n.Kind != ohimark.KindDocument || !n.IsOpen() {
		return nil, errors.New("md2pdf: stream does not open with a document")
	}
	return m.blocks(ohimark.KindDocument)
}

// mdReader walks ohimark's events over a source held whole in memory, so that a
// node's span can be sliced rather than read back.
type mdReader struct {
	p         ohimark.Parser
	src       []byte
	codeColor string
}

func (m *mdReader) raw(n ohimark.Node) string { return string(m.src[n.Start:n.End]) }

// blocks reads blocks until the close of the container until.
func (m *mdReader) blocks(until ohimark.Kind) (out []Block, err error) {
	for {
		n, err := m.p.Next()
		if err == io.EOF {
			return out, nil
		} else if err != nil {
			return out, err
		}
		if n.IsClose() && n.Kind == until {
			return out, nil
		}
		var b Block
		switch n.Kind {
		case ohimark.KindParagraph:
			b, err = m.paragraph()
		case ohimark.KindHeading:
			var r inlineResult
			r, err = m.inlines(ohimark.KindHeading)
			b = Block{Kind: KindHeading, Level: int(n.Attr), Lines: r.lines}
		case ohimark.KindThematicBreak:
			b = Block{Kind: KindRule}
		case ohimark.KindCodeBlock:
			b, err = m.code()
		case ohimark.KindBlockQuote:
			b.Kind = KindQuote
			b.Children, err = m.blocks(ohimark.KindBlockQuote)
		case ohimark.KindList:
			b, err = m.list(n)
		case ohimark.KindTable:
			b, err = m.table(n)
		default:
			continue
		}
		if err != nil {
			return out, err
		}
		out = append(out, b)
	}
}

func (m *mdReader) list(open ohimark.Node) (Block, error) {
	b := Block{Kind: KindList, Ordered: open.Flags&ohimark.FlagOrdered != 0}
	if b.Ordered {
		b.Start = int(open.Attr)
	}
	for {
		n, err := m.p.Next()
		if err != nil {
			return b, err
		}
		switch {
		case n.Kind == ohimark.KindList && n.IsClose():
			return b, nil
		case n.Kind == ohimark.KindItem && n.IsOpen():
			item, err := m.blocks(ohimark.KindItem)
			if err != nil {
				return b, err
			}
			b.Items = append(b.Items, item)
		}
	}
}

func (m *mdReader) code() (Block, error) {
	b := Block{Kind: KindCode}
	var sb strings.Builder
	for {
		n, err := m.p.Next()
		if err != nil {
			return b, err
		}
		switch n.Kind {
		case ohimark.KindCodeBlock:
			if n.IsClose() {
				b.Src = strings.TrimRight(sb.String(), "\n")
				return b, nil
			}
		case ohimark.KindInfo:
			b.Info = strings.TrimSpace(m.raw(n))
		case ohimark.KindRaw:
			sb.WriteString(m.raw(n))
		}
	}
}

// table reads a GFM pipe table. A row with fewer cells than its table has
// columns is padded with empty ones, which ohimark leaves to the renderer.
func (m *mdReader) table(open ohimark.Node) (Block, error) {
	b := Block{Kind: KindTable, Align: make([]CellAlign, max(int(open.Attr), 0))}
	var row []string
	for {
		n, err := m.p.Next()
		if err != nil {
			return b, err
		}
		switch {
		case n.Kind == ohimark.KindTable && n.IsClose():
			return b, nil
		case n.Kind == ohimark.KindTableRow && n.IsOpen():
			row = make([]string, 0, len(b.Align))
		case n.Kind == ohimark.KindTableRow && n.IsClose():
			for len(row) < len(b.Align) {
				row = append(row, "")
			}
			b.Rows = append(b.Rows, row)
		case n.Kind == ohimark.KindTableCell && n.IsOpen():
			r, err := m.inlines(ohimark.KindTableCell)
			if err != nil {
				return b, err
			}
			j := len(row)
			if j >= len(b.Align) {
				continue
			}
			if len(b.Rows) == 0 {
				// The header row carries its column's alignment.
				b.Align[j] = cellAlign(n.Attr)
			}
			cell := Block{Lines: r.lines}
			row = append(row, cell.Joined())
		}
	}
}

func cellAlign(attr int32) CellAlign {
	switch attr {
	case ohimark.AlignLeft:
		return AlignLeft
	case ohimark.AlignRight:
		return AlignRight
	case ohimark.AlignCenter:
		return AlignCenter
	}
	return AlignNone
}

// paragraph reads one paragraph, which may turn out to be a picture standing
// alone.
func (m *mdReader) paragraph() (Block, error) {
	r, err := m.inlines(ohimark.KindParagraph)
	if err != nil {
		return Block{}, err
	}
	if r.image != nil && !r.other {
		return *r.image, nil
	}
	return Block{Kind: KindParagraph, Lines: r.lines}, nil
}

type inlineResult struct {
	lines []Line
	image *Block // the first picture met
	other bool   // anything visible besides the picture
}

// inlines converts a leaf block's inline events to markup, one line per source
// line, up to and including the close of kind.
func (m *mdReader) inlines(kind ohimark.Kind) (r inlineResult, err error) {
	var (
		sb      strings.Builder
		strong  int
		inLink  bool // inside a link, where text is not linkified again
		opened  bool // the link's <a> has been written
		img     *Block
		visible bool // the line has visible text
		plain   bool // some of it outside strong
	)
	flush := func(hard bool) {
		r.lines = append(r.lines, Line{Markup: sb.String(), Bold: visible && !plain, Hard: hard})
		sb.Reset()
		visible, plain = false, false
	}
	text := func(s string) {
		if img != nil {
			img.Alt += s
			return
		}
		if strings.TrimSpace(s) != "" {
			visible, r.other = true, true
			if strong == 0 {
				plain = true
			}
		}
		if inLink {
			sb.WriteString(Escape(s))
		} else {
			sb.WriteString(linkify(s))
		}
	}
	for {
		n, err := m.p.Next()
		if err != nil {
			return r, err
		}
		raw := m.raw(n)
		switch n.Kind {
		case kind:
			if n.IsClose() {
				flush(false)
				return r, nil
			}
		case ohimark.KindText:
			text(raw)
		case ohimark.KindEscape:
			text(raw[1:])
		case ohimark.KindEntity:
			text(html.UnescapeString(raw))
		case ohimark.KindSoftBreak:
			flush(false)
		case ohimark.KindHardBreak:
			flush(true)
		case ohimark.KindStrong:
			if n.IsOpen() {
				strong++
				sb.WriteString("<b>")
			} else {
				strong--
				sb.WriteString("</b>")
			}
		case ohimark.KindEmph:
			if n.IsOpen() {
				sb.WriteString("<i>")
			} else {
				sb.WriteString("</i>")
			}
		case ohimark.KindCodeSpan:
			if m.codeColor == "" {
				break
			}
			if n.IsOpen() {
				sb.WriteString(`<font color="` + m.codeColor + `">`)
			} else {
				sb.WriteString("</font>")
			}
		case ohimark.KindLink:
			if n.IsOpen() {
				inLink, opened = true, false
			} else {
				if opened {
					sb.WriteString("</a>")
				}
				inLink = false
			}
		case ohimark.KindImage:
			if n.IsOpen() {
				img = &Block{Kind: KindImage}
			} else {
				if r.image == nil {
					r.image = img
				} else {
					r.other = true
				}
				// A picture inside running text is named, not drawn.
				sb.WriteString("<i>" + Escape(img.Alt) + "</i>")
				img = nil
			}
		case ohimark.KindDest:
			switch {
			case img != nil:
				img.Src = raw
			case inLink:
				sb.WriteString(`<a href="` + escapeAttr(raw) + `">`)
				opened = true
			}
		case ohimark.KindAutolink:
			dest := strings.Trim(raw, "<>")
			href := dest
			if strings.Contains(dest, "@") && !strings.Contains(dest, ":") {
				href = "mailto:" + dest
			}
			visible, r.other = true, true
			if strong == 0 {
				plain = true
			}
			sb.WriteString(`<a href="` + escapeAttr(href) + `">` + Escape(dest) + "</a>")
		}
	}
}
