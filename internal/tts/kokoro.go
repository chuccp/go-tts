package tts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

const kokoroStyleDim = 256

type KokoroConfig struct {
	ModelPath  string
	VoicesPath string
	RuntimeLib string
	EspeakPath string
}

type KokoroEngine struct {
	cfg       KokoroConfig
	initErr   error
	session   *ort.DynamicAdvancedSession
	mu        sync.Mutex
	voices    []Voice
	styles    map[string]floatTensor
	tokenizer map[rune]int64
}

func NewKokoroEngine(cfg KokoroConfig) *KokoroEngine {
	e := &KokoroEngine{
		cfg:       cfg,
		styles:    make(map[string]floatTensor),
		tokenizer: kokoroVocabulary(),
	}
	if cfg.EspeakPath == "" {
		cfg.EspeakPath = FindEspeakPath("")
	}
	e.cfg.EspeakPath = cfg.EspeakPath
	e.initErr = e.initialize()
	return e
}

func (e *KokoroEngine) initialize() error {
	if _, err := os.Stat(e.cfg.ModelPath); err != nil {
		return fmt.Errorf("Kokoro model is not ready: %w", err)
	}
	if _, err := exec.LookPath(e.cfg.EspeakPath); err != nil {
		return fmt.Errorf("espeak-ng was not found; set -espeak or PATH")
	}
	tensors, err := LoadVoiceTensorFile(e.cfg.VoicesPath)
	if err != nil {
		return fmt.Errorf("load Kokoro voices: %w", err)
	}
	validStyles := make(map[string]floatTensor)
	voices := make([]Voice, 0, len(tensors))
	for name, tensor := range tensors {
		rows, columns := tensorRows(tensor)
		if columns != kokoroStyleDim || rows == 0 {
			continue
		}
		validStyles[name] = tensor
		locale := kokoroLocale(name)
		voices = append(voices, Voice{
			ID:       name,
			Name:     prettyKokoroName(name),
			Locale:   locale,
			Language: kokoroLanguageName(locale),
		})
	}
	if len(voices) == 0 {
		return errors.New("Kokoro voice pack contains no 256-dimensional voice profiles")
	}
	sortVoices(voices)
	e.voices = voices
	e.styles = validStyles
	if err := ensureONNXRuntime(FindONNXRuntime(e.cfg.RuntimeLib)); err != nil {
		return err
	}
	session, err := newONNXSession(
		e.cfg.ModelPath,
		[]string{"tokens", "style", "speed"},
		[]string{"audio"},
	)
	if err != nil {
		return err
	}
	e.session = session
	return nil
}

func (e *KokoroEngine) ID() string          { return "kokoro" }
func (e *KokoroEngine) Name() string        { return "Kokoro" }
func (e *KokoroEngine) Description() string { return "本地 ONNX · 82M · 多语言音色" }
func (e *KokoroEngine) Local() bool         { return true }
func (e *KokoroEngine) SupportsSpeed() bool { return true }
func (e *KokoroEngine) Available() error    { return e.initErr }
func (e *KokoroEngine) DefaultVoice() string {
	for _, voice := range e.voices {
		if voice.ID == "af_heart" {
			return voice.ID
		}
	}
	if len(e.voices) > 0 {
		return e.voices[0].ID
	}
	return ""
}
func (e *KokoroEngine) Voices(context.Context) ([]Voice, error) {
	return append([]Voice(nil), e.voices...), e.initErr
}

func (e *KokoroEngine) Synthesize(ctx context.Context, text, voiceID string, speed float64) (Audio, error) {
	if e.initErr != nil {
		return Audio{}, e.initErr
	}
	tensor, ok := e.styles[voiceID]
	if !ok {
		for name, candidate := range e.styles {
			if strings.EqualFold(name, voiceID) {
				tensor, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return Audio{}, fmt.Errorf("unknown Kokoro voice %q", voiceID)
	}
	chunks := SplitText(text, 360)
	if len(chunks) == 0 {
		return Audio{}, ErrEmptyText
	}
	language := kokoroLocale(voiceID)
	allSamples := make([]float32, 0, 24000*2)
	for index, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return Audio{}, err
		}
		samples, err := e.synthesizeChunk(ctx, chunk, language, tensor, float32(speed), 0)
		if err != nil {
			return Audio{}, fmt.Errorf("Kokoro chunk %d: %w", index+1, err)
		}
		if index > 0 {
			allSamples = appendSilence(allSamples, 24000, 90)
		}
		allSamples = append(allSamples, samples...)
	}
	if len(allSamples) == 0 {
		return Audio{}, errors.New("Kokoro produced empty audio")
	}
	return Audio{
		Data:       Float32ToWAV(allSamples, 24000),
		MIMEType:   "audio/wav",
		Extension:  "wav",
		SampleRate: 24000,
	}, nil
}

func (e *KokoroEngine) synthesizeChunk(
	ctx context.Context,
	text, language string,
	style floatTensor,
	speed float32,
	depth int,
) ([]float32, error) {
	phonemes, err := phonemize(ctx, e.cfg.EspeakPath, language, text)
	if err != nil {
		return nil, err
	}
	tokens := make([]int64, 0, len([]rune(phonemes))+2)
	for _, r := range phonemes {
		if token, ok := e.tokenizer[r]; ok {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 {
		return nil, errors.New("tokenizer produced no Kokoro tokens")
	}
	if len(tokens) > 500 && depth < 4 {
		chunks := SplitText(text, len([]rune(text))/2)
		if len(chunks) < 2 {
			runes := []rune(text)
			mid := len(runes) / 2
			chunks = []string{string(runes[:mid]), string(runes[mid:])}
		}
		var samples []float32
		for index, chunk := range chunks {
			part, err := e.synthesizeChunk(ctx, chunk, language, style, speed, depth+1)
			if err != nil {
				return nil, err
			}
			if index > 0 {
				samples = appendSilence(samples, 24000, 60)
			}
			samples = append(samples, part...)
		}
		return samples, nil
	}

	rows, columns := tensorRows(style)
	styleIndex := len(tokens)
	if styleIndex >= rows {
		styleIndex = rows - 1
	}
	styleData := style.Data[styleIndex*columns : (styleIndex+1)*columns]
	padded := make([]int64, 0, len(tokens)+2)
	padded = append(padded, 0)
	padded = append(padded, tokens...)
	padded = append(padded, 0)

	tokenTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(padded))), padded)
	if err != nil {
		return nil, err
	}
	defer tokenTensor.Destroy()
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
	outputs := make([]ort.Value, 1)
	if err := e.session.Run(
		[]ort.Value{tokenTensor, styleTensor, speedTensor},
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

func (e *KokoroEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	session := e.session
	e.session = nil
	return closeSession(session)
}

func kokoroLocale(voice string) string {
	if len(voice) < 1 {
		return "en-us"
	}
	switch voice[0] {
	case 'a':
		return "en-us"
	case 'b':
		return "en-gb"
	case 'f':
		return "fr-fr"
	case 'h':
		return "hi"
	case 'i':
		return "it"
	case 'j':
		return "ja"
	case 'p':
		return "pt-br"
	case 'z':
		return "cmn"
	default:
		return "en-us"
	}
}

func kokoroLanguageName(locale string) string {
	switch locale {
	case "en-us":
		return "English (US)"
	case "en-gb":
		return "English (UK)"
	case "fr-fr":
		return "Français"
	case "hi":
		return "हिन्दी"
	case "it":
		return "Italiano"
	case "ja":
		return "日本語"
	case "pt-br":
		return "Português (BR)"
	case "cmn":
		return "中文"
	default:
		return strings.ToUpper(locale)
	}
}

func prettyKokoroName(voice string) string {
	name := voice
	name = strings.ReplaceAll(name, "_", " ")
	parts := strings.Fields(name)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func kokoroVocabulary() map[rune]int64 {
	const (
		pad         = "$"
		punctuation = ";:,.!?¡¿—…\"«»“” "
		letters     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
		lettersIPA  = "ɑɐɒæɓʙβɔɕçɗɖðʤəɘɚɛɜɝɞɟʄɡɠɢʛɦɧħɥʜɨɪʝɭɬɫɮʟɱɯɰŋɳɲɴøɵɸθœɶʘɹɺɾɻʀʁɽʂʃʈʧʉʊʋⱱʌɣɤʍχʎʏʑʐʒʔʡʕʢǀǁǂǃˈˌːˑʼʴʰʱʲʷˠˤ˞↓↑→↗↘'̩'ᵻ"
	)
	vocabulary := make(map[rune]int64)
	for index, r := range []rune(pad + punctuation + letters + lettersIPA) {
		vocabulary[r] = int64(index)
	}
	return vocabulary
}
