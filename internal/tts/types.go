package tts

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MinSpeed = 0.5
	MaxSpeed = 2.0
)

var (
	ErrEmptyText   = errors.New("text is required")
	ErrTextTooLong = errors.New("text is too long")
	ErrBadSpeed    = fmt.Errorf("speed must be between %.1f and %.1f", MinSpeed, MaxSpeed)
)

type Voice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Locale       string `json:"locale,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Language     string `json:"language,omitempty"`
	LanguageCode string `json:"languageCode,omitempty"`
}

type Audio struct {
	Data       []byte
	MIMEType   string
	Extension  string
	SampleRate int
}

type Engine interface {
	ID() string
	Name() string
	Description() string
	Local() bool
	SupportsSpeed() bool
	Available() error
	Voices(context.Context) ([]Voice, error)
	Synthesize(context.Context, string, string, float64) (Audio, error)
	Close() error
}

type EngineStatus struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Local         bool    `json:"local"`
	Available     bool    `json:"available"`
	SupportsSpeed bool    `json:"supportsSpeed"`
	DefaultVoice  string  `json:"defaultVoice,omitempty"`
	Error         string  `json:"error,omitempty"`
	Voices        []Voice `json:"voices"`
}

type Service struct {
	engines      map[string]Engine
	order        []string
	maxTextRunes int
}

type voicePreviewer interface {
	PreviewVoices() []Voice
}

type defaultVoiceProvider interface {
	DefaultVoice() string
}

func NewService(engines ...Engine) *Service {
	s := &Service{
		engines:      make(map[string]Engine, len(engines)),
		maxTextRunes: 5000,
	}
	for _, engine := range engines {
		if engine == nil {
			continue
		}
		id := engine.ID()
		if _, exists := s.engines[id]; exists {
			continue
		}
		s.engines[id] = engine
		s.order = append(s.order, id)
	}
	return s
}

func (s *Service) SetMaxTextRunes(limit int) {
	if limit > 0 {
		s.maxTextRunes = limit
	}
}

func (s *Service) MaxTextRunes() int {
	return s.maxTextRunes
}

func (s *Service) DefaultVoice(engineID string) string {
	engine, ok := s.engines[strings.ToLower(strings.TrimSpace(engineID))]
	if !ok {
		return ""
	}
	if provider, ok := engine.(defaultVoiceProvider); ok {
		return provider.DefaultVoice()
	}
	return ""
}

func (s *Service) Statuses(ctx context.Context) []EngineStatus {
	statuses := make([]EngineStatus, 0, len(s.order))
	for _, id := range s.order {
		engine := s.engines[id]
		status := EngineStatus{
			ID:            engine.ID(),
			Name:          engine.Name(),
			Description:   engine.Description(),
			Local:         engine.Local(),
			SupportsSpeed: engine.SupportsSpeed(),
		}
		if err := engine.Available(); err != nil {
			status.Error = err.Error()
		} else {
			status.Available = true
		}
		if provider, ok := engine.(defaultVoiceProvider); ok {
			status.DefaultVoice = provider.DefaultVoice()
		}
		var voices []Voice
		if previewer, ok := engine.(voicePreviewer); ok {
			voices = previewer.PreviewVoices()
		} else {
			voices, _ = engine.Voices(ctx)
		}
		status.Voices = voices
		statuses = append(statuses, status)
	}
	return statuses
}

func (s *Service) Voices(ctx context.Context, engineID string) ([]Voice, error) {
	engine, err := s.engine(engineID)
	if err != nil {
		return nil, err
	}
	if err := engine.Available(); err != nil {
		return nil, err
	}
	voices, err := engine.Voices(ctx)
	if err != nil {
		return voices, fmt.Errorf("load voices: %w", err)
	}
	return voices, nil
}

func (s *Service) Synthesize(ctx context.Context, engineID, voice, text string, speed float64) (Audio, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Audio{}, ErrEmptyText
	}
	if len([]rune(text)) > s.maxTextRunes {
		return Audio{}, fmt.Errorf("%w: maximum is %d characters", ErrTextTooLong, s.maxTextRunes)
	}
	if speed < MinSpeed || speed > MaxSpeed {
		return Audio{}, ErrBadSpeed
	}
	engine, err := s.engine(engineID)
	if err != nil {
		return Audio{}, err
	}
	if err := engine.Available(); err != nil {
		return Audio{}, err
	}
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voices, _ := engine.Voices(ctx)
		if len(voices) == 0 {
			return Audio{}, errors.New("voice is required")
		}
		voice = voices[0].ID
	}
	if err := validateVoice(engine, ctx, voice); err != nil {
		return Audio{}, err
	}
	return engine.Synthesize(ctx, text, voice, speed)
}

func (s *Service) Close() error {
	var errs []error
	for i := len(s.order) - 1; i >= 0; i-- {
		if err := s.engines[s.order[i]].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) engine(engineID string) (Engine, error) {
	engineID = strings.ToLower(strings.TrimSpace(engineID))
	engine, ok := s.engines[engineID]
	if !ok {
		return nil, fmt.Errorf("unknown engine %q", engineID)
	}
	return engine, nil
}

func validateVoice(engine Engine, ctx context.Context, voiceID string) error {
	voices, err := engine.Voices(ctx)
	if err != nil {
		// Edge TTS may still synthesize with a previously selected voice when
		// only the voice-list endpoint is temporarily unavailable.
		return nil
	}
	for _, voice := range voices {
		if strings.EqualFold(voice.ID, voiceID) {
			return nil
		}
	}
	return fmt.Errorf("voice %q is not available for %s", voiceID, engine.Name())
}

func sortVoices(voices []Voice) {
	sort.SliceStable(voices, func(i, j int) bool {
		if voices[i].Locale != voices[j].Locale {
			return voices[i].Locale < voices[j].Locale
		}
		return voices[i].Name < voices[j].Name
	})
}
