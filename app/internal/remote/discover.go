package remote

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// DiscoveryNetworks lists the ranges around private interface addresses from
// the most local to the widest: every /24, then every /16, then the /12 or /8
// private block. 192.168 stops at /16. Deduplication avoids scanning a VPN
// range twice, and the scanner skips addresses an earlier range covered.
func DiscoveryNetworks(addresses []netip.Addr) []netip.Prefix {
	var networks []netip.Prefix
	seen := make(map[netip.Prefix]bool)
	for _, bits := range []int{24, 16, 12, 8} {
		for _, address := range addresses {
			address = address.Unmap()
			if !address.Is4() || !address.IsPrivate() {
				continue
			}
			widest := 8
			switch address.As4()[0] {
			case 172:
				widest = 12
			case 192:
				widest = 16
			}
			if bits < 16 && bits != widest {
				continue
			}
			prefix := netip.PrefixFrom(address, bits).Masked()
			if !seen[prefix] {
				seen[prefix] = true
				networks = append(networks, prefix)
			}
		}
	}
	return networks
}

// LocalNetworks lists the private ranges of this machine's active interfaces.
func LocalNetworks() ([]netip.Prefix, error) {
	addresses, err := InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	return DiscoveryNetworks(addresses), nil
}

// InterfaceAddrs lists the addresses of this machine's up, non-loopback
// interfaces.
func InterfaceAddrs() ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addresses []netip.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			prefix, err := netip.ParsePrefix(addr.String())
			if err == nil {
				addresses = append(addresses, prefix.Addr())
			}
		}
	}
	return addresses, nil
}

// NetworkHosts counts, for each network, the addresses no earlier network
// covers: the addresses a scan probes while working through that network.
func NetworkHosts(networks []netip.Prefix) []uint64 {
	hosts := make([]uint64, len(networks))
	for i, network := range networks {
		hosts[i] = 1 << (32 - network.Bits())
		for j, earlier := range networks[:i] {
			if earlier.Bits() <= network.Bits() && earlier.Contains(network.Addr()) {
				hosts[i] = 0
				break
			}
			if network.Contains(earlier.Addr()) {
				hosts[i] -= hosts[j]
			}
		}
	}
	return hosts
}

// Hosts counts the addresses a scan of networks probes.
func Hosts(networks []netip.Prefix) uint64 {
	var n uint64
	for _, hosts := range NetworkHosts(networks) {
		n += hosts
	}
	return n
}

// probeTimeout bounds each address of a scan, including its dial.
const probeTimeout = 500 * time.Millisecond

// probesInFlight bounds the addresses a scan probes at once.
const probesInFlight = 2048

// Scanner keeps the next address and a bounded list of unfinished probes.
// Run calls must be sequential. Cancellation leaves unfinished probes for the
// next Run, while completed probes and matches are retained by the caller.
type Scanner struct {
	networks []netip.Prefix
	network  int
	next     netip.Addr
	retry    []netip.Addr
}

func NewScanner(networks []netip.Prefix) *Scanner {
	return &Scanner{networks: networks}
}

// Scan probes up to probesInFlight addresses at once. found runs on the
// caller's goroutine; an error from it stops the scan. probed counts completed
// probes, including addresses that do not answer.
func Scan(ctx context.Context, networks []netip.Prefix, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	return NewScanner(networks).Run(ctx, port, probed, found)
}

// runDial gives each probe in flight its own goroutine and dial. It works on
// every platform; Run uses it where no poller is available.
func (s *Scanner) runDial(ctx context.Context, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		address     netip.Addr
		target      *url.URL
		info        Info
		err         error
		interrupted bool
	}
	jobs := make(chan netip.Addr)
	results := make(chan result)
	retry := s.retry
	s.retry = nil
	produced := make(chan []netip.Addr, 1)
	var workers sync.WaitGroup
	for range probesInFlight {
		workers.Go(func() {
			for address := range jobs {
				probeCtx, stop := context.WithTimeout(ctx, probeTimeout)
				target, info, err := CheckDirect(probeCtx, net.JoinHostPort(address.String(), port))
				interrupted := err != nil && probeCtx.Err() == context.Canceled
				stop()
				// Always drain results, even on cancellation, so no dispatched address
				// or successful match is lost when the scan pauses.
				results <- result{address, target, info, err, interrupted}
			}
		})
	}
	go func() {
		defer close(jobs)
		for i, address := range retry {
			if ctx.Err() != nil {
				produced <- retry[i:]
				return
			}
			select {
			case jobs <- address:
			case <-ctx.Done():
				produced <- retry[i:]
				return
			}
		}
		for {
			address, ok := s.nextAddress()
			if !ok {
				break
			}
			if ctx.Err() != nil {
				produced <- []netip.Addr{address}
				return
			}
			select {
			case jobs <- address:
			case <-ctx.Done():
				produced <- []netip.Addr{address}
				return
			}
		}
		produced <- nil
	}()
	go func() { workers.Wait(); close(results) }()
	var stopErr error
	for r := range results {
		if r.interrupted || stopErr != nil {
			s.retry = append(s.retry, r.address)
			continue
		}
		if probed != nil {
			probed.Add(1)
		}
		if r.err == nil {
			if err := found(r.target, r.info); err != nil {
				stopErr = err
				cancel()
			}
		}
	}
	s.retry = append(s.retry, (<-produced)...)
	if stopErr != nil {
		return stopErr
	}
	return ctx.Err()
}

// nextAddress advances the network cursor past addresses an earlier network
// covered. Addresses it returns are the caller's to probe or retry.
func (s *Scanner) nextAddress() (netip.Addr, bool) {
	for s.network < len(s.networks) {
		network := s.networks[s.network]
		if !s.next.IsValid() {
			s.next = network.Addr()
		}
		for network.Contains(s.next) {
			address := s.next
			s.next = s.next.Next()
			if !s.scanned(address) {
				return address, true
			}
		}
		s.network++
		s.next = netip.Addr{}
	}
	return netip.Addr{}, false
}

// scanned reports whether a network before the current one holds address.
func (s *Scanner) scanned(address netip.Addr) bool {
	for _, network := range s.networks[:s.network] {
		if network.Contains(address) {
			return true
		}
	}
	return false
}
