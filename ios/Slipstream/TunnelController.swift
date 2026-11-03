import Foundation
import NetworkExtension
import SwiftUI

/// Slipstream tünelini (VPN yapılandırmasını) kurar, açar, kapatır ve
/// tüneldeki motoru yönetir. macOS'taki EngineController'ın iOS karşılığıdır;
/// istekler HTTP yerine NETunnelProviderSession mesajlarıyla gider.
@MainActor
final class TunnelController: ObservableObject {
    @Published private(set) var vpnStatus: NEVPNStatus = .invalid
    @Published private(set) var status: EngineStatus?
    @Published private(set) var profiles: [Profile] = Profile.builtin
    @Published private(set) var logs: [LogEntry] = []
    @Published private(set) var probe: ProbeStatus?
    @Published private(set) var rateHistory: [RateSample] = []
    @Published private(set) var loaded = false
    @Published var lastError: String?
    @Published private(set) var pendingProxyMode: String?

    @AppStorage("profile", store: UserDefaults(suiteName: Shared.appGroup))
    var selectedProfile = "dengeli"
    @AppStorage("alwaysOn", store: UserDefaults(suiteName: Shared.appGroup))
    var alwaysOn = false

    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?
    private var pollTask: Task<Void, Never>?
    private var lastLogSeq: UInt64 = 0
    private var lastTotals: (up: Int64, down: Int64, at: Date)?
    private var foreground = true
    static let rateWindow = 40

    var isConnected: Bool { vpnStatus == .connected }
    var isBusy: Bool { [.connecting, .disconnecting, .reasserting].contains(vpnStatus) }

    init() {
        Task { await load() }
    }

    // MARK: - VPN yapılandırması

    func load() async {
        do {
            let all = try await NETunnelProviderManager.loadAllFromPreferences()
            manager = all.first(where: {
                ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier == Shared.tunnelBundleID
            }) ?? Self.makeManager()
        } catch {
            lastError = "VPN ayarları okunamadı: \(error.localizedDescription)"
            manager = Self.makeManager()
        }
        observeStatus()
        loaded = true
    }

    private static func makeManager() -> NETunnelProviderManager {
        let m = NETunnelProviderManager()
        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = Shared.tunnelBundleID
        proto.serverAddress = "Bu cihaz"
        proto.disconnectOnSleep = false
        m.protocolConfiguration = proto
        m.localizedDescription = "Slipstream"
        m.isEnabled = true
        return m
    }

    private func observeStatus() {
        if let statusObserver { NotificationCenter.default.removeObserver(statusObserver) }
        guard let conn = manager?.connection else { return }
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange, object: conn, queue: .main
        ) { [weak self] _ in
            Task { @MainActor in self?.statusChanged() }
        }
        statusChanged()
    }

    private func statusChanged() {
        let new = manager?.connection.status ?? .invalid
        guard new != vpnStatus || status == nil else { return }
        vpnStatus = new
        if new == .connected {
            startPolling()
            Task { await loadProfiles() }
        } else if new == .disconnected || new == .invalid {
            stopPolling()
            status = nil
        }
    }

    func toggle() {
        if isConnected || vpnStatus == .connecting { disconnect() } else { connect() }
    }

    func connect() {
        guard let m = manager else { return }
        lastError = nil
        Task {
            do {
                m.isEnabled = true
                applyOnDemand(to: m, enabled: alwaysOn)
                // İlk seferde iOS "VPN yapılandırması ekle" iznini burada sorar.
                try await m.saveToPreferences()
                try await m.loadFromPreferences()
                observeStatus()
                try m.connection.startVPNTunnel(options: ["profile": selectedProfile as NSString])
            } catch {
                lastError = Self.describe(error)
            }
        }
    }

    func disconnect() {
        guard let m = manager else { return }
        Task {
            // "Her zaman açık" açıksa iOS tüneli hemen yeniden açar; elle
            // kapatınca bu kural da kapatılır.
            if m.isOnDemandEnabled {
                m.isOnDemandEnabled = false
                try? await m.saveToPreferences()
            }
            m.connection.stopVPNTunnel()
        }
    }

    func setAlwaysOn(_ on: Bool) {
        alwaysOn = on
        objectWillChange.send()
        guard let m = manager, isConnected else { return }
        Task {
            applyOnDemand(to: m, enabled: on)
            do { try await m.saveToPreferences() } catch { lastError = Self.describe(error) }
        }
    }

    private func applyOnDemand(to m: NETunnelProviderManager, enabled: Bool) {
        let rule = NEOnDemandRuleConnect()
        rule.interfaceTypeMatch = .any
        m.onDemandRules = [rule]
        m.isOnDemandEnabled = enabled
    }

    private static func describe(_ error: Error) -> String {
        let ns = error as NSError
        if ns.domain == NEVPNErrorDomain, ns.code == NEVPNError.Code.configurationReadWriteFailed.rawValue {
            return "VPN yapılandırmasına izin verilmedi. Tekrar dokunup \"İzin Ver\"i seç."
        }
        return error.localizedDescription
    }

    // MARK: - Ön plan ve yoklama

    func setForeground(_ on: Bool) {
        foreground = on
        if on, isConnected { startPolling() } else { stopPolling() }
    }

    private func startPolling() {
        guard foreground else { return }
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                try? await Task.sleep(nanoseconds: 1_500_000_000)
            }
        }
    }

    private func stopPolling() {
        pollTask?.cancel()
        pollTask = nil
        rateHistory = []
        lastTotals = nil
    }

    func refresh() async {
        guard isConnected else { return }
        if let s: EngineStatus = try? await call("GET", "/api/status") {
            recordRate(s.stats)
            status = s
            if selectedProfile != s.profile { selectedProfile = s.profile }
        }
        if let l: LogsResponse = try? await call("GET", "/api/logs?since=\(lastLogSeq)"),
           let entries = l.entries, !entries.isEmpty {
            logs.append(contentsOf: entries)
            if logs.count > 300 { logs.removeFirst(logs.count - 300) }
            lastLogSeq = entries.last?.seq ?? lastLogSeq
        }
        if probe?.running == true {
            probe = try? await call("GET", "/api/probe")
        }
    }

    private func recordRate(_ st: Stats) {
        let now = Date()
        defer { lastTotals = (st.bytesUp, st.bytesDown, now) }
        guard let last = lastTotals else { return }
        let dt = now.timeIntervalSince(last.at)
        guard dt > 0.2 else { return }
        let up = max(0, Double(st.bytesUp - last.up) / dt)
        let down = max(0, Double(st.bytesDown - last.down) / dt)
        rateHistory.append(RateSample(up: up, down: down))
        if rateHistory.count > Self.rateWindow { rateHistory.removeFirst(rateHistory.count - Self.rateWindow) }
    }

    // MARK: - Motor ayarları

    func loadProfiles() async {
        if let r: ProfilesResponse = try? await call("GET", "/api/profiles"), !r.profiles.isEmpty { profiles = r.profiles }
    }

    func selectProfile(_ id: String) {
        selectedProfile = id
        objectWillChange.send()
        guard isConnected else { return }
        send("POST", "/api/profile", body: ["id": id])
    }

    func setDNS(_ providers: [String]) {
        send("POST", "/api/dns", body: ["providers": providers])
    }

    var proxyMode: String { pendingProxyMode ?? status?.proxyMode ?? "global" }

    func setProxyMode(_ mode: String) {
        struct Body: Encodable { let enabled: Bool; let mode: String }
        guard isConnected, mode != proxyMode else { return }
        pendingProxyMode = mode
        Task {
            do {
                let _: Empty = try await call("POST", "/api/systemproxy", body: Body(enabled: true, mode: mode))
                await refresh()
            } catch { lastError = error.localizedDescription }
            if pendingProxyMode == mode { pendingProxyMode = nil }
        }
    }

    func loadDomains() async -> DomainsResponse? {
        try? await call("GET", "/api/domains")
    }

    func saveDomains(mode: String, domains: [String]) {
        send("POST", "/api/domains", body: DomainsResponse(mode: mode, domains: domains))
    }

    func setAutoLearn(_ on: Bool) {
        send("POST", "/api/autolearn", body: ["enabled": on])
    }

    func clearLearned() {
        send("DELETE", "/api/learned", body: Optional<[String: String]>.none)
    }

    func setAdBlock(_ on: Bool, allow: [String]? = nil) {
        struct Body: Encodable { let enabled: Bool; let allow: [String]? }
        send("POST", "/api/adblock", body: Body(enabled: on, allow: allow))
    }

    func setAdLists(_ ids: [String]) {
        struct Body: Encodable { let enabled: Bool; let lists: [String] }
        send("POST", "/api/adblock", body: Body(enabled: status?.adBlock?.enabled ?? true, lists: ids))
    }

    func updateAdLists() {
        send("POST", "/api/adblock/update", body: Optional<[String: String]>.none)
    }

    func startProbe() {
        Task {
            do {
                let _: Empty = try await call("POST", "/api/probe", body: ["apply": true])
                probe = ProbeStatus(running: true, report: nil)
            } catch { lastError = error.localizedDescription }
        }
    }

    // MARK: - Tünele istek

    private struct Empty: Decodable {}

    private struct APIError: LocalizedError {
        let message: String
        var errorDescription: String? { message }
    }

    private func send<B: Encodable>(_ method: String, _ path: String, body: B?) {
        Task {
            do {
                let _: Empty = try await call(method, path, body: body)
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    private func call<T: Decodable>(_ method: String, _ path: String) async throws -> T {
        try await call(method, path, body: Optional<[String: String]>.none)
    }

    private func call<T: Decodable, B: Encodable>(_ method: String, _ path: String, body: B?) async throws -> T {
        guard isConnected, let session = manager?.connection as? NETunnelProviderSession else {
            throw APIError(message: "Slipstream bağlı değil.")
        }
        var bodyString: String?
        if let body { bodyString = String(data: try JSONEncoder().encode(body), encoding: .utf8) }
        let msg = try JSONEncoder().encode(ProviderRequest(method: method, path: path, body: bodyString))
        let data: Data = try await withCheckedThrowingContinuation { cont in
            do {
                try session.sendProviderMessage(msg) { resp in
                    if let resp {
                        cont.resume(returning: resp)
                    } else {
                        cont.resume(throwing: APIError(message: "Tünel yanıt vermedi."))
                    }
                }
            } catch {
                cont.resume(throwing: error)
            }
        }
        let r = try JSONDecoder().decode(ProviderResponse.self, from: data)
        guard (200..<300).contains(r.status) else {
            throw APIError(message: r.body.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        if T.self == Empty.self { return Empty() as! T }
        return try JSONDecoder().decode(T.self, from: Data(r.body.utf8))
    }
}
