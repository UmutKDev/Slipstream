import AppKit
import ServiceManagement
import SwiftUI

// MARK: - Pencere

enum SettingsPane: String, CaseIterable, Identifiable, Hashable {
    case general, protection, dns, domains, log, about
    var id: String { rawValue }

    var title: String {
        switch self {
        case .general: return "Genel"
        case .protection: return "Koruma"
        case .dns: return "Şifreli DNS"
        case .domains: return "Alan adları"
        case .log: return "Günlük"
        case .about: return "Hakkında"
        }
    }

    var subtitle: String {
        switch self {
        case .general: return "Açılış davranışı ve trafiğin nasıl yönlendirileceği"
        case .protection: return "Otomatik öğrenme ve reklam engelleyici"
        case .dns: return "ISS'nin sahte DNS yanıtlarını aşmak için kullanılan sağlayıcılar"
        case .domains: return "Atlatmanın uygulanacağı siteler"
        case .log: return "Motorun son olayları"
        case .about: return "Sürüm ve uygulama bilgisi"
        }
    }

    var symbol: String {
        switch self {
        case .general: return "gearshape.fill"
        case .protection: return "shield.lefthalf.filled"
        case .dns: return "lock.shield.fill"
        case .domains: return "globe"
        case .log: return "text.alignleft"
        case .about: return "info.circle.fill"
        }
    }

    var color: Color {
        switch self {
        case .general: return .gray
        case .protection: return Theme.ok
        case .dns: return Theme.accent
        case .domains: return .teal
        case .log: return .orange
        case .about: return .purple
        }
    }
}

struct SettingsView: View {
    @EnvironmentObject private var engine: EngineController
    @State private var pane: SettingsPane? = .general

    var body: some View {
        NavigationSplitView {
            List(SettingsPane.allCases, selection: $pane) { p in
                Label {
                    Text(p.title)
                } icon: {
                    IconTile(symbol: p.symbol, color: p.color, size: 22)
                }
                .padding(.vertical, 2)
                .tag(p)
            }
            .listStyle(.sidebar)
            .navigationSplitViewColumnWidth(min: 180, ideal: 190, max: 220)
            .safeAreaInset(edge: .bottom) { EngineBadge().padding(10) }
        } detail: {
            let current = pane ?? .general
            Group {
                if current == .log {
                    PaneContainer(pane: current, scrolls: false) { LogView() }
                } else {
                    PaneContainer(pane: current) {
                        switch current {
                        case .general: GeneralSettings()
                        case .protection: ProtectionSettings()
                        case .dns: DNSSettings()
                        case .domains: DomainSettings()
                        case .about: AboutView()
                        case .log: EmptyView()
                        }
                    }
                }
            }
            .id(current)
        }
        .frame(minWidth: 720, minHeight: 520)
    }
}

/// Sağ taraftaki sayfa çerçevesi: başlık, gerekirse uyarı, içerik.
private struct PaneContainer<Content: View>: View {
    @EnvironmentObject private var engine: EngineController
    let pane: SettingsPane
    var scrolls = true
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 12) {
                IconTile(symbol: pane.symbol, color: pane.color, size: 36)
                VStack(alignment: .leading, spacing: 2) {
                    Text(pane.title).font(.title2.weight(.semibold))
                    Text(pane.subtitle).font(.callout).foregroundStyle(.secondary)
                }
            }
            .padding(.horizontal, 24)
            .padding(.top, 20)
            .padding(.bottom, 14)

            if !engine.isRunning && pane != .about && pane != .log {
                OffBanner().padding(.horizontal, 24).padding(.bottom, 10)
            }

            if scrolls {
                ScrollView {
                    GlassGroup(spacing: 2) {
                        VStack(alignment: .leading, spacing: 18) { content }
                    }
                        .padding(.horizontal, 24)
                        .padding(.bottom, 24)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                content
                    .padding(.horizontal, 24)
                    .padding(.bottom, 24)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(AuroraBackground(active: engine.isRunning, intensity: 0.6).ignoresSafeArea())
    }
}

private struct OffBanner: View {
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "power").foregroundStyle(Theme.accent)
            Text("Koruma kapalı. Ayarların çoğu koruma açıkken değiştirilebilir.")
                .font(.callout)
            Spacer()
            Button("Korumayı aç", action: engine.start)
                .glassButton(prominent: true)
                .tint(Theme.accent)
                .controlSize(.small)
                .disabled(engine.state == .starting)
        }
        .padding(12)
        .glassSurface(cornerRadius: 14, tint: Theme.accent.opacity(0.35))
    }
}

/// Kenar çubuğunun altındaki küçük motor durumu.
private struct EngineBadge: View {
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(engine.isRunning ? Theme.ok : Color.secondary)
                .frame(width: 8, height: 8)
            VStack(alignment: .leading, spacing: 1) {
                Text(engine.isRunning ? "Koruma açık" : "Koruma kapalı").font(.caption.weight(.medium))
                if engine.isRunning, let s = engine.status {
                    Text(Theme.duration(s.stats.uptimeSec)).font(.caption2).foregroundStyle(.secondary)
                }
            }
            Spacer()
        }
        .padding(10)
        .glassSurface(cornerRadius: 12, tint: engine.isRunning ? Theme.ok.opacity(0.3) : nil)
    }
}

// MARK: - Ortak parçalar

/// Başlıklı ayar kartı.
struct SettingsCard<Content: View>: View {
    var title: String?
    var footer: String?
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let title {
                Text(title.uppercased(with: Locale(identifier: "tr_TR")))
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .padding(.leading, 4)
            }
            VStack(spacing: 0) { content }
                .glassSurface(cornerRadius: 18)
            if let footer {
                Text(footer)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, 4)
            }
        }
    }
}

/// Simge, başlık, açıklama ve sağda bir denetimden oluşan satır.
struct SettingRow<Trailing: View>: View {
    var symbol: String?
    var color: Color = .gray
    let title: String
    var detail: String?
    var divider = true
    @ViewBuilder let trailing: Trailing

    var body: some View {
        VStack(spacing: 0) {
            HStack(alignment: .center, spacing: 12) {
                if let symbol { IconTile(symbol: symbol, color: color) }
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                    if let detail {
                        Text(detail)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                Spacer(minLength: 12)
                trailing
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            if divider {
                Divider().padding(.leading, symbol == nil ? 14 : 52)
            }
        }
    }
}

struct SwitchView: View {
    @Binding var isOn: Bool
    var body: some View {
        Toggle("", isOn: $isOn).labelsHidden().toggleStyle(.switch)
    }
}

private func lines(_ text: String) -> [String] {
    text.split(whereSeparator: \.isNewline)
        .map { $0.trimmingCharacters(in: .whitespaces) }
        .filter { !$0.isEmpty }
}

// MARK: - Genel

struct GeneralSettings: View {
    @EnvironmentObject private var engine: EngineController
    @AppStorage("systemProxy") private var systemProxy = true
    @AppStorage("startOnLaunch") private var startOnLaunch = true
    @AppStorage("proxyPort") private var proxyPort = 8881
    @State private var openAtLogin = SMAppService.mainApp.status == .enabled

    private var mode: String { engine.status?.proxyMode ?? "global" }

    var body: some View {
        SettingsCard(title: "Açılış") {
            SettingRow(symbol: "person.crop.circle.fill", color: .blue,
                       title: "Oturum açılınca başlat",
                       detail: "Mac açıldığında Slipstream menü çubuğunda hazır olur.") {
                SwitchView(isOn: $openAtLogin)
            }
            SettingRow(symbol: "bolt.fill", color: Theme.ok,
                       title: "Açılınca korumayı başlat",
                       detail: "Uygulama açılır açılmaz engeller aşılmaya başlar.",
                       divider: false) {
                SwitchView(isOn: $startOnLaunch)
            }
        }
        .onChange(of: openAtLogin) { on in
            do {
                if on { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
            } catch {
                engine.lastError = "Oturum açılış ayarı değiştirilemedi: \(error.localizedDescription)"
                openAtLogin = SMAppService.mainApp.status == .enabled
            }
        }

        SettingsCard(title: "Sistem proxy'si",
                     footer: systemProxy ? nil : "Kapalıyken tarayıcına ya da uygulamaya elle 127.0.0.1:\(String(proxyPort)) adresini HTTP veya SOCKS5 proxy'si olarak girebilirsin.") {
            SettingRow(symbol: "network", color: Theme.accent,
                       title: "Sistem proxy'sini otomatik ayarla",
                       detail: "Safari, Chrome ve sistem ayarına uyan uygulamalar Slipstream'den geçer.",
                       divider: false) {
                SwitchView(isOn: Binding(get: { systemProxy }, set: { engine.setSystemProxy($0) }))
            }
        }

        VStack(alignment: .leading, spacing: 6) {
            Text("YÖNLENDİRME")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .padding(.leading, 4)
            HStack(spacing: 12) {
                ModeCard(symbol: "globe.europe.africa.fill",
                         title: "Tüm trafik",
                         detail: "Her site Slipstream'den geçer. En geniş koruma, reklam engelleyici her yerde çalışır.",
                         selected: mode == "global") { engine.setProxyMode("global") }
                ModeCard(symbol: "line.3.horizontal.decrease.circle.fill",
                         title: "Yalnız engelli siteler",
                         detail: "Listedeki ve öğrenilen siteler geçer, gerisi doğrudan gider. Daha hızlıdır; Slipstream kapanırsa internet kesilmez.",
                         selected: mode == "pac") { engine.setProxyMode("pac") }
            }
            .fixedSize(horizontal: false, vertical: true)
            .disabled(!engine.isRunning || !systemProxy)
            .opacity(engine.isRunning && systemProxy ? 1 : 0.55)
        }

        SettingsCard(title: "Gelişmiş") {
            SettingRow(symbol: "number", color: .gray,
                       title: "Proxy portu",
                       detail: engine.isRunning ? "Değiştirmek için önce korumayı kapat." : "Varsayılan 8881.",
                       divider: false) {
                HStack(spacing: 6) {
                    TextField("", value: $proxyPort, format: .number.grouping(.never))
                        .textFieldStyle(.roundedBorder)
                        .multilineTextAlignment(.trailing)
                        .font(.body.monospacedDigit())
                        .frame(width: 70)
                    Stepper("", value: $proxyPort, in: 1025...65000).labelsHidden()
                }
                .disabled(engine.isRunning)
            }
        }
    }
}

private struct ModeCard: View {
    let symbol: String
    let title: String
    let detail: String
    let selected: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Image(systemName: symbol)
                        .font(.title2)
                        .foregroundStyle(selected ? Theme.accent : .secondary)
                    Spacer()
                    Image(systemName: selected ? "checkmark.circle.fill" : "circle")
                        .font(.title3)
                        .foregroundStyle(selected ? Theme.accent : Color.secondary.opacity(0.5))
                }
                Text(title).font(.headline)
                Text(detail)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            .padding(14)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .glassSurface(cornerRadius: 18, tint: selected ? Theme.accent.opacity(0.35) : nil, interactive: true)
            .overlay(
                RoundedRectangle(cornerRadius: 18, style: .continuous)
                    .strokeBorder(Theme.accent.opacity(selected ? 0.9 : 0), lineWidth: 1.5)
            )
            .contentShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
            .animation(.easeInOut(duration: 0.2), value: selected)
        }
        .buttonStyle(.plain)
    }
}

// MARK: - Koruma

struct ProtectionSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var allowText = ""
    @State private var loaded = false

    private var status: EngineStatus? { engine.status }

    var body: some View {
        SettingsCard(title: "Site başına otomatik öğrenme",
                     footer: "Bir site açılmazsa Slipstream bağlantıyı fark ettirmeden başka bir yöntemle yeniden kurar ve çalışan yöntemi o site için hatırlar.") {
            SettingRow(symbol: "brain.head.profile", color: .pink,
                       title: "Otomatik öğrenme",
                       detail: "Çalışmayan sitede sıradaki yöntemi dene ve kaydet.") {
                SwitchView(isOn: Binding(get: { status?.autoLearn ?? true }, set: { engine.setAutoLearn($0) }))
                    .disabled(!engine.isRunning)
            }
            SettingRow(symbol: "checkmark.seal.fill", color: Theme.ok,
                       title: "Öğrenilen siteler",
                       detail: "\(status?.learned ?? 0) site için özel yöntem kayıtlı.",
                       divider: false) {
                Button("Sıfırla", action: engine.clearLearned)
                    .disabled(!engine.isRunning || (status?.learned ?? 0) == 0)
            }
        }

        SettingsCard(title: "Reklam ve izleyici engelleyici",
                     footer: "AdGuard DNS filtresi ve StevenBlack listeleri kullanılır, günde bir kez güncellenir. Proxy kullanan başka bir reklam engelleyiciyle çakışabilir.") {
            SettingRow(symbol: "hand.raised.fill", color: .red,
                       title: "Reklamları ve izleyicileri engelle",
                       detail: adDetail) {
                SwitchView(isOn: Binding(get: { status?.adBlock?.enabled ?? false }, set: { engine.setAdBlock($0) }))
                    .disabled(!engine.isRunning)
            }
            SettingRow(symbol: "arrow.triangle.2.circlepath", color: .gray,
                       title: "Engel listeleri",
                       detail: "Listeleri beklemeden yeniden indir.",
                       divider: false) {
                Button("Şimdi güncelle", action: engine.updateAdLists)
                    .disabled(!engine.isRunning)
            }
        }

        SettingsCard(title: "Engellenmeyecek alan adları",
                     footer: "Her satıra bir alan adı yaz. Bir site reklam engelleyici yüzünden bozulursa buraya ekle.") {
            VStack(alignment: .trailing, spacing: 8) {
                DomainEditor(text: $allowText, placeholder: "ornek.com", minHeight: 90)
                Button("Kaydet") {
                    engine.setAdBlock(status?.adBlock?.enabled ?? true, allow: lines(allowText))
                }
                .disabled(!engine.isRunning)
            }
            .padding(12)
        }
        .onAppear {
            guard !loaded else { return }
            allowText = (status?.adBlock?.allow ?? []).joined(separator: "\n")
            loaded = true
        }
    }

    private var adDetail: String {
        guard let info = status?.adBlock, engine.isRunning else { return "Koruma açıkken çalışır." }
        if !info.hasLists { return "Listeler indiriliyor…" }
        return "\(info.domains.formatted()) alan adı listede, \(info.blocked.formatted()) istek engellendi."
    }
}

/// Satır satır alan adı düzenleyici.
private struct DomainEditor: View {
    @Binding var text: String
    var placeholder: String
    var minHeight: CGFloat = 160

    var body: some View {
        ZStack(alignment: .topLeading) {
            TextEditor(text: $text)
                .font(.system(.body, design: .monospaced))
                .scrollContentBackground(.hidden)
                .padding(6)
            if text.isEmpty {
                Text(placeholder)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(.tertiary)
                    .padding(.horizontal, 11)
                    .padding(.vertical, 6)
                    .allowsHitTesting(false)
            }
        }
        .frame(minHeight: minHeight)
        .background(.black.opacity(0.12), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(.white.opacity(0.10)))
    }
}

// MARK: - DNS

struct DNSSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var selected: [String] = []

    private var changed: Bool { selected != (engine.status?.dns ?? []) }

    var body: some View {
        SettingsCard(title: "Sağlayıcılar",
                     footer: "ISS'ler engelli siteler için sahte DNS yanıtı döndürür. Slipstream adları şifreli DNS (DoH) ile çözer; ilk sağlayıcı yanıt vermezse sıradakine geçer.") {
            ForEach(Array(DNSProviderInfo.all.enumerated()), id: \.element.id) { i, p in
                SettingRow(symbol: icon(p.id), color: color(p.id),
                           title: p.name,
                           detail: order(p.id),
                           divider: i < DNSProviderInfo.all.count - 1) {
                    SwitchView(isOn: Binding(
                        get: { selected.contains(p.id) },
                        set: { on in
                            if on { selected.append(p.id) } else { selected.removeAll { $0 == p.id } }
                        }
                    ))
                }
            }
        }

        HStack {
            Button("DNS önbelleğini temizle", action: engine.flushDNS)
                .glassButton()
                .disabled(!engine.isRunning)
            Spacer()
            Button("Uygula") { engine.setDNS(selected) }
                .glassButton(prominent: true)
                .tint(Theme.accent)
                .disabled(selected.isEmpty || !engine.isRunning || !changed)
                .keyboardShortcut(.defaultAction)
        }
        .onAppear { selected = engine.status?.dns ?? ["cloudflare", "google"] }
    }

    private func order(_ id: String) -> String? {
        guard let i = selected.firstIndex(of: id) else { return nil }
        return i == 0 ? "Birincil" : "\(i + 1). sırada yedek"
    }

    private func icon(_ id: String) -> String {
        switch id {
        case "cloudflare": return "cloud.fill"
        case "google": return "magnifyingglass"
        case "quad9": return "9.circle.fill"
        case "adguard": return "hand.raised.fill"
        default: return "desktopcomputer"
        }
    }

    private func color(_ id: String) -> Color {
        switch id {
        case "cloudflare": return .orange
        case "google": return .blue
        case "quad9": return .indigo
        case "adguard": return Theme.ok
        default: return .gray
        }
    }
}

// MARK: - Alan adları

struct DomainSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var mode = "all"
    @State private var text = ""
    @State private var loaded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("ATLATMA UYGULANACAK BAĞLANTILAR")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .padding(.leading, 4)
            HStack(spacing: 12) {
                ModeCard(symbol: "square.stack.3d.up.fill",
                         title: "Tüm bağlantılar",
                         detail: "Slipstream'den geçen her bağlantıya atlatma uygulanır.",
                         selected: mode == "all") { mode = "all" }
                ModeCard(symbol: "list.bullet.rectangle.fill",
                         title: "Yalnız liste",
                         detail: "Yalnız aşağıdaki alan adlarına uygulanır, diğerleri dokunulmadan geçer.",
                         selected: mode == "list") { mode = "list" }
            }
            .fixedSize(horizontal: false, vertical: true)
        }

        SettingsCard(title: "Liste",
                     footer: "Her satıra bir alan adı yaz. \"discord.com\" alt alan adlarını da kapsar. Liste, \"Yalnız engelli siteler\" yönlendirmesinde hangi sitelerin Slipstream'den geçeceğini de belirler.") {
            VStack(alignment: .trailing, spacing: 8) {
                DomainEditor(text: $text, placeholder: "discord.com\nroblox.com", minHeight: 200)
                HStack {
                    Text("\(lines(text).count) alan adı").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button("Kaydet") { engine.saveDomains(mode: mode, domains: lines(text)) }
                        .glassButton(prominent: true)
                        .tint(Theme.accent)
                        .disabled(!engine.isRunning)
                        .keyboardShortcut(.defaultAction)
                }
            }
            .padding(12)
        }
        .task {
            guard !loaded, let d = await engine.loadDomains() else { return }
            mode = d.mode
            text = d.domains.joined(separator: "\n")
            loaded = true
        }
    }
}

// MARK: - Günlük

struct LogView: View {
    @EnvironmentObject private var engine: EngineController
    @State private var warningsOnly = false

    private var entries: [LogEntry] {
        warningsOnly ? engine.logs.filter { $0.level != "info" } : engine.logs
    }

    var body: some View {
        VStack(spacing: 10) {
            HStack {
                Picker("", selection: $warningsOnly) {
                    Text("Tümü").tag(false)
                    Text("Uyarılar").tag(true)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .frame(width: 180)
                Spacer()
                Button {
                    let text = entries.map { "\($0.clock)  \($0.msg)" }.joined(separator: "\n")
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(text, forType: .string)
                } label: {
                    Label("Kopyala", systemImage: "doc.on.doc")
                }
                .glassButton()
                .disabled(entries.isEmpty)
            }

            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 3) {
                        ForEach(entries) { e in
                            HStack(alignment: .firstTextBaseline, spacing: 10) {
                                Text(e.clock).foregroundStyle(.tertiary)
                                Circle()
                                    .fill(e.level == "info" ? Color.secondary.opacity(0.5) : Theme.warn)
                                    .frame(width: 6, height: 6)
                                Text(e.msg)
                                    .foregroundStyle(e.level == "info" ? Color.primary : Theme.warn)
                                    .textSelection(.enabled)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                            .font(.system(.caption, design: .monospaced))
                            .id(e.id)
                        }
                    }
                    .padding(12)
                }
                .overlay {
                    if entries.isEmpty {
                        Text(engine.isRunning ? "Henüz kayıt yok" : "Koruma açılınca olaylar burada görünür")
                            .foregroundStyle(.secondary)
                    }
                }
                .glassSurface(cornerRadius: 18)
                .onChange(of: engine.logs.count) { _ in
                    if let last = entries.last { proxy.scrollTo(last.id, anchor: .bottom) }
                }
            }
        }
    }
}

// MARK: - Hakkında

struct AboutView: View {
    @EnvironmentObject private var engine: EngineController

    private var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "–"
    }

    var body: some View {
        VStack(spacing: 14) {
            IconTile(symbol: "bolt.shield.fill", color: Theme.accent, size: 72)
                .shadow(color: Theme.accent.opacity(0.4), radius: 14)
            Text("Slipstream").font(.largeTitle.bold())
            Text("Uygulama \(appVersion) · Motor \(engine.status?.version ?? "–")")
                .font(.callout)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 12)

        SettingsCard {
            SettingRow(symbol: "eye.slash.fill", color: .gray,
                       title: "Gizlilik",
                       detail: "Slipstream trafiğini şifrelemez ve IP adresini gizlemez. Yalnız ISS'nin DPI cihazının engelli siteleri tanımasını zorlaştırır. Bir VPN değildir.") { EmptyView() }
            SettingRow(symbol: "chevron.left.forwardslash.chevron.right", color: .black,
                       title: "Kaynak kodu",
                       detail: "github.com/UmutKDev/Slipstream",
                       divider: false) {
                Link("Aç", destination: URL(string: "https://github.com/UmutKDev/Slipstream")!)
            }
        }
    }
}
