package tts

import (
	"context"
	"testing"
)

func TestChinesePhonemesUseKokoroInventory(t *testing.T) {
	phonemes, err := chinesePhonemes(context.Background(), "你好呀")
	if err != nil {
		t.Fatal(err)
	}
	if phonemes != "ni↓xau↓ ja" {
		t.Fatalf("phonemes = %q, want %q", phonemes, "ni↓xau↓ ja")
	}
}

func TestChinesePhonemesResolvePolyphonicWords(t *testing.T) {
	tests := map[string]string{
		"银行行长": "i↗nxa↗ŋ xa↗ŋʈʂa↓ŋ",
		"行走":   "ɕi↗ŋtsou↓",
		"重庆":   "ʈʂʰʊ↗ŋtɕʰi↘ŋ",
		"重复":   "ʈʂʰʊ↗ŋfu↘",
		"长大":   "ʈʂa↓ŋta↘",
		"长度":   "ʈʂʰa↗ŋtu↘",
		"音乐":   "i→nɥe↘",
		"快乐":   "kʰwai↘lɤ↘",
	}
	for text, want := range tests {
		got, err := chinesePhonemes(context.Background(), text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if got != want {
			t.Errorf("%s phonemes = %q, want %q", text, got, want)
		}
	}
}

func TestChinesePhonemesNormalizePunctuation(t *testing.T) {
	got, err := chinesePhonemes(context.Background(), "你好，世界。")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ni↓xau↓, ʂɹ̩↘tɕje↘." {
		t.Fatalf("phonemes = %q", got)
	}
}

func TestChinesePhonemesUseKnownKokoroTokens(t *testing.T) {
	vocabulary := kokoroVocabulary()
	for _, text := range []string{
		"你好，世界。",
		"银行行长",
		"重庆重复",
		"长大长度",
		"音乐快乐",
	} {
		phonemes, err := chinesePhonemes(context.Background(), text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		for _, r := range phonemes {
			if r == ' ' {
				continue
			}
			if _, ok := vocabulary[r]; !ok {
				t.Fatalf("%s produced unsupported Kokoro rune %q in %q", text, r, phonemes)
			}
		}
	}
}
