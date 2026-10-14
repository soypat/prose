package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
)

const (
	fontRegular = prose.FamilyText
	fontBold    = prose.FamilyText + "-Bold"
)

// piece is one element of the story and whether it must share a page with the
// element after it.
type piece struct {
	e        prose.Elem
	keepNext bool
}

// layout sets blocks as story elements.
type layout struct {
	k   *prose.Kit
	th  prose.Theme
	dir string // what an image's path is relative to
	// hi is the second colour a proposal's cover is allowed.
	hi color.Color
	// ragged sets passages flush left, for a column too narrow to justify
	// without opening holes between its words.
	ragged bool
	// pages and toc record where each division and section landed, for the
	// contents on the cover.
	pages  *pageFinder
	toc    []tocEntry
	pieces []piece
	err    error
}

// tocEntry is one line of the contents: a division, or a numbered section.
type tocEntry struct {
	part       bool
	num, title string
	page       *int
}

// entry records a heading for the contents and returns the probe that learns
// its page, to be drawn with the heading.
func (l *layout) entry(part bool, num, title string) prose.Elem {
	if l.pages == nil {
		return nil
	}
	e := tocEntry{part: part, num: num, title: title, page: new(int)}
	l.toc = append(l.toc, e)
	return probe{l.pages, e.page}
}

func (l *layout) add(e prose.Elem)  { l.pieces = append(l.pieces, piece{e: e}) }
func (l *layout) hold(e prose.Elem) { l.pieces = append(l.pieces, piece{e: e, keepNext: true}) }

// sub is a layout for blocks set inside another element, a cell or an item.
func (l *layout) sub(bs []block, ragged bool) prose.Elem {
	s := &layout{k: l.k, th: l.th, dir: l.dir, hi: l.hi, ragged: ragged}
	s.blocks(bs)
	if s.err != nil && l.err == nil {
		l.err = s.err
	}
	return l.k.Stack(s.story()...)
}

// story folds the pieces into elements, binding each held piece to the one
// after it. A table takes what it is held to as its lead, so that a heading is
// kept with the table's first rows rather than with the whole of it.
func (l *layout) story() []prose.Elem {
	var out, held []prose.Elem
	for _, p := range l.pieces {
		e := p.e
		if len(held) > 0 {
			if t, ok := e.(*table); ok && t.lead == nil {
				t.lead = l.k.Keep(held...)
			} else {
				e = l.k.Keep(append(held, e)...)
			}
			held = nil
		}
		if p.keepNext {
			held = append(held, e)
			continue
		}
		out = append(out, e)
	}
	return append(out, held...)
}

func (l *layout) blocks(bs []block) {
	for i := 0; i < len(bs); i++ {
		b := &bs[i]
		switch b.kind {
		case bHeading:
			l.heading(b)
		case bPara:
			switch {
			case isTotal(b):
				l.total(b)
			case isLabel(b):
				j := i + 1
				for j < len(bs) && owned(&bs[j]) {
					j++
				}
				l.term(b, bs[i+1:j])
				i = j - 1
			default:
				l.para(b)
			}
		case bList:
			l.list(b)
		case bTable:
			l.table(b)
		case bCode:
			l.add(l.k.Code(b.text))
		case bQuote:
			l.quote(b)
		case bImage:
			l.image(b)
		case bRule:
			// A section draws its own rule, so a break before a heading is
			// drawn already. One between two passages is not.
			if i > 0 && i+1 < len(bs) && bs[i+1].kind != bHeading {
				l.add(l.k.Stack(l.k.Space(4), l.k.Rule(0.5), l.k.Space(12)))
			}
		}
	}
}

// isTotal is two strong lines and nothing else: a label over an amount.
func isTotal(b *block) bool { return b.allBold() && len(b.lines) == 2 }

// isLabel is a paragraph whose first line is strong and whose rest, if any, is
// not: a term and, optionally, its definition.
func isLabel(b *block) bool {
	if b.kind != bPara || len(b.lines) == 0 || !b.lines[0].bold || isTotal(b) {
		return false
	}
	for _, ln := range b.lines[1:] {
		if ln.bold {
			return false
		}
	}
	return true
}

// owned reports whether b belongs to the label above it.
func owned(b *block) bool {
	switch b.kind {
	case bPara:
		return !isLabel(b) && !isTotal(b)
	case bList, bCode, bQuote:
		return true
	}
	return false
}

var (
	sectionNum = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+(.+)$`)
	trailTag   = regexp.MustCompile(`^(.*?)\s*\[([^\]]+)\]$`)
)

// plain is a heading or label as the reader should see it: no trailing colon,
// and recased if the source shouted it.
func plain(b *block) string {
	s := strings.TrimSpace(stripTags(b.joined()))
	return sentenceCase(strings.TrimSpace(strings.TrimSuffix(s, ":")))
}

func (l *layout) heading(b *block) {
	k, r := l.k, l.th.Rhythm
	title := plain(b)
	switch b.level {
	case 1:
		l.part(title)
	case 2:
		if m := sectionNum.FindStringSubmatch(title); m != nil {
			num := m[1]
			if len(num) == 1 {
				num = "0" + num
			}
			name := sentenceCase(m[2])
			l.hold(k.Section(num, escape(name), l.entry(false, num, name)))
			return
		}
		l.hold(k.Keep(
			l.entry(false, "", title),
			k.Space(r.SectionBefore),
			piudoc.HRule{Thickness: 0.9, Color: l.th.Palette.Hair, SpaceAfter: r.SectionRule},
			k.Bookmark(title, 1),
			k.H2(escape(title)),
			k.Space(r.SectionAfter),
		))
	case 3:
		m := trailTag.FindStringSubmatch(title)
		if m == nil {
			l.hold(k.Keep(k.Bookmark(title, 2), k.Subsection(escape(title))))
			return
		}
		// A heading that ends in a bracket — a stage and its duration — sets
		// the bracket as a tag on the heading's right, over a shared rule.
		sub := l.th.Scale.Sub
		sub.SpaceAfter = 0
		t := &piudoc.Table{
			ColWidths: []float64{0, 130},
			Rows: [][]piudoc.Cell{{
				{Drawer: k.Styled(escape(m[1]), sub)},
				{Drawer: k.Styled(escape(strings.ToUpper(m[2])), l.th.Scale.Eyebrow)},
			}},
			Pad: prose.Padding{Bottom: 6},
		}
		t.Style.All().Valign(prose.Bottom)
		t.Style.Col(1).Align(prose.Right)
		t.Style.Row(0).LineBelow(0.6, l.th.Palette.Hair)
		l.hold(k.Keep(k.Space(r.SubBefore+8), k.Bookmark(m[1], 2), t, k.Space(8)))
	default:
		l.hold(k.H3(escape(title)))
	}
}

// part opens one of the proposal's divisions on a page of its own.
func (l *layout) part(title string) {
	k := l.k
	l.add(pageBreak{})
	l.hold(k.Keep(
		l.entry(true, "", title),
		k.Bookmark(title, 0),
		k.Title(escape(title)),
		k.Space(10),
		k.ShortRule(58, 2.4, l.th.Palette.Accent),
		k.Space(8),
	))
}

// body is the running text's style, flush left where the column is narrow.
func (l *layout) body() prose.Style {
	st := l.th.Scale.Body
	if l.ragged {
		st.Align = prose.Left
	}
	return st
}

func (l *layout) para(b *block) {
	text := b.joined()
	if strings.HasSuffix(stripTags(text), ":") {
		// What ends in a colon introduces what follows; it is never left at
		// the foot of a page without it.
		l.hold(l.k.Styled(text, l.body()))
		return
	}
	l.add(l.k.Styled(text, l.body()))
}

func (l *layout) list(b *block) {
	k, th := l.k, l.th
	// A list's items are set ragged, which is how a list scans, and closer
	// than its paragraphs; only the last keeps the full gap to what follows.
	last := th.Scale.Body
	last.Align = prose.Left
	tight := last
	tight.SpaceAfter = 3.5
	for i, item := range b.items {
		bs := th.Bullet
		if b.ordered {
			bs = prose.BulletStyle{Marker: fmt.Sprintf("%d.", b.start+i), Style: th.Scale.Marker, Indent: 18}
		}
		if len(item) == 1 && item[0].kind == bPara {
			st := tight
			if i == len(b.items)-1 {
				st = last
			}
			// A marker is drawn where its item starts, so an item that broke
			// across a page would leave its marker alone at the foot.
			l.add(k.Keep(k.Builder().Bullet(bs, k.Styled(item[0].joined(), st))))
			continue
		}
		l.add(k.Builder().Bullet(bs, l.sub(item, true)))
	}
}

func (l *layout) quote(b *block) {
	for i := range b.children {
		c := &b.children[i]
		if c.kind == bPara {
			l.add(l.k.Stack(l.k.Note(c.joined()), l.k.Space(9)))
		} else {
			l.blocks(b.children[i : i+1])
		}
	}
}

func (l *layout) image(b *block) {
	img, err := prose.OpenImage(os.DirFS(l.dir), b.text)
	if err != nil {
		if l.err == nil {
			l.err = err
		}
		return
	}
	l.add(l.k.Stack(l.k.Figure(img, 0, escape(b.alt)), l.k.Space(12)))
}

// term sets a label beside what it owns, as one row of a ruled two-column
// grid: the way a proposal's conditions — validity, payment, exclusions — are
// read, by scanning the left column for the one in question.
func (l *layout) term(label *block, owns []block) {
	k := l.k
	name := plainLines(label.lines[:1])
	var content []prose.Elem
	if len(label.lines) > 1 {
		// The label's own further lines are an address block, set as written.
		rest := block{kind: bPara, lines: append([]line(nil), label.lines[1:]...)}
		for i := range rest.lines {
			rest.lines[i].hard = true
		}
		st := l.th.Scale.Body
		st.Align = prose.Left
		content = append(content, k.Styled(rest.joined(), st))
	}
	if len(owns) > 0 {
		content = append(content, l.sub(owns, true))
	}
	if len(content) == 0 {
		l.hold(k.Keep(k.Space(4), k.Sub(escape(name))))
		return
	}
	l.add(l.termRow(name, k.Stack(content...)))
}

// termRow is one row of the ruled grid: a label, and beside it what it owns.
func (l *layout) termRow(label string, content prose.Elem) prose.Elem {
	pal := l.th.Palette
	// The label keeps the gap a paragraph leaves, so a label longer than what
	// it owns does not sit on the rule beneath it.
	labelSt := prose.Style{Font: fontBold, Size: 9, Leading: 13, Color: pal.Ink, SpaceAfter: l.th.Scale.Body.SpaceAfter}
	t := &piudoc.Table{
		ColWidths: []float64{150, 0},
		Rows: [][]piudoc.Cell{{
			{Drawer: l.k.Styled(escape(label), labelSt)},
			{Drawer: content},
		}},
		Pad: prose.Padding{Right: 14, Top: 10},
	}
	t.Style.All().Valign(prose.Top)
	t.Style.Col(-1).Pad(prose.Padding{Top: 10})
	t.Style.Row(0).LineAbove(0.5, pal.Hair).LineBelow(0.5, pal.Hair)
	return t
}

// contents lists the divisions and sections at the pages known lands them on.
// The first pass knows none and sets the same rows without numbers, so the
// pass that has them lays out identically.
func (l *layout) contents(known []int) prose.Elem {
	k, pal := l.k, l.th.Palette
	numSt := prose.Style{Font: fontBold, Size: 9.2, Leading: 13, Color: pal.Accent}
	partSt := prose.Style{Font: fontBold, Size: 9.2, Leading: 13, Color: pal.Ink}
	pageSt := prose.Style{Font: fontRegular, Size: 9.2, Leading: 13, Color: pal.Muted}
	rows := make([][]piudoc.Cell, len(l.toc))
	for i, e := range l.toc {
		page := ""
		if i < len(known) && known[i] > 0 {
			page = strconv.Itoa(known[i])
		}
		st := l.th.Scale.Cell
		if e.part {
			st = partSt
		}
		rows[i] = []piudoc.Cell{
			{Drawer: k.Styled(escape(e.num), numSt)},
			{Drawer: k.Styled(escape(e.title), st)},
			{Drawer: k.Styled(page, pageSt)},
		}
	}
	t := &piudoc.Table{ColWidths: []float64{30, 0, 30}, Rows: rows,
		Pad: prose.Padding{Top: 2.5, Bottom: 2.5}}
	t.Style.All().Valign(prose.Top)
	t.Style.Col(-1).Align(prose.Right)
	for i, e := range l.toc {
		if e.part && i > 0 {
			t.Style.Row(i).Pad(prose.Padding{Top: 9, Bottom: 2.5})
		}
	}
	return t
}

// total is a label over an amount. Under a table it becomes the table's foot,
// held to its last row; anywhere else it stands as a bar of its own.
func (l *layout) total(b *block) {
	bar := l.totalBar(plainLines(b.lines[:1]), stripTags(b.lines[1].markup))
	if n := len(l.pieces); n > 0 {
		if t, ok := l.pieces[n-1].e.(*table); ok && t.foot == nil {
			t.foot = bar
			return
		}
	}
	l.add(l.k.Stack(bar, l.k.Space(14)))
}

func (l *layout) totalBar(label, amount string) prose.Elem {
	k, pal := l.k, l.th.Palette
	labelSt := prose.Style{Font: fontBold, Size: 8.4, Leading: 11, Color: pal.Paper}
	amountSt := prose.Style{Font: fontBold, Size: 14, Leading: 17, Color: pal.Paper}
	t := &piudoc.Table{
		ColWidths: []float64{0, 170},
		Rows: [][]piudoc.Cell{{
			{Drawer: k.Styled(escape(strings.ToUpper(label)), labelSt)},
			{Drawer: k.Styled(escape(strings.TrimSpace(amount)), amountSt)},
		}},
		Pad: prose.Padding{Left: 12, Right: 12, Top: 10, Bottom: 10},
	}
	t.Style.All().Background(pal.Ink).Valign(prose.Middle)
	t.Style.Col(1).Align(prose.Right)
	return t
}

// plain over lines rather than a block, for a label cut from a paragraph.
func plainLines(lines []line) string { return plain(&block{kind: bPara, lines: lines}) }

func (l *layout) table(b *block) {
	k, th := l.k, l.th
	ncol := len(b.align)
	if ncol == 0 || len(b.rows) == 0 {
		return
	}
	rows := b.rows
	headless := true
	for _, c := range rows[0] {
		if strings.TrimSpace(stripTags(c)) != "" {
			headless = false
		}
	}
	cellSt, headSt := th.Scale.Cell, th.Scale.CellHead
	if ncol >= 5 {
		// A wide table trades a point of size for a column that does not wrap.
		cellSt.Size, cellSt.Leading = 8.4, 12
	}
	aligns := make([]prose.Align, ncol)
	for j := range aligns {
		switch b.align[j] {
		case alignCenter:
			aligns[j] = prose.Center
		case alignRight:
			aligns[j] = prose.Right
		case alignLeft:
			aligns[j] = prose.Left
		default:
			if numericColumn(rows[1:], j) {
				aligns[j] = prose.Right
			}
		}
	}
	const padX, padY = 6, 6
	widths := colWidths(rows, headless, headSt, cellSt, th.Measure(), 2*padX)

	mk := func(cells []string, st prose.Style, rule float64, ruleCol color.Color) prose.Elem {
		row := make([]piudoc.Cell, ncol)
		for j := range row {
			row[j].Drawer = k.Styled(cells[j], st)
		}
		t := &piudoc.Table{ColWidths: widths, Rows: [][]piudoc.Cell{row},
			Pad: prose.Padding{Left: padX, Right: padX, Top: padY, Bottom: padY}}
		t.Style.All().Valign(prose.Top)
		// The outer columns sit flush with the measure, as the text above does.
		t.Style.Col(0).Pad(prose.Padding{Right: padX, Top: padY, Bottom: padY})
		if ncol > 1 {
			t.Style.Col(-1).Pad(prose.Padding{Left: padX, Top: padY, Bottom: padY})
		}
		for j, a := range aligns {
			t.Style.Col(j).Align(a)
		}
		if rule > 0 {
			t.Style.Row(0).LineBelow(rule, ruleCol)
		}
		return t
	}
	tb := &table{after: 14}
	if !headless {
		head := mk(rows[0], headSt, 0.9, th.Palette.Ink).(*piudoc.Table)
		// Headers of one line and of three all stand on the rule.
		head.Style.All().Valign(prose.Bottom)
		tb.head = head
	}
	body := rows[1:]
	for i, r := range body {
		rule, col := th.Hairline, th.Palette.Hair
		if i == len(body)-1 {
			rule, col = 0.9, th.Palette.Ink // The table's close, which a total sits under.
		}
		tb.rows = append(tb.rows, mk(r, cellSt, rule, col))
	}
	l.add(tb)
}

var numericCell = regexp.MustCompile(`^[\d.,\s]+$`)

// numericColumn reports whether most of a column's cells are amounts, which
// are right-aligned so their places line up.
func numericColumn(rows [][]string, j int) bool {
	num, filled := 0, 0
	for _, r := range rows {
		s := strings.TrimSpace(stripTags(r[j]))
		if s == "" {
			continue
		}
		filled++
		s = strings.NewReplacer("U$S", "", "US$", "", "$", "", "%", "", "€", "").Replace(s)
		if numericCell.MatchString(s) && strings.ContainsAny(s, "0123456789") {
			num++
		}
	}
	return num > 0 && 2*num >= filled
}

// colWidths shares the measure among columns the way a browser lays out an
// automatic table: each gets at least its longest word, and what is left goes
// to the columns in proportion to how much more they would take to set their
// longest cell unbroken.
func colWidths(rows [][]string, headless bool, headSt, cellSt prose.Style, measure, pad float64) []float64 {
	n := len(rows[0])
	lo, hi := make([]float64, n), make([]float64, n)
	for i, row := range rows {
		if i == 0 && headless {
			continue
		}
		st := cellSt
		if i == 0 {
			st = headSt
		}
		for j, c := range row {
			face := piupage.FontHelvetica
			if i == 0 || strings.HasPrefix(strings.TrimSpace(c), "<b>") {
				face = piupage.FontHelveticaBold
			}
			txt := stripTags(c)
			hi[j] = max(hi[j], piupage.StringWidth(face, txt, st.Size))
			for _, w := range strings.Fields(txt) {
				lo[j] = max(lo[j], piupage.StringWidth(face, w, st.Size))
			}
		}
	}
	var sumLo, sumHi float64
	for j := range lo {
		lo[j] += pad + 1
		hi[j] += pad + 1
		sumLo += lo[j]
		sumHi += hi[j]
	}
	w := make([]float64, n)
	switch {
	case sumHi <= measure:
		// Everything fits on one line; the slack goes where the text is.
		for j := range w {
			w[j] = hi[j] + (measure-sumHi)*hi[j]/sumHi
		}
	case sumLo >= measure:
		for j := range w {
			w[j] = lo[j] * measure / sumLo
		}
	default:
		share := (measure - sumLo) / (sumHi - sumLo)
		for j := range w {
			w[j] = lo[j] + (hi[j]-lo[j])*share
		}
	}
	return w
}

// table draws a row at a time, so that a table broken across pages carries
// its header onto each, and holds what comes before and after it to its first
// and last rows: a heading never ends a page above its table, nor does a total
// begin one below it.
type table struct {
	lead  prose.Elem
	head  prose.Elem
	rows  []prose.Elem
	foot  prose.Elem
	after float64
}

func (t *table) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (adv int, y float64, err error) {
	y = yTop
	height := func(es []prose.Elem) (h float64, err error) {
		for _, e := range es {
			if e == nil {
				continue
			}
			eh, err := piudoc.Measure(dst[adv:], e, f, y-h)
			if err != nil {
				return 0, err
			}
			h += eh
		}
		return h, nil
	}
	draw := func(es []prose.Elem) error {
		for _, e := range es {
			if e == nil {
				continue
			}
			n, yn, err := e.Draw(dst[adv:], f, y)
			adv, y = adv+n, yn
			if err != nil {
				return err
			}
		}
		return nil
	}
	n := len(t.rows)
	for i := 0; i < max(n, 1); i++ {
		var group []prose.Elem
		if i == 0 {
			group = append(group, t.lead, t.head)
		}
		if i < n {
			group = append(group, t.rows[i])
		}
		if i >= n-1 {
			group = append(group, t.foot)
		}
		h, err := height(group)
		if err != nil {
			return adv, y, err
		}
		if y-h < f.Bottom && y < f.Top {
			adv++
			if adv >= len(dst) {
				return adv, y, io.ErrShortBuffer
			}
			y = f.Top
			if i > 0 && t.head != nil {
				group = append([]prose.Elem{t.head}, group...)
			}
		}
		if err := draw(group); err != nil {
			return adv, y, err
		}
	}
	return adv, y - t.after, nil
}

// pageFinder learns which page an element lands on. Its anchor is drawn first
// in the story and is handed every page; a probe later in the story is handed
// a tail of those same pages, and the first of them is its own.
type pageFinder struct{ all []piupage.Canvas }

type anchor struct{ p *pageFinder }

func (a anchor) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	a.p.all = dst
	return 0, yTop, nil
}

// probe draws nothing and records its page. Held with a heading, it is
// measured where the heading would go and then drawn where it does go, and
// the later of the two is the one it keeps.
type probe struct {
	p    *pageFinder
	page *int
}

func (pr probe) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	for i := range pr.p.all {
		if &pr.p.all[i] == &dst[0] {
			*pr.page = i + 1
			break
		}
	}
	return 0, yTop, nil
}

// pageBreak starts a fresh page, unless the cursor is at the top of one.
type pageBreak struct{}

func (pageBreak) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	if yTop >= f.Top {
		return 0, yTop, nil
	}
	if len(dst) < 2 {
		return 0, yTop, io.ErrShortBuffer
	}
	return 1, f.Top, nil
}

// backdrop fills the page from its top edge to h below the cursor, bleeding off
// both sides, and sets content over it. The cursor ends at the backdrop's foot
// whatever the content's height, so what follows is placed against the
// ground and not against the text on it.
type backdrop struct {
	page    prose.PageSize
	h       float64
	ground  color.Color
	content prose.Elem
}

func (b backdrop) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	foot := yTop - b.h
	dst[0].FillRect(0, foot, b.page.W, b.page.H-foot, b.ground)
	_, _, err := b.content.Draw(dst[:1], f, yTop)
	return 0, foot, err
}
