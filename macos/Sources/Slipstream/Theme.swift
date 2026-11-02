import SwiftUI

enum Theme {
    static let accent = Color(red: 0.89, green: 0.12, blue: 0.20) // Türk kırmızısı
    static let ok = Color(red: 0.16, green: 0.72, blue: 0.45)
    static let warn = Color.orange

    static func bytes(_ n: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: n, countStyle: .binary)
    }

    static func duration(_ s: Int64) -> String {
        let h = s / 3600, m = (s % 3600) / 60
        if h > 0 { return "\(h) sa \(m) dk" }
        if m > 0 { return "\(m) dk" }
        return "\(s) sn"
    }
}

/// Menüdeki küçük istatistik kutusu.
struct StatTile: View {
    let title: String
    let value: String
    let symbol: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Label(title, systemImage: symbol)
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.system(.title3, design: .rounded).weight(.semibold))
                .monospacedDigit()
                .lineLimit(1)
                .minimumScaleFactor(0.7)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(10)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}
