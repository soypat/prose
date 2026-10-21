// Command proposal sets a markdown proposal as a PDF: a cover carrying the
// title and the offer's particulars, then each top-level division on pages of
// its own. It is github.com/soypat/prose/md2pdf with a cover of its own and the
// handlers a proposal's conventions call for.
//
//	go run ./examples/proposal -i examples/proposal/example-proposal.md
//
// The document's shape is read from the markdown's own conventions:
//
//   - Paragraphs before the first "#" heading are the covering letter.
//   - The first "#" heading is the title, a "##" right under it the subtitle,
//     and a headerless two-column table after those the offer's particulars.
//   - Every later "#" heading opens a division on a new page; "## 1. Name" is a
//     numbered section and "### Name [tag]" a heading with the tag set right.
//   - Two strong lines on their own, a label over an amount, are a total; under
//     a table they become its foot.
//   - A strong first line followed by text or a list is a term and what it
//     owns, set as one row of a ruled two-column grid.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soypat/piudf/piupage"
	"github.com/soypat/prose"
	"github.com/soypat/prose/md2pdf"
)

func main() {
	in := flag.String("i", "local/proposal/proposal.md", "markdown proposal to set")
	out := flag.String("o", "", "PDF to write; defaults to the input's name with a .pdf extension")
	org := flag.String("org", "Indgrade", "organisation the proposal is from")
	logoPath := flag.String("logo", "", "logo for the cover, PNG or JPEG; defaults to logo.* or <org>.* beside the markdown")
	flag.Parse()
	if *out == "" {
		*out = strings.TrimSuffix(*in, filepath.Ext(*in)) + ".pdf"
	}
	start := time.Now()
	n, err := run(*in, *out, *org, *logoPath)
	if err != nil {
		log.Fatalln("proposal:", err)
	}
	log.Printf("wrote %s (%.0f kB) in %s", *out, float64(n)/1000, time.Since(start).Round(time.Millisecond))
}

// Palette is a navy that reads as engineering rather than marketing, with a
// steel blue to point with and an amber kept for the cover alone.
func palette() prose.Palette {
	return prose.Palette{
		Ink:    piupage.HexColor("#0f2133"),
		Body:   piupage.HexColor("#2a3441"),
		Muted:  piupage.HexColor("#67727e"),
		Hair:   piupage.HexColor("#d3dae1"),
		Accent: piupage.HexColor("#1c5d91"),
		Band:   piupage.HexColor("#eef2f6"),
		CodeBg: piupage.HexColor("#f3f5f8"),
		Paper:  piupage.HexColor("#ffffff"),
		Link:   piupage.HexColor("#9fc6e6"),
	}
}

// defaultTitle is what a proposal with no title of its own is called.
const defaultTitle = "Propuesta"

func run(in, out, org, logoPath string) (int64, error) {
	src, err := os.ReadFile(in)
	if err != nil {
		return 0, err
	}
	if logoPath == "" {
		logoPath = findLogo(filepath.Dir(in), org)
	}
	var lg *logo
	if logoPath != "" {
		if lg, err = loadLogo(logoPath, palette().Ink); err != nil {
			return 0, err
		}
	}
	th := prose.DefaultTheme()
	// The scale was resolved against the default palette; zeroing it has the
	// kit resolve it again against this one.
	th.Palette, th.Scale, th.Bullet = palette(), prose.Scale{}, prose.BulletStyle{}
	th.Margins = prose.Margins{Left: 64, Right: 64, Top: 64, Bottom: 68}

	c := md2pdf.Converter{
		Theme: th,
		// The cover replaces the title and the running head with its own.
		Meta: prose.Meta{
			Title:   defaultTitle,
			Author:  org,
			Creator: "github.com/soypat/prose",
			Lang:    "es",
			Date:    time.Now(),
		},
		Head:         runningHead(org, defaultTitle, ""),
		Budget:       prose.Budget{MaxPages: 64, BytesPerPage: 96 << 10, EncScratch: 16 << 10},
		FS:           os.DirFS(filepath.Dir(in)),
		BreakBefore:  1,
		SentenceCase: true,
		Handlers: []md2pdf.Handler{
			(&coverPage{org: org, logo: lg}).handle,
			md2pdf.Tags,
			md2pdf.Totals,
			md2pdf.Terms,
		},
	}
	d, err := c.Convert(in, src)
	if err != nil {
		return 0, err
	}
	return d.WriteFile(out)
}

// runningHead is the organisation and the title at the head of every page but
// the cover, the offer's reference opposite, and the folio at the foot.
func runningHead(org, title, reference string) prose.RunningHead {
	return prose.RunningHead{
		Left:      strings.ToUpper(org) + "  ·  " + title,
		Right:     reference,
		SkipFirst: true,
		Rules:     true,
		Folio:     func(num, total int) string { return fmt.Sprintf("Página %d de %d", num, total) },
	}
}
