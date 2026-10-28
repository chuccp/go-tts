package tts

import (
	"regexp"
	"strings"
	"unicode"
)

var whitespaceRE = regexp.MustCompile(`\s+`)

func cleanText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case r < 32:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(b.String(), " "))
}

// SplitText preserves sentence boundaries where possible and hard-splits
// unusually long sentences on rune boundaries.
func SplitText(text string, maxRunes int) []string {
	text = cleanText(text)
	if text == "" {
		return nil
	}
	if maxRunes < 40 {
		maxRunes = 40
	}

	sentences := splitSentences([]rune(text))
	chunks := make([]string, 0, len(sentences))
	var current strings.Builder
	currentLen := 0

	flush := func() {
		chunk := strings.TrimSpace(current.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		current.Reset()
		currentLen = 0
	}

	for _, sentence := range sentences {
		runes := []rune(strings.TrimSpace(sentence))
		if len(runes) == 0 {
			continue
		}
		if currentLen > 0 && currentLen+1+len(runes) > maxRunes {
			flush()
		}
		if len(runes) > maxRunes {
			for len(runes) > maxRunes {
				cut := preferredRuneCut(runes, maxRunes)
				chunks = append(chunks, strings.TrimSpace(string(runes[:cut])))
				runes = []rune(strings.TrimSpace(string(runes[cut:])))
			}
			if len(runes) > 0 {
				if currentLen > 0 {
					flush()
				}
				current.WriteString(string(runes))
				currentLen = len(runes)
			}
			continue
		}
		if currentLen > 0 {
			current.WriteRune(' ')
			currentLen++
		}
		current.WriteString(string(runes))
		currentLen += len(runes)
	}
	flush()
	return chunks
}

func splitSentences(runes []rune) []string {
	var result []string
	start := 0
	for i, r := range runes {
		if isSentenceEnd(r) {
			j := i + 1
			for j < len(runes) && isClosingPunctuation(runes[j]) {
				j++
			}
			result = append(result, string(runes[start:j]))
			start = j
		}
	}
	if start < len(runes) {
		result = append(result, string(runes[start:]))
	}
	return result
}

func preferredRuneCut(runes []rune, limit int) int {
	if limit > len(runes) {
		limit = len(runes)
	}
	for i := limit - 1; i >= limit/2; i-- {
		switch runes[i] {
		case ' ', '\t', ',', ';', ':', '，', '；', '：':
			return i + 1
		}
	}
	return limit
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？', ';', '；':
		return true
	default:
		return false
	}
}

func isClosingPunctuation(r rune) bool {
	switch r {
	case '"', '\'', '”', '’', ')', '）', ']', '】':
		return true
	default:
		return false
	}
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_'
}

// BasicEnglishTokens mirrors the Kitten ONNX reference preprocessing:
// words and punctuation are separated by a single space before tokenization.
func BasicEnglishTokens(phonemes string) []rune {
	var tokens []rune
	runes := []rune(phonemes)
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			i++
			continue
		}
		if isWordRune(runes[i]) {
			start := i
			for i < len(runes) && isWordRune(runes[i]) {
				i++
			}
			if len(tokens) > 0 {
				tokens = append(tokens, ' ')
			}
			tokens = append(tokens, runes[start:i]...)
			continue
		}
		if len(tokens) > 0 {
			tokens = append(tokens, ' ')
		}
		tokens = append(tokens, runes[i])
		i++
	}
	return tokens
}
