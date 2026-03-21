import Foundation
#if canImport(Darwin)
import Darwin
#endif

/// Safely set the sun_path field of a sockaddr_un from a Swift String.
/// Uses withUnsafeMutableBytes to correctly access the full sun_path buffer.
/// Returns false if the path was truncated (exceeds sun_path capacity).
@discardableResult
func setUnixSocketPath(_ addr: inout sockaddr_un, to path: String) -> Bool {
    var truncated = false
    withUnsafeMutableBytes(of: &addr.sun_path) { buffer in
        buffer.baseAddress!.initializeMemory(as: UInt8.self, repeating: 0, count: buffer.count)
        path.withCString { cStr in
            let pathLen = strlen(cStr)
            let len = min(pathLen, buffer.count - 1)
            if pathLen > buffer.count - 1 { truncated = true }
            buffer.baseAddress!.copyMemory(from: cStr, byteCount: len)
        }
    }
    return !truncated
}

/// Write all bytes to a file descriptor, handling partial writes and EINTR.
/// Returns true on success, false on failure.
func writeAllToFD(_ fd: Int32, _ data: Data) -> Bool {
    data.withUnsafeBytes { ptr -> Bool in
        guard let base = ptr.baseAddress else { return false }
        var written = 0
        while written < ptr.count {
            let n = Darwin.write(fd, base + written, ptr.count - written)
            if n < 0 {
                if errno == EINTR { continue }
                return false
            }
            if n == 0 { return false }
            written += n
        }
        return true
    }
}
