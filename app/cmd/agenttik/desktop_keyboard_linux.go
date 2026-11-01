//go:build desktop && linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

#define GRAB_RETRY_MS 50
#define GRAB_TRIES 60

static guint retry;
static int tries;

static GdkWindow *appWindow(void) {
	GdkWindow *found = NULL;
	GList *windows = gtk_window_list_toplevels();
	for (GList *w = windows; w != NULL && found == NULL; w = w->next) {
		GtkWindow *window = GTK_WINDOW(w->data);
		if (gtk_window_get_window_type(window) == GTK_WINDOW_TOPLEVEL && gtk_widget_get_visible(GTK_WIDGET(window))) {
			found = gtk_widget_get_window(GTK_WIDGET(window));
		}
	}
	g_list_free(windows);
	return found;
}

// The window manager may hold the keyboard for a moment while the window
// changes size, and the grab fails until it lets go.
static gboolean tryGrab(gpointer unused) {
	GdkWindow *window = appWindow();
	GdkSeat *seat = gdk_display_get_default_seat(gdk_display_get_default());
	if (window != NULL && gdk_seat_grab(seat, window, GDK_SEAT_CAPABILITY_KEYBOARD, TRUE, NULL, NULL, NULL, NULL) == GDK_GRAB_SUCCESS) {
		retry = 0;
		return G_SOURCE_REMOVE;
	}
	if (++tries < GRAB_TRIES) {
		return G_SOURCE_CONTINUE;
	}
	g_warning("could not take the keyboard");
	retry = 0;
	return G_SOURCE_REMOVE;
}

static gboolean startGrab(gpointer unused) {
	if (retry == 0) {
		tries = 0;
		retry = g_timeout_add(GRAB_RETRY_MS, tryGrab, NULL);
	}
	return G_SOURCE_REMOVE;
}

static gboolean stopGrab(gpointer unused) {
	if (retry != 0) {
		g_source_remove(retry);
		retry = 0;
	}
	gdk_seat_ungrab(gdk_display_get_default_seat(gdk_display_get_default()));
	return G_SOURCE_REMOVE;
}

static void grabKeyboard(void) { g_idle_add(startGrab, NULL); }
static void releaseKeyboard(void) { g_idle_add(stopGrab, NULL); }
*/
import "C"

// grabKeyboard sends every key to the window, ahead of the window manager's
// shortcuts: an X grab, or on Wayland a request to inhibit shortcuts, which
// the desktop may ask the person to allow. Both calls hand over to GTK's loop
// and may be made from any goroutine.
func grabKeyboard()    { C.grabKeyboard() }
func releaseKeyboard() { C.releaseKeyboard() }
