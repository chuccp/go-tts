package tts

import (
	"os"
	"path/filepath"
	"testing"
)

func loadMeloTestEngine(t *testing.T) *MeloEngine {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tokensPath := filepath.Join(root, "models", "melo-zh", "tokens.txt")
	lexiconPath := filepath.Join(root, "models", "melo-zh", "lexicon.txt")
	if _, err := os.Stat(tokensPath); err != nil {
		t.Skipf("Melo tokens are missing: %s", tokensPath)
	}
	if _, err := os.Stat(lexiconPath); err != nil {
		t.Skipf("Melo lexicon is missing: %s", lexiconPath)
	}
	tokens, err := loadMeloTokens(tokensPath)
	if err != nil {
		t.Fatal(err)
	}
	lexicon, maxWord, err := loadMeloLexicon(lexiconPath, tokens)
	if err != nil {
		t.Fatal(err)
	}
	return &MeloEngine{tokens: tokens, lexicon: lexicon, maxWord: maxWord}
}

func TestMeloLexiconUsesLongestChineseWord(t *testing.T) {
	engine := loadMeloTestEngine(t)
	phones, tones := engine.textToTokens("银行行长")
	if len(phones) == 0 || len(phones) != len(tones) {
		t.Fatalf("invalid Melo token output: phones=%d tones=%d", len(phones), len(tones))
	}

	wantPhones := append([]int64{}, engine.lexicon["银行"].phones...)
	wantTones := append([]int64{}, engine.lexicon["银行"].tones...)
	for _, syllable := range []string{"hang2", "zhang3"} {
		p, t := meloPinyinToPhones(syllable, engine.tokens)
		wantPhones = append(wantPhones, p...)
		wantTones = append(wantTones, t...)
	}
	if len(phones) != len(wantPhones) {
		t.Fatalf("token count = %d, want %d", len(phones), len(wantPhones))
	}
	for i := range phones {
		if phones[i] != wantPhones[i] || tones[i] != wantTones[i] {
			t.Fatalf("Melo token[%d] = (%d,%d), want (%d,%d)", i, phones[i], tones[i], wantPhones[i], wantTones[i])
		}
	}
}

func TestMeloTokenizerHandlesChinesePunctuation(t *testing.T) {
	engine := loadMeloTestEngine(t)
	phones, tones := engine.textToTokens("你好，世界。")
	if len(phones) == 0 || len(phones) != len(tones) {
		t.Fatalf("invalid Melo punctuation output: phones=%d tones=%d", len(phones), len(tones))
	}
}
