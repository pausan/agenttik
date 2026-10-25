package com.pausan.agenttik;

import android.Manifest;
import android.annotation.SuppressLint;
import android.content.pm.PackageManager;
import android.media.AudioFormat;
import android.media.AudioRecord;
import android.media.MediaRecorder;
import android.net.Uri;
import android.util.Base64;
import androidx.activity.ComponentActivity;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.core.content.ContextCompat;
import androidx.webkit.JavaScriptReplyProxy;
import androidx.webkit.WebMessageCompat;
import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;
import android.webkit.WebView;
import java.io.ByteArrayOutputStream;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.function.Predicate;
import org.json.JSONObject;

// WebView hides getUserMedia from plain-HTTP pages, which is how LAN servers
// are reached. The remote page records through this instead: commands
// "permission", "start", "stop" and "cancel" arrive as {id, command}; "stop"
// replies with two minutes at most of 16 kHz mono PCM16, base64, which is
// what the web recorder sends to the server. Only the allowed origin's main
// frame is answered. Everything runs on the UI thread except the reader.
class Microphone {

    private static final int RATE = 16000;
    private static final int LIMIT = RATE * 2 * 120;

    private final ActivityResultLauncher<String> launcher;
    private final List<Runnable> waiting = new ArrayList<>();
    private final ComponentActivity activity;
    private AudioRecord record;
    private Thread reader;
    private ByteArrayOutputStream pcm;

    Microphone(ComponentActivity activity, WebView webView, Predicate<Uri> allowed) {
        this.activity = activity;
        launcher = activity.registerForActivityResult(new ActivityResultContracts.RequestPermission(), granted -> {
            List<Runnable> done = new ArrayList<>(waiting);
            waiting.clear();
            for (Runnable r : done) r.run();
        });
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)) return;
        WebViewCompat.addWebMessageListener(webView, "agenttikMicrophone", Collections.singleton("*"),
            (view, message, origin, isMainFrame, reply) -> {
                if (isMainFrame && allowed.test(origin)) handle(message, reply);
            });
    }

    private void handle(WebMessageCompat message, JavaScriptReplyProxy reply) {
        int id;
        String command;
        try {
            JSONObject json = new JSONObject(message.getData());
            id = json.getInt("id");
            command = json.getString("command");
        } catch (Exception e) {
            return;
        }
        switch (command) {
            case "permission":
                if (granted()) {
                    send(reply, id, null, null);
                    return;
                }
                waiting.add(() -> send(reply, id, null, granted() ? null : "Allow microphone access in system settings to dictate."));
                if (waiting.size() == 1) launcher.launch(Manifest.permission.RECORD_AUDIO);
                return;
            case "start":
                send(reply, id, null, start());
                return;
            case "stop":
                byte[] data = stop();
                if (data == null) send(reply, id, null, "The microphone did not start.");
                else send(reply, id, Base64.encodeToString(data, Base64.NO_WRAP), null);
                return;
            case "cancel":
                stop();
                send(reply, id, null, null);
                return;
            default:
                send(reply, id, null, "Unknown microphone command.");
        }
    }

    private boolean granted() {
        return ContextCompat.checkSelfPermission(activity, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED;
    }

    @SuppressLint("MissingPermission")
    private String start() {
        stop();
        if (!granted()) return "Allow microphone access in system settings to dictate.";
        int min = AudioRecord.getMinBufferSize(RATE, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT);
        AudioRecord r = new AudioRecord(MediaRecorder.AudioSource.VOICE_RECOGNITION, RATE,
            AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT, Math.max(min, RATE));
        if (r.getState() != AudioRecord.STATE_INITIALIZED) {
            r.release();
            return "The microphone could not start.";
        }
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        r.startRecording();
        Thread t = new Thread(() -> {
            byte[] buf = new byte[4096];
            while (true) {
                int n = r.read(buf, 0, buf.length);
                if (n <= 0) break;
                synchronized (out) {
                    out.write(buf, 0, Math.min(n, LIMIT - out.size()));
                }
            }
        }, "agenttik-microphone");
        t.start();
        record = r;
        reader = t;
        pcm = out;
        return null;
    }

    private byte[] stop() {
        if (record == null) return null;
        record.stop();
        try {
            reader.join(1000);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        record.release();
        byte[] data;
        synchronized (pcm) {
            data = pcm.toByteArray();
        }
        record = null;
        reader = null;
        pcm = null;
        return data;
    }

    private static void send(JavaScriptReplyProxy reply, int id, String data, String error) {
        JSONObject json = new JSONObject();
        try {
            json.put("id", id);
            if (data != null) json.put("data", data);
            if (error != null) json.put("error", error);
        } catch (Exception e) {
            return;
        }
        reply.postMessage(json.toString());
    }
}
