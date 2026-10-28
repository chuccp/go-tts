package ttssvc

import (
	"context"
	"testing"

	"github.com/chuccp/go-web-frame/config"
	framecore "github.com/chuccp/go-web-frame/core"
)

func TestServiceInitFromFrameworkConfig(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Put("tts.max_chars", 1234)
	cfg.Put("tts.edge.default_voice", "en-US-AvaMultilingualNeural")
	cfg.Put("tts.kokoro.model", "missing/kokoro.onnx")
	cfg.Put("tts.kokoro.voices", "missing/kokoro.npz")
	cfg.Put("tts.kitten.model", "missing/kitten.onnx")
	cfg.Put("tts.kitten.voices", "missing/voices.npz")

	service := New()
	if err := service.Init(framecore.NewContext(cfg, context.Background())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = service.Close()
	})
	if got := service.MaxTextRunes(); got != 1234 {
		t.Fatalf("max text runes = %d, want 1234", got)
	}
	if got := service.DefaultVoice("edge"); got != "en-US-AvaMultilingualNeural" {
		t.Fatalf("default Edge voice = %q", got)
	}
	statuses := service.Statuses(context.Background())
	if len(statuses) != 3 {
		t.Fatalf("engine count = %d", len(statuses))
	}
	wantOrder := []string{"kokoro", "kitten", "edge"}
	for i, want := range wantOrder {
		if statuses[i].ID != want {
			t.Fatalf("engine order[%d] = %q, want %q", i, statuses[i].ID, want)
		}
	}
	if !statuses[2].Available {
		t.Fatalf("Edge should be available: %+v", statuses[2])
	}
}

func TestStringSettingFallback(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	if got := stringSetting(cfg, "missing", "fallback"); got != "fallback" {
		t.Fatalf("fallback = %q", got)
	}
	cfg.Put("custom.path", "configured")
	if got := stringSetting(cfg, "custom.path", "fallback"); got != "configured" {
		t.Fatalf("configured value = %q", got)
	}
}
