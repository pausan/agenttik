// Package speech transcribes bounded recordings in short-lived native processes.
// Models are cached on the host; nothing is loaded or downloaded at startup.
package speech

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const MaxSamples = 16000 * 120

//go:embed bin/*
var helpers embed.FS

type Model struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Bytes          int64  `json:"bytes"`
	file, checksum string
}

var Models = []Model{
	{"whistle", "Whistle (default) · 7 languages", 16919407, "whistle.cact", "b6e02f048568ac5d01a2042556c658061e699acbc0aa2a1439f52f3d461dffeb"},
	{"whisper-tiny", "Whisper Tiny · multilingual", 32152673, "ggml-tiny-q5_1.bin", "818710568da3ca15689e31a743197b520007872ff9576237bda97bd1b469c3d7"},
	{"whisper-base", "Whisper Base · multilingual", 59707625, "ggml-base-q5_1.bin", "422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898"},
	{"whisper-small", "Whisper Small · multilingual", 190085487, "ggml-small-q5_1.bin", "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb"},
	{"whisper-medium", "Whisper Medium · multilingual", 539212467, "ggml-medium-q5_0.bin", "19fea4b380c3a618ec4723c3eef2eb785ffba0d0538cf43f8f235e7b3b34220f"},
}

func Lookup(id string) (Model, bool) {
	for _, m := range Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

type Service struct {
	lifetime context.Context
	stop     context.CancelFunc
	dir      string
	gate     chan struct{}
	client   *http.Client
	// Tests substitute local artifacts and a fake process, without network or inference.
	baseURL string
	run     func(context.Context, string, ...string) ([]byte, error)
}

func New(dir string) *Service {
	ctx, stop := context.WithCancel(context.Background())
	return &Service{lifetime: ctx, stop: stop, dir: dir, gate: make(chan struct{}, 1), client: &http.Client{Timeout: 15 * time.Minute}, baseURL: "https://huggingface.co", run: run}
}

func (s *Service) Close() { s.stop() }
func (s *Service) operation(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	stop := context.AfterFunc(s.lifetime, cancel)
	return ctx, func() { stop(); cancel() }
}

func (s *Service) lock(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Ready(id string) bool {
	m, ok := Lookup(id)
	if !ok {
		return false
	}
	info, err := os.Stat(filepath.Join(s.dir, m.file))
	return err == nil && info.Size() == m.Bytes
}

func (s *Service) Prepare(ctx context.Context, id string) error {
	ctx, done := s.operation(ctx)
	defer done()
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer func() { <-s.gate }()
	_, _, err := s.prepare(ctx, id)
	return err
}

func (s *Service) prepare(ctx context.Context, id string) (string, string, error) {
	m, ok := Lookup(id)
	if !ok {
		return "", "", errors.New("unknown speech model")
	}
	var helper string
	if id == "whistle" {
		artifact, ok := whistleRuntime[runtime.GOOS+"/"+runtime.GOARCH]
		if !ok {
			return "", "", errors.New("Whistle is unavailable on this platform; choose Whisper in General settings")
		}
		helper = filepath.Join(s.dir, "needle-3.3.3"+exeSuffix())
		url := s.baseURL + "/Cactus-Compute/needle3/resolve/d6c7d6a2e9655505e0f8e23339356dd1705900eb/" + artifact.path
		if err := s.download(ctx, url, helper, artifact.size, artifact.checksum, 0700); err != nil {
			return "", "", err
		}
	} else {
		name := "whisper-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix()
		data, err := helpers.ReadFile("bin/" + name)
		if err != nil {
			return "", "", errors.New("this build is missing its Whisper helper; rebuild Agenttik with make speech")
		}
		hash := sha256.Sum256(data)
		helper = filepath.Join(s.dir, fmt.Sprintf("whisper-%x%s", hash[:8], exeSuffix()))
		if err := publish(helper, data, 0700); err != nil {
			return "", "", err
		}
	}
	repo := "/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/"
	if id == "whistle" {
		repo = "/Cactus-Compute/whistle/resolve/741118687e1f008f06d1351a402a731274789790/"
	}
	model := filepath.Join(s.dir, m.file)
	if err := s.download(ctx, s.baseURL+repo+m.file, model, m.Bytes, m.checksum, 0600); err != nil {
		return "", "", err
	}
	return helper, model, nil
}

func (s *Service) Transcribe(ctx context.Context, id string, pcm []byte) (string, error) {
	ctx, done := s.operation(ctx)
	defer done()
	if len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > MaxSamples*2 {
		return "", errors.New("recording must be 16 kHz mono PCM16, at most 2 minutes")
	}
	if err := s.lock(ctx); err != nil {
		return "", err
	}
	defer func() { <-s.gate }()
	helper, model, err := s.prepare(ctx, id)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(s.dir, "recording-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(wav, wave(pcm), 0600); err != nil {
		return "", err
	}
	args := []string{"--model", model, "--audio", wav}
	if id != "whistle" {
		args = []string{"-m", model, "-f", wav, "-l", "auto", "-t", "4", "-nt", "-np", "-ng"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := s.run(ctx, helper, args...)
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}
	text := string(out)
	if id == "whistle" {
		var result struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(out, &result); err != nil {
			return "", errors.New("Whistle returned an invalid transcript")
		}
		text = result.Text
	}
	return strings.TrimSpace(text), nil
}

func wave(pcm []byte) []byte {
	b := make([]byte, 44+len(pcm))
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+len(pcm)))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(pcm)))
	copy(b[44:], pcm)
	return b
}

func (s *Service) download(ctx context.Context, url, path string, size int64, checksum string, mode os.FileMode) error {
	if info, err := os.Stat(path); err == nil && info.Size() == size {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download speech model: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download speech model: HTTP %d", res.StatusCode)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(res.Body, size+1))
	if err != nil {
		return err
	}
	if n != size || hex.EncodeToString(hash.Sum(nil)) != checksum {
		return errors.New("speech download failed its integrity check; try again")
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func publish(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(data)) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".helper-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if left := (64 << 10) - b.Len(); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	configureCommand(cmd)
	cmd.Dir = os.TempDir()
	var out, stderr cappedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

var whistleRuntime = map[string]struct {
	path     string
	size     int64
	checksum string
}{
	"linux/amd64":   {"linux-x86_64/needle", 1550176, "77cd35ee75c53df8532b397e11ed7cf94a45d89a9f14f50881fd9c68ba54a2d0"},
	"linux/arm64":   {"linux-arm64/needle", 1439720, "a7dd07f740763507edbca760dc7e7065b6da6b6bd6c3a05288ca974f14ce0fcb"},
	"darwin/arm64":  {"macos-arm64/needle", 1107096, "0a480f7e01d1fcf9df2e6dbc29af29411406720a5c5cb73024fcd6d16aa25b60"},
	"windows/amd64": {"windows-x86_64/needle.exe", 1570816, "0d4bc1da8c51f5600f5c4f4bb4a92e0abe142fae7b69e695ccab93e6745c883d"},
}
