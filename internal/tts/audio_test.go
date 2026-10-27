package tts

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestFloat32ToWAV(t *testing.T) {
	t.Parallel()
	samples := []float32{-2, -0.5, 0, 0.5, 2}
	wav := Float32ToWAV(samples, 24000)
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" ||
		string(wav[12:16]) != "fmt " || string(wav[36:40]) != "data" {
		t.Fatal("invalid WAV header")
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != 24000 {
		t.Fatalf("sample rate = %d", got)
	}
	if got := binary.LittleEndian.Uint16(wav[22:24]); got != 1 {
		t.Fatalf("channel count = %d", got)
	}
	if got := binary.LittleEndian.Uint16(wav[34:36]); got != 16 {
		t.Fatalf("bits per sample = %d", got)
	}
	want := []int16{-math.MaxInt16, -16383, 0, 16383, math.MaxInt16}
	for index, expected := range want {
		got := int16(binary.LittleEndian.Uint16(wav[44+index*2:]))
		if got != expected {
			t.Errorf("sample %d = %d, want %d", index, got, expected)
		}
	}
}

func TestAppendSilence(t *testing.T) {
	t.Parallel()
	samples := appendSilence([]float32{1}, 24000, 100)
	if len(samples) != 2401 {
		t.Fatalf("sample count = %d, want 2401", len(samples))
	}
	if samples[0] != 1 || samples[2400] != 0 {
		t.Fatalf("unexpected samples: first=%v last=%v", samples[0], samples[2400])
	}
}
