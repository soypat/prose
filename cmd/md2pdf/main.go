// Command md2pdf sets a markdown file as a PDF.
//
//	go run ./cmd/md2pdf notes.md
//	go run ./cmd/md2pdf -o notes.pdf -paper letter -break 1 -ext toc,terms notes.md
//	cat notes.md | go run ./cmd/md2pdf -o notes.pdf -
//
// Text is set in the standard-14 faces unless -fontdir names a directory of
// .ttf files. A document whose text needs a glyph its faces lack is refused
// rather than written, so text beyond Latin-1 wants a font that carries it:
//
//	go run ./cmd/md2pdf -fontdir /usr/share/fonts/TTF -font DejaVuSans,DejaVuSans-Bold,DejaVuSans-Oblique notes.md
//
// Extensions are conventions beyond markdown's own, chosen with -ext:
//
//   - toc: a paragraph reading [TOC] alone is replaced by the contents.
//   - totals: two strong lines, a label over an amount, are set as a total bar.
//   - terms: a strong line and the blocks under it are set as a ruled term row.
//   - tags: "### Name [tag]" sets the bracket as a tag at the heading's right.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soypat/prose"
	"github.com/soypat/prose/md2pdf"
)

// extensions are the handlers -ext can name, in the order they are asked: a
// total is two strong lines and would read as a term if terms were asked first.
var extensions = []struct {
	name string
	h    md2pdf.Handler
}{
	{"toc", md2pdf.TOC},
	{"tags", md2pdf.Tags},
	{"totals", md2pdf.Totals},
	{"terms", md2pdf.Terms},
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("md2pdf: ")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: md2pdf [flags] file.md\n\nfile.md may be - to read standard input.\n\n")
		flag.PrintDefaults()
	}
	var (
		out      = flag.String("o", "", "PDF to write; defaults to the input's name with a .pdf extension")
		title    = flag.String("title", "", "document title; defaults to the first heading")
		author   = flag.String("author", "", "document author")
		lang     = flag.String("lang", "en", "document language, as a BCP 47 tag")
		paper    = flag.String("paper", "a4", "paper size: a4 or letter")
		brk      = flag.Int("break", 0, "start a page before every heading of this level or higher; 0 never")
		depth    = flag.Int("depth", 2, "deepest heading level the contents lists")
		recase   = flag.Bool("recase", false, "recase headings the source set in capitals")
		head     = flag.Bool("head", true, "set the title as a running head on every page but the first")
		exts     = flag.String("ext", "toc", "comma-separated extensions: "+extNames())
		fontDir  = flag.String("fontdir", "", "directory of the .ttf files -font and -mono name")
		font     = flag.String("font", "", "text faces in -fontdir, without .ttf, as Regular[,Bold[,Italic]]")
		mono     = flag.String("mono", "", "code faces in -fontdir, without .ttf, as Regular[,Bold[,Italic]]")
		maxPages = flag.Int("maxpages", md2pdf.DefaultBudget().MaxPages, "most pages the document may run to")
	)
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	in := flag.Arg(0)
	if *out == "" {
		if in == "-" {
			log.Fatal("reading standard input needs -o")
		}
		*out = strings.TrimSuffix(in, filepath.Ext(in)) + ".pdf"
	}

	th := prose.DefaultTheme()
	switch strings.ToLower(*paper) {
	case "a4":
	case "letter":
		th.Page = prose.SizeLetter()
	default:
		log.Fatalf("unknown paper %q", *paper)
	}
	handlers, err := parseExt(*exts)
	if err != nil {
		log.Fatal(err)
	}
	budget := md2pdf.DefaultBudget()
	budget.MaxPages = *maxPages

	src, dir, err := readInput(in)
	if err != nil {
		log.Fatal(err)
	}
	c := md2pdf.Converter{
		Theme:         th,
		Fonts:         fonts(*fontDir, *font, *mono),
		Budget:        budget,
		FS:            os.DirFS(dir),
		Handlers:      handlers,
		BreakBefore:   *brk,
		SentenceCase:  *recase,
		ContentsDepth: *depth,
	}
	start := time.Now()
	bs, err := c.Parse(in, src)
	if err != nil {
		log.Fatal(err)
	}
	// The blocks are read before they are set, so the running head can carry
	// the title the first heading gives.
	if *title == "" {
		*title = md2pdf.Title(bs)
	}
	c.Meta = prose.Meta{Title: *title, Author: *author, Lang: *lang,
		Creator: "github.com/soypat/prose/cmd/md2pdf", Date: time.Now()}
	if *head {
		c.Head = prose.RunningHead{Left: *title, SkipFirst: true, Rules: true}
	}
	d, err := c.Render(bs)
	var n int64
	if err == nil {
		n, err = d.WriteFile(*out)
	}
	if errors.Is(err, io.ErrShortBuffer) {
		log.Fatalf("%s runs past %d pages; raise -maxpages", in, *maxPages)
	} else if err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%.0f kB) in %s", *out, float64(n)/1000, time.Since(start).Round(time.Millisecond))
}

// readInput reads the markdown and says what directory its pictures' paths
// are relative to.
func readInput(name string) (src []byte, dir string, err error) {
	if name == "-" {
		src, err = io.ReadAll(os.Stdin)
		return src, ".", err
	}
	src, err = os.ReadFile(name)
	return src, filepath.Dir(name), err
}

func parseExt(list string) ([]md2pdf.Handler, error) {
	want := make(map[string]bool)
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(strings.ToLower(name)); name != "" {
			want[name] = true
		}
	}
	var hs []md2pdf.Handler
	for _, e := range extensions {
		if want[e.name] {
			hs = append(hs, e.h)
			delete(want, e.name)
		}
	}
	for name := range want {
		return nil, fmt.Errorf("unknown extension %q; have %s", name, extNames())
	}
	return hs, nil
}

func extNames() string {
	names := make([]string, len(extensions))
	for i, e := range extensions {
		names[i] = e.name
	}
	return strings.Join(names, ", ")
}

func fonts(dir, text, mono string) prose.Fonts {
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	f := prose.Fonts{Text: split(text), Mono: split(mono)}
	if f.Text != nil || f.Mono != nil {
		if dir == "" {
			dir = "."
		}
		f.FS = os.DirFS(dir)
	}
	return f
}
