package tts

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalEngineIntegration(t *testing.T) {
	if os.Getenv("TTS_INTEGRATION") != "1" {
		t.Skip("set TTS_INTEGRATION=1 to run local ONNX engine tests")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	runtimeLib := filepath.Join(root, "runtime", "onnxruntime.dll")
	if runtime.GOOS != "windows" {
		runtimeLib = filepath.Join(root, "runtime", "libonnxruntime.so")
	}
	espeak := filepath.Join(root, "runtime", "espeak-ng", "espeak-ng.exe")
	if runtime.GOOS != "windows" {
		espeak = filepath.Join(root, "runtime", "espeak-ng", "espeak-ng")
	}
	paths := []string{
		runtimeLib,
		espeak,
		filepath.Join(root, "models", "kokoro", "kokoro-v0_19.onnx"),
		filepath.Join(root, "models", "kokoro", "voices-v1.0.bin"),
		filepath.Join(root, "models", "kitten", "kitten_tts_nano_v0_8.onnx"),
		filepath.Join(root, "models", "kitten", "voices.npz"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("integration asset is missing: %s", path)
		}
	}

	kokoro := NewKokoroEngine(KokoroConfig{
		ModelPath:  filepath.Join(root, "models", "kokoro", "kokoro-v0_19.onnx"),
		VoicesPath: filepath.Join(root, "models", "kokoro", "voices-v1.0.bin"),
		RuntimeLib: runtimeLib,
		EspeakPath: espeak,
	})
	kitten := NewKittenEngine(KittenConfig{
		ModelPath:  filepath.Join(root, "models", "kitten", "kitten_tts_nano_v0_8.onnx"),
		VoicesPath: filepath.Join(root, "models", "kitten", "voices.npz"),
		ConfigPath: filepath.Join(root, "models", "kitten", "config.json"),
		RuntimeLib: runtimeLib,
		EspeakPath: espeak,
	})
	if err := kokoro.Available(); err != nil {
		t.Fatalf("Kokoro initialization failed: %v", err)
	}
	if err := kitten.Available(); err != nil {
		t.Fatalf("Kitten initialization failed: %v", err)
	}
	service := NewService(kokoro, kitten)
	defer service.Close()

	tests := []struct {
		name   string
		engine string
		voice  string
		text   string
		speed  float64
	}{
		{
			name:   "kokoro",
			engine: "kokoro",
			voice:  "af_heart",
			text:   "Hello from the Kokoro integration test.",
			speed:  1.2,
		},
		{
			name:   "kitten",
			engine: "kitten",
			voice:  "Bella",
			text:   "Hello from the Kitten integration test.",
			speed:  0.8,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			audio, err := service.Synthesize(
				context.Background(),
				test.engine,
				test.voice,
				test.text,
				test.speed,
			)
			if err != nil {
				t.Fatal(err)
			}
			validateNonSilentWAV(t, audio.Data)
		})
	}
}

func validateNonSilentWAV(t *testing.T, data []byte) {
	t.Helper()
	if len(data) < 44 {
		t.Fatalf("WAV is too short: %d bytes", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatal("invalid WAV header")
	}
	if got := binary.LittleEndian.Uint32(data[24:28]); got != 24000 {
		t.Fatalf("sample rate = %d, want 24000", got)
	}
	dataSize := int(binary.LittleEndian.Uint32(data[40:44]))
	if dataSize <= 0 || dataSize+44 > len(data) {
		t.Fatalf("invalid WAV data size: %d (payload %d)", dataSize, len(data)-44)
	}
	var peak int
	for offset := 44; offset+1 < 44+dataSize; offset += 2 {
		sample := int(int16(binary.LittleEndian.Uint16(data[offset:])))
		if sample < 0 {
			sample = -sample
		}
		if sample > peak {
			peak = sample
		}
	}
	if peak < 100 {
		t.Fatalf("audio appears silent; peak amplitude = %d", peak)
	}
}
