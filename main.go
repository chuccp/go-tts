package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cao/go-tts/internal/tts"
	appweb "github.com/cao/go-tts/internal/web"
	wf "github.com/chuccp/go-web-frame"
	"github.com/chuccp/go-web-frame/config"
	framelog "github.com/chuccp/go-web-frame/log"
)

func main() {
	var (
		addr         string
		kokoroModel  string
		kokoroVoices string
		kittenModel  string
		kittenVoices string
		kittenConfig string
		onnxLib      string
		espeakPath   string
		edgeVoice    string
		edgeProxy    string
		maxTextRunes int
	)

	flag.StringVar(&addr, "addr", envDefault("TTS_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	flag.StringVar(&kokoroModel, "kokoro-model", envDefault("KOKORO_MODEL", filepath.Join("models", "kokoro", "kokoro-v0_19.onnx")), "Kokoro ONNX model path")
	flag.StringVar(&kokoroVoices, "kokoro-voices", envDefault("KOKORO_VOICES", filepath.Join("models", "kokoro", "voices-v1.0.bin")), "Kokoro voice NPZ/NPY/BIN file or directory")
	flag.StringVar(&kittenModel, "kitten-model", envDefault("KITTEN_MODEL", filepath.Join("models", "kitten", "kitten_tts_nano_v0_8.onnx")), "Kitten TTS ONNX model path")
	flag.StringVar(&kittenVoices, "kitten-voices", envDefault("KITTEN_VOICES", filepath.Join("models", "kitten", "voices.npz")), "Kitten voice NPZ/NPY/BIN file or directory")
	flag.StringVar(&kittenConfig, "kitten-config", envDefault("KITTEN_CONFIG", filepath.Join("models", "kitten", "config.json")), "Kitten model config path")
	flag.StringVar(&onnxLib, "onnx-lib", os.Getenv("ONNXRUNTIME_LIB"), "ONNX Runtime shared library path; auto-detected when empty")
	flag.StringVar(&espeakPath, "espeak", "", "espeak-ng executable path; auto-detected when empty")
	flag.StringVar(&edgeVoice, "edge-voice", envDefault("EDGE_VOICE", "zh-CN-XiaoxiaoNeural"), "initial Edge TTS voice")
	flag.StringVar(&edgeProxy, "edge-proxy", os.Getenv("EDGE_PROXY"), "optional HTTP proxy for Edge TTS")
	flag.IntVar(&maxTextRunes, "max-chars", 5000, "maximum input length in characters")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)
	runtimeLib := tts.FindONNXRuntime(onnxLib)
	kokoro := tts.NewKokoroEngine(tts.KokoroConfig{
		ModelPath:  kokoroModel,
		VoicesPath: kokoroVoices,
		RuntimeLib: runtimeLib,
		EspeakPath: espeakPath,
	})
	kitten := tts.NewKittenEngine(tts.KittenConfig{
		ModelPath:  kittenModel,
		VoicesPath: kittenVoices,
		ConfigPath: kittenConfig,
		RuntimeLib: runtimeLib,
		EspeakPath: espeakPath,
	})
	edge := tts.NewEdgeEngine(tts.EdgeConfig{
		DefaultVoice: edgeVoice,
		ProxyURL:     edgeProxy,
	})
	service := tts.NewService(edge, kokoro, kitten)
	service.SetMaxTextRunes(maxTextRunes)
	defer func() {
		if err := service.Close(); err != nil {
			logger.Printf("close engines: %v", err)
		}
	}()

	for _, engine := range []tts.Engine{edge, kokoro, kitten} {
		if err := engine.Available(); err != nil {
			logger.Printf("%s unavailable: %v", engine.Name(), err)
		} else {
			voices, _ := engine.Voices(context.Background())
			logger.Printf("%s ready (%d voices)", engine.Name(), len(voices))
		}
	}

	webApp, err := appweb.NewApp(service, logger)
	if err != nil {
		logger.Fatalf("initialize web application: %v", err)
	}
	port, err := addrPort(addr)
	if err != nil {
		logger.Fatalf("invalid listen address %q: %v", addr, err)
	}
	config := config.NewConfig()
	config.Put("web.server.port", port)
	config.Put("web.server.read_header_timeout", 5)
	config.Put("web.server.read_timeout", 30)
	framelog.InitLogger(&framelog.Config{Level: "info"})
	builder := wf.NewBuilder(config)
	builder.Filter(appweb.NewRequestFilter(logger))
	builder.Rest(webApp)
	handler := builder.Build().GetHandler()
	if handler == nil {
		logger.Fatal("initialize Go Web Frame handler")
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		logger.Print("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	logger.Printf("VoxLab listening on http://%s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("serve: %v", err)
	}
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func addrPort(addr string) (int, error) {
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			portText = strings.TrimPrefix(addr, ":")
		} else {
			return 0, err
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return port, nil
}
