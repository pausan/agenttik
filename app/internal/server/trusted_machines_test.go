package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func TestTrustedMachineSignsInWithItsKey(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret password"), bcrypt.MinCost)
	secret, _ := netauth.NewSecret()
	if err := remoteStore.SetServerAuth(store.ServerConfig{AuthEnabled: true, PasswordHash: string(hash), TOTPSecret: secret}); err != nil {
		t.Fatal(err)
	}
	remoteSrv.reloadCredentials()
	handler := remoteSrv.auth.Wrap(adaptor.FiberApp(remoteSrv.app))
	hs := httptest.NewServer(handler)
	defer hs.Close()
	local, localStore := newTestServer(t)
	id := machineID(t, remoteStore)

	status := func() int {
		resp := do(t, local, "GET", "/api/projects?remote="+id, nil)
		resp.Body.Close()
		return resp.StatusCode
	}
	trusted := func() []store.TrustedMachine {
		return decode[struct{ Machines []store.TrustedMachine }](t, do(t, remoteSrv, "GET", "/api/server/trusted", nil)).Machines
	}

	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": hs.URL})); state.Status != "signin" {
		t.Fatalf("%+v", state)
	}
	code, _ := netauth.Code(secret, time.Now())
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/login", map[string]string{"password": "secret password", "code": code})); state.Status != "ok" {
		t.Fatalf("%+v", state)
	}
	list := trusted()
	localName, _ := localStore.ServerName()
	if len(list) != 1 || list[0].MachineID != machineID(t, localStore) || list[0].Name != localName || list[0].Address != "127.0.0.1" {
		t.Fatalf("trusted: %+v", list)
	}

	// The remote restarting ends the session; the key opens a new one.
	remoteSrv.auth.Revoke()
	if got := status(); got != 200 {
		t.Fatalf("after revoke: %d", got)
	}

	// So does reaching the remote at another address.
	moved := httptest.NewServer(handler)
	defer moved.Close()
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": moved.URL})); state.Status != "ok" {
		t.Fatalf("moved: %+v", state)
	}
	remoteSrv.auth.Revoke()
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/connect", nil)); state.Status != "ok" {
		t.Fatalf("reconnect at the new address: %+v", state)
	}

	// Removing it on the remote ends its session and asks for the password.
	resp := do(t, remoteSrv, "DELETE", "/api/server/trusted/"+list[0].Key, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 || len(trusted()) != 0 {
		t.Fatalf("forget: %d", resp.StatusCode)
	}
	if got := status(); got != 401 {
		t.Fatalf("after removal: %d", got)
	}
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/connect", nil)); state.Status != "signin" {
		t.Fatalf("%+v", state)
	}

	// Signing in again, here with the password alone, trusts it again.
	if err := remoteStore.SetServerAuth(store.ServerConfig{AuthEnabled: true, PasswordHash: string(hash), TOTPSecret: secret, TOTPDisabled: true}); err != nil {
		t.Fatal(err)
	}
	remoteSrv.reloadCredentials()
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes/"+id+"/login", map[string]string{"password": "secret password"})); state.Status != "ok" {
		t.Fatalf("%+v", state)
	}
	remoteSrv.auth.Revoke()
	if got := status(); got != 200 || len(trusted()) != 1 {
		t.Fatalf("trusted again: %d", got)
	}
}
