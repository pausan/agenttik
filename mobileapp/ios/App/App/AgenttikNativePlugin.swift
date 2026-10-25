import AVFoundation
import Capacitor
import Foundation
import WebKit

// The little the connect page cannot do from JavaScript: list this phone's
// IPv4 addresses for the scan, and keep the chosen agenttik inside the
// WebView instead of handing it to the system browser. It also gives that
// agenttik a microphone (see Microphone) and a way back to the connect page
// (see AppChannel).
@objc(AgenttikNativePlugin)
public class AgenttikNativePlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "AgenttikNativePlugin"
    public let jsName = "AgenttikNative"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "addresses", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "allow", returnType: CAPPluginReturnPromise)
    ]

    private var allowed: URL?

    override public func load() {
        webView?.configuration.userContentController.addScriptMessageHandler(
            Microphone(plugin: self), contentWorld: .page, name: "agenttikMicrophone")
        webView?.configuration.userContentController.add(AppChannel(plugin: self), contentWorld: .page, name: "agenttikApp")
    }

    @objc func addresses(_ call: CAPPluginCall) {
        var list: [String] = []
        var head: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&head) == 0 else {
            call.reject("Could not list network addresses")
            return
        }
        defer { freeifaddrs(head) }
        var cursor = head
        while let ifa = cursor {
            let flags = Int32(ifa.pointee.ifa_flags)
            if let addr = ifa.pointee.ifa_addr, addr.pointee.sa_family == UInt8(AF_INET),
               flags & IFF_UP != 0, flags & IFF_LOOPBACK == 0 {
                var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
                if getnameinfo(addr, socklen_t(addr.pointee.sa_len), &host, socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 {
                    list.append(String(cString: host))
                }
            }
            cursor = ifa.pointee.ifa_next
        }
        call.resolve(["addresses": list])
    }

    @objc func allow(_ call: CAPPluginCall) {
        guard let origin = call.getString("origin"), let url = URL(string: origin) else {
            call.reject("Must provide origin")
            return
        }
        allowed = url
        call.resolve()
    }

    override public func shouldOverrideLoad(_ navigationAction: WKNavigationAction) -> NSNumber? {
        guard let allowed = allowed, let url = navigationAction.request.url else { return nil }
        let same = url.scheme == allowed.scheme && url.host == allowed.host && port(url) == port(allowed)
        return same ? NSNumber(value: false) : nil
    }

    func isAllowed(_ origin: WKSecurityOrigin) -> Bool {
        guard let allowed = allowed else { return false }
        let port = origin.port == 0 ? (origin.protocol == "https" ? 443 : 80) : origin.port
        return origin.protocol == allowed.scheme && origin.host == allowed.host && port == self.port(allowed)
    }

    // disconnect forgets the remote and loads the connect page with
    // #signed-out, which it reads as "do not reconnect".
    func disconnect() {
        allowed = nil
        guard let start = bridge?.config.appStartServerURL,
              var parts = URLComponents(url: start, resolvingAgainstBaseURL: false) else { return }
        parts.fragment = "signed-out"
        if let url = parts.url { webView?.load(URLRequest(url: url)) }
    }

    private func port(_ url: URL) -> Int {
        url.port ?? (url.scheme == "https" ? 443 : 80)
    }
}

class BridgeViewController: CAPBridgeViewController {
    override open func capacitorDidLoad() {
        bridge?.registerPluginInstance(AgenttikNativePlugin())
    }
}

// Log out in the remote's Settings posts "disconnect" once it has signed out.
// Only the allowed origin's main frame is heard.
class AppChannel: NSObject, WKScriptMessageHandler {
    private weak var plugin: AgenttikNativePlugin?

    init(plugin: AgenttikNativePlugin) {
        self.plugin = plugin
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        guard message.frameInfo.isMainFrame, message.body as? String == "disconnect",
              let plugin = plugin, plugin.isAllowed(message.frameInfo.securityOrigin) else { return }
        plugin.disconnect()
    }
}

// WKWebView hides getUserMedia from plain-HTTP pages, which is how LAN servers
// are reached. The remote page records through this instead: it posts
// {command} with "permission", "start", "level", "stop" or "cancel"; "level"
// replies with the RMS (0..1) of the latest buffer, for the meter; "stop"
// replies with two minutes at most of 16 kHz mono PCM16, base64, which is
// what the web recorder sends to the server. Only the allowed origin's main frame is
// answered.
class Microphone: NSObject, WKScriptMessageHandlerWithReply {
    private static let limit = 16000 * 2 * 120
    private static let denied = "Allow microphone access in Settings to dictate."

    private weak var plugin: AgenttikNativePlugin?
    private var engine: AVAudioEngine?
    private let lock = NSLock()
    private var pcm = Data()
    private var level: Float = 0

    init(plugin: AgenttikNativePlugin) {
        self.plugin = plugin
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage,
                               replyHandler: @escaping (Any?, String?) -> Void) {
        guard message.frameInfo.isMainFrame, plugin?.isAllowed(message.frameInfo.securityOrigin) == true,
              let body = message.body as? [String: Any], let command = body["command"] as? String else {
            replyHandler(nil, "Microphone access is unavailable.")
            return
        }
        let session = AVAudioSession.sharedInstance()
        switch command {
        case "permission":
            session.requestRecordPermission { granted in
                DispatchQueue.main.async { replyHandler(nil, granted ? nil : Microphone.denied) }
            }
        case "start":
            replyHandler(nil, start())
        case "level":
            lock.lock()
            let value = engine == nil ? 0 : level
            lock.unlock()
            replyHandler(Double(value), nil)
        case "stop":
            if let data = stop() {
                replyHandler(data.base64EncodedString(), nil)
            } else {
                replyHandler(nil, "The microphone did not start.")
            }
        case "cancel":
            _ = stop()
            replyHandler(nil, nil)
        default:
            replyHandler(nil, "Unknown microphone command.")
        }
    }

    private func start() -> String? {
        _ = stop()
        guard AVAudioSession.sharedInstance().recordPermission == .granted else { return Microphone.denied }
        let engine = AVAudioEngine()
        do {
            try AVAudioSession.sharedInstance().setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker])
            try AVAudioSession.sharedInstance().setActive(true)
            let input = engine.inputNode
            let source = input.outputFormat(forBus: 0)
            guard let target = AVAudioFormat(commonFormat: .pcmFormatInt16, sampleRate: 16000, channels: 1, interleaved: true),
                  let converter = AVAudioConverter(from: source, to: target) else {
                return "The microphone could not start."
            }
            lock.lock(); pcm = Data(); level = 0; lock.unlock()
            input.installTap(onBus: 0, bufferSize: 4096, format: source) { [weak self] buffer, _ in
                let capacity = AVAudioFrameCount(Double(buffer.frameLength) * 16000 / source.sampleRate) + 1
                guard let self = self, let out = AVAudioPCMBuffer(pcmFormat: target, frameCapacity: capacity) else { return }
                var fed = false
                converter.convert(to: out, error: nil) { _, status in
                    if fed {
                        status.pointee = .noDataNow
                        return nil
                    }
                    fed = true
                    status.pointee = .haveData
                    return buffer
                }
                guard let samples = out.int16ChannelData else { return }
                var sum: Float = 0
                for i in 0..<Int(out.frameLength) {
                    let v = Float(samples[0][i]) / 32768
                    sum += v * v
                }
                let rms = out.frameLength > 0 ? (sum / Float(out.frameLength)).squareRoot() : 0
                self.lock.lock()
                self.level = rms
                let count = min(Int(out.frameLength) * 2, Microphone.limit - self.pcm.count)
                if count > 0 { self.pcm.append(Data(bytes: samples[0], count: count)) }
                self.lock.unlock()
            }
            try engine.start()
        } catch {
            engine.inputNode.removeTap(onBus: 0)
            return "The microphone could not start."
        }
        self.engine = engine
        return nil
    }

    private func stop() -> Data? {
        guard let engine = engine else { return nil }
        engine.inputNode.removeTap(onBus: 0)
        engine.stop()
        self.engine = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        lock.lock()
        defer { lock.unlock() }
        return pcm
    }
}
