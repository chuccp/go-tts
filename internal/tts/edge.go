package tts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	edgeClientToken    = edgeTrustedClientToken
	edgeChromeVersion  = "143.0.3650.75"
	edgeTokenVersion   = "2-" + edgeChromeVersion
	edgeSpeechEndpoint = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeVoiceEndpoint  = "https://speech.platform.bing.com/consumer/speech/synthesize/readaloud/voices/list"
)

type EdgeConfig struct {
	DefaultVoice string
	ProxyURL     string
	Timeout      time.Duration
}

type EdgeEngine struct {
	defaultVoice   string
	speechEndpoint string
	voiceEndpoint  string
	httpClient     *http.Client
	dialer         *websocket.Dialer
	voiceMu        sync.Mutex
	voiceCache     []Voice
	voiceExpiry    time.Time
}

type edgeVoiceResponse struct {
	ShortName    string `json:"ShortName"`
	Gender       string `json:"Gender"`
	Locale       string `json:"Locale"`
	FriendlyName string `json:"FriendlyName"`
}

func NewEdgeEngine(cfg EdgeConfig) *EdgeEngine {
	if cfg.DefaultVoice == "" {
		cfg.DefaultVoice = "zh-CN-XiaoxiaoNeural"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		transport = transport.Clone()
	} else {
		transport = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	if cfg.ProxyURL != "" {
		if proxyURL, err := url.Parse(cfg.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	dialer := &websocket.Dialer{
		Proxy:             transport.Proxy,
		HandshakeTimeout:  15 * time.Second,
		EnableCompression: true,
	}
	return &EdgeEngine{
		defaultVoice:   cfg.DefaultVoice,
		speechEndpoint: edgeSpeechEndpoint,
		voiceEndpoint:  edgeVoiceEndpoint,
		httpClient:     &http.Client{Transport: transport, Timeout: 20 * time.Second},
		dialer:         dialer,
	}
}

func (e *EdgeEngine) ID() string          { return "edge" }
func (e *EdgeEngine) Name() string        { return "Edge TTS" }
func (e *EdgeEngine) Description() string { return "在线 · 400+ 音色 · 90+ 语言" }
func (e *EdgeEngine) Local() bool         { return false }
func (e *EdgeEngine) SupportsSpeed() bool { return true }
func (e *EdgeEngine) Available() error    { return nil }
func (e *EdgeEngine) DefaultVoice() string {
	return e.defaultVoice
}

func (e *EdgeEngine) PreviewVoices() []Voice {
	e.voiceMu.Lock()
	defer e.voiceMu.Unlock()
	if len(e.voiceCache) > 0 {
		return append([]Voice(nil), e.voiceCache...)
	}
	return fallbackEdgeVoices()
}

func (e *EdgeEngine) Voices(ctx context.Context) ([]Voice, error) {
	e.voiceMu.Lock()
	if len(e.voiceCache) > 0 && time.Now().Before(e.voiceExpiry) {
		result := append([]Voice(nil), e.voiceCache...)
		e.voiceMu.Unlock()
		return result, nil
	}
	stale := append([]Voice(nil), e.voiceCache...)
	e.voiceMu.Unlock()

	voices, err := e.fetchVoices(ctx)
	if err != nil {
		if len(stale) > 0 {
			return stale, nil
		}
		return fallbackEdgeVoices(), err
	}
	e.voiceMu.Lock()
	e.voiceCache = append([]Voice(nil), voices...)
	e.voiceExpiry = time.Now().Add(time.Hour)
	e.voiceMu.Unlock()
	return voices, nil
}

func (e *EdgeEngine) fetchVoices(ctx context.Context) ([]Voice, error) {
	now := time.Now()
	query := url.Values{}
	query.Set("trustedclienttoken", edgeClientToken)
	query.Set("Sec-MS-GEC", generateEdgeToken(now))
	query.Set("Sec-MS-GEC-Version", edgeTokenVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.voiceEndpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	setEdgeHeaders(req.Header)
	response, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Edge voices: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return nil, fmt.Errorf("fetch Edge voices: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload []edgeVoiceResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Edge voices: %w", err)
	}
	voices := make([]Voice, 0, len(payload))
	for _, item := range payload {
		if item.ShortName == "" {
			continue
		}
		name := friendlyEdgeVoiceName(item.ShortName)
		voices = append(voices, Voice{
			ID:           item.ShortName,
			Name:         name,
			Locale:       item.Locale,
			Gender:       item.Gender,
			Language:     edgeLanguageName(item.Locale),
			LanguageCode: edgeLanguageCode(item.Locale),
		})
	}
	if len(voices) == 0 {
		return nil, errors.New("Edge returned an empty voice list")
	}
	sort.SliceStable(voices, func(i, j int) bool {
		if edgeLocalePriority(voices[i].Locale) != edgeLocalePriority(voices[j].Locale) {
			return edgeLocalePriority(voices[i].Locale) < edgeLocalePriority(voices[j].Locale)
		}
		if voices[i].Locale != voices[j].Locale {
			return voices[i].Locale < voices[j].Locale
		}
		return voices[i].Name < voices[j].Name
	})
	return voices, nil
}

func (e *EdgeEngine) Synthesize(ctx context.Context, text, voice string, speed float64) (Audio, error) {
	var output bytes.Buffer
	chunks := SplitText(text, 600)
	if len(chunks) == 0 {
		return Audio{}, ErrEmptyText
	}
	for index, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return Audio{}, err
		}
		audio, err := e.synthesizeChunkWithRetry(ctx, chunk, voice, speed)
		if err != nil {
			return Audio{}, fmt.Errorf("Edge chunk %d: %w", index+1, err)
		}
		_, _ = output.Write(audio)
	}
	if output.Len() == 0 {
		return Audio{}, errors.New("Edge returned no audio")
	}
	return Audio{
		Data:       output.Bytes(),
		MIMEType:   "audio/mpeg",
		Extension:  "mp3",
		SampleRate: 24000,
	}, nil
}

func (e *EdgeEngine) synthesizeChunkWithRetry(ctx context.Context, text, voice string, speed float64) ([]byte, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(250*(1<<(attempt-1))) * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		audio, err := e.synthesizeChunk(ctx, text, voice, speed)
		if err == nil {
			return audio, nil
		}
		lastErr = err
		if ctx.Err() != nil || !isRetryableEdgeError(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("Edge connection retry limit reached: %w", lastErr)
}

func isRetryableEdgeError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var closeError *websocket.CloseError
	if errors.As(err, &closeError) {
		switch closeError.Code {
		case websocket.CloseAbnormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseInternalServerErr,
			websocket.CloseServiceRestart,
			websocket.CloseTryAgainLater:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "read edge stream") ||
		strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "edge websocket: ")
}

func (e *EdgeEngine) synthesizeChunk(ctx context.Context, text, voice string, speed float64) ([]byte, error) {
	connectionID := randomHex(16)
	token := generateEdgeToken(time.Now())
	endpoint := fmt.Sprintf("%s?TrustedClientToken=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		e.speechEndpoint, edgeClientToken, connectionID, token, edgeTokenVersion)
	header := http.Header{}
	setEdgeHeaders(header)
	conn, response, err := e.dialer.DialContext(ctx, endpoint, header)
	if err != nil {
		if response != nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
			_ = response.Body.Close()
			return nil, fmt.Errorf("Edge WebSocket HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("Edge WebSocket: %w", err)
	}
	defer conn.Close()
	cancelRead := make(chan struct{})
	defer close(cancelRead)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(time.Now())
		case <-cancelRead:
		}
	}()

	deadline := time.Now().Add(90 * time.Second)
	if requestDeadline, ok := ctx.Deadline(); ok && requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	_ = conn.SetWriteDeadline(deadline)
	_ = conn.SetReadDeadline(deadline)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(edgeSpeechConfig(time.Now()))); err != nil {
		return nil, err
	}
	requestID := randomHex(16)
	ssml := buildEdgeSSML(text, voice, speed)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(edgeSSMLMessage(requestID, time.Now(), ssml))); err != nil {
		return nil, err
	}

	var audio bytes.Buffer
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
					return nil, context.DeadlineExceeded
				}
			}
			return nil, fmt.Errorf("read Edge stream: %w", err)
		}
		switch messageType {
		case websocket.TextMessage:
			headers, _, err := parseEdgeHeaders(message)
			if err != nil {
				return nil, err
			}
			path := strings.TrimSpace(string(headers["Path"]))
			switch path {
			case "turn.end":
				if audio.Len() == 0 {
					return nil, errors.New("Edge completed without audio")
				}
				return audio.Bytes(), nil
			case "", "response", "turn.start", "audio.metadata":
				continue
			default:
				return nil, fmt.Errorf("unexpected Edge response path %q", path)
			}
		case websocket.BinaryMessage:
			if len(message) < 4 {
				return nil, errors.New("invalid Edge binary audio message")
			}
			headerLength := int(binary.BigEndian.Uint16(message[:2]))
			if headerLength < 2 || headerLength > len(message) {
				return nil, fmt.Errorf("Edge binary audio header length %d exceeds message size %d", headerLength, len(message))
			}
			headers, err := parseEdgeHeaderLines(message[2:headerLength])
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(string(headers["Path"])) != "audio" {
				continue
			}
			dataStart := headerLength + 2
			if dataStart > len(message) {
				dataStart = len(message)
			}
			audioData := message[dataStart:]
			if len(audioData) == 0 {
				continue
			}
			if contentLength := strings.TrimSpace(string(headers["Content-Length"])); contentLength != "" {
				expected, _ := strconv.Atoi(contentLength)
				actual := len(audioData)
				if expected > 0 && expected != actual {
					return nil, fmt.Errorf("Edge audio length %d does not match header %d", actual, expected)
				}
			}
			_, _ = audio.Write(audioData)
		default:
			continue
		}
	}
}

func (e *EdgeEngine) Close() error { return nil }

func setEdgeHeaders(header http.Header) {
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0")
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")
	header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	header.Set("Accept", "*/*")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Cookie", "muid="+randomHex(16)+";")
	header.Set("Sec-CH-UA", `"Not;A Brand";v="99", "Microsoft Edge";v="143", "Chromium";v="143"`)
	header.Set("Sec-CH-UA-Mobile", "?0")
	header.Set("Sec-Fetch-Site", "none")
	header.Set("Sec-Fetch-Mode", "cors")
	header.Set("Sec-Fetch-Dest", "empty")
}

func edgeSpeechConfig(now time.Time) string {
	return "X-Timestamp:" + edgeTimestamp(now) + "\r\n" +
		"Content-Type:application/json; charset=utf-8\r\n" +
		"Path:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}` +
		"\r\n"
}

func edgeSSMLMessage(requestID string, now time.Time, ssml string) string {
	return "X-RequestId:" + requestID + "\r\n" +
		"Content-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + edgeTimestamp(now) + "Z\r\n" +
		"Path:ssml\r\n\r\n" + ssml
}

func edgeTimestamp(now time.Time) string {
	return now.UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
}

func buildEdgeSSML(text, voice string, speed float64) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(text))
	locale := escapeXMLAttribute(edgeVoiceLocale(voice))
	rate := int(math.Round((speed - 1) * 100))
	return "<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='" +
		locale + "'><voice name='" + escapeXMLAttribute(voice) + "'><prosody pitch='+0Hz' rate='" +
		fmt.Sprintf("%+d%%", rate) + "' volume='+0%'>" +
		escaped.String() + "</prosody></voice></speak>"
}

func escapeXMLAttribute(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "&apos;", `"`, "&quot;")
	return replacer.Replace(value)
}

func edgeVoiceLocale(voice string) string {
	parts := strings.Split(voice, "-")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	return "en-US"
}

func parseEdgeHeaders(message []byte) (map[string][]byte, []byte, error) {
	end := bytes.Index(message, []byte("\r\n\r\n"))
	if end < 0 {
		return nil, nil, errors.New("Edge text message has no header separator")
	}
	headers, err := parseEdgeHeaderLines(message[:end])
	return headers, message[end+4:], err
}

func parseEdgeHeaderLines(data []byte) (map[string][]byte, error) {
	headers := make(map[string][]byte)
	for _, line := range bytes.Split(data, []byte("\r\n")) {
		if len(line) == 0 {
			continue
		}
		key, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			return nil, fmt.Errorf("invalid Edge header %q", line)
		}
		headers[string(bytes.TrimSpace(key))] = bytes.TrimSpace(value)
	}
	return headers, nil
}

func randomHex(bytesCount int) string {
	data := make([]byte, bytesCount)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(data)
}

func fallbackEdgeVoices() []Voice {
	items := [][4]string{
		{"zh-CN-XiaoxiaoNeural", "晓晓", "zh-CN", "Female"},
		{"zh-CN-XiaoyiNeural", "晓伊", "zh-CN", "Female"},
		{"zh-CN-YunxiNeural", "云希", "zh-CN", "Male"},
		{"zh-CN-YunjianNeural", "云健", "zh-CN", "Male"},
		{"zh-TW-HsiaoChenNeural", "曉臻", "zh-TW", "Female"},
		{"en-US-AvaMultilingualNeural", "Ava Multilingual", "en-US", "Female"},
		{"en-US-EmmaMultilingualNeural", "Emma Multilingual", "en-US", "Female"},
		{"en-US-AriaNeural", "Aria", "en-US", "Female"},
		{"en-US-GuyNeural", "Guy", "en-US", "Male"},
		{"en-GB-SoniaNeural", "Sonia", "en-GB", "Female"},
		{"en-GB-RyanNeural", "Ryan", "en-GB", "Male"},
		{"ja-JP-NanamiNeural", "Nanami", "ja-JP", "Female"},
		{"ja-JP-KeitaNeural", "Keita", "ja-JP", "Male"},
		{"ko-KR-SunHiNeural", "Sun-Hi", "ko-KR", "Female"},
		{"fr-FR-DeniseNeural", "Denise", "fr-FR", "Female"},
		{"de-DE-KatjaNeural", "Katja", "de-DE", "Female"},
		{"es-ES-ElviraNeural", "Elvira", "es-ES", "Female"},
	}
	voices := make([]Voice, 0, len(items))
	for _, item := range items {
		voices = append(voices, Voice{
			ID:           item[0],
			Name:         item[1],
			Locale:       item[2],
			Gender:       item[3],
			Language:     edgeLanguageName(item[2]),
			LanguageCode: edgeLanguageCode(item[2]),
		})
	}
	return voices
}

func friendlyEdgeVoiceName(shortName string) string {
	parts := strings.Split(shortName, "-")
	if len(parts) < 3 {
		return shortName
	}
	return strings.TrimSuffix(parts[len(parts)-1], "Neural")
}

func edgeLanguageName(locale string) string {
	language := strings.Split(locale, "-")[0]
	names := map[string]string{
		"zh": "中文", "en": "English", "ja": "日本語", "ko": "한국어",
		"fr": "Français", "de": "Deutsch", "es": "Español", "pt": "Português",
		"it": "Italiano", "ru": "Русский", "ar": "العربية", "hi": "हिन्दी",
	}
	if name, ok := names[language]; ok {
		return name
	}
	return strings.ToUpper(language)
}

func edgeLanguageCode(locale string) string {
	if language, _, ok := strings.Cut(locale, "-"); ok {
		return strings.ToLower(language)
	}
	return strings.ToLower(locale)
}

func edgeLocalePriority(locale string) int {
	switch {
	case locale == "zh-CN":
		return 0
	case strings.HasPrefix(locale, "zh"):
		return 1
	case strings.HasPrefix(locale, "en"):
		return 2
	default:
		return 3
	}
}
