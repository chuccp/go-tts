package tts

import (
	"math/rand"
	"strings"
	"testing"
)

func TestSplitTextDoesNotLoseContent(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewSource(42))
	pieces := []string{
		"Hello world. ", "这是一个测试。", "Numbers 12345! ",
		"Unicode: café, 日本語, 😀. ", "A very long word without spaces ",
		"line one\nline two\r\nline three ", "Semicolon; colon: comma, ",
	}
	for iteration := 0; iteration < 200; iteration++ {
		var input strings.Builder
		for count := 0; count < 1+random.Intn(20); count++ {
			input.WriteString(pieces[random.Intn(len(pieces))])
		}
		chunks := SplitText(input.String(), 40+random.Intn(80))
		if len(chunks) == 0 {
			t.Fatalf("iteration %d returned no chunks", iteration)
		}
		for _, chunk := range chunks {
			if cleanText(chunk) == "" {
				t.Fatalf("iteration %d returned an empty chunk", iteration)
			}
		}
		got := strings.Join(strings.Fields(strings.Join(chunks, " ")), "")
		want := strings.Join(strings.Fields(input.String()), "")
		if got != want {
			t.Fatalf("iteration %d lost content\n got: %q\nwant: %q", iteration, got, want)
		}
	}
}

func TestSplitTextHonorsRuneLimit(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("😀中a", 300)
	for _, limit := range []int{40, 41, 79, 400} {
		for _, chunk := range SplitText(text, limit) {
			if got := len([]rune(chunk)); got > limit {
				t.Fatalf("limit %d: chunk has %d runes", limit, got)
			}
		}
	}
}
