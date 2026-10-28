package ttssvc

import (
	"context"
	"errors"
	"strings"

	"github.com/cao/go-tts/internal/tts"
	framecfg "github.com/chuccp/go-web-frame/config"
	"github.com/chuccp/go-web-frame/core"
)

// Service is the Go Web Frame service that owns all TTS engines.
type Service struct {
	core.IService
	service *tts.Service
}

func New() *Service {
	return &Service{}
}

// NewWithService injects an already constructed service, primarily for tests.
func NewWithService(service *tts.Service) *Service {
	return &Service{service: service}
}

func (s *Service) Init(ctx *core.Context) error {
	if s.service != nil {
		return nil
	}
	cfg := ctx.GetConfig()
	maxChars := cfg.GetInt("tts.max_chars")
	if maxChars <= 0 {
		maxChars = 5000
	}
	runtimeLib := cfg.GetString("tts.onnx.runtime_lib")
	kokoro := tts.NewKokoroEngine(tts.KokoroConfig{
		ModelPath:  stringSetting(cfg, "tts.kokoro.model", "models/kokoro/kokoro-v0_19.onnx"),
		VoicesPath: stringSetting(cfg, "tts.kokoro.voices", "models/kokoro/voices-v1.0.bin"),
		RuntimeLib: runtimeLib,
	})
	kitten := tts.NewKittenEngine(tts.KittenConfig{
		ModelPath:  stringSetting(cfg, "tts.kitten.model", "models/kitten/kitten_tts_nano_v0_8.onnx"),
		VoicesPath: stringSetting(cfg, "tts.kitten.voices", "models/kitten/voices.npz"),
		ConfigPath: stringSetting(cfg, "tts.kitten.config", "models/kitten/config.json"),
		RuntimeLib: runtimeLib,
	})
	edge := tts.NewEdgeEngine(tts.EdgeConfig{
		DefaultVoice: stringSetting(cfg, "tts.edge.default_voice", "zh-CN-XiaoxiaoNeural"),
		ProxyURL:     cfg.GetString("tts.edge.proxy"),
	})
	s.service = tts.NewService(kokoro, kitten, edge)
	s.service.SetMaxTextRunes(maxChars)
	return nil
}

func (s *Service) Close() error {
	if s.service == nil {
		return nil
	}
	return s.service.Close()
}

func (s *Service) Statuses(ctx context.Context) []tts.EngineStatus {
	if s.service == nil {
		return nil
	}
	return s.service.Statuses(ctx)
}

func (s *Service) Voices(ctx context.Context, engineID string) ([]tts.Voice, error) {
	if s.service == nil {
		return nil, errors.New("TTS service is not initialized")
	}
	return s.service.Voices(ctx, engineID)
}

func (s *Service) Synthesize(ctx context.Context, engineID, voice, text string, speed float64) (tts.Audio, error) {
	if s.service == nil {
		return tts.Audio{}, errors.New("TTS service is not initialized")
	}
	return s.service.Synthesize(ctx, engineID, voice, text, speed)
}

func (s *Service) DefaultVoice(engineID string) string {
	if s.service == nil {
		return ""
	}
	return s.service.DefaultVoice(engineID)
}

func (s *Service) MaxTextRunes() int {
	if s.service == nil {
		return 5000
	}
	return s.service.MaxTextRunes()
}

func stringSetting(cfg framecfg.IConfig, key, fallback string) string {
	if value := cfg.GetString(key); strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
