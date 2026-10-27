package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cao/go-tts/internal/ttssvc"
	appweb "github.com/cao/go-tts/internal/web"
	wf "github.com/chuccp/go-web-frame"
	"github.com/chuccp/go-web-frame/config"
)

func main() {
	cfg, err := config.LoadSingleFileConfig("application.yml")
	if err != nil {
		log.Fatalf("load application.yml: %v", err)
	}
	app, err := appweb.NewApp()
	if err != nil {
		log.Fatalf("initialize web application: %v", err)
	}
	ttsService := ttssvc.New()
	defer func() {
		if err := ttsService.Close(); err != nil {
			log.Printf("close TTS service: %v", err)
		}
	}()

	builder := wf.NewBuilder(cfg)
	builder.Service(ttsService)
	builder.Filter(appweb.NewRequestFilter())
	builder.Rest(app)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := builder.Build().Run(ctx); err != nil {
		log.Printf("web application stopped: %v", err)
	}
}
