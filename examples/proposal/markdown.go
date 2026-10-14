package main

import (
	"bytes"
	"errors"
	"html"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/soypat/ohimark"
)

// The markdown is read into blocks before anything is set, because how a block
// is set depends on its neighbours: a heading is held with what follows it, a
// total belongs to the table above it, and a bold label owns the blocks under
// it. ohimark's event stream says what a construct is; these say where it sits.

type blockKind uint8

const (
	bPara blockKind = iota
	bHeading
	bList
	bTable
	bCode
	bQuote
	bRule
	bImage
)

// line is one source line of a paragraph or heading, as prose markup.
type line struct {
	markup string
	// bold is set when every visible character on the line is strong, which is
	// how the source marks a label or a total rather than running text.
	bold bool
	// hard is set when the line ended in a hard break rather than a soft one.
	hard bool
}

type block struct {
	kind     blockKind
	level    int    // bHeading: 1..6
	lines    []line // bPara, bHeading
	children []block
	// bList.
	items   [][]block
	ordered bool
	start   int
	// bTable: cells as markup, rows[0] the header. align is per column.
	rows  [][]string
	align []align
	// bCode: the source. bImage: the destination.
	text string
	alt  string // bImage
}

type align uint8

const (
	alignNone align = iota
	alignLeft
	alignCenter
	alignRight
)

// joined is a paragraph's or heading's lines run together as one passage: soft
// breaks fold into spaces, hard breaks are kept.
func (b *block) joined() string {
	var sb strings.Builder
	for i, ln := range b.lines {
		if i > 0 {
			if b.lines[i-1].hard {
				sb.WriteString("<br/>")
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(ln.markup)
	}
	return sb.String()
}

// allBold reports whether every line of a paragraph is strong.
func (b *block) allBold() bool {
	if b.kind != bPara || len(b.lines) == 0 {
		return false
	}
	for _, ln := range b.lines {
		if !ln.bold {
			return false
		}
	}
	return true
}

// mdReader walks ohimark's events over a source held whole in memory, so that a
// node's span can be sliced rather than read back.
type mdReader struct {
	p   ohimark.Parser
	src []byte
	// codeColor is how a code span is marked, the markup dialect having no
	// monospace of its own.
	codeColor string
}

func parseMarkdown(name string, src []byte, codeColor string) ([]block, error) {
	m := &mdReader{src: src, codeColor: codeColor}
	if err := m.p.Reset(name, bytes.NewReader(src), make([]byte, 4096)); err != nil {
		return nil, err
	}
	n, err := m.p.Next()
	if err != nil {
		return nil, err
	}
	if n.Kind != ohimark.KindDocument || !n.IsOpen() {
		return nil, errors.New("markdown: stream does not open with a document")
	}
	return m.blocks(ohimark.KindDocument)
}

func (m *mdReader) raw(n ohimark.Node) string { return string(m.src[n.Start:n.End]) }

// blocks reads blocks until the close of the container until.
func (m *mdReader) blocks(until ohimark.Kind) (out []block, err error) {
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
		switch n.Kind {
		case ohimark.KindParagraph:
			b, err := m.paragraph()
			if err != nil {
				return out, err
			}
			out = append(out, b)
		case ohimark.KindHeading:
			r, err := m.inlines(ohimark.KindHeading)
			if err != nil {
				return out, err
			}
			out = append(out, block{kind: bHeading, level: int(n.Attr), lines: r.lines})
		case ohimark.KindThematicBreak:
			out = append(out, block{kind: bRule})
		case ohimark.KindCodeBlock:
			b, err := m.code()
			if err != nil {
				return out, err
			}
			out = append(out, b)
		case ohimark.KindBlockQuote:
			children, err := m.blocks(ohimark.KindBlockQuote)
			if err != nil {
				return out, err
			}
			out = append(out, block{kind: bQuote, children: children})
		case ohimark.KindList:
			b, err := m.list(n)
			if err != nil {
				return out, err
			}
			out = append(out, b)
		}
	}
}

func (m *mdReader) list(open ohimark.Node) (block, error) {
	b := block{kind: bList, ordered: open.Flags&ohimark.FlagOrdered != 0}
	if b.ordered {
		b.start = int(open.Attr)
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
			b.items = append(b.items, item)
		}
	}
}

func (m *mdReader) code() (block, error) {
	var sb strings.Builder
	for {
		n, err := m.p.Next()
		if err != nil {
			return block{}, err
		}
		if n.Kind == ohimark.KindCodeBlock && n.IsClose() {
			return block{kind: bCode, text: strings.TrimRight(sb.String(), "\n")}, nil
		}
		if n.Kind == ohimark.KindRaw {
			sb.WriteString(m.raw(n))
		}
	}
}

// paragraph reads one paragraph, which may turn out to be a table — ohimark
// reads CommonMark, which has none, so a pipe table arrives as a paragraph and
// is recognised by its delimiter row — or a picture standing alone.
func (m *mdReader) paragraph() (block, error) {
	r, err := m.inlines(ohimark.KindParagraph)
	if err != nil {
		return block{}, err
	}
	if t, ok := m.table(m.raw(r.close)); ok {
		return t, err
	}
	if r.image != nil && !r.other {
		return *r.image, nil
	}
	return block{kind: bPara, lines: r.lines}, nil
}

type inlineResult struct {
	lines []line
	close ohimark.Node
	image *block // the first picture met
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
		img     *block
		visible bool // the line has visible text
		plain   bool // some of it outside strong
	)
	flush := func(hard bool) {
		r.lines = append(r.lines, line{markup: sb.String(), bold: visible && !plain, hard: hard})
		sb.Reset()
		visible, plain = false, false
	}
	text := func(s string) {
		if img != nil {
			img.alt += s
			return
		}
		if strings.TrimSpace(s) != "" {
			visible, r.other = true, true
			if strong == 0 {
				plain = true
			}
		}
		if inLink {
			sb.WriteString(escape(s))
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
				r.close = n
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
				img = &block{kind: bImage}
			} else {
				if r.image == nil {
					r.image = img
				} else {
					r.other = true
				}
				// A picture inside running text is named, not drawn.
				sb.WriteString("<i>" + escape(img.alt) + "</i>")
				img = nil
			}
		case ohimark.KindDest:
			switch {
			case img != nil:
				img.text = raw
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
			sb.WriteString(`<a href="` + escapeAttr(href) + `">` + escape(dest) + "</a>")
		}
	}
}

var (
	delimCell = regexp.MustCompile(`^\s*:?-+:?\s*$`)
	// bareLink finds the addresses a reader expects to click: a URL, or an email.
	bareLink = regexp.MustCompile(`https?://[^\s<>|]*[^\s<>|.,;:!?)]|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

// table reads a GFM pipe table out of a paragraph's source, if it is one.
func (m *mdReader) table(src string) (block, bool) {
	lines := strings.Split(strings.TrimSpace(src), "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "|") {
		return block{}, false
	}
	delim := splitRow(lines[1])
	if len(delim) == 0 {
		return block{}, false
	}
	b := block{kind: bTable, align: make([]align, len(delim))}
	for i, d := range delim {
		if !delimCell.MatchString(d) {
			return block{}, false
		}
		d = strings.TrimSpace(d)
		l, r := strings.HasPrefix(d, ":"), strings.HasSuffix(d, ":")
		switch {
		case l && r:
			b.align[i] = alignCenter
		case r:
			b.align[i] = alignRight
		case l:
			b.align[i] = alignLeft
		}
	}
	for i, ln := range lines {
		if i == 1 {
			continue
		}
		cells := splitRow(ln)
		row := make([]string, len(delim))
		for j := range row {
			if j < len(cells) {
				row[j] = m.cell(cells[j])
			}
		}
		b.rows = append(b.rows, row)
	}
	return b, true
}

// cell converts one table cell's source to markup by parsing it on its own. A
// cell that reads as anything but one paragraph — "- x" is a list — is taken
// literally.
func (m *mdReader) cell(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	blocks, err := parseMarkdown("cell", []byte(src), m.codeColor)
	if err != nil || len(blocks) != 1 || blocks[0].kind != bPara {
		return escape(src)
	}
	return blocks[0].joined()
}

// splitRow splits a table row at its unescaped pipes, dropping the outer ones.
func splitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, `\|`) {
		s = s[:len(s)-1]
	}
	var cells []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '|':
			cells = append(cells, strings.ReplaceAll(s[start:i], `\|`, "|"))
			start = i + 1
		}
	}
	return append(cells, strings.ReplaceAll(s[start:], `\|`, "|"))
}

// escape makes text safe to set as markup. The dialect resolves only &amp;,
// &lt;, &gt; and &#160;, so those are all it may use.
func escape(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	return markupEscaper.Replace(s)
}

var markupEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// escapeAttr makes a link target safe to quote in a tag.
func escapeAttr(s string) string {
	return escape(strings.ReplaceAll(s, `"`, "%22"))
}

// linkify escapes text and turns the addresses in it into links.
func linkify(s string) string {
	locs := bareLink.FindAllStringIndex(s, -1)
	if locs == nil {
		return escape(s)
	}
	var sb strings.Builder
	prev := 0
	for _, l := range locs {
		addr := s[l[0]:l[1]]
		href := addr
		if !strings.Contains(addr, "://") {
			href = "mailto:" + addr
		}
		sb.WriteString(escape(s[prev:l[0]]))
		sb.WriteString(`<a href="` + escapeAttr(href) + `">` + escape(addr) + "</a>")
		prev = l[1]
	}
	sb.WriteString(escape(s[prev:]))
	return sb.String()
}

// stripTags is what a reader sees of markup: the text, entities resolved.
func stripTags(s string) string {
	var sb strings.Builder
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:i])
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			break
		}
		s = s[i+j+1:]
	}
	return html.UnescapeString(sb.String())
}

// sentenceCase recases a heading the source set in capitals. A heading with any
// lowercase in it was cased deliberately and is left alone.
func sentenceCase(s string) string {
	for _, r := range s {
		if unicode.IsLower(r) {
			return s
		}
	}
	rs := []rune(strings.ToLower(s))
	for i, r := range rs {
		if unicode.IsLetter(r) {
			rs[i] = unicode.ToUpper(r)
			break
		}
	}
	return string(rs)
}
