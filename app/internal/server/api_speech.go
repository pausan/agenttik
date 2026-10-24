package server

import (
	"context"
	"encoding/base64"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/speech"
)

type speechEngine interface {
	Ready(string) bool
	Prepare(context.Context, string) error
	Transcribe(context.Context, string, []byte) (string, error)
	Close()
}

func (s *Server) getSpeech(c *fiber.Ctx) error {
	config, err := s.store.GetSpeechConfig()
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"model": config.Model, "correct": config.Correct, "models": speech.Models, "ready": s.speech.Ready(config.Model)})
}

func (s *Server) putSpeech(c *fiber.Ctx) error {
	config, err := s.store.GetSpeechConfig()
	if err != nil {
		return err
	}
	if err := c.BodyParser(&config); err != nil {
		return badRequest("invalid speech preferences")
	}
	if _, ok := speech.Lookup(config.Model); !ok {
		return badRequest("unknown speech model")
	}
	if err := s.store.SetSpeechConfig(config); err != nil {
		return err
	}
	return s.getSpeech(c)
}

func (s *Server) prepareSpeech(c *fiber.Ctx) error {
	var req struct {
		Model string `json:"model"`
	}
	if err := c.BodyParser(&req); err != nil {
		return badRequest("invalid speech request")
	}
	if _, ok := speech.Lookup(req.Model); !ok {
		return badRequest("unknown speech model")
	}
	if err := s.speech.Prepare(context.Background(), req.Model); err != nil {
		return badRequest("%v", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) transcribeSpeech(c *fiber.Ctx) error {
	var req struct {
		Model string `json:"model"`
		PCM   string `json:"pcm"`
	}
	if err := c.BodyParser(&req); err != nil {
		return badRequest("invalid recording")
	}
	if _, ok := speech.Lookup(req.Model); !ok {
		return badRequest("unknown speech model")
	}
	if len(req.PCM) > base64.StdEncoding.EncodedLen(speech.MaxSamples*2) {
		return badRequest("recordings must be at most 2 minutes")
	}
	pcm, err := base64.StdEncoding.DecodeString(req.PCM)
	if err != nil || len(pcm) == 0 || len(pcm)%2 != 0 {
		return badRequest("invalid PCM16 recording")
	}
	text, err := s.speech.Transcribe(context.Background(), req.Model, pcm)
	if err != nil {
		return badRequest("%v", err)
	}
	config, err := s.store.GetSpeechConfig()
	if err != nil {
		return err
	}
	warning := ""
	if config.Correct && text != "" {
		choice, err := s.actionModel("transcription")
		if err != nil {
			warning = "Correction unavailable. The original transcript was kept."
		} else if corrected := s.runner.CorrectTranscription(text, choice); corrected != "" {
			text = corrected
		} else {
			warning = "Correction failed. The original transcript was kept."
		}
	}
	return c.JSON(fiber.Map{"text": text, "warning": warning})
}
