package tts

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:embed data/pinyin_dict.json data/phrases_dict.json
var chinesePinyinAssets embed.FS

type chinesePinyinData struct {
	chars     map[rune]string
	phrases   map[string][]string
	maxPhrase int
}

var (
	chinesePinyinOnce sync.Once
	chinesePinyin     *chinesePinyinData
	chinesePinyinErr  error
)

var pinyinInitials = []string{
	"zh", "ch", "sh", "b", "p", "m", "f", "d", "t", "n", "l",
	"g", "k", "h", "j", "q", "x", "r", "z", "c", "s",
}

var pinyinInitialIPA = map[string]string{
	"b": "p", "p": "pʰ", "m": "m", "f": "f",
	"d": "t", "t": "tʰ", "n": "n", "l": "l",
	"g": "k", "k": "kʰ", "h": "x",
	"j": "tɕ", "q": "tɕʰ", "x": "ɕ",
	"zh": "ʈʂ", "ch": "ʈʂʰ", "sh": "ʂ", "r": "ɻ",
	"z": "ts", "c": "tsʰ", "s": "s",
}

type ipaFinal struct {
	vowel string
	coda  string
}

var pinyinFinalIPA = map[string]ipaFinal{
	"a": {"a", ""}, "ai": {"ai", ""}, "an": {"a", "n"}, "ang": {"a", "ŋ"}, "ao": {"au", ""},
	"e": {"ɤ", ""}, "ei": {"ei", ""}, "en": {"ə", "n"}, "eng": {"ə", "ŋ"},
	"i": {"i", ""}, "ia": {"ja", ""}, "ian": {"jɛ", "n"}, "iang": {"ja", "ŋ"}, "iao": {"jau", ""},
	"ie": {"je", ""}, "in": {"i", "n"}, "ing": {"i", "ŋ"}, "iong": {"jʊ", "ŋ"},
	"ong": {"ʊ", "ŋ"}, "ou": {"ou", ""},
	"u": {"u", ""}, "ua": {"wa", ""}, "uai": {"wai", ""}, "uan": {"wa", "n"}, "uang": {"wa", "ŋ"},
	"uei": {"wei", ""}, "uen": {"wə", "n"}, "ueng": {"wə", "ŋ"}, "uo": {"wo", ""},
	"o": {"wo", ""},
	"ü": {"y", ""}, "üe": {"ɥe", ""}, "üan": {"ɥɛ", "n"}, "ün": {"y", "n"},
}

var pinyinSpecialIPA = map[string]ipaFinal{
	"m": {"m", ""}, "n": {"n", ""}, "ng": {"ŋ", ""}, "hm": {"h", "m"}, "hng": {"h", "ŋ"},
	"ê": {"ɛ", ""}, "io": {"jɔ", ""}, "o": {"ɔ", ""},
}

var pinyinToneMark = map[rune]struct {
	base rune
	tone int
}{
	'ā': {'a', 1}, 'á': {'a', 2}, 'ǎ': {'a', 3}, 'à': {'a', 4},
	'ē': {'e', 1}, 'é': {'e', 2}, 'ě': {'e', 3}, 'è': {'e', 4},
	'ī': {'i', 1}, 'í': {'i', 2}, 'ǐ': {'i', 3}, 'ì': {'i', 4},
	'ō': {'o', 1}, 'ó': {'o', 2}, 'ǒ': {'o', 3}, 'ò': {'o', 4},
	'ū': {'u', 1}, 'ú': {'u', 2}, 'ǔ': {'u', 3}, 'ù': {'u', 4},
	'ǖ': {'ü', 1}, 'ǘ': {'ü', 2}, 'ǚ': {'ü', 3}, 'ǜ': {'ü', 4},
}

func loadChinesePinyinData() (*chinesePinyinData, error) {
	chinesePinyinOnce.Do(func() {
		charBytes, err := chinesePinyinAssets.ReadFile("data/pinyin_dict.json")
		if err != nil {
			chinesePinyinErr = err
			return
		}
		phraseBytes, err := chinesePinyinAssets.ReadFile("data/phrases_dict.json")
		if err != nil {
			chinesePinyinErr = err
			return
		}

		var rawChars map[string]string
		if err := json.Unmarshal(charBytes, &rawChars); err != nil {
			chinesePinyinErr = fmt.Errorf("parse Chinese character pinyin data: %w", err)
			return
		}
		var rawPhrases map[string][][]string
		if err := json.Unmarshal(phraseBytes, &rawPhrases); err != nil {
			chinesePinyinErr = fmt.Errorf("parse Chinese phrase pinyin data: %w", err)
			return
		}

		data := &chinesePinyinData{
			chars:   make(map[rune]string, len(rawChars)),
			phrases: make(map[string][]string, len(rawPhrases)),
		}
		for key, value := range rawChars {
			codePoint, err := strconv.Atoi(key)
			if err != nil {
				continue
			}
			readings := strings.Split(value, ",")
			for _, reading := range readings {
				if pinyin, ok := toTone3(reading); ok {
					data.chars[rune(codePoint)] = pinyin
					break
				}
			}
		}
		for phrase, groups := range rawPhrases {
			runes := []rune(phrase)
			if len(runes) < 2 || !allHan(runes) {
				continue
			}
			syllables := make([]string, 0, len(groups))
			valid := true
			for _, group := range groups {
				if len(group) == 0 {
					valid = false
					break
				}
				pinyin, ok := toTone3(group[0])
				if !ok {
					valid = false
					break
				}
				syllables = append(syllables, pinyin)
			}
			if !valid || len(syllables) != len(runes) {
				continue
			}
			data.phrases[phrase] = syllables
			if len(runes) > data.maxPhrase {
				data.maxPhrase = len(runes)
			}
		}
		chinesePinyin = data
	})
	return chinesePinyin, chinesePinyinErr
}

// chinesePhonemes converts Mandarin text into the Kokoro/Misaki phoneme
// inventory. Phrases are matched longest-first so polyphonic characters use
// their pronunciation for the surrounding word.
func chinesePhonemes(ctx context.Context, text string) (string, error) {
	data, err := loadChinesePinyinData()
	if err != nil {
		return "", err
	}
	runes := []rune(text)
	var output strings.Builder
	pendingSpace := false

	appendUnit := func(unit string, punctuation bool) {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			return
		}
		if punctuation {
			output.WriteString(unit)
		} else {
			if output.Len() > 0 && pendingSpace {
				output.WriteByte(' ')
			}
			output.WriteString(unit)
		}
		pendingSpace = true
	}

	for i := 0; i < len(runes); {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		r := runes[i]
		if unicode.Is(unicode.Han, r) {
			if phraseLen, syllables := matchChinesePhrase(data, runes, i); phraseLen > 0 {
				var word strings.Builder
				for _, syllable := range syllables {
					word.WriteString(pinyinToIPA(syllable))
				}
				appendUnit(word.String(), false)
				i += phraseLen
				continue
			}

			var word strings.Builder
			for i < len(runes) && unicode.Is(unicode.Han, runes[i]) {
				if word.Len() > 0 {
					if phraseLen, _ := matchChinesePhrase(data, runes, i); phraseLen > 0 {
						break
					}
				}
				syllable, ok := data.chars[runes[i]]
				if !ok {
					return "", fmt.Errorf("no Mandarin pronunciation for %q", string(runes[i]))
				}
				word.WriteString(pinyinToIPA(syllable))
				i++
			}
			appendUnit(word.String(), false)
			continue
		}

		if mapped, ok := chinesePunctuation(r); ok {
			appendUnit(mapped, true)
			i++
			continue
		}
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if r >= '0' && r <= '9' {
			digits := []rune("零一二三四五六七八九")
			syllable := data.chars[digits[int(r-'0')]]
			appendUnit(pinyinToIPA(syllable), false)
			i++
			continue
		}
		if isASCIILetter(r) {
			start := i
			for i < len(runes) && (isASCIILetter(runes[i]) || runes[i] == '\'' || runes[i] == '-') {
				i++
			}
			english, err := phonemizeEnglish(ctx, string(runes[start:i]), false)
			if err != nil {
				english = string(runes[start:i])
			}
			appendUnit(english, false)
			continue
		}
		i++
	}
	return strings.TrimSpace(output.String()), nil
}

func matchChinesePhrase(data *chinesePinyinData, runes []rune, start int) (int, []string) {
	maxLength := min(data.maxPhrase, len(runes)-start)
	for length := maxLength; length >= 2; length-- {
		part := runes[start : start+length]
		if !allHan(part) {
			continue
		}
		if syllables, ok := data.phrases[string(part)]; ok {
			return length, syllables
		}
	}
	return 0, nil
}

func allHan(runes []rune) bool {
	for _, r := range runes {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	return len(runes) > 0
}

func toTone3(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", false
	}
	tone := 5
	var normalized strings.Builder
	for _, r := range value {
		if marked, ok := pinyinToneMark[r]; ok {
			normalized.WriteRune(marked.base)
			tone = marked.tone
			continue
		}
		if r == 'v' {
			normalized.WriteRune('ü')
			continue
		}
		if r >= '1' && r <= '5' {
			tone = int(r - '0')
			continue
		}
		normalized.WriteRune(r)
	}
	base := normalized.String()
	if base == "" {
		return "", false
	}
	return base + strconv.Itoa(tone), true
}

func splitPinyinSyllable(value string) (string, string) {
	value = strings.ReplaceAll(value, "v", "ü")
	for _, initial := range pinyinInitials {
		if strings.HasPrefix(value, initial) {
			final := strings.TrimPrefix(value, initial)
			if initial == "j" || initial == "q" || initial == "x" {
				if strings.HasPrefix(final, "u") {
					final = "ü" + strings.TrimPrefix(final, "u")
				}
			}
			switch final {
			case "iu":
				final = "iou"
			case "ui":
				final = "uei"
			case "un":
				if initial != "j" && initial != "q" && initial != "x" {
					final = "uen"
				}
			}
			return initial, final
		}
	}
	if strings.HasPrefix(value, "y") {
		rest := strings.TrimPrefix(value, "y")
		switch rest {
		case "i":
			return "", "i"
		case "a":
			return "", "ia"
		case "e":
			return "", "ie"
		case "ao":
			return "", "iao"
		case "ou":
			return "", "iou"
		case "an":
			return "", "ian"
		case "in":
			return "", "in"
		case "ang":
			return "", "iang"
		case "ing":
			return "", "ing"
		case "ong":
			return "", "iong"
		case "u":
			return "", "ü"
		case "ue":
			return "", "üe"
		case "uan":
			return "", "üan"
		case "un":
			return "", "ün"
		default:
			return "", rest
		}
	}
	if strings.HasPrefix(value, "w") {
		rest := strings.TrimPrefix(value, "w")
		switch rest {
		case "u":
			return "", "u"
		case "a":
			return "", "ua"
		case "o":
			return "", "uo"
		case "ai":
			return "", "uai"
		case "ei":
			return "", "uei"
		case "an":
			return "", "uan"
		case "en":
			return "", "uen"
		case "ang":
			return "", "uang"
		case "eng":
			return "", "ueng"
		default:
			return "", rest
		}
	}
	return "", value
}

func pinyinToIPA(value string) string {
	tone := 5
	if len(value) > 0 {
		if parsed, err := strconv.Atoi(value[len(value)-1:]); err == nil && parsed >= 1 && parsed <= 5 {
			tone = parsed
			value = value[:len(value)-1]
		}
	}
	if value == "er" {
		return "ɚ" + pinyinToneIPA(tone)
	}
	if special, ok := pinyinSpecialIPA[value]; ok {
		return special.vowel + pinyinToneIPA(tone) + special.coda
	}

	initial, final := splitPinyinSyllable(value)
	ipa := pinyinInitialIPA[initial]
	switch {
	case (initial == "z" || initial == "c" || initial == "s") && final == "i":
		ipa += "ɹ̩"
	case (initial == "zh" || initial == "ch" || initial == "sh" || initial == "r") && final == "i":
		ipa += "ɹ̩"
	default:
		mapped := pinyinFinalIPA[final]
		return ipa + mapped.vowel + pinyinToneIPA(tone) + mapped.coda
	}
	return ipa + pinyinToneIPA(tone)
}

func pinyinToneIPA(tone int) string {
	switch tone {
	case 1:
		return "→"
	case 2:
		return "↗"
	case 3:
		return "↓"
	case 4:
		return "↘"
	default:
		return ""
	}
}

func chinesePunctuation(r rune) (string, bool) {
	switch r {
	case '，', '、':
		return ",", true
	case '。', '．':
		return ".", true
	case '！':
		return "!", true
	case '？':
		return "?", true
	case '：':
		return ":", true
	case '；':
		return ";", true
	case '“', '「', '《', '【':
		return "\"", true
	case '”', '」', '》', '】':
		return "\"", true
	case '（':
		return "(", true
	case '）':
		return ")", true
	}
	return "", false
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
