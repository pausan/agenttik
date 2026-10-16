//go:build !linux && !darwin

package remote

import (
	"context"
	"net/url"
	"sync/atomic"
)

func (s *Scanner) Run(ctx context.Context, port string, probed *atomic.Uint64, found func(*url.URL, Info) error) error {
	return s.runDial(ctx, port, probed, found)
}
