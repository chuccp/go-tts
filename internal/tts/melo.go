package tts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

	ort "github.com/yalue/onnxruntime_go"
)

type MeloConfig struct {
	ModelPath   string
	LexiconPath string
	TokensPath  string
	RuntimeLib  string
}

type meloPhoneEntry struct {
	phones []int64
	tones  []int64
}

type MeloEngine struct {
	cfg     MeloConfig
	initErr error
	voices  []Voice

	loadOnce sync.Once
	loadErr  error
	mu       sync.Mutex
	session  *ort.DynamicAdvancedSession
	lexicon  map[string]meloPhoneEntry
	tokens   map[string]int64
	maxWord  int

	sampleRate int
	addBlank   bool
	speakerID  int64
}

func NewMeloEngine(cfg MeloConfig) *MeloEngine {
	e := &MeloEngine{
		cfg:    cfg,
		voices: []Voice{{ID: "huayan", Name: "华研", Locale: "zh-CN", Language: "中文", LanguageCode: "zh"}},
	}
	for _, path := range []string{e.cfg.ModelPath, e.cfg.LexiconPath, e.cfg.TokensPath} {
		if _, err := os.Stat(path); err != nil {
			e.initErr = fmt.Errorf("Melo TTS asset is not ready: %w", err)
			break
		}
	}
	return e
}

func (e *MeloEngine) ID() string          { return "melo" }
func (e *MeloEngine) Name() string        { return "Melo TTS" }
func (e *MeloEngine) Description() string { return "本地 ONNX · 中文专用 · 词典韵律" }
func (e *MeloEngine) Local() bool         { return true }
func (e *MeloEngine) SupportsSpeed() bool { return true }
func (e *MeloEngine) SpeedRange() (float64, float64) {
	return MinLocalSpeed, MaxSpeed
}
func (e *MeloEngine) Available() error     { return e.initErr }
func (e *MeloEngine) DefaultVoice() string { return "huayan" }
func (e *MeloEngine) Voices(context.Context) ([]Voice, error) {
	return append([]Voice(nil), e.voices...), e.initErr
}

func (e *MeloEngine) Synthesize(ctx context.Context, text, voiceID string, speed float64) (Audio, error) {
	if e.initErr != nil {
		return Audio{}, e.initErr
	}
	if !strings.EqualFold(strings.TrimSpace(voiceID), "huayan") {
		return Audio{}, fmt.Errorf("unknown Melo voice %q", voiceID)
	}
	if err := e.load(); err != nil {
		return Audio{}, err
	}
	chunks := SplitText(text, 220)
	if len(chunks) == 0 {
		return Audio{}, ErrEmptyText
	}
	var allSamples []float32
	for i, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return Audio{}, err
		}
		samples, err := e.synthesizeChunk(chunk, speed)
		if err != nil {
			return Audio{}, fmt.Errorf("Melo chunk %d: %w", i+1, err)
		}
		if i > 0 {
			allSamples = appendSilence(allSamples, e.sampleRate, 90)
		}
		allSamples = append(allSamples, samples...)
	}
	if len(allSamples) == 0 {
		return Audio{}, errors.New("Melo produced empty audio")
	}
	return Audio{
		Data:       Float32ToWAV(allSamples, e.sampleRate),
		MIMEType:   "audio/wav",
		Extension:  "wav",
		SampleRate: e.sampleRate,
	}, nil
}

func (e *MeloEngine) load() error {
	e.loadOnce.Do(func() {
		if err := ensureONNXRuntime(FindONNXRuntime(e.cfg.RuntimeLib)); err != nil {
			e.loadErr = err
			return
		}
		tokens, err := loadMeloTokens(e.cfg.TokensPath)
		if err != nil {
			e.loadErr = err
			return
		}
		lexicon, maxWord, err := loadMeloLexicon(e.cfg.LexiconPath, tokens)
		if err != nil {
			e.loadErr = err
			return
		}
		session, err := newONNXSession(
			e.cfg.ModelPath,
			[]string{"x", "x_lengths", "tones", "sid", "noise_scale", "noise_scale_w", "length_scale"},
			[]string{"y"},
		)
		if err != nil {
			e.loadErr = err
			return
		}
		metadata, err := session.GetModelMetadata()
		if err != nil {
			_ = session.Destroy()
			e.loadErr = fmt.Errorf("read Melo metadata: %w", err)
			return
		}
		defer metadata.Destroy()
		sampleRate, err := meloMetadataInt(metadata, "sample_rate", 44100)
		if err != nil {
			_ = session.Destroy()
			e.loadErr = err
			return
		}
		addBlank, err := meloMetadataInt(metadata, "add_blank", 1)
		if err != nil {
			_ = session.Destroy()
			e.loadErr = err
			return
		}
		speakerID, err := meloMetadataInt(metadata, "speaker_id", 0)
		if err != nil {
			_ = session.Destroy()
			e.loadErr = err
			return
		}
		e.tokens = tokens
		e.lexicon = lexicon
		e.maxWord = maxWord
		e.session = session
		e.sampleRate = sampleRate
		e.addBlank = addBlank != 0
		e.speakerID = int64(speakerID)
	})
	return e.loadErr
}

func (e *MeloEngine) synthesizeChunk(text string, speed float64) ([]float32, error) {
	phones, tones := e.textToTokens(text)
	if len(phones) == 0 {
		return nil, errors.New("Melo tokenizer produced no tokens")
	}
	if e.addBlank {
		blankPhones := make([]int64, 2*len(phones)+1)
		blankTones := make([]int64, 2*len(tones)+1)
		for i := range phones {
			blankPhones[2*i+1] = phones[i]
			blankTones[2*i+1] = tones[i]
		}
		phones, tones = blankPhones, blankTones
	}
	x, err := ort.NewTensor(ort.NewShape(1, int64(len(phones))), phones)
	if err != nil {
		return nil, err
	}
	defer x.Destroy()
	xLength, err := ort.NewTensor(ort.NewShape(1), []int64{int64(len(phones))})
	if err != nil {
		return nil, err
	}
	defer xLength.Destroy()
	tonesTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(tones))), tones)
	if err != nil {
		return nil, err
	}
	defer tonesTensor.Destroy()
	sid, err := ort.NewTensor(ort.NewShape(1), []int64{e.speakerID})
	if err != nil {
		return nil, err
	}
	defer sid.Destroy()
	noiseScale, err := ort.NewTensor(ort.NewShape(1), []float32{0.6})
	if err != nil {
		return nil, err
	}
	defer noiseScale.Destroy()
	noiseScaleW, err := ort.NewTensor(ort.NewShape(1), []float32{0.8})
	if err != nil {
		return nil, err
	}
	defer noiseScaleW.Destroy()
	lengthScale, err := ort.NewTensor(ort.NewShape(1), []float32{float32(1 / speed)})
	if err != nil {
		return nil, err
	}
	defer lengthScale.Destroy()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session == nil {
		return nil, errors.New("Melo TTS is closed")
	}
	outputs := make([]ort.Value, 1)
	if err := e.session.Run(
		[]ort.Value{x, xLength, tonesTensor, sid, noiseScale, noiseScaleW, lengthScale},
		outputs,
	); err != nil {
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

func (e *MeloEngine) textToTokens(text string) ([]int64, []int64) {
	runes := []rune(strings.ToLower(text))
	for i, r := range runes {
		switch r {
		case '，', '、':
			runes[i] = ','
		case '。', '．':
			runes[i] = '.'
		case '！':
			runes[i] = '!'
		case '？':
			runes[i] = '?'
		case '：':
			runes[i] = ':'
		case '；':
			runes[i] = ';'
		}
	}
	phones := make([]int64, 0, len(runes)*2)
	tones := make([]int64, 0, len(runes)*2)
	spaceID := e.tokens["_"]
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			phones = append(phones, spaceID)
			tones = append(tones, 0)
			i++
			continue
		}
		matchLength := 0
		var match meloPhoneEntry
		maxLength := min(e.maxWord, len(runes)-i)
		for length := maxLength; length >= 1; length-- {
			if entry, ok := e.lexicon[string(runes[i:i+length])]; ok {
				matchLength = length
				match = entry
				break
			}
		}
		if unicode.Is(unicode.Han, runes[i]) && matchLength < 2 {
			if length, entry := meloPinyinFallback(runes, i, e.tokens); length > 1 {
				phones = append(phones, entry.phones...)
				tones = append(tones, entry.tones...)
				i += length
				continue
			}
		}
		if matchLength == 0 {
			if unicode.Is(unicode.Han, runes[i]) {
				if length, entry := meloPinyinFallback(runes, i, e.tokens); length > 0 {
					phones = append(phones, entry.phones...)
					tones = append(tones, entry.tones...)
					i += length
					continue
				}
			}
			if id, ok := e.tokens["UNK"]; ok {
				phones = append(phones, id)
				tones = append(tones, 0)
			}
			i++
			continue
		}
		phones = append(phones, match.phones...)
		tones = append(tones, match.tones...)
		i += matchLength
	}
	return phones, tones
}

func meloPinyinFallback(runes []rune, start int, tokens map[string]int64) (int, meloPhoneEntry) {
	data, err := loadChinesePinyinData()
	if err != nil {
		return 0, meloPhoneEntry{}
	}
	syllables := []string(nil)
	length := 0
	if phraseLength, phrase := matchChinesePhrase(data, runes[start:], 0); phraseLength > 0 {
		length = phraseLength
		syllables = phrase
	} else if syllable, ok := data.chars[runes[start]]; ok {
		length = 1
		syllables = []string{syllable}
	}
	if length == 0 {
		return 0, meloPhoneEntry{}
	}
	var entry meloPhoneEntry
	for _, syllable := range syllables {
		phones, tones := meloPinyinToPhones(syllable, tokens)
		if len(phones) == 0 {
			return 0, meloPhoneEntry{}
		}
		entry.phones = append(entry.phones, phones...)
		entry.tones = append(entry.tones, tones...)
	}
	return length, entry
}

func meloPinyinToPhones(value string, tokens map[string]int64) ([]int64, []int64) {
	tone := int64(0)
	if len(value) > 0 {
		if parsed, err := strconv.ParseInt(value[len(value)-1:], 10, 64); err == nil {
			tone = parsed
			value = value[:len(value)-1]
		}
	}
	value = strings.ReplaceAll(value, "ü", "v")
	initials := []string{
		"zh", "ch", "sh", "b", "p", "m", "f", "d", "t", "n", "l",
		"g", "k", "h", "j", "q", "x", "r", "z", "c", "s", "y", "w",
	}
	initial := ""
	final := value
	for _, candidate := range initials {
		if strings.HasPrefix(value, candidate) {
			initial = candidate
			final = strings.TrimPrefix(value, candidate)
			break
		}
	}
	if initial == "j" || initial == "q" || initial == "x" {
		if strings.HasPrefix(final, "u") {
			final = "v" + strings.TrimPrefix(final, "u")
		}
	}
	if initial == "y" {
		switch final {
		case "an":
			final = "En"
		case "u":
			final = "v"
		case "ue":
			final = "ve"
		case "uan":
			final = "van"
		case "un":
			final = "vn"
		}
	}
	switch initial {
	case "z", "c", "s":
		if final == "i" {
			final = "i0"
		}
	case "zh", "ch", "sh", "r":
		if final == "i" {
			final = "ir"
		}
	}
	if initial == "" && value == "er" {
		initial, final = "EE", "er"
	}
	phoneNames := make([]string, 0, 2)
	if initial != "" {
		phoneNames = append(phoneNames, initial)
	}
	if final != "" {
		phoneNames = append(phoneNames, final)
	}
	phones := make([]int64, 0, len(phoneNames))
	tones := make([]int64, 0, len(phoneNames))
	for _, name := range phoneNames {
		id, ok := tokens[name]
		if !ok {
			return nil, nil
		}
		phones = append(phones, id)
		tones = append(tones, tone)
	}
	return phones, tones
}

func loadMeloTokens(path string) (map[string]int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		result[fields[0]] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func loadMeloLexicon(path string, tokens map[string]int64) (map[string]meloPhoneEntry, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	result := make(map[string]meloPhoneEntry, 200000)
	maxWord := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || len(fields)%2 == 0 {
			continue
		}
		half := (len(fields) - 1) / 2
		phones := make([]int64, half)
		tones := make([]int64, half)
		valid := true
		for i := 0; i < half; i++ {
			id, ok := tokens[fields[1+i]]
			if !ok {
				valid = false
				break
			}
			tone, err := strconv.ParseInt(fields[1+half+i], 10, 64)
			if err != nil {
				valid = false
				break
			}
			phones[i] = id
			tones[i] = tone
		}
		if !valid {
			continue
		}
		result[fields[0]] = meloPhoneEntry{phones: phones, tones: tones}
		if length := len([]rune(fields[0])); length > maxWord {
			maxWord = length
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	if len(result) == 0 {
		return nil, 0, errors.New("Melo lexicon is empty")
	}
	return result, maxWord, nil
}

func meloMetadataInt(metadata *ort.ModelMetadata, key string, fallback int) (int, error) {
	value, present, err := metadata.LookupCustomMetadataMap(key)
	if err != nil {
		return 0, fmt.Errorf("read Melo metadata %s: %w", key, err)
	}
	if !present || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid Melo metadata %s=%q", key, value)
	}
	return parsed, nil
}

func (e *MeloEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	session := e.session
	e.session = nil
	return closeSession(session)
}
