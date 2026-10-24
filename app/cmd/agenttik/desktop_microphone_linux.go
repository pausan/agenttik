//go:build desktop && linux

package main

/*
#cgo pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// WebKitGTK otherwise denies getUserMedia without a permission handler.
// Only audio requests from the app window are accepted, never cameras.
static gboolean microphone_permission(WebKitWebView *view, WebKitPermissionRequest *request, gpointer data) {
  const gchar *uri = webkit_web_view_get_uri(view);
  if (!uri || !g_str_has_prefix(uri, "wails://")) return FALSE;
  if (!WEBKIT_IS_USER_MEDIA_PERMISSION_REQUEST(request)) return FALSE;
  WebKitUserMediaPermissionRequest *media = WEBKIT_USER_MEDIA_PERMISSION_REQUEST(request);
  if (!webkit_user_media_permission_is_for_audio_device(media) ||
       webkit_user_media_permission_is_for_video_device(media)) return FALSE;
  webkit_permission_request_allow(request);
  return TRUE;
}
static void configure_microphone_widget(GtkWidget *widget, gpointer data) {
  if (WEBKIT_IS_WEB_VIEW(widget)) {
    WebKitWebView *view = WEBKIT_WEB_VIEW(widget);
    WebKitWebContext *context = webkit_web_view_get_context(view);
    webkit_security_manager_register_uri_scheme_as_secure(webkit_web_context_get_security_manager(context), "wails");
    g_signal_connect(view, "permission-request", G_CALLBACK(microphone_permission), NULL);
  } else if (GTK_IS_CONTAINER(widget)) {
    gtk_container_foreach(GTK_CONTAINER(widget), configure_microphone_widget, NULL);
  }
}
static gboolean configure_microphones(gpointer data) {
  GList *windows = gtk_window_list_toplevels();
  for (GList *node = windows; node; node = node->next) configure_microphone_widget(GTK_WIDGET(node->data), NULL);
  g_list_free(windows);
  return G_SOURCE_REMOVE;
}
static void start_microphones(void) { g_idle_add(configure_microphones, NULL); }
*/
import "C"

func startNative(*window) { C.start_microphones() }
