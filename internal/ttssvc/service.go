package ttssvc

import (
	"context"
	"errors"
	"fmt"
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
	engines, err := configuredEngines(cfg, runtimeLib)
	if err != nil {
		return err
	}
	s.service = tts.NewService(engines...)
	s.service.SetMaxTextRunes(maxChars)
	return nil
}

func configuredEngines(cfg framecfg.IConfig, runtimeLib string) ([]tts.Engine, error) {
	var order []string
	if err := cfg.UnmarshalKey("tts.engine_order", &order); err != nil {
		return nil, fmt.Errorf("read tts.engine_order: %w", err)
	}
	if len(order) == 0 {
		return nil, errors.New("tts.engine_order must contain at least one engine")
	}

	builders := map[string]func() tts.Engine{
		"edge": func() tts.Engine {
			return tts.NewEdgeEngine(tts.EdgeConfig{
				DefaultVoice: stringSetting(cfg, "tts.edge.default_voice", "zh-CN-XiaoxiaoNeural"),
				ProxyURL:     cfg.GetString("tts.edge.proxy"),
			})
		},
		"kitten": func() tts.Engine {
			return tts.NewKittenEngine(tts.KittenConfig{
				ModelPath:  stringSetting(cfg, "tts.kitten.model", "models/kitten/kitten_tts_nano_v0_8.onnx"),
				VoicesPath: stringSetting(cfg, "tts.kitten.voices", "models/kitten/voices.npz"),
				ConfigPath: stringSetting(cfg, "tts.kitten.config", "models/kitten/config.json"),
				RuntimeLib: runtimeLib,
			})
		},
		"kokoro": func() tts.Engine {
			if !cfg.HasKey("tts.kokoro.model") && !cfg.HasKey("tts.kokoro.voices") {
				return nil
			}
			return tts.NewKokoroEngine(tts.KokoroConfig{
				ModelPath:  stringSetting(cfg, "tts.kokoro.model", "models/kokoro/kokoro-v1.0-quantized.onnx"),
				VoicesPath: stringSetting(cfg, "tts.kokoro.voices", "models/kokoro/voices-v1.0.bin"),
				RuntimeLib: runtimeLib,
			})
		},
		"melo": func() tts.Engine {
			return tts.NewMeloEngine(tts.MeloConfig{
				ModelPath:   stringSetting(cfg, "tts.melo.model", "models/melo-zh/model.onnx"),
				LexiconPath: stringSetting(cfg, "tts.melo.lexicon", "models/melo-zh/lexicon.txt"),
				TokensPath:  stringSetting(cfg, "tts.melo.tokens", "models/melo-zh/tokens.txt"),
				RuntimeLib:  runtimeLib,
			})
		},
		"piper-ja": func() tts.Engine {
			if !cfg.HasKey("tts.piper.japanese.model") && !cfg.HasKey("tts.piper.japanese.config") {
				return nil
			}
			return tts.NewPiperJapaneseEngine(tts.PiperJapaneseConfig{
				ModelPath:  stringSetting(cfg, "tts.piper.japanese.model", "models/piper-ja/model.onnx"),
				ConfigPath: stringSetting(cfg, "tts.piper.japanese.config", "models/piper-ja/model.onnx.json"),
				RuntimeLib: runtimeLib,
			})
		},
	}

	engines := make([]tts.Engine, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, rawID := range order {
		id := strings.ToLower(strings.TrimSpace(rawID))
		if id == "" {
			return nil, errors.New("tts.engine_order contains an empty engine ID")
		}
		if seen[id] {
			return nil, fmt.Errorf("tts.engine_order contains duplicate engine %q", id)
		}
		build, ok := builders[id]
		if !ok {
			return nil, fmt.Errorf("tts.engine_order contains unknown engine %q", id)
		}
		seen[id] = true
		if engine := build(); engine != nil {
			engines = append(engines, engine)
		}
	}
	return engines, nil
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
