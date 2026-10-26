package com.pausan.agenttik;

import android.os.Bundle;
import android.webkit.WebView;
import androidx.activity.OnBackPressedCallback;
import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {

    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(AgenttikNativePlugin.class);
        super.onCreate(savedInstanceState);
        // Back walks the remote's history, then returns to the connect page,
        // then leaves. The connect page's own entry is often not in the
        // history: WebView skips entries a page left without a user gesture,
        // as the automatic reconnect does. On the connect page Back leaves
        // even with the remote behind it, so it never undoes a log out.
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                WebView webView = bridge.getWebView();
                String url = webView.getUrl();
                if (url == null || url.startsWith(bridge.getLocalUrl())) {
                    finish();
                } else if (webView.canGoBack()) {
                    webView.goBack();
                } else {
                    webView.loadUrl(bridge.getLocalUrl());
                }
            }
        });
    }
}
