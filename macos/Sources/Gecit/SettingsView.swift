import ServiceManagement
import SwiftUI

struct SettingsView: View {
    var body: some View {
        TabView {
            GeneralSettings().tabItem { Label("Genel", systemImage: "gearshape") }
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
                Toggle("Oturum açılınca Geçit'i başlat", isOn: $openAtLogin)
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
                Text("Açıkken Safari, Chrome ve sistem proxy'sine uyan tüm uygulamalar Geçit'ten geçer. Kapalıyken uygulamalara elle 127.0.0.1:\(String(proxyPort)) adresini HTTP ya da SOCKS5 proxy'si olarak girebilirsin.")
                    .font(.caption).foregroundStyle(.secondary)
                Stepper("Proxy portu: \(String(proxyPort))", value: $proxyPort, in: 1025...65000)
                    .disabled(engine.isRunning)
            }
        }
        .formStyle(.grouped)
    }
}

struct DNSSettings: View {
    @EnvironmentObject private var engine: EngineController
    @State private var selected: [String] = []

    var body: some View {
        Form {
            Section {
                Text("ISS'ler engelli siteler için sahte DNS yanıtı döndürür. Geçit adları şifreli DNS (DoH) ile çözer. İlk sağlayıcı yanıt vermezse sıradakine geçer.")
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
            Text("Geçit").font(.title.bold())
            Text("Motor sürümü \(engine.status?.version ?? "–")").foregroundStyle(.secondary)
            Text("Geçit trafiğini şifrelemez ve IP adresini gizlemez. Yalnız ISS'nin DPI cihazının engelli siteleri tanımasını zorlaştırır.")
                .multilineTextAlignment(.center)
                .font(.callout)
                .frame(maxWidth: 420)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
