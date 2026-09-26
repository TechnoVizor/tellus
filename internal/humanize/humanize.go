// Package humanize turns Go identifiers and table names into readable labels.
package humanize

import (
	"strings"
	"unicode"
)

// Name converts an identifier into a label:
// "CreatedAt" becomes "Created at", "CategoryID" becomes "Category ID",
// "order_items" becomes "Order items".
func Name(s string) string {
	var words []string
	for _, part := range strings.Fields(strings.ReplaceAll(s, "_", " ")) {
		words = append(words, splitCamel(part)...)
	}
	for i, w := range words {
		switch {
		case isAcronym(w):
			// keep ID, HTML, URL as written
		case i == 0:
			words[i] = upperFirst(strings.ToLower(w))
		default:
			words[i] = strings.ToLower(w)
		}
	}
	return strings.Join(words, " ")
}

// splitCamel splits "HTMLBody" into "HTML", "Body" and "CreatedAt" into
// "Created", "At".
func splitCamel(s string) []string {
	runes := []rune(s)
	var words []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		lowerToUpper := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur)
		acronymEnd := unicode.IsUpper(prev) && unicode.IsUpper(cur) &&
			i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if lowerToUpper || acronymEnd {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	return append(words, string(runes[start:]))
}

func isAcronym(w string) bool {
	if len([]rune(w)) < 2 {
		return false
	}
	for _, r := range w {
		if unicode.IsLower(r) {
			return false
		}
	}
	return true
}

func upperFirst(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
