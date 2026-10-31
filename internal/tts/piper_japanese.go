package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

type PiperJapaneseConfig struct {
	ModelPath  string
	ConfigPath string
	RuntimeLib string
}

type piperJapaneseModelConfig struct {
	PhonemeType string `json:"phoneme_type"`
	Audio       struct {
		SampleRate int `json:"sample_rate"`
	} `json:"audio"`
	Inference struct {
		NoiseScale  float32 `json:"noise_scale"`
		LengthScale float32 `json:"length_scale"`
		NoiseW      float32 `json:"noise_w"`
	} `json:"inference"`
	PhonemeIDMap     map[string][]int64 `json:"phoneme_id_map"`
	SpeakerIDMap     map[string]int64   `json:"speaker_id_map"`
	DefaultSpeakerID int64              `json:"default_speaker_id"`
	LanguageIDMap    map[string]int64   `json:"language_id_map"`
	ProsodyIDMap     map[string][]int64 `json:"prosody_id_map"`
}

type piperJapaneseSpeaker struct {
	ID     string
	Name   string
	Gender string
	SID    int64
}

type PiperJapaneseEngine struct {
	cfg      PiperJapaneseConfig
	initErr  error
	model    piperJapaneseModelConfig
	voices   []Voice
	speakers map[string]piperJapaneseSpeaker

	loadOnce    sync.Once
	loadErr     error
	mu          sync.Mutex
	session     *ort.DynamicAdvancedSession
	inputs      []ort.InputOutputInfo
	output      string
	language    int64
	lengthScale float64
}

func NewPiperJapaneseEngine(cfg PiperJapaneseConfig) *PiperJapaneseEngine {
	engine := &PiperJapaneseEngine{
		cfg:      cfg,
		speakers: make(map[string]piperJapaneseSpeaker),
	}
	engine.initErr = engine.initialize()
	return engine
}

func (e *PiperJapaneseEngine) initialize() error {
	for _, path := range []string{e.cfg.ModelPath, e.cfg.ConfigPath} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("Piper Japanese asset is not ready: %w", err)
		}
	}
	data, err := os.ReadFile(e.cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("read Piper Japanese config: %w", err)
	}
	if err := json.Unmarshal(data, &e.model); err != nil {
		return fmt.Errorf("parse Piper Japanese config: %w", err)
	}
	if len(e.model.PhonemeIDMap) == 0 {
		return errors.New("Piper Japanese config has no phoneme_id_map")
	}
	if e.model.Audio.SampleRate <= 0 {
		e.model.Audio.SampleRate = 22050
	}
	if e.model.Inference.NoiseScale <= 0 {
		e.model.Inference.NoiseScale = 0.667
	}
	if e.model.Inference.NoiseW <= 0 {
		e.model.Inference.NoiseW = 0.8
	}
	e.lengthScale = float64(e.model.Inference.LengthScale)
	if e.lengthScale < 1.5 && e.model.PhonemeType == "multilingual" {
		// The Piper Plus multilingual models speak too quickly at 1.0.
		e.lengthScale = 1.5
	}
	if e.lengthScale <= 0 {
		e.lengthScale = 1
	}

	ids := make([]string, 0, len(e.model.SpeakerIDMap))
	for name := range e.model.SpeakerIDMap {
		ids = append(ids, name)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		e.speakers["default"] = piperJapaneseSpeaker{
			ID:     "default",
			Name:   "日语女声",
			Gender: "female",
			SID:    e.model.DefaultSpeakerID,
		}
	} else {
		for _, id := range ids {
			speaker := piperJapaneseSpeaker{ID: id, SID: e.model.SpeakerIDMap[id]}
			switch strings.ToLower(id) {
			case "tsukuyomi":
				speaker.Name = "つくよみ"
				speaker.Gender = "female"
			case "female":
				speaker.Name = "日语女声"
				speaker.Gender = "female"
			case "male":
				speaker.Name = "日语男声"
				speaker.Gender = "male"
			default:
				speaker.Name = "日语音色 " + id
			}
			e.speakers[id] = speaker
		}
	}
	for _, id := range sortedKeys(e.speakers) {
		speaker := e.speakers[id]
		e.voices = append(e.voices, Voice{
			ID:           speaker.ID,
			Name:         speaker.Name,
			Locale:       "ja-JP",
			Gender:       speaker.Gender,
			Language:     "日本語",
			LanguageCode: "ja",
		})
	}
	e.language = e.model.LanguageIDMap["ja"]
	return nil
}

func (e *PiperJapaneseEngine) ID() string          { return "piper-ja" }
func (e *PiperJapaneseEngine) Name() string        { return "Piper 日本語" }
func (e *PiperJapaneseEngine) Description() string { return "本地 ONNX · 40 MB · 日语专用" }
func (e *PiperJapaneseEngine) Local() bool         { return true }
func (e *PiperJapaneseEngine) SupportsSpeed() bool { return true }
func (e *PiperJapaneseEngine) SpeedRange() (float64, float64) {
	return MinLocalSpeed, MaxSpeed
}
func (e *PiperJapaneseEngine) Available() error { return e.initErr }
func (e *PiperJapaneseEngine) Voices(context.Context) ([]Voice, error) {
	return append([]Voice(nil), e.voices...), e.initErr
}
func (e *PiperJapaneseEngine) DefaultVoice() string {
	if _, ok := e.speakers["tsukuyomi"]; ok {
		return "tsukuyomi"
	}
	if _, ok := e.speakers["female"]; ok {
		return "female"
	}
	if len(e.voices) > 0 {
		return e.voices[0].ID
	}
	return ""
}

func (e *PiperJapaneseEngine) Synthesize(ctx context.Context, text, voiceID string, speed float64) (Audio, error) {
	if e.initErr != nil {
		return Audio{}, e.initErr
	}
	speaker, ok := e.speakers[strings.ToLower(strings.TrimSpace(voiceID))]
	if !ok {
		return Audio{}, fmt.Errorf("unknown Piper Japanese voice %q", voiceID)
	}
	if err := e.load(); err != nil {
		return Audio{}, err
	}
	chunks := SplitText(text, 360)
	if len(chunks) == 0 {
		return Audio{}, ErrEmptyText
	}
	var result []float32
	for index, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return Audio{}, err
		}
		phonemes, err := japanesePhonemes(chunk)
		if err != nil {
			return Audio{}, fmt.Errorf("Piper Japanese chunk %d: %w", index+1, err)
		}
		samples, err := e.synthesizeChunk(phonemes, speaker.SID, speed)
		if err != nil {
			return Audio{}, fmt.Errorf("Piper Japanese chunk %d: %w", index+1, err)
		}
		if index > 0 {
			result = appendSilence(result, e.model.Audio.SampleRate, 80)
		}
		result = append(result, samples...)
	}
	if len(result) == 0 {
		return Audio{}, errors.New("Piper Japanese produced empty audio")
	}
	if len([]rune(strings.TrimSpace(text))) <= 10 {
		padding := e.model.Audio.SampleRate * 300 / 1000
		result = append(make([]float32, padding), result...)
		result = append(result, make([]float32, padding)...)
	}
	return Audio{
		Data:       Float32ToWAV(result, e.model.Audio.SampleRate),
		MIMEType:   "audio/wav",
		Extension:  "wav",
		SampleRate: e.model.Audio.SampleRate,
	}, nil
}

func (e *PiperJapaneseEngine) load() error {
	e.loadOnce.Do(func() {
		if err := ensureONNXRuntime(FindONNXRuntime(e.cfg.RuntimeLib)); err != nil {
			e.loadErr = err
			return
		}
		inputs, outputs, err := ort.GetInputOutputInfo(e.cfg.ModelPath)
		if err != nil {
			e.loadErr = fmt.Errorf("inspect Piper Japanese model: %w", err)
			return
		}
		inputSet := make(map[string]bool, len(inputs))
		for _, input := range inputs {
			inputSet[input.Name] = true
		}
		for _, required := range []string{"input", "input_lengths", "scales"} {
			if !inputSet[required] {
				e.loadErr = fmt.Errorf("unsupported Piper Japanese inputs: %v", inputs)
				return
			}
		}
		for _, input := range inputs {
			switch input.Name {
			case "input", "input_lengths", "scales", "sid", "lid", "language_ids", "prosody_features":
			default:
				e.loadErr = fmt.Errorf("unsupported Piper Japanese input %q: %v", input.Name, inputs)
				return
			}
		}
		e.inputs = inputs
		e.output = "output"
		outputFound := false
		for _, output := range outputs {
			if output.Name == e.output {
				outputFound = true
				break
			}
		}
		if !outputFound {
			e.loadErr = fmt.Errorf("unsupported Piper Japanese outputs: %v", outputs)
			return
		}
		inputNames := make([]string, 0, len(e.inputs))
		for _, input := range e.inputs {
			inputNames = append(inputNames, input.Name)
		}
		session, err := newONNXSession(e.cfg.ModelPath, inputNames, []string{e.output})
		if err != nil {
			e.loadErr = err
			return
		}
		e.session = session
	})
	return e.loadErr
}

func (e *PiperJapaneseEngine) synthesizeChunk(phonemes []string, speakerID int64, speed float64) ([]float32, error) {
	ids, err := buildPiperJapaneseIDs(phonemes, e.model.PhonemeIDMap)
	if err != nil {
		return nil, err
	}

	values := make([]ort.Value, 0, len(e.inputs))
	defer func() {
		for _, value := range values {
			if value != nil {
				_ = value.Destroy()
			}
		}
	}()
	for _, input := range e.inputs {
		var value ort.Value
		var err error
		switch input.Name {
		case "input":
			value, err = ort.NewTensor(ort.NewShape(1, int64(len(ids))), ids)
		case "input_lengths":
			value, err = ort.NewTensor(ort.NewShape(1), []int64{int64(len(ids))})
		case "scales":
			value, err = ort.NewTensor(ort.NewShape(3), []float32{
				e.model.Inference.NoiseScale,
				float32(e.lengthScale / speed),
				e.model.Inference.NoiseW,
			})
		case "sid":
			value, err = ort.NewTensor(ort.NewShape(1), []int64{speakerID})
		case "lid", "language_ids":
			value, err = ort.NewTensor(ort.NewShape(1), []int64{e.language})
		case "prosody_features":
			dimension := int64(3)
			if len(input.Dimensions) > 0 {
				last := input.Dimensions[len(input.Dimensions)-1]
				if last > 0 {
					dimension = last
				}
			}
			shape := ort.NewShape(1, int64(len(ids)), dimension)
			features := make([]int64, int64(len(ids))*dimension)
			value, err = ort.NewTensor(shape, features)
		}
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, fmt.Errorf("unsupported Piper Japanese input %q", input.Name)
		}
		values = append(values, value)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session == nil {
		return nil, errors.New("Piper Japanese is closed")
	}
	outputs := make([]ort.Value, 1)
	if err := e.session.Run(values, outputs); err != nil {
		return nil, err
	}
	defer func() {
		for _, output := range outputs {
			if output != nil {
				_ = output.Destroy()
			}
		}
	}()
	return flattenTensor(outputs[0])
}

func buildPiperJapaneseIDs(phonemes []string, inventory map[string][]int64) ([]int64, error) {
	padID, ok := firstPiperPhonemeID(inventory, "_")
	if !ok {
		return nil, errors.New("Piper Japanese phoneme map has no pad symbol")
	}
	eosToken := "$"
	if len(phonemes) > 0 && phonemes[len(phonemes)-1] == "?" {
		eosToken = "?"
		phonemes = phonemes[:len(phonemes)-1]
	}
	ids := make([]int64, 0, len(phonemes)*3+3)
	if bosID, ok := firstPiperPhonemeID(inventory, "^"); ok {
		ids = append(ids, bosID)
	}
	ids = append(ids, padID)
	for _, phoneme := range normalizePiperPhonemes(phonemes, inventory) {
		values := inventory[phoneme]
		if len(values) == 0 {
			continue
		}
		ids = append(ids, values...)
		ids = append(ids, padID)
	}
	if eosID, ok := firstPiperPhonemeID(inventory, eosToken); ok {
		ids = append(ids, eosID)
	} else if eosID, ok := firstPiperPhonemeID(inventory, "$"); ok {
		ids = append(ids, eosID)
	}
	if len(ids) <= 2 {
		return nil, errors.New("Piper Japanese tokenizer produced no usable tokens")
	}
	return ids, nil
}

func normalizePiperPhonemes(phonemes []string, inventory map[string][]int64) []string {
	result := make([]string, 0, len(phonemes))
	has := func(symbol string) bool { return len(inventory[symbol]) > 0 }
	for _, phoneme := range phonemes {
		switch phoneme {
		case "ɴ":
			if !has("ɴ") && has("N") {
				phoneme = "N"
			}
		case "ʦ":
			if !has("ʦ") {
				result = append(result, "t", "s")
				continue
			}
		case "ɸ":
			if !has("ɸ") && has("f") {
				phoneme = "f"
			}
		case "ʑ":
			if !has("ʑ") && has("j") {
				phoneme = "j"
			}
		case "ʲ":
			if !has("ʲ") && has("y") {
				phoneme = "y"
			}
		case ":":
			if !has(":") && has("ː") {
				phoneme = "ː"
			}
		case "ɾ":
			if !has("ɾ") && has("r") {
				phoneme = "r"
			}
		case "ɕ":
			if !has("ɕ") && has("s") {
				phoneme = "s"
			}
		}
		if has(phoneme) {
			result = append(result, phoneme)
		}
	}
	return result
}

func (e *PiperJapaneseEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	session := e.session
	e.session = nil
	return closeSession(session)
}

func firstPiperPhonemeID(ids map[string][]int64, key string) (int64, bool) {
	values := ids[key]
	if len(values) == 0 {
		return 0, false
	}
	return values[0], true
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
