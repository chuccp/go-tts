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
	if got := cfg.GetString("tts.kokoro.model"); got != "models/kokoro/kokoro-v1.0-quantized.onnx" {
		t.Fatalf("tts.kokoro.model = %q, want the quality-tested v1.0 quantized model", got)
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
