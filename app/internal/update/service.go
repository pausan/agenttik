// Package update checks published GitHub releases and stages user-approved updates.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
)

const releasesURL = "https://api.github.com/repos/pausan/agenttik/releases?per_page=100"
const maxDownload = 512 << 20

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type Offer struct {
	Version string `json:"version"`
	Asset   Asset  `json:"asset"`
}
type Status struct {
	Available *Offer `json:"available,omitempty"`
	Busy      bool   `json:"busy"`
	Ready     bool   `json:"ready"`
	Error     string `json:"error,omitempty"`
}
type saved struct {
	Checked time.Time `json:"checked"`
	Ignored string    `json:"ignored"`
	Offer   *Offer    `json:"offer,omitempty"`
}
type Service struct {
	mu                                sync.Mutex
	state                             saved
	status                            Status
	path, version, target, goos, arch string
	bundle                            bool
	client                            *http.Client
	endpoint                          string
}

func New(dir, version string) (*Service, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	s := &Service{path: filepath.Join(dir, "updates.json"), version: version, target: executable,
		goos: runtime.GOOS, arch: runtime.GOARCH, endpoint: releasesURL,
		client: &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 10 {
				return errors.New("unsafe download redirect")
			}
			return nil
		}}}
	if runtime.GOOS == "darwin" && filepath.Base(filepath.Dir(executable)) == "MacOS" && filepath.Base(filepath.Dir(filepath.Dir(executable))) == "Contents" {
		bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
		if strings.HasSuffix(bundle, ".app") {
			s.target = bundle
			s.bundle = true
		}
	}
	data, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(data, &s.state); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	s.collectResults(dir)
	return s, nil
}

func (s *Service) save() error {
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(s.path+".tmp", s.path)
}
func stable(value string) *semver.Version {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(path.Base(value), "v"))
	if err != nil || v.Prerelease() != "" {
		return nil
	}
	return v
}
func selectOffer(releases []Release, current, goos, arch string, bundle bool) *Offer {
	base := stable(current)
	if base == nil {
		return nil
	}
	var offer *Offer
	for _, r := range releases {
		v := stable(r.Tag)
		if r.Draft || r.Prerelease || v == nil || !v.GreaterThan(base) {
			continue
		}
		name := "agenttik_" + path.Base(r.Tag) + "_" + goos + "_" + arch
		if goos == "windows" {
			name += ".exe"
		} else if bundle {
			name += ".app.zip"
		}
		for _, a := range r.Assets {
			if a.Name == name && a.Size > 0 && a.Size <= maxDownload && validDigest(a.Digest) && (a.URL == "https://github.com/pausan/agenttik/releases/download/"+r.Tag+"/"+name || a.URL == "https://github.com/pausan/agenttik/releases/download/"+url.PathEscape(r.Tag)+"/"+name) {
				offer = &Offer{Version: r.Tag, Asset: a}
				base = v
				break
			}
		}
	}
	return offer
}
func validDigest(d string) bool {
	if !strings.HasPrefix(d, "sha256:") {
		return false
	}
	b, err := hex.DecodeString(strings.TrimPrefix(d, "sha256:"))
	return err == nil && len(b) == sha256.Size
}
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() Status {
	st := s.status
	// Revalidate persisted metadata before offering or downloading anything.
	if o := s.state.Offer; o != nil && o.Version != s.state.Ignored {
		st.Available = selectOffer([]Release{{Tag: o.Version, Assets: []Asset{o.Asset}}}, s.version, s.goos, s.arch, s.bundle)
	}
	return st
}
func (s *Service) Ignore(version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Busy || s.status.Ready {
		return errors.New("an update is already in progress")
	}
	if s.state.Offer == nil || s.state.Offer.Version != version {
		return errors.New("this update is no longer available")
	}
	previous := s.state.Ignored
	s.state.Ignored = version
	if err := s.save(); err != nil {
		s.state.Ignored = previous
		return err
	}
	s.status.Error = ""
	return nil
}

// Run checks once at startup when due, then sleeps until the next daily check.
// The attempt timestamp is persisted even on network errors to bound retries.
func (s *Service) Run(ctx context.Context) {
	if stable(s.version) == nil {
		return
	}
	for {
		s.mu.Lock()
		due := time.Until(s.state.Checked.Add(24 * time.Hour))
		s.mu.Unlock()
		if due <= 0 || due > 24*time.Hour {
			s.check(ctx)
			due = 24 * time.Hour
		}
		timer := time.NewTimer(due)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (s *Service) check(ctx context.Context) {
	s.mu.Lock()
	s.state.Checked = time.Now()
	if err := s.save(); err != nil {
		log.Printf("update check state: %v", err)
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", s.endpoint, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agenttik/"+s.version)
	res, err := s.client.Do(req)
	if err != nil {
		log.Printf("update check: %v", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("update check: HTTP %d", res.StatusCode)
		return
	}
	var releases []Release
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&releases); err != nil {
		log.Printf("update check: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Busy || s.status.Ready {
		return
	}
	s.state.Offer = selectOffer(releases, s.version, s.goos, s.arch, s.bundle)
	if err := s.save(); err != nil {
		log.Printf("update check state: %v", err)
	}
}

// Start returns promptly; the UI reads progress from Status. No caller can
// supply a download URL or an installation path.
func (s *Service) Start(ctx context.Context, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.statusLocked()
	if s.status.Busy || s.status.Ready {
		return errors.New("an update is already in progress")
	}
	if st.Available == nil || st.Available.Version != version {
		return errors.New("this update is no longer available")
	}
	s.status = Status{Busy: true}
	go func() {
		err := s.prepare(ctx, *st.Available)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.status.Busy = false
		s.status.Ready = err == nil
		if err != nil {
			s.status.Error = err.Error()
		}
	}()
	return nil
}
func (s *Service) prepare(ctx context.Context, offer Offer) error {
	dir, err := os.MkdirTemp(filepath.Dir(s.path), "update-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	archive := filepath.Join(dir, "download")
	if err := s.download(ctx, offer.Asset, archive); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, "installer")
	if s.goos == "windows" {
		helper += ".exe"
	}
	if err := copyFile(executable, helper, 0700); err != nil {
		return err
	}
	plan := Plan{Parent: os.Getpid(), Target: s.target, Bundle: s.bundle, Digest: offer.Asset.Digest, Archive: archive}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	manifest := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		return err
	}
	// A failed launch may still have started an elevated process. Preserve
	// its inputs and leave a cancellation marker whenever readiness fails.
	keep = true
	ready := false
	defer func() {
		if !ready {
			_ = os.WriteFile(manifest+".cancel", []byte("cancelled"), 0600)
		}
	}()
	if err := launchHelper(ctx, helper, manifest, s.target); err != nil {
		return err
	}
	for i := 0; i < 1200; i++ {
		if b, err := os.ReadFile(manifest + ".error"); err == nil {
			return errors.New(string(b))
		}
		if _, err := os.Stat(manifest + ".ready"); err == nil {
			ready = true
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("update helper did not start; check the administrator prompt")
}
func (s *Service) download(ctx context.Context, asset Asset, path string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", res.StatusCode)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(res.Body, asset.Size+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != asset.Size || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != asset.Digest {
		return errors.New("download size or SHA-256 mismatch; executable unchanged")
	}
	return nil
}

// Completed helper files are cleaned on the next launch (Windows cannot
// remove the helper executable while it is running).
func (s *Service) collectResults(dir string) {
	entries, _ := os.ReadDir(dir)
	var latest time.Time
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "update-") {
			continue
		}
		root := filepath.Join(dir, entry.Name())
		manifest := filepath.Join(root, "plan.json")
		for _, suffix := range []string{".error", ".done"} {
			info, err := os.Stat(manifest + suffix)
			if err != nil {
				continue
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
				s.status.Error = ""
				if suffix == ".error" {
					if b, err := os.ReadFile(manifest + suffix); err == nil {
						s.status.Error = "The last update failed: " + string(b)
					}
				}
			}
			_ = os.RemoveAll(root)
			break
		}
	}
}
