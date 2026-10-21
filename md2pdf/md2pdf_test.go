package md2pdf

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"
	"testing/fstest"
)

const sample = `Before the title.

# Title

## 1. First

Text with **bold**, *emphasis*, ` + "`code`" + ` and https://example.com.

| Item | Qty | Note |
|:-----|----:|:----:|
| One  | 1   | a    |
| Two  | 2   |

**TOTAL**
**USD 3.00**

**Payment:**

- Half now.
- Half later.

### Stage [2 weeks]

` + "```go\nx := 1\n```" + `

> Quoted.

---

1. First
   - nested
2. Second

![Alt text](pic.png)
`

func TestParse(t *testing.T) {
	bs, err := Parse("sample.md", []byte(sample), ParseOptions{CodeColor: "#123456"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Kind{
		KindParagraph, KindHeading, KindHeading, KindParagraph, KindTable,
		KindParagraph, KindParagraph, KindList, KindHeading, KindCode,
		KindQuote, KindRule, KindList, KindImage,
	}
	if len(bs) != len(want) {
		t.Fatalf("got %d blocks, want %d: %+v", len(bs), len(want), bs)
	}
	for i, k := range want {
		if bs[i].Kind != k {
			t.Errorf("block %d: kind %d, want %d", i, bs[i].Kind, k)
		}
	}
	if got := Title(bs); got != "Title" {
		t.Errorf("Title = %q", got)
	}
	if bs[2].Level != 2 || bs[2].Text() != "1. First" {
		t.Errorf("heading: level %d, text %q", bs[2].Level, bs[2].Text())
	}

	p := bs[3].Joined()
	for _, s := range []string{"<b>bold</b>", "<i>emphasis</i>", `<font color="#123456">code</font>`, `<a href="https://example.com">`} {
		if !strings.Contains(p, s) {
			t.Errorf("paragraph %q lacks %q", p, s)
		}
	}

	tb := bs[4]
	if got := tb.Align; len(got) != 3 || got[0] != AlignLeft || got[1] != AlignRight || got[2] != AlignCenter {
		t.Errorf("table align = %v", got)
	}
	if len(tb.Rows) != 3 || tb.Rows[1][0] != "One" {
		t.Errorf("table rows = %q", tb.Rows)
	}
	if last := tb.Rows[2]; len(last) != 3 || last[2] != "" {
		t.Errorf("short row not padded: %q", last)
	}

	if !bs[5].AllBold() || len(bs[5].Lines) != 2 {
		t.Errorf("total lines = %+v", bs[5].Lines)
	}
	if !bs[6].Lines[0].Bold {
		t.Error("label line not bold")
	}
	if code := bs[9]; code.Src != "x := 1" || code.Info != "go" {
		t.Errorf("code = %q, info %q", code.Src, code.Info)
	}
	if q := bs[10]; len(q.Children) != 1 || q.Children[0].Kind != KindParagraph {
		t.Errorf("quote children = %+v", q.Children)
	}
	ol := bs[12]
	if !ol.Ordered || ol.Start != 1 || len(ol.Items) != 2 || len(ol.Items[0]) != 2 || ol.Items[0][1].Kind != KindList {
		t.Errorf("ordered list = %+v", ol)
	}
	if img := bs[13]; img.Src != "pic.png" || img.Alt != "Alt text" {
		t.Errorf("image = %q, alt %q", img.Src, img.Alt)
	}
}

func TestConvert(t *testing.T) {
	c := Converter{
		FS:       fstest.MapFS{"pic.png": {Data: onePixelPNG(t)}},
		Handlers: []Handler{TOC, Tags, Totals, Terms},
	}
	d, err := c.Convert("sample.md", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", buf.Bytes()[:min(16, buf.Len())])
	}
	if d.Meta.Title != "Title" {
		t.Errorf("Meta.Title = %q", d.Meta.Title)
	}
}

func TestPicturesNeedFS(t *testing.T) {
	var c Converter
	if _, err := c.Convert("pic.md", []byte("![x](pic.png)\n")); err == nil {
		t.Fatal("a picture with no FS to read it from was set")
	}
}

func TestMissingGlyphRefused(t *testing.T) {
	var c Converter
	if _, err := c.Convert("arrow.md", []byte("A → B\n")); err == nil {
		t.Fatal("text the standard-14 faces cannot draw was set")
	}
}

// TestContentsPages sets parts one to a page and checks the contents learns
// the page each landed on.
func TestContentsPages(t *testing.T) {
	src := "[TOC]\n\n# One\n\n## Inside one\n\n# Two\n\n# Three\n"
	c := Converter{BreakBefore: 1, Handlers: []Handler{TOC}}
	bs, err := c.Parse("parts.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	d, toc, err := c.layout(bs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !toc.asked {
		t.Fatal("[TOC] did not ask for the contents")
	}
	if err := d.Write(io.Discard); err != nil {
		t.Fatal(err)
	}
	// The contents fills the first page, so each part opens the page after.
	want := []int{2, 2, 3, 4}
	got := toc.pages()
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pages = %v, want %v", got, want)
		}
	}
}

// TestHandlerChain checks handlers are asked in order, that a declined block
// reaches the next handler and then the default, and that a handler setting
// several blocks has them skipped.
func TestHandlerChain(t *testing.T) {
	src := "first\n\nsecond\n\nthird\n\n```\ncode\n```\n"
	var asked []string
	var r0 *Renderer
	both := func(r *Renderer, bs []Block, i int) int {
		r0 = r
		asked = append(asked, "both:"+bs[i].Text())
		if bs[i].Kind == KindParagraph && bs[i].Text() == "first" {
			r.Add(r.Kit.Body("first and second"))
			return 2
		}
		return 0
	}
	code := func(r *Renderer, bs []Block, i int) int {
		asked = append(asked, "code")
		if bs[i].Kind != KindCode {
			return 0
		}
		r.Add(r.Kit.Body("no code here"))
		return 1
	}
	c := Converter{Handlers: []Handler{both, code}}
	if _, err := c.Convert("chain.md", []byte(src)); err != nil {
		t.Fatal(err)
	}
	want := []string{"both:first", "both:third", "code", "both:", "code"}
	if strings.Join(asked, "|") != strings.Join(want, "|") {
		t.Errorf("asked %q, want %q", asked, want)
	}
	// The anchor, the two handled elements, and "third" set by default.
	if n := len(r0.pieces); n != 4 {
		t.Errorf("story has %d pieces, want 4", n)
	}
}

func TestTotalUnderTable(t *testing.T) {
	src := "| A | B |\n|---|---|\n| 1 | 2 |\n\n**TOTAL**\n**3**\n\n**TOTAL**\n**3**\n"
	var r0 *Renderer
	spy := func(r *Renderer, bs []Block, i int) int { r0 = r; return 0 }
	c := Converter{Handlers: []Handler{spy, Totals}}
	if _, err := c.Convert("total.md", []byte(src)); err != nil {
		t.Fatal(err)
	}
	// The first total is the table's foot; the second, with no table above it
	// to sit under, stands on its own.
	if n := len(r0.pieces); n != 3 {
		t.Fatalf("story has %d pieces, want 3", n)
	}
	tb, ok := r0.pieces[1].e.(*Table)
	if !ok || tb.Foot == nil {
		t.Fatalf("total did not become the table's foot: %T", r0.pieces[1].e)
	}
}

func onePixelPNG(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
