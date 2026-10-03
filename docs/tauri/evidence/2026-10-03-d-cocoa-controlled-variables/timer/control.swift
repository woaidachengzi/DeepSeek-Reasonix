import AppKit
import Foundation

final class Probe: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    var tokens: [NSObjectProtocol] = []
    var events: [String] = []
    var records: [[String: Any]] = []
    var step = 0
    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 320, y: 160, width: 1280, height: 820), styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView], backing: .buffered, defer: false)
        window.title = "Plain empty Cocoa control"
        window.isReleasedWhenClosed = false
        for (name, label) in [(NSWindow.willMiniaturizeNotification, "will-mini"), (NSWindow.didMiniaturizeNotification, "did-mini"), (NSWindow.didDeminiaturizeNotification, "did-demini"), (NSWindow.didBecomeKeyNotification, "key"), (NSWindow.didResignKeyNotification, "resign-key"), (NSWindow.didChangeOcclusionStateNotification, "occlusion")] {
            tokens.append(NotificationCenter.default.addObserver(forName: name, object: window, queue: .main) { [weak self] _ in self?.events.append(label) })
        }
        window.makeKeyAndOrderFront(nil)
        schedule()
    }
    func schedule() {
        _ = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: false) { [weak self] _ in self?.sample() }
    }
    func sample() {
        precondition(Thread.isMainThread)
        if step == 0 {
            window.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
        }
        if step == 4 { window.miniaturize(window) }
        if step == 14 { window.deminiaturize(window) }
        let r: [String: Any] = ["step": step, "nativeKey": window.isKeyWindow, "applicationActive": NSApp.isActive, "nativeVisible": window.isVisible, "nativeMiniaturized": window.isMiniaturized, "nativeEvents": events]
        records.append(r)
        print(String(data: try! JSONSerialization.data(withJSONObject: r, options: [.sortedKeys]), encoding: .utf8)!)
        fflush(stdout)
        step += 1
        if step < 24 { schedule(); return }
        let precondition = ["nativeKey", "applicationActive", "nativeVisible"].allSatisfy { records[3][$0] as? Bool == true }
        let minimized = records[5..<14].contains { $0["nativeMiniaturized"] as? Bool == true }
        let restored = minimized && records[15...].contains { $0["nativeMiniaturized"] as? Bool == false }
        let passed = precondition && minimized && restored && events.contains("will-mini") && events.contains("did-mini") && events.contains("did-demini")
        let result: [String: Any] = ["stage": passed ? "passed" : "failed", "precondition": precondition, "minimized": minimized, "restored": restored, "events": events]
        print(String(data: try! JSONSerialization.data(withJSONObject: result, options: [.sortedKeys]), encoding: .utf8)!)
        fflush(stdout)
        for token in tokens { NotificationCenter.default.removeObserver(token) }
        window.orderOut(nil)
        exit(passed ? 0 : 2)
    }
}
let app = NSApplication.shared
let probe = Probe()
app.setActivationPolicy(.regular)
app.delegate = probe
app.run()
