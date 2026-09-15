package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/pausan/agenttik/app/internal/netauth"
	"github.com/pausan/agenttik/app/internal/remote"
)

func TestVersionBeforeLogin(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetVersion("1.2.3")
	gate := netauth.New(func() netauth.Credentials { return netauth.Credentials{Enabled: true} })
	handler := gate.Wrap(adaptor.FiberApp(srv.app))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", remote.VersionPath, nil))
	if rec.Code != 200 {
		t.Fatalf("version: %d", rec.Code)
	}
	info := decode[remote.Info](t, rec.Result())
	if info.Application != "agenttik" || info.Version != "1.2.3" {
		t.Fatalf("%+v", info)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("version is cacheable")
	}
	for _, path := range []string{"/api/projects", "/api/server/auth/code"} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 401 {
			t.Fatalf("%s bypassed login: %d", path, rec.Code)
		}
	}
}
