import AppKit
import SwiftUI

@main
struct SlipstreamApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate

    var body: some Scene {
        MenuBarExtra {
            MenuView()
                .environmentObject(delegate.engine)
        } label: {
            MenuBarIcon()
                .environmentObject(delegate.engine)
        }
        .menuBarExtraStyle(.window)

        Window("Slipstream Ayarları", id: "settings") {
            SettingsView()
                .environmentObject(delegate.engine)
                .frame(minWidth: 620, minHeight: 440)
        }
        .windowResizability(.contentSize)
    }
}

struct MenuBarIcon: View {
    @EnvironmentObject private var engine: EngineController

    var body: some View {
        Image(systemName: engine.isRunning ? "shield.lefthalf.filled" : "shield.slash")
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    /// Motor uygulama boyunca tek kopyadır. Menü hiç açılmasa da kapanışta
    /// sistem proxy'sinin geri alınabilmesi için burada tutulur.
    let engine = EngineController()

    func applicationWillTerminate(_ notification: Notification) {
        // Motor kapanmadan çıkılırsa sistem proxy'si açık kalır ve internet kesilir.
        engine.shutdownSync()
    }
}
