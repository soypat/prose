package md2pdf

import (
	"fmt"
	"html"
	"image/color"
	"regexp"
	"strings"
	"unicode"
)

// Escape makes text safe to set as markup. The dialect resolves only &amp;,
// &lt;, &gt; and &#160;, so those are all it may use.
func Escape(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	return markupEscaper.Replace(s)
}

var markupEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// escapeAttr makes a link target safe to quote in a tag.
func escapeAttr(s string) string {
	return Escape(strings.ReplaceAll(s, `"`, "%22"))
}

// bareLink finds the addresses a reader expects to click: a URL, or an email.
var bareLink = regexp.MustCompile(`https?://[^\s<>|]*[^\s<>|.,;:!?)]|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// linkify escapes text and turns the addresses in it into links.
func linkify(s string) string {
	locs := bareLink.FindAllStringIndex(s, -1)
	if locs == nil {
		return Escape(s)
	}
	var sb strings.Builder
	prev := 0
	for _, l := range locs {
		addr := s[l[0]:l[1]]
		href := addr
		if !strings.Contains(addr, "://") {
			href = "mailto:" + addr
		}
		sb.WriteString(Escape(s[prev:l[0]]))
		sb.WriteString(`<a href="` + escapeAttr(href) + `">` + Escape(addr) + "</a>")
		prev = l[1]
	}
	sb.WriteString(Escape(s[prev:]))
	return sb.String()
}

// StripTags is what a reader sees of markup: the text, entities resolved.
func StripTags(s string) string {
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

// SentenceCase recases a heading the source set in capitals. A heading with any
// lowercase in it was cased deliberately and is left alone.
func SentenceCase(s string) string {
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

// hexColor renders a colour as markup names one, and no colour as nothing.
func hexColor(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
