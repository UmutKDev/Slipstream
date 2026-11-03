import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var tunnel: TunnelController
    @Environment(\.dismiss) private var dismiss

    private var isOn: Bool { tunnel.isConnected }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Toggle("Her zaman açık", isOn: Binding(
                        get: { tunnel.alwaysOn },
                        set: { tunnel.setAlwaysOn($0) }
                    ))
                } footer: {
                    Text("Açıkken iOS tüneli ağ değişince ve telefon yeniden başlayınca kendisi açar.")
                }

                if !isOn {
                    Section {
                        Label("Aşağıdaki ayarlar koruma açıkken değiştirilebilir.", systemImage: "info.circle")
                            .foregroundStyle(.secondary)
                    }
                }

                dnsSection
                adBlockSection
                sitesSection

                Section("Tanılama") {
                    NavigationLink("Günlük") { LogsView().environmentObject(tunnel) }
                }

                Section {
                    LabeledContent("Motor sürümü", value: tunnel.status?.version ?? "–")
                } footer: {
                    Text("Slipstream cihazında yerel bir VPN tüneli kurar; trafiğin dışarıda bir sunucuya gitmez. Web trafiği tünelin proxy ayarıyla motordan geçer, DNS sorguları şifreli DNS ile çözülür. Proxy ayarını yok sayan bazı uygulamalar (ör. QUIC kullanan oyunlar) motoru atlayabilir.")
                }
            }
            .navigationTitle("Ayarlar")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Bitti") { dismiss() }
                }
            }
        }
    }

    // MARK: - DNS

    private var dnsSection: some View {
        Section {
            Picker("Şifreli DNS", selection: Binding(
                get: { tunnel.status?.dns.first ?? "cloudflare" },
                set: { id in
                    // İlk sağlayıcı yanıt vermezse ikincisine geçilir.
                    let fallback = id == "cloudflare" ? "google" : "cloudflare"
                    tunnel.setDNS(id == "system" ? ["system"] : [id, fallback])
                }
            )) {
                ForEach(DNSProviderInfo.all) { p in Text(p.name).tag(p.id) }
            }
            .disabled(!isOn)
        } header: {
            Text("DNS")
        } footer: {
            Text("ISS'nin DNS'i engelli sitelere sahte adres döndürür. Şifreli DNS bunu aşar.")
        }
    }

    // MARK: - Reklam engelleyici

    private var adBlockSection: some View {
        let info = tunnel.status?.adBlock
        let selected = Set(info?.lists ?? [])
        return Section {
            ForEach(info?.catalog ?? []) { list in
                Toggle(isOn: Binding(
                    get: { selected.contains(list.id) },
                    set: { on in
                        var ids = selected
                        if on { ids.insert(list.id) } else { ids.remove(list.id) }
                        let ordered = (info?.catalog ?? []).map(\.id).filter(ids.contains)
                        tunnel.setAdLists(ordered)
                    }
                )) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(list.name)
                        Text(list.description).font(.caption).foregroundStyle(.secondary)
                    }
                }
            }
            Button {
                tunnel.updateAdLists()
            } label: {
                if info?.updating == true {
                    Label("Güncelleniyor…", systemImage: "arrow.triangle.2.circlepath")
                } else {
                    Label("Listeleri şimdi güncelle", systemImage: "arrow.down.circle")
                }
            }
            .disabled(info?.updating == true)
        } header: {
            Text("Reklam listeleri")
        } footer: {
            if let info, info.hasLists {
                Text("\(info.domains.formatted()) alan adı engelleniyor. Listeler günde bir kez kendiliğinden güncellenir.")
            } else {
                Text("Listeler koruma açılınca indirilir.")
            }
        }
        .disabled(!isOn)
    }

    // MARK: - Siteler

    private var sitesSection: some View {
        Section {
            NavigationLink("Engelli site listesi") { DomainsView().environmentObject(tunnel) }
            Toggle("Çalışmayan sitede başka ayar dene", isOn: Binding(
                get: { tunnel.status?.autoLearn ?? true },
                set: { tunnel.setAutoLearn($0) }
            ))
            HStack {
                Text("Öğrenilen siteler")
                Spacer()
                Text("\(tunnel.status?.learned ?? 0)").foregroundStyle(.secondary)
            }
            Button("Öğrenilenleri unut", role: .destructive) { tunnel.clearLearned() }
                .disabled((tunnel.status?.learned ?? 0) == 0)
        } header: {
            Text("Siteler")
        } footer: {
            Text("Bir site açılmazsa Slipstream sıradaki yöntemi dener ve o site için hatırlar.")
        }
        .disabled(!isOn)
    }
}

/// Engelli site listesini düzenler.
struct DomainsView: View {
    @EnvironmentObject private var tunnel: TunnelController
    @State private var text = ""
    @State private var allSites = true
    @State private var loaded = false

    var body: some View {
        Form {
            Section {
                Toggle("Bütün sitelere uygula", isOn: $allSites)
            } footer: {
                Text(allSites
                     ? "Engel aşma her siteye uygulanır. Listedeki siteler \"Yalnız listedekiler\" kipinde kullanılır."
                     : "Engel aşma yalnız aşağıdaki sitelere uygulanır.")
            }
            Section("Siteler (her satıra bir alan adı)") {
                TextEditor(text: $text)
                    .font(.system(.body, design: .monospaced))
                    .frame(minHeight: 260)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            }
        }
        .navigationTitle("Engelli siteler")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button("Kaydet") {
                    let list = text.split(whereSeparator: \.isNewline)
                        .map { $0.trimmingCharacters(in: .whitespaces) }
                        .filter { !$0.isEmpty }
                    tunnel.saveDomains(mode: allSites ? "all" : "list", domains: list)
                }
                .disabled(!loaded)
            }
        }
        .task {
            if let d = await tunnel.loadDomains() {
                text = d.domains.joined(separator: "\n")
                allSites = d.mode == "all"
                loaded = true
            }
        }
    }
}

/// Motorun son günlük kayıtları.
struct LogsView: View {
    @EnvironmentObject private var tunnel: TunnelController

    var body: some View {
        List(Array(tunnel.logs.reversed())) { e in
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(e.clock).monospacedDigit()
                    Text(e.level.uppercased())
                        .foregroundStyle(e.level == "error" ? Color.red : e.level == "warn" ? Color.orange : Color.secondary)
                }
                .font(.caption2)
                Text(e.msg).font(.caption)
            }
        }
        .overlay {
            if tunnel.logs.isEmpty {
                Text(tunnel.isConnected ? "Henüz kayıt yok" : "Koruma kapalı")
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Günlük")
    }
}
