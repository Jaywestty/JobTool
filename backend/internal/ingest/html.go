package ingest

import (
	"html"
	"regexp"
	"strings"
)

var (
	tagRe     = regexp.MustCompile(`(?s)<[^>]+>`)
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

// StripHTML removes tags, unescapes entities and collapses whitespace.
func StripHTML(s string) string {
	s = commentRe.ReplaceAllString(s, " ")
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// FirstLine returns the text before the first tag or newline, unescaped and
// trimmed. Used for RSS items that put structured data on line one.
func FirstLine(raw string) string {
	head := raw
	if i := strings.IndexAny(head, "<\n"); i >= 0 {
		head = head[:i]
	}
	return strings.TrimSpace(spaceRe.ReplaceAllString(html.UnescapeString(head), " "))
}
