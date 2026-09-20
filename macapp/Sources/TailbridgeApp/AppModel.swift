import Foundation
import SwiftUI

@MainActor
final class AppModel: ObservableObject {
    @Published var profiles: [DeviceProfile] = []
    @Published var statuses: [String: BridgeStatus] = [:]
    @Published var selectedProfileID: String?
    @Published var isBusy = false
    @Published var errorMessage: String?
    @Published var showingAddDevice = false

    let cli: CLIClient?
    private let rootURL: URL
    private var refreshTask: Task<Void, Never>?

    init() {
        cli = try? CLIClient()
        rootURL = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("iPhone Tailnet Bridge", isDirectory: true)
    }

    deinit {
        refreshTask?.cancel()
    }

    func start() {
        guard refreshTask == nil else { return }
        reload()
        refreshTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(2))
                self?.reload()
            }
        }
    }

    func reload() {
        let profilesURL = rootURL.appendingPathComponent("Profiles", isDirectory: true)
        let decoder = JSONDecoder()
        let files = (try? FileManager.default.contentsOfDirectory(at: profilesURL, includingPropertiesForKeys: nil)) ?? []
        profiles = files.filter { $0.pathExtension == "json" }.compactMap { url in
            guard let data = try? Data(contentsOf: url), var profile = try? decoder.decode(DeviceProfile.self, from: data) else { return nil }
            profile.fileURL = url
            return profile
        }.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }

        let statusURL = rootURL.appendingPathComponent("status.json")
        if let data = try? Data(contentsOf: statusURL), let document = try? decoder.decode(BridgeStatusDocument.self, from: data) {
            statuses = Dictionary(uniqueKeysWithValues: document.devices.map { ($0.id, $0) })
        }
        if selectedProfileID == nil || !profiles.contains(where: { $0.id == selectedProfileID }) {
            selectedProfileID = profiles.first?.id
        }
    }

    var selectedProfile: DeviceProfile? {
        profiles.first(where: { $0.id == selectedProfileID })
    }

    func status(for profile: DeviceProfile) -> BridgeStatus? {
        statuses[profile.id]
    }

    func setEnabled(_ enabled: Bool, profile: DeviceProfile) async {
        guard let cli, let path = profile.fileURL?.path else {
            errorMessage = "The bridge helper or profile is unavailable."
            return
        }
        isBusy = true
        defer { isBusy = false }
        do {
            _ = try await cli.run([enabled ? "enable" : "disable", "--profile", path])
            if enabled {
                _ = try await cli.run(["install"])
            } else {
                _ = try? await cli.run(["restart"])
            }
            try? await Task.sleep(for: .milliseconds(600))
            reload()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func discoverDevices() async throws -> [DiscoveredDevice] {
        guard let cli else { throw CommandFailure(command: "discover", message: "The bridge helper is unavailable.") }
        let output = try await cli.run(["discover", "--timeout", "8s", "--json"])
        return try JSONDecoder().decode([DiscoveredDevice].self, from: Data(output.utf8))
    }

    func tailscalePeers() async throws -> [TailscalePeer] {
        guard let cli else { throw CommandFailure(command: "peers", message: "The bridge helper is unavailable.") }
        let output = try await cli.run(["peers", "--json"])
        return try JSONDecoder().decode([TailscalePeer].self, from: Data(output.utf8))
    }

    func addDevice(_ device: DiscoveredDevice, peer: TailscalePeer, name: String) async throws {
        guard let cli else { throw CommandFailure(command: "setup", message: "The bridge helper is unavailable.") }
        _ = try await cli.run([
            "setup", "--timeout", "8s",
            "--device", device.id,
            "--peer", peer.stableID,
            "--name", name,
        ])
        _ = try await cli.run(["install"])
        try? await Task.sleep(for: .milliseconds(700))
        reload()
        selectedProfileID = profiles.first(where: { $0.peer.nodeID == peer.stableID })?.id ?? profiles.first?.id
    }
}
