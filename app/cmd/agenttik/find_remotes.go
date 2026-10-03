package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/pausan/agenttik/app/internal/remote"
)

// scanRemotes prints each match as it is found. Only this goroutine writes.
func scanRemotes(ctx context.Context, networks []netip.Prefix, port string, out io.Writer) (int, error) {
	count := 0
	err := remote.Scan(ctx, networks, port, nil, func(target *url.URL, info remote.Info) error {
		if _, err := fmt.Fprintf(out, "%s\t%q\t%s\n", target, info.Name, strconv.Quote(info.Version)); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
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
	networks, err := remote.LocalNetworks()
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
