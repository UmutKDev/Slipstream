import SwiftUI

struct MenuView: View {
    @EnvironmentObject private var engine: EngineController
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        GlassGroup(spacing: 2) {
            VStack(spacing: 14) {
                header
                hero
                if let err = errorText {
                    Label(err, systemImage: "exclamationmark.triangle.fill")
                        .font(.caption)
                        .foregroundStyle(Theme.warn)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(10)
                        .glassSurface(cornerRadius: 12, tint: Theme.warn.opacity(0.4))
                }
                traffic
                stats
                adBlockRow
                profileCard
                probeSection
                footer
            }
            .padding(16)
        }
        .frame(width: 340)
        .fixedSize(horizontal: false, vertical: true)
        .background(AuroraBackground(active: isOn))
        .animation(.easeInOut(duration: 0.35), value: engine.state)
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
        case .stopped: return "Engelli sitelere erişmek için dokun"
        case .failed: return "Ayrıntı aşağıda"
        }
    }

    private var tint: Color { isOn ? Theme.ok : Theme.accent }

    // MARK: - Bölümler

    private var header: some View {
        HStack(spacing: 10) {
            IconTile(symbol: "bolt.shield.fill", color: tint, size: 26)
            Text("Slipstream").font(.system(.headline, design: .rounded))
            Spacer()
            StatusPill(on: isOn, starting: isStarting)
        }
    }

    private var hero: some View {
        VStack(spacing: 12) {
            PowerButton(on: isOn, starting: isStarting, tint: tint, action: engine.toggle)
            VStack(spacing: 3) {
                Text(statusTitle)
                    .font(.system(.title2, design: .rounded).weight(.semibold))
                    .contentTransition(.opacity)
                Text(statusDetail)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 4)
    }

    private var traffic: some View {
        let samples = engine.rateHistory
        let now = samples.last
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label("Canlı trafik", systemImage: "waveform.path.ecg")
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.secondary)
                Spacer()
                RateLabel(symbol: "arrow.down", value: isOn ? Theme.rate(now?.down ?? 0) : "–", color: tint)
                RateLabel(symbol: "arrow.up", value: isOn ? Theme.rate(now?.up ?? 0) : "–", color: Theme.accent2)
            }
            ZStack {
                Sparkline(values: samples.map(\.up), color: Theme.accent2, minCount: EngineController.rateWindow)
                    .opacity(0.8)
                Sparkline(values: samples.map(\.down), color: tint, minCount: EngineController.rateWindow)
                if !isOn {
                    Text("Koruma açılınca trafik burada akar")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }
            }
            .frame(height: 44)
        }
        .padding(12)
        .glassSurface(cornerRadius: 16)
    }

    private var stats: some View {
        let s = isOn ? engine.status?.stats : nil
        return HStack(spacing: 10) {
            StatTile(title: "Bağlantı", value: s.map { "\($0.total)" } ?? "–", symbol: "arrow.left.arrow.right")
            StatTile(title: "Atlatılan", value: s.map { "\($0.bypassed)" } ?? "–", symbol: "checkmark.shield")
            StatTile(title: "Toplam", value: s.map { Theme.bytes($0.bytesUp + $0.bytesDown) } ?? "–", symbol: "chart.bar.fill")
        }
        .opacity(isOn ? 1 : 0.6)
    }

    private var adBlockRow: some View {
        let info = engine.status?.adBlock
        let active = info?.enabled == true && isOn
        return HStack(spacing: 10) {
            IconTile(symbol: "hand.raised.fill", color: active ? .red : .gray, size: 28)
            VStack(alignment: .leading, spacing: 1) {
                Text("Reklam engelleyici").font(.callout.weight(.medium))
                Text(adBlockDetail(info)).font(.caption2).foregroundStyle(.secondary)
                    .contentTransition(.numericText())
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
        .padding(12)
        .glassSurface(cornerRadius: 16)
    }

    private func adBlockDetail(_ info: AdBlockInfo?) -> String {
        guard isOn, let info else { return "Koruma açıkken çalışır" }
        if !info.enabled { return "Kapalı" }
        if !info.hasLists { return "Listeler indiriliyor…" }
        return "\(info.blocked) istek engellendi · \(info.domains.formatted()) alan adı"
    }

    private var profileCard: some View {
        HStack(spacing: 10) {
            IconTile(symbol: "antenna.radiowaves.left.and.right", color: Theme.accent, size: 28)
            VStack(alignment: .leading, spacing: 2) {
                Text("İnternet sağlayıcın").font(.caption2).foregroundStyle(.secondary)
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
                .pickerStyle(.menu)
                .fixedSize()
                if isOn, let st = engine.status?.strategy {
                    Text(st.summary).font(.caption2).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(12)
        .glassSurface(cornerRadius: 16)
    }

    private var probeSection: some View {
        let running = engine.probe?.running == true
        return VStack(alignment: .leading, spacing: 8) {
            Button {
                engine.startProbe()
            } label: {
                Label(running ? "Hattın deneniyor…" : "En iyi ayarı otomatik bul", systemImage: "wand.and.stars")
                    .font(.callout.weight(.medium))
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 3)
            }
            .glassButton(prominent: isOn && !running)
            .tint(Theme.accent)
            .controlSize(.large)
            .disabled(running || !isOn)
            if running {
                ProgressView().progressViewStyle(.linear).tint(Theme.accent)
            } else if let rep = engine.probe?.report, isOn {
                if let best = rep.best {
                    Label("Bulundu: \(best.ok)/\(best.total) site açıldı, ortalama \(best.latencyMs) ms", systemImage: "checkmark.circle.fill")
                        .font(.caption).foregroundStyle(Theme.ok)
                } else {
                    Label("Çalışan bir ayar bulunamadı. Agresif profili dene.", systemImage: "exclamationmark.circle.fill")
                        .font(.caption).foregroundStyle(Theme.warn)
                }
            }
        }
    }

    private var footer: some View {
        HStack(spacing: 8) {
            Button {
                NSApp.activate(ignoringOtherApps: true)
                openWindow(id: "settings")
            } label: {
                Label("Ayarlar", systemImage: "gearshape.fill")
                    .frame(maxWidth: .infinity)
            }
            Button {
                NSApp.terminate(nil)
            } label: {
                Label("Çık", systemImage: "power")
                    .frame(maxWidth: .infinity)
            }
        }
        .glassButton()
        .controlSize(.regular)
    }
}

private struct RateLabel: View {
    let symbol: String
    let value: String
    let color: Color

    var body: some View {
        HStack(spacing: 3) {
            Image(systemName: symbol).foregroundStyle(color)
            Text(value).monospacedDigit().contentTransition(.numericText())
        }
        .font(.caption.weight(.semibold))
    }
}

/// Ortadaki büyük cam açma/kapama düğmesi.
private struct PowerButton: View {
    let on: Bool
    let starting: Bool
    let tint: Color
    let action: () -> Void
    @State private var hovering = false

    var body: some View {
        Button(action: action) {
            ZStack {
                // Açıkken dışa doğru yayılan halka.
                if on {
                    TimelineView(.animation(minimumInterval: 1 / 30)) { tl in
                        let phase = tl.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 2.4) / 2.4
                        Circle()
                            .stroke(tint.opacity(0.6), lineWidth: 2)
                            .frame(width: 106, height: 106)
                            .scaleEffect(0.98 + 0.26 * phase)
                            .opacity(0.9 * (1 - phase))
                    }
                }
                ZStack {
                    if on {
                        Circle().fill(
                            RadialGradient(colors: [tint.opacity(0.9), tint.opacity(0.55)],
                                           center: .topLeading, startRadius: 4, endRadius: 110)
                        )
                    }
                    if starting {
                        ProgressView().controlSize(.large)
                    } else {
                        Image(systemName: "power")
                            .font(.system(size: 38, weight: .semibold))
                            .foregroundStyle(on ? Color.white : tint)
                            .shadow(color: on ? .black.opacity(0.2) : .clear, radius: 3, y: 1)
                    }
                }
                .frame(width: 104, height: 104)
                .glassCircle(tint: on ? tint.opacity(0.35) : nil, interactive: true)
                .shadow(color: on ? tint.opacity(0.55) : .black.opacity(0.15), radius: on ? 22 : 10)
            }
            .frame(width: 132, height: 132)
            .scaleEffect(hovering ? 1.04 : 1)
            .animation(.spring(response: 0.3, dampingFraction: 0.7), value: hovering)
            .contentShape(Circle())
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
                .shadow(color: on ? Theme.ok : .clear, radius: 3)
            Text(on ? "Açık" : (starting ? "Başlıyor" : "Kapalı"))
                .font(.caption.weight(.semibold))
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 4)
        .glassCapsule()
    }
}
