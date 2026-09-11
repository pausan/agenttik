package server

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/netauth"
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
	AuthEnabled bool   `json:"auth_enabled"`
	HasPassword bool   `json:"has_password"`
	TOTPSecret  string `json:"totp_secret,omitempty"`
	TOTPURI     string `json:"totp_uri,omitempty"`
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
	info.HasPassword = cfg.PasswordHash != ""
	info.TOTPSecret = cfg.TOTPSecret
	if cfg.TOTPSecret != "" {
		info.TOTPURI = netauth.URI(cfg.TOTPSecret, totpIssuer, totpAccount())
	}
	return info
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
		Enabled    bool   `json:"enabled"`
		Password   string `json:"password"`
		TOTPSecret string `json:"totp_secret"`
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
	if body.Enabled {
		if cfg.PasswordHash == "" {
			return badRequest("set a password before asking for one")
		}
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
