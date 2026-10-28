package tts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKittenDefersONNXSessionUntilFirstSynthesis(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "models", "kitten", "kitten_tts_nano_v0_8.onnx")
	voicesPath := filepath.Join(root, "models", "kitten", "voices.npz")
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("Kitten model is missing: %s", modelPath)
	}
	if _, err := os.Stat(voicesPath); err != nil {
		t.Skipf("Kitten voices are missing: %s", voicesPath)
	}
	engine := NewKittenEngine(KittenConfig{
		ModelPath:  modelPath,
		VoicesPath: voicesPath,
		ConfigPath: filepath.Join(root, "models", "kitten", "config.json"),
	})
	if err := engine.Available(); err != nil {
		t.Fatal(err)
	}
	if engine.session != nil {
		t.Fatal("Kitten opened its ONNX session during inventory initialization")
	}
}
