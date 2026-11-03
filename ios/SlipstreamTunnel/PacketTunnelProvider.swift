import Foundation
import NetworkExtension
import Slipcore

/// Slipstream'in iOS tünelidir. Trafik cihazdan çıkmaz:
///
/// * Web trafiği tünelin proxy ayarıyla eklentideki yerel motora gelir.
///   Motor SNI bölme, parçalama ve DoH ile engeli aşar.
/// * Tünele yalnız sahte DNS adresi yönlendirilir. Cihazın bütün DNS
///   sorguları buraya gelir; reklam alan adları engellenir, gerisi DoH ile
///   çözülür. Böylece ISS'nin DNS zehirlemesi de aşılır.
final class PacketTunnelProvider: NEPacketTunnelProvider {
    static let tunnelAddress = "198.18.0.1"
    static let dnsAddress = "198.18.0.2"

    private var bridge: CoreBridge?
    private var running = false

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        var opts: [String: String] = [:]
        if let profile = options?["profile"] as? String { opts["profile"] = profile }
        let optJSON = (try? JSONSerialization.data(withJSONObject: opts))
            .flatMap { String(data: $0, encoding: .utf8) } ?? ""

        let bridge = CoreBridge(provider: self)
        self.bridge = bridge
        var err: NSError?
        let settings = SlipcoreStart(Shared.engineDirectory.path, optJSON, bridge, &err)
        if let err {
            completionHandler(err)
            return
        }
        setTunnelNetworkSettings(Self.networkSettings(from: settings)) { [weak self] error in
            guard let self else { return }
            if error == nil {
                self.running = true
                self.readPackets()
            } else {
                SlipcoreStop()
            }
            completionHandler(error)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        running = false
        SlipcoreStop()
        bridge = nil
        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        guard let req = try? JSONDecoder().decode(ProviderRequest.self, from: messageData) else {
            completionHandler?(nil)
            return
        }
        // Tarama gibi uzun istekler tünelin paket döngüsünü bekletmesin.
        DispatchQueue.global(qos: .userInitiated).async {
            let out = SlipcoreRequest(req.method, req.path, req.body ?? "")
            completionHandler?(out.data(using: .utf8))
        }
    }

    /// Motor proxy ayarını değiştirdiğinde (kip ya da PAC listesi) tünelin ağ
    /// ayarları yenilenir.
    func reapply(_ settings: String) {
        guard running else { return }
        reasserting = true
        setTunnelNetworkSettings(Self.networkSettings(from: settings)) { [weak self] _ in
            self?.reasserting = false
        }
    }

    private func readPackets() {
        packetFlow.readPackets { [weak self] packets, _ in
            guard let self, self.running else { return }
            for p in packets { SlipcoreHandlePacket(p) }
            self.readPackets()
        }
    }

    // MARK: - Ağ ayarları

    private struct ProxyConfig: Decodable {
        var enabled: Bool
        var mode: String
        var host: String
        var port: Int
        var pac: String?
    }

    static func networkSettings(from json: String) -> NEPacketTunnelNetworkSettings {
        let s = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        s.mtu = 1500

        // Tünele yalnız sahte DNS adresi girer; gerisi her zamanki yoldan gider.
        let v4 = NEIPv4Settings(addresses: [tunnelAddress], subnetMasks: ["255.255.255.255"])
        v4.includedRoutes = [NEIPv4Route(destinationAddress: dnsAddress, subnetMask: "255.255.255.255")]
        s.ipv4Settings = v4

        let dns = NEDNSSettings(servers: [dnsAddress])
        dns.matchDomains = [""] // bütün alan adları
        s.dnsSettings = dns

        if let cfg = try? JSONDecoder().decode(ProxyConfig.self, from: Data(json.utf8)), cfg.enabled {
            let proxy = NEProxySettings()
            if cfg.mode == "pac", let pac = cfg.pac, !pac.isEmpty {
                proxy.autoProxyConfigurationEnabled = true
                proxy.proxyAutoConfigurationJavaScript = pac
            } else {
                let server = NEProxyServer(address: cfg.host, port: cfg.port)
                proxy.httpEnabled = true
                proxy.httpServer = server
                proxy.httpsEnabled = true
                proxy.httpsServer = server
            }
            proxy.excludeSimpleHostnames = true
            proxy.exceptionList = ["*.local", "localhost", "127.0.0.1", "169.254/16",
                                   "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"]
            // Tünel varsayılan rota olmadığı için proxy'nin bütün alan
            // adlarına uygulanması açıkça istenmeli.
            proxy.matchDomains = [""]
            s.proxySettings = proxy
        }
        return s
    }
}

/// Go motorunun tünele yazdığı paketleri ve proxy değişikliklerini iletir.
final class CoreBridge: NSObject, SlipcoreDelegateProtocol {
    private weak var provider: PacketTunnelProvider?

    init(provider: PacketTunnelProvider) {
        self.provider = provider
    }

    func writePacket(_ pkt: Data?) {
        guard let pkt, let provider else { return }
        provider.packetFlow.writePackets([pkt], withProtocols: [NSNumber(value: AF_INET)])
    }

    func proxyChanged(_ settings: String?) {
        guard let settings, let provider else { return }
        DispatchQueue.main.async { provider.reapply(settings) }
    }
}
