import AppKit
import WebKit
import Foundation

final class Probe: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var timer: Timer?
    var tokens: [NSObjectProtocol] = []
    var events: [String] = []
    var phase = "load"
    var deadline = Date().addingTimeInterval(10)
    let start = Date()
    func trace(_ stage: String) {
        let data: [String: Any] = ["stage": stage, "elapsedMs": Int(Date().timeIntervalSince(start)*1000), "active": NSApp.isActive, "key": window?.isKeyWindow ?? false, "visible": window?.isVisible ?? false, "occlusionVisible": window?.occlusionState.contains(.visible) ?? false, "minimized": window?.isMiniaturized ?? false, "events": events]
        print(String(data: try! JSONSerialization.data(withJSONObject: data, options: [.sortedKeys]), encoding: .utf8)!)
        fflush(stdout)
    }
    func finish(_ code: Int32) {
        trace(code == 0 ? "passed" : "failed-" + phase)
        for token in tokens { NotificationCenter.default.removeObserver(token) }
        timer?.invalidate()
        window?.orderOut(nil)
        exit(code)
    }
    func applicationDidFinishLaunching(_ notification: Notification) {
        window = NSWindow(contentRect: NSRect(x: 320, y: 160, width: 1280, height: 820), styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView], backing: .buffered, defer: false)
        window.title = "Reasonix private Cocoa minimize diagnostic"
        window.isReleasedWhenClosed = false
        web = WKWebView(frame: window.contentView!.bounds)
        web.autoresizingMask = [.width, .height]
        window.contentView!.addSubview(web)
        web.navigationDelegate = self
        for (name, label) in [(NSWindow.willMiniaturizeNotification, "will-mini"), (NSWindow.didMiniaturizeNotification, "did-mini"), (NSWindow.didDeminiaturizeNotification, "did-demini"), (NSWindow.didBecomeKeyNotification, "key"), (NSWindow.didResignKeyNotification, "resign-key"), (NSWindow.didChangeOcclusionStateNotification, "occlusion")] {
            tokens.append(NotificationCenter.default.addObserver(forName: name, object: window, queue: nil) { [weak self] _ in self?.events.append(label) })
        }
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        web.loadHTMLString("<html><body>Private fixed Cocoa diagnostic</body></html>", baseURL: nil)
        timer = Timer.scheduledTimer(withTimeInterval: 0.05, repeats: true) { [weak self] _ in self?.tick() }
        trace("ready")
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        if phase != "load" { return }
        trace("loaded")
        window.orderOut(nil)
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        phase = "key"
        deadline = Date().addingTimeInterval(5)
    }
    func tick() {
        if Date() > deadline { finish(1); return }
        if phase == "key" && NSApp.isActive && window.isKeyWindow {
            trace("minimize-ready")
            phase = "mini"
            deadline = Date().addingTimeInterval(5)
            window.miniaturize(window)
            trace("minimize-requested")
        } else if phase == "mini" && window.isMiniaturized && events.contains("will-mini") && events.contains("did-mini") {
            trace("minimized")
            phase = "demini"
            deadline = Date().addingTimeInterval(5)
            window.deminiaturize(window)
        } else if phase == "demini" && !window.isMiniaturized && events.contains("did-demini") {
            finish(0)
        }
    }
}
let app = NSApplication.shared
let probe = Probe()
app.setActivationPolicy(.regular)
app.delegate = probe
app.run()
