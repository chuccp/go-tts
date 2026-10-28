package main

import (
	"testing"

	"github.com/chuccp/go-web-frame/config"
)

func TestDefaultApplicationYAML(t *testing.T) {
	t.Parallel()
	cfg, err := config.LoadSingleFileConfig("application.yml")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetInt("web.server.port"); got != 8080 {
		t.Fatalf("web.server.port = %d, want 8080", got)
	}
	if got := cfg.GetInt("tts.max_chars"); got != 5000 {
		t.Fatalf("tts.max_chars = %d, want 5000", got)
	}
	if cfg.HasKey("tts.kokoro.model") {
		t.Fatal("tts.kokoro should be omitted from the default configuration")
	}
	if got := cfg.GetString("tts.melo.model"); got != "models/melo-zh/model.onnx" {
		t.Fatalf("tts.melo.model = %q", got)
	}
	if cfg.HasKey("tts.piper.japanese.model") {
		t.Fatal("tts.piper.japanese should be omitted from the default configuration")
	}
	if !cfg.GetBoolOrDefault("captcha.enabled", false) {
		t.Fatal("captcha.enabled should default to true")
	}
	if got := cfg.GetInt("captcha.limit"); got != 10 {
		t.Fatalf("captcha.limit = %d, want 10", got)
	}
	if got := cfg.GetInt("captcha.period"); got != 600 {
		t.Fatalf("captcha.period = %d, want 600", got)
	}
}
