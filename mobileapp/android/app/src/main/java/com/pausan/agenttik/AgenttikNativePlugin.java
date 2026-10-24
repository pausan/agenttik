package com.pausan.agenttik;

import android.net.Uri;
import com.getcapacitor.JSArray;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.NetworkInterface;
import java.util.Collections;

// The little the connect page cannot do from JavaScript: list this phone's
// IPv4 addresses for the scan, and keep the chosen agenttik inside the
// WebView instead of handing it to the system browser.
@CapacitorPlugin(name = "AgenttikNative")
public class AgenttikNativePlugin extends Plugin {

    private volatile String allowed;

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

    @Override
    public Boolean shouldOverrideLoad(Uri url) {
        String origin = allowed;
        if (origin == null || url.getScheme() == null || url.getHost() == null) return null;
        Uri o = Uri.parse(origin);
        boolean same = url.getScheme().equals(o.getScheme()) && url.getHost().equals(o.getHost()) && port(url) == port(o);
        return same ? Boolean.FALSE : null;
    }

    private static int port(Uri u) {
        if (u.getPort() != -1) return u.getPort();
        return "https".equals(u.getScheme()) ? 443 : 80;
    }
}
