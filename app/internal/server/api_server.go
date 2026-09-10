package server

import (
	"net"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

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
	host, portStr := s.withDefaults(store.ServerConfig{Host: strings.TrimSpace(body.Host), Port: body.Port})
	port, _ := strconv.Atoi(portStr)
	cfg := store.ServerConfig{Enabled: body.Enabled, Host: host, Port: port}

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
	return info
}
