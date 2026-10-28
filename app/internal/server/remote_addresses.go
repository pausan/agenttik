package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/remote"
)

// A machine can be reached on more than one address: its local network
// address at home, its public one from anywhere else. Each saved remote keeps
// the ones that have worked, and the ones it reports, and every connection
// picks the best that answers.
const (
	maxRemoteAddresses = 8
	// remoteStagger is how long one address gets before the next is tried
	// as well. A local address answers well within it.
	remoteStagger = 300 * time.Millisecond
	// remoteGrace is how long better addresses still get once a worse one
	// has answered.
	remoteGrace = time.Second
)

var errOtherMachine = errors.New("another agenttik answers there")

// serverAddresses lists the addresses other machines can reach this one on:
// its listeners on every non-loopback IPv4 interface, then the public tunnel.
func (s *Server) serverAddresses(c *fiber.Ctx) error {
	var listeners []string
	if s.listenAddr != "" {
		listeners = append(listeners, s.listenAddr)
	}
	if s.network != nil {
		if st := s.network.Status(); st.Listening {
			listeners = append(listeners, st.Addr)
		}
	}
	interfaces, _ := remote.InterfaceAddrs()
	addresses := listenerAddresses(listeners, interfaces)
	if s.tunnel != nil {
		if st := s.tunnel.Status(); st.Ready && st.URL != "" {
			addresses = append(addresses, st.URL)
		}
	}
	return c.JSON(fiber.Map{"addresses": addresses})
}

// listenerAddresses turns listeners into URLs. A listener on every interface
// is listed once per interface address.
func listenerAddresses(listeners []string, interfaces []netip.Addr) []string {
	addresses := []string{}
	add := func(ip netip.Addr, port uint16) {
		ip = ip.Unmap()
		if !ip.Is4() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return
		}
		u := "http://" + netip.AddrPortFrom(ip, port).String()
		if !slices.Contains(addresses, u) {
			addresses = append(addresses, u)
		}
	}
	for _, listener := range listeners {
		ap, err := netip.ParseAddrPort(listener)
		if err != nil {
			continue
		}
		if !ap.Addr().IsUnspecified() {
			add(ap.Addr(), ap.Port())
			continue
		}
		for _, ip := range interfaces {
			add(ip, ap.Port())
		}
	}
	return addresses
}

// localAddress tells a local network address from a public one: private,
// loopback and link-local IPs, and names without a domain or under a local
// one.
func localAddress(address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	return !strings.Contains(host, ".") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home.arpa")
}

// known lists the remote's addresses, the one in use first, then the others
// from the most recently reached.
func (r savedRemote) known() []string {
	list := []string{r.Address}
	for _, a := range r.Addresses {
		if !slices.Contains(list, a) {
			list = append(list, a)
		}
	}
	return list
}

// candidates is the order addresses are tried in: local networks first, then
// the most recently reached.
func (r savedRemote) candidates() []string {
	list := r.known()
	slices.SortStableFunc(list, func(a, b string) int {
		la, lb := localAddress(a), localAddress(b)
		switch {
		case la && !lb:
			return -1
		case lb && !la:
			return 1
		}
		return 0
	})
	return list
}

// use makes address the one in use and the most recent, keeping at most
// maxRemoteAddresses.
func (r *savedRemote) use(address string) {
	r.Address = address
	r.Addresses = r.known()
	if len(r.Addresses) > maxRemoteAddresses {
		r.Addresses = r.Addresses[:maxRemoteAddresses]
	}
}

// learn adds addresses the remote reports after the known ones.
func (r *savedRemote) learn(addresses []string) {
	list := r.known()
	for _, a := range addresses {
		if len(list) >= maxRemoteAddresses {
			break
		}
		if u, err := remote.Parse(a); err == nil && !slices.Contains(list, u.String()) {
			list = append(list, u.String())
		}
	}
	r.Addresses = list
}

// locate tries the remote's addresses in preference order, starting the
// next one when the last has failed or has had remoteStagger, and answers
// the best where the machine itself answers. Once one answers, better ones
// still pending get remoteGrace. With none, it answers errOtherMachine if
// another agenttik answered somewhere, else the best address's error.
func locate(ctx context.Context, r savedRemote) (string, remote.Info, error) {
	candidates := r.candidates()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		i    int
		info remote.Info
		err  error
	}
	results := make(chan result, len(candidates))
	started, pending := 0, 0
	start := func() {
		i, address := started, candidates[started]
		started++
		pending++
		go func() {
			_, info, err := remote.Check(ctx, address)
			if err == nil && info.ID != r.ID {
				err = errOtherMachine
			}
			results <- result{i, info, err}
		}()
	}
	start()
	stagger := time.NewTimer(remoteStagger)
	defer stagger.Stop()
	var grace <-chan time.Time
	best, failed := -1, make([]error, len(candidates))
	var bestInfo remote.Info
	for pending > 0 {
		select {
		case res := <-results:
			pending--
			if res.err != nil {
				failed[res.i] = res.err
			} else if best < 0 || res.i < best {
				best, bestInfo = res.i, res.info
				if grace == nil {
					grace = time.After(remoteGrace)
				}
			}
		case <-stagger.C:
		case <-grace:
			return candidates[best], bestInfo, nil
		}
		if best == 0 || best > 0 && !slices.Contains(failed[:best], nil) {
			return candidates[best], bestInfo, nil
		}
		if started < len(candidates) && best < 0 {
			start()
			stagger.Reset(remoteStagger)
		}
	}
	if best >= 0 {
		return candidates[best], bestInfo, nil
	}
	if slices.ContainsFunc(failed, func(err error) bool { return errors.Is(err, errOtherMachine) }) {
		return "", remote.Info{}, errOtherMachine
	}
	return "", remote.Info{}, failed[0]
}

// reach finds the best address of a saved remote and, when it changed, saves
// it as the one in use.
func (m *remoteManager) reach(ctx context.Context, r savedRemote) (savedRemote, remote.Info, error) {
	address, info, err := locate(ctx, r)
	m.noteReach(ctx, r.ID, err)
	if err != nil {
		return r, info, err
	}
	if address == r.Address {
		return r, info, nil
	}
	updated, err := m.update(r.ID, func(saved *savedRemote) { saved.use(address) })
	return updated, info, err
}

// learnAddresses asks a connected remote where else it can be reached.
// Servers too old to say answer 404 and keep the addresses already known.
func (m *remoteManager) learnAddresses(ctx context.Context, r savedRemote) savedRemote {
	res, err := m.request(ctx, r, "GET", "/api/server/addresses", nil, http.Header{"Accept": {"application/json"}})
	if err != nil {
		return r
	}
	defer res.Body.Close()
	var list struct {
		Addresses []string `json:"addresses"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&list) != nil {
		return r
	}
	next := r
	next.learn(list.Addresses)
	if slices.Equal(next.Addresses, r.Addresses) {
		return r
	}
	if updated, err := m.update(r.ID, func(saved *savedRemote) { saved.learn(list.Addresses) }); err == nil {
		return updated
	}
	return r
}
