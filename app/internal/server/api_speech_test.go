package server

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/pausan/agenttik/app/internal/store"
)

type fakeSpeech struct {
	model string
	text  string
}

func (*fakeSpeech) Ready(string) bool                               { return false }
func (s *fakeSpeech) Prepare(_ context.Context, model string) error { s.model = model; return nil }
func (s *fakeSpeech) Transcribe(_ context.Context, model string, _ []byte) (string, error) {
	s.model = model
	return s.text, nil
}
func (*fakeSpeech) Close() {}

func TestSpeechPreferencesAndTranscription(t *testing.T) {
	s, st := newTestServer(t)
	engine := &fakeSpeech{text: "Hola, revisa el código."}
	s.speech = engine
	initial := decode[store.SpeechConfig](t, do(t, s, "GET", "/api/speech", nil))
	if initial.Model != "whistle" || initial.Correct {
		t.Fatal(initial)
	}
	saved := decode[store.SpeechConfig](t, do(t, s, "PUT", "/api/speech", map[string]any{"model": "whisper-small", "correct": true}))
	if saved.Model != "whisper-small" || !saved.Correct {
		t.Fatal(saved)
	}
	if res := do(t, s, "PUT", "/api/speech", map[string]string{"model": "unknown"}); res.StatusCode != 400 {
		t.Fatal(res.StatusCode)
	}
	if res := do(t, s, "POST", "/api/speech/prepare", map[string]string{"model": "whisper-small"}); res.StatusCode != 204 || engine.model != "whisper-small" {
		t.Fatal(res.StatusCode, engine.model)
	}
	for _, pcm := range []string{"", "???", base64.StdEncoding.EncodeToString([]byte{1})} {
		if res := do(t, s, "POST", "/api/speech/transcribe", map[string]string{"model": "whistle", "pcm": pcm}); res.StatusCode != 400 {
			t.Fatal("accepted invalid PCM")
		}
	}
	result := decode[map[string]string](t, do(t, s, "POST", "/api/speech/transcribe", map[string]string{"model": "whistle", "pcm": "AAA="}))
	if result["text"] != engine.text || result["warning"] == "" {
		t.Fatal("correction failure lost raw transcript", result)
	}
	if err := st.ResetPreferences(); err != nil {
		t.Fatal(err)
	}
	reset, err := st.GetSpeechConfig()
	if err != nil || reset.Model != "whistle" || reset.Correct {
		t.Fatal(reset, err)
	}
}
