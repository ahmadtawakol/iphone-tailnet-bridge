import Foundation

struct CommandFailure: LocalizedError {
    let command: String
    let message: String

    var errorDescription: String? {
        message.isEmpty ? "\(command) failed." : message
    }
}
final class CLIClient: @unchecked Sendable {
    let executableURL: URL

    init() throws {
        let fileManager = FileManager.default
        var candidates: [URL] = []
        if let explicit = ProcessInfo.processInfo.environment["TAILBRIDGE_CLI_PATH"], !explicit.isEmpty {
            candidates.append(URL(fileURLWithPath: explicit))
        }
        if let resources = Bundle.main.resourceURL {
            candidates.append(resources.appendingPathComponent("iphone-tailnet-bridge"))
        }
        candidates.append(fileManager.homeDirectoryForCurrentUser.appendingPathComponent(".local/bin/iphone-tailnet-bridge"))
        guard let found = candidates.first(where: { fileManager.isExecutableFile(atPath: $0.path) }) else {
            throw CommandFailure(command: "bridge", message: "The bridge helper is missing. Reinstall iPhone Tailnet Bridge.")
        }
        executableURL = found
    }

    func run(_ arguments: [String]) async throws -> String {
        try await withCheckedThrowingContinuation { continuation in
            let process = Process()
            let output = Pipe()
            let errors = Pipe()
            process.executableURL = executableURL
            process.arguments = arguments
            process.standardOutput = output
            process.standardError = errors
            process.terminationHandler = { process in
                let stdout = String(data: output.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
                let stderr = String(data: errors.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
                if process.terminationStatus == 0 {
                    continuation.resume(returning: stdout)
                } else {
                    let message = stderr.trimmingCharacters(in: .whitespacesAndNewlines)
                        .replacingOccurrences(of: "error: ", with: "")
                    continuation.resume(throwing: CommandFailure(command: arguments.first ?? "bridge", message: message))
                }
            }
            do {
                try process.run()
            } catch {
                continuation.resume(throwing: error)
            }
        }
    }
}
