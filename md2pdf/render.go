package md2pdf

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	piudoc "github.com/soypat/piudf/piupage/piudoc"
	"github.com/soypat/prose"
)

const (
	fontRegular = prose.FamilyText
	fontBold    = prose.FamilyText + "-Bold"
)

// Handler sets blocks the renderer would otherwise set by their kind alone. It
// is handed the run of sibling blocks and the index of the next to set, so it
// can read what surrounds that block, and returns how many blocks from bs[i] on
// it has set. Zero declines them and the next handler is asked, so a handler
// may also add an element and decline, to precede what is set after it.
type Handler func(r *Renderer, bs []Block, i int) int

// Renderer lays blocks out as story elements. A [Converter] makes one for the
// document, and [Renderer.Sub] one more for each passage nested in another: a
// list item, a quote, a term's definition.
type Renderer struct {
	// Doc is the document being set. A handler may change its Meta and Head,
	// which are read only when it is written.
	Doc *prose.Doc
	// Kit makes elements in the document's theme.
	Kit *prose.Kit
	// Ragged sets passages flush left, for a column too narrow to justify
	// without opening holes between its words.
	Ragged bool

	c      *Converter
	sh     *shared
	nested bool
	pieces []piece
	// begun is set once anything has been added.
	begun bool
}

// shared is what a renderer and every renderer nested in it hold in common.
type shared struct {
	err   error
	later []*later
	toc   contents
	// titled is set when the document has a single "#", its title.
	titled bool
}

// piece is one element of the story and whether it must share a page with the
// element after it.
type piece struct {
	e        prose.Elem
	keepNext bool
}

// Theme is the vocabulary the document is set in.
func (r *Renderer) Theme() prose.Theme { return r.Kit.Theme }

// Nested reports whether r sets a passage inside another block rather than the
// document itself.
func (r *Renderer) Nested() bool { return r.nested }

// Add appends an element to the story.
func (r *Renderer) Add(e prose.Elem) {
	r.pieces = append(r.pieces, piece{e: e})
	r.begun = true
}

// Hold appends an element that must share a page with the one after it: a
// heading, or a line that introduces what follows. A table takes what is held
// to it as its lead, keeping it with the table's first rows rather than with
// the whole of it.
func (r *Renderer) Hold(e prose.Elem) {
	r.pieces = append(r.pieces, piece{e: e, keepNext: true})
	r.begun = true
}

// Last is the element added last, or nil. A handler reaches back through it to
// what it belongs to, as a total does to the table above it.
func (r *Renderer) Last() prose.Elem {
	if len(r.pieces) == 0 {
		return nil
	}
	return r.pieces[len(r.pieces)-1].e
}

// Fail records err as the document's fault. The first recorded is the one the
// converter reports, and no document is written.
func (r *Renderer) Fail(err error) {
	if err != nil && r.sh.err == nil {
		r.sh.err = err
	}
}

// Blocks sets bs, asking each of the converter's handlers in turn before
// setting a block by its kind.
func (r *Renderer) Blocks(bs []Block) {
	for i := 0; i < len(bs); {
		n := 0
		for _, h := range r.c.Handlers {
			if n = h(r, bs, i); n > 0 {
				break
			}
		}
		if n <= 0 {
			n = r.Default(bs, i)
		}
		i += n
	}
}

// Sub sets bs as one element, for blocks nested inside another. ragged sets its
// passages flush left.
func (r *Renderer) Sub(bs []Block, ragged bool) prose.Elem {
	s := &Renderer{Doc: r.Doc, Kit: r.Kit, Ragged: ragged, c: r.c, sh: r.sh, nested: true}
	s.Blocks(bs)
	return r.Kit.Stack(s.story()...)
}

// Default sets bs[i] by its kind alone, as though no handler had been given,
// and returns 1.
func (r *Renderer) Default(bs []Block, i int) int {
	b := &bs[i]
	switch b.Kind {
	case KindHeading:
		r.heading(b)
	case KindParagraph:
		r.paragraph(b)
	case KindList:
		r.list(b)
	case KindTable:
		r.table(b)
	case KindCode:
		r.Add(r.Kit.Code(b.Src))
	case KindQuote:
		r.quote(b)
	case KindImage:
		r.image(b)
	case KindRule:
		// A section draws its own rule, so a break before a heading is drawn
		// already, and one before anything has been set separates nothing.
		if r.begun && i+1 < len(bs) && bs[i+1].Kind != KindHeading {
			k := r.Kit
			r.Add(k.Stack(k.Space(4), k.Rule(0.5), k.Space(12)))
		}
	}
	return 1
}

// Plain is a heading or label as a reader should see it: its text with no
// markup and no trailing colon, recased if the converter asks for it.
func (r *Renderer) Plain(b *Block) string {
	s := strings.TrimSpace(strings.TrimSuffix(b.Text(), ":"))
	if r.c.SentenceCase {
		s = SentenceCase(s)
	}
	return s
}

// Body is the running text's style, flush left where the renderer is ragged.
func (r *Renderer) Body() prose.Style {
	st := r.Kit.Theme.Scale.Body
	if r.Ragged {
		st.Align = prose.Left
	}
	return st
}

// OpenImage reads a picture the markdown names out of the converter's FS.
func (r *Renderer) OpenImage(name string) (prose.Image, error) {
	switch {
	case strings.Contains(name, "://"):
		return nil, fmt.Errorf("md2pdf: %s: remote pictures are not fetched", name)
	case r.c.FS == nil:
		return nil, fmt.Errorf("md2pdf: %s: no FS to read pictures from", name)
	}
	return prose.OpenImage(r.c.FS, path.Clean(name))
}

// Later is an element made by f once the whole document has been laid out, for
// what depends on all of it: the contents on a cover, say. An f that returns
// nil draws nothing.
func (r *Renderer) Later(f func() prose.Elem) prose.Elem {
	l := &later{f: f}
	r.sh.later = append(r.sh.later, l)
	return l
}

// story folds the pieces into elements, binding each held piece to the one
// after it. A table takes what it is held to as its lead, so that a heading is
// kept with the table's first rows rather than with the whole of it.
func (r *Renderer) story() []prose.Elem {
	var out, held []prose.Elem
	for _, p := range r.pieces {
		e := p.e
		if len(held) > 0 {
			if t, ok := e.(*Table); ok && t.Lead == nil {
				t.Lead = r.Kit.Keep(held...)
			} else {
				e = r.Kit.Keep(append(held, e)...)
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

var sectionNum = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+(.+)$`)

// heading sets a "#" as a title under an accent rule, a "##" as a section, a
// numbered one with its number in the accent, a "###" as a subsection and
// anything deeper as a run-in heading.
func (r *Renderer) heading(b *Block) {
	k, th := r.Kit, r.Kit.Theme
	title := r.Plain(b)
	broke := !r.nested && b.Level <= r.c.BreakBefore
	gap := r.begun && !broke
	if broke {
		r.Add(PageBreak())
	}
	switch b.Level {
	case 1:
		var space, entry prose.Elem = k.Space(0), k.Space(0)
		if gap {
			space = k.Space(2 * th.Rhythm.SectionBefore)
		}
		// A document with one "#" is titled by it, and its contents is what
		// comes under the title; one with several is divided into parts.
		if !r.sh.titled {
			entry = r.Entry(1, "", title)
		}
		r.Hold(k.Keep(
			space,
			entry,
			k.Bookmark(title, 0),
			k.Title(Escape(title)),
			k.Space(10),
			k.ShortRule(58, 2.4, th.Palette.Accent),
			k.Space(8),
		))
	case 2:
		if m := sectionNum.FindStringSubmatch(title); m != nil {
			num := m[1]
			if len(num) == 1 {
				num = "0" + num
			}
			r.Hold(k.Section(num, Escape(m[2]), r.Entry(2, num, m[2])))
			return
		}
		rh := th.Rhythm
		r.Hold(k.Keep(
			r.Entry(2, "", title),
			k.Space(rh.SectionBefore),
			piudoc.HRule{Thickness: 0.9, Color: th.Palette.Hair, SpaceAfter: rh.SectionRule},
			k.Bookmark(title, 1),
			k.H2(Escape(title)),
			k.Space(rh.SectionAfter),
		))
	case 3:
		r.Hold(k.Keep(r.Entry(3, "", title), k.Bookmark(title, 2), k.Subsection(Escape(title))))
	default:
		r.Hold(k.Keep(r.Entry(b.Level, "", title), k.H3(Escape(title))))
	}
}

func (r *Renderer) paragraph(b *Block) {
	text := b.Joined()
	if strings.HasSuffix(StripTags(text), ":") {
		// What ends in a colon introduces what follows; it is never left at
		// the foot of a page without it.
		r.Hold(r.Kit.Styled(text, r.Body()))
		return
	}
	r.Add(r.Kit.Styled(text, r.Body()))
}

func (r *Renderer) list(b *Block) {
	k, th := r.Kit, r.Kit.Theme
	// A list's items are set ragged, which is how a list scans, and closer
	// than its paragraphs; only the last keeps the full gap to what follows.
	last := th.Scale.Body
	last.Align = prose.Left
	tight := last
	tight.SpaceAfter = 3.5
	for i, item := range b.Items {
		bs := th.Bullet
		if b.Ordered {
			bs = prose.BulletStyle{Marker: fmt.Sprintf("%d.", b.Start+i), Style: th.Scale.Marker, Indent: 18}
		}
		if len(item) == 1 && item[0].Kind == KindParagraph {
			st := tight
			if i == len(b.Items)-1 {
				st = last
			}
			// A marker is drawn where its item starts, so an item that broke
			// across a page would leave its marker alone at the foot.
			r.Add(k.Keep(k.Builder().Bullet(bs, k.Styled(item[0].Joined(), st))))
			continue
		}
		r.Add(k.Builder().Bullet(bs, r.Sub(item, true)))
	}
}

func (r *Renderer) quote(b *Block) {
	k := r.Kit
	for i := range b.Children {
		c := &b.Children[i]
		if c.Kind == KindParagraph {
			r.Add(k.Stack(k.Note(c.Joined()), k.Space(9)))
		} else {
			r.Blocks(b.Children[i : i+1])
		}
	}
}

func (r *Renderer) image(b *Block) {
	img, err := r.OpenImage(b.Src)
	if err != nil {
		r.Fail(err)
		return
	}
	k := r.Kit
	if b.Alt == "" {
		r.Add(k.Stack(k.Image(img, 0), k.Space(12)))
		return
	}
	r.Add(k.Stack(k.Figure(img, 0, Escape(b.Alt)), k.Space(12)))
}
