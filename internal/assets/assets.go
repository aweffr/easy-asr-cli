package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	SileroVADModelURL    = "https://raw.githubusercontent.com/snakers4/silero-vad/b163605b3f44c3aadf28f97b125a2f7c461e9a7f/src/silero_vad/data/silero_vad_16k_op15.onnx"
	SileroVADModelSHA256 = "7ed98ddbad84ccac4cd0aeb3099049280713df825c610a8ed34543318f1b2c49"
	maxModelBytes        = 10 << 20
)

func DefaultModelDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "easy_asr", "models"), nil
}

func DefaultSileroVADModelPath() (string, error) {
	dir, err := DefaultModelDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "silero_vad_16k_op15.onnx"), nil
}

func InstallSileroVADModel(ctx context.Context, targetPath string) (string, error) {
	if targetPath == "" {
		var err error
		targetPath, err = DefaultSileroVADModelPath()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SileroVADModelURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download silero vad model failed with HTTP %d", resp.StatusCode)
	}
	tmp := targetPath + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	reader := io.LimitReader(resp.Body, maxModelBytes+1)
	written, err := io.Copy(io.MultiWriter(file, hasher), reader)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if written > maxModelBytes {
		_ = file.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("downloaded silero vad model exceeds %d bytes", maxModelBytes)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	if sum != SileroVADModelSHA256 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("silero vad model checksum mismatch: got %s", sum)
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return targetPath, nil
}

func ResolveSileroVADModelPath(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	return DefaultSileroVADModelPath()
}

func DefaultONNXRuntimeLibraryCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/opt/homebrew/lib/libonnxruntime.dylib",
			"/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
		}
	case "linux":
		return []string{"/usr/local/lib/libonnxruntime.so", "/usr/lib/libonnxruntime.so"}
	default:
		return nil
	}
}

func ResolveONNXRuntimeLibraryPath(configured string) string {
	if configured != "" {
		return configured
	}
	for _, candidate := range DefaultONNXRuntimeLibraryCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func CheckExecutable(name string) error {
	_, err := exec.LookPath(name)
	return err
}
