package tts

import (
	"context"
	"errors"
	"testing"
)

type stubEngine struct {
	available     bool
	voices        []Voice
	voicesErr     error
	synthesizeErr error
	gotText       string
	gotVoice      string
	gotSpeed      float64
}

func (e *stubEngine) ID() string           { return "stub" }
func (e *stubEngine) Name() string         { return "Stub" }
func (e *stubEngine) Description() string  { return "stub engine" }
func (e *stubEngine) Local() bool          { return true }
func (e *stubEngine) SupportsSpeed() bool  { return true }
func (e *stubEngine) Close() error         { return nil }
func (e *stubEngine) DefaultVoice() string { return "default" }
func (e *stubEngine) Available() error {
	if e.available {
		return nil
	}
	return errors.New("unavailable")
}
func (e *stubEngine) Voices(context.Context) ([]Voice, error) {
	return e.voices, e.voicesErr
}
func (e *stubEngine) Synthesize(_ context.Context, text, voice string, speed float64) (Audio, error) {
	e.gotText = text
	e.gotVoice = voice
	e.gotSpeed = speed
	if e.synthesizeErr != nil {
		return Audio{}, e.synthesizeErr
	}
	return Audio{Data: []byte("audio"), MIMEType: "audio/wav", Extension: "wav"}, nil
}

type rangedStubEngine struct {
	stubEngine
}

func (e *rangedStubEngine) SpeedRange() (float64, float64) { return 0.25, 1.5 }

func TestEngineSpeedRanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		engine Engine
		min    float64
	}{
		{"edge", &EdgeEngine{}, MinSpeed},
		{"kitten", &KittenEngine{}, MinLocalSpeed},
		{"kokoro", &KokoroEngine{}, MinLocalSpeed},
		{"melo", &MeloEngine{}, MinLocalSpeed},
		{"piper-ja", &PiperJapaneseEngine{}, MinLocalSpeed},
	}
	for _, test := range tests {
		min, max := speedRange(test.engine)
		if min != test.min || max != MaxSpeed {
			t.Errorf("%s speed range = %g-%g, want %g-%g", test.name, min, max, test.min, MaxSpeed)
		}
	}
}

func TestServiceUsesEngineSpeedRange(t *testing.T) {
	t.Parallel()
	engine := &rangedStubEngine{stubEngine{available: true, voices: []Voice{{ID: "default"}}}}
	service := NewService(engine)
	if _, err := service.Synthesize(context.Background(), "stub", "default", "hello", 0.25); err != nil {
		t.Fatalf("range minimum should be accepted: %v", err)
	}
	if _, err := service.Synthesize(context.Background(), "stub", "default", "hello", 0.24); !errors.Is(err, ErrBadSpeed) {
		t.Fatalf("below-range speed error = %v", err)
	}
	if _, err := service.Synthesize(context.Background(), "stub", "default", "hello", 1.51); !errors.Is(err, ErrBadSpeed) {
		t.Fatalf("above-range speed error = %v", err)
	}
}

func TestServiceSynthesisValidationAndFallbackVoice(t *testing.T) {
	t.Parallel()
	engine := &stubEngine{
		available: true,
		voices:    []Voice{{ID: "default"}},
		voicesErr: errors.New("temporary voice-list failure"),
	}
	service := NewService(engine)

	audio, err := service.Synthesize(context.Background(), " STUB ", "", "  hello  ", 1.25)
	if err != nil {
		t.Fatal(err)
	}
	if string(audio.Data) != "audio" {
		t.Fatalf("audio = %q", audio.Data)
	}
	if engine.gotText != "hello" || engine.gotVoice != "default" || engine.gotSpeed != 1.25 {
		t.Fatalf("unexpected synthesis arguments: text=%q voice=%q speed=%v", engine.gotText, engine.gotVoice, engine.gotSpeed)
	}

	if _, err := service.Synthesize(context.Background(), "stub", "default", "", 1); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("empty text error = %v", err)
	}
	if _, err := service.Synthesize(context.Background(), "stub", "default", "hello", 0.49); !errors.Is(err, ErrBadSpeed) {
		t.Fatalf("low speed error = %v", err)
	}
	if _, err := service.Synthesize(context.Background(), "stub", "default", "hello", 2.01); !errors.Is(err, ErrBadSpeed) {
		t.Fatalf("high speed error = %v", err)
	}
	if _, err := service.Synthesize(context.Background(), "missing", "default", "hello", 1); err == nil {
		t.Fatal("unknown engine should fail")
	}
	strictEngine := &stubEngine{available: true, voices: []Voice{{ID: "default"}}}
	if _, err := NewService(strictEngine).Synthesize(context.Background(), "stub", "missing", "hello", 1); err == nil {
		t.Fatal("unknown voice should fail")
	}
}

func TestServiceCharacterLimitAndAvailability(t *testing.T) {
	t.Parallel()
	engine := &stubEngine{available: true, voices: []Voice{{ID: "default"}}}
	service := NewService(engine)
	service.SetMaxTextRunes(2)
	if _, err := service.Synthesize(context.Background(), "stub", "default", "你好", 1); err != nil {
		t.Fatalf("two runes should be accepted: %v", err)
	}
	if _, err := service.Synthesize(context.Background(), "stub", "default", "你好吗", 1); !errors.Is(err, ErrTextTooLong) {
		t.Fatalf("three runes should exceed limit: %v", err)
	}

	engine.available = false
	if _, err := service.Synthesize(context.Background(), "stub", "default", "你好", 1); err == nil || err.Error() != "unavailable" {
		t.Fatalf("unavailable engine error = %v", err)
	}
}

func TestServiceStatusIncludesDefaultVoice(t *testing.T) {
	t.Parallel()
	engine := &stubEngine{available: true, voices: []Voice{{ID: "default"}}}
	statuses := NewService(engine).Statuses(context.Background())
	if len(statuses) != 1 {
		t.Fatalf("status count = %d", len(statuses))
	}
	if !statuses[0].Available || statuses[0].DefaultVoice != "default" {
		t.Fatalf("unexpected status: %+v", statuses[0])
	}
}
