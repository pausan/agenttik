package server

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type fileInfo struct {
	Dir   bool       `json:"dir,omitempty"`
	Size  int64      `json:"size"`
	Image *imageInfo `json:"image,omitempty"`
}

type imageInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Bits   int `json:"bits,omitempty"`
}

// Metadata never inlines file content. A bounded header read is enough for
// common raster formats; unknown formats still have an exact byte size.
func (s *Server) projectFileInfo(c *fiber.Ctx) error {
	root, rel, abs, err := s.previewFilePath(c)
	if err != nil {
		return err
	}
	var body fileInfo
	var header []byte
	isImage := strings.HasPrefix(rawTypes[strings.ToLower(filepath.Ext(rel))], "image/")
	if rev := c.Query("rev"); rev != "" {
		if !rawRev.MatchString(rev) {
			return badRequest("invalid revision")
		}
		spec := rev + ":" + filepath.ToSlash(filepath.Clean(rel))
		size, err := runGit(root, "cat-file", "-s", spec)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "file is not in this revision")
		}
		body.Size, err = strconv.ParseInt(strings.TrimSpace(size), 10, 64)
		if err != nil {
			return err
		}
		if isImage && body.Size <= maxRawBytes {
			header, _ = gitBlob(root, spec)
		}
	} else {
		info, err := os.Stat(abs)
		if err != nil {
			return badRequest("cannot read %s: %v", rel, err)
		}
		if info.IsDir() {
			c.Set(fiber.HeaderCacheControl, "no-store")
			return c.JSON(fileInfo{Dir: true})
		}
		if !info.Mode().IsRegular() {
			return badRequest("%s is not a regular file", rel)
		}
		body.Size = info.Size()
		if isImage {
			f, err := os.Open(abs)
			if err == nil {
				defer f.Close()
				header, _ = io.ReadAll(io.LimitReader(f, 64<<10))
			}
		}
	}
	body.Image = readImageInfo(header)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(body)
}

// Bit depth describes the stored pixel (including alpha), or palette index.
// DecodeConfig reads dimensions without allocating a full pixel buffer.
func readImageInfo(b []byte) *imageInfo {
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(b)); err == nil {
		info := &imageInfo{Width: cfg.Width, Height: cfg.Height}
		switch format {
		case "png":
			channels := map[byte]int{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}
			info.Bits = int(b[24]) * channels[b[25]]
		case "gif":
			if b[10]&0x80 != 0 {
				info.Bits = int(b[10]&7) + 1
			}
		case "jpeg":
			switch cfg.ColorModel {
			case color.GrayModel:
				info.Bits = 8
			case color.YCbCrModel:
				info.Bits = 24
			case color.CMYKModel:
				info.Bits = 32
			}
		}
		return info
	}
	if len(b) >= 30 && string(b[:2]) == "BM" {
		headerSize := binary.LittleEndian.Uint32(b[14:])
		if headerSize == 12 {
			return validImageInfo(int(binary.LittleEndian.Uint16(b[18:])), int(binary.LittleEndian.Uint16(b[20:])), int(binary.LittleEndian.Uint16(b[24:])))
		}
		if headerSize >= 40 {
			w, h := int(int32(binary.LittleEndian.Uint32(b[18:]))), int(int32(binary.LittleEndian.Uint32(b[22:])))
			if h < 0 {
				h = -h
			}
			return validImageInfo(w, h, int(binary.LittleEndian.Uint16(b[28:])))
		}
	}
	if len(b) >= 22 && bytes.Equal(b[:4], []byte{0, 0, 1, 0}) && binary.LittleEndian.Uint16(b[4:]) > 0 {
		// ICO can contain several sizes. Report the largest directory entry.
		var best *imageInfo
		for i, count := 6, int(binary.LittleEndian.Uint16(b[4:])); count > 0 && i+16 <= len(b); i, count = i+16, count-1 {
			w, h := int(b[i]), int(b[i+1])
			if w == 0 {
				w = 256
			}
			if h == 0 {
				h = 256
			}
			info := validImageInfo(w, h, int(binary.LittleEndian.Uint16(b[i+6:])))
			if best == nil || w*h > best.Width*best.Height {
				best = info
			}
		}
		return best
	}
	if len(b) >= 25 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		bits := 24
		switch string(b[12:16]) {
		case "VP8X":
			if len(b) >= 30 {
				if b[20]&0x10 != 0 {
					bits = 32
				}
				return validImageInfo(uint24(b[24:])+1, uint24(b[27:])+1, bits)
			}
		case "VP8L":
			if b[20] == 0x2f {
				v := binary.LittleEndian.Uint32(b[21:])
				if v&(1<<28) != 0 {
					bits = 32
				}
				return validImageInfo(int(v&0x3fff)+1, int((v>>14)&0x3fff)+1, bits)
			}
		case "VP8 ":
			if len(b) >= 30 && bytes.Equal(b[23:26], []byte{0x9d, 0x01, 0x2a}) {
				return validImageInfo(int(binary.LittleEndian.Uint16(b[26:])&0x3fff), int(binary.LittleEndian.Uint16(b[28:])&0x3fff), bits)
			}
		}
	}
	return nil
}

func uint24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }

func validImageInfo(w, h, bits int) *imageInfo {
	if w <= 0 || h <= 0 {
		return nil
	}
	return &imageInfo{Width: w, Height: h, Bits: bits}
}
