package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/pausan/agenttik/app/internal/remote"
)

// discoveryNetworks expands private interface addresses to /8, intersected
// with private address space. Deduplication avoids scanning a VPN range twice.
func discoveryNetworks(addresses []netip.Addr) []netip.Prefix {
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

func localDiscoveryNetworks() ([]netip.Prefix, error) {
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
	return discoveryNetworks(addresses), nil
}

// scanRemotes streams addresses through a fixed worker pool, keeping memory
// and open sockets bounded even for a /8. Only the caller writes output.
func scanRemotes(ctx context.Context, networks []netip.Prefix, port string, out io.Writer) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan netip.Addr)
	results := make(chan string)
	var workers sync.WaitGroup
	for range 128 {
		workers.Go(func() {
			for address := range jobs {
				probeCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
				target, info, err := remote.CheckDirect(probeCtx, net.JoinHostPort(address.String(), port))
				stop()
				if err != nil {
					continue
				}
				result := fmt.Sprintf("%s\t%q\t%s\n", target, info.Name, strconv.Quote(info.Version))
				select {
				case results <- result:
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
	count := 0
	var writeErr error
	for result := range results {
		if writeErr != nil {
			continue
		}
		if _, err := io.WriteString(out, result); err != nil {
			writeErr = err
			cancel()
			continue
		}
		count++
	}
	if writeErr != nil {
		return count, writeErr
	}
	return count, ctx.Err()
}

func runFindRemotes(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid --addr: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return errors.New("discovery port must be between 1 and 65535")
	}
	networks, err := localDiscoveryNetworks()
	if err != nil {
		return err
	}
	if len(networks) == 0 {
		return errors.New("no active private IPv4 networks found")
	}
	fmt.Fprintf(os.Stderr, "Scanning %v on port %s; a /8 can take hours. Press Ctrl+C to stop.\n", networks, port)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	count, err := scanRemotes(ctx, networks, port, os.Stdout)
	fmt.Fprintf(os.Stderr, "Found %d agenttik instance(s).\n", count)
	return err
}
