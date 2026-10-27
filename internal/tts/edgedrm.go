package tts

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/bits"
	"strings"
	"time"
)

const (
	edgeTrustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeFiletimeEpoch      = int64(11644473600)
	edgeTokenWindow        = int64(300)
)

var (
	sha256Initial = [8]uint32{
		0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
		0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
	}
	edgeSHA256State = [8]uint32{
		0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A,
		0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2,
	}
	sha256Schedule = [14]uint32{
		0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA,
		0xA622EE8C, 0xD1DD9491, 0xC0DD25A0, 0xF9E70AB6,
		0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33,
		0xD258B060, 0x72766564,
	}
	sha256RoundConstants = [64]uint32{
		0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5,
		0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
		0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3,
		0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
		0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC,
		0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
		0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7,
		0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
		0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13,
		0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
		0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3,
		0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
		0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5,
		0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
		0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208,
		0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
	}
	cs64Coefficients = [14][2]uint32{
		{0xE82E91B1, 0xB32700D0}, {0x44DE24C6, 0x88DFF51D},
		{0x3BEB65BC, 0x29514038}, {0xC89C5933, 0xB328CBDF},
		{0x8D1EC211, 0xD405B4D5}, {0x314EF5D5, 0x26E55F3C},
		{0xE92491B1, 0x607801C6}, {0xF0A5B9D5, 0x82B2C1FB},
		{0x06064CF3, 0xFA51A3DA}, {0xDA79FA60, 0x207CC464},
		{0xBED20560, 0x3C3C74F3}, {0x89F9C9F8, 0xD2D14B61},
		{0x35FA38C6, 0xBB04A2C1}, {0xCB49E3C6, 0xC5626167},
	}
)

func generateEdgeToken(now time.Time) string {
	ticks := now.UTC().Unix() + edgeFiletimeEpoch
	ticks -= ticks % edgeTokenWindow
	ticks *= 10_000_000
	data := []byte(fmt.Sprintf("%d%s", ticks, edgeTrustedClientToken))
	digest := edgeSHA256Digest(data)
	first, second := edgeCS64Digest(data, digest)
	var packed [8]byte
	binary.LittleEndian.PutUint32(packed[0:4], first)
	binary.LittleEndian.PutUint32(packed[4:8], second)
	return strings.ToUpper(hex.EncodeToString(packed[:])) + "-" +
		strings.ToUpper(hex.EncodeToString(digest))
}

func edgeSHA256Digest(data []byte) []byte {
	// The Edge token algorithm intentionally starts the SHA-256 schedule at
	// round 14 with a precomputed state. The result is still a valid digest of
	// the complete byte stream, but the unusual construction is part of the
	// service's current token format.
	state := edgeSHA256Init(data[:7])
	length := 57 + len(data)
	paddingLength := ((55-length)%64 + 64) % 64
	padded := make([]byte, 0, len(data)+paddingLength+9)
	padded = append(padded, data[7:]...)
	padded = append(padded, 0x80)
	padded = append(padded, make([]byte, paddingLength)...)
	var lengthBytes [8]byte
	binary.BigEndian.PutUint64(lengthBytes[:], uint64(length*8))
	padded = append(padded, lengthBytes[:]...)

	for offset := 0; offset < len(padded); offset += 64 {
		var words [64]uint32
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(padded[offset+i*4 : offset+i*4+4])
		}
		expandSHA256Schedule(&words)
		working := sha256Rounds(state, words, 0)
		for i := range state {
			state[i] += working[i]
		}
	}
	result := make([]byte, 32)
	for i, word := range state {
		binary.BigEndian.PutUint32(result[i*4:], word)
	}
	return result
}

func edgeSHA256Init(data []byte) [8]uint32 {
	if len(data) < 7 {
		zero := sha256.Sum256(data)
		var state [8]uint32
		for i := range state {
			state[i] = binary.BigEndian.Uint32(zero[i*4:])
		}
		return state
	}
	var words [64]uint32
	words[14] = uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	words[15] = binary.BigEndian.Uint32(data[3:7])
	for index := 16; index <= 29; index++ {
		value := sha256Schedule[index-16]
		if index-2 >= 14 {
			sigma := sha256Sigma1(words[index-2])
			if index == 16 {
				sigma ^= 0x000B92C0
			}
			value += sigma
		}
		if index-7 >= 14 {
			value += words[index-7]
		}
		if index-15 >= 14 {
			sigma := sha256Sigma0(words[index-15])
			if index == 29 {
				sigma ^= 0x059C0B80
			}
			value += sigma
		}
		words[index] = value
	}
	words[30] = sha256Sigma1(words[28]) + words[23] + sha256Sigma0(words[15]) +
		words[14] + 0xC9DC06A7 - sha256RoundConstants[14]
	expandSHA256ScheduleFrom(&words, 31)
	working := sha256Rounds(edgeSHA256State, words, 14)
	var state [8]uint32
	for i := range state {
		state[i] = sha256Initial[i] + working[i]
	}
	return state
}

func sha256Rounds(state [8]uint32, words [64]uint32, firstRound int) [8]uint32 {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3],
		state[4], state[5], state[6], state[7]
	for index := firstRound; index < 64; index++ {
		constant := sha256RoundConstants[index]
		if firstRound == 14 && index == 14 {
			constant = 0xC9DC06A7
		}
		sum1 := bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)
		choice := (e & f) ^ (^e & g)
		temp1 := h + sum1 + choice + constant + words[index]
		sum0 := bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)
		majority := (a & b) ^ (a & c) ^ (b & c)
		temp2 := sum0 + majority
		h, g, f, e, d, c, b, a = g, f, e, d+temp1, c, b, a, temp1+temp2
	}
	return [8]uint32{a, b, c, d, e, f, g, h}
}

func expandSHA256Schedule(words *[64]uint32) {
	expandSHA256ScheduleFrom(words, 16)
}

func expandSHA256ScheduleFrom(words *[64]uint32, first int) {
	for index := first; index < 64; index++ {
		words[index] = sha256Sigma1(words[index-2]) + words[index-7] +
			sha256Sigma0(words[index-15]) + words[index-16]
	}
}

func sha256Sigma0(value uint32) uint32 {
	return bits.RotateLeft32(value, -7) ^ bits.RotateLeft32(value, -18) ^ (value >> 3)
}

func sha256Sigma1(value uint32) uint32 {
	return bits.RotateLeft32(value, -17) ^ bits.RotateLeft32(value, -19) ^ (value >> 10)
}

func edgeCS64Digest(data, digest []byte) (uint32, uint32) {
	keyA := binary.LittleEndian.Uint32(digest[0:4]) | 1
	keyB := binary.LittleEndian.Uint32(digest[4:8]) | 1
	factors := [2]uint32{keyA * 0x82893933, keyB * 0x82893933}
	terms := make([]uint32, 0, 32)
	offsets := make([]uint32, 0, 32)
	for index, coefficient := range cs64Coefficients {
		terms = append(terms, coefficient[0]*factors[index%2])
		offsets = append(offsets, coefficient[1])
	}

	buffer := append([]byte{0}, data...)
	for len(buffer)%4 != 0 {
		buffer = append(buffer, 0)
	}
	for index := 0; index < len(buffer); index += 4 {
		word := binary.LittleEndian.Uint32(buffer[index : index+4])
		key := keyA
		offsetKey := uint32(0xEF0569FB)
		if (index/4)%2 == 1 {
			key = keyB
			offsetKey = 0xC31713DB
		}
		term := word * key
		offset := rotate16(word) * offsetKey
		if index == 0 {
			term += 0xF2F90B1A * factors[0]
			offset += 0x0B1A0000
		}
		terms = append(terms, term)
		offsets = append(offsets, offset)
	}
	if len(terms)%2 != 0 {
		terms = append(terms, 0)
		offsets = append(offsets, 0)
	}

	var swapAcc, swapLast, state, acc uint32
	for index := 0; index < len(terms); index += 2 {
		x, y := terms[index], terms[index+1]
		t := rotate16(offsets[index] + x)
		q := t * 0x76F50F34
		swapAcc += rotate16(q)*0xC3EFEA97 + q
		t = rotate16(offsets[index+1] + y)
		q = t * 0x1881CE3C
		swapLast = rotate16(q)*0x35BD1EC9 + q
		swapAcc += swapLast

		state = state*keyA + x
		state = rotate16(state) * 0xCF98B111
		state = rotate16(state) * 0x87085B9F
		state = rotate16(state) * 0x12CEB96D
		state = rotate16(state) * 0x257E1D83
		acc += state
		t = state*keyB + y
		t = rotate16(t) * 0xA27416F5
		t = rotate16(t) * 0xD38396FF
		state = rotate16(rotate16(t)*0x7C932B89) * 0xBFA49F69
		acc += state
	}
	return swapLast ^ state, swapAcc ^ acc
}

func rotate16(value uint32) uint32 {
	return bits.RotateLeft32(value, 16)
}
