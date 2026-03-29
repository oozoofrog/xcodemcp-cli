import ArgumentParser
import Foundation
import XcodeCLICore

struct UpdateCommand: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "update",
        abstract: "Update the installed xcodecli binary"
    )

    func run() async throws {
        let executablePath = try resolveCurrentUpdateExecutablePath()
        let result = try await Updater.run(
            currentVersion: Version.current,
            executablePath: executablePath,
            processRunner: SystemProcessRunner()
        )

        if result.alreadyUpToDate {
            print("xcodecli is already up to date (\(result.currentVersion))")
        } else {
            print("xcodecli updated from \(result.currentVersion) to \(result.targetVersion) (\(result.mode))")
        }
    }
}

private func resolveCurrentUpdateExecutablePath() throws -> String {
    if let path = Bundle.main.executablePath, !path.isEmpty {
        return URL(fileURLWithPath: path).resolvingSymlinksInPath().path
    }

    let argv0 = CommandLine.arguments.first ?? ""
    guard !argv0.isEmpty else {
        throw ValidationError("cannot resolve current executable path")
    }

    if (argv0 as NSString).isAbsolutePath {
        return (argv0 as NSString).standardizingPath
    }

    let cwd = FileManager.default.currentDirectoryPath
    return ((cwd as NSString).appendingPathComponent(argv0) as NSString).standardizingPath
}
