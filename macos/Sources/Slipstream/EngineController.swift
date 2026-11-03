import Foundation
import SwiftUI

/// slipstreamd motorunu başlatır, durdurur ve kontrol API'si üzerinden yönetir.
@MainActor
final class EngineController: ObservableObject {
    enum State: Equatable {
        case stopped
        case starting
        case running
        case failed(String)
    }

    @Published private(set) var state: State = .stopped
    @Published private(set) var status: EngineStatus?
    @Published private(set) var profiles: [Profile] = Profile.builtin
    @Published private(set) var logs: [LogEntry] = []
    @Published private(set) var probe: ProbeStatus?
    @Published var lastError: String?
    /// Kullanıcının seçtiği ama motorun henüz uygulamadığı yönlendirme kipi.
    @Published private(set) var pendingProxyMode: String?
    /// Son ölçümlerdeki indirme ve yükleme hızı (bayt/sn), en eskisi başta.
    @Published private(set) var rateHistory: [RateSample] = []

    @AppStorage("systemProxy") var useSystemProxy = true
    @AppStorage("proxyPort") var proxyPort = 8881
    @AppStorage("apiPort") var apiPort = 8882
    @AppStorage("startOnLaunch") var startOnLaunch = true
    @AppStorage("profile") var selectedProfile = "dengeli"

    private var process: Process?
    private var pollTask: Task<Void, Never>?
    private var lastLogSeq: UInt64 = 0
    private let token = UUID().uuidString + UUID().uuidString
    private var stdoutBuffer = Data()
    private var lastTotals: (up: Int64, down: Int64, at: Date)?
    static let rateWindow = 40

    /// Önceki oturum düzgün kapanmadıysa kalan işaret dosyası.
    private var markerURL: URL {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Slipstream", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("running")
    }

    var isRunning: Bool { state == .running }

    init() {
        recoverFromCrash()
        if startOnLaunch { start() }
    }

    // MARK: - Yaşam döngüsü

    func toggle() {
        switch state {
        case .running, .starting: stop()
        default: start()
        }
    }

    func start() {
        guard process == nil else { return }
        guard let bin = engineURL() else {
            state = .failed("Motor dosyası (slipstreamd) uygulama paketinde bulunamadı.")
            return
        }
        state = .starting
        lastError = nil
        let p = Process()
        p.executableURL = bin
        var args = ["run",
                    "-listen", "127.0.0.1:\(proxyPort)",
                    "-api", "127.0.0.1:\(apiPort)",
                    "-token", token,
                    "-profile", selectedProfile]
        if useSystemProxy { args.append("-system-proxy") }
        p.arguments = args

        let out = Pipe()
        p.standardOutput = out
        p.standardError = FileHandle.nullDevice
        out.fileHandleForReading.readabilityHandler = { [weak self] h in
            let data = h.availableData
            guard !data.isEmpty else { return }
            Task { @MainActor in self?.consume(data) }
        }
        p.terminationHandler = { [weak self] proc in
            Task { @MainActor in self?.engineExited(code: proc.terminationStatus) }
        }
        do {
            try p.run()
            process = p
            FileManager.default.createFile(atPath: markerURL.path, contents: Data())
        } catch {
            state = .failed("Motor başlatılamadı: \(error.localizedDescription)")
        }
    }

    func stop() {
        pollTask?.cancel()
        pollTask = nil
        rateHistory = []
        lastTotals = nil
        guard let p = process else {
            state = .stopped
            return
        }
        // SIGTERM alan motor sistem proxy'sini kendisi geri alır.
        p.terminate()
    }

    /// Uygulama kapanırken çağrılır. Motorun proxy'yi geri alması için kısa süre bekler.
    func shutdownSync() {
        guard let p = process else { return }
        p.terminate()
        let deadline = Date().addingTimeInterval(2)
        while p.isRunning && Date() < deadline {
            RunLoop.current.run(until: Date().addingTimeInterval(0.05))
        }
        if p.isRunning { disableSystemProxyDirectly() }
        try? FileManager.default.removeItem(at: markerURL)
    }

    private func engineExited(code: Int32) {
        (process?.standardOutput as? Pipe)?.fileHandleForReading.readabilityHandler = nil
        process = nil
        pollTask?.cancel()
        pollTask = nil
        try? FileManager.default.removeItem(at: markerURL)
        if case .failed = state { return }
        if code == 0 || code == 15 {
            state = .stopped
        } else {
            state = .failed("Motor beklenmedik biçimde kapandı (kod \(code)). Port başka bir uygulama tarafından kullanılıyor olabilir.")
            disableSystemProxyDirectly()
        }
        status = nil
    }

    private func consume(_ data: Data) {
        stdoutBuffer.append(data)
        while let nl = stdoutBuffer.firstIndex(of: 0x0A) {
            let line = stdoutBuffer[stdoutBuffer.startIndex..<nl]
            stdoutBuffer.removeSubrange(stdoutBuffer.startIndex...nl)
            if let ev = try? JSONDecoder().decode(ReadyEvent.self, from: line), ev.event == "ready" {
                state = .running
                startPolling()
                Task { await loadProfiles() }
            }
        }
    }

    private func engineURL() -> URL? {
        if let u = Bundle.main.url(forAuxiliaryExecutable: "slipstreamd") { return u }
        if let u = Bundle.main.resourceURL?.appendingPathComponent("slipstreamd"),
           FileManager.default.isExecutableFile(atPath: u.path) { return u }
        // Geliştirme sırasında: depo kökündeki derlenmiş motor.
        let dev = URL(fileURLWithPath: FileManager.default.currentDirectoryPath).appendingPathComponent("../bin/slipstreamd")
        return FileManager.default.isExecutableFile(atPath: dev.path) ? dev : nil
    }

    /// Önceki oturum çöktüyse sistem proxy'si ölü bir porta işaret ediyor
    /// olabilir. Bu durumda internet kesilir, o yüzden açılışta geri alınır.
    private func recoverFromCrash() {
        if FileManager.default.fileExists(atPath: markerURL.path) {
            disableSystemProxyDirectly()
            try? FileManager.default.removeItem(at: markerURL)
        }
    }

    private func disableSystemProxyDirectly() {
        guard let bin = engineURL() else { return }
        let p = Process()
        p.executableURL = bin
        p.arguments = ["sysproxy", "off"]
        try? p.run()
        p.waitUntilExit()
    }

    // MARK: - API

    private func startPolling() {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                try? await Task.sleep(nanoseconds: 1_500_000_000)
            }
        }
    }

    func refresh() async {
        guard isRunning else { return }
        if let s: EngineStatus = try? await call("GET", "/api/status") {
            recordRate(s.stats)
            status = s
            if selectedProfile != s.profile { selectedProfile = s.profile }
        }
        if let l: LogsResponse = try? await call("GET", "/api/logs?since=\(lastLogSeq)"),
           let entries = l.entries, !entries.isEmpty {
            logs.append(contentsOf: entries)
            if logs.count > 500 { logs.removeFirst(logs.count - 500) }
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
        // Motor yeniden başlarsa sayaçlar sıfırlanır; eksi hız gösterme.
        let up = max(0, Double(st.bytesUp - last.up) / dt)
        let down = max(0, Double(st.bytesDown - last.down) / dt)
        rateHistory.append(RateSample(up: up, down: down))
        if rateHistory.count > Self.rateWindow { rateHistory.removeFirst(rateHistory.count - Self.rateWindow) }
    }

    func loadProfiles() async {
        if let r: ProfilesResponse = try? await call("GET", "/api/profiles"), !r.profiles.isEmpty { profiles = r.profiles }
    }

    func selectProfile(_ id: String) {
        selectedProfile = id
        objectWillChange.send()
        guard isRunning else { return }
        Task {
            do {
                let _: Empty = try await call("POST", "/api/profile", body: ["id": id])
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func setDNS(_ providers: [String]) {
        Task {
            do {
                let _: Empty = try await call("POST", "/api/dns", body: ["providers": providers])
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func flushDNS() {
        Task { let _: Empty? = try? await call("POST", "/api/dns/flush") }
    }

    /// Ekranda gösterilecek yönlendirme kipi; seçim anında yansır.
    var proxyMode: String { pendingProxyMode ?? status?.proxyMode ?? "global" }

    /// Sistem proxy'si değiştiriliyor mu (networksetup birkaç saniye sürebilir).
    var proxyApplying: Bool { pendingProxyMode != nil || status?.proxyBusy == true }

    func setProxyMode(_ mode: String) {
        struct Body: Encodable { let enabled: Bool; let mode: String }
        guard isRunning, mode != proxyMode else { return }
        pendingProxyMode = mode
        Task {
            do {
                let _: Empty = try await call("POST", "/api/systemproxy", body: Body(enabled: useSystemProxy, mode: mode))
                await refresh()
            } catch { lastError = error.localizedDescription }
            if pendingProxyMode == mode { pendingProxyMode = nil }
        }
    }

    func setSystemProxy(_ on: Bool) {
        useSystemProxy = on
        guard isRunning else { return }
        Task {
            do {
                let _: Empty = try await call("POST", "/api/systemproxy", body: ["enabled": on])
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func loadDomains() async -> DomainsResponse? {
        try? await call("GET", "/api/domains")
    }

    func saveDomains(mode: String, domains: [String]) {
        Task {
            do {
                let _: Empty = try await call("POST", "/api/domains", body: DomainsResponse(mode: mode, domains: domains))
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func setAutoLearn(_ on: Bool) {
        Task {
            do {
                let _: Empty = try await call("POST", "/api/autolearn", body: ["enabled": on])
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func clearLearned() {
        Task {
            do {
                let _: Empty = try await call("DELETE", "/api/learned")
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func setAdBlock(_ on: Bool, allow: [String]? = nil) {
        struct Body: Encodable { let enabled: Bool; let allow: [String]? }
        Task {
            do {
                let _: Empty = try await call("POST", "/api/adblock", body: Body(enabled: on, allow: allow))
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    /// Kullanılacak engel listelerini değiştirir; yeni listeler arka planda iner.
    func setAdLists(_ ids: [String]) {
        struct Body: Encodable { let enabled: Bool; let lists: [String] }
        guard isRunning else { return }
        let enabled = status?.adBlock?.enabled ?? true
        Task {
            do {
                let _: Empty = try await call("POST", "/api/adblock", body: Body(enabled: enabled, lists: ids))
                await refresh()
            } catch { lastError = error.localizedDescription }
        }
    }

    func updateAdLists() {
        Task {
            do {
                let _: Empty = try await call("POST", "/api/adblock/update")
            } catch { lastError = error.localizedDescription }
        }
    }

    func startProbe(apply: Bool = true) {
        Task {
            do {
                let _: Empty = try await call("POST", "/api/probe", body: ["apply": apply])
                probe = ProbeStatus(running: true, report: nil)
            } catch { lastError = error.localizedDescription }
        }
    }

    private struct Empty: Decodable {}

    private struct APIError: LocalizedError {
        let message: String
        var errorDescription: String? { message }
    }

    private func call<T: Decodable>(_ method: String, _ path: String) async throws -> T {
        try await call(method, path, body: Optional<[String: String]>.none)
    }

    private func call<T: Decodable, B: Encodable>(_ method: String, _ path: String, body: B?) async throws -> T {
        guard let url = URL(string: "http://127.0.0.1:\(apiPort)\(path)") else { throw URLError(.badURL) }
        var req = URLRequest(url: url, timeoutInterval: 5)
        req.httpMethod = method
        req.setValue(token, forHTTPHeaderField: "X-Slipstream-Token")
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONEncoder().encode(body)
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        if let http = resp as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
            throw APIError(message: String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "HTTP \(http.statusCode)")
        }
        if T.self == Empty.self { return Empty() as! T }
        return try JSONDecoder().decode(T.self, from: data)
    }
}
