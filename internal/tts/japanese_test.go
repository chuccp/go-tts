package tts

import (
	"reflect"
	"testing"
)

func TestJapanesePhonemesUseDictionaryReadings(t *testing.T) {
	t.Parallel()
	phonemes, err := japanesePhonemes("こんにちは、世界。")
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{"k", "o", "ɴ", "n", "i", "t", "ɕ", "i", "w", "a", ","}
	if len(phonemes) < len(wantPrefix) || !reflect.DeepEqual(phonemes[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("phoneme prefix = %#v, want %#v", phonemes, wantPrefix)
	}
}

func TestJapanesePhonemesReadKanjiAndMixedText(t *testing.T) {
	t.Parallel()
	phonemes, err := japanesePhonemes("東京でVoxLabを使う")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"東", "京", "V", "x", "L"} {
		for _, phoneme := range phonemes {
			if phoneme == forbidden {
				t.Fatalf("unconverted text %q leaked into phonemes: %#v", forbidden, phonemes)
			}
		}
	}
	if !containsPhonemeSequence(phonemes, []string{"t", "o", ":", "k", "ʲ", "o", ":"}) {
		t.Fatalf("東京 was not read as トーキョー: %#v", phonemes)
	}

	phonemes, err = japanesePhonemes("銀行に行きます")
	if err != nil {
		t.Fatal(err)
	}
	if !containsPhonemeSequence(phonemes, []string{"ɡ", "i", "ɴ", "k", "o", ":"}) {
		t.Fatalf("銀行 was not read as ギンコー: %#v", phonemes)
	}
}

func TestJapanesePhonemesReadNumbers(t *testing.T) {
	t.Parallel()
	phonemes, err := japanesePhonemes("2026年")
	if err != nil {
		t.Fatal(err)
	}
	for _, phoneme := range phonemes {
		for _, digit := range "0123456789" {
			if phoneme == string(digit) {
				t.Fatalf("number leaked into phonemes: %#v", phonemes)
			}
		}
	}
}

func TestJapaneseNumberReadings(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"0":       "ゼロ",
		"10":      "ジュウ",
		"100":     "ヒャク",
		"2026":    "ニセンニジュウロク",
		"1,234":   "センニヒャクサンジュウヨン",
		"3.14":    "サンテンイチヨン",
		"10000":   "イチマン",
		"1000000": "ヒャクマン",
	}
	for input, want := range tests {
		if got := japaneseNumberToKana(input); got != want {
			t.Errorf("japaneseNumberToKana(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestJapanesePhonemesRejectEmptyInput(t *testing.T) {
	t.Parallel()
	if _, err := japanesePhonemes(" \n "); err == nil {
		t.Fatal("empty Japanese input should fail")
	}
}

func TestKatakanaToPhonemes(t *testing.T) {
	t.Parallel()
	got := katakanaToPhonemes("コンニチハ")
	want := []string{"k", "o", "ɴ", "n", "i", "t", "ɕ", "i", "h", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("phonemes = %#v, want %#v", got, want)
	}

	got = katakanaToPhonemes("キャット")
	want = []string{"k", "ʲ", "a", "q", "t", "o"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contracted phonemes = %#v, want %#v", got, want)
	}
}

func containsPhonemeSequence(phonemes, sequence []string) bool {
	if len(sequence) == 0 {
		return true
	}
	for start := 0; start+len(sequence) <= len(phonemes); start++ {
		matched := true
		for index := range sequence {
			if phonemes[start+index] != sequence[index] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
