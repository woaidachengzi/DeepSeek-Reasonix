import AppKit
import Foundation
import CoreGraphics
// Read-only native display availability; no window, activation or UI action.
let screens = NSScreen.screens
var ids = [CGDirectDisplayID](repeating: 0, count: 8)
var count: UInt32 = 0
let status = CGGetActiveDisplayList(8, &ids, &count)
let sample: [String: Any] = ["coreGraphicsStatus": status.rawValue, "coreGraphicsActiveCount": count, "nativeScreenCount": screens.count, "nativeMainScreenPresent": NSScreen.main != nil, "nativeScreens": screens.map { screen in ["width": screen.frame.width, "height": screen.frame.height, "scale": screen.backingScaleFactor] }]
print(String(data: try JSONSerialization.data(withJSONObject: sample, options: [.sortedKeys]), encoding: .utf8)!)
