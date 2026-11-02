import ServiceManagement
import SwiftUI

struct SettingsView: View {
    var body: some View {
        TabView {
            GeneralSettings().tabItem { Label("Genel", systemImage: "gearshape") }
            ProtectionSettings().tabItem { Label("Koruma", systemImage: "shield.lefthalf.filled") }
            DNSSettings().tabItem { Label("DNS", systemImage: "network") }
            DomainSettings().tabItem { Label("Alan adları", systemImage: "list.bullet") }
            LogView().tabItem { Label("Günlük", systemImage: "text.alignleft") }
            AboutView().tabItem { Label("Hakkında", systemImage: "info.circle") }
        }
        .padding()
    }
}

struct GeneralSettings: View {
    @EnvironmentObject private var engine: EngineController
    @AppStorage("systemProxy") private var systemProxy = true
    @AppStorage("startOnLaunch") private var startOnLaunch = true
    @AppStorage("proxyPort") private var proxyPort = 8881
    @State private var openAtLogin = SMAppService.mainApp.status == .enabled

    var body: some View {
        Form {
            Section {
                Toggle("Oturum açılınca Slipstream'i başlat", isOn: $openAtLogin)
                    .onChange(of: openAtLogin) { on in
                        do {
                            if on { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
                        } catch {
                            engine.lastError = "Oturum açılış ayarı değiştirilemedi: \(error.localizedDescription)"
                            openAtLogin = SMAppService.mainApp.status == .enabled
                        }
                    }
                Toggle("Uygulama açılınca korumayı başlat", isOn: $startOnLaunch)
            }
            Section {
                Toggle("Sistem proxy'sini otomatik ayarla", isOn: Binding(
                    get: { systemProxy },
                    set: { engine.setSystemProxy($0) }
                ))
                Text("Açıkken Safari, Chrome ve sistem proxy'sine uyan tüm uygulamalar Slipstream'den geçer. Kapalıyken uygulamalara elle 127.0.0.1:\(String(proxyPort)) adresini HTTP ya da SOCKS5 proxy'si olarak girebilirsin.")
                    .font(.caption).foregroundStyle(.secondary)
                Stepper("Proxy portu: \(String(proxyPort))", value: $proxyPort, in: 1025...65000)
                    .disabled(engine.isRunning)
            }
        }
        .formStyle(.grouped)
    }
}

struct ProtectionSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var allowText = ""
    @State private var loaded = false

    private var status: EngineStatus? { engine.status }

    var body: some View {
        Form {
            Section("Site başına otomatik öğrenme") {
                Toggle("Çalışmayan sitede sıradaki yöntemi dene ve hatırla", isOn: Binding(
                    get: { status?.autoLearn ?? true },
                    set: { engine.setAutoLearn($0) }
                ))
                .disabled(!engine.isRunning)
                Text("Bir site açılmazsa Slipstream bağlantıyı fark ettirmeden başka bir yöntemle yeniden kurar. Çalışan yöntemi o site için kaydeder, bir dahaki sefere doğrudan onu kullanır.")
                    .font(.caption).foregroundStyle(.secondary)
                HStack {
                    Text("Öğrenilen site sayısı: \(status?.learned ?? 0)")
                    Spacer()
                    Button("Sıfırla", action: engine.clearLearned)
                        .disabled(!engine.isRunning || (status?.learned ?? 0) == 0)
                }
            }
            Section("Reklam ve izleyici engelleyici") {
                Toggle("Reklamları ve izleyicileri engelle", isOn: Binding(
                    get: { status?.adBlock?.enabled ?? false },
                    set: { engine.setAdBlock($0) }
                ))
                .disabled(!engine.isRunning)
                if let info = status?.adBlock {
                    LabeledContent("Listedeki alan adı", value: info.domains.formatted())
                    LabeledContent("Engellenen istek", value: info.blocked.formatted())
                }
                Text("AdGuard DNS filtresi ve StevenBlack listeleri kullanılır, günde bir kez güncellenir. Mac'te ayrıca proxy kullanan bir reklam engelleyici (ör. AdGuard uygulaması) varsa ikisi çakışabilir.")
                    .font(.caption).foregroundStyle(.secondary)
                Text("Engellenmeyecek alan adları (her satıra bir tane)").font(.caption)
                TextEditor(text: $allowText)
                    .font(.system(.body, design: .monospaced))
                    .frame(minHeight: 70)
                HStack {
                    Button("Listeleri şimdi güncelle", action: engine.updateAdLists)
                    Spacer()
                    Button("İstisnaları kaydet") {
                        let list = allowText.split(whereSeparator: \.isNewline)
                            .map { $0.trimmingCharacters(in: .whitespaces) }
                            .filter { !$0.isEmpty }
                        engine.setAdBlock(status?.adBlock?.enabled ?? true, allow: list)
                    }
                }
                .disabled(!engine.isRunning)
            }
        }
        .formStyle(.grouped)
        .onAppear {
            guard !loaded else { return }
            allowText = (status?.adBlock?.allow ?? []).joined(separator: "\n")
            loaded = true
        }
    }
}

struct DNSSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var selected: [String] = []

    var body: some View {
        Form {
            Section {
                Text("ISS'ler engelli siteler için sahte DNS yanıtı döndürür. Slipstream adları şifreli DNS (DoH) ile çözer. İlk sağlayıcı yanıt vermezse sıradakine geçer.")
                    .font(.caption).foregroundStyle(.secondary)
                ForEach(DNSProviderInfo.all) { p in
                    Toggle(p.name, isOn: Binding(
                        get: { selected.contains(p.id) },
                        set: { on in
                            if on { selected.append(p.id) } else { selected.removeAll { $0 == p.id } }
                        }
                    ))
                }
            }
            HStack {
                Button("DNS önbelleğini temizle", action: engine.flushDNS)
                Spacer()
                Button("Uygula") { engine.setDNS(selected) }
                    .disabled(selected.isEmpty || !engine.isRunning)
                    .keyboardShortcut(.defaultAction)
            }
        }
        .formStyle(.grouped)
        .onAppear { selected = engine.status?.dns ?? ["cloudflare", "google"] }
    }
}

struct DomainSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var mode = "all"
    @State private var text = ""
    @State private var loaded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Picker("Atlatma uygulanacak bağlantılar", selection: $mode) {
                Text("Tüm bağlantılar").tag("all")
                Text("Yalnız listedeki alan adları").tag("list")
            }
            .pickerStyle(.radioGroup)
            Text("Her satıra bir alan adı yaz. \"discord.com\" yazınca alt alan adları da kapsanır.")
                .font(.caption).foregroundStyle(.secondary)
            TextEditor(text: $text)
                .font(.system(.body, design: .monospaced))
                .border(.quaternary)
                .disabled(mode == "all")
                .opacity(mode == "all" ? 0.5 : 1)
            HStack {
                Spacer()
                Button("Kaydet") {
                    let list = text.split(whereSeparator: \.isNewline)
                        .map { $0.trimmingCharacters(in: .whitespaces) }
                        .filter { !$0.isEmpty }
                    engine.saveDomains(mode: mode, domains: list)
                }
                .disabled(!engine.isRunning)
                .keyboardShortcut(.defaultAction)
            }
        }
        .task {
            guard !loaded, let d = await engine.loadDomains() else { return }
            mode = d.mode
            text = d.domains.joined(separator: "\n")
            loaded = true
        }
    }
}

struct LogView: View {
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        ScrollViewReader { proxy in
            List(engine.logs) { e in
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text(e.clock).foregroundStyle(.secondary)
                    Text(e.msg)
                        .foregroundStyle(e.level == "info" ? Color.primary : Theme.warn)
                        .textSelection(.enabled)
                }
                .font(.system(.caption, design: .monospaced))
                .id(e.id)
            }
            .onChange(of: engine.logs.count) { _ in
                if let last = engine.logs.last { proxy.scrollTo(last.id, anchor: .bottom) }
            }
        }
    }
}

struct AboutView: View {
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: "shield.lefthalf.filled").font(.system(size: 48)).foregroundStyle(Theme.accent)
            Text("Slipstream").font(.title.bold())
            Text("Motor sürümü \(engine.status?.version ?? "–")").foregroundStyle(.secondary)
            Text("Slipstream trafiğini şifrelemez ve IP adresini gizlemez. Yalnız ISS'nin DPI cihazının engelli siteleri tanımasını zorlaştırır.")
                .multilineTextAlignment(.center)
                .font(.callout)
                .frame(maxWidth: 420)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
