package tts

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	onnxMu      sync.Mutex
	onnxStarted bool
	onnxErr     error
	onnxLib     string
)

func FindONNXRuntime(explicit string) string {
	if explicit == "" {
		explicit = os.Getenv("ONNXRUNTIME_LIB")
	}
	if explicit != "" {
		return explicit
	}
	names := []string{"onnxruntime.dll"}
	if runtime.GOOS != "windows" {
		if runtime.GOOS == "darwin" {
			names = []string{"libonnxruntime.dylib"}
		} else {
			names = []string{"libonnxruntime.so", "libonnxruntime.so.1.25.0"}
		}
	}
	candidates := make([]string, 0, len(names)*7)
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		for _, name := range names {
			candidates = append(candidates, filepath.Join(dir, name), filepath.Join(dir, "runtime", name))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		for _, name := range names {
			candidates = append(candidates,
				filepath.Join(cwd, name),
				filepath.Join(cwd, "runtime", name),
				filepath.Join(cwd, "lib", name),
			)
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return names[0]
}

func ensureONNXRuntime(path string) error {
	onnxMu.Lock()
	defer onnxMu.Unlock()
	if onnxStarted {
		if onnxErr == nil && path != "" && onnxLib != path {
			return fmt.Errorf("ONNX Runtime is already loaded from %s, cannot also load %s", onnxLib, path)
		}
		return onnxErr
	}
	onnxStarted = true
	onnxLib = path
	ort.SetSharedLibraryPath(path)
	if err := ort.InitializeEnvironment(); err != nil {
		onnxErr = fmt.Errorf("initialize ONNX Runtime from %s: %w", path, err)
		return onnxErr
	}
	return nil
}

func newONNXSession(modelPath string, inputs, outputs []string) (*ort.DynamicAdvancedSession, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("read model %s: %w", modelPath, err)
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer options.Destroy()
	session, err := ort.NewDynamicAdvancedSession(modelPath, inputs, outputs, options)
	if err != nil {
		return nil, fmt.Errorf("open ONNX model %s: %w", modelPath, err)
	}
	return session, nil
}

func flattenTensor(value ort.Value) ([]float32, error) {
	tensor, ok := value.(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("expected float32 model output, got %T", value)
	}
	data := tensor.GetData()
	result := make([]float32, len(data))
	copy(result, data)
	return result, nil
}

func closeSession(session *ort.DynamicAdvancedSession) error {
	if session == nil {
		return nil
	}
	return session.Destroy()
}
