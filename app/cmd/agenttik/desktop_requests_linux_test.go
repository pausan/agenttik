//go:build desktop && linux

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Check the ownership rule directly. Waiting for the race to produce a native
// crash is not a reliable regression assertion.
func TestDesktopRequestRelease(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("requires Xvfb")
	}
	pkg := "webkit2gtk-4.1"
	if err := exec.Command("pkg-config", "--exists", pkg).Run(); err != nil {
		pkg = "webkit2gtk-4.0"
	}
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", pkg).Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "release.c")
	const fixture = `#include "desktop_requests_linux.c"

static gboolean finalized;
static gboolean on_main;
static gint call_done;
static gboolean call_valid;
static void on_gtk(gpointer data) {
    call_valid = g_main_context_is_owner(g_main_context_default()) && *(int *)data == 7;
    *(int *)data = 42;
}
static gpointer invoke_worker(gpointer unused) {
    int value = 7;
    call_on_gtk(on_gtk, &value);
    call_valid &= value == 42;
    g_atomic_int_set(&call_done, TRUE);
    return NULL;
}
static void destroyed(gpointer unused, GObject *object) {
    finalized = TRUE;
    on_main = g_main_context_is_owner(g_main_context_default());
}
static gpointer worker(gpointer object) {
    g_object_unref(object);
    return NULL;
}
int main(int argc, char **argv) {
    gtk_init(&argc, &argv);
    GThread *caller = g_thread_new("invoke", invoke_worker, NULL);
    while (!g_atomic_int_get(&call_done)) {
        g_main_context_iteration(NULL, FALSE);
        g_usleep(1000);
    }
    g_thread_join(caller);
    if (!call_valid) { g_printerr("synchronous GTK dispatch failed\n"); return 4; }
    // A request without a task is sufficient to exercise GObject lifetime.
    GObject *request = g_object_new(WEBKIT_TYPE_URI_SCHEME_REQUEST, NULL);
    g_object_weak_ref(request, destroyed, NULL);
    g_thread_join(g_thread_new("release", worker, request));
    if (finalized) { g_printerr("request finalized on worker\n"); return 1; }
    while (!finalized) g_main_context_iteration(NULL, TRUE);
    if (!on_main) { g_printerr("request finalized outside GTK loop\n"); return 2; }

    // Unrelated objects must still be released immediately by their owner.
    finalized = FALSE;
    GObject *other = g_object_new(G_TYPE_OBJECT, NULL);
    g_object_weak_ref(other, destroyed, NULL);
    g_thread_join(g_thread_new("release", worker, other));
    if (!finalized || on_main) { g_printerr("unrelated release was deferred\n"); return 3; }
    return 0;
}
`
	if err := os.WriteFile(source, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "release")
	args := []string{source, "-I.", "-Wl,--wrap=g_object_unref", "-Wl,--wrap=webkit_uri_scheme_request_get_http_body", "-Wl,--wrap=webkit_uri_scheme_request_finish_with_response", "-Wl,--wrap=webkit_uri_scheme_request_finish_error", "-o", binary}
	args = append(args, strings.Fields(string(flags))...)
	if output, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("request release: %v\n%s", err, output)
	}
}
