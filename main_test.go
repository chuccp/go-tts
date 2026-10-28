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
	if got := cfg.GetString("tts.kokoro.model"); got == "" {
		t.Fatal("tts.kokoro.model is empty")
	}
	if !cfg.GetBoolOrDefault("tts.captcha.enabled", false) {
		t.Fatal("tts.captcha.enabled should default to true")
	}
	if got := cfg.GetInt("tts.captcha.limit"); got != 10 {
		t.Fatalf("tts.captcha.limit = %d, want 10", got)
	}
	if got := cfg.GetString("tts.captcha.window"); got != "10m" {
		t.Fatalf("tts.captcha.window = %q, want 10m", got)
	}
}
