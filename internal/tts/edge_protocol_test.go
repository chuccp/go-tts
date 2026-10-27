package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEdgeSynthesizeProtocol(t *testing.T) {
	t.Parallel()
	serverErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("TrustedClientToken") != edgeClientToken ||
			query.Get("Sec-MS-GEC") == "" || query.Get("Sec-MS-GEC-Version") != edgeTokenVersion {
			serverErrors <- fmt.Errorf("missing Edge authentication query parameters")
			http.Error(w, "missing query", http.StatusBadRequest)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErrors <- err
			return
		}
		defer conn.Close()

		_, config, err := conn.ReadMessage()
		if err != nil {
			serverErrors <- err
			return
		}
		if !bytes.Contains(config, []byte("Path:speech.config")) ||
			!bytes.Contains(config, []byte("audio-24khz-48kbitrate-mono-mp3")) {
			serverErrors <- fmt.Errorf("unexpected speech config: %s", config)
			return
		}
		_, request, err := conn.ReadMessage()
		if err != nil {
			serverErrors <- err
			return
		}
		for _, want := range []string{
			"Path:ssml",
			"rate='+25%'",
			"Hello &amp; &lt;world&gt;",
			"voice name='en-US-TestNeural'",
		} {
			if !bytes.Contains(request, []byte(want)) {
				serverErrors <- fmt.Errorf("SSML does not contain %q: %s", want, request)
				return
			}
		}

		_ = conn.WriteMessage(websocket.TextMessage, []byte("X-RequestId:1\r\nPath:turn.start\r\n\r\n{}"))
		_ = conn.WriteMessage(websocket.BinaryMessage, testEdgeAudioFrame("first", []byte("MP3-A")))
		_ = conn.WriteMessage(websocket.TextMessage, []byte("X-RequestId:1\r\nPath:audio.metadata\r\n\r\n{}"))
		_ = conn.WriteMessage(websocket.BinaryMessage, testEdgeAudioFrame("second", []byte("MP3-B")))
		_ = conn.WriteMessage(websocket.BinaryMessage, testEdgeAudioFrame("terminal", nil))
		_ = conn.WriteMessage(websocket.TextMessage, []byte("X-RequestId:1\r\nPath:turn.end\r\n\r\n{}"))
	}))
	defer server.Close()

	engine := NewEdgeEngine(EdgeConfig{DefaultVoice: "en-US-TestNeural"})
	engine.speechEndpoint = "ws" + strings.TrimPrefix(server.URL, "http")

	audio, err := engine.Synthesize(
		context.Background(),
		"Hello & <world>",
		"en-US-TestNeural",
		1.25,
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverErrors:
		t.Fatal(err)
	default:
	}
	if string(audio.Data) != "MP3-AMP3-B" {
		t.Fatalf("audio = %q", audio.Data)
	}
	if audio.MIMEType != "audio/mpeg" || audio.Extension != "mp3" || audio.SampleRate != 24000 {
		t.Fatalf("unexpected audio metadata: %+v", audio)
	}
}

func TestEdgeVoiceListCachingAndFallback(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("Sec-MS-GEC") == "" {
			http.Error(w, "missing token", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"ShortName":"en-US-TestNeural","Gender":"Female","Locale":"en-US"},
			{"ShortName":"zh-TW-TestNeural","Gender":"Female","Locale":"zh-TW"},
			{"ShortName":"zh-CN-TestNeural","Gender":"Male","Locale":"zh-CN"}
		]`))
	}))
	engine := NewEdgeEngine(EdgeConfig{})
	engine.voiceEndpoint = server.URL

	voices, err := engine.Voices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) != 3 || voices[0].ID != "zh-CN-TestNeural" || voices[1].ID != "zh-TW-TestNeural" {
		t.Fatalf("unexpected voice order: %+v", voices)
	}
	if _, err := engine.Voices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("voice endpoint requests = %d, want 1", got)
	}
	server.Close()

	fresh := NewEdgeEngine(EdgeConfig{})
	fresh.voiceEndpoint = server.URL
	fallback, err := fresh.Voices(context.Background())
	if err == nil {
		t.Fatal("closed voice server should return a warning error")
	}
	if len(fallback) == 0 {
		t.Fatal("expected fallback voices")
	}
}

func TestEdgeContextCancellationDoesNotWaitForReadTimeout(t *testing.T) {
	t.Parallel()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	requestsRead := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
		requestsRead <- struct{}{}
		<-release
	}))
	defer server.Close()
	defer close(release)

	engine := NewEdgeEngine(EdgeConfig{})
	engine.speechEndpoint = "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := engine.synthesizeChunk(ctx, "hello", "en-US-TestNeural", 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
	select {
	case <-requestsRead:
	case <-time.After(time.Second):
		t.Fatal("fake server did not receive both request messages")
	}
}

func TestEdgeRejectsMalformedAudioAndMissingAudio(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		handler func(*websocket.Conn)
		want    string
	}{
		{
			name: "malformed header",
			handler: func(conn *websocket.Conn) {
				_, _, _ = conn.ReadMessage()
				_, _, _ = conn.ReadMessage()
				_ = conn.WriteMessage(websocket.BinaryMessage, []byte{0, 200, 0, 0})
			},
			want: "header length",
		},
		{
			name: "turn end without audio",
			handler: func(conn *websocket.Conn) {
				_, _, _ = conn.ReadMessage()
				_, _, _ = conn.ReadMessage()
				_ = conn.WriteMessage(websocket.TextMessage, []byte("X-RequestId:1\r\nPath:turn.end\r\n\r\n{}"))
			},
			want: "without audio",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				test.handler(conn)
			}))
			defer server.Close()
			engine := NewEdgeEngine(EdgeConfig{})
			engine.speechEndpoint = "ws" + strings.TrimPrefix(server.URL, "http")
			_, err := engine.synthesizeChunk(context.Background(), "hello", "en-US-TestNeural", 1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestBuildEdgeSSMLSpeedRoundingAndEscaping(t *testing.T) {
	t.Parallel()
	tests := map[float64]string{
		0.8: "rate='-20%'",
		0.9: "rate='-10%'",
		1.2: "rate='+20%'",
		1.9: "rate='+90%'",
	}
	for speed, want := range tests {
		ssml := buildEdgeSSML("text", "zh-CN-TestNeural", speed)
		if !strings.Contains(ssml, want) {
			t.Errorf("speed %.1f missing %s: %s", speed, want, ssml)
		}
	}
	ssml := buildEdgeSSML("text", "en-US-A'B\"><C", 1)
	if strings.Contains(ssml, "A'B") || !strings.Contains(ssml, "&apos;") {
		t.Fatalf("voice attribute was not escaped: %s", ssml)
	}
}

func testEdgeAudioFrame(requestID string, payload []byte) []byte {
	header := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:audio/mpeg\r\nPath:audio\r\nContent-Length:%d",
		requestID,
		len(payload),
	)
	headerLength := 2 + len(header)
	frame := make([]byte, 2, headerLength+2+len(payload))
	binary.BigEndian.PutUint16(frame, uint16(headerLength))
	frame = append(frame, header...)
	frame = append(frame, '\r', '\n')
	frame = append(frame, payload...)
	return frame
}
