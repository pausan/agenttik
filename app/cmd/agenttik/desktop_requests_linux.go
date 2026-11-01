//go:build desktop && linux

package main

/*
#cgo pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
#cgo LDFLAGS: -Wl,--wrap=g_object_unref -Wl,--wrap=webkit_uri_scheme_request_get_http_body -Wl,--wrap=webkit_uri_scheme_request_finish_with_response -Wl,--wrap=webkit_uri_scheme_request_finish_error
*/
import "C"
