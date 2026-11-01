package com.pausan.agenttik;

import android.content.Context;
import android.content.SharedPreferences;
import android.net.Uri;
import android.util.Log;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import com.getcapacitor.BridgeWebViewClient;
import com.getcapacitor.JSArray;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.NetworkInterface;
import java.util.Collections;

// The little the connect page cannot do from JavaScript: list this phone's
// IPv4 addresses for the scan, and keep the chosen agenttik inside the
// WebView instead of handing it to the system browser. It also gives that
// agenttik a microphone (see Microphone), a way back to the connect page, and
// a place to leave the addresses it reports for that page to pick up.
@CapacitorPlugin(name = "AgenttikNative")
public class AgenttikNativePlugin extends Plugin {

    private static final String ADDRESSES = "addresses:";
    private static final int MAX_MESSAGE = 4096;

    private volatile String allowed;

    @Override
    public void load() {
        bridge.setWebViewClient(new BridgeWebViewClient(bridge) {
            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                super.onReceivedError(view, request, error);
                // A failed document replaces the remote UI with WebView's
                // error page, which has no reconnect code. Subresource errors
                // leave the UI alive and must not take it away.
                if (!request.isForMainFrame() || !same(request.getUrl())) return;
                lost(view, error.getErrorCode() + " " + error.getDescription());
            }
        });
        new Microphone(getActivity(), bridge.getWebView(), this::same);
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)) return;
        // Log out in the remote's Settings posts "disconnect" once it has
        // signed out. The connect page reads #signed-out as "do not reconnect".
        // The remote also posts "lost" when its server stops answering, and
        // "addresses:<JSON list>" with where that server can be reached.
        WebViewCompat.addWebMessageListener(bridge.getWebView(), "agenttikApp", Collections.singleton("*"),
            (view, message, origin, isMainFrame, reply) -> {
                String data = message.getData();
                if (!isMainFrame || !same(origin) || data == null) return;
                if (data.equals("disconnect")) {
                    allowed = null;
                    view.loadUrl(bridge.getLocalUrl() + "/#signed-out");
                } else if (data.equals("lost")) {
                    lost(view, "not answering");
                } else if (data.startsWith(ADDRESSES) && data.length() <= MAX_MESSAGE) {
                    learnedStore().edit().putString("origin", allowed)
                        .putString("addresses", data.substring(ADDRESSES.length())).apply();
                }
            });
    }

    private void lost(WebView view, String reason) {
        String origin = allowed;
        allowed = null;
        Log.w("Agenttik", "Connection lost to " + origin + ": " + reason);
        view.post(() -> view.loadUrl(bridge.getLocalUrl() + "/#connection-lost=" + Uri.encode(origin)));
    }

    private SharedPreferences learnedStore() {
        return getContext().getSharedPreferences("agenttik-learned", Context.MODE_PRIVATE);
    }

    @PluginMethod
    public void addresses(PluginCall call) {
        JSArray list = new JSArray();
        try {
            for (NetworkInterface ni : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                if (!ni.isUp() || ni.isLoopback()) continue;
                for (InetAddress a : Collections.list(ni.getInetAddresses())) {
                    if (a instanceof Inet4Address) list.put(a.getHostAddress());
                }
            }
        } catch (Exception e) {
            call.reject("Could not list network addresses", e);
            return;
        }
        JSObject ret = new JSObject();
        ret.put("addresses", list);
        call.resolve(ret);
    }

    @PluginMethod
    public void allow(PluginCall call) {
        String origin = call.getString("origin");
        if (origin == null) {
            call.reject("Must provide origin");
            return;
        }
        allowed = origin;
        call.resolve();
    }

    // learned hands the connect page the latest addresses a remote reported,
    // once.
    @PluginMethod
    public void learned(PluginCall call) {
        SharedPreferences store = learnedStore();
        JSObject ret = new JSObject();
        ret.put("origin", store.getString("origin", null));
        ret.put("addresses", store.getString("addresses", null));
        store.edit().clear().apply();
        call.resolve(ret);
    }

    @Override
    public Boolean shouldOverrideLoad(Uri url) {
        return same(url) ? Boolean.FALSE : null;
    }

    private boolean same(Uri url) {
        String origin = allowed;
        if (origin == null || url.getScheme() == null || url.getHost() == null) return false;
        Uri o = Uri.parse(origin);
        return url.getScheme().equals(o.getScheme()) && url.getHost().equals(o.getHost()) && port(url) == port(o);
    }

    private static int port(Uri u) {
        if (u.getPort() != -1) return u.getPort();
        return "https".equals(u.getScheme()) ? 443 : 80;
    }
}
