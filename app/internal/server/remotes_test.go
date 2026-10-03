package server

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// listen serves s on a loopback port, as a remote machine would.
func listen(t *testing.T, s *Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go s.Listener(ln)
	t.Cleanup(func() { s.Shutdown() })
	return ln.Addr().String()
}

func machineID(t *testing.T, st *store.Store) string {
	t.Helper()
	id, err := st.MachineID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRemoteProfilesForwardAPIAndPersist(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	resp := do(t, remoteSrv, "POST", "/api/projects", map[string]string{"path": t.TempDir(), "name": "On the remote"})
	resp.Body.Close()
	address := listen(t, remoteSrv)
	local, localStore := newTestServer(t)

	state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address}))
	id := machineID(t, remoteStore)
	if state.Status != "ok" || state.Remote.ID != id || len(state.Remote.Profiles) != 1 || state.Remote.Profiles[0].ID != "default" {
		t.Fatalf("%+v", state)
	}
	projects := func(path string) string {
		resp := do(t, local, "GET", path, nil)
		defer resp.Body.Close()
		var b strings.Builder
		bufio.NewReader(resp.Body).WriteTo(&b)
		return b.String()
	}
	if got := projects("/api/projects?remote=" + id); !strings.Contains(got, "On the remote") {
		t.Fatalf("remote projects: %s", got)
	}
	if got := projects("/api/projects"); strings.Contains(got, "On the remote") {
		t.Fatal("remote project listed locally")
	}
	// The catalog and the window stay on this machine.
	if got := projects("/api/profiles?remote=" + id + "&profile=default"); !strings.Contains(got, `"private":false`) {
		t.Fatalf("profiles left this machine: %s", got)
	}
	resp = do(t, local, "GET", "/api/projects?remote=unknown", nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown remote: %d", resp.StatusCode)
	}

	// A new manager over the same directory finds the remote without asking it.
	again := newRemoteManager(localStore.Dir(), localStore.MachineID)
	if r, err := again.get(id); err != nil || r.Name == "" || len(r.Profiles) != 1 {
		t.Fatalf("saved remote: %+v %v", r, err)
	}

	resp = do(t, local, "POST", "/api/remotes", map[string]string{"address": listen(t, local)})
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("added itself: %d", resp.StatusCode)
	}
	resp = do(t, local, "DELETE", "/api/remotes/"+id, nil)
	resp.Body.Close()
	if list := decode[struct{ Remotes []remoteView }](t, do(t, local, "GET", "/api/remotes", nil)); resp.StatusCode != 204 || len(list.Remotes) != 0 {
		t.Fatalf("delete: %d %+v", resp.StatusCode, list)
	}
}

func TestRemoteSignInKeepsSessionOnThisMachine(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret password"), bcrypt.MinCost)
	secret, _ := netauth.NewSecret()
	gate := netauth.New(func() netauth.Credentials {
		return netauth.Credentials{Enabled: true, PasswordHash: string(hash), Secret: secret}
	})
	hs := httptest.NewServer(gate.Wrap(adaptor.FiberApp(remoteSrv.app)))
	defer hs.Close()
	local, localStore := newTestServer(t)
	id := machineID(t, remoteStore)

	state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": hs.URL}))
	if state.Status != "signin" || !state.CodeRequired {
		t.Fatalf("%+v", state)
	}
	resp := do(t, local, "GET", "/api/projects?remote="+id, nil)
	resp.Body.Close()
	if resp.StatusCode != 401 || resp.Header.Get("X-Agenttik-Remote-Auth") != "required" {
		t.Fatalf("unsigned request: %d", resp.StatusCode)
	}
	code, _ := netauth.Code(secret, time.Now())
	resp = do(t, local, "POST", "/api/remotes/"+id+"/login", map[string]string{"password": "wrong", "code": code})
	if body := decode[map[string]string](t, resp); resp.StatusCode != 401 || body["error"] != "Wrong password or code." {
		t.Fatalf("wrong password: %d %v", resp.StatusCode, body)
	}
	state = decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/login", map[string]string{"password": "secret password", "code": code}))
	if state.Status != "ok" || !state.Remote.SignedIn {
		t.Fatalf("%+v", state)
	}
	resp = do(t, local, "GET", "/api/projects?remote="+id, nil)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Set-Cookie") != "" {
		t.Fatalf("signed request: %d %q", resp.StatusCode, resp.Header.Get("Set-Cookie"))
	}
	resp = do(t, local, "GET", "/api/remotes", nil)
	var b strings.Builder
	bufio.NewReader(resp.Body).WriteTo(&b)
	resp.Body.Close()
	if strings.Contains(b.String(), "cookie") {
		t.Fatalf("session reached the browser: %s", b.String())
	}
	saved, _ := os.ReadFile(localStore.Dir() + "/remotes.json")
	if info, _ := os.Stat(localStore.Dir() + "/remotes.json"); !strings.Contains(string(saved), "agenttik_session") || info.Mode().Perm() != 0600 {
		t.Fatal("session was not kept privately")
	}
}

func TestRemoteMovedAndFoundAgain(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	address := listen(t, remoteSrv)
	other, _ := newTestServer(t)
	otherAddress := listen(t, other)
	local, _ := newTestServer(t)
	id := machineID(t, remoteStore)
	decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address}))

	local.remotes.update(id, func(r *savedRemote) { r.Address = "http://" + otherAddress })
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/connect", nil)); state.Status != "moved" {
		t.Fatalf("%+v", state)
	}

	_, port, _ := net.SplitHostPort(address)
	local.remotes.update(id, func(r *savedRemote) { r.Address = "http://192.0.2.1:" + port })
	local.remotes.networks = func() ([]netip.Prefix, error) {
		return []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}, nil
	}
	resp := do(t, local, "POST", "/api/remotes/"+id+"/find", nil)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("find: %d", resp.StatusCode)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := decode[map[string]any](t, do(t, local, "GET", "/api/remotes/"+id+"/find", nil))
		if status["running"] == false {
			if status["found"] != "http://"+address {
				t.Fatalf("%v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scan did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/connect", nil)); state.Status != "ok" {
		t.Fatalf("%+v", state)
	}
}

func TestRemoteStreamFlushesThroughThisMachine(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	address := listen(t, remoteSrv)
	local, _ := newTestServer(t)
	decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address}))
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + listen(t, local) + "/api/stream?remote=" + machineID(t, remoteStore))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != ": open\n" {
		t.Fatalf("first line %q: %v", line, err)
	}
}
