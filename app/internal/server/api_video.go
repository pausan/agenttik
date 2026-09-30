package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// videoTypes is every video the video endpoint serves, and the content type
// each is served as. Like rawTypes it is an allowlist, not a sniff. Browsers
// play only some of these; the rest are converted by ffmpeg on request.
var videoTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
	".wmv":  "video/x-ms-wmv",
	".flv":  "video/x-flv",
	".3gp":  "video/3gpp",
	".m2ts": "video/mp2t",
}

// videoFile resolves a working-tree video. There is no rev: a video is far
// past what git show should push through memory, and a diff of one is git's
// one binary line.
func (s *Server) videoFile(c *fiber.Ctx) (rel, abs, kind string, err error) {
	_, rel, abs, err = s.previewFilePath(c)
	if err != nil {
		return "", "", "", err
	}
	kind, ok := videoTypes[strings.ToLower(filepath.Ext(rel))]
	if !ok {
		return "", "", "", badRequest("%s is not a video agenttik plays", rel)
	}
	return rel, abs, kind, nil
}

// projectVideo streams a video. The file is sent as it is, in the ranges the
// player asks for, so seeking never reads what is skipped and nothing is held
// in memory. transcode=1 converts it with ffmpeg instead, from start seconds,
// for the formats the browser cannot decode.
func (s *Server) projectVideo(c *fiber.Ctx) error {
	rel, abs, kind, err := s.videoFile(c)
	if err != nil {
		return err
	}
	if c.Query("transcode") == "1" {
		return s.transcodeVideo(c, abs)
	}
	f, err := os.Open(abs)
	if err != nil {
		return badRequest("cannot read %s: %v", rel, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return badRequest("%s is not a regular file", rel)
	}
	size := info.Size()
	start, end := int64(0), size-1
	c.Set(fiber.HeaderAcceptRanges, "bytes")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	if r := c.Get(fiber.HeaderRange); r != "" && size > 0 {
		from, to, err := fasthttp.ParseByteRange([]byte(r), int(size))
		if err != nil {
			f.Close()
			c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes */%d", size))
			return c.SendStatus(fiber.StatusRequestedRangeNotSatisfiable)
		}
		start, end = int64(from), int64(to)
		c.Status(fiber.StatusPartialContent)
		c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	}
	c.Set(fiber.HeaderContentType, kind)
	// fasthttp closes the stream once it is sent, or the client has gone.
	body := struct {
		io.Reader
		io.Closer
	}{io.NewSectionReader(f, start, end-start+1), f}
	c.Context().SetBodyStream(body, int(end-start+1))
	return nil
}

// transcodeVideo pipes ffmpeg's WebM straight to the client. VP9 and Opus
// play in Chromium, Firefox and WebKit alike, H.264 not in every build of
// them, and WebM plays while it is still being written. It cannot be seeked
// by range, so the player seeks by asking again from a new start. ffmpeg runs only while someone is reading;
// a closed connection, or the server closing, stops it.
func (s *Server) transcodeVideo(c *fiber.Ctx, abs string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fiber.NewError(fiber.StatusNotImplemented, "ffmpeg is not installed")
	}
	start, _ := strconv.ParseFloat(c.Query("start"), 64)
	if math.IsNaN(start) || math.IsInf(start, 0) || start < 0 {
		start = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, ffmpeg, transcodeArgs(abs, start)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	go func() {
		select {
		case <-s.closing:
		case <-ctx.Done():
		}
		cancel()
	}()
	c.Set(fiber.HeaderContentType, "video/webm")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			cancel()
			_ = cmd.Wait()
		}()
		buf := make([]byte, 64<<10)
		for {
			n, err := out.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil || w.Flush() != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	})
	return nil
}

// transcodeArgs keeps the first video and audio stream, either of which may
// be missing. The file: prefix stops a name being read as another protocol,
// the scale rounds odd sizes, which yuv420p cannot hold, and realtime keeps
// the encoder ahead of playback.
func transcodeArgs(abs string, start float64) []string {
	return []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", strconv.FormatFloat(start, 'f', 3, 64),
		"-i", "file:" + abs,
		"-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn",
		"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-row-mt", "1",
		"-b:v", "0", "-crf", "32", "-pix_fmt", "yuv420p",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:a", "libopus", "-ac", "2", "-ar", "48000", "-b:a", "128k",
		"-f", "webm", "pipe:1",
	}
}

type videoInfo struct {
	Duration  float64 `json:"duration"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Video     string  `json:"video,omitempty"`
	Audio     string  `json:"audio,omitempty"`
	Transcode bool    `json:"transcode"`
}

// projectVideoInfo is what a converted stream cannot say about itself: how
// long the whole file is, so the player can draw its seek bar. transcode says
// whether ffmpeg is there to convert it at all.
func (s *Server) projectVideoInfo(c *fiber.Ctx) error {
	rel, abs, _, err := s.videoFile(c)
	if err != nil {
		return err
	}
	var body videoInfo
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		body.Transcode = true
	}
	if ffprobe, err := exec.LookPath("ffprobe"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-print_format", "json",
			"-show_entries", "format=duration:stream=codec_type,codec_name,width,height", "file:"+abs).Output()
		if err != nil {
			return badRequest("cannot read %s as a video", rel)
		}
		body.probe(out)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(body)
}

// probe reads ffprobe's JSON. Its numbers arrive as strings.
func (v *videoInfo) probe(out []byte) {
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &p) != nil {
		return
	}
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil && d > 0 && !math.IsInf(d, 0) {
		v.Duration = d
	}
	for _, st := range p.Streams {
		switch {
		case st.Type == "video" && v.Video == "":
			v.Video, v.Width, v.Height = st.Codec, st.Width, st.Height
		case st.Type == "audio" && v.Audio == "":
			v.Audio = st.Codec
		}
	}
}
