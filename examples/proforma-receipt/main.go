// Command proforma-receipt sets a pro forma invoice as a PDF from a JSON
// description of it: who bills whom, for what, and how to be paid.
//
//	go run ./examples/proforma-receipt -template          # writes proforma.json to fill in
//	go run ./examples/proforma-receipt                    # proforma.json -> proforma.pdf
//	go run ./examples/proforma-receipt -i inv.json -o out.pdf
//	go run ./examples/proforma-receipt -i - < inv.json > inv.pdf
//
// The JSON is what -template writes. Amounts are decimal numbers of at most two
// places, held as whole cents so that the total is the sum of its lines to the
// cent. A field left empty is left off the page; a missing invoice number,
// currency, party name or item is refused.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/soypat/prose"
	"github.com/soypat/prose/md2pdf"
)

// Args is everything a pro forma invoice says.
type Args struct {
	Number     string  `json:"number"`
	IssueDate  string  `json:"issue_date"`  // YYYY-MM-DD; empty is today.
	PaymentDue string  `json:"payment_due"` // A YYYY-MM-DD date, or words such as "Upon receipt".
	Currency   string  `json:"currency"`    // ISO 4217 code, e.g. "USD".
	From       Party   `json:"from"`
	BillTo     Party   `json:"bill_to"`
	Items      []Item  `json:"items"`
	Payment    Payment `json:"payment"`
	Note       string  `json:"note"`
}

// Party is one side of the invoice. Address may run over several lines,
// separated by "\n".
type Party struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Address string `json:"address"`
	Email   string `json:"email"`
}

// Item is one line of the services billed.
type Item struct {
	Description string `json:"description"`
	Amount      Money  `json:"amount"`
}

// Payment is how the client pays: a method, and whatever the client needs to
// pay by it, in the order given — an account email, an IBAN, a SWIFT code.
type Payment struct {
	Method  string   `json:"method"`
	Details []Detail `json:"details"`
}

// Detail is one labelled particular of the payment.
type Detail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Flags struct {
	GenerateTemplate bool
	InputFile        string // "" or "-" reads stdin.
	Output           string // If not set, InputFile with its .json trailer swapped for .pdf; stdout when reading stdin.
}

func main() {
	var flags Flags
	flag.StringVar(&flags.InputFile, "i", "proforma.json", `JSON describing the invoice; "-" reads stdin`)
	flag.StringVar(&flags.Output, "o", "", `PDF to write; defaults to the input with .pdf for .json, or stdout when reading stdin; "-" is stdout`)
	flag.BoolVar(&flags.GenerateTemplate, "template", false, `write a template to the -i path ("-" is stdout) and exit`)
	flag.Parse()
	if err := run(flags); err != nil {
		log.Fatalln("proforma-receipt:", err)
	}
}

func run(flags Flags) error {
	in := flags.InputFile
	stdin := in == "" || in == "-"
	if flags.GenerateTemplate {
		return writeTemplate(in, stdin)
	}

	var r io.Reader = os.Stdin
	if !stdin {
		f, err := os.Open(in)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	} else {
		in = "stdin"
	}
	var args Args
	dec := json.NewDecoder(r)
	// A misspelt field would otherwise vanish from the invoice without a word.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}

	doc, err := prose.New(prose.DefaultTheme(), prose.Fonts{})
	if err != nil {
		return err
	}
	if err := proforma(&args, doc); err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}

	out := flags.Output
	switch {
	case out == "" && stdin:
		out = "-"
	case out == "":
		out = strings.TrimSuffix(flags.InputFile, ".json") + ".pdf"
	}
	if out == "-" {
		if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			return errors.New("refusing to write a PDF to a terminal: redirect stdout or pass -o")
		}
		return doc.Write(os.Stdout)
	}
	n, err := doc.WriteFile(out)
	if err != nil {
		return err
	}
	log.Printf("wrote %s (%.0f kB)", out, float64(n)/1000)
	return nil
}

// writeTemplate writes an invoice to fill in. It will not overwrite a file
// already there, which is likely an invoice somebody filled in.
func writeTemplate(name string, stdout bool) error {
	b, err := json.MarshalIndent(templateArgs(time.Now()), "", "\t")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if stdout {
		_, err = os.Stdout.Write(b)
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	log.Printf("wrote template %s", name)
	return nil
}

// templateArgs is an invoice whose every field shows what goes in it.
func templateArgs(now time.Time) Args {
	return Args{
		Number:     fmt.Sprintf("INV-%d-001", now.Year()),
		IssueDate:  now.Format(isoDate),
		PaymentDue: "Upon receipt",
		Currency:   "USD",
		From: Party{
			Name:    "[Your name]",
			Address: "[Your address]",
			Email:   "[Your email]",
		},
		BillTo: Party{
			Name:    "[Client's name]",
			Company: "[Company, if applicable]",
			Address: "[Client's billing address]",
			Email:   "[Client's email]",
		},
		Items: []Item{
			{Description: "[Description of services performed]", Amount: 100_00},
		},
		Payment: Payment{
			Method:  "PayPal",
			Details: []Detail{{Label: "PayPal Email", Value: "[Your PayPal email]"}},
		},
		Note: "This is a pro forma invoice issued for payment purposes. It is not proof of payment or a tax receipt.",
	}
}

const (
	isoDate  = "2006-01-02"
	longDate = "January 2, 2006"
)

// validate names every required field that is missing, so that one run reports
// all of them.
func (a *Args) validate() error {
	var missing []string
	need := func(field, v string) {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, field)
		}
	}
	need("number", a.Number)
	need("currency", a.Currency)
	need("from.name", a.From.Name)
	need("bill_to.name", a.BillTo.Name)
	if len(a.Items) == 0 {
		missing = append(missing, "items")
	}
	for i, it := range a.Items {
		need(fmt.Sprintf("items[%d].description", i), it.Description)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return nil
}

const fontBold = prose.FamilyText + "-Bold"

// proforma sets the invoice described by args into doc.
func proforma(args *Args, doc *prose.Doc) error {
	if err := args.validate(); err != nil {
		return err
	}
	issued := time.Now()
	if args.IssueDate != "" {
		t, err := time.Parse(isoDate, args.IssueDate)
		if err != nil {
			return fmt.Errorf("issue_date %q: want YYYY-MM-DD", args.IssueDate)
		}
		issued = t
	}
	cur := strings.ToUpper(strings.TrimSpace(args.Currency))
	var total Money
	for _, it := range args.Items {
		total += it.Amount
	}

	doc.Meta = prose.Meta{
		Title:   "Pro forma invoice " + args.Number,
		Author:  args.From.Name,
		Subject: "Pro forma invoice " + args.Number + " to " + args.BillTo.Name,
		Creator: "github.com/soypat/prose",
		Lang:    "en",
		Date:    issued,
	}
	// An invoice is one page as a rule; one that runs over says which invoice
	// and which page at the foot of every sheet.
	doc.Head = prose.RunningHead{Folio: func(num, total int) string {
		if total == 1 {
			return ""
		}
		return fmt.Sprintf("%s  ·  Page %d of %d", args.Number, num, total)
	}}

	k, th := doc.Kit(), doc.Theme()
	pal := th.Palette

	doc.Title("Pro Forma Invoice").
		Space(9).
		Add(k.ShortRule(58, 2.4, pal.Accent)).
		Space(18)

	// The particulars, label over value, with the sum owed beside them so the
	// reader has it without going down the page.
	cards := []prose.Card{
		{Top: "INVOICE NUMBER", Bottom: esc(args.Number)},
		{Top: "ISSUE DATE", Bottom: issued.Format(longDate)},
	}
	if args.PaymentDue != "" {
		cards = append(cards, prose.Card{Top: "PAYMENT DUE", Bottom: esc(due(args.PaymentDue))})
	}
	cards = append(cards, prose.Card{Top: "TOTAL DUE", Bottom: esc(total.Format(cur, true))})
	doc.Add(k.Cards(prose.CardStyle{
		Widths:      make([]float64, len(cards)), // Equal shares.
		TopStyle:    th.Scale.CellHead,
		BottomStyle: prose.Style{Font: fontBold, Size: 10.5, Leading: 14, Color: pal.Ink},
		Gap:         3,
		Pad:         prose.Padding{Right: 14, Top: 10, Bottom: 10},
		RuleAbove:   0.9,
		RuleBelow:   0.5,
	}, cards...))

	doc.Space(24).Add(k.Columns([]float64{0, 24, 0},
		party(k, "FROM", args.From), k.Space(0), party(k, "BILL TO", args.BillTo)))

	// The services, ruled heavy above and below as a ledger is; the column
	// head doubles as the section's.
	const amountCol = 150
	t := k.Table(0, amountCol).Head("SERVICES", "AMOUNT").Align(1, prose.Right)
	for _, it := range args.Items {
		t.Row(lines(it.Description), esc(it.Amount.Format(cur, false)))
	}
	t.Hairlines().Style().Row(-1).LineBelow(0.9, pal.Ink)
	doc.Space(30).Add(t.Done())

	sum := esc(total.Format(cur, true))
	subtotal := k.Table(0, amountCol).Row("Subtotal", sum).
		Align(0, prose.Right).Align(1, prose.Right).Done()
	doc.Add(k.Keep(subtotal, k.Space(4), totalBar(k, "Total due", sum)))

	pay := k.Table(130, 0).Head("PAYMENT INFORMATION", "")
	label := th.Scale.Cell
	label.Color = pal.Muted
	row := func(l, v string) {
		if strings.TrimSpace(v) != "" {
			pay.RowOf(k.Styled(esc(l), label), k.Styled(linked(v), th.Scale.Cell))
		}
	}
	row("Payment Method", args.Payment.Method)
	for _, dt := range args.Payment.Details {
		row(dt.Label, dt.Value)
	}
	row("Currency", cur)
	doc.Add(k.Keep(k.Space(30), pay.Hairlines().Done()))

	if strings.TrimSpace(args.Note) != "" {
		doc.Space(24).Note(lines(args.Note))
	}
	return doc.Err()
}

// party is one side of the invoice as an address block under its label.
func party(k *prose.Kit, label string, p Party) prose.Elem {
	th := k.Theme
	line := th.Scale.Body
	line.Align, line.SpaceAfter = prose.Left, 0
	name := line
	name.Font, name.Color, name.SpaceAfter = fontBold, th.Palette.Ink, 1
	elems := []prose.Elem{
		k.Styled(label, th.Scale.CellHead),
		k.Space(5),
		k.Styled(esc(p.Name), name),
	}
	for _, s := range []string{p.Company, p.Address} {
		if strings.TrimSpace(s) != "" {
			elems = append(elems, k.Styled(lines(s), line))
		}
	}
	if strings.TrimSpace(p.Email) != "" {
		elems = append(elems, k.Styled(linked(p.Email), line))
	}
	return k.Keep(elems...)
}

// totalBar is a label and the sum owed, reversed out of the ink. Its right
// padding matches the table cells' so the amount lines up with those above it.
func totalBar(k *prose.Kit, label, amount string) prose.Elem {
	pal := k.Theme.Palette
	labelSt := prose.Style{Font: fontBold, Size: 8.4, Leading: 11, Color: pal.Paper}
	amountSt := prose.Style{Font: fontBold, Size: 14, Leading: 17, Color: pal.Paper}
	return k.Table(0, 200).
		RowOf(k.Styled(esc(strings.ToUpper(label)), labelSt), k.Styled(amount, amountSt)).
		Pad(prose.Padding{Left: 14, Right: k.Theme.Pad.Right, Top: 10, Bottom: 10}).
		Band(pal.Ink).
		Valign(prose.Middle).
		Align(1, prose.Right).
		Done()
}

// due is the payment due as printed: a date in the issue date's form, or the
// words as written.
func due(s string) string {
	if t, err := time.Parse(isoDate, strings.TrimSpace(s)); err == nil {
		return t.Format(longDate)
	}
	return s
}

// esc makes data safe to set as markup.
func esc(s string) string { return md2pdf.Escape(s) }

// lines escapes text and keeps the line breaks it was written with.
func lines(s string) string {
	return strings.ReplaceAll(esc(strings.TrimSpace(s)), "\n", "<br/>")
}

// linked escapes a value and makes it a link if it is wholly an email address
// or a web address, which a reader paying the invoice will want to click.
func linked(s string) string {
	s = strings.TrimSpace(s)
	href := ""
	switch {
	case strings.ContainsAny(s, " \t\n<>\""):
	case strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "http://"):
		href = s
	case strings.Count(s, "@") == 1 && !strings.HasPrefix(s, "@") && strings.Contains(s[strings.IndexByte(s, '@'):], "."):
		href = "mailto:" + s
	}
	if href == "" {
		return lines(s)
	}
	return `<a href="` + esc(href) + `">` + esc(s) + `</a>`
}

// Money is an amount in cents. In JSON it is a decimal number of at most two
// places, bare or quoted — 1250, 1250.5, "1250.50". It is never read through a
// float, so no amount is rounded on its way in.
type Money int64

func (m *Money) UnmarshalJSON(b []byte) error {
	s := string(b)
	if strings.HasPrefix(s, `"`) {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	}
	v, err := ParseMoney(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

func (m Money) MarshalJSON() ([]byte, error) {
	sign, whole, cents := m.parts()
	return fmt.Appendf(nil, "%s%d.%02d", sign, whole, cents), nil
}

// ParseMoney reads a decimal amount of at most two places into cents.
func ParseMoney(s string) (Money, error) {
	digits, neg := strings.CutPrefix(s, "-")
	whole, frac, dot := strings.Cut(digits, ".")
	if whole == "" || (dot && frac == "") || len(frac) > 2 || !isDigits(whole) || !isDigits(frac) {
		return 0, fmt.Errorf("amount %q: want a decimal number of at most two places, such as 1250.00", s)
	}
	frac += "00"[len(frac):]
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q: %w", s, err)
	}
	if neg {
		n = -n
	}
	return Money(n), nil
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parts is m's sign, whole units and cents.
func (m Money) parts() (sign string, whole, cents int64) {
	n := int64(m)
	if n < 0 {
		sign, n = "-", -n
	}
	return sign, n / 100, n % 100
}

// symbols are the currencies written with a sign before the amount. Any other
// is written by its code after it.
var symbols = map[string]string{"USD": "$", "EUR": "€", "GBP": "£"}

// Format writes m with its thousands grouped, as "$1,250.00", or "$1,250.00 USD"
// when withCode is set; a currency with no sign is always written by its code,
// as "1,250.00 CHF".
func (m Money) Format(cur string, withCode bool) string {
	sign, whole, cents := m.parts()
	w := strconv.FormatInt(whole, 10)
	for i := len(w) - 3; i > 0; i -= 3 {
		w = w[:i] + "," + w[i:]
	}
	sym, ok := symbols[cur]
	s := fmt.Sprintf("%s%s%s.%02d", sign, sym, w, cents)
	if !ok || withCode {
		s += " " + cur
	}
	return s
}
