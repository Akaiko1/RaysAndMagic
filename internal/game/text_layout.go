package game

import (
	"strings"
	"unicode/utf8"
)

// truncateRunes limits text by displayed characters without splitting UTF-8.
// suffix is included in maxRunes and is omitted when the limit is too small.
func truncateRunes(text string, maxRunes int, suffix string) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= maxRunes {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-len(suffixRunes)]) + suffix
}

// wrapText wraps by rune count and splits oversized tokens, so every returned
// line is guaranteed to fit maxChars even for URLs, content keys, and CJK text.
func wrapText(text string, maxChars int) []string {
	if maxChars <= 0 {
		return nil
	}
	if utf8.RuneCountInString(text) <= maxChars {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, len(words))
	current := make([]rune, 0, maxChars)
	flush := func() {
		if len(current) > 0 {
			lines = append(lines, string(current))
			current = current[:0]
		}
	}

	for _, word := range words {
		wordRunes := []rune(word)
		if len(current) > 0 && len(current)+1+len(wordRunes) <= maxChars {
			current = append(current, ' ')
			current = append(current, wordRunes...)
			continue
		}
		flush()
		for len(wordRunes) > maxChars {
			lines = append(lines, string(wordRunes[:maxChars]))
			wordRunes = wordRunes[maxChars:]
		}
		current = append(current, wordRunes...)
	}
	flush()
	return lines
}

func wrapDebugText(text string, maxWidth int) []string {
	return wrapText(text, maxWidth/debugTextCharWidth)
}

func truncateWrappedLines(lines []string, maxLines, maxWidth int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	lines = append([]string(nil), lines[:maxLines]...)
	maxChars := maxWidth / debugTextCharWidth
	lines[maxLines-1] = truncateRunes(lines[maxLines-1]+"...", maxChars, "...")
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
