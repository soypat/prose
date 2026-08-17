package prose

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	canvas "github.com/soypat/piudf/piupage"
	piudoc "github.com/soypat/piudf/piupage/piudoc"
)

// Fonts says what backs a theme's families. A family is bound by the first of
// these that is given: a family the caller already holds, the files named here,
// or its standard-14 counterpart — so a document needs no font files at all.
//
// Text and Mono name .ttf files without their extension, in Regular, Bold,
// Italic order; only Regular is required, and a missing weight falls back to
// the one before it.
type Fonts struct {
	FS         fs.FS
	Dir        string
	Text, Mono []string
	// TextFamily and MonoFamily bind faces the caller already has: a standard-14
	// face other than the default, or a font loaded elsewhere. See [Builtin]. A
	// family whose Regular is nil is unset.
	TextFamily, MonoFamily piudoc.Family
}

// Meta is what a reader's viewer shows about the document.
type Meta struct {
	Title, Author, Subject, Creator, Lang string
	Date                                  time.Time
}

// RunningHead is the furniture on every page but, optionally, the first: a
// document announces itself with its masthead, and a page number on the sheet
// that carries the title tells the reader nothing.
type RunningHead struct {
	// Left and Right are the head's two texts, set at the margins; either may be empty.
	Left, Right string
	// SkipFirst leaves page one bare, for the sheet that carries the masthead.
	SkipFirst bool
	// Rules draws a hairline under the head and above the folio.
	Rules bool
	// Folio renders the page number; nil means "N of M", and "" draws none.
	Folio func(num, total int) string
	// Style is the furniture's own; a zero style means Scale.Caption in Muted.
	Style Style
}

// Budget bounds a document's memory. piudoc allocates nothing of its own, so
// the whole cost is decided here and is knowable before anything is written.
// Embedded faces spend two bytes a glyph, which is what makes a page of an
// embedded document dearer than one of plain Helvetica.
type Budget struct{ MaxPages, BytesPerPage, EncScratch int }

// DefaultBudget holds a dozen pages of embedded text.
func DefaultBudget() Budget {
	return Budget{MaxPages: 12, BytesPerPage: 64 << 10, EncScratch: 8 << 10}
}

// Doc accumulates a story in a theme and writes it as a PDF. Its append methods
// chain; the elements they append are made by the [Kit] behind [Doc.Kit].
type Doc struct {
	Meta   Meta
	Head   RunningHead
	Budget Budget

	kit   *Kit
	story []Elem
}

// New loads f and returns a document set in th.
func New(th Theme, f Fonts) (*Doc, error) {
	k, err := NewKit(th, f)
	if err != nil {
		return nil, err
	}
	return &Doc{Budget: DefaultBudget(), kit: k}, nil
}

// Kit is the element maker, for the pieces that nest inside others.
func (d *Doc) Kit() *Kit { return d.kit }

// Theme is the vocabulary the document is set in.
func (d *Doc) Theme() Theme { return d.kit.Theme }

// Err reports the first fault met while the story was built.
func (d *Doc) Err() error { return d.kit.Err() }

// Add appends what the named methods do not cover.
func (d *Doc) Add(elems ...Elem) *Doc {
	d.story = append(d.story, elems...)
	return d
}

// Write renders the story. It fails rather than write a document whose text the
// embedded faces cannot draw, so a fault reaches the author and never a reader.
func (d *Doc) Write(w io.Writer) error {
	if err := d.Err(); err != nil {
		return err
	}
	th := d.kit.Theme
	b := d.Budget
	if b.MaxPages == 0 {
		b = DefaultBudget()
	}
	pd := &piudoc.Doc{
		Size:    th.Page,
		Margins: th.Margins,
		Title:   d.Meta.Title,
		Author:  d.Meta.Author,
		Subject: d.Meta.Subject,
		Creator: d.Meta.Creator,
		Lang:    d.Meta.Lang,
		Date:    d.Meta.Date,
		OnPage:  d.decorate,
	}
	pages := make([]canvas.Canvas, b.MaxPages)
	return pd.Build(w, pages, d.story,
		make([]byte, b.EncScratch), make([]byte, b.MaxPages*b.BytesPerPage))
}

// WriteFile renders the story to name and reports the bytes written. A document
// that fails to render leaves no file behind.
func (d *Doc) WriteFile(name string) (size int64, err error) {
	if err = d.Err(); err != nil {
		return 0, err
	}
	f, err := os.Create(name)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(name)
			size = 0
		}
	}()
	if err = d.Write(f); err != nil {
		return 0, err
	}
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// decorate draws the running head and folio described by d.Head.
func (d *Doc) decorate(c *canvas.Canvas, p piudoc.PageInfo) {
	h := d.Head
	if h.SkipFirst && p.Num == 1 {
		return
	}
	th := d.kit.Theme
	st := h.Style
	if st.Size == 0 {
		st = th.Scale.Caption
	}
	face := d.kit.faces[baseFamily(st.Font)]
	if face == nil {
		return
	}
	top := p.Size.H - p.Margins.Top
	right := p.Size.W - p.Margins.Right
	c.SetFont(face, st.Size)

	if h.Left != "" {
		c.Text(p.Margins.Left, top+26, h.Left, st.Color)
	}
	if h.Right != "" {
		c.TextRight(right, top+26, h.Right, st.Color)
	}
	if h.Rules && (h.Left != "" || h.Right != "") {
		c.Line(p.Margins.Left, top+20, right, top+20, 0.5, th.Palette.Hair)
	}

	folio := h.Folio
	if folio == nil {
		folio = func(num, total int) string { return fmt.Sprintf("%d of %d", num, total) }
	}
	if s := folio(p.Num, p.Total); s != "" {
		if h.Rules {
			c.Line(p.Margins.Left, p.Margins.Bottom-26, right, p.Margins.Bottom-26, 0.5, th.Palette.Hair)
		}
		c.TextRight(right, p.Margins.Bottom-38, s, st.Color)
	}
}

// Appending elements. Each is the [Kit] constructor of the same name, appended
// to the story rather than returned.

func (d *Doc) Eyebrow(markup string) *Doc   { return d.Add(d.kit.Eyebrow(markup)) }
func (d *Doc) Title(markup string) *Doc     { return d.Add(d.kit.Title(markup)) }
func (d *Doc) Lead(markup string) *Doc      { return d.Add(d.kit.Lead(markup)) }
func (d *Doc) Body(markup string) *Doc      { return d.Add(d.kit.Body(markup)) }
func (d *Doc) Small(markup string) *Doc     { return d.Add(d.kit.Small(markup)) }
func (d *Doc) H2(markup string) *Doc        { return d.Add(d.kit.H2(markup)) }
func (d *Doc) H3(markup string) *Doc        { return d.Add(d.kit.H3(markup)) }
func (d *Doc) Sub(markup string) *Doc       { return d.Add(d.kit.Sub(markup)) }
func (d *Doc) Note(markup string) *Doc      { return d.Add(d.kit.Note(markup)) }
func (d *Doc) Panel(markup string) *Doc     { return d.Add(d.kit.Panel(markup)) }
func (d *Doc) Code(src string) *Doc         { return d.Add(d.kit.Code(src)) }
func (d *Doc) Bullet(markup string) *Doc    { return d.Add(d.kit.Bullet(markup)) }
func (d *Doc) Rule(thickness float64) *Doc  { return d.Add(d.kit.Rule(thickness)) }
func (d *Doc) Space(h float64) *Doc         { return d.Add(d.kit.Space(h)) }
func (d *Doc) Subsection(title string) *Doc { return d.Add(d.kit.Subsection(title)) }

// Bullets appends a whole list, which is what most lists are.
func (d *Doc) Bullets(items ...string) *Doc {
	for _, it := range items {
		d.Bullet(it)
	}
	return d
}

// Section appends a numbered division and the element held with its heading.
func (d *Doc) Section(num, title string, first Elem) *Doc {
	return d.Add(d.kit.Section(num, title, first))
}

// Bookmark names a place in the reader's navigation pane.
func (d *Doc) Bookmark(title string, level int) *Doc {
	return d.Add(d.kit.Bookmark(title, level))
}
