import SwiftUI

struct MenuView: View {
    @EnvironmentObject private var engine: EngineController
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(spacing: 16) {
            header
            hero
            if let err = errorText {
                Label(err, systemImage: "exclamationmark.triangle.fill")
                    .font(.caption)
                    .foregroundStyle(Theme.warn)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
            }
            stats
            adBlockRow
            profilePicker
            probeSection
            Divider()
            footer
        }
        .padding(16)
        .frame(width: 320)
        .fixedSize(horizontal: false, vertical: true)
        .animation(.easeInOut(duration: 0.25), value: engine.state)
    }

    // MARK: - Durum

    private var isOn: Bool { engine.isRunning }
    private var isStarting: Bool { engine.state == .starting }

    private var errorText: String? {
        if case .failed(let m) = engine.state { return m }
        return engine.lastError
    }

    private var statusTitle: String {
        switch engine.state {
        case .running: return "Engeller aşılıyor"
        case .starting: return "Başlatılıyor…"
        case .stopped: return "Koruma kapalı"
        case .failed: return "Başlatılamadı"
        }
    }

    private var statusDetail: String {
        switch engine.state {
        case .running:
            if let s = engine.status { return "\(Theme.duration(s.stats.uptimeSec)) · DNS: \(s.dns.first?.capitalized ?? "–")" }
            return "Bağlantı hazırlanıyor"
        case .starting: return "Motor ve şifreli DNS hazırlanıyor"
        case .stopped: return "Engelli sitelere erişmek için aç"
        case .failed: return "Ayrıntı aşağıda"
        }
    }

    private var tint: Color { isOn ? Theme.ok : Theme.accent }

    // MARK: - Bölümler

    private var header: some View {
        HStack(spacing: 8) {
            Image(systemName: "bolt.shield.fill")
                .foregroundStyle(tint)
            Text("Slipstream").font(.headline)
            Spacer()
            StatusPill(on: isOn, starting: isStarting)
        }
    }

    private var hero: some View {
        VStack(spacing: 12) {
            PowerButton(on: isOn, starting: isStarting, tint: tint, action: engine.toggle)
            VStack(spacing: 3) {
                Text(statusTitle)
                    .font(.system(.title3, design: .rounded).weight(.semibold))
                Text(statusDetail)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 6)
    }

    private var stats: some View {
        let s = isOn ? engine.status?.stats : nil
        return Grid(horizontalSpacing: 8, verticalSpacing: 8) {
            GridRow {
                StatTile(title: "Bağlantı", value: s.map { "\($0.total)" } ?? "–", symbol: "arrow.left.arrow.right")
                StatTile(title: "Atlatılan", value: s.map { "\($0.bypassed)" } ?? "–", symbol: "checkmark.shield")
            }
            GridRow {
                StatTile(title: "Gönderilen", value: s.map { Theme.bytes($0.bytesUp) } ?? "–", symbol: "arrow.up")
                StatTile(title: "Alınan", value: s.map { Theme.bytes($0.bytesDown) } ?? "–", symbol: "arrow.down")
            }
        }
        .opacity(isOn ? 1 : 0.55)
    }

    private var adBlockRow: some View {
        let info = engine.status?.adBlock
        return HStack(spacing: 8) {
            Image(systemName: "hand.raised.fill")
                .foregroundStyle(info?.enabled == true && isOn ? Theme.ok : .secondary)
            VStack(alignment: .leading, spacing: 1) {
                Text("Reklam engelleyici").font(.callout)
                Text(adBlockDetail(info)).font(.caption2).foregroundStyle(.secondary)
            }
            Spacer()
            Toggle("", isOn: Binding(
                get: { info?.enabled ?? false },
                set: { engine.setAdBlock($0) }
            ))
            .labelsHidden()
            .toggleStyle(.switch)
            .controlSize(.small)
            .disabled(!isOn)
        }
        .padding(10)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    private func adBlockDetail(_ info: AdBlockInfo?) -> String {
        guard isOn, let info else { return "Koruma açıkken çalışır" }
        if !info.enabled { return "Kapalı" }
        if !info.hasLists { return "Listeler indiriliyor…" }
        return "\(info.blocked) istek engellendi · \(info.domains.formatted()) alan adı"
    }

    private var profilePicker: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("İnternet sağlayıcın").font(.caption).foregroundStyle(.secondary)
            Picker("", selection: Binding(
                get: { engine.selectedProfile },
                set: { engine.selectProfile($0) }
            )) {
                ForEach(engine.profiles) { p in Text(p.name).tag(p.id) }
                if engine.selectedProfile == "custom" {
                    Text("Otomatik bulunan ayar").tag("custom")
                }
            }
            .labelsHidden()
            if isOn, let st = engine.status?.strategy {
                Text(st.summary).font(.caption2).foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var probeSection: some View {
        let running = engine.probe?.running == true
        return VStack(alignment: .leading, spacing: 6) {
            Button {
                engine.startProbe()
            } label: {
                Label(running ? "Hattın deneniyor…" : "En iyi ayarı otomatik bul", systemImage: "wand.and.stars")
                    .frame(maxWidth: .infinity)
            }
            .controlSize(.large)
            .disabled(running || !isOn)
            if running {
                ProgressView().progressViewStyle(.linear)
            } else if let rep = engine.probe?.report, isOn {
                if let best = rep.best {
                    Text("Bulundu: \(best.ok)/\(best.total) site açıldı, ortalama \(best.latencyMs) ms")
                        .font(.caption).foregroundStyle(Theme.ok)
                } else {
                    Text("Çalışan bir ayar bulunamadı. Agresif profili dene.")
                        .font(.caption).foregroundStyle(Theme.warn)
                }
            } else if !isOn {
                Text("Otomatik bulma için önce korumayı aç.")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    private var footer: some View {
        HStack {
            Button {
                NSApp.activate(ignoringOtherApps: true)
                openWindow(id: "settings")
            } label: {
                Label("Ayarlar", systemImage: "gearshape")
            }
            Spacer()
            Button {
                NSApp.terminate(nil)
            } label: {
                Label("Çık", systemImage: "xmark.circle")
            }
        }
        .buttonStyle(.borderless)
        .foregroundStyle(.secondary)
    }
}

/// Ortadaki büyük açma/kapama düğmesi.
private struct PowerButton: View {
    let on: Bool
    let starting: Bool
    let tint: Color
    let action: () -> Void
    @State private var hovering = false

    var body: some View {
        Button(action: action) {
            ZStack {
                Circle()
                    .fill(on ? AnyShapeStyle(tint.gradient) : AnyShapeStyle(.quaternary.opacity(0.6)))
                    .shadow(color: on ? tint.opacity(0.55) : .clear, radius: 16)
                Circle()
                    .strokeBorder(on ? Color.white.opacity(0.25) : tint.opacity(hovering ? 0.9 : 0.6), lineWidth: 3)
                if starting {
                    ProgressView().controlSize(.large)
                } else {
                    Image(systemName: "power")
                        .font(.system(size: 34, weight: .semibold))
                        .foregroundStyle(on ? Color.white : tint)
                }
            }
            .frame(width: 96, height: 96)
            .scaleEffect(hovering ? 1.04 : 1)
            .animation(.spring(response: 0.3, dampingFraction: 0.7), value: hovering)
        }
        .buttonStyle(.plain)
        .onHover { hovering = $0 }
        .help(on ? "Korumayı kapat" : "Korumayı aç")
        .keyboardShortcut(.defaultAction)
        .accessibilityLabel(on ? "Korumayı kapat" : "Korumayı aç")
    }
}

/// Başlıktaki küçük durum etiketi.
private struct StatusPill: View {
    let on: Bool
    let starting: Bool

    var body: some View {
        HStack(spacing: 5) {
            Circle()
                .fill(on ? Theme.ok : (starting ? Theme.warn : Color.secondary))
                .frame(width: 7, height: 7)
            Text(on ? "Açık" : (starting ? "Başlıyor" : "Kapalı"))
                .font(.caption.weight(.medium))
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 3)
        .background(.quaternary.opacity(0.6), in: Capsule())
    }
}
