package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cao/go-tts/internal/tts"
	"github.com/cao/go-tts/internal/ttssvc"
	wf "github.com/chuccp/go-web-frame"
	"github.com/chuccp/go-web-frame/config"
)

const testPort = 19009

type fakeEngine struct {
	available    bool
	audio        []byte
	voices       []tts.Voice
	voicesErr    error
	defaultVoice string
}

func (f *fakeEngine) ID() string          { return "fake" }
func (f *fakeEngine) Name() string        { return "Fake TTS" }
func (f *fakeEngine) Description() string { return "test engine" }
func (f *fakeEngine) Local() bool         { return false }
func (f *fakeEngine) SupportsSpeed() bool { return true }
func (f *fakeEngine) Close() error        { return nil }
func (f *fakeEngine) DefaultVoice() string {
	return f.defaultVoice
}
func (f *fakeEngine) Available() error {
	if f.available {
		return nil
	}
	return io.ErrClosedPipe
}
func (f *fakeEngine) Voices(context.Context) ([]tts.Voice, error) {
	if f.voices != nil {
		return f.voices, f.voicesErr
	}
	return []tts.Voice{{ID: "test", Name: "Test", Locale: "zh-CN"}}, f.voicesErr
}
func (f *fakeEngine) Synthesize(context.Context, string, string, float64) (tts.Audio, error) {
	return tts.Audio{
		Data:      f.audio,
		MIMEType:  "audio/mpeg",
		Extension: "mp3",
	}, nil
}

func TestHomeAndSynthesize(t *testing.T) {
	t.Parallel()
	engine := &fakeEngine{available: true, audio: []byte("mp3-data")}
	handler := newTestHandler(t, tts.NewService(engine))

	home := httptest.NewRecorder()
	handler.ServeHTTP(home, newTestRequest(http.MethodGet, "/", ""))
	if home.Code != http.StatusOK {
		t.Fatalf("home status = %d", home.Code)
	}
	if !strings.Contains(home.Body.String(), "Fake TTS") || !strings.Contains(home.Body.String(), "VoxLab") {
		t.Fatal("home page is missing engine or title content")
	}

	form := url.Values{
		"engine": {"fake"},
		"voice":  {"test"},
		"text":   {"hello"},
		"speed":  {"1.25"},
	}
	request := newTestRequest(http.MethodPost, "/api/synthesize", form.Encode())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("synthesize status = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
	if response.Body.String() != "mp3-data" {
		t.Fatalf("audio body = %q", response.Body.String())
	}
}

func TestSynthesizeRejectsInvalidRequest(t *testing.T) {
	t.Parallel()
	engine := &fakeEngine{available: true}
	handler := newTestHandler(t, tts.NewService(engine))
	form := url.Values{
		"engine": {"fake"},
		"voice":  {"test"},
		"text":   {"hello"},
		"speed":  {"4"},
	}
	request := newTestRequest(http.MethodPost, "/api/synthesize", form.Encode())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, tts.NewService(&fakeEngine{available: true}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newTestRequest(http.MethodGet, "/healthz", ""))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"fake"`) {
		t.Fatalf("unexpected health response: %d %s", response.Code, response.Body.String())
	}
}

func TestHomePrioritizesEngineDefaultVoice(t *testing.T) {
	t.Parallel()
	engine := &fakeEngine{
		available:    true,
		defaultVoice: "second",
		voices: []tts.Voice{
			{ID: "first", Name: "First", Locale: "zh-CN"},
			{ID: "second", Name: "Second", Locale: "zh-CN"},
		},
	}
	handler := newTestHandler(t, tts.NewService(engine))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newTestRequest(http.MethodGet, "/", ""))
	body := response.Body.String()
	second := strings.Index(body, `value="second"`)
	first := strings.Index(body, `value="first"`)
	if second < 0 || first < 0 || second > first {
		t.Fatalf("default voice was not prioritized: second=%d first=%d", second, first)
	}
}

func TestVoicesReturnsFallbackWithWarning(t *testing.T) {
	t.Parallel()
	engine := &fakeEngine{
		available: true,
		voices:    []tts.Voice{{ID: "fallback", Name: "Fallback"}},
		voicesErr: errors.New("upstream unavailable"),
	}
	handler := newTestHandler(t, tts.NewService(engine))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newTestRequest(http.MethodGet, "/api/voices?engine=fake", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Voice-Source") != "fallback" ||
		!strings.Contains(response.Body.String(), "upstream unavailable") ||
		!strings.Contains(response.Body.String(), `"fallback"`) {
		t.Fatalf("unexpected fallback response: %#v %s", response.Header(), response.Body.String())
	}
}

func TestHTTPMethodsAndSecurityHeaders(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, tts.NewService(&fakeEngine{available: true}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, newTestRequest(http.MethodGet, "/api/synthesize", ""))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %#v", response.Header())
	}

	static := httptest.NewRecorder()
	handler.ServeHTTP(static, newTestRequest(http.MethodGet, "/static/app.js", ""))
	if static.Code != http.StatusOK || !strings.Contains(static.Body.String(), "synthesize") {
		t.Fatalf("static asset response: %d", static.Code)
	}
}

func TestSynthesizeRejectsUnknownEngine(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, tts.NewService(&fakeEngine{available: true}))
	form := url.Values{"engine": {"missing"}, "voice": {"test"}, "text": {"hello"}, "speed": {"1"}}
	request := newTestRequest(http.MethodPost, "/api/synthesize", form.Encode())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

func newTestHandler(t *testing.T, service *tts.Service) http.Handler {
	t.Helper()
	app, err := NewApp()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.NewConfig()
	cfg.Put("web.server.port", testPort)
	builder := wf.NewBuilder(cfg)
	builder.Service(ttssvc.NewWithService(service))
	builder.Filter(NewRequestFilter())
	builder.Rest(app)
	handler := builder.Build().GetHandler()
	if handler == nil {
		t.Fatal("Go Web Frame returned a nil handler")
	}
	return handler
}

func newTestRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = fmt.Sprintf("127.0.0.1:%d", testPort)
	return request
}
