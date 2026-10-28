package tts

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var (
	japaneseTokenizerOnce  sync.Once
	japaneseTokenizer      *tokenizer.Tokenizer
	japanesePronunciations map[int32]string
	japaneseTokenizerErr   error
)

//go:embed data/ipa_pronunciations.tsv.gz
var japanesePronunciationData []byte

// japanesePhonemes converts Japanese text into the phoneme inventory used by
// the local Piper Japanese model. Readings come from the embedded IPADIC
// dictionary, so this path does not launch espeak-ng or another process.
func japanesePhonemes(text string) ([]string, error) {
	text = normalizeJapaneseText(text)
	if text == "" {
		return nil, ErrEmptyText
	}
	japaneseTokenizerOnce.Do(func() {
		japaneseTokenizer, japaneseTokenizerErr = tokenizer.New(ipa.DictShrink())
		if japaneseTokenizerErr == nil {
			japanesePronunciations, japaneseTokenizerErr = loadJapanesePronunciations(japanesePronunciationData)
		}
	})
	if japaneseTokenizerErr != nil {
		return nil, japaneseTokenizerErr
	}

	var phonemes []string
	for _, token := range japaneseTokenizer.Tokenize(text) {
		surface := strings.TrimSpace(token.Surface)
		if surface == "" {
			continue
		}
		if value, ok := japanesePunctuation[surface]; ok {
			phonemes = append(phonemes, value)
			continue
		}
		var reading string
		var ok bool
		if token.Class == tokenizer.KNOWN {
			reading, ok = japanesePronunciations[int32(token.ID)]
		}
		if !ok {
			reading, ok = token.Pronunciation()
		}
		if !ok || strings.TrimSpace(reading) == "" {
			reading, ok = token.Reading()
		}
		if ok && containsKana(reading) {
			if particle, ok := japaneseParticleReading(token, surface); ok {
				reading = particle
			}
			phonemes = append(phonemes, katakanaToPhonemes(reading)...)
			continue
		}
		if containsKana(surface) {
			phonemes = append(phonemes, katakanaToPhonemes(hiraganaToKatakana(surface))...)
			continue
		}
		if value, ok := japanesePunctuation[surface]; ok {
			phonemes = append(phonemes, value)
		}
	}
	if len(phonemes) == 0 {
		return nil, errors.New("Japanese text frontend produced no phonemes")
	}
	return phonemes, nil
}

func loadJapanesePronunciations(compressed []byte) (map[int32]string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open Japanese pronunciation data: %w", err)
	}
	defer reader.Close()
	result := make(map[int32]string, 400000)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		idText, pronunciation, ok := strings.Cut(scanner.Text(), "\t")
		if !ok || pronunciation == "" {
			continue
		}
		id, err := strconv.ParseInt(idText, 10, 32)
		if err != nil {
			continue
		}
		result[int32(id)] = pronunciation
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Japanese pronunciation data: %w", err)
	}
	if len(result) == 0 {
		return nil, errors.New("Japanese pronunciation data is empty")
	}
	return result, nil
}

func normalizeJapaneseText(text string) string {
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer(
		"：", ",",
		"；", ",",
		"，", ",",
		"。", ".",
		"！", "!",
		"？", "?",
		"、", ",",
		"・", ",",
		"…", "…",
		"\n", ".",
		"\r", ".",
	)
	text = replacer.Replace(text)
	text = japaneseNumberPattern.ReplaceAllStringFunc(text, japaneseNumberToKana)
	text = strings.NewReplacer(
		"こんにちは", "こんにちわ",
		"こんばんは", "こんばんわ",
	).Replace(text)
	var normalized strings.Builder
	normalized.Grow(len(text))
	for _, char := range strings.ToLower(text) {
		if value, ok := japaneseSymbolReadings[char]; ok {
			normalized.WriteString(value)
			continue
		}
		normalized.WriteRune(char)
	}
	return strings.TrimSpace(normalized.String())
}

var japaneseNumberPattern = regexp.MustCompile(`[0-9]{1,3}(?:,[0-9]{3})+|[0-9]+(?:\.[0-9]+)?`)

var japaneseNumberDigits = [...]string{"ゼロ", "イチ", "ニ", "サン", "ヨン", "ゴ", "ロク", "ナナ", "ハチ", "キュウ"}

// japaneseNumberToKana reads ordinary cardinals. Phone-number digit-by-digit
// speech requires author intent, so ordinary numbers use the natural grouping.
func japaneseNumberToKana(value string) string {
	whole, fraction, hasFraction := strings.Cut(strings.ReplaceAll(value, ",", ""), ".")
	if strings.TrimSpace(whole) == "" {
		return value
	}
	number, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return japaneseDigitsToKana(strings.ReplaceAll(value, ",", ""))
	}
	result := japaneseCardinalToKana(number)
	if hasFraction && fraction != "" {
		result += "テン" + japaneseDigitsToKana(fraction)
	}
	return result
}

func japaneseDigitsToKana(digits string) string {
	var result strings.Builder
	for _, digit := range digits {
		if digit >= '0' && digit <= '9' {
			result.WriteString(japaneseNumberDigits[digit-'0'])
		}
	}
	return result.String()
}

func japaneseCardinalToKana(number int64) string {
	if number == 0 {
		return japaneseNumberDigits[0]
	}
	units := []string{"", "マン", "オク", "チョウ", "ケイ"}
	var groups []int
	for number > 0 {
		groups = append(groups, int(number%10000))
		number /= 10000
	}
	var result strings.Builder
	for index := len(groups) - 1; index >= 0; index-- {
		if groups[index] == 0 {
			continue
		}
		result.WriteString(japaneseGroupToKana(groups[index]))
		if index < len(units) {
			result.WriteString(units[index])
		}
	}
	return result.String()
}

func japaneseGroupToKana(group int) string {
	digits := [...]string{"", "イチ", "ニ", "サン", "ヨン", "ゴ", "ロク", "ナナ", "ハチ", "キュウ"}
	units := [...]string{"", "ジュウ", "ヒャク", "セン"}
	digitsNames := [4]int{}
	for index := range digitsNames {
		digitsNames[index] = group % 10
		group /= 10
	}
	var result strings.Builder
	for index := 3; index >= 0; index-- {
		digit := digitsNames[index]
		if digit == 0 {
			continue
		}
		if digit != 1 || index == 0 {
			result.WriteString(digits[digit])
		}
		result.WriteString(units[index])
	}
	return result.String()
}

func japaneseParticleReading(token tokenizer.Token, surface string) (string, bool) {
	pos := token.POS()
	if len(pos) == 0 || pos[0] != "助詞" {
		return "", false
	}
	switch surface {
	case "は":
		return "ワ", true
	case "へ":
		return "エ", true
	case "を":
		return "オ", true
	default:
		return "", false
	}
}

func containsKana(text string) bool {
	for _, char := range text {
		if unicode.In(char, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

func hiraganaToKatakana(text string) string {
	var result strings.Builder
	result.Grow(len(text))
	for _, char := range text {
		switch {
		case char == 'う':
			result.WriteRune('ウ')
		case char >= 'ぁ' && char <= 'ゖ':
			result.WriteRune(char + ('ァ' - 'ぁ'))
		default:
			result.WriteRune(char)
		}
	}
	return result.String()
}

// katakanaToPhonemes maps standard Japanese kana to the single-codepoint IPA
// symbols accepted by Piper's Japanese voice model.
func katakanaToPhonemes(text string) []string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	result := make([]string, 0, len(runes)*2)
	for index := 0; index < len(runes); {
		matched := ""
		for length := 2; length >= 1; length-- {
			if index+length > len(runes) {
				continue
			}
			candidate := string(runes[index : index+length])
			if value, ok := japaneseKanaPhonemes[candidate]; ok {
				result = append(result, value...)
				matched = candidate
				break
			}
		}
		if matched != "" {
			index += len([]rune(matched))
			continue
		}
		if value, ok := japanesePunctuation[string(runes[index])]; ok {
			result = append(result, value)
		}
		index++
	}
	return result
}

var japanesePunctuation = map[string]string{
	",": ",",
	".": ".",
	"!": "!",
	"?": "?",
	"…": "…",
	"'": "'",
	"-": "-",
	"、": ",",
	"。": ".",
	"！": "!",
	"？": "?",
	"・": ",",
}

var japaneseSymbolReadings = map[rune]string{
	'a': "エー", 'b': "ビー", 'c': "シー", 'd': "ディー", 'e': "イー",
	'f': "エフ", 'g': "ジー", 'h': "エイチ", 'i': "アイ", 'j': "ジェー",
	'k': "ケー", 'l': "エル", 'm': "エム", 'n': "エヌ", 'o': "オー",
	'p': "ピー", 'q': "キュー", 'r': "アール", 's': "エス", 't': "ティー",
	'u': "ユー", 'v': "ブイ", 'w': "ダブリュー", 'x': "エックス",
	'y': "ワイ", 'z': "ゼット",
	'#': "シャープ", '%': "パーセント", '&': "アンド", '+': "プラス",
	'=': "イコール", '@': "アット", '<': "小なり", '>': "大なり",
}

// The value is a sequence of Piper phonemes, not a romanization.
var japaneseKanaPhonemes = map[string][]string{
	"ア": {"a"}, "イ": {"i"}, "ウ": {"u"}, "エ": {"e"}, "オ": {"o"},
	"ァ": {"a"}, "ィ": {"i"}, "ゥ": {"u"}, "ェ": {"e"}, "ォ": {"o"},
	"カ": {"k", "a"}, "キ": {"k", "i"}, "ク": {"k", "u"}, "ケ": {"k", "e"}, "コ": {"k", "o"},
	"ガ": {"ɡ", "a"}, "ギ": {"ɡ", "i"}, "グ": {"ɡ", "u"}, "ゲ": {"ɡ", "e"}, "ゴ": {"ɡ", "o"},
	"サ": {"s", "a"}, "シ": {"ɕ", "i"}, "ス": {"s", "u"}, "セ": {"s", "e"}, "ソ": {"s", "o"},
	"ザ": {"z", "a"}, "ジ": {"d", "ʑ", "i"}, "ズ": {"d", "z", "u"}, "ゼ": {"z", "e"}, "ゾ": {"z", "o"},
	"タ": {"t", "a"}, "チ": {"t", "ɕ", "i"}, "ツ": {"ʦ", "u"}, "テ": {"t", "e"}, "ト": {"t", "o"},
	"ダ": {"d", "a"}, "ヂ": {"d", "ʑ", "i"}, "ヅ": {"d", "z", "u"}, "デ": {"d", "e"}, "ド": {"d", "o"},
	"ナ": {"n", "a"}, "ニ": {"n", "i"}, "ヌ": {"n", "u"}, "ネ": {"n", "e"}, "ノ": {"n", "o"},
	"ハ": {"h", "a"}, "ヒ": {"h", "i"}, "フ": {"ɸ", "u"}, "ヘ": {"h", "e"}, "ホ": {"h", "o"},
	"バ": {"b", "a"}, "ビ": {"b", "i"}, "ブ": {"b", "u"}, "ベ": {"b", "e"}, "ボ": {"b", "o"},
	"パ": {"p", "a"}, "ピ": {"p", "i"}, "プ": {"p", "u"}, "ペ": {"p", "e"}, "ポ": {"p", "o"},
	"マ": {"m", "a"}, "ミ": {"m", "i"}, "ム": {"m", "u"}, "メ": {"m", "e"}, "モ": {"m", "o"},
	"ヤ": {"j", "a"}, "ユ": {"j", "u"}, "ヨ": {"j", "o"},
	"ャ": {"j", "a"}, "ュ": {"j", "u"}, "ョ": {"j", "o"},
	"ラ": {"ɾ", "a"}, "リ": {"ɾ", "i"}, "ル": {"ɾ", "u"}, "レ": {"ɾ", "e"}, "ロ": {"ɾ", "o"},
	"ワ": {"w", "a"}, "ヰ": {"i"}, "ヱ": {"e"}, "ヲ": {"o"},
	"ヮ": {"w", "a"},
	"ン": {"ɴ"}, "ッ": {"q"}, "ー": {":"}, "ヴ": {"v", "u"},

	"キャ": {"k", "ʲ", "a"}, "キュ": {"k", "ʲ", "u"}, "キョ": {"k", "ʲ", "o"},
	"ギャ": {"ɡ", "ʲ", "a"}, "ギュ": {"ɡ", "ʲ", "u"}, "ギョ": {"ɡ", "ʲ", "o"},
	"シャ": {"ɕ", "a"}, "シュ": {"ɕ", "u"}, "シェ": {"ɕ", "e"}, "ショ": {"ɕ", "o"},
	"ジャ": {"d", "ʑ", "a"}, "ジュ": {"d", "ʑ", "u"}, "ジェ": {"d", "ʑ", "e"}, "ジョ": {"d", "ʑ", "o"},
	"チャ": {"t", "ɕ", "a"}, "チュ": {"t", "ɕ", "u"}, "チェ": {"t", "ɕ", "e"}, "チョ": {"t", "ɕ", "o"},
	"ニャ": {"n", "ʲ", "a"}, "ニュ": {"n", "ʲ", "u"}, "ニョ": {"n", "ʲ", "o"},
	"ヒャ": {"h", "ʲ", "a"}, "ヒュ": {"h", "ʲ", "u"}, "ヒョ": {"h", "ʲ", "o"},
	"ビャ": {"b", "ʲ", "a"}, "ビュ": {"b", "ʲ", "u"}, "ビョ": {"b", "ʲ", "o"},
	"ピャ": {"p", "ʲ", "a"}, "ピュ": {"p", "ʲ", "u"}, "ピョ": {"p", "ʲ", "o"},
	"ミャ": {"m", "ʲ", "a"}, "ミュ": {"m", "ʲ", "u"}, "ミョ": {"m", "ʲ", "o"},
	"リャ": {"ɾ", "ʲ", "a"}, "リュ": {"ɾ", "ʲ", "u"}, "リョ": {"ɾ", "ʲ", "o"},
	"ファ": {"ɸ", "a"}, "フィ": {"ɸ", "i"}, "フェ": {"ɸ", "e"}, "フォ": {"ɸ", "o"},
	"ティ": {"t", "i"}, "ディ": {"d", "i"}, "トゥ": {"t", "u"}, "ドゥ": {"d", "u"},
	"ウィ": {"w", "i"}, "ウェ": {"w", "e"}, "ウォ": {"w", "o"},
	"ツァ": {"ʦ", "a"}, "ツィ": {"ʦ", "i"}, "ツェ": {"ʦ", "e"}, "ツォ": {"ʦ", "o"},
}
