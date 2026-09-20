import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationSplitView {
            List(model.profiles, selection: $model.selectedProfileID) { profile in
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(profile.name)
                        Text(model.status(for: profile)?.title ?? (profile.enabled ? "Starting" : "Off"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } icon: {
                    Image(systemName: model.status(for: profile)?.symbol ?? "iphone")
                        .foregroundStyle(model.status(for: profile)?.tint ?? .secondary)
                }
                .tag(profile.id)
            }
            .navigationTitle("Devices")
            .navigationSplitViewColumnWidth(min: 190, ideal: 210, max: 260)
            .safeAreaInset(edge: .bottom) {
                Button {
                    model.showingAddDevice = true
                } label: {
                    Label("Add iPhone", systemImage: "plus")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderless)
                .padding(10)
            }
        } detail: {
            if let profile = model.selectedProfile {
                DeviceDetail(profile: profile, status: model.status(for: profile))
            } else {
                FirstRunView()
            }
        }
        .frame(minWidth: 720, minHeight: 460)
        .toolbar {
            ToolbarItem {
                Button {
                    model.reload()
                } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
            }
        }
        .sheet(isPresented: $model.showingAddDevice) {
            AddDeviceView()
                .environmentObject(model)
        }
        .alert("iPhone Tailnet Bridge", isPresented: Binding(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("OK", role: .cancel) { model.errorMessage = nil }
        } message: {
            Text(model.errorMessage ?? "")
        }
    }
}

private struct FirstRunView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "iphone.and.arrow.forward")
                .font(.system(size: 42))
                .foregroundStyle(.secondary)
            Text("Connect an iPhone")
                .font(.title2.bold())
            Text("Pair the iPhone with Xcode once, keep it on this Wi-Fi for setup, and sign in to the same Tailscale network.")
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 430)
            Button("Add iPhone") { model.showingAddDevice = true }
                .buttonStyle(.borderedProminent)
        }
        .padding(40)
    }
}

private struct DeviceDetail: View {
    @EnvironmentObject private var model: AppModel
    let profile: DeviceProfile
    let status: BridgeStatus?

    var body: some View {
        Form {
            Section {
                HStack(spacing: 12) {
                    Image(systemName: status?.symbol ?? "iphone")
                        .font(.system(size: 28))
                        .foregroundStyle(status?.tint ?? .secondary)
                        .frame(width: 36)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(status?.title ?? (profile.enabled ? "Starting" : "Off"))
                            .font(.headline)
                        Text(status?.message ?? (profile.enabled ? "Waiting for the bridge service" : "Bridge disabled"))
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Toggle("Bridge", isOn: Binding(
                        get: { profile.enabled },
                        set: { enabled in Task { await model.setEnabled(enabled, profile: profile) } }
                    ))
                    .toggleStyle(.switch)
                    .labelsHidden()
                    .disabled(model.isBusy)
                }
                .padding(.vertical, 4)
            }

            Section("Connection") {
                LabeledContent("Tailscale device", value: profile.peer.name)
                LabeledContent("Tailnet address", value: profile.peer.address)
                if let interface = status?.interface {
                    LabeledContent("Mac interface", value: interface)
                }
                LabeledContent("Captured services", value: String(profile.services.count))
            }

            if let relay = status?.relay, status?.state == "active" || status?.state == "degraded" {
                Section("Relay") {
                    LabeledContent("Listeners", value: "\(relay.tcpListeners) TCP · \(relay.udpListeners) UDP")
                    LabeledContent("Active connections", value: String(relay.activeTCP + relay.activeUDP))
                    LabeledContent("Completed TCP connections", value: String(relay.acceptedTCP))
                    if relay.targetDialFailures > 0 {
                        LabeledContent("Failed phone connections", value: String(relay.targetDialFailures))
                            .foregroundStyle(.orange)
                    }
                }
            }

            Section {
                Text("When the iPhone is on this Wi-Fi, Apple’s normal connection is used. The proxy activates only after the phone moves to another Wi-Fi while remaining connected to Tailscale.")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .navigationTitle(profile.name)
    }
}

struct AddDeviceView: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var model: AppModel
    @State private var devices: [DiscoveredDevice] = []
    @State private var peers: [TailscalePeer] = []
    @State private var selectedDeviceID: String?
    @State private var selectedPeerID: String?
    @State private var name = ""
    @State private var loading = true
    @State private var saving = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Add iPhone").font(.title2.bold())
                Spacer()
            }
            .padding()
            Divider()

            if loading {
                ProgressView("Looking for paired iPhones and Tailscale devices…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let errorMessage {
                VStack(spacing: 12) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.system(size: 34))
                        .foregroundStyle(.orange)
                    Text("Setup couldn’t continue").font(.headline)
                    Text(errorMessage)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                    Button("Try Again") { Task { await load() } }
                }
                .padding(32)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if devices.isEmpty {
                VStack(spacing: 12) {
                    Image(systemName: "iphone.slash")
                        .font(.system(size: 34))
                        .foregroundStyle(.secondary)
                    Text("No paired iPhone found").font(.headline)
                    Text("Keep the iPhone unlocked on this Wi-Fi, enable Developer Mode, and pair it with Xcode once over USB.")
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                    Button("Scan Again") { Task { await load() } }
                }
                .padding(32)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                Form {
                    Picker("Paired iPhone", selection: $selectedDeviceID) {
                        ForEach(devices) { device in Text(device.name).tag(Optional(device.id)) }
                    }
                    Picker("Tailscale device", selection: $selectedPeerID) {
                        ForEach(peers) { peer in Text("\(peer.displayName) — \(peer.ipv4)").tag(Optional(peer.stableID)) }
                    }
                    TextField("Display name", text: $name)
                }
                .formStyle(.grouped)
            }

            Divider()
            HStack {
                Button("Cancel", role: .cancel) { dismiss() }
                Spacer()
                Button("Add Device") { Task { await save() } }
                    .buttonStyle(.borderedProminent)
                    .disabled(selectedDevice == nil || selectedPeer == nil || saving)
            }
            .padding()
        }
        .frame(width: 520, height: 390)
        .task { await load() }
    }

    private var selectedDevice: DiscoveredDevice? {
        devices.first(where: { $0.id == selectedDeviceID })
    }

    private var selectedPeer: TailscalePeer? {
        peers.first(where: { $0.stableID == selectedPeerID })
    }

    private func load() async {
        loading = true
        errorMessage = nil
        do {
            async let foundDevices = model.discoverDevices()
            async let foundPeers = model.tailscalePeers()
            let (newDevices, newPeers) = try await (foundDevices, foundPeers)
            devices = newDevices
            peers = newPeers
            selectedDeviceID = devices.first?.id
            selectedPeerID = bestPeer(for: devices.first)?.stableID ?? peers.first?.stableID
            name = devices.first?.name ?? ""
        } catch {
            errorMessage = error.localizedDescription
        }
        loading = false
    }

    private func bestPeer(for device: DiscoveredDevice?) -> TailscalePeer? {
        guard let device else { return nil }
        let normalized = device.name.lowercased().filter(\.isLetter)
        return peers.first { $0.displayName.lowercased().filter(\.isLetter) == normalized }
    }

    private func save() async {
        guard let device = selectedDevice, let peer = selectedPeer else { return }
        saving = true
        defer { saving = false }
        do {
            try await model.addDevice(device, peer: peer, name: name.isEmpty ? device.name : name)
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
