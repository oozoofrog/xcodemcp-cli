import Testing
@testable import xcodecli
import XcodeCLICore

@Suite("Tool Catalog Support")
struct ToolCatalogSupportTests {
    @Test("buildToolCatalogData keeps tool names in source order and highlights in requested order")
    func buildCatalogDataOrdering() {
        let tools: [JSONValue] = [
            .object([
                "name": .string("RunAllTests"),
                "description": .string("Runs all tests"),
                "inputSchema": .object([
                    "required": .array([.string("tabIdentifier")])
                ]),
            ]),
            .object([
                "name": .string("XcodeRead"),
                "description": .string("Reads a file"),
                "inputSchema": .object([
                    "required": .array([.string("tabIdentifier"), .string("filePath")])
                ]),
            ]),
            .object([
                "name": .string("BuildProject"),
                "description": .string("Builds the project"),
            ]),
        ]

        let result = buildToolCatalogData(
            tools,
            highlightToolNames: ["BuildProject", "XcodeRead", "MissingTool", "RunAllTests"]
        )

        #expect(result.names == ["RunAllTests", "XcodeRead", "BuildProject"])
        #expect(result.count == 3)
        #expect(
            result.highlights == [
                ToolHighlightData(name: "BuildProject", description: "Builds the project", requiredArgs: []),
                ToolHighlightData(name: "XcodeRead", description: "Reads a file", requiredArgs: ["tabIdentifier", "filePath"]),
                ToolHighlightData(name: "RunAllTests", description: "Runs all tests", requiredArgs: ["tabIdentifier"]),
            ]
        )
    }

    @Test("requiredArgsFromTool filters non-string values and returns empty on malformed schema")
    func requiredArgsExtraction() {
        let toolWithMixedRequired: JSONValue = .object([
            "name": .string("XcodeLS"),
            "inputSchema": .object([
                "required": .array([.string("tabIdentifier"), .int(1), .bool(true), .string("path")])
            ]),
        ])
        let malformedTool: JSONValue = .object([
            "name": .string("BrokenTool"),
            "inputSchema": .array([]),
        ])

        #expect(requiredArgsFromTool(toolWithMixedRequired) == ["tabIdentifier", "path"])
        #expect(requiredArgsFromTool(malformedTool).isEmpty)
    }

    @Test("toolName returns name or empty string for non-object values")
    func toolNameExtraction() {
        let named: JSONValue = .object(["name": .string("BuildProject")])
        let unnamed: JSONValue = .object(["description": .string("no name")])
        let nonObject: JSONValue = .string("not a tool")

        #expect(toolName(named) == "BuildProject")
        #expect(toolName(unnamed).isEmpty)
        #expect(toolName(nonObject).isEmpty)
    }

    @Test("toolDescription returns empty string when description is missing")
    func toolDescriptionFallback() {
        let described: JSONValue = .object([
            "name": .string("BuildProject"),
            "description": .string("Builds project"),
        ])
        let undescribed: JSONValue = .object([
            "name": .string("BuildProject")
        ])

        #expect(toolDescription(described) == "Builds project")
        #expect(toolDescription(undescribed).isEmpty)
    }
}
