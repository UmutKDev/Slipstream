import NetworkExtension
import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var tunnel: TunnelController
    @State private var showSettings = false

    var body: some View {
        NavigationStack {
            ScrollView {
                GlassGroup(spacing: 4) {
                    VStack(spacing: 16) {
                        hero
                        if let err = tunnel.lastError {
                            Label(err, systemImage: "exclamationmark.triangle.fill")
                                .font(.footnote)
                                .foregroundStyle(Theme.warn)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .fixedSize(horizontal: false, vertical: true)
                                .padding(12)
                                .glassSurface(cornerRadius: 14, tint: Theme.warn.opacity(0.4))
                                .onTapGesture { tunnel.lastError = nil }
                        }
                        traffic
                        stats
                        adBlockCard
                        profileCard
                        modeCard
                        probeSection
                    }
                    .padding(.horizontal, 16)
                    .padding(.bottom, 24)
                }
            }
            .background(AuroraBackground(active: isOn).ignoresSafeArea())
            .navigationTitle("Slipstream")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { showSettings = true } label: { Image(systemName: "gearshape") }
                        .accessibilityLabel("Ayarlar")
                }
            }
            .sheet(isPresented: $showSettings) {
                SettingsView().environmentObject(tunnel)
            }
            .animation(.easeInOut(duration: 0.35), value: tunnel.vpnStatus)
        }
    }

    // MARK: - Durum

    private var isOn: Bool { tunnel.isConnected }
    private var tint: Color { isOn ? Theme.ok : Theme.accent }

    private var statusTitle: String {
        switch tunnel.vpnStatus {
        case .connected: return "Engeller aşılıyor"
        case .connecting, .reasserting: return "Bağlanıyor…"
        case .disconnecting: return "Kapatılıyor…"
        default: return "Koruma kapalı"
        }
    }

    private var statusDetail: String {
        switch tunnel.vpnStatus {
        case .connected:
            if let s = tunnel.status { return "\(Theme.duration(s.stats.uptimeSec)) · DNS: \(s.dns.first?.capitalized ?? "–")" }
            return "Bağlantı hazırlanıyor"
        case .connecting, .reasserting: return "Motor ve şifreli DNS hazırlanıyor"
        default: return "Engelli sitelere erişmek için dokun"
        }
    }

    // MARK: - Bölümler

    private var hero: some View {
        VStack(spacing: 14) {
            PowerButton(on: isOn, busy: tunnel.isBusy, tint: tint, action: tunnel.toggle)
                .disabled(!tunnel.loaded)
            VStack(spacing: 4) {
                Text(statusTitle)
                    .font(.system(.title2, design: .rounded).weight(.semibold))
                    .contentTransition(.opacity)
                Text(statusDetail)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 8)
    }

    private var traffic: some View {
        let samples = tunnel.rateHistory
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
                Sparkline(values: samples.map(\.up), color: Theme.accent2, minCount: TunnelController.rateWindow)
                    .opacity(0.8)
                Sparkline(values: samples.map(\.down), color: tint, minCount: TunnelController.rateWindow)
                if !isOn {
                    Text("Koruma açılınca trafik burada akar")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }
            }
            .frame(height: 52)
        }
        .padding(14)
        .glassSurface(cornerRadius: 18)
    }

    private var stats: some View {
        let s = isOn ? tunnel.status?.stats : nil
        return HStack(spacing: 10) {
            StatTile(title: "Bağlantı", value: s.map { "\($0.total)" } ?? "–", symbol: "arrow.left.arrow.right")
            StatTile(title: "Atlatılan", value: s.map { "\($0.bypassed)" } ?? "–", symbol: "checkmark.shield")
            StatTile(title: "Toplam", value: s.map { Theme.bytes($0.bytesUp + $0.bytesDown) } ?? "–", symbol: "chart.bar.fill")
        }
        .opacity(isOn ? 1 : 0.6)
    }

    private var adBlockCard: some View {
        let info = tunnel.status?.adBlock
        let active = info?.enabled == true && isOn
        return HStack(spacing: 12) {
            IconTile(symbol: "hand.raised.fill", color: active ? .red : .gray, size: 32)
            VStack(alignment: .leading, spacing: 2) {
                Text("Reklam engelleyici").font(.callout.weight(.medium))
                Text(adBlockDetail(info)).font(.caption).foregroundStyle(.secondary)
                    .contentTransition(.numericText())
            }
            Spacer()
            Toggle("Reklam engelleyici", isOn: Binding(
                get: { info?.enabled ?? false },
                set: { tunnel.setAdBlock($0) }
            ))
            .labelsHidden()
            .disabled(!isOn)
        }
        .padding(14)
        .glassSurface(cornerRadius: 18)
    }

    private func adBlockDetail(_ info: AdBlockInfo?) -> String {
        guard isOn, let info else { return "Koruma açıkken bütün uygulamalarda çalışır" }
        if !info.enabled { return "Kapalı" }
        if !info.hasLists { return "Listeler indiriliyor…" }
        return "\(info.blocked) istek engellendi · \(info.domains.formatted()) alan adı"
    }

    private var profileCard: some View {
        HStack(spacing: 12) {
            IconTile(symbol: "antenna.radiowaves.left.and.right", color: Theme.accent, size: 32)
            VStack(alignment: .leading, spacing: 2) {
                Text("İnternet sağlayıcın").font(.caption).foregroundStyle(.secondary)
                Picker("İnternet sağlayıcın", selection: Binding(
                    get: { tunnel.selectedProfile },
                    set: { tunnel.selectProfile($0) }
                )) {
                    ForEach(tunnel.profiles) { p in Text(p.name).tag(p.id) }
                    if tunnel.selectedProfile == "custom" {
                        Text("Otomatik bulunan ayar").tag("custom")
                    }
                }
                .labelsHidden()
                .pickerStyle(.menu)
                .tint(.primary)
                if isOn, let st = tunnel.status?.strategy {
                    Text(st.summary).font(.caption2).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(14)
        .glassSurface(cornerRadius: 18)
    }

    private var modeCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Hangi trafik geçsin?", systemImage: "arrow.triangle.branch")
                .font(.caption.weight(.medium))
                .foregroundStyle(.secondary)
            Picker("Kip", selection: Binding(
                get: { tunnel.proxyMode },
                set: { tunnel.setProxyMode($0) }
            )) {
                Text("Tüm trafik").tag("global")
                Text("Yalnız listedekiler").tag("pac")
            }
            .pickerStyle(.segmented)
            .disabled(!isOn)
            Text(tunnel.proxyMode == "pac"
                 ? "Yalnız engelli site listesi ve öğrenilen siteler motordan geçer. Pil ve hız için en iyisi."
                 : "Bütün web trafiği motordan geçer. Engelli sitenin listede olması gerekmez.")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(14)
        .glassSurface(cornerRadius: 18)
    }

    private var probeSection: some View {
        let running = tunnel.probe?.running == true
        return VStack(alignment: .leading, spacing: 10) {
            Button {
                tunnel.startProbe()
            } label: {
                Label(running ? "Hattın deneniyor…" : "En iyi ayarı otomatik bul", systemImage: "wand.and.stars")
                    .font(.callout.weight(.medium))
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 4)
            }
            .glassButton()
            .disabled(!isOn || running)
            if let rep = tunnel.probe?.report, !running {
                if let best = rep.best {
                    Text("Bulunan ayar: \(best.strategy.summary) (\(best.ok)/\(best.total) site açıldı)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    Text("Çalışan bir ayar bulunamadı. Başka bir profil dene.")
                        .font(.caption)
                        .foregroundStyle(Theme.warn)
                }
            }
        }
    }
}

/// Büyük açma kapama düğmesi.
struct PowerButton: View {
    let on: Bool
    let busy: Bool
    let tint: Color
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            ZStack {
                Circle()
                    .fill(RadialGradient(colors: [tint.opacity(on ? 0.55 : 0.25), .clear],
                                         center: .center, startRadius: 10, endRadius: 110))
                    .frame(width: 200, height: 200)
                    .blur(radius: 8)
                Image(systemName: "power")
                    .font(.system(size: 54, weight: .semibold))
                    .foregroundStyle(on ? .white : tint)
                    .frame(width: 140, height: 140)
                    .background(Circle().fill(on ? tint : .clear))
                    .glassCircle(tint: on ? tint : nil, interactive: true)
                    .overlay {
                        if busy { ProgressView().controlSize(.large).tint(.white) }
                    }
            }
        }
        .buttonStyle(.plain)
        .sensoryFeedback(.impact, trigger: on)
        .accessibilityLabel(on ? "Korumayı kapat" : "Korumayı aç")
    }
}

/// Trafik kartındaki küçük hız etiketi.
struct RateLabel: View {
    let symbol: String
    let value: String
    let color: Color

    var body: some View {
        HStack(spacing: 3) {
            Image(systemName: symbol).foregroundStyle(color)
            Text(value).monospacedDigit()
        }
        .font(.caption2.weight(.medium))
    }
}
