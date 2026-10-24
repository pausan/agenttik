import Capacitor
import Foundation
import WebKit

// The little the connect page cannot do from JavaScript: list this phone's
// IPv4 addresses for the scan, and keep the chosen agenttik inside the
// WebView instead of handing it to the system browser.
@objc(AgenttikNativePlugin)
public class AgenttikNativePlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "AgenttikNativePlugin"
    public let jsName = "AgenttikNative"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "addresses", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "allow", returnType: CAPPluginReturnPromise)
    ]

    private var allowed: URL?

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

    private func port(_ url: URL) -> Int {
        url.port ?? (url.scheme == "https" ? 443 : 80)
    }
}

class BridgeViewController: CAPBridgeViewController {
    override open func capacitorDidLoad() {
        bridge?.registerPluginInstance(AgenttikNativePlugin())
    }
}
