import Foundation

// MARK: - Plist Rendering and Management

/// Ensures the LaunchAgent plist exists with the correct content.
/// Returns (changed, registeredBinary) where changed indicates the plist was written.
public func ensureLaunchAgentPlist(
    paths: AgentPaths.Paths, label: String, binaryPath: String
) throws -> (changed: Bool, registeredBinary: String) {
    let desired = renderLaunchAgentPlist(paths: paths, label: label, binaryPath: binaryPath)
    let plistDir = (paths.plistPath as NSString).deletingLastPathComponent

    if let existingData = FileManager.default.contents(atPath: paths.plistPath) {
        let existing = String(data: existingData, encoding: .utf8) ?? ""
        let registered = readLaunchAgentBinaryPathFromString(existing) ?? ""

        if existing == desired {
            return (false, registered)
        }

        do {
            try FileManager.default.createDirectory(atPath: plistDir, withIntermediateDirectories: true, attributes: nil)
            try desired.write(toFile: paths.plistPath, atomically: true, encoding: .utf8)
        } catch {
            throw wrapPlistPermissionError(error, plistPath: paths.plistPath, plistDir: plistDir)
        }
        return (true, registered)
    }

    do {
        try FileManager.default.createDirectory(atPath: plistDir, withIntermediateDirectories: true, attributes: nil)
        try desired.write(toFile: paths.plistPath, atomically: true, encoding: .utf8)
    } catch {
        throw wrapPlistPermissionError(error, plistPath: paths.plistPath, plistDir: plistDir)
    }
    return (true, "")
}

/// Wraps permission-denied errors with actionable guidance for the user.
private func wrapPlistPermissionError(_ error: Error, plistPath: String, plistDir: String) -> Error {
    let desc = error.localizedDescription.lowercased()
    guard desc.contains("permission denied") || desc.contains("not permitted") else {
        return error
    }

    let fm = FileManager.default
    var hints: [String] = []

    // Check directory ownership
    if let attrs = try? fm.attributesOfItem(atPath: plistDir),
       let ownerID = attrs[.ownerAccountID] as? UInt,
       ownerID != getuid() {
        let ownerName = attrs[.ownerAccountName] as? String ?? "uid \(ownerID)"
        hints.append("\(plistDir) is owned by \(ownerName). Fix with: sudo chown $(whoami) \(plistDir)")
    }

    // Check if existing plist file is not writable
    if fm.fileExists(atPath: plistPath) && !fm.isWritableFile(atPath: plistPath) {
        if let attrs = try? fm.attributesOfItem(atPath: plistPath),
           let ownerID = attrs[.ownerAccountID] as? UInt,
           ownerID != getuid() {
            let ownerName = attrs[.ownerAccountName] as? String ?? "uid \(ownerID)"
            hints.append("\(plistPath) is owned by \(ownerName). Fix with: sudo chown $(whoami) \(plistPath)")
        } else {
            hints.append("\(plistPath) is not writable. Fix with: chmod u+w \(plistPath)")
        }
    }

    if hints.isEmpty {
        hints.append("Check permissions on \(plistDir) and \(plistPath)")
    }

    let guidance = hints.joined(separator: "\n  ")
    return XcodeCLIError.agentUnavailable(
        stage: "write plist",
        underlying: "permission denied writing LaunchAgent plist.\n  \(guidance)"
    )
}

/// Read the binary path from an existing plist file.
public func readLaunchAgentBinaryPath(_ plistPath: String) -> String? {
    guard let data = FileManager.default.contents(atPath: plistPath),
          let content = String(data: data, encoding: .utf8) else { return nil }
    return readLaunchAgentBinaryPathFromString(content)
}

/// Parse binary path from plist XML content.
/// Extracts the first <string> element inside the ProgramArguments <array>.
func readLaunchAgentBinaryPathFromString(_ content: String) -> String? {
    guard let data = content.data(using: .utf8) else { return nil }

    class PlistParser: NSObject, XMLParserDelegate {
        var inProgramArguments = false
        var foundKey = false
        var currentElement = ""
        var binaryPath: String?
        var collectingText = false
        var currentText = ""

        func parser(_ parser: XMLParser, didStartElement elementName: String,
                     namespaceURI: String?, qualifiedName: String?, attributes: [String: String]) {
            currentElement = elementName
            if elementName == "key" || (elementName == "string" && inProgramArguments) {
                collectingText = true
                currentText = ""
            }
        }

        func parser(_ parser: XMLParser, foundCharacters string: String) {
            if collectingText {
                currentText += string
            }
        }

        func parser(_ parser: XMLParser, didEndElement elementName: String,
                     namespaceURI: String?, qualifiedName: String?) {
            if elementName == "key" {
                collectingText = false
                inProgramArguments = (currentText == "ProgramArguments")
            } else if elementName == "string" && inProgramArguments && binaryPath == nil {
                collectingText = false
                binaryPath = currentText
                parser.abortParsing()
            } else if elementName == "array" && inProgramArguments {
                inProgramArguments = false
            }
        }
    }

    let parser = XMLParser(data: data)
    let delegate = PlistParser()
    parser.delegate = delegate
    parser.parse()
    return delegate.binaryPath
}

/// Render a LaunchAgent plist XML string.
public func renderLaunchAgentPlist(
    paths: AgentPaths.Paths, label: String, binaryPath: String
) -> String {
    """
    <?xml version="1.0" encoding="UTF-8"?>
    <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
    <plist version="1.0">
    <dict>
    \t<key>Label</key>
    \t<string>\(xmlEscape(label))</string>
    \t<key>ProgramArguments</key>
    \t<array>
    \t\t<string>\(xmlEscape(binaryPath))</string>
    \t\t<string>agent</string>
    \t\t<string>run</string>
    \t\t<string>--launch-agent</string>
    \t</array>
    \t<key>RunAtLoad</key>
    \t<true/>
    \t<key>StandardOutPath</key>
    \t<string>\(xmlEscape(paths.logPath))</string>
    \t<key>StandardErrorPath</key>
    \t<string>\(xmlEscape(paths.logPath))</string>
    </dict>
    </plist>
    """ + "\n"
}

func xmlEscape(_ raw: String) -> String {
    raw
        .replacingOccurrences(of: "&", with: "&amp;")
        .replacingOccurrences(of: "<", with: "&lt;")
        .replacingOccurrences(of: ">", with: "&gt;")
        .replacingOccurrences(of: "\"", with: "&quot;")
        .replacingOccurrences(of: "'", with: "&apos;")
}
