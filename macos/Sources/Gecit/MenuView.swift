import SwiftUI

struct MenuView: View {
    @EnvironmentObject private var engine: EngineController
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header
            powerButton
            if let err = errorText {
                Text(err)
                    .font(.caption)
                    .foregroundStyle(Theme.warn)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if engine.isRunning, let s = engine.status {
                stats(s.stats)
                profilePicker(current: s.profile, strategy: s.strategy)
                probeSection
            }
            Divider()
            footer
        }
        .padding(16)
        .frame(width: 320)
    }

    private var errorText: String? {
        if case .failed(let m) = engine.state { return m }
        return engine.lastError
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: "shield.lefthalf.filled")
                .font(.title2)
                .foregroundStyle(engine.isRunning ? Theme.ok : .secondary)
            VStack(alignment: .leading, spacing: 2) {
                Text("Geçit").font(.headline)
                Text(subtitle).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
        }
    }

    private var subtitle: String {
        switch engine.state {
        case .running:
            if let s = engine.status { return "Koruma açık · \(Theme.duration(s.stats.uptimeSec))" }
            return "Koruma açık"
        case .starting: return "Başlatılıyor…"
        case .stopped: return "Koruma kapalı"
        case .failed: return "Hata"
        }
    }

    private var powerButton: some View {
        Button(action: engine.toggle) {
            HStack {
                Image(systemName: "power")
                Text(engine.isRunning ? "Kapat" : "Aç")
                    .fontWeight(.semibold)
                Spacer()
                if engine.state == .starting { ProgressView().controlSize(.small) }
            }
            .padding(.vertical, 10)
            .padding(.horizontal, 14)
            .frame(maxWidth: .infinity)
            .foregroundStyle(.white)
            .background(engine.isRunning ? Theme.ok : Theme.accent,
                        in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        }
        .buttonStyle(.plain)
        .keyboardShortcut(.defaultAction)
    }

    private func stats(_ s: Stats) -> some View {
        Grid(horizontalSpacing: 8, verticalSpacing: 8) {
            GridRow {
                StatTile(title: "Bağlantı", value: "\(s.total)", symbol: "arrow.left.arrow.right")
                StatTile(title: "Atlatılan", value: "\(s.bypassed)", symbol: "checkmark.shield")
            }
            GridRow {
                StatTile(title: "Gönderilen", value: Theme.bytes(s.bytesUp), symbol: "arrow.up")
                StatTile(title: "Alınan", value: Theme.bytes(s.bytesDown), symbol: "arrow.down")
            }
        }
    }

    private func profilePicker(current: String, strategy: Strategy) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("İnternet sağlayıcın").font(.caption).foregroundStyle(.secondary)
            Picker("", selection: Binding(
                get: { current },
                set: { engine.selectProfile($0) }
            )) {
                ForEach(engine.profiles) { p in Text(p.name).tag(p.id) }
                if current == "custom" { Text("Otomatik bulunan (\(strategy.name))").tag("custom") }
            }
            .labelsHidden()
            Text(strategy.summary).font(.caption2).foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private var probeSection: some View {
        let running = engine.probe?.running == true
        VStack(alignment: .leading, spacing: 6) {
            Button {
                engine.startProbe()
            } label: {
                Label(running ? "Hattın deneniyor…" : "En iyi ayarı otomatik bul", systemImage: "wand.and.stars")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .disabled(running)
            if running { ProgressView().progressViewStyle(.linear) }
            if let rep = engine.probe?.report, !running {
                if let best = rep.best {
                    Text("Bulundu: \(best.strategy.name), \(best.ok)/\(best.total) site, ortalama \(best.latencyMs) ms")
                        .font(.caption).foregroundStyle(Theme.ok)
                } else {
                    Text("Çalışan bir ayar bulunamadı. Agresif profili dene.")
                        .font(.caption).foregroundStyle(Theme.warn)
                }
            }
        }
    }

    private var footer: some View {
        HStack {
            Button("Ayarlar…") {
                NSApp.activate(ignoringOtherApps: true)
                openWindow(id: "settings")
            }
            Spacer()
            Button("Çık") { NSApp.terminate(nil) }
        }
        .buttonStyle(.borderless)
    }
}
