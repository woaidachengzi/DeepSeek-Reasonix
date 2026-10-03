// Read-only activation timeline. Never activates, hides or controls any app.
import AppKit
import CoreGraphics
import Foundation

let deadline = Date().addingTimeInterval(120)
let start = ProcessInfo.processInfo.systemUptime
var previous: Data?
while Date() < deadline {
    autoreleasepool {
        let front = NSWorkspace.shared.frontmostApplication
        let session = CGSessionCopyCurrentDictionary() as? [String: Any]
        let previews = NSRunningApplication.runningApplications(withBundleIdentifier: "io.reasonix.desktop.preview")
            .map { app -> [String: Any] in
                ["pid": app.processIdentifier, "active": app.isActive,
                 "hidden": app.isHidden, "terminated": app.isTerminated]
            }.sorted { ($0["pid"] as! Int32) < ($1["pid"] as! Int32) }
        var state: [String: Any] = [
            "frontmostPid": front?.processIdentifier ?? -1,
            "frontmostBundle": front?.bundleIdentifier ?? "unavailable",
            "frontmostActive": front?.isActive ?? false,
            "previews": previews,
            "screenLocked": (session?["CGSSessionScreenIsLocked"] as? Bool).map { $0 as Any } ?? NSNull(),
            "onConsole": (session?["kCGSSessionOnConsoleKey"] as? Bool).map { $0 as Any } ?? NSNull(),
            "loginDone": (session?["kCGSessionLoginDoneKey"] as? Bool).map { $0 as Any } ?? NSNull(),
        ]
        guard let key = try? JSONSerialization.data(withJSONObject: state, options: [.sortedKeys]) else { return }
        if previous != key {
            previous = key
            state["unixMs"] = Int64(Date().timeIntervalSince1970 * 1000)
            state["elapsedMs"] = Int64((ProcessInfo.processInfo.systemUptime - start) * 1000)
            if let data = try? JSONSerialization.data(withJSONObject: state, options: [.sortedKeys]) {
                FileHandle.standardOutput.write(data)
                FileHandle.standardOutput.write(Data([10]))
            }
        }
    }
    Thread.sleep(forTimeInterval: 0.05)
}
