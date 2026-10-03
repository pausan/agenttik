package server

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/quicktunnel"
	"github.com/pausan/agenttik/app/internal/store"
)

// serverConfigInfo is what GET and PUT /api/server both answer: the saved
// setting, defaults filled in, plus whether it is actually listening right
// now and, if it should be but is not, why.
type serverConfigInfo struct {
	// Available is false in web mode, where the process is already the
	// server on whatever address it was started with — there is nothing here
	// to turn on or move.
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Listening bool   `json:"listening"`
	Addr      string `json:"addr,omitempty"`
	Error     string `json:"error,omitempty"`

	// The lock in front of that listener. HasPassword rather than the
	// password: the hash never leaves the process, and whether one is set is
	// the only thing the form needs to know. The seed does travel, because
	// the pane shows it as text to copy and as a QR to scan — both are
	// behind this same lock once it is on.
	TOTPEnabled bool   `json:"totp_enabled"`
	AuthEnabled bool   `json:"auth_enabled"`
	HasPassword bool   `json:"has_password"`
	TOTPSecret  string `json:"totp_secret,omitempty"`
	TOTPURI     string `json:"totp_uri,omitempty"`

	Tunnel *tunnelInfo `json:"tunnel,omitempty"`
}

// tunnelInfo is the public trycloudflare.com address: whether it is wanted,
// and what the tunnel is doing about it right now.
type tunnelInfo struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url,omitempty"`
	Connected bool   `json:"connected"`
	Ready     bool   `json:"ready"`
	Location  string `json:"location,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) getServerConfig(c *fiber.Ctx) error {
	if s.network == nil {
		return c.JSON(serverConfigInfo{Available: false})
	}
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}
	return c.JSON(s.serverConfigInfo(cfg))
}

// putServerConfig applies a host, a port and whether to listen at all, in one
// call: turning the server on with a fresh address and pointing a running one
// at a new address are the same operation. A validation or bind failure
// leaves the saved setting and the live listener exactly as they were.
func (s *Server) putServerConfig(c *fiber.Ctx) error {
	if s.network == nil {
		return badRequest("nothing to configure: this is a web launch, already serving on its own address")
	}
	var body struct {
		Enabled bool   `json:"enabled"`
		Host    string `json:"host"`
		Port    int    `json:"port"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	// Read what is there and change only the address on it: the lock lives
	// in the same row, and answering with a fresh struct would report it as
	// gone every time the port moved.
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}
	host, portStr := s.withDefaults(store.ServerConfig{Host: strings.TrimSpace(body.Host), Port: body.Port})
	port, _ := strconv.Atoi(portStr)
	cfg.Enabled, cfg.Host, cfg.Port = body.Enabled, host, port

	if cfg.Enabled {
		if net.ParseIP(cfg.Host) == nil {
			return badRequest("%q is not an IP address", cfg.Host)
		}
		if cfg.Port < 1 || cfg.Port > 65535 {
			return badRequest("port must be between 1 and 65535")
		}
		if err := s.network.Start(net.JoinHostPort(host, portStr)); err != nil {
			return badRequest("listen on %s: %v", net.JoinHostPort(host, portStr), err)
		}
	} else {
		s.network.Stop()
	}

	if err := s.store.SetServerConfig(cfg); err != nil {
		return err
	}
	return c.JSON(s.serverConfigInfo(cfg))
}

func (s *Server) serverConfigInfo(cfg store.ServerConfig) serverConfigInfo {
	host, portStr := s.withDefaults(cfg)
	port, _ := strconv.Atoi(portStr)
	info := serverConfigInfo{Available: true, Enabled: cfg.Enabled, Host: host, Port: port}
	st := s.network.Status()
	info.Listening, info.Addr, info.Error = st.Listening, st.Addr, st.Error
	info.AuthEnabled = cfg.AuthEnabled
	info.TOTPEnabled = !cfg.TOTPDisabled
	info.HasPassword = cfg.PasswordHash != ""
	info.TOTPSecret = cfg.TOTPSecret
	if cfg.TOTPSecret != "" {
		info.TOTPURI = netauth.URI(cfg.TOTPSecret, totpIssuer, totpAccount())
	}
	ts := s.tunnel.Status()
	info.Tunnel = &tunnelInfo{Enabled: cfg.TunnelEnabled, URL: ts.URL, Connected: ts.Connected, Ready: ts.Ready, Location: ts.Location, Error: ts.Error}
	return info
}

// putServerTunnel turns the public address on or off. Turning it on reuses
// the last tunnel, so the address stays the same, unless renew asks for a new
// one; turning it off keeps that tunnel for next time. Coming up takes
// seconds, so this answers at once and GET /api/server follows it. Even an
// unchanged "on" restarts the connection, which is how a failing one is
// retried now rather than at the end of its backoff.
func (s *Server) putServerTunnel(c *fiber.Ctx) error {
	if s.network == nil {
		return badRequest("nothing to configure: this is a web launch, already serving on its own address")
	}
	var body struct {
		Enabled bool `json:"enabled"`
		Renew   bool `json:"renew"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	// Stopped before reading: a tunnel coming up saves its credentials as it
	// gets them, and must not write "on" back after this turns it off.
	s.tunnel.Stop()
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}
	if body.Renew {
		cfg.TunnelCredentials = ""
	}
	cfg.TunnelEnabled = body.Enabled
	if err := s.store.SetServerTunnel(cfg.TunnelEnabled, cfg.TunnelCredentials); err != nil {
		return err
	}
	if cfg.TunnelEnabled {
		s.startTunnel(cfg.TunnelCredentials)
	}
	return c.JSON(s.serverConfigInfo(cfg))
}

// startTunnel brings the tunnel up on saved, the JSON of the last one handed
// out, or a new one when that is blank or unreadable; whichever tunnel it
// ends up on is saved for next time.
func (s *Server) startTunnel(saved string) {
	var creds quicktunnel.Credentials
	if saved != "" {
		if err := json.Unmarshal([]byte(saved), &creds); err != nil {
			creds = quicktunnel.Credentials{}
		}
	}
	s.tunnel.Start(creds, func(fresh quicktunnel.Credentials) {
		b, err := json.Marshal(fresh)
		if err == nil {
			err = s.store.SetServerTunnel(true, string(b))
		}
		if err != nil {
			log.Printf("server: save tunnel: %v", err)
		}
	})
}

// totpIssuer is the name the authenticator app files the entry under, and
// totpAccount tells one machine's agenttik from another's in that same list.
const totpIssuer = "agenttik"

func totpAccount() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "this machine"
}

// putServerAuth sets the lock on the exposed server: whether it is asked for
// at all, the password, and the authenticator seed. A blank password or seed
// in the body means "leave that one alone", so changing one does not require
// resending the other; turning the lock on for the first time rolls a seed
// rather than making the user find one.
//
// Any accepted change ends every browser session already open. Whoever was
// signed in got there with a secret that is being replaced, and replacing one
// is usually a response to somebody else knowing it.
func (s *Server) putServerAuth(c *fiber.Ctx) error {
	if s.network == nil {
		return badRequest("nothing to configure: this is a web launch, already serving on its own address")
	}
	var body struct {
		Enabled     bool   `json:"enabled"`
		TOTPEnabled *bool  `json:"totp_enabled"`
		Password    string `json:"password"`
		TOTPSecret  string `json:"totp_secret"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}

	if pw := body.Password; pw != "" {
		hash, err := netauth.HashPassword(pw)
		if err != nil {
			return badRequest("%v", err)
		}
		cfg.PasswordHash = hash
	}
	if seed := strings.TrimSpace(body.TOTPSecret); seed != "" {
		secret, err := netauth.NormalizeSecret(seed)
		if err != nil {
			return badRequest("%v", err)
		}
		cfg.TOTPSecret = secret
	}
	if body.TOTPEnabled != nil {
		cfg.TOTPDisabled = !*body.TOTPEnabled
	}
	if body.Enabled && !cfg.TOTPDisabled {
		if cfg.TOTPSecret == "" {
			secret, err := netauth.NewSecret()
			if err != nil {
				return err
			}
			cfg.TOTPSecret = secret
		}
	}
	cfg.AuthEnabled = body.Enabled

	if err := s.store.SetServerAuth(cfg); err != nil {
		return err
	}
	s.reloadCredentials()
	s.auth.Revoke()
	return c.JSON(s.serverConfigInfo(cfg))
}

// resetServerTOTP rolls a fresh random seed, which is the other half of being
// able to type one in: a seed that has been seen by the wrong person is
// replaced here rather than invented by hand.
func (s *Server) resetServerTOTP(c *fiber.Ctx) error {
	if s.network == nil {
		return badRequest("nothing to configure: this is a web launch, already serving on its own address")
	}
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}
	secret, err := netauth.NewSecret()
	if err != nil {
		return err
	}
	cfg.TOTPSecret = secret
	if err := s.store.SetServerAuth(cfg); err != nil {
		return err
	}
	s.reloadCredentials()
	s.auth.Revoke()
	return c.JSON(s.serverConfigInfo(cfg))
}

// serverTOTPQR draws the current seed as the QR an authenticator app scans.
// It is an image rather than a data URI in the JSON above so that re-reading
// the setting — which the pane does on every change — does not carry a few
// kilobytes of PNG with it.
func (s *Server) serverTOTPQR(c *fiber.Ctx) error {
	if s.network == nil {
		return badRequest("nothing to configure: this is a web launch, already serving on its own address")
	}
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}
	if cfg.TOTPSecret == "" {
		return fiber.NewError(fiber.StatusNotFound, "no authenticator seed has been set")
	}
	png, err := netauth.QRPNG(netauth.URI(cfg.TOTPSecret, totpIssuer, totpAccount()))
	if err != nil {
		return err
	}
	c.Type("png")
	c.Set("Cache-Control", "no-store")
	return c.Send(png)
}

// serverTOTPCode uses the server clock, just like login verification.
func (s *Server) serverTOTPCode(c *fiber.Ctx) error {
	if s.network == nil {
		return badRequest("nothing to configure: this is a web launch")
	}
	cfg, err := s.store.GetServerConfig()
	if err != nil {
		return err
	}
	if cfg.TOTPSecret == "" {
		return fiber.NewError(fiber.StatusNotFound, "no authenticator seed has been set")
	}
	now := time.Now()
	code, err := netauth.Code(cfg.TOTPSecret, now)
	if err != nil {
		return err
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(fiber.Map{"code": code, "refresh_after_ms": now.Truncate(netauth.Period).Add(netauth.Period).Sub(now).Milliseconds()})
}

// putServerName is also available in web mode; it does not change the listener.
func (s *Server) putServerName(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	name := strings.TrimSpace(body.Name)
	if utf8.RuneCountInString(name) < 3 {
		return badRequest("server name must contain at least 3 characters")
	}
	if err := s.store.SetServerName(name); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"name": name})
}
