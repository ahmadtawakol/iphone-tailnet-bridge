import Foundation
import SwiftUI

struct DeviceProfile: Decodable, Identifiable, Hashable {
    struct Peer: Decodable, Hashable {
        let nodeID: String?
        let name: String
        let dnsName: String?
        let address: String
    }

    struct Service: Decodable, Hashable {
        let instance: String
        let type: String
        let hostname: String
        let port: Int
    }

    let id: String
    let name: String
    let provider: String
    let peer: Peer
    let services: [Service]
    let enabled: Bool
    var fileURL: URL?
}

struct BridgeStatusDocument: Decodable {
    let devices: [BridgeStatus]
}

struct BridgeStatus: Decodable, Identifiable {
    struct Relay: Decodable {
        let tcpListeners: Int
        let udpListeners: Int
        let activeTCP: Int64
        let activeUDP: Int64
        let acceptedTCP: Int64
        let rejectedClients: Int64
        let targetDialFailures: Int64
        let bytesToPhone: Int64
        let bytesFromPhone: Int64
    }

    let id: String
    let name: String
    let state: String
    let message: String
    let interface: String?
    let localIP: String?
    let targetIP: String?
    let relay: Relay

    var tint: Color {
        switch state {
        case "active": return .green
        case "degraded", "unreachable", "needs_wifi": return .orange
        case "local": return .blue
        case "error": return .red
        default: return .secondary
        }
    }

    var symbol: String {
        switch state {
        case "active": return "checkmark.circle.fill"
        case "degraded": return "exclamationmark.triangle.fill"
        case "unreachable": return "wifi.exclamationmark"
        case "needs_wifi": return "wifi.slash"
        case "local": return "wifi.circle.fill"
        case "starting", "waiting": return "clock.fill"
        case "error": return "xmark.circle.fill"
        default: return "pause.circle.fill"
        }
    }

    var title: String {
        switch state {
        case "active": return "Active"
        case "degraded": return "Degraded"
        case "unreachable": return "Unreachable"
        case "needs_wifi": return "Wi-Fi Required"
        case "local": return "Local"
        case "starting": return "Starting"
        case "waiting": return "Waiting"
        case "error": return "Error"
        default: return "Off"
        }
    }
}

struct DiscoveredDevice: Decodable, Identifiable, Hashable {
    let id: String
    let name: String
    let addresses: [String]?
}

struct TailscalePeer: Decodable, Identifiable, Hashable {
    let id: String?
    let nodeID: UInt64
    let hostName: String
    let dnsName: String
    let online: Bool
    let tailscaleIPs: [String]

    enum CodingKeys: String, CodingKey {
        case id = "ID"
        case nodeID = "NodeID"
        case hostName = "HostName"
        case dnsName = "DNSName"
        case online = "Online"
        case tailscaleIPs = "TailscaleIPs"
    }

    var stableID: String { id ?? String(nodeID) }
    var displayName: String {
        if !hostName.isEmpty && hostName.lowercased() != "localhost" { return hostName }
        return dnsName.split(separator: ".").first.map(String.init) ?? stableID
    }

    var ipv4: String {
        tailscaleIPs.first(where: { $0.contains(".") }) ?? ""
    }
}
