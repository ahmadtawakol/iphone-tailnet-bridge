import SwiftUI

@main
struct TailbridgeApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        Window("iPhone Tailnet Bridge", id: "main") {
            ContentView()
                .environmentObject(model)
                .task { model.start() }
        }
        .defaultSize(width: 820, height: 540)

        MenuBarExtra {
            MenuBarContent()
                .environmentObject(model)
                .task { model.start() }
        } label: {
            Image(systemName: menuSymbol)
        }
    }

    private var menuSymbol: String {
        if model.statuses.values.contains(where: { $0.state == "active" }) {
            return "iphone.radiowaves.left.and.right"
        }
        if model.statuses.values.contains(where: { $0.state == "error" || $0.state == "unreachable" || $0.state == "needs_wifi" }) {
            return "iphone.trianglebadge.exclamationmark"
        }
        return "iphone"
    }
}

private struct MenuBarContent: View {
    @Environment(\.openWindow) private var openWindow
    @EnvironmentObject private var model: AppModel

    var body: some View {
        if model.profiles.isEmpty {
            Text("No iPhone configured")
        } else {
            ForEach(model.profiles) { profile in
                let status = model.status(for: profile)
                Button {
                    Task { await model.setEnabled(!profile.enabled, profile: profile) }
                } label: {
                    Label("\(profile.name): \(status?.title ?? (profile.enabled ? "Starting" : "Off"))", systemImage: status?.symbol ?? "iphone")
                }
            }
        }
        Divider()
        Button("Open iPhone Tailnet Bridge…") { openWindow(id: "main") }
        Button("Quit") { NSApplication.shared.terminate(nil) }
    }
}
