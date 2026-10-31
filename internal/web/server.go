package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cao/go-tts/internal/clickguard"
	"github.com/cao/go-tts/internal/langdetect"
	"github.com/cao/go-tts/internal/tts"
	"github.com/cao/go-tts/internal/ttssvc"
	"github.com/chuccp/go-web-frame/core"
	framelog "github.com/chuccp/go-web-frame/log"
	frameweb "github.com/chuccp/go-web-frame/web"
	"go.uber.org/zap"
)

//go:embed templates/*.html static/*
var assets embed.FS

type App struct {
	core.IService
	service   *ttssvc.Service
	captcha   *clickguard.Manager
	templates *template.Template
	static    http.FileSystem
	edgeGate  *requestGate
}

const clientCookieName = "voxlab_client"

type pageData struct {
	Engines          []tts.EngineStatus
	DefaultEngine    string
	DefaultVoices    []tts.Voice
	DefaultLanguage  string
	DefaultLanguages []languageOption
	MaxTextRunes     int
	EngineDataJSON   template.JS
}

type languageOption struct {
	Code  string
	Name  string
	Count int
}

type clientEngine struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Local         bool        `json:"local"`
	Available     bool        `json:"available"`
	SupportsSpeed bool        `json:"supportsSpeed"`
	MinSpeed      float64     `json:"minSpeed"`
	MaxSpeed      float64     `json:"maxSpeed"`
	DefaultVoice  string      `json:"defaultVoice,omitempty"`
	Error         string      `json:"error,omitempty"`
	Voices        []tts.Voice `json:"voices"`
}

func NewApp() (*App, error) {
	templates, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("open static assets: %w", err)
	}
	return &App{
		templates: templates,
		static:    http.FS(staticFS),
	}, nil
}

// Init registers the application through Go Web Frame's controller system.
func (a *App) Init(ctx *core.Context) error {
	a.service = core.MustService[*ttssvc.Service](ctx)
	cfg := ctx.GetConfig()
	captchaConfig := clickguard.DefaultConfig()
	captchaConfig.Enabled = cfg.GetBoolOrDefault("captcha.enabled", captchaConfig.Enabled)
	captchaConfig.Limit = cfg.GetIntOrDefault("captcha.limit", captchaConfig.Limit)
	periodSeconds := cfg.GetIntOrDefault("captcha.period", int(captchaConfig.Window/time.Second))
	if periodSeconds <= 0 {
		return fmt.Errorf("captcha.period must be a positive number of seconds")
	}
	captchaConfig.Window = time.Duration(periodSeconds) * time.Second
	captcha, err := clickguard.New(captchaConfig)
	if err != nil {
		return err
	}
	a.captcha = captcha
	a.edgeGate = newRequestGate(
		cfg.GetIntOrDefault("tts.edge.max_concurrency", 4),
		cfg.GetIntOrDefault("tts.edge.max_queue", 20),
	)
	ctx.Handle(http.MethodGet, "/", a.home)
	ctx.Handle(http.MethodHead, "/", a.home)
	ctx.Any("/api/voices", a.voices)
	ctx.Handle(http.MethodGet, "/api/edge/session", a.edgeSession)
	ctx.Any("/api/detect-language", a.detectLanguage)
	ctx.Any("/api/synthesize", a.synthesize)
	ctx.Handle(http.MethodGet, "/api/captcha/challenge", a.captchaChallenge)
	ctx.Handle(http.MethodPost, "/api/captcha/verify", a.verifyCaptcha)
	ctx.Any("/healthz", a.health)
	ctx.StaticFs("/static", a.static)
	return nil
}

func (a *App) edgeSession(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodGet {
		return nil, methodNotAllowed(req, http.MethodGet)
	}
	header := req.Response().Header()
	header.Set("Cache-Control", "no-store")
	writeJSON(req, http.StatusOK, tts.NewEdgeDirectConnection(time.Now()))
	return nil, nil
}

func (a *App) home(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodGet && req.Request().Method != http.MethodHead {
		return nil, methodNotAllowed(req, http.MethodGet, http.MethodHead)
	}
	ctx, cancel := context.WithTimeout(req.Ctx(), 3*time.Second)
	defer cancel()
	statuses := a.service.Statuses(ctx)
	defaultEngine := "kitten"
	found := false
	for _, status := range statuses {
		if status.ID == defaultEngine && status.Available {
			found = true
			break
		}
	}
	if !found {
		for _, status := range statuses {
			if status.Available {
				defaultEngine = status.ID
				found = true
				break
			}
		}
	}
	if !found && len(statuses) > 0 {
		defaultEngine = statuses[0].ID
	}
	var defaultVoices []tts.Voice
	defaultLanguage := ""
	for _, status := range statuses {
		if status.ID == defaultEngine {
			defaultVoices = prioritizeVoice(status.Voices, a.service.DefaultVoice(defaultEngine))
			for _, voice := range defaultVoices {
				if voice.ID == a.service.DefaultVoice(defaultEngine) {
					defaultLanguage = voice.LanguageCode
					break
				}
			}
			if defaultLanguage == "" && len(defaultVoices) > 0 {
				defaultLanguage = defaultVoices[0].LanguageCode
			}
			break
		}
	}
	defaultRoles := voicesForLanguage(defaultVoices, defaultLanguage)
	clientEngines := make([]clientEngine, 0, len(statuses))
	for _, status := range statuses {
		clientEngines = append(clientEngines, clientEngine{
			ID:            status.ID,
			Name:          status.Name,
			Description:   status.Description,
			Local:         status.Local,
			Available:     status.Available,
			SupportsSpeed: status.SupportsSpeed,
			MinSpeed:      status.MinSpeed,
			MaxSpeed:      status.MaxSpeed,
			DefaultVoice:  status.DefaultVoice,
			Error:         status.Error,
			Voices:        status.Voices,
		})
	}
	engineJSON, err := json.Marshal(clientEngines)
	if err != nil {
		return nil, frameweb.NewInternalError().WithError(err)
	}
	data := pageData{
		Engines:          statuses,
		DefaultEngine:    defaultEngine,
		DefaultVoices:    defaultRoles,
		DefaultLanguage:  defaultLanguage,
		DefaultLanguages: buildLanguageOptions(defaultVoices, defaultLanguage),
		MaxTextRunes:     a.service.MaxTextRunes(),
		EngineDataJSON:   template.JS(engineJSON),
	}
	var body bytes.Buffer
	if err := a.templates.ExecuteTemplate(&body, "index.html", data); err != nil {
		return nil, frameweb.NewInternalError().WithError(err)
	}
	header := req.Response().Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	if req.Request().Method == http.MethodHead {
		req.Response().WriteHeader(http.StatusOK)
		return nil, nil
	}
	req.GinContext().Data(http.StatusOK, "text/html; charset=utf-8", body.Bytes())
	return nil, nil
}

func (a *App) detectLanguage(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodPost {
		return nil, methodNotAllowed(req, http.MethodPost)
	}
	req.Request().Body = http.MaxBytesReader(req.GinContext().Writer, req.Request().Body, 64<<10)
	if err := req.Request().ParseForm(); err != nil {
		writeJSON(req, http.StatusBadRequest, map[string]any{"error": "invalid form data"})
		return nil, nil
	}
	text := strings.TrimSpace(req.GetFormParam("text"))
	if text == "" {
		writeJSON(req, http.StatusBadRequest, map[string]any{"error": "text is required"})
		return nil, nil
	}
	writeJSON(req, http.StatusOK, langdetect.Detect(text))
	return nil, nil
}

func (a *App) voices(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodGet {
		return nil, methodNotAllowed(req, http.MethodGet)
	}
	engineID := strings.TrimSpace(req.Query("engine"))
	if engineID == "" {
		writeJSON(req, http.StatusBadRequest, map[string]any{"error": "engine is required"})
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(req.Ctx(), 20*time.Second)
	defer cancel()
	voices, err := a.service.Voices(ctx, engineID)
	if err != nil && len(voices) == 0 {
		writeJSON(req, http.StatusBadGateway, map[string]any{
			"warning": err.Error(),
			"voices":  []tts.Voice{},
		})
		return nil, nil
	}
	if len(voices) == 0 {
		writeJSON(req, http.StatusBadGateway, map[string]any{"error": "no voices available"})
		return nil, nil
	}
	header := req.Response().Header()
	header.Set("Cache-Control", "private, max-age=300")
	response := map[string]any{"voices": voices}
	if err != nil {
		header.Set("X-Voice-Source", "fallback")
		response["warning"] = err.Error()
	}
	writeJSON(req, http.StatusOK, response)
	return nil, nil
}

func (a *App) synthesize(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodPost {
		return nil, methodNotAllowed(req, http.MethodPost)
	}
	req.Request().Body = http.MaxBytesReader(req.GinContext().Writer, req.Request().Body, 64<<10)
	if err := req.Request().ParseForm(); err != nil {
		writeJSON(req, http.StatusBadRequest, map[string]any{"error": "invalid form data"})
		return nil, nil
	}
	speed, err := strconv.ParseFloat(defaultString(req.GetFormParam("speed"), "1.0"), 64)
	if err != nil {
		writeJSON(req, http.StatusBadRequest, map[string]any{"error": "invalid speed"})
		return nil, nil
	}
	allowed, err := a.allowSynthesis(req)
	if err != nil {
		return nil, frameweb.NewInternalError().WithError(err)
	}
	if !allowed {
		return nil, nil
	}
	engineID := req.GetFormParam("engine")
	voice := req.GetFormParam("voice")
	text := req.GetFormParam("text")
	if normalizedEngineID(engineID) == "edge" {
		if err := a.edgeGate.acquire(req.Ctx()); err != nil {
			header := req.Response().Header()
			header.Set("Retry-After", "2")
			status := http.StatusTooManyRequests
			message := "Edge TTS is busy; retry shortly"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusRequestTimeout
				message = "request canceled while waiting for Edge TTS"
			}
			writeJSON(req, status, map[string]any{"error": message})
			return nil, nil
		}
		defer a.edgeGate.release()
	}
	audio, err := a.service.Synthesize(req.Ctx(), engineID, voice, text, speed)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, tts.ErrEmptyText) ||
			errors.Is(err, tts.ErrTextTooLong) ||
			errors.Is(err, tts.ErrBadSpeed) ||
			strings.Contains(err.Error(), "unknown engine") ||
			strings.Contains(err.Error(), "voice is required") ||
			strings.Contains(err.Error(), "not available") {
			status = http.StatusBadRequest
		}
		framelog.Error("synthesis failed",
			zap.String("engine", engineID),
			zap.String("voice", voice),
			zap.Error(err),
		)
		writeJSON(req, status, map[string]any{"error": err.Error()})
		return nil, nil
	}
	filename := fmt.Sprintf("tts-%s-%s.%s", engineID, time.Now().Format("20060102-150405"), audio.Extension)
	header := req.Response().Header()
	header.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	header.Set("Cache-Control", "no-store")
	header.Set("X-TTS-Engine", engineID)
	header.Set("X-TTS-Voice", voice)
	req.GinContext().Data(http.StatusOK, audio.MIMEType, audio.Data)
	return nil, nil
}

func normalizedEngineID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (a *App) allowSynthesis(req *frameweb.Request) (bool, error) {
	if !a.captcha.Enabled() {
		return true, nil
	}
	ip, client := a.captchaClient(req)
	if ticket := strings.TrimSpace(req.GetFormParam("captchaTicket")); ticket != "" &&
		a.captcha.ConsumeTicket(ticket, client) {
		a.captcha.Reset(ip, client)
		a.captcha.Allow(ip, client)
		return true, nil
	}
	if a.captcha.Allow(ip, client) {
		return true, nil
	}
	challenge, err := a.captcha.NewChallenge()
	if err != nil {
		return false, err
	}
	a.writeCaptchaRequired(req, challenge)
	return false, nil
}

func (a *App) captchaChallenge(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodGet {
		return nil, methodNotAllowed(req, http.MethodGet)
	}
	a.captchaClient(req)
	challenge, err := a.captcha.NewChallenge()
	if err != nil {
		return nil, frameweb.NewInternalError().WithError(err)
	}
	header := req.Response().Header()
	header.Set("Cache-Control", "no-store")
	writeJSON(req, http.StatusOK, map[string]any{
		"captchaRequired": true,
		"captcha":         challenge,
		"limit":           a.captcha.Limit(),
		"windowSeconds":   int(a.captcha.Window() / time.Second),
	})
	return nil, nil
}

func (a *App) verifyCaptcha(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodPost {
		return nil, methodNotAllowed(req, http.MethodPost)
	}
	req.Request().Body = http.MaxBytesReader(req.GinContext().Writer, req.Request().Body, 32<<10)
	if err := req.Request().ParseForm(); err != nil {
		writeJSON(req, http.StatusBadRequest, map[string]any{"error": "invalid form data"})
		return nil, nil
	}
	_, client := a.captchaClient(req)
	result, err := a.captcha.Verify(
		req.GetFormParam("clickCode"),
		req.GetFormParam("clickXs"),
		req.GetFormParam("clickYs"),
		client,
	)
	if err != nil {
		challenge, challengeErr := a.captcha.NewChallenge()
		if challengeErr != nil {
			return nil, frameweb.NewInternalError().WithError(challengeErr)
		}
		writeJSON(req, http.StatusBadRequest, map[string]any{
			"error":           "captcha verification failed",
			"captchaRequired": true,
			"captcha":         challenge,
		})
		return nil, nil
	}
	header := req.Response().Header()
	header.Set("Cache-Control", "no-store")
	writeJSON(req, http.StatusOK, result)
	return nil, nil
}

func (a *App) writeCaptchaRequired(req *frameweb.Request, challenge *clickguard.Challenge) {
	header := req.Response().Header()
	header.Set("Cache-Control", "no-store")
	writeJSON(req, http.StatusPreconditionRequired, map[string]any{
		"captchaRequired": true,
		"captcha":         challenge,
		"limit":           a.captcha.Limit(),
		"windowSeconds":   int(a.captcha.Window() / time.Second),
	})
}

func (a *App) captchaClient(req *frameweb.Request) (string, string) {
	client := strings.TrimSpace(req.Cookie().Get(clientCookieName))
	if !validClientToken(client) {
		raw := make([]byte, 24)
		if _, err := rand.Read(raw); err != nil {
			client = fmt.Sprintf("%d", time.Now().UnixNano())
		} else {
			client = base64.RawURLEncoding.EncodeToString(raw)
		}
		secure := req.Request().TLS != nil || strings.EqualFold(req.GetHeader("X-Forwarded-Proto"), "https")
		req.Cookie().Set(
			clientCookieName,
			client,
			frameweb.WithHttpOnly(true),
			frameweb.WithSameSite(http.SameSiteLaxMode),
			frameweb.WithSecure(secure),
			frameweb.WithPath("/"),
		)
	}
	ip := strings.TrimSpace(req.ClientIP())
	if ip == "" {
		ip = strings.TrimSpace(req.RemoteAddr())
	}
	return ip, client
}

func validClientToken(value string) bool {
	if len(value) < 24 || len(value) > 80 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func (a *App) health(req *frameweb.Request) (any, error) {
	if req.Request().Method != http.MethodGet {
		return nil, methodNotAllowed(req, http.MethodGet)
	}
	ctx, cancel := context.WithTimeout(req.Ctx(), 2*time.Second)
	defer cancel()
	type engineHealth struct {
		ID        string `json:"id"`
		Available bool   `json:"available"`
		Error     string `json:"error,omitempty"`
	}
	statuses := a.service.Statuses(ctx)
	result := make([]engineHealth, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, engineHealth{
			ID:        status.ID,
			Available: status.Available,
			Error:     status.Error,
		})
	}
	writeJSON(req, http.StatusOK, map[string]any{
		"status":  "ok",
		"engines": result,
	})
	return nil, nil
}

type RequestFilter struct {
	core.IFilter
}

func NewRequestFilter() *RequestFilter {
	return &RequestFilter{}
}

func (f *RequestFilter) Init(*core.Context) error {
	return nil
}

func (f *RequestFilter) Handle(next frameweb.FilterChain, req *frameweb.Request) (any, error) {
	started := time.Now()
	header := req.Response().Header()
	header.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; media-src 'self' blob: data:; style-src 'self'; script-src 'self'; connect-src 'self' wss://speech.platform.bing.com; base-uri 'none'; frame-ancestors 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")

	result, err := next.Next()
	path := req.URL().Path
	if path != "/static/app.js" && path != "/static/style.css" {
		framelog.Info("request",
			zap.String("method", req.Request().Method),
			zap.String("path", path),
			zap.Duration("duration", time.Since(started).Round(time.Millisecond)),
			zap.Error(err),
		)
	}
	return result, err
}

func methodNotAllowed(req *frameweb.Request, allowed ...string) error {
	req.Response().Header().Set("Allow", strings.Join(allowed, ", "))
	return frameweb.NewMethodNotAllowed()
}

func writeJSON(req *frameweb.Request, status int, value any) {
	req.Response().JSON(status, value)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func prioritizeVoice(voices []tts.Voice, preferred string) []tts.Voice {
	if preferred == "" {
		return voices
	}
	result := make([]tts.Voice, 0, len(voices))
	for _, voice := range voices {
		if voice.ID == preferred {
			result = append(result, voice)
			break
		}
	}
	for _, voice := range voices {
		if voice.ID != preferred {
			result = append(result, voice)
		}
	}
	return result
}

func voicesForLanguage(voices []tts.Voice, languageCode string) []tts.Voice {
	if languageCode == "" {
		return voices
	}
	result := make([]tts.Voice, 0, len(voices))
	for _, voice := range voices {
		if voice.LanguageCode == languageCode {
			result = append(result, voice)
		}
	}
	return result
}

func buildLanguageOptions(voices []tts.Voice, preferred string) []languageOption {
	counts := make(map[string]int)
	names := make(map[string]string)
	for _, voice := range voices {
		if voice.LanguageCode == "" {
			continue
		}
		counts[voice.LanguageCode]++
		if names[voice.LanguageCode] == "" {
			names[voice.LanguageCode] = voice.Language
		}
	}
	options := make([]languageOption, 0, len(counts))
	for code, count := range counts {
		options = append(options, languageOption{Code: code, Name: names[code], Count: count})
	}
	priority := map[string]int{"zh": 0, "en": 1, "ja": 2, "ko": 3}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Code == preferred {
			return true
		}
		if options[j].Code == preferred {
			return false
		}
		pi, oki := priority[options[i].Code]
		pj, okj := priority[options[j].Code]
		if oki && okj && pi != pj {
			return pi < pj
		}
		if oki != okj {
			return oki
		}
		return options[i].Name < options[j].Name
	})
	return options
}
