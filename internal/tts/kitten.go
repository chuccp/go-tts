package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

type KittenConfig struct {
	ModelPath  string
	VoicesPath string
	ConfigPath string
	RuntimeLib string
	EspeakPath string
}

type kittenModelConfig struct {
	SpeedPriors  map[string]float32 `json:"speed_priors"`
	VoiceAliases map[string]string  `json:"voice_aliases"`
}

type KittenEngine struct {
	cfg           KittenConfig
	initErr       error
	session       *ort.DynamicAdvancedSession
	mu            sync.Mutex
	voices        []Voice
	styles        map[string]floatTensor
	aliases       map[string]string
	internalNames map[string]string
	speedPriors   map[string]float32
	tokenizer     map[rune]int64
}

func NewKittenEngine(cfg KittenConfig) *KittenEngine {
	if cfg.EspeakPath == "" {
		cfg.EspeakPath = FindEspeakPath("")
	}
	e := &KittenEngine{
		cfg:           cfg,
		styles:        make(map[string]floatTensor),
		aliases:       make(map[string]string),
		internalNames: make(map[string]string),
		speedPriors:   make(map[string]float32),
		tokenizer:     kokoroVocabulary(),
	}
	e.initErr = e.initialize()
	return e
}

func (e *KittenEngine) initialize() error {
	if _, err := os.Stat(e.cfg.ModelPath); err != nil {
		return fmt.Errorf("Kitten model is not ready: %w", err)
	}
	if _, err := exec.LookPath(e.cfg.EspeakPath); err != nil {
		return fmt.Errorf("espeak-ng was not found; set -espeak or PATH")
	}
	tensors, err := LoadVoiceTensorFile(e.cfg.VoicesPath)
	if err != nil {
		return fmt.Errorf("load Kitten voices: %w", err)
	}
	for name, tensor := range tensors {
		if _, columns := tensorRows(tensor); columns != kokoroStyleDim {
			continue
		}
		e.styles[strings.ToLower(name)] = tensor
	}
	if len(e.styles) == 0 {
		return errors.New("Kitten voice pack contains no 256-dimensional voice profiles")
	}

	modelConfig := defaultKittenConfig()
	if e.cfg.ConfigPath != "" {
		data, err := os.ReadFile(e.cfg.ConfigPath)
		if err == nil {
			if err := json.Unmarshal(data, &modelConfig); err != nil {
				return fmt.Errorf("parse Kitten config: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read Kitten config: %w", err)
		}
	}
	for alias, internal := range modelConfig.VoiceAliases {
		internalKey := strings.ToLower(internal)
		if _, ok := e.styles[internalKey]; !ok {
			continue
		}
		e.aliases[strings.ToLower(alias)] = internalKey
		e.internalNames[internalKey] = alias
	}
	for internal := range e.styles {
		if _, ok := e.internalNames[internal]; !ok {
			e.internalNames[internal] = internal
		}
	}
	for voice, prior := range modelConfig.SpeedPriors {
		if prior > 0 {
			e.speedPriors[strings.ToLower(voice)] = prior
		}
	}

	e.voices = e.buildVoices()
	if err := ensureONNXRuntime(FindONNXRuntime(e.cfg.RuntimeLib)); err != nil {
		return err
	}
	session, err := newONNXSession(
		e.cfg.ModelPath,
		[]string{"input_ids", "style", "speed"},
		[]string{"waveform", "duration"},
	)
	if err != nil {
		return err
	}
	e.session = session
	return nil
}

func (e *KittenEngine) ID() string          { return "kitten" }
func (e *KittenEngine) Name() string        { return "Kitten TTS" }
func (e *KittenEngine) Description() string { return "本地 ONNX · 轻量快速 · 8 种音色" }
func (e *KittenEngine) Local() bool         { return true }
func (e *KittenEngine) SupportsSpeed() bool { return true }
func (e *KittenEngine) Available() error    { return e.initErr }
func (e *KittenEngine) DefaultVoice() string {
	for _, voice := range e.voices {
		if voice.ID == "Bella" {
			return voice.ID
		}
	}
	if len(e.voices) > 0 {
		return e.voices[0].ID
	}
	return ""
}
func (e *KittenEngine) Voices(context.Context) ([]Voice, error) {
	return append([]Voice(nil), e.voices...), e.initErr
}

func (e *KittenEngine) buildVoices() []Voice {
	seen := make(map[string]bool)
	voices := make([]Voice, 0, len(e.styles))
	for _, internal := range e.aliases {
		if seen[internal] {
			continue
		}
		seen[internal] = true
		voices = append(voices, Voice{
			ID:           e.internalNames[internal],
			Name:         e.internalNames[internal],
			Locale:       "en-US",
			Language:     "English",
			LanguageCode: "en",
		})
	}
	keys := make([]string, 0, len(e.styles))
	for internal := range e.styles {
		keys = append(keys, internal)
	}
	sort.Strings(keys)
	for _, internal := range keys {
		if seen[internal] {
			continue
		}
		voices = append(voices, Voice{
			ID:           internal,
			Name:         internal,
			Locale:       "en-US",
			Language:     "English",
			LanguageCode: "en",
		})
	}
	sortVoices(voices)
	return voices
}

func (e *KittenEngine) Synthesize(ctx context.Context, text, voiceID string, speed float64) (Audio, error) {
	if e.initErr != nil {
		return Audio{}, e.initErr
	}
	internal, tensor, ok := e.resolveVoice(voiceID)
	if !ok {
		return Audio{}, fmt.Errorf("unknown Kitten voice %q", voiceID)
	}
	effectiveSpeed := float32(speed)
	if prior, ok := e.speedPriors[internal]; ok {
		effectiveSpeed *= prior
	}
	chunks := SplitText(text, 400)
	if len(chunks) == 0 {
		return Audio{}, ErrEmptyText
	}
	var allSamples []float32
	for index, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return Audio{}, err
		}
		samples, err := e.synthesizeChunk(ctx, ensurePunctuation(chunk), tensor, effectiveSpeed)
		if err != nil {
			return Audio{}, fmt.Errorf("Kitten chunk %d: %w", index+1, err)
		}
		if index > 0 {
			allSamples = appendSilence(allSamples, 24000, 80)
		}
		allSamples = append(allSamples, samples...)
	}
	if len(allSamples) == 0 {
		return Audio{}, errors.New("Kitten produced empty audio")
	}
	return Audio{
		Data:       Float32ToWAV(allSamples, 24000),
		MIMEType:   "audio/wav",
		Extension:  "wav",
		SampleRate: 24000,
	}, nil
}

func (e *KittenEngine) synthesizeChunk(
	ctx context.Context,
	text string,
	style floatTensor,
	speed float32,
) ([]float32, error) {
	phonemes, err := phonemize(ctx, e.cfg.EspeakPath, "en-us", text)
	if err != nil {
		return nil, err
	}
	tokenRunes := BasicEnglishTokens(phonemes)
	tokens := []int64{0}
	for _, r := range tokenRunes {
		if token, ok := e.tokenizer[r]; ok {
			tokens = append(tokens, token)
		}
	}
	tokens = append(tokens, 10, 0)
	if len(tokens) <= 3 {
		return nil, errors.New("tokenizer produced no Kitten tokens")
	}

	rows, columns := tensorRows(style)
	styleRow := len([]rune(text))
	if styleRow >= rows {
		styleRow = rows - 1
	}
	styleData := style.Data[styleRow*columns : (styleRow+1)*columns]
	inputTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(tokens))), tokens)
	if err != nil {
		return nil, err
	}
	defer inputTensor.Destroy()
	styleTensor, err := ort.NewTensor(ort.NewShape(1, kokoroStyleDim), styleData)
	if err != nil {
		return nil, err
	}
	defer styleTensor.Destroy()
	speedTensor, err := ort.NewTensor(ort.NewShape(1), []float32{speed})
	if err != nil {
		return nil, err
	}
	defer speedTensor.Destroy()

	e.mu.Lock()
	defer e.mu.Unlock()
	outputs := make([]ort.Value, 2)
	if err := e.session.Run([]ort.Value{inputTensor, styleTensor, speedTensor}, outputs); err != nil {
		return nil, err
	}
	defer func() {
		for _, output := range outputs {
			if output != nil {
				_ = output.Destroy()
			}
		}
	}()
	samples, err := flattenTensor(outputs[0])
	if err != nil {
		return nil, err
	}
	if len(samples) > 5000 {
		samples = samples[:len(samples)-5000]
	}
	return samples, nil
}

func (e *KittenEngine) resolveVoice(voiceID string) (string, floatTensor, bool) {
	key := strings.ToLower(strings.TrimSpace(voiceID))
	if internal, ok := e.aliases[key]; ok {
		return internal, e.styles[internal], true
	}
	tensor, ok := e.styles[key]
	return key, tensor, ok
}

func (e *KittenEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	session := e.session
	e.session = nil
	return closeSession(session)
}

func defaultKittenConfig() kittenModelConfig {
	aliases := map[string]string{
		"Bella":  "expr-voice-2-f",
		"Jasper": "expr-voice-2-m",
		"Luna":   "expr-voice-3-f",
		"Bruno":  "expr-voice-3-m",
		"Rosie":  "expr-voice-4-f",
		"Hugo":   "expr-voice-4-m",
		"Kiki":   "expr-voice-5-f",
		"Leo":    "expr-voice-5-m",
	}
	priors := map[string]float32{
		"expr-voice-2-f": 0.8,
		"expr-voice-2-m": 0.8,
		"expr-voice-3-f": 0.8,
		"expr-voice-3-m": 0.8,
		"expr-voice-4-f": 0.8,
		"expr-voice-4-m": 0.9,
		"expr-voice-5-f": 0.8,
		"expr-voice-5-m": 0.8,
	}
	return kittenModelConfig{SpeedPriors: priors, VoiceAliases: aliases}
}

func ensurePunctuation(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	last := []rune(text)
	switch last[len(last)-1] {
	case '.', '!', '?', '。', '！', '？':
		return text
	default:
		return text + "."
	}
}
