import SwiftUI

@main
struct SlipstreamApp: App {
    @StateObject private var tunnel = TunnelController()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(tunnel)
                .tint(Theme.accent)
        }
        .onChange(of: scenePhase) { _, phase in
            // Arka planda motor yoklanmaz; pil harcanmasın.
            tunnel.setForeground(phase == .active)
            if phase == .active { Task { await tunnel.refresh() } }
        }
    }
}
