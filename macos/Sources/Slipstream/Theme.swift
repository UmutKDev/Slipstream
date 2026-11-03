import AppKit
import SwiftUI

enum Theme {
    static let accent = Color(red: 0.38, green: 0.42, blue: 0.98) // Slipstream mavisi
    static let accent2 = Color(red: 0.62, green: 0.36, blue: 0.98) // mor geçiş
    static let ok = Color(red: 0.16, green: 0.78, blue: 0.52)
    static let okDeep = Color(red: 0.05, green: 0.55, blue: 0.62)
    static let warn = Color.orange
    /// Liquid Glass olmayan sistemlerde kart zemini.
    static let card = Color(nsColor: .controlBackgroundColor)

    static func bytes(_ n: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: n, countStyle: .binary)
    }

    static func rate(_ bps: Double) -> String {
        if bps < 1 { return "0 KB/sn" }
        return ByteCountFormatter.string(fromByteCount: Int64(bps), countStyle: .binary) + "/sn"
    }

    static func duration(_ s: Int64) -> String {
        let h = s / 3600, m = (s % 3600) / 60
        if h > 0 { return "\(h) sa \(m) dk" }
        if m > 0 { return "\(m) dk" }
        return "\(s) sn"
    }
}

// MARK: - Liquid Glass
//
// macOS 26 ve Xcode 26 ile derlenince Apple'ın Liquid Glass malzemesi
// kullanılır. Daha eski sistemlerde aynı görünüme yakın bir buzlu cam
// (material) karşılığı çizilir; böylece uygulama macOS 13'te de çalışır.

private struct LegacyGlass: ViewModifier {
    let radius: CGFloat
    let tint: Color?

    func body(content: Content) -> some View {
        let shape = RoundedRectangle(cornerRadius: radius, style: .continuous)
        content
            .background {
                ZStack {
                    shape.fill(.ultraThinMaterial)
                    if let tint { shape.fill(tint.opacity(0.22)) }
                    shape.fill(LinearGradient(colors: [.white.opacity(0.10), .clear],
                                              startPoint: .top, endPoint: .center))
                }
            }
            .overlay(shape.strokeBorder(
                LinearGradient(colors: [.white.opacity(0.35), .white.opacity(0.06)],
                               startPoint: .topLeading, endPoint: .bottomTrailing),
                lineWidth: 0.8))
            .shadow(color: .black.opacity(0.12), radius: 10, y: 4)
    }
}

extension View {
    /// İçeriği yuvarlak köşeli bir cam yüzeye yerleştirir.
    @ViewBuilder
    func glassSurface(cornerRadius: CGFloat = 16, tint: Color? = nil, interactive: Bool = false) -> some View {
        #if compiler(>=6.2)
        if #available(macOS 26.0, *) {
            self.glassEffect(Glass.regular.tint(tint).interactive(interactive),
                             in: RoundedRectangle(cornerRadius: cornerRadius, style: .continuous))
        } else {
            self.modifier(LegacyGlass(radius: cornerRadius, tint: tint))
        }
        #else
        self.modifier(LegacyGlass(radius: cornerRadius, tint: tint))
        #endif
    }

    /// Kapsül biçiminde cam yüzey (durum etiketleri, küçük düğmeler).
    @ViewBuilder
    func glassCapsule(tint: Color? = nil, interactive: Bool = false) -> some View {
        #if compiler(>=6.2)
        if #available(macOS 26.0, *) {
            self.glassEffect(Glass.regular.tint(tint).interactive(interactive), in: Capsule())
        } else {
            self.modifier(LegacyGlass(radius: 999, tint: tint))
        }
        #else
        self.modifier(LegacyGlass(radius: 999, tint: tint))
        #endif
    }

    /// Daire biçiminde cam yüzey (açma düğmesi).
    @ViewBuilder
    func glassCircle(tint: Color? = nil, interactive: Bool = false) -> some View {
        #if compiler(>=6.2)
        if #available(macOS 26.0, *) {
            self.glassEffect(Glass.regular.tint(tint).interactive(interactive), in: Circle())
        } else {
            self.modifier(LegacyGlass(radius: 999, tint: tint))
        }
        #else
        self.modifier(LegacyGlass(radius: 999, tint: tint))
        #endif
    }

    /// Cam düğme stili; eski sistemlerde standart çerçeveli düğme.
    @ViewBuilder
    func glassButton(prominent: Bool = false) -> some View {
        #if compiler(>=6.2)
        if #available(macOS 26.0, *) {
            if prominent { self.buttonStyle(.glassProminent) } else { self.buttonStyle(.glass) }
        } else {
            if prominent { self.buttonStyle(.borderedProminent) } else { self.buttonStyle(.bordered) }
        }
        #else
        if prominent { self.buttonStyle(.borderedProminent) } else { self.buttonStyle(.bordered) }
        #endif
    }
}

/// Yakın cam yüzeylerin birbirine akarak birleşmesini sağlar (macOS 26).
struct GlassGroup<Content: View>: View {
    var spacing: CGFloat = 12
    @ViewBuilder let content: Content

    var body: some View {
        #if compiler(>=6.2)
        if #available(macOS 26.0, *) {
            GlassEffectContainer(spacing: spacing) { content }
        } else {
            content
        }
        #else
        content
        #endif
    }
}

/// Camın arkasında yavaşça kayan renk bulutları. Cam yüzeyler bu renkleri
/// kırarak derinlik kazanır; koruma açıkken yeşile, kapalıyken maviye döner.
struct AuroraBackground: View {
    var active: Bool
    var intensity: Double = 1

    var body: some View {
        TimelineView(.animation(minimumInterval: 1 / 20)) { tl in
            let t = tl.date.timeIntervalSinceReferenceDate
            GeometryReader { geo in
                let w = geo.size.width, h = geo.size.height
                ZStack {
                    blob(active ? Theme.ok : Theme.accent, size: w * 0.95)
                        .offset(x: -w * 0.28 + sin(t / 7) * w * 0.10, y: -h * 0.30 + cos(t / 9) * h * 0.06)
                    blob(active ? Theme.okDeep : Theme.accent2, size: w * 0.85)
                        .offset(x: w * 0.32 + cos(t / 8) * w * 0.10, y: -h * 0.05 + sin(t / 6) * h * 0.08)
                    blob(active ? Theme.accent : Color.pink, size: w * 0.75)
                        .offset(x: -w * 0.10 + sin(t / 10) * w * 0.12, y: h * 0.35 + cos(t / 7) * h * 0.05)
                }
                .frame(width: w, height: h)
            }
        }
        .opacity(0.55 * intensity)
        .blur(radius: 50)
        .animation(.easeInOut(duration: 0.8), value: active)
        .allowsHitTesting(false)
    }

    private func blob(_ c: Color, size: CGFloat) -> some View {
        Circle().fill(c).frame(width: size, height: size)
    }
}

/// Renkli kare simge (Sistem Ayarları'ndaki gibi).
struct IconTile: View {
    let symbol: String
    let color: Color
    var size: CGFloat = 26

    var body: some View {
        Image(systemName: symbol)
            .font(.system(size: size * 0.5, weight: .semibold))
            .foregroundStyle(.white)
            .frame(width: size, height: size)
            .background(
                LinearGradient(colors: [color.opacity(0.85), color], startPoint: .top, endPoint: .bottom),
                in: RoundedRectangle(cornerRadius: size * 0.28, style: .continuous)
            )
            .overlay(
                RoundedRectangle(cornerRadius: size * 0.28, style: .continuous)
                    .strokeBorder(.white.opacity(0.25), lineWidth: 0.5)
            )
            .shadow(color: color.opacity(0.35), radius: size * 0.12, y: size * 0.05)
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
                .contentTransition(.numericText())
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .glassSurface(cornerRadius: 14)
    }
}

/// İndirme ve yükleme hızının küçük alan grafiği.
struct Sparkline: View {
    let values: [Double]
    let color: Color
    var minCount = 40

    var body: some View {
        GeometryReader { geo in
            let pts = points(in: geo.size)
            ZStack {
                if pts.count > 1 {
                    Path { p in
                        p.move(to: CGPoint(x: pts[0].x, y: geo.size.height))
                        pts.forEach { p.addLine(to: $0) }
                        p.addLine(to: CGPoint(x: pts[pts.count - 1].x, y: geo.size.height))
                        p.closeSubpath()
                    }
                    .fill(LinearGradient(colors: [color.opacity(0.45), color.opacity(0.0)],
                                         startPoint: .top, endPoint: .bottom))
                    Path { p in
                        p.move(to: pts[0])
                        pts.dropFirst().forEach { p.addLine(to: $0) }
                    }
                    .stroke(color, style: StrokeStyle(lineWidth: 1.6, lineCap: .round, lineJoin: .round))
                }
            }
        }
    }

    private func points(in size: CGSize) -> [CGPoint] {
        guard !values.isEmpty else { return [] }
        let peak = max(values.max() ?? 0, 16 * 1024)
        let n = max(values.count, minCount)
        let step = size.width / CGFloat(max(n - 1, 1))
        let offset = n - values.count
        return values.enumerated().map { i, v in
            CGPoint(x: CGFloat(i + offset) * step,
                    y: size.height - CGFloat(v / peak) * (size.height - 2) - 1)
        }
    }
}
