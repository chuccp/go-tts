package langdetect

import (
	"strings"
	"unicode"
)

type Result struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Confidence int    `json:"confidence"`
}

var languageNames = map[string]string{
	"ar": "العربية",
	"de": "Deutsch",
	"en": "English",
	"es": "Español",
	"fr": "Français",
	"hi": "हिन्दी",
	"it": "Italiano",
	"ja": "日本語",
	"ko": "한국어",
	"pt": "Português",
	"ru": "Русский",
	"th": "ไทย",
	"zh": "中文",
	"el": "Ελληνικά",
	"he": "עברית",
}

func Name(code string) string {
	if name, ok := languageNames[code]; ok {
		return name
	}
	if code == "" {
		return "未知"
	}
	return strings.ToUpper(code)
}

// Detect applies script detection first, then lightweight lexical rules for
// Latin-script languages.
func Detect(text string) Result {
	var latin, han, kana, hangul, cyrillic, arabic, devanagari, thai, greek, hebrew int
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Latin):
			latin++
		case unicode.In(r, unicode.Han):
			han++
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.In(r, unicode.Hangul):
			hangul++
		case unicode.In(r, unicode.Cyrillic):
			cyrillic++
		case unicode.In(r, unicode.Arabic):
			arabic++
		case unicode.In(r, unicode.Devanagari):
			devanagari++
		case unicode.In(r, unicode.Thai):
			thai++
		case unicode.In(r, unicode.Greek):
			greek++
		case unicode.In(r, unicode.Hebrew):
			hebrew++
		}
	}
	switch {
	case kana > 0:
		return result("ja", 98)
	case hangul > 0:
		return result("ko", 98)
	case han > 0 && han >= kana:
		return result("zh", 95)
	case devanagari > 0:
		return result("hi", 95)
	case arabic > 0:
		return result("ar", 93)
	case cyrillic > 0:
		return result("ru", 92)
	case thai > 0:
		return result("th", 92)
	case greek > 0:
		return result("el", 92)
	case hebrew > 0:
		return result("he", 92)
	case latin > 0:
		return detectLatin(text)
	default:
		return result("", 0)
	}
}

func detectLatin(text string) Result {
	lower := strings.ToLower(text)
	scores := map[string]int{
		"en": 1,
		"fr": 0,
		"de": 0,
		"es": 0,
		"pt": 0,
		"it": 0,
	}
	accentGroups := map[string]string{
		"fr": "àâæçéèêëîïôœùûüÿ",
		"de": "äöüß",
		"es": "áéíóúñ¿¡ü",
		"pt": "áâãàçéêíóôõúü",
		"it": "àèéìíîòóùú",
	}
	for code, accents := range accentGroups {
		for _, r := range lower {
			if strings.ContainsRune(accents, r) {
				scores[code] += 2
			}
		}
	}
	words := map[string][]string{
		"en": {"the", "and", "is", "are", "this", "that", "hello", "welcome", "with", "for", "you", "from"},
		"fr": {"le", "la", "les", "des", "une", "et", "est", "pour", "avec", "bonjour", "merci", "vous"},
		"de": {"der", "die", "das", "und", "ist", "nicht", "ein", "eine", "mit", "für", "hallo", "danke"},
		"es": {"el", "la", "los", "las", "una", "y", "es", "para", "con", "hola", "gracias", "usted"},
		"pt": {"o", "a", "os", "as", "uma", "e", "é", "para", "com", "olá", "obrigado", "você"},
		"it": {"il", "lo", "la", "gli", "le", "una", "e", "è", "per", "con", "ciao", "grazie", "tu"},
	}
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	for _, token := range tokens {
		for code, dictionary := range words {
			for _, word := range dictionary {
				if token == word {
					scores[code] += 3
					break
				}
			}
		}
	}

	bestCode := "en"
	bestScore := -1
	for code, score := range scores {
		if score > bestScore {
			bestCode, bestScore = code, score
		}
	}
	confidence := 55
	if bestScore > 1 {
		confidence = 70 + bestScore*3
		if confidence > 96 {
			confidence = 96
		}
	}
	return result(bestCode, confidence)
}

func result(code string, confidence int) Result {
	return Result{Code: code, Name: Name(code), Confidence: confidence}
}
