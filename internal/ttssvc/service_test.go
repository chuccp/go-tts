package ttssvc

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/chuccp/go-web-frame/config"
	framecore "github.com/chuccp/go-web-frame/core"
)

func TestServiceInitFromFrameworkConfig(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Put("tts.max_chars", 1234)
	cfg.Put("tts.engine_order", []string{"piper-ja", "melo", "kokoro", "kitten", "edge"})
	cfg.Put("tts.edge.default_voice", "en-US-AvaMultilingualNeural")
	cfg.Put("tts.kokoro.model", "missing/kokoro.onnx")
	cfg.Put("tts.kokoro.voices", "missing/kokoro.npz")
	cfg.Put("tts.kitten.model", "missing/kitten.onnx")
	cfg.Put("tts.kitten.voices", "missing/voices.npz")
	cfg.Put("tts.melo.model", "missing/melo.onnx")
	cfg.Put("tts.melo.lexicon", "missing/lexicon.txt")
	cfg.Put("tts.melo.tokens", "missing/tokens.txt")
	cfg.Put("tts.piper.japanese.model", "missing/ja.onnx")
	cfg.Put("tts.piper.japanese.config", "missing/ja.onnx.json")

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
	if len(statuses) != 5 {
		t.Fatalf("engine count = %d", len(statuses))
	}
	wantOrder := []string{"piper-ja", "melo", "kokoro", "kitten", "edge"}
	for i, want := range wantOrder {
		if statuses[i].ID != want {
			t.Fatalf("engine order[%d] = %q, want %q", i, statuses[i].ID, want)
		}
	}
	if !statuses[4].Available {
		t.Fatalf("Edge should be available: %+v", statuses[4])
	}
}

func TestOptionalEnginesAreDisabledWhenNotConfigured(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Put("tts.engine_order", []string{"melo", "edge", "kitten"})
	cfg.Put("tts.kitten.model", "missing/kitten.onnx")
	cfg.Put("tts.kitten.voices", "missing/voices.npz")
	cfg.Put("tts.melo.model", "missing/melo.onnx")
	cfg.Put("tts.melo.lexicon", "missing/lexicon.txt")
	cfg.Put("tts.melo.tokens", "missing/tokens.txt")
	service := New()
	if err := service.Init(framecore.NewContext(cfg, context.Background())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	statuses := service.Statuses(context.Background())
	wantOrder := []string{"melo", "edge", "kitten"}
	if len(statuses) != len(wantOrder) {
		t.Fatalf("engine count = %d, want %d", len(statuses), len(wantOrder))
	}
	for i, want := range wantOrder {
		if statuses[i].ID != want {
			t.Fatalf("engine order[%d] = %q, want %q", i, statuses[i].ID, want)
		}
	}
}

func TestEngineOrderRejectsUnknownEngine(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Put("tts.engine_order", []string{"edge", "does-not-exist"})
	service := New()
	err := service.Init(framecore.NewContext(cfg, context.Background()))
	if err == nil || !strings.Contains(err.Error(), "unknown engine") {
		t.Fatalf("Init error = %v, want unknown engine", err)
	}
}

func TestEngineOrderIsReadFromConfig(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Put("tts.engine_order", []string{"kitten", "edge"})
	service := New()
	if err := service.Init(framecore.NewContext(cfg, context.Background())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

	statuses := service.Statuses(context.Background())
	got := make([]string, 0, len(statuses))
	for _, status := range statuses {
		got = append(got, status.ID)
	}
	if want := []string{"kitten", "edge"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("engine order = %v, want %v", got, want)
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
