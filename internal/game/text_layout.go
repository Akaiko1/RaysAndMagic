package game

import (
	"strings"
	"unicode/utf8"
)

// truncateRunes limits text by displayed characters without splitting UTF-8.
// suffix is included in maxRunes and is omitted when the limit is too small.
// wrapUIText wraps text to lines no wider than maxWidth pixels in the active
// UI font and splits a token wider than a line, so every returned line fits
// even for URLs, content keys and CJK text.
func wrapUIText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return nil
	}
	if uiTextWidth(text) <= maxWidth {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	space := uiTextWidth(" ")
	var lines []string
	current, currentW := "", 0
	for _, word := range words {
		w := uiTextWidth(word)
		if current != "" && currentW+space+w <= maxWidth {
			current, currentW = current+" "+word, currentW+space+w
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		for w > maxWidth {
			head := uiTextPrefix(word, maxWidth)
			if head == "" { // a glyph wider than the line still gets a line
				_, size := utf8.DecodeRuneInString(word)
				head = word[:size]
			}
			lines = append(lines, head)
			word = word[len(head):]
			w = uiTextWidth(word)
		}
		current, currentW = word, w
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func truncateWrappedLines(lines []string, maxLines, maxWidth int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	lines = append([]string(nil), lines[:maxLines]...)
	lines[maxLines-1] = clipUITextSuffix(lines[maxLines-1]+"...", maxWidth, "...")
	return lines
}

func appendRunesLimited(text string, input []rune, maxRunes int) string {
	remaining := maxRunes - utf8.RuneCountInString(text)
	if remaining <= 0 || len(input) == 0 {
		return text
	}
	if len(input) > remaining {
		input = input[:remaining]
	}
	return text + string(input)
}

func removeLastRune(text string) string {
	if text == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(text)
	if size <= 0 || size > len(text) {
		return text
	}
	return text[:len(text)-size]
}
