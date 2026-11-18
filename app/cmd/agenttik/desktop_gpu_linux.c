//go:build desktop && linux

#include <string.h>

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// The GPU check runs in a child process (see desktop_gpu_linux.go), so a
// driver that crashes or hangs takes only the child down. It draws a solid
// colour in an offscreen view with acceleration forced on, reads the pixels
// the widget really shows, and then asks WebKit which GL renderer it got.

enum { CHECK_WAITING, CHECK_DRAWN, CHECK_SOFTWARE, CHECK_GPU };

typedef struct {
    GtkWidget *window;
    WebKitWebView *view;
    int state;
    gboolean polling;
} gpu_check;

static gboolean green_at_centre(GtkWidget *window) {
    GdkPixbuf *shot = gtk_offscreen_window_get_pixbuf(GTK_OFFSCREEN_WINDOW(window));
    if (!shot) return FALSE;
    gboolean green = FALSE;
    int w = gdk_pixbuf_get_width(shot), h = gdk_pixbuf_get_height(shot);
    if (w > 0 && h > 0 && gdk_pixbuf_get_n_channels(shot) >= 3) {
        const guchar *p = gdk_pixbuf_get_pixels(shot) + (h / 2) * gdk_pixbuf_get_rowstride(shot)
            + (w / 2) * gdk_pixbuf_get_n_channels(shot);
        green = p[0] < 32 && p[1] > 223 && p[2] < 32;
    }
    g_object_unref(shot);
    return green;
}

// The renderer line of WebKit's own GPU page, in its render process section.
static const char RENDERER_SCRIPT[] =
    "(() => { const t = document.body ? document.body.innerText : '';"
    " const s = t.slice(Math.max(0, t.indexOf('(Render Process)')));"
    " const m = s.match(/GL_RENDERER\\s*\\n([^\\n]*)/); return m ? m[1] : ''; })()";

static gboolean software_renderer(const char *renderer) {
    static const char *const names[] = {"llvmpipe", "softpipe", "swrast", "swiftshader", "software", NULL};
    gchar *lower = g_ascii_strdown(renderer, -1);
    gboolean found = FALSE;
    for (int i = 0; names[i] && !found; i++) found = strstr(lower, names[i]) != NULL;
    g_free(lower);
    return found;
}

static void renderer_read(GObject *view, GAsyncResult *result, gpointer data) {
    gpu_check *check = data;
    JSCValue *value = webkit_web_view_evaluate_javascript_finish(WEBKIT_WEB_VIEW(view), result, NULL);
    gchar *renderer = value && jsc_value_is_string(value) ? jsc_value_to_string(value) : g_strdup("");
    check->state = software_renderer(renderer) ? CHECK_SOFTWARE : CHECK_GPU;
    g_print("%s\n", renderer);
    g_free(renderer);
    if (value) g_object_unref(value);
    gtk_main_quit();
}

static gboolean poll_pixels(gpointer data) {
    gpu_check *check = data;
    if (!green_at_centre(check->window)) return G_SOURCE_CONTINUE;
    check->state = CHECK_DRAWN;
    check->polling = FALSE;
    webkit_web_view_load_uri(check->view, "webkit://gpu");
    return G_SOURCE_REMOVE;
}

static void load_changed(WebKitWebView *view, WebKitLoadEvent event, gpointer data) {
    gpu_check *check = data;
    if (event != WEBKIT_LOAD_FINISHED) return;
    if (check->state == CHECK_WAITING && !check->polling) {
        check->polling = TRUE;
        g_timeout_add(20, poll_pixels, check);
    } else if (check->state == CHECK_DRAWN) {
        webkit_web_view_evaluate_javascript(view, RENDERER_SCRIPT, -1, NULL, NULL, NULL, renderer_read, check);
    }
}

// The GPU page is missing from older WebKit; drawing is then enough.
static void load_failed(WebKitWebView *view, WebKitLoadEvent event, gchar *uri, GError *error, gpointer data) {
    gpu_check *check = data;
    if (check->state != CHECK_DRAWN) return;
    check->state = CHECK_GPU;
    gtk_main_quit();
}

// Returns 0 when accelerated drawing reaches the screen on a hardware
// renderer, 1 when nothing was drawn, 2 for a software GL renderer and 3
// without a display. The parent process enforces the time limit.
int agenttik_gpu_check(void) {
    if (!gtk_init_check(NULL, NULL)) return 3;
    gpu_check check = {0};
    check.window = gtk_offscreen_window_new();
    gtk_window_set_default_size(GTK_WINDOW(check.window), 32, 32);
    check.view = WEBKIT_WEB_VIEW(webkit_web_view_new());
    webkit_settings_set_hardware_acceleration_policy(webkit_web_view_get_settings(check.view),
                                                     WEBKIT_HARDWARE_ACCELERATION_POLICY_ALWAYS);
    gtk_container_add(GTK_CONTAINER(check.window), GTK_WIDGET(check.view));
    g_signal_connect(check.view, "load-changed", G_CALLBACK(load_changed), &check);
    g_signal_connect(check.view, "load-failed", G_CALLBACK(load_failed), &check);
    gtk_widget_show_all(check.window);
    webkit_web_view_load_html(check.view, "<html style=\"background:#00ff00\"></html>", NULL);
    gtk_main();
    return check.state == CHECK_GPU ? 0 : check.state == CHECK_SOFTWARE ? 2 : 1;
}
