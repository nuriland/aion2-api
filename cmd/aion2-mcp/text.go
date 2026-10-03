package main

import (
	"html"
	"regexp"
	"strings"
)

var (
	dropped  = regexp.MustCompile(`(?is)<!--.*?-->|<(script|style)\b.*?</(script|style)>`)
	links    = regexp.MustCompile(`(?is)<a\b[^>]*?\bhref="(https?://[^"]+)"[^>]*>(.*?)</a>`) // in-page #anchors mean nothing outside the page
	listItem = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	breaks   = regexp.MustCompile(`(?i)<br\s*/?>|</?(p|div|h[1-6]|ul|ol|li|tr|table|blockquote)\b[^>]*>`)
	tags     = regexp.MustCompile(`<[^>]*(>|$)`)                  // a body cut off mid-tag leaves one unclosed
	spaces   = regexp.MustCompile(`[ \t\x{a0}\x{200b}\x{feff}]+`) // NC's editor pads empty paragraphs with zero-width spaces
	dashes   = regexp.MustCompile(`(?m)^-\n+`)                    // <li><p>…</p></li> leaves the dash on a line of its own
	blanks   = regexp.MustCompile(`\n\s*\n\s*\n+`)
)

// htmlText flattens a post body to plain text
func htmlText(body string) string {
	body = dropped.ReplaceAllString(body, "")
	body = links.ReplaceAllStringFunc(body, link)
	body = listItem.ReplaceAllString(body, "\n- ")
	body = breaks.ReplaceAllString(body, "\n")
	body = tags.ReplaceAllString(body, "")
	body = html.UnescapeString(body)
	body = spaces.ReplaceAllString(body, " ")

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	body = strings.Join(lines, "\n")
	body = dashes.ReplaceAllString(body, "- ")
	body = blanks.ReplaceAllString(body, "\n\n")
	return strings.TrimSpace(body)
}

// link keeps where a link goes
func link(a string) string {
	m := links.FindStringSubmatch(a)
	href, text := m[1], strings.TrimSpace(tags.ReplaceAllString(m[2], ""))
	if text == "" || text == href {
		return href
	}
	return text + " (" + href + ")"
}
