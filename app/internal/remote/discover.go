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

// Scan streams addresses through a fixed worker pool, keeping memory and open
// sockets bounded even for a /8. found runs on the caller's goroutine, one
// match at a time; an error from it stops the scan and is returned. probed,
// when given, counts finished probes for progress.
func Scan(ctx context.Context, networks []netip.Prefix, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type match struct {
		target *url.URL
		info   Info
	}
	jobs := make(chan netip.Addr)
	results := make(chan match)
	var workers sync.WaitGroup
	for range 128 {
		workers.Go(func() {
			for address := range jobs {
				probeCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
				target, info, err := CheckDirect(probeCtx, net.JoinHostPort(address.String(), port))
				stop()
				if probed != nil {
					probed.Add(1)
				}
				if err != nil {
					continue
				}
				select {
				case results <- match{target, info}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, network := range networks {
			for address := network.Addr(); network.Contains(address); address = address.Next() {
				select {
				case jobs <- address:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	var stopErr error
	for m := range results {
		if stopErr != nil {
			continue
		}
		if err := found(m.target, m.info); err != nil {
			stopErr = err
			cancel()
		}
	}
	if stopErr != nil {
		return stopErr
	}
	return ctx.Err()
}
