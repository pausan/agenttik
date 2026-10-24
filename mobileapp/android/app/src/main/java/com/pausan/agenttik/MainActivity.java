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
        // as the automatic reconnect does.
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                WebView webView = bridge.getWebView();
                String url = webView.getUrl();
                if (webView.canGoBack()) {
                    webView.goBack();
                } else if (url != null && !url.startsWith(bridge.getLocalUrl())) {
                    webView.loadUrl(bridge.getLocalUrl());
                } else {
                    finish();
                }
            }
        });
    }
}
