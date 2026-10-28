package tts

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPiperJapaneseReportsMissingAssets(t *testing.T) {
	t.Parallel()
	engine := NewPiperJapaneseEngine(PiperJapaneseConfig{
		ModelPath:  filepath.Join(t.TempDir(), "missing.onnx"),
		ConfigPath: filepath.Join(t.TempDir(), "missing.onnx.json"),
	})
	if err := engine.Available(); err == nil {
		t.Fatal("missing model should make the engine unavailable")
	}
	voices, _ := engine.Voices(context.Background())
	if len(voices) != 0 {
		t.Fatalf("unavailable engine returned voices: %#v", voices)
	}
}

func TestPiperJapaneseBuildsJapaneseVoices(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.onnx")
	configPath := filepath.Join(dir, "model.onnx.json")
	if err := os.WriteFile(modelPath, []byte("not loaded during inventory"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `{
		"phoneme_type":"multilingual",
		"audio":{"sample_rate":22050},
		"phoneme_id_map":{"_":[0],"a":[14],"k":[23]},
		"speaker_id_map":{"female":0,"male":1},
		"default_speaker_id":0
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	engine := NewPiperJapaneseEngine(PiperJapaneseConfig{
		ModelPath:  modelPath,
		ConfigPath: configPath,
	})
	if err := engine.Available(); err != nil {
		t.Fatal(err)
	}
	voices, err := engine.Voices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) != 2 || voices[0].LanguageCode != "ja" || engine.DefaultVoice() != "female" {
		t.Fatalf("unexpected Japanese engine inventory: default=%q voices=%#v", engine.DefaultVoice(), voices)
	}
	if engine.lengthScale != 1.5 {
		t.Fatalf("natural length scale = %v, want 1.5", engine.lengthScale)
	}
}

func TestPiperJapaneseNormalizesPhonemesForModelInventory(t *testing.T) {
	t.Parallel()
	inventory := map[string][]int64{
		"N": {1}, "t": {2}, "s": {3}, "u": {4}, "f": {5},
		"j": {6}, "y": {7}, "i": {8}, "ː": {9}, "r": {10},
	}
	got := normalizePiperPhonemes(
		[]string{"ɴ", "ʦ", "u", "ɸ", "a", "ʑ", "i", "k", "ʲ", "a", ":", "ɾ"},
		inventory,
	)
	want := []string{"N", "t", "s", "u", "f", "j", "i", "y", "ː", "r"}
	if len(got) != len(want) {
		t.Fatalf("phonemes = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("phoneme[%d] = %q, want %q (all: %#v)", index, got[index], want[index], got)
		}
	}
}

func TestPiperJapaneseAddsBosPadAndQuestionEos(t *testing.T) {
	t.Parallel()
	inventory := map[string][]int64{
		"_": {0}, "^": {1}, "$": {2}, "?": {3},
		"k": {10}, "o": {11}, "N": {12},
	}
	ids, err := buildPiperJapaneseIDs([]string{"k", "o", "N", "?"}, inventory)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{1, 0, 10, 0, 11, 0, 12, 0, 3}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %#v, want %#v", ids, want)
	}
}
