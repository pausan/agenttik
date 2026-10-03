// Package quicktunnel puts agenttik on the internet through a Cloudflare Quick
// Tunnel — the free, account-less https://<words>.trycloudflare.com address
// `cloudflared tunnel --url` hands out — without the cloudflared binary.
//
// It speaks the same protocol cloudflared does with `--protocol http2`: one
// HTTPS call asks the trycloudflare service for a tunnel, then a TLS
// connection to Cloudflare's edge on port 7844 carries HTTP/2 the other way
// round — the edge is the client, this process the server — and its first
// stream registers the connection with one Cap'n Proto RPC. Every later
// stream is a visitor's request, answered by the handler given to New.
package quicktunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// serviceURL is where cloudflared asks for a quick tunnel.
const serviceURL = "https://api.trycloudflare.com/tunnel"

// Credentials are what the service hands back: the tunnel's identity, the
// secret that proves a connection belongs to it, and the public hostname it
// answers on. They are saved so the next run can try the same hostname.
type Credentials struct {
	ID         string `json:"id"`
	AccountTag string `json:"account_tag"`
	Secret     []byte `json:"secret"`
	Hostname   string `json:"hostname"`
}

// URL is the public address visitors open.
func (c Credentials) URL() string { return "https://" + c.Hostname }

// Request asks the trycloudflare service for a brand new tunnel, which comes
// with a brand new random hostname.
func Request(ctx context.Context, userAgent string) (Credentials, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL, bytes.NewReader(nil))
	if err != nil {
		return Credentials{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("request a quick tunnel: %w", err)
	}
	defer resp.Body.Close()
	return decodeResponse(resp.StatusCode, io.LimitReader(resp.Body, 1<<20))
}

func decodeResponse(status int, body io.Reader) (Credentials, error) {
	var data struct {
		Success bool        `json:"success"`
		Result  Credentials `json:"result"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(body).Decode(&data); err != nil {
		return Credentials{}, fmt.Errorf("quick tunnel service answered %d: %w", status, err)
	}
	if len(data.Errors) > 0 {
		return Credentials{}, fmt.Errorf("quick tunnel service: %s (code %d)", data.Errors[0].Message, data.Errors[0].Code)
	}
	if !data.Success || data.Result.ID == "" || data.Result.Hostname == "" || len(data.Result.Secret) == 0 {
		return Credentials{}, fmt.Errorf("quick tunnel service answered %d without a tunnel", status)
	}
	return data.Result, nil
}

// errRejected is a registration the edge refused outright — the tunnel no
// longer exists, typically — as opposed to a network failure worth retrying.
var errRejected = errors.New("edge rejected the tunnel")
