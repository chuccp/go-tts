package tts

import (
	"context"
	"errors"
	"fmt"
	"sync"

	g2p "github.com/androiddrew/go-g2p"
	"github.com/androiddrew/go-g2p/fallback/neural"
	ort "github.com/yalue/onnxruntime_go"
)

var (
	englishPhonemizerMu sync.Mutex
	englishPhonemizer   *g2p.Engine
)

func phonemizeEnglish(ctx context.Context, text string, british bool) (string, error) {
	engine, err := getEnglishPhonemizer()
	if err != nil {
		return "", err
	}
	dialect := g2p.US
	if british {
		dialect = g2p.GB
	}
	result, err := engine.Phonemize(ctx, g2p.Request{
		Text:    text,
		Dialect: dialect,
	})
	if err != nil {
		var incomplete *g2p.IncompleteError
		if errors.As(err, &incomplete) && result.Phonemes != "" {
			return result.Phonemes, nil
		}
		return "", fmt.Errorf("English phonemizer: %w", err)
	}
	return result.Phonemes, nil
}

func getEnglishPhonemizer() (*g2p.Engine, error) {
	englishPhonemizerMu.Lock()
	defer englishPhonemizerMu.Unlock()
	if englishPhonemizer != nil {
		return englishPhonemizer, nil
	}
	if !ort.IsInitialized() {
		return nil, errors.New("ONNX Runtime must be initialized before loading the English frontend")
	}
	fallback, err := neural.New(neural.Config{Threads: 1})
	if err != nil {
		return nil, fmt.Errorf("initialize English fallback: %w", err)
	}
	engine, err := g2p.New(g2p.Config{
		Fallback: fallback,
		Threads:  1,
	})
	if err != nil {
		_ = fallback.Close()
		return nil, fmt.Errorf("initialize English G2P: %w", err)
	}
	englishPhonemizer = engine
	return engine, nil
}
