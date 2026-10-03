// Read-only activation timeline. Never activates, hides or controls any app.
import AppKit
import CoreGraphics
import Foundation

let start = ProcessInfo.processInfo.systemUptime
// Workspace notifications and KVO state need the main run loop serviced.
let tokens = [NSWorkspace.didActivateApplicationNotification,
              NSWorkspace.didDeactivateApplicationNotification,
              NSWorkspace.didTerminateApplicationNotification].map { name in
    NSWorkspace.shared.notificationCenter.addObserver(forName: name, object: nil, queue: .main) { note in
        guard let app = note.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication else { return }
        let event: [String: Any] = ["kind": "workspace-notification", "name": note.name.rawValue,
            "pid": app.processIdentifier, "bundle": app.bundleIdentifier ?? "unavailable",
            "unixMs": Int64(Date().timeIntervalSince1970 * 1000),
            "elapsedMs": Int64((ProcessInfo.processInfo.systemUptime - start) * 1000)]
        if let data = try? JSONSerialization.data(withJSONObject: event, options: [.sortedKeys]) {
            FileHandle.standardOutput.write(data)
            FileHandle.standardOutput.write(Data([10]))
        }
    }
}
var previous: Data?
while ProcessInfo.processInfo.systemUptime - start < 120 {
    autoreleasepool {
        let front = NSWorkspace.shared.frontmostApplication
        let session = CGSessionCopyCurrentDictionary() as? [String: Any]
        let previews = NSRunningApplication.runningApplications(withBundleIdentifier: "io.reasonix.desktop.preview")
            .sorted { $0.processIdentifier < $1.processIdentifier }
            .map { app -> [String: Any] in
                ["pid": app.processIdentifier, "active": app.isActive,
                 "hidden": app.isHidden, "terminated": app.isTerminated]
            }
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
    if !RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.05)) {
        Thread.sleep(forTimeInterval: 0.05)
    }
}

for token in tokens { NSWorkspace.shared.notificationCenter.removeObserver(token) }
