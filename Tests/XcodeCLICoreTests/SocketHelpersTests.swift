import Testing
import Foundation
#if canImport(Darwin)
import Darwin
#endif
@testable import XcodeCLICore

@Suite("SocketHelpers")
struct SocketHelpersTests {
    @Test("setUnixSocketPath with normal path returns true")
    func normalPath() {
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        #expect(setUnixSocketPath(&addr, to: "/tmp/test.sock"))
    }

    @Test("setUnixSocketPath with path > 103 chars returns false")
    func longPathTruncated() {
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let longPath = "/" + String(repeating: "a", count: 200)
        #expect(!setUnixSocketPath(&addr, to: longPath))
    }

    @Test("writeAllToFD writes all bytes to a pipe")
    func writeAllToPipe() {
        let pipe = Pipe()
        let fd = pipe.fileHandleForWriting.fileDescriptor
        let testData = Data("hello world\n".utf8)
        #expect(writeAllToFD(fd, testData))
        pipe.fileHandleForWriting.closeFile()
        let readBack = pipe.fileHandleForReading.availableData
        #expect(readBack == testData)
    }

    @Test("writeAllToFD with empty data returns true")
    func writeEmptyData() {
        let pipe = Pipe()
        let fd = pipe.fileHandleForWriting.fileDescriptor
        #expect(writeAllToFD(fd, Data()))
    }
}
