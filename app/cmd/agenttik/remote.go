package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pausan/agenttik/app/internal/remote"
)

func runRemote(address string, webOnly bool) error {
	target, info, err := remote.Check(context.Background(), address)
	if err != nil {
		return err
	}
	client := remote.NewClient(http.NotFoundHandler())
	client.Connect(target)
	fmt.Printf("Connecting to agenttik %s at %s\n", info.Version, target)
	if !webOnly {
		err := runRemoteDesktop(client, "agenttik — "+target.Host)
		if !errors.Is(err, errNoDesktop) {
			return err
		}
	}
	// This is a client session, so it must never listen on a network interface.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: client, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("agenttik remote client: http://%s\n", ln.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return srv.Close()
	}
}
