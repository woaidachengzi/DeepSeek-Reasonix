import AppKit
import CoreGraphics
let current = NSWorkspace.shared.frontmostApplication
let session = CGSessionCopyCurrentDictionary() as? [String: Any]
let state: [String: Any] = ["frontmostPid": current?.processIdentifier ?? -1, "frontmostBundle": current?.bundleIdentifier ?? "unavailable", "frontmostActive": current?.isActive ?? false, "screenLocked": (session?["CGSSessionScreenIsLocked"] as? Bool).map { $0 as Any } ?? NSNull(), "onConsole": (session?["kCGSSessionOnConsoleKey"] as? Bool).map { $0 as Any } ?? NSNull(), "loginDone": (session?["kCGSessionLoginDoneKey"] as? Bool).map { $0 as Any } ?? NSNull()]
let data = try JSONSerialization.data(withJSONObject: state, options: [.sortedKeys])
print(String(decoding: data, as: UTF8.self))
