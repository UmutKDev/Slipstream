import Foundation

/// Motorun JSON API'sindeki strateji. Alanlar isteğe bağlı, çünkü Go
/// tarafı boş değerleri JSON'a yazmıyor.
struct Strategy: Codable, Hashable {
    var name: String
    var recordSplit: Bool?
    var split: String?
    var splitPos: Int?
    var chunkSize: Int?
    var delayMs: Int?
    var hostCase: Bool?
    var hostNoSpace: Bool?
    var hostMixedValue: Bool?
    var httpSplit: Bool?

    /// Kullanıcıya gösterilen kısa özet.
    var summary: String {
        var parts: [String] = []
        if recordSplit == true { parts.append("TLS kaydı bölme") }
        switch split {
        case "sni": parts.append("SNI'den bölme")
        case "fixed": parts.append("\(splitPos ?? 2). bayttan bölme")
        case "chunk": parts.append("\(chunkSize ?? 4) baytlık parçalar")
        default: break
        }
        if let d = delayMs, d > 0 { parts.append("\(d) ms gecikme") }
        return parts.isEmpty ? "Dönüşüm yok" : parts.joined(separator: " · ")
    }
}

struct Stats: Codable, Hashable {
    var total: Int64
    var active: Int64
    var bypassed: Int64
    var failed: Int64
    var bytesUp: Int64
    var bytesDown: Int64
    var uptimeSec: Int64
}

struct EngineStatus: Codable {
    var version: String
    var listen: String
    var profile: String
    var strategy: Strategy
    var dns: [String]
    var mode: String
    var systemProxy: Bool
    var stats: Stats
}

struct Profile: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var description: String
    var dns: [String]
}

extension Profile {
    /// Motor kapalıyken de seçim yapılabilsin diye uygulamaya gömülü liste.
    /// Motor açılınca kendi listesiyle değiştirilir.
    static let builtin: [Profile] = [
        .init(id: "dengeli", name: "Dengeli (önerilen)", description: "", dns: []),
        .init(id: "turktelekom", name: "Türk Telekom", description: "", dns: []),
        .init(id: "superonline", name: "Turkcell Superonline", description: "", dns: []),
        .init(id: "vodafone", name: "Vodafone Net", description: "", dns: []),
        .init(id: "turknet", name: "TurkNet", description: "", dns: []),
        .init(id: "millenicom", name: "Millenicom ve diğer altyapılar", description: "", dns: []),
        .init(id: "agresif", name: "Agresif", description: "", dns: []),
    ]
}

struct ProfilesResponse: Codable {
    var profiles: [Profile]
}

struct LogEntry: Codable, Identifiable, Hashable {
    var seq: UInt64
    var time: String
    var level: String
    var msg: String
    var id: UInt64 { seq }

    /// "2026-11-02T19:01:23.57Z" biçiminden "19:01:23" çıkarır.
    var clock: String {
        guard let t = time.firstIndex(of: "T") else { return time }
        return String(time[time.index(after: t)...].prefix(8))
    }
}

struct LogsResponse: Codable {
    var entries: [LogEntry]?
}

struct ProbeResult: Codable, Hashable {
    var strategy: Strategy
    var ok: Int
    var total: Int
    var latencyMs: Int64
}

struct ProbeReport: Codable {
    var blocked: [String]?
    var best: ProbeResult?
    var results: [ProbeResult]?
}

struct ProbeStatus: Codable {
    var running: Bool
    var report: ProbeReport?
}

struct DomainsResponse: Codable {
    var mode: String
    var domains: [String]
}

/// Motorun açılışta stdout'a yazdığı hazır satırı.
struct ReadyEvent: Codable {
    var event: String
    var proxy: String
    var api: String
    var version: String
}

struct DNSProviderInfo: Identifiable, Hashable {
    let id: String
    let name: String

    static let all: [DNSProviderInfo] = [
        .init(id: "cloudflare", name: "Cloudflare (1.1.1.1)"),
        .init(id: "google", name: "Google (8.8.8.8)"),
        .init(id: "quad9", name: "Quad9 (9.9.9.9)"),
        .init(id: "adguard", name: "AdGuard (reklam engelli)"),
        .init(id: "system", name: "Sistem DNS'i (önerilmez)"),
    ]
}
