package tts

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type floatTensor struct {
	Shape []int
	Data  []float32
}

var shapeRE = regexp.MustCompile(`['"]shape['"]\s*:\s*\(([^)]*)\)`)

func LoadVoiceTensorFile(path string) (map[string]floatTensor, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return loadVoiceDirectory(path)
	}
	return loadVoiceFile(path)
}

func loadVoiceDirectory(dir string) (map[string]floatTensor, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]floatTensor)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".npz" && ext != ".npy" && ext != ".bin" {
			continue
		}
		tensors, err := loadVoiceFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", entry.Name(), err)
		}
		for name, tensor := range tensors {
			result[name] = tensor
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no .npz, .npy, or .bin voice files found in %s", dir)
	}
	return result, nil
}

func loadVoiceFile(path string) (map[string]floatTensor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return readNPZ(data)
	}
	if bytes.HasPrefix(data, []byte("\x93NUMPY")) {
		tensor, err := readNPY(data)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		return map[string]floatTensor{name: tensor}, nil
	}
	tensor, err := readRawFloat32(data)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return map[string]floatTensor{name: tensor}, nil
}

func readNPZ(data []byte) (map[string]floatTensor, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open npz: %w", err)
	}
	result := make(map[string]floatTensor)
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		if !strings.HasSuffix(strings.ToLower(file.Name), ".npy") {
			continue
		}
		names = append(names, file.Name)
	}
	sort.Strings(names)
	for _, name := range names {
		file, err := reader.Open(name)
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(file)
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		tensor, err := readNPY(content)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		key := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		result[key] = tensor
	}
	if len(result) == 0 {
		return nil, errors.New("npz contains no .npy entries")
	}
	return result, nil
}

func readNPY(data []byte) (floatTensor, error) {
	if len(data) < 10 || !bytes.HasPrefix(data, []byte("\x93NUMPY")) {
		return floatTensor{}, errors.New("invalid npy header")
	}
	major := data[6]
	var headerSize, offset int
	switch major {
	case 1:
		headerSize = int(binary.LittleEndian.Uint16(data[8:10]))
		offset = 10
	case 2, 3:
		if len(data) < 12 {
			return floatTensor{}, errors.New("truncated npy header")
		}
		headerSize = int(binary.LittleEndian.Uint32(data[8:12]))
		offset = 12
	default:
		return floatTensor{}, fmt.Errorf("unsupported npy version %d", major)
	}
	if headerSize < 0 || offset+headerSize > len(data) {
		return floatTensor{}, errors.New("npy header length exceeds file size")
	}
	header := string(data[offset : offset+headerSize])
	if !strings.Contains(header, "'<f4'") && !strings.Contains(header, `"<f4"`) {
		return floatTensor{}, errors.New("npy voice data must be little-endian float32")
	}
	shapeMatch := shapeRE.FindStringSubmatch(header)
	if shapeMatch == nil {
		return floatTensor{}, errors.New("npy shape is missing")
	}
	shape, err := parseShape(shapeMatch[1])
	if err != nil {
		return floatTensor{}, err
	}
	raw := data[offset+headerSize:]
	expected, err := shapeSize(shape)
	if err != nil {
		return floatTensor{}, err
	}
	if len(raw) != expected*4 {
		return floatTensor{}, fmt.Errorf("npy data size %d does not match shape %v", len(raw), shape)
	}
	floats := make([]float32, expected)
	for i := range floats {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return floatTensor{Shape: shape, Data: floats}, nil
}

func readRawFloat32(data []byte) (floatTensor, error) {
	if len(data) == 0 || len(data)%4 != 0 {
		return floatTensor{}, errors.New("raw voice data must be a non-empty multiple of 4 bytes")
	}
	floats := make([]float32, len(data)/4)
	for i := range floats {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	shape := []int{len(floats)}
	if len(floats)%256 == 0 {
		shape = []int{len(floats) / 256, 256}
	}
	return floatTensor{Shape: shape, Data: floats}, nil
}

func parseShape(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty npy shape")
	}
	parts := strings.Split(raw, ",")
	shape := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("invalid npy dimension %q", part)
		}
		shape = append(shape, value)
	}
	if len(shape) == 0 {
		return nil, errors.New("empty npy shape")
	}
	return shape, nil
}

func shapeSize(shape []int) (int, error) {
	if len(shape) == 0 {
		return 0, errors.New("empty shape")
	}
	size := 1
	for _, dimension := range shape {
		if dimension < 0 {
			return 0, errors.New("invalid tensor dimensions")
		}
		if dimension == 0 {
			return 0, nil
		}
		if size > math.MaxInt/dimension {
			return 0, errors.New("invalid tensor dimensions")
		}
		size *= dimension
	}
	return size, nil
}

func tensorRows(tensor floatTensor) (rows, columns int) {
	if len(tensor.Shape) == 1 {
		return 1, tensor.Shape[0]
	}
	columns = tensor.Shape[len(tensor.Shape)-1]
	elements := len(tensor.Data)
	if columns > 0 {
		rows = elements / columns
	}
	return rows, columns
}
