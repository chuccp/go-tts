package tts

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVoiceTensorFileNPZ(t *testing.T) {
	t.Parallel()
	var npy bytes.Buffer
	npy.WriteString("\x93NUMPY")
	npy.WriteByte(1)
	npy.WriteByte(0)
	_ = binary.Write(&npy, binary.LittleEndian, uint16(118))
	header := "{'descr': '<f4', 'fortran_order': False, 'shape': (2, 256), }"
	npy.WriteString(header)
	npy.Write(make([]byte, 118-len(header)-1))
	npy.WriteByte('\n')
	for index := 0; index < 512; index++ {
		_ = binary.Write(&npy, binary.LittleEndian, math.Float32bits(float32(index)/10))
	}

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	entry, err := zipWriter.Create("voice_a.npy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(npy.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "voices.npz")
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	tensors, err := LoadVoiceTensorFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tensor, ok := tensors["voice_a"]
	if !ok {
		t.Fatal("voice_a was not loaded")
	}
	if rows, columns := tensorRows(tensor); rows != 2 || columns != 256 {
		t.Fatalf("shape = %dx%d, want 2x256", rows, columns)
	}
	if len(tensor.Data) != 512 || tensor.Data[511] != 51.1 {
		t.Fatalf("unexpected data length/value: %d / %f", len(tensor.Data), tensor.Data[511])
	}
}

func TestSplitTextHandlesCJKAndLongSentences(t *testing.T) {
	t.Parallel()
	text := "第一句话。第二句话！这是一个很长的句子，需要在合适的位置拆分，而不是在 UTF-8 字节中间截断。我们继续增加一些文字，确保它超过四十二个字符。"
	chunks := SplitText(text, 42)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks, want at least 3: %q", len(chunks), chunks)
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 42 {
			t.Fatalf("chunk exceeds limit: %q", chunk)
		}
	}
}

func TestBuildEdgeSSMLUsesSpeed(t *testing.T) {
	t.Parallel()
	ssml := buildEdgeSSML("5 < 6 & 7", "zh-CN-XiaoxiaoNeural", 1.25)
	for _, want := range []string{
		"xml:lang='zh-CN'",
		"rate='+25%'",
		"5 &lt; 6 &amp; 7",
	} {
		if !bytes.Contains([]byte(ssml), []byte(want)) {
			t.Errorf("SSML does not contain %q: %s", want, ssml)
		}
	}
}
