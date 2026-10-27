package tts

import (
	"bytes"
	"encoding/binary"
	"math"
)

func Float32ToWAV(samples []float32, sampleRate int) []byte {
	const bitsPerSample = 16
	dataSize := len(samples) * (bitsPerSample / 8)
	var out bytes.Buffer
	out.Grow(44 + dataSize)

	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+dataSize))
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(&out, binary.LittleEndian, uint16(2))
	_ = binary.Write(&out, binary.LittleEndian, uint16(bitsPerSample))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(dataSize))

	for _, sample := range samples {
		sample = float32(math.Max(-1, math.Min(1, float64(sample))))
		_ = binary.Write(&out, binary.LittleEndian, int16(sample*math.MaxInt16))
	}
	return out.Bytes()
}

func appendSilence(samples []float32, sampleRate int, milliseconds int) []float32 {
	count := sampleRate * milliseconds / 1000
	if count <= 0 {
		return samples
	}
	return append(samples, make([]float32, count)...)
}
