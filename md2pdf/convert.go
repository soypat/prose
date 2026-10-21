// Package md2pdf sets markdown as a PDF with [prose].
//
// The pipeline has three stages, each reachable on its own: [Parse] reads
// markdown into [Block] values; a [Converter] lays the blocks out as a
// [prose.Doc], asking its [Handler] chain before setting any block by its kind;
// and the document is written as prose writes any other. [Converter.Convert]
// runs the first two at once.
//
// A converter with no handlers sets plain markdown: headings, paragraphs,
// lists, tables, code, quotes and pictures. Conventions beyond markdown's own
// are handlers — [Totals], [Terms], [Tags] and [TOC] ship with the package —
// and a document with conventions of its own brings handlers of its own:
//
//	c := md2pdf.Converter{Handlers: []md2pdf.Handler{md2pdf.TOC, md2pdf.Totals}}
//	d, err := c.Convert("notes.md", src)
//	if err != nil {
//		return err
//	}
//	_, err = d.WriteFile("notes.pdf")
package md2pdf

import (
	"io"
	"io/fs"
	"strconv"

	"github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
)

// Converter sets markdown as a PDF. Its zero value sets plain markdown in
// prose's default theme and the standard-14 faces; every field is a choice
// laid over that.
type Converter struct {
	// Theme is the document's visual vocabulary. One with no page size is
	// taken as unset, and prose.DefaultTheme is used.
	Theme prose.Theme
	Fonts prose.Fonts
	// Meta and Head are the document's before any handler changes them. A Meta
	// with no title takes the first heading's.
	Meta prose.Meta
	Head prose.RunningHead
	// Budget bounds the document's memory. A zero one is [DefaultBudget].
	Budget prose.Budget
	// FS is what a picture's path is read out of. Without one, a document that
	// names a picture fails.
	FS fs.FS
	// Handlers are asked in order to set each block before it is set by its
	// kind. See [Handler].
	Handlers []Handler
	// BreakBefore starts a page before every heading of this level or a higher
	// one, so 1 sets each "#" on a page of its own. Zero breaks only where a
	// page fills.
	BreakBefore int
	// SentenceCase recases a heading or label the source set in capitals.
	SentenceCase bool
	// ContentsDepth is the deepest heading the contents lists. Zero is 2.
	ContentsDepth int
}

// DefaultBudget holds a hundred-odd pages, which a markdown source rarely
// outgrows; the default of prose itself holds a dozen.
func DefaultBudget() prose.Budget {
	return prose.Budget{MaxPages: 128, BytesPerPage: 96 << 10, EncScratch: 16 << 10}
}

func (c *Converter) theme() prose.Theme {
	if c.Theme.Page.W == 0 {
		return prose.DefaultTheme()
	}
	return c.Theme
}

func (c *Converter) depth() int {
	if c.ContentsDepth == 0 {
		return 2
	}
	return c.ContentsDepth
}

// Convert parses src and sets it. name is what errors call the source.
func (c *Converter) Convert(name string, src []byte) (*prose.Doc, error) {
	bs, err := c.Parse(name, src)
	if err != nil {
		return nil, err
	}
	return c.Render(bs)
}

// Parse reads markdown into blocks as the converter would set them, with code
// spans in the theme's accent. Parsing apart from rendering lets a caller read
// the blocks first: to title the document, or to take a cover off the front.
func (c *Converter) Parse(name string, src []byte) ([]Block, error) {
	return Parse(name, src, ParseOptions{CodeColor: hexColor(c.theme().Palette.Accent)})
}

// Render sets blocks as a document ready to be written. A document whose
// contents were asked for is set twice: the first pass learns which page each
// heading lands on, and the second sets those pages in the contents.
func (c *Converter) Render(bs []Block) (*prose.Doc, error) {
	d, toc, err := c.layout(bs, nil)
	if err != nil || !toc.asked {
		return d, err
	}
	if err = d.Write(io.Discard); err != nil {
		return nil, err
	}
	d, _, err = c.layout(bs, toc.pages())
	return d, err
}

// layout sets bs once, with the contents showing the pages in known.
func (c *Converter) layout(bs []Block, known []int) (*prose.Doc, *contents, error) {
	d, err := prose.New(c.theme(), c.Fonts)
	if err != nil {
		return nil, nil, err
	}
	d.Meta, d.Head, d.Budget = c.Meta, c.Head, c.Budget
	if d.Budget.MaxPages == 0 {
		d.Budget = DefaultBudget()
	}
	if d.Meta.Title == "" {
		d.Meta.Title = Title(bs)
	}
	parts := 0
	for i := range bs {
		if bs[i].Kind == KindHeading && bs[i].Level == 1 {
			parts++
		}
	}
	sh := &shared{toc: contents{finder: new(pageFinder), known: known}, titled: parts == 1}
	r := &Renderer{Doc: d, Kit: d.Kit(), c: c, sh: sh}
	r.pieces = append(r.pieces, piece{e: anchor{sh.toc.finder}})
	r.Blocks(bs)
	// What is made later may itself ask for more, so the list is walked by
	// index as it grows.
	for i := 0; i < len(sh.later); i++ {
		sh.later[i].e = sh.later[i].f()
	}
	if sh.err != nil {
		return nil, nil, sh.err
	}
	d.Add(r.story()...)
	return d, &sh.toc, d.Err()
}

// contents is what the table of contents lists and where.
type contents struct {
	entries []entry
	finder  *pageFinder
	// known is the page of each entry as the previous pass laid it out.
	known []int
	// asked is set once the contents is asked for, and with it a second pass.
	asked bool
}

type entry struct {
	level      int
	num, title string
	page       *int
}

func (c *contents) pages() []int {
	known := make([]int, len(c.entries))
	for i, e := range c.entries {
		known[i] = *e.page
	}
	return known
}

// Entry records a heading in the contents and returns the element that learns
// its page, to be held with the heading. A heading deeper than the converter's
// ContentsDepth, or one nested in another block, is not listed, and its entry
// draws nothing.
func (r *Renderer) Entry(level int, num, title string) prose.Elem {
	if r.nested || level > r.c.depth() {
		return r.Kit.Space(0)
	}
	e := entry{level: level, num: num, title: title, page: new(int)}
	r.sh.toc.entries = append(r.sh.toc.entries, e)
	return probe{r.sh.toc.finder, e.page}
}

// Contents is the table of contents: every heading [Renderer.Entry] recorded,
// and the page it landed on. It lists only the headings met so far, so it is
// asked for inside [Renderer.Later]; asking for it at all has the converter set
// the document twice, the first time to learn the pages. It is nil when no
// heading was recorded.
func (r *Renderer) Contents() prose.Elem {
	toc := &r.sh.toc
	toc.asked = true
	if len(toc.entries) == 0 {
		return nil
	}
	k, th := r.Kit, r.Kit.Theme
	pal := th.Palette
	numSt := prose.Style{Font: fontBold, Size: 9.2, Leading: 13, Color: pal.Accent}
	partSt := prose.Style{Font: fontBold, Size: 9.2, Leading: 13, Color: pal.Ink}
	pageSt := prose.Style{Font: fontRegular, Size: 9.2, Leading: 13, Color: pal.Muted}
	numbered := false
	for _, e := range toc.entries {
		numbered = numbered || e.num != ""
	}
	// The pass that knows no pages sets the same rows without numbers, so the
	// pass that has them lays out identically.
	rows := make([][]piudoc.Cell, len(toc.entries))
	for i, e := range toc.entries {
		page := ""
		if i < len(toc.known) && toc.known[i] > 0 {
			page = strconv.Itoa(toc.known[i])
		}
		st := th.Scale.Cell
		if e.level == 1 {
			st = partSt
		}
		rows[i] = []piudoc.Cell{{Drawer: k.Styled(Escape(e.title), st)}, {Drawer: k.Styled(page, pageSt)}}
		if numbered {
			rows[i] = append([]piudoc.Cell{{Drawer: k.Styled(Escape(e.num), numSt)}}, rows[i]...)
		}
	}
	widths, titleCol := []float64{0, 30}, 0
	if numbered {
		widths, titleCol = []float64{30, 0, 30}, 1
	}
	pad := prose.Padding{Top: 2.5, Bottom: 2.5}
	t := &piudoc.Table{ColWidths: widths, Rows: rows, Pad: pad}
	t.Style.All().Valign(prose.Top)
	t.Style.Col(-1).Align(prose.Right)
	for i, e := range toc.entries {
		switch {
		case e.level == 1 && i > 0:
			t.Style.Row(i).Pad(prose.Padding{Top: 9, Bottom: 2.5})
		case e.level > 2:
			indent := pad
			indent.Left = 12 * float64(e.level-2)
			t.Style.Range(titleCol, i, titleCol, i).Pad(indent)
		}
	}
	return t
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

// PageBreak starts a fresh page, unless the story is at the top of one.
func PageBreak() prose.Elem { return pageBreak{} }

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

// later is an element whose making waits until the story has been laid out.
type later struct {
	f func() prose.Elem
	e prose.Elem
}

func (l *later) Draw(dst []piupage.Canvas, f piudoc.Frame, yTop float64) (int, float64, error) {
	if l.e == nil {
		return 0, yTop, nil
	}
	return l.e.Draw(dst, f, yTop)
}
