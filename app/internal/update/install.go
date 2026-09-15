package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Plan is only read by the short-lived copy of the current executable.
// The helper stages on the destination volume before reporting readiness.
type Plan struct {
	Parent                  int
	Target, Digest, Archive string
	Bundle                  bool
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
func verify(path, digest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if !validDigest(digest) || "sha256:"+hex.EncodeToString(h.Sum(nil)) != digest {
		return errors.New("update checksum mismatch")
	}
	return nil
}
func unpackBundle(ctx context.Context, archive, dir string) (string, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer r.Close()
	var total uint64
	for _, f := range r.File {
		name := filepath.FromSlash(f.Name)
		clean := filepath.Clean(name)
		if clean != "agenttik.app" && !strings.HasPrefix(clean, "agenttik.app"+string(os.PathSeparator)) && clean != "__MACOSX" && !strings.HasPrefix(clean, "__MACOSX"+string(os.PathSeparator)) {
			return "", errors.New("unexpected path in app archive")
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlinks are not supported in app archives")
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > maxDownload || total > 2*maxDownload {
			return "", errors.New("app archive is too large")
		}
	}
	// ditto retains the bundle metadata used by Apple's code signature.
	if out, err := exec.CommandContext(ctx, "/usr/bin/ditto", "-x", "-k", archive, dir).CombinedOutput(); err != nil {
		return "", fmt.Errorf("extract app: %s: %w", out, err)
	}
	source := filepath.Join(dir, "agenttik.app")
	if out, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", source).CombinedOutput(); err != nil {
		return "", fmt.Errorf("verify app signature: %s: %w", out, err)
	}
	return source, nil
}

// Helper runs before normal startup, without opening the database or UI.
func Helper(manifest string) (err error) {
	defer func() {
		suffix := ".done"
		message := "installed"
		if err != nil {
			suffix = ".error"
			message = err.Error()
		}
		_ = os.WriteFile(manifest+suffix, []byte(message), 0644)
	}()
	if _, cancelled := os.Stat(manifest + ".cancel"); cancelled == nil {
		return errors.New("update cancelled")
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Parent <= 0 || !filepath.IsAbs(p.Target) || !filepath.IsAbs(p.Archive) {
		return errors.New("invalid update plan")
	}
	original, err := os.Lstat(p.Target)
	if err != nil {
		return err
	}
	if original.Mode()&os.ModeSymlink != 0 || original.IsDir() != p.Bundle {
		return errors.New("installation path changed")
	}
	staging, err := os.MkdirTemp(filepath.Dir(p.Target), ".agenttik-update-")
	if err != nil {
		return err
	}
	defer func() {
		// Never remove the only surviving copy if rollback also failed.
		if _, statErr := os.Stat(filepath.Join(staging, "previous")); err == nil || errors.Is(statErr, os.ErrNotExist) {
			os.RemoveAll(staging)
		}
	}()
	next := filepath.Join(staging, filepath.Base(p.Target))
	if p.Bundle {
		// Copy into the protected staging directory before verification and
		// extraction, so the input cannot change between these steps.
		archive := filepath.Join(staging, "bundle.zip")
		err = copyFile(p.Archive, archive, 0600)
		if err == nil {
			err = verify(archive, p.Digest)
		}
		if err == nil {
			next, err = unpackBundle(context.Background(), archive, staging)
		}
	} else {
		err = copyFile(p.Archive, next, 0755)
		if err == nil {
			err = verify(next, p.Digest)
		}
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifest+".ready", []byte("ready"), 0644); err != nil {
		return err
	}
	if err := waitParent(p.Parent); err != nil {
		return err
	}
	if _, cancelled := os.Stat(manifest + ".cancel"); cancelled == nil {
		return errors.New("update cancelled")
	}
	now, err := os.Lstat(p.Target)
	if err != nil {
		return err
	}
	if !os.SameFile(original, now) || !original.ModTime().Equal(now.ModTime()) {
		return errors.New("executable changed while update was pending; update cancelled")
	}
	return replace(p.Target, next, filepath.Join(staging, "previous"))
}

func replace(target, next, backup string) error {
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("keep current executable: %w", err)
	}
	if err := os.Rename(next, target); err != nil {
		if rollback := os.Rename(backup, target); rollback != nil {
			return fmt.Errorf("install: %v; restore: %v; previous executable saved at %s", err, rollback, backup)
		}
		return fmt.Errorf("install failed; previous executable restored: %w", err)
	}
	return nil
}

func writableDirectory(target string) bool {
	f, err := os.CreateTemp(filepath.Dir(target), ".agenttik-write-test-")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}
