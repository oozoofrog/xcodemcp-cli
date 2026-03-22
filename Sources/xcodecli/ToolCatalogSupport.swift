import Foundation
import XcodeCLICore

struct ToolHighlightData: Equatable {
    let name: String
    let description: String
    let requiredArgs: [String]
}

struct ToolCatalogData: Equatable {
    let names: [String]
    let highlights: [ToolHighlightData]

    var count: Int { names.count }
}

func findToolByName(_ tools: [JSONValue], _ name: String) -> JSONValue? {
    tools.first { tool in
        if case .object(let obj) = tool, case .string(let toolName) = obj["name"] {
            return toolName == name
        }
        return false
    }
}

func requiredArgsFromTool(_ tool: JSONValue) -> [String] {
    guard case .object(let obj) = tool,
          case .object(let schema) = obj["inputSchema"],
          case .array(let req) = schema["required"]
    else {
        return []
    }

    return req.compactMap {
        if case .string(let arg) = $0 { return arg }
        return nil
    }
}

func toolName(_ tool: JSONValue) -> String {
    guard case .object(let obj) = tool,
          case .string(let name) = obj["name"]
    else {
        return ""
    }
    return name
}

func toolDescription(_ tool: JSONValue) -> String {
    guard case .object(let obj) = tool,
          case .string(let description) = obj["description"]
    else {
        return ""
    }
    return description
}

func buildToolCatalogData(_ tools: [JSONValue], highlightToolNames: [String]) -> ToolCatalogData {
    let names = tools.compactMap { tool -> String? in
        if case .object(let obj) = tool, case .string(let name) = obj["name"] {
            return name
        }
        return nil
    }

    let highlights = highlightToolNames.compactMap { name -> ToolHighlightData? in
        guard let tool = findToolByName(tools, name) else { return nil }
        return ToolHighlightData(
            name: name,
            description: toolDescription(tool),
            requiredArgs: requiredArgsFromTool(tool)
        )
    }

    return ToolCatalogData(names: names, highlights: highlights)
}
