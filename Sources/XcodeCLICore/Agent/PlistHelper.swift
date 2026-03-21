import Foundation

// MARK: - Plist Rendering and Management

/// Ensures the LaunchAgent plist exists with the correct content.
/// Returns (changed, registeredBinary) where changed indicates the plist was written.
public func ensureLaunchAgentPlist(
    paths: AgentPaths.Paths, label: String, binaryPath: String
) throws -> (changed: Bool, registeredBinary: String) {
    let desired = renderLaunchAgentPlist(paths: paths, label: label, binaryPath: binaryPath)

    if let existingData = FileManager.default.contents(atPath: paths.plistPath) {
        let existing = String(data: existingData, encoding: .utf8) ?? ""
        let registered = readLaunchAgentBinaryPathFromString(existing) ?? ""

        if existing == desired {
            return (false, registered)
        }

        let plistDir = (paths.plistPath as NSString).deletingLastPathComponent
        try FileManager.default.createDirectory(atPath: plistDir, withIntermediateDirectories: true, attributes: nil)
        try desired.write(toFile: paths.plistPath, atomically: true, encoding: .utf8)
        return (true, registered)
    }

    let plistDir = (paths.plistPath as NSString).deletingLastPathComponent
    try FileManager.default.createDirectory(atPath: plistDir, withIntermediateDirectories: true, attributes: nil)
    try desired.write(toFile: paths.plistPath, atomically: true, encoding: .utf8)
    return (true, "")
}

/// Read the binary path from an existing plist file.
public func readLaunchAgentBinaryPath(_ plistPath: String) -> String? {
    guard let data = FileManager.default.contents(atPath: plistPath),
          let content = String(data: data, encoding: .utf8) else { return nil }
    return readLaunchAgentBinaryPathFromString(content)
}

/// Parse binary path from plist XML content.
/// Extracts the first <string> element inside the ProgramArguments <array>.
private func readLaunchAgentBinaryPathFromString(_ content: String) -> String? {
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

    """
}

private func xmlEscape(_ raw: String) -> String {
    raw
        .replacingOccurrences(of: "&", with: "&amp;")
        .replacingOccurrences(of: "<", with: "&lt;")
        .replacingOccurrences(of: ">", with: "&gt;")
        .replacingOccurrences(of: "\"", with: "&quot;")
        .replacingOccurrences(of: "'", with: "&apos;")
}
