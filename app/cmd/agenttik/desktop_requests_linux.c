//go:build desktop && linux

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

void __real_g_object_unref(gpointer object);

static gboolean release_scheme_request(gpointer object) {
    __real_g_object_unref(object);
    return G_SOURCE_REMOVE;
}

// Wails v2 closes URI requests on its asset-server goroutines. WebKit's
// request task shares non-thread-safe FormData with the GTK loop: releasing
// it there can race with cancellation and destroy the body twice. Transfer
// that owned reference to the main loop instead. g_idle_add always queues;
// g_main_context_invoke may run inline on the calling worker.
//
// Only direct calls linked into this executable are wrapped. Calls inside
// GTK/WebKit shared libraries and releases of other GObjects are unchanged.
void __wrap_g_object_unref(gpointer object) {
    if (object && (WEBKIT_IS_URI_SCHEME_REQUEST(object) ||
        g_str_equal(G_OBJECT_TYPE_NAME(object), "WebKitFormDataInputStream")) &&
        !g_main_context_is_owner(g_main_context_default())) {
        g_idle_add_full(G_PRIORITY_DEFAULT, release_scheme_request, object, NULL);
        return;
    }
    __real_g_object_unref(object);
}

// Request-body creation retains the same non-thread-safe FormData. Response
// completion also mutates request state read by GTK's async callbacks. Run
// both synchronously on GTK, keeping the caller's arguments alive until done.
typedef struct {
    GMutex mutex;
    GCond cond;
    gboolean done;
    void (*function)(gpointer);
    gpointer data;
} GtkCall;

static gboolean dispatch_gtk_call(gpointer data) {
    GtkCall *call = data;
    call->function(call->data);
    g_mutex_lock(&call->mutex);
    call->done = TRUE;
    g_cond_signal(&call->cond);
    g_mutex_unlock(&call->mutex);
    return G_SOURCE_REMOVE;
}

static void call_on_gtk(void (*function)(gpointer), gpointer data) {
    if (g_main_context_is_owner(g_main_context_default())) {
        function(data);
        return;
    }
    GtkCall call = { .function = function, .data = data };
    g_mutex_init(&call.mutex);
    g_cond_init(&call.cond);
    g_mutex_lock(&call.mutex);
    g_idle_add_full(G_PRIORITY_DEFAULT, dispatch_gtk_call, &call, NULL);
    while (!call.done) g_cond_wait(&call.cond, &call.mutex);
    g_mutex_unlock(&call.mutex);
    g_cond_clear(&call.cond);
    g_mutex_clear(&call.mutex);
}

#if WEBKIT_CHECK_VERSION(2, 40, 0)
GInputStream *__real_webkit_uri_scheme_request_get_http_body(WebKitURISchemeRequest *request);
typedef struct { WebKitURISchemeRequest *request; GInputStream *body; } BodyCall;
static void get_body(gpointer data) {
    BodyCall *call = data;
    call->body = __real_webkit_uri_scheme_request_get_http_body(call->request);
}
GInputStream *__wrap_webkit_uri_scheme_request_get_http_body(WebKitURISchemeRequest *request) {
    BodyCall call = { .request = request };
    call_on_gtk(get_body, &call);
    return call.body;
}
#endif

void __real_webkit_uri_scheme_request_finish_with_response(WebKitURISchemeRequest *request, WebKitURISchemeResponse *response);
void __real_webkit_uri_scheme_request_finish_error(WebKitURISchemeRequest *request, GError *error);
typedef struct { WebKitURISchemeRequest *request; WebKitURISchemeResponse *response; GError *error; } FinishCall;
static void finish_response(gpointer data) {
    FinishCall *call = data;
    __real_webkit_uri_scheme_request_finish_with_response(call->request, call->response);
}
static void finish_error(gpointer data) {
    FinishCall *call = data;
    __real_webkit_uri_scheme_request_finish_error(call->request, call->error);
}
void __wrap_webkit_uri_scheme_request_finish_with_response(WebKitURISchemeRequest *request, WebKitURISchemeResponse *response) {
    FinishCall call = { .request = request, .response = response };
    call_on_gtk(finish_response, &call);
}
void __wrap_webkit_uri_scheme_request_finish_error(WebKitURISchemeRequest *request, GError *error) {
    FinishCall call = { .request = request, .error = error };
    call_on_gtk(finish_error, &call);
}
