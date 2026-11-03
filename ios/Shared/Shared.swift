import Foundation

/// Uygulama ile tünel eklentisinin ortak sabitleri.
enum Shared {
    static let appGroup = "group.com.umutkdev.slipstream"
    static let tunnelBundleID = "com.umutkdev.slipstream.ios.tunnel"

    /// Ayarların, öğrenilen sitelerin ve reklam listelerinin tutulduğu ortak klasör.
    static var engineDirectory: URL {
        let base = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroup)
            ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        let dir = base.appendingPathComponent("engine", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir
    }
}

/// Uygulamanın tünele gönderdiği istek. Motorun macOS'taki kontrol
/// API'siyle aynı yol ve gövdeyi taşır.
struct ProviderRequest: Codable {
    var method: String
    var path: String
    var body: String?
}

/// Tünelin yanıtı: HTTP durum kodu ve gövde.
struct ProviderResponse: Codable {
    var status: Int
    var body: String
}
