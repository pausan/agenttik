package netserver

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newManager proxies to a backend that answers "ok", and returns a port
// nothing is listening on.
func newManager(t *testing.T) (*Manager, string) {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "ok")
	}))
	t.Cleanup(backend.Close)
	m := New(strings.TrimPrefix(backend.URL, "http://"))
	t.Cleanup(func() { m.Stop() })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	return m, port
}

func get(t *testing.T, addr string) {
	t.Helper()
	res, err := http.Get("http://" + addr)
	if err != nil {
		t.Fatalf("GET %s: %v", addr, err)
	}
	defer res.Body.Close()
	if body, _ := io.ReadAll(res.Body); string(body) != "ok" {
		t.Fatalf("GET %s = %q, want ok", addr, body)
	}
}

func TestStartMovesHostOnSamePort(t *testing.T) {
	m, port := newManager(t)
	if err := m.Start("127.0.0.1:" + port); err != nil {
		t.Fatal(err)
	}
	if err := m.Start("0.0.0.0:" + port); err != nil {
		t.Fatalf("move to 0.0.0.0: %v", err)
	}
	get(t, "127.0.0.1:"+port)
	if err := m.Start("127.0.0.1:" + port); err != nil {
		t.Fatalf("move back to 127.0.0.1: %v", err)
	}
	get(t, "127.0.0.1:"+port)
}

func TestStartSameAddressAgain(t *testing.T) {
	m, port := newManager(t)
	for i := 0; i < 2; i++ {
		if err := m.Start("127.0.0.1:" + port); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
	}
	get(t, "127.0.0.1:"+port)
}

func TestStartFailedSamePortMoveKeepsOld(t *testing.T) {
	m, port := newManager(t)
	if err := m.Start("127.0.0.1:" + port); err != nil {
		t.Fatal(err)
	}
	// 192.0.2.1 is reserved for documentation, so no machine has it.
	if err := m.Start("192.0.2.1:" + port); err == nil {
		t.Fatal("bind to an address not on this machine succeeded")
	}
	st := m.Status()
	if !st.Listening || st.Addr != "127.0.0.1:"+port || st.Error == "" {
		t.Fatalf("status = %+v, want still on 127.0.0.1:%s with the error", st, port)
	}
	get(t, "127.0.0.1:"+port)
}

func TestStartTakenPortKeepsOld(t *testing.T) {
	m, port := newManager(t)
	if err := m.Start("127.0.0.1:" + port); err != nil {
		t.Fatal(err)
	}
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	if err := m.Start(taken.Addr().String()); err == nil {
		t.Fatal("bind to a taken port succeeded")
	}
	get(t, "127.0.0.1:"+port)
}
