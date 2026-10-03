import AppKit
import Foundation
let url = URL(string: "http://127.0.0.1/")!
if let app = NSWorkspace.shared.urlForApplication(toOpen: url) {
 let bundle = Bundle(url: app)
 let result: [String: Any] = ["applicationPath": app.path, "bundleIdentifier": bundle?.bundleIdentifier ?? "unknown", "running": NSWorkspace.shared.runningApplications.contains { $0.bundleURL == app }]
 print(String(data: try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys]), encoding: .utf8)!)
} else { print("{\"handlerAvailable\":false}") }
