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

// DiscoveryNetworks expands private interface addresses to /8, intersected
// with private address space. Deduplication avoids scanning a VPN range twice.
func DiscoveryNetworks(addresses []netip.Addr) []netip.Prefix {
	var networks []netip.Prefix
	seen := make(map[netip.Prefix]bool)
	for _, address := range addresses {
		address = address.Unmap()
		if !address.Is4() || !address.IsPrivate() {
			continue
		}
		bits := 8
		if address.As4()[0] == 172 {
			bits = 12
		}
		if address.As4()[0] == 192 {
			bits = 16
		}
		prefix := netip.PrefixFrom(address, bits).Masked()
		if !seen[prefix] {
			seen[prefix] = true
			networks = append(networks, prefix)
		}
	}
	return networks
}

// LocalNetworks lists the private ranges of this machine's active interfaces.
func LocalNetworks() ([]netip.Prefix, error) {
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
	return DiscoveryNetworks(addresses), nil
}

// Hosts counts the addresses a scan of networks probes.
func Hosts(networks []netip.Prefix) uint64 {
	var n uint64
	for _, network := range networks {
		n += 1 << (32 - network.Bits())
	}
	return n
}

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

// Scan streams addresses through a fixed worker pool. found runs on the
// caller's goroutine; an error from it stops the scan. probed counts completed
// probes, including addresses that do not answer.
func Scan(ctx context.Context, networks []netip.Prefix, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	return NewScanner(networks).Run(ctx, port, probed, found)
}

func (s *Scanner) Run(ctx context.Context, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
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
	for range 128 {
		workers.Go(func() {
			for address := range jobs {
				probeCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
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
		for s.network < len(s.networks) {
			network := s.networks[s.network]
			if !s.next.IsValid() {
				s.next = network.Addr()
			}
			for network.Contains(s.next) {
				if ctx.Err() != nil {
					produced <- nil
					return
				}
				select {
				case jobs <- s.next:
					s.next = s.next.Next()
				case <-ctx.Done():
					produced <- nil
					return
				}
			}
			s.network++
			s.next = netip.Addr{}
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
