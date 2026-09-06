package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/soypat/piudf/piupage"
	"github.com/soypat/prose"
)

const name = "cv"

func TestGenerateProspectus(t *testing.T) {
	var buf bytes.Buffer
	// One run before the benchmark. A document prose refuses — a glyph the faces
	// cannot draw, a tag the dialect does not carry — fails here saying so, where
	// the same fault inside b.Loop is a b.Fatal that leaves a zero
	// BenchmarkResult, whose nil Extra then panics over the top of the message.
	if err := run(&buf, testing.BenchmarkResult{}); err != nil {
		t.Fatal(err)
	}
	results := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			buf.Reset()
			err := run(&buf, testing.BenchmarkResult{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	results.Extra["size"] = float64(buf.Len())
	buf.Reset()
	run(&buf, results)
	os.WriteFile(name+".pdf", buf.Bytes(), 0666)
}

func run(w io.Writer, results testing.BenchmarkResult) error {
	th := cvTheme()

	// Both families are bound to standard-14 faces, which every reader carries.
	// Passing a zero Fonts binds the same two; naming them says so out loud.
	d, err := prose.New(th, prose.Fonts{
		TextFamily: prose.Builtin(piupage.FontHelvetica),
		MonoFamily: prose.Builtin(piupage.FontCourier),
	})
	if err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	d.Meta = prose.Meta{
		Title:   "Patricio Whittingslow — Curriculum Vitae",
		Author:  "Patricio Whittingslow",
		Subject: "Mechanical engineer. Aerospace, embedded systems and software.",
		Creator: "soypat/piudf",
		Lang:    "en",
		Date:    time.Now(),
	}
	d.Budget = budget
	d.Head = prose.RunningHead{
		Left:      "Patricio Whittingslow — Curriculum Vitae",
		Right:     "graded.sp@gmail.com",
		SkipFirst: true,
		Rules:     true,
	}

	story(d, results)
	err = d.Write(w)
	if err != nil {
		return err
	}
	return nil
}

const locBA = "Buenos Aires, Argentina"

// budget is the whole of the document's memory: piudoc allocates nothing of its
// own, so this is knowable before anything is written, and the colophon prints
// it beside what the run actually spent.
var budget = prose.Budget{MaxPages: 8, BytesPerPage: 512 << 10, EncScratch: 16 << 10}

// cvTheme tightens the default theme for a CV, which is read in glances rather
// than in paragraphs: a smaller scale, narrower margins, and about a third of
// the air between blocks. The type is set small on purpose — the page is a
// reference, not a text.
func cvTheme() prose.Theme {
	th := prose.DefaultTheme()
	th.Margins = prose.Margins{Left: 44, Right: 44, Top: 44, Bottom: 46}
	th.Pad = prose.Padding{Right: 10, Top: 2.5, Bottom: 2.5}

	s := &th.Scale
	size := func(st *prose.Style, size, leading float64) {
		st.Size, st.Leading = size, leading
	}
	size(&s.Title, 23, 26)
	size(&s.Eyebrow, 7.2, 10)
	size(&s.H2, 11.5, 14)
	size(&s.H3, 8.6, 11.4)
	size(&s.Sub, 9, 12)
	size(&s.Body, 8, 11)
	size(&s.Small, 7.2, 9.8)
	size(&s.Cell, 7.8, 10.6)
	size(&s.CellHead, 6.8, 9.4)
	size(&s.Marker, 8, 11)
	size(&s.Caption, 6.8, 9.2)
	s.H3.SpaceAfter, s.Sub.SpaceAfter = 1, 2
	s.Body.Align, s.Body.SpaceAfter = prose.Left, 1.5

	// Bullet copied the marker style when DefaultTheme filled the scale, so it
	// keeps the old size unless it is taken again here.
	th.Bullet.Style, th.Bullet.Indent = s.Marker, 9
	th.Rhythm.SectionBefore, th.Rhythm.SectionRule, th.Rhythm.SectionAfter = 11, 6, 5
	return th
}

// Column geometry. The two-column band is one table row, and piudoc never
// splits a row across a page, so everything inside it has to fit on the page
// it starts on. Conference talks are therefore laid full width underneath,
// where nine of them cost a line each instead of the two a 175pt column forces.
const (
	sideCol = 175
	gutter  = 16
)

// citedMark flags a publication somebody else wrote, so that a reader skimming
// the card does not read the whole list as authorship.
const citedMark = "†"

// if filter is non-nil then only items with filter tags will be included.
func story(d *prose.Doc, br testing.BenchmarkResult, filter ...Tag) {
	const conferenceLink = "https://www.youtube.com/playlist?list=PLx6oFeWGZv8mW4p6dhHFlAWRGH1IGXWDk"
	const kB = 1 << 10

	k, th := d.Kit(), d.Theme()

	// Masthead: name, trade, and every way to reach the author on one line.
	d.Eyebrow("MECHANICAL ENGINEER — AEROSPACE, EMBEDDED SYSTEMS").
		Title("Patricio Whittingslow").
		Space(3).
		Small(strings.Join([]string{
			locBA,
			`<a href="mailto:graded.sp@gmail.com">graded.sp@gmail.com</a>`,
			`<a href="https://github.com/soypat">github.com/soypat</a>`,
			`<a href="https://www.linkedin.com/in/patriciowhittingslow">LinkedIn</a>`,
			`<a href="` + conferenceLink + `">Talks</a>`,
			"English, Español, Portugués",
		}, " — ")).
		Space(7).
		Rule(1.2).
		Space(9)

	// The main column carries the work; the cards beside it carry the record.
	// Talks go under the experience rather than beside it: the jobs run shorter
	// than the three cards, and a talk costs one line at this measure against
	// the two a 175pt column would force.
	col := []prose.Elem{sectionHead(k, th, "Experience")}
	col = append(col, events(k, th, jobs, filter)...)
	if t := talkTable(k, filter); t != nil {
		col = append(col, k.Space(6), sectionHead(k, th, `<a href="https://www.youtube.com/playlist?list=PLx6oFeWGZv8mW4p6dhHFlAWRGH1IGXWDk">Conference talks</a>`), t)
	}
	main := k.Stack(col...)

	var pubs []prose.Elem
	var cited bool
	for _, p := range publications {
		if wants(p.Tags, filter) {
			pubs = append(pubs, publicationEntry(k, p))
			cited = cited || p.Cited
		}
	}
	if cited {
		pubs = append(pubs, k.Styled(citedMark+" Written by others, building on my open source work.",
			th.Scale.Caption))
	}
	var classes []prose.Elem
	for _, t := range teaching {
		if wants(t.Tags, filter) {
			classes = append(classes, teachingEntry(k, t))
		}
	}
	side := k.Stack(
		card(k, th, "Education", events(k, th, education, filter)...),
		card(k, th, "Publications", pubs...),
		card(k, th, "Teaching", classes...),
	)

	d.Add(k.Columns([]float64{th.Measure() - sideCol - gutter, gutter, sideCol},
		main, k.Space(0), side))

	// The colophon is only meaningful once the benchmark has run; the first
	// pass through run carries a zero result.
	if br.N > 0 {
		d.Space(9).Add(k.Styled(fmt.Sprintf(
			`Typeset by <a href="https://github.com/soypat/prose">prose</a>`+
				" in %s, using %.1f MB of memory for this %.0f kB PDF file.",
			time.Duration(br.NsPerOp()).Round(100*time.Microsecond),
			float64(br.AllocedBytesPerOp())/(kB*kB), br.Extra["size"]/kB,
		), th.Scale.Caption))
	}
}

// talkTable is one row a talk: what it was called and where it was given, with
// the date ranged right. It returns nil when the filter keeps none, so the
// heading above it is never left standing alone.
func talkTable(k *prose.Kit, filter []Tag) prose.Elem {
	tbl, any := k.Table(0, 54), false
	for _, t := range talks {
		if !wants(t.Tags, filter) {
			continue
		}
		any = true
		row := "<b>" + esc(t.Headline) + "</b>"
		if t.Description != "" {
			row += " — " + esc(t.Description)
		}
		if t.Location != "" {
			row += ", " + esc(t.Location)
		}
		tbl.Row(row, dateStr(t.Start))
	}
	if !any {
		return nil
	}
	return tbl.Align(1, prose.Right).Hairlines().Done()
}

// sectionHead is Kit.Section's heading without the number or the leading space:
// a CV's divisions are named, and the first one sits at the top of its column.
func sectionHead(k *prose.Kit, th prose.Theme, title string) prose.Elem {
	return k.Keep(
		k.Bookmark(title, 1),
		k.H2(title),
		k.Space(th.Rhythm.SectionAfter),
	)
}

// card is a sidebar block: a heading and its entries on a tinted ground. An
// empty card draws nothing, which is what a filter that matched nothing should
// leave behind.
func card(k *prose.Kit, th prose.Theme, title string, entries ...prose.Elem) prose.Elem {
	if len(entries) == 0 {
		return k.Space(0)
	}
	inner := k.Stack(append(
		[]prose.Elem{k.Sub(title), k.Rule(0.8), k.Space(5)},
		entries...,
	)...)
	body := k.Table(0).
		Pad(prose.Padding{Left: 11, Right: 11, Top: 9, Bottom: 9}).
		Band(th.Palette.Band).
		RowOf(inner).
		Done()
	return k.Stack(body, k.Space(9))
}

// events renders one entry per event: the name, a muted line of role, dates and
// place, and a bullet for each sub-event the filter keeps. The date sits on the
// meta line rather than in a margin column of its own, which buys the entry the
// better part of an inch of measure.
func events(k *prose.Kit, th prose.Theme, evs []Event, filter []Tag) []prose.Elem {
	var out []prose.Elem
	for _, e := range evs {
		if !wants(e.Tags, filter) {
			continue
		}
		lines := []prose.Elem{k.H3(headline(e.Heading))}
		if m := meta(e); m != "" {
			lines = append(lines, k.Small(m))
		}
		for _, s := range e.SubEvents {
			if !wants(s.Tags, filter) {
				continue
			}
			lines = append(lines, k.Bullet(bullet(s.Heading)))
		}
		out = append(out, k.Keep(append(lines, k.Space(6))...))
	}
	return out
}

// publicationEntry prints the citation a reader needs — authors and venue — and
// drops the ITBA form's refereed and foreign flags, which bucket a publication
// on that form and say nothing on a CV.
func publicationEntry(k *prose.Kit, p Publication) prose.Elem {
	title := headline(p.Heading)
	if p.Cited {
		title += " " + citedMark
	}
	lines := []prose.Elem{k.Styled(title, k.Theme.Scale.Body)}
	var parts []string
	if len(p.Authors) > 0 {
		parts = append(parts, esc(authors(p.Authors)))
	}
	if p.Venue != "" {
		parts = append(parts, esc(p.Venue))
	}
	if p.Description != "" {
		parts = append(parts, esc(p.Description))
	}
	if y := dateStr(p.Date); y != "" {
		parts = append(parts, y)
	}
	if s := sentences(parts); s != "" {
		lines = append(lines, k.Small(s))
	}
	return k.Keep(append(lines, k.Space(6))...)
}

// teachingEntry prints the post and the course. The dedication the ITBA form
// asks for — hours a week, semesters taught — stays in the data.
func teachingEntry(k *prose.Kit, t Event) prose.Elem {
	lines := []prose.Elem{k.Styled(headline(t.Heading), k.Theme.Scale.Body)}
	var parts []string
	if t.Description != "" {
		parts = append(parts, esc(t.Description))
	}
	if r := dateRange(t.Start, t.End); r != "" {
		parts = append(parts, r)
	}
	for _, c := range t.SubEvents {
		parts = append(parts, esc(c.Headline))
	}
	if len(parts) > 0 {
		lines = append(lines, k.Small(strings.Join(parts, " — ")))
	}
	return k.Keep(append(lines, k.Space(6))...)
}

// authors keeps a citation to one line in a narrow column: the lead author
// carries the entry and the rest are the et al. the form already implies.
func authors(a []string) string {
	if len(a) <= 1 {
		return strings.Join(a, ", ")
	}
	if strings.HasSuffix(a[0], "et al.") {
		return a[0]
	}
	return a[0] + " et al."
}

// meta is the muted line under an entry's name: role, dates, and the place only
// when it is not the one the whole CV is set in.
func meta(e Event) string {
	var parts []string
	if e.Description != "" {
		parts = append(parts, esc(e.Description))
	}
	if r := dateRange(e.Start, e.End); r != "" {
		parts = append(parts, r)
	}
	if e.Location != "" && e.Location != locBA {
		parts = append(parts, esc(e.Location))
	}
	return strings.Join(parts, " — ")
}

// wants reports whether an item belongs in a CV built for filter. An item with
// no tags of its own is unconditional.
func wants(tags, filter []Tag) bool {
	if len(filter) == 0 || len(tags) == 0 {
		return true
	}
	for _, t := range tags {
		for _, f := range filter {
			if t == f {
				return true
			}
		}
	}
	return false
}

func headline(h Heading) string {
	s := "<b>" + esc(h.Headline) + "</b>"
	if h.Href != "" {
		s = `<a href="` + h.Href + `">` + s + `</a>`
	}
	return s
}

func bullet(h Heading) string {
	s := esc(h.Headline)
	if h.Href != "" {
		s = `<a href="` + h.Href + `">` + s + `</a>`
	}
	if h.Description == "" {
		return s
	}
	return "<b>" + s + ".</b> " + esc(h.Description)
}

// sentences runs parts together, adding the period between them only where the
// part before has not already ended in one — "et al." is a part that has.
func sentences(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			if !strings.HasSuffix(b.String(), ".") {
				b.WriteByte('.')
			}
			b.WriteByte(' ')
		}
		b.WriteString(p)
	}
	return b.String()
}

// esc keeps data out of the markup dialect's way.
func esc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	return strings.ReplaceAll(s, "<", "&lt;")
}

// dayYearOnly marks a date whose source gave only a year, so dateStr prints the
// year alone. It is a day rather than a zero month because time.Date folds
// month zero into the previous December, which no later reader could tell from
// a date that really is December.
const dayYearOnly = 2

// mo=0 means no start month printing.
func dateJob(year, mo int) time.Time {
	if mo == 0 {
		return time.Date(year, time.January, dayYearOnly, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(year, time.Month(mo), 1, 0, 0, 0, 0, time.UTC)
}

func dateStr(t time.Time) string {
	switch {
	case t.IsZero():
		return ""
	case t.Equal(present):
		return "Present"
	case t.Day() == dayYearOnly:
		return strconv.Itoa(t.Year())
	}
	return t.Format("Jan 2006")
}

func dateRange(start, end time.Time) string {
	s, e := dateStr(start), dateStr(end)
	switch {
	case s == "" && e == "":
		return ""
	case s == e:
		return s // A post that began and ended in the same year reads once.
	case s == "":
		return e
	case e == "":
		return s
	case start.Year() == end.Year() && start.Day() != dayYearOnly &&
		end.Day() != dayYearOnly && !end.Equal(present):
		// Two months of one year name it once: "Oct–Dec 2025".
		return start.Format("Jan") + "–" + e
	}
	return s + "–" + e
}

var present = time.Now()

// education keeps the markdown's wording verbatim: Headline is the bullet's
// lead phrase, Description the rest of it, Href the link the markdown hangs
// on that bullet.
var education = []Event{
	{
		Heading: Heading{Headline: "Instituto Tecnológico de Buenos Aires", Description: "Mechanical Engineering"},
		Tags:    []Tag{TagMechanical, TagAerospace, TagIT},
		Start:   dateJob(2013, 0), End: dateJob(2023, 0),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline: "Teacher assistant for Thermodynamics",
			}, Tags: []Tag{TagMechanical}},
			{Heading: Heading{
				Headline:    "Award for best thesis",
				Description: `"Diseño, ensamblaje, integración y testeo de un vehículo VTVL eléctrico"`,
				Href:        "https://ri.itba.edu.ar/entities/publication/53e369ba-39e6-45d8-ad72-e505434fa94e",
			}, Tags: []Tag{TagAerospace, TagMechanical}},
			{Heading: Heading{
				Headline:    "OpenSpace competition finalist",
				Description: "Report on low-cost FTIR. Simulations performed with an in-house modified 6S (Fortran).",
				Href:        "https://github.com/spacemeters/reports/blob/master/informeFinal.pdf",
			}, Tags: []Tag{TagAerospace, TagIT}},
		},
	},
}

// publications, talks and teaching come from the ITBA faculty CV, which the
// markdown does not carry. That document dates talks to the day; dateJob keeps
// month and year, which is all the CV prints.
var publications = []Publication{
	{
		Heading:  Heading{Headline: "Go for Space Applications: From 5 DoF Flight Simulator to Legacy Model Migration"},
		Kind:     PubProceedings,
		Authors:  []string{"Patricio Whittingslow", "Juan Pablo Goncalves Sellanes"},
		Venue:    "IAA Latin American Conference on Space and Society",
		Volume:   "2",
		Tags:     []Tag{TagAerospace, TagIT},
		Date:     dateJob(2026, 0),
		Location: "Usina Cultural Salta, Argentina",
	},
	{
		Heading:  Heading{Headline: "Concept, Development and Design of a New CubeSat Project at ITBA"},
		Kind:     PubProceedings,
		Authors:  []string{"Alexis Caratozzolo et al."},
		Venue:    "IAA Latin American Conference on Space and Society",
		Volume:   "2",
		Tags:     []Tag{TagAerospace},
		Date:     dateJob(2026, 0),
		Location: "Usina Cultural Salta, Argentina",
	},
	{
		Heading: Heading{
			Headline: "BLTESTI: Benchmarking Lightweight TinyJAMBU on Embedded Systems for Trusted IoT",
			Href:     "https://doi.org/10.1109/SOCC58585.2023.10256731",
		},
		Kind:     PubJournal,
		Cited:    true,
		Refereed: true,
		Foreign:  true,
		Authors:  []string{"Mohamed El-Hadedy et al."},
		Venue:    "2023 IEEE 36th International System-on-Chip Conference (SOCC)",
		Tags:     []Tag{TagEmbedded, TagIT},
		Date:     dateJob(2023, 0),
	},
	{
		Heading: Heading{
			Headline:    "Automate Your Home Using Go",
			Description: "Part III, Chapter 4: Networking a Temperature Monitor",
		},
		Kind:    PubBookChapter,
		Cited:   true,
		Foreign: true,
		Authors: []string{"Ricardo Gerardi", "Mike Riley"},
		Tags:    []Tag{TagEmbedded, TagIT},
	},
}

// talks is newest first. Location is set only where the source names a city.
var talks = []Event{
	{
		Heading: Heading{Headline: "UI design for Biotechnology firms", Description: "Speaker @ FyneConf 2026"},
		Tags:    []Tag{TagIT},
		Start:   dateJob(2026, 9),
	},
	{
		Heading: Heading{Headline: "Go and Fortran in aerospace", Description: "Speaker @ Gophercon LATAM 2026"},
		Tags:    []Tag{TagAerospace, TagIT},
		Start:   dateJob(2026, 9),
	},
	{
		Heading: Heading{Headline: "Networking on Microcontrollers", Description: "Go devroom @ FOSDEM 2026"},
		Tags:    []Tag{TagEmbedded, TagIT},
		Start:   dateJob(2026, 2),
	},
	{
		Heading: Heading{Headline: "An Operating System in Go", Description: "Keynote Speaker @ Gophercon 2025"},
		Tags:    []Tag{TagEmbedded, TagIT},
		Start:   dateJob(2025, 8),
	},
	{
		Heading:  Heading{Headline: "Flight, Fortran and Go", Description: "Speaker @ Gophercon AU 2024"},
		Tags:     []Tag{TagAerospace, TagIT},
		Start:    dateJob(2024, 11),
		Location: "Sydney, Australia",
	},
	{
		Heading:  Heading{Headline: "Go in the Smallest of Places", Description: "Keynote Speaker @ Gophercon 2024"},
		Tags:     []Tag{TagEmbedded, TagIT},
		Start:    dateJob(2024, 7),
		Location: "Chicago, USA",
	},
	{
		Heading:  Heading{Headline: "Cognitive Load and Go", Description: "Speaker @ Gophercon 2023"},
		Tags:     []Tag{TagIT},
		Start:    dateJob(2023, 9),
		Location: "San Diego, USA",
	},
	{
		Heading:  Heading{Headline: "Aerospace Go", Description: "Keynote Speaker @ Gophercon 2022"},
		Tags:     []Tag{TagAerospace, TagIT},
		Start:    dateJob(2022, 10),
		Location: "Chicago, USA",
	},
	{
		Heading: Heading{Headline: "Go in real-time safety critical applications", Description: "Speaker @ Embedded Fest 2021"},
		Tags:    []Tag{TagEmbedded, TagIT},
		Start:   dateJob(2021, 12),
	},
}

// teaching keeps course names in their original Spanish.
var teaching = []Event{
	{
		Heading: Heading{Headline: "Instituto Tecnológico de Buenos Aires", Description: "Titular"},
		Tags:    []Tag{TagEmbedded},
		Start:   dateJob(2025, 10), End: dateJob(2025, 12),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "16.91 – Sistemas Embebidos con Aplicaciones en Biomedicina",
				Description: "3 hours weekly, 1 semester.",
			}, Tags: []Tag{TagEmbedded}},
		},
	},
	{
		Heading: Heading{Headline: "Universidad de San Andrés (Victoria)", Description: "Jefe de Trabajos Prácticos"},
		Tags:    []Tag{TagIT},
		Start:   dateJob(2023, 2), End: dateJob(2023, 12),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "Introducción al Pensamiento Computacional (Ingeniería)",
				Description: "8 hours weekly, 2 semesters.",
			}, Tags: []Tag{TagIT}},
		},
	},
	{
		Heading: Heading{Headline: "Instituto Tecnológico de Buenos Aires", Description: "Ayudante"},
		Tags:    []Tag{TagMechanical},
		Start:   dateJob(2018, 8), End: dateJob(2019, 7),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "Termodinámica",
				Description: "Under Ricardo Lauretta and Martin Rafael David. 6 hours weekly, 2 semesters.",
			}, Tags: []Tag{TagMechanical}},
		},
	},
}

// jobs is newest first, as the CV prints it.
var jobs = []Event{
	{
		Heading: Heading{Headline: "Independent Consultant", Description: "Full-time"},
		Tags:    []Tag{TagIT, TagEmbedded},
		Start:   dateJob(2025, 11), End: present,
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "Netbird GmbH",
				Description: "High performance compute expert for low latency applications.",
				Href:        "https://netbird.io/",
			}, Tags: []Tag{TagIT}},
			{Heading: Heading{
				Headline:    "AMKEN LLC",
				Description: "Firmware design for 3D printing, PnP machines and next-gen drone motor controllers using GaNFETs.",
				Href:        "https://amken3d.com/",
			}, Tags: []Tag{TagEmbedded, TagMechanical}},
			{Heading: Heading{
				Headline:    "Indgrade",
				Description: "Machine vision expert for keeping track of personnel hours.",
				Href:        "https://www.instagram.com/indgrade.ing/",
			}, Tags: []Tag{TagIT}},
			{Heading: Heading{
				Headline:    "TinyGo",
				Description: "Firmware design expert.",
				Href:        "https://tinygo.org/",
			}, Tags: []Tag{TagEmbedded, TagIT}},
		},
	},
	{
		Heading: Heading{Headline: "Novo Space", Description: "Project Lead | Full-time"},
		Tags:    []Tag{TagAerospace, TagMechanical, TagIT},
		Start:   dateJob(2024, 8), End: dateJob(2025, 11),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "Solder joint fatigue simulation",
				Description: "Led development of in-house software to replace ANSYS Sherlock, saving 80k USD/year in licensing.",
			}, Tags: []Tag{TagMechanical, TagIT}},
			{Heading: Heading{
				Headline:    "PCB macromechanics",
				Description: "Development of novel macromechanical theory to model PCB composite materials.",
			}, Tags: []Tag{TagMechanical, TagAerospace}},
			{Heading: Heading{
				Headline:    "RFD methodology",
				Description: "Led efforts to implement RFD for discussion and documentation. Today used by the whole company.",
				Href:        "https://rfd.shared.oxide.computer/rfd/0001",
			}, Tags: []Tag{TagIT}},
		},
	},
	{
		Heading: Heading{Headline: "Stämm Biotech", Description: "Firmware Tech Lead | Full-time"},
		Tags:    []Tag{TagEmbedded, TagIT},
		Start:   dateJob(2022, 11), End: dateJob(2024, 8),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "Firmware monorepo",
				Description: "Led design and implementation of a monorepo consolidating company firmware with CI/CD testing, formatting, memory checking and linting.",
			}, Tags: []Tag{TagEmbedded, TagIT}},
			{Heading: Heading{
				Headline:    "Quality standards",
				Description: "Advocated for implementation of ISO 9001 and 13485 standards of quality.",
			}, Tags: []Tag{TagEmbedded, TagIT}},
		},
	},
	{
		Heading: Heading{Headline: "LIA Aerospace", Description: "Mechanical Engineer | Full-time"},
		Tags:    []Tag{TagAerospace, TagMechanical, TagEmbedded, TagIT},
		Start:   dateJob(2020, 10), End: dateJob(2022, 11),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "HTP distillation",
				Description: "Co-designed a hydrogen peroxide distillation plant reaching 98% purity.",
				// Href:        "https://ntrs.nasa.gov/citations/20040015349",
			}, Tags: []Tag{TagAerospace, TagMechanical}},
			{Heading: Heading{
				Headline:    "Software tech lead",
				Description: "Tech lead for all software/firmware projects: rocket engine test bench, VTVL prototype control, 6DOF trajectory simulation with state of the art Runge-Kutta-Nyström 12th order integration.",
			}, Tags: []Tag{TagAerospace, TagEmbedded, TagIT}},
		},
	},
	{
		Heading: Heading{Headline: "Satellogic", Description: "Mechanical Engineer | Internship"},
		Tags:    []Tag{TagAerospace, TagMechanical},
		Start:   dateJob(2019, 0), End: dateJob(2019, 0),
		Location: locBA,
		SubEvents: []Event{
			{Heading: Heading{
				Headline:    "Structural verification",
				Description: "Response modes (vibration) for satellite structures; reports answering SpaceX Falcon 9 user requirements.",
			}, Tags: []Tag{TagAerospace, TagMechanical}},
		},
	},
}
