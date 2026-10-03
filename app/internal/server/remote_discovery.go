package server

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/remote"
)

type discoveredMachine struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Version string `json:"version"`
}

type machineDiscovery struct {
	scanner  *remote.Scanner
	port     int
	total    uint64
	probed   atomic.Uint64
	cancel   context.CancelFunc
	done     chan struct{}
	running  bool
	complete bool
	machines []discoveredMachine
	err      string
}

// discoveryMu serializes start, pause and restart. Never wait for a scan
// while holding mu: its result callback needs that lock.
func (m *remoteManager) pauseDiscoveryLocked() {
	m.mu.Lock()
	scan := m.discovery
	if scan == nil || !scan.running {
		m.mu.Unlock()
		return
	}
	scan.cancel()
	done := scan.done
	m.mu.Unlock()
	<-done
}

func (m *remoteManager) stopFindLocked() {
	m.mu.Lock()
	scan := m.scan
	if scan == nil {
		m.mu.Unlock()
		return
	}
	scan.cancel()
	done := scan.done
	m.mu.Unlock()
	<-done
}

func (m *remoteManager) pauseDiscovery() {
	m.discoveryMu.Lock()
	defer m.discoveryMu.Unlock()
	m.pauseDiscoveryLocked()
}

func (m *remoteManager) discoveryState() fiber.Map {
	m.mu.Lock()
	defer m.mu.Unlock()
	scan := m.discovery
	if scan == nil {
		return fiber.Map{"running": false, "machines": []discoveredMachine{}}
	}
	return fiber.Map{
		"running": scan.running, "complete": scan.complete, "port": scan.port,
		"probed": scan.probed.Load(), "total": scan.total,
		"machines": append([]discoveredMachine{}, scan.machines...), "error": scan.err,
	}
}

func (s *Server) discoverMachines(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	var body struct {
		Port    int  `json:"port"`
		Restart bool `json:"restart"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&body); err != nil {
			return badRequest("invalid search")
		}
	}
	if body.Port < 0 || body.Port > 65535 {
		return badRequest("discovery port must be between 1 and 65535")
	}
	m.discoveryMu.Lock()
	defer m.discoveryMu.Unlock()
	m.mu.Lock()
	scan := m.discovery
	m.mu.Unlock()
	if scan != nil && !body.Restart {
		if body.Port != 0 && body.Port != scan.port {
			return badRequest("restart the search to change its port")
		}
		if scan.running || scan.complete {
			return c.JSON(m.discoveryState())
		}
	} else {
		networks, err := m.networks()
		if err != nil {
			return err
		}
		if len(networks) == 0 {
			return badRequest("no active private IPv4 networks found")
		}
		port := body.Port
		if port == 0 {
			port = 7717
		}
		m.pauseDiscoveryLocked()
		scan = &machineDiscovery{scanner: remote.NewScanner(networks), port: port, total: remote.Hosts(networks)}
	}
	self, err := m.self()
	if err != nil {
		return err
	}
	m.stopFindLocked()
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	scan.cancel, scan.done = cancel, make(chan struct{})
	scan.running, scan.err = true, ""
	m.discovery = scan
	m.mu.Unlock()
	go func() {
		defer close(scan.done)
		defer cancel()
		err := scan.scanner.Run(ctx, strconv.Itoa(scan.port), &scan.probed, func(target *url.URL, info remote.Info) error {
			if info.ID == "" || info.ID == self {
				return nil
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			for _, machine := range scan.machines {
				if machine.ID == info.ID {
					return nil
				}
			}
			scan.machines = append(scan.machines, discoveredMachine{info.ID, remoteName(info, target.String()), target.String(), info.Version})
			return nil
		})
		m.mu.Lock()
		defer m.mu.Unlock()
		scan.running = false
		scan.complete = err == nil
		if err != nil && !errors.Is(err, context.Canceled) {
			scan.err = err.Error()
		}
	}()
	return c.Status(202).JSON(m.discoveryState())
}

func (s *Server) discoveryStatus(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	return c.JSON(m.discoveryState())
}

func (s *Server) pauseMachineDiscovery(c *fiber.Ctx) error {
	m, err := s.remoteManager()
	if err != nil {
		return err
	}
	m.pauseDiscovery()
	return c.JSON(m.discoveryState())
}
