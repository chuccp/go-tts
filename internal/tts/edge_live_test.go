package tts

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestEdgeLiveIntegration(t *testing.T) {
	if os.Getenv("TTS_NETWORK_INTEGRATION") != "1" {
		t.Skip("set TTS_NETWORK_INTEGRATION=1 to call the Microsoft Edge TTS service")
	}
	voice := os.Getenv("TTS_EDGE_VOICE")
	if voice == "" {
		voice = "en-US-AvaMultilingualNeural"
	}
	text := os.Getenv("TTS_EDGE_TEXT")
	if text == "" {
		text = "This is a live Edge TTS integration test."
	}
	engine := NewEdgeEngine(EdgeConfig{DefaultVoice: voice})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	voices, err := engine.Voices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) < 100 {
		t.Fatalf("only %d Edge voices were returned", len(voices))
	}
	audio, err := engine.Synthesize(ctx, text, voice, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(audio.Data) < 1024 || audio.MIMEType != "audio/mpeg" {
		t.Fatalf("unexpected Edge audio: %d bytes, %s", len(audio.Data), audio.MIMEType)
	}
	if audio.Data[0] != 0xFF || audio.Data[1]&0xE0 != 0xE0 {
		t.Fatalf("response is not an MPEG audio stream: %x %x", audio.Data[0], audio.Data[1])
	}
}
