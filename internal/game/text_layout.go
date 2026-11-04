package game

import (
	"strings"
	"unicode/utf8"
)

// wrapUIText wraps text to lines no wider than maxWidth pixels in the active
// UI font and splits a token wider than a line, so every returned line fits
// even for URLs, content keys and CJK text.
func wrapUIText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return nil
	}
	// A newline starts a paragraph: each one wraps on its own.
	if strings.Contains(text, "\n") {
		var lines []string
		for _, paragraph := range strings.Split(text, "\n") {
			lines = append(lines, wrapUIText(paragraph, maxWidth)...)
		}
		return lines
	}
	if uiTextWidth(text) <= maxWidth {
		return []string{text}
	}
	return wrapWords(strings.Fields(text), maxWidth)
}

// firstUIWordsLine cuts the first line off words, joined by single spaces, and
// returns the words left for the next. A word may hold spaces of its own; it
// is cut only when it is wider than a line, and its tail starts the rest.
func firstUIWordsLine(words []string, maxWidth int) (string, []string) {
	if joined := strings.Join(words, " "); uiTextWidth(joined) <= maxWidth {
		return joined, nil
	}
	return cutUILine(words, maxWidth)
}

// wrapWords is the greedy cut behind wrapUIText, once the whole text is known
// not to fit: one cutUILine per line.
func wrapWords(words []string, maxWidth int) []string {
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	for len(words) > 0 {
		var line string
		line, words = cutUILine(words, maxWidth)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// cutUILine takes words while they fit a line, joined by single spaces; a
// first word wider than the line gives up its widest head instead, and its
// tail, without the spaces at the cut, leads the words left.
func cutUILine(words []string, maxWidth int) (string, []string) {
	space := uiTextWidth(" ")
	line, lineW := "", 0
	for i, word := range words {
		w := uiTextWidth(word)
		if line != "" {
			if lineW+space+w > maxWidth {
				return line, words[i:]
			}
			line, lineW = line+" "+word, lineW+space+w
			continue
		}
		if w <= maxWidth {
			line, lineW = word, w
			continue
		}
		head := uiTextPrefix(word, maxWidth)
		if head == "" { // a glyph wider than the line still gets a line
			_, size := utf8.DecodeRuneInString(word)
			head = word[:size]
		}
		if tail := strings.TrimLeft(word[len(head):], " "); tail != "" {
			return head, append([]string{tail}, words[i+1:]...)
		}
		return head, words[i+1:]
	}
	return line, nil
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
