import Foundation
import XcodeCLICore

struct GuideCommandArgument: Equatable {
    let key: String
    let value: JSONValue
}

struct GuideCommandSpec: Equatable {
    let toolName: String
    let timeout: String
    let arguments: [GuideCommandArgument]
}

func formatToolCallCommand(_ spec: GuideCommandSpec) -> String {
    let renderedArguments = spec.arguments.map { argument in
        "\"\(argument.key)\":\(renderJSONValue(argument.value))"
    }.joined(separator: ",")

    return "xcodecli tool call \(spec.toolName) --timeout \(spec.timeout) --json '{\(renderedArguments)}'"
}

func buildGuideCommands(_ windowMatch: GuideWindowMatch, specs: [GuideCommandSpec]) -> [String] {
    guideCommandsPrefix(windowMatch) + specs.map(formatToolCallCommand)
}

private func renderJSONValue(_ value: JSONValue) -> String {
    let encoder = JSONEncoder()
    guard let data = try? encoder.encode(value),
          let rendered = String(data: data, encoding: .utf8)
    else {
        return "null"
    }
    return rendered
}
