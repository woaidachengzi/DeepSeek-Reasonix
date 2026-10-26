import AppKit
import WebKit

final class Fixture: NSObject, NSApplicationDelegate {
    let window = NSWindow(contentRect: NSRect(x: 180, y: 180, width: 720, height: 480), styleMask: [.titled, .closable, .miniaturizable], backing: .buffered, defer: false)
    let field = NSTextField(frame: NSRect(x: 30, y: 365, width: 600, height: 32))
    let status = NSTextField(labelWithString: "等待临时授权，只测试本窗口。")
    var count = 0
    let web = WKWebView(frame: NSRect(x: 30, y: 20, width: 650, height: 225))
    var busy = false
    var process: Process!
    var input: FileHandle!
    var output: FileHandle!
    var buffer = Data()
    var sequence = 0
    var timer: Timer?
    var sentryTimer: Timer?
    var sentryDirectory: URL?
    var sentryMonitoring = false
    var sentryBegin: Date?
    var sentryFocusChanges = 0
    var sentryPointerChanges = 0
    var sentryKeyEvents = 0
    var sentryOrigin = NSPoint.zero
    var sentryLast = Data()
    var activationObserver: NSObjectProtocol?
    var keyObserver: Any?
    func applicationDidFinishLaunching(_ notification: Notification) {
        window.title = "Reasonix Cua QA · 仅本地测试"
        field.placeholderString = "Cua 验收文本框"
        field.setAccessibilityLabel("Cua 验收文本框")
        window.contentView!.addSubview(field)
        let button = NSButton(title: "Cua 本地测试按钮", target: self, action: #selector(clicked))
        button.frame = NSRect(x: 30, y: 310, width: 250, height: 36)
        window.contentView!.addSubview(button)
        status.frame = NSRect(x: 30, y: 265, width: 650, height: 30)
        window.contentView!.addSubview(status)
        web.loadHTMLString("<html><body><h3>本地网页验收</h3><label>网页测试文本 <input aria-label='网页测试文本'></label><button onclick='this.textContent=\"网页点击成功\"'>网页测试按钮</button></body></html>", baseURL: nil)
        window.contentView!.addSubview(web)
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        activationObserver = NSWorkspace.shared.notificationCenter.addObserver(forName: NSWorkspace.didActivateApplicationNotification, object: nil, queue: .main) { note in
            guard self.sentryMonitoring,
                  let app = note.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
                  app.processIdentifier != getpid() else { return }
            self.sentryFocusChanges += 1
        }
        keyObserver = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { event in
            if self.sentryMonitoring { self.sentryKeyEvents += 1 }
            return event
        }
        sentryTimer = Timer.scheduledTimer(withTimeInterval: 0.02, repeats: true) { _ in self.sentryTick() }
        process = Process()
        process.executableURL = Bundle.main.executableURL!.deletingLastPathComponent().appendingPathComponent("reasonix-cua-runtime")
        let stdin = Pipe(), stdout = Pipe()
        process.standardInput = stdin; process.standardOutput = stdout
        process.standardError = FileHandle.nullDevice
        input = stdin.fileHandleForWriting; output = stdout.fileHandleForReading
        do { try process.run() } catch { log(["launch_error": "\(error)"]); return }
        DispatchQueue.global().async { self.log(self.call(["action":"status"])) }
        timer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { _ in
            let flag = "/private/tmp/reasonix-cua-qa-command"
            guard let command = try? String(contentsOfFile: flag, encoding: .utf8) else { return }
            try? FileManager.default.removeItem(atPath: flag)
            guard !self.busy else { return }
            self.busy = true
            DispatchQueue.global().async { self.run(command.trimmingCharacters(in: .whitespacesAndNewlines)) }
        }
    }
    @objc func clicked() { count += 1; status.stringValue = "本地按钮计数：\(count)" }
    func sentryTick() {
        guard let directory = sentryDirectory else { return }
        let control = directory.appendingPathComponent("command")
        if let command = try? String(contentsOf: control, encoding: .utf8) {
            try? FileManager.default.removeItem(at: control)
            if command == "begin" {
                // Setup only. No activation, restoration or pointer changes
                // are allowed after the monitor begins.
                field.stringValue = ""
                window.makeKeyAndOrderFront(nil)
                NSApp.activate(ignoringOtherApps: true)
                window.makeFirstResponder(field)
                sentryFocusChanges = 0; sentryPointerChanges = 0; sentryKeyEvents = 0
                sentryBegin = Date(timeIntervalSinceNow: 0.4)
                status.stringValue = "后台并行验收：请在本窗口输入 reasonixparallel2026"
            } else if command == "end" {
                sentryMonitoring = false; sentryBegin = nil
            }
        }
        if let begin = sentryBegin, Date() >= begin {
            sentryBegin = nil
            sentryOrigin = NSEvent.mouseLocation
            sentryMonitoring = true
        }
        let ownFront = NSWorkspace.shared.frontmostApplication?.processIdentifier == getpid()
        if sentryMonitoring {
            // Events catch brief app activation; polling also catches a lost
            // foreground posture when no workspace notification arrives.
            if !ownFront && sentryFocusChanges == 0 { sentryFocusChanges = 1 }
            if NSEvent.mouseLocation != sentryOrigin && sentryPointerChanges == 0 { sentryPointerChanges = 1 }
        }
        let snapshot: [String:Any] = ["monitoring":sentryMonitoring,"own_frontmost":ownFront,
            "focus_changes":sentryFocusChanges,"pointer_changes":sentryPointerChanges,
            "key_events":sentryKeyEvents,"text":field.currentEditor()?.string ?? field.stringValue]
        if let data = try? JSONSerialization.data(withJSONObject:snapshot,options:[.sortedKeys]), data != sentryLast {
            try? data.write(to:directory.appendingPathComponent("state.json"),options:.atomic)
            sentryLast = data
        }
    }
    func call(_ request: [String:Any]) -> [String:Any] {
        sequence += 1
        let app = NSRunningApplication.current
        let target: [String:Any] = ["pid":app.processIdentifier,"name":"Reasonix Cua QA","bundle_id":Bundle.main.bundleIdentifier!,"launch_time":app.launchDate!.timeIntervalSince1970]
        let envelope: [String:Any] = ["protocol":1,"id":sequence,"request":request,"approved_target":target,"foreground_approved":request["delivery_mode"] as? String == "foreground"]
        input.write(try! JSONSerialization.data(withJSONObject: envelope) + Data([10]))
        while buffer.firstIndex(of: 10) == nil {
            let chunk = output.availableData
            if chunk.isEmpty { return ["error":"worker ended"] }
            buffer.append(chunk)
        }
        let end = buffer.firstIndex(of: 10)!
        let line = buffer.prefix(upTo: end); buffer.removeSubrange(...end)
        return (try? JSONSerialization.jsonObject(with: line)) as? [String:Any] ?? ["error":"invalid reply"]
    }
    func log(_ value: [String:Any]) {
        var small = value
        if let images = small["images"] as? [[String:Any]] { small["images"] = images.map { ["mime_type":$0["mime_type"] ?? "", "bytes":($0["data_base64"] as? String)?.count ?? 0] } }
        let data = try! JSONSerialization.data(withJSONObject: small, options: [.sortedKeys]) + Data([10])
        let path = "/private/tmp/reasonix-cua-qa-results.jsonl"
        if !FileManager.default.fileExists(atPath:path) { FileManager.default.createFile(atPath:path,contents:nil) }
        let handle = try! FileHandle(forWritingTo: URL(fileURLWithPath:path)); handle.seekToEndOfFile(); handle.write(data); try? handle.close()
    }
    func run(_ mode: String) {
        defer { DispatchQueue.main.async { self.busy = false } }
        if mode == "browser" || mode == "background" {
            let test = Process()
            let directory = Bundle.main.executableURL!.deletingLastPathComponent()
            test.executableURL = URL(fileURLWithPath:"/opt/homebrew/bin/python3")
            test.arguments = [Bundle.main.resourceURL!.appendingPathComponent("browser_acceptance.py").path,
                "--runtime",directory.appendingPathComponent("reasonix-cua-runtime").path,
                "--state-helper",directory.appendingPathComponent("browser-fixture-info").path,
                "--output",directory.deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("browser-results").path,
                "--allow-local-input"]
            if mode == "background" {
                let directory = FileManager.default.temporaryDirectory.appendingPathComponent("reasonix-background-sentry-"+UUID().uuidString)
                do { try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:true,attributes:[.posixPermissions:0o700]) }
                catch { log(["test_error":"could not create private sentry directory"]); return }
                DispatchQueue.main.sync { self.sentryDirectory = directory; self.sentryLast = Data() }
                test.arguments! += ["--background-only","--sentry-control",directory.path]
            }
            let pipe = Pipe(); test.standardOutput=pipe; test.standardError=pipe
            do {
                try test.run()
                let output = pipe.fileHandleForReading.readDataToEndOfFile()
                test.waitUntilExit()
                log(["browser_acceptance_exit":test.terminationStatus,"output":String(data:output,encoding:.utf8) ?? ""])
            } catch { log(["browser_acceptance_error":"\(error)"]) }
            return
        }
        if mode == "coordinates" || mode == "second-screen" {
            coordinateTest(second: mode == "second-screen"); return
        }
        if mode == "web" { webTest(); return }

        var base: [String:Any] = ["pid":getpid(),"window_id":DispatchQueue.main.sync { window.windowNumber }]
        base["action"] = "observe"
        let observed = call(base); log(observed)
        if mode == "observe" { return }
        let elements = (observed["result"] as? [String:Any])?["elements"] as? [[String:Any]] ?? []
        guard let token = elements.first(where: { ($0["label"] as? String) == "Cua 验收文本框" })?["element_token"] as? String else { log(["test_error":"native field token missing"]); return }
        var type = base; type["action"] = "type"; type["element_token"] = token; type["text"] = "Reasonix Cua 本地输入 123"
        log(call(type))
        let second = call(base); log(second)
        let refreshed = (second["result"] as? [String:Any])?["elements"] as? [[String:Any]] ?? []
        guard let buttonToken = refreshed.first(where: { ($0["label"] as? String) == "Cua 本地测试按钮" })?["element_token"] as? String else { log(["test_error":"native button token missing"]); return }
        var click = base; click["action"] = "click"; click["element_token"] = buttonToken
        log(call(click))
        log(call(base))
        let check = DispatchQueue.main.sync { ["field_value":field.stringValue,"click_count":count] as [String:Any] }
        log(["fixture_readback":check])
        DispatchQueue.main.async { self.status.stringValue = "测试结束：\(check)" }
    }
    func observation() -> [String:Any] {
        return call(["action":"observe","pid":getpid(),"window_id":DispatchQueue.main.sync { window.windowNumber }])
    }
    func rows(_ reply: [String:Any]) -> [[String:Any]] {
        return (reply["result"] as? [String:Any])?["elements"] as? [[String:Any]] ?? []
    }
    func coordinateTest(second: Bool) {
        if second {
            let moved = DispatchQueue.main.sync { () -> Bool in
                guard NSScreen.screens.count > 1 else { return false }
                let frame = NSScreen.screens[1].visibleFrame
                window.setFrameOrigin(NSPoint(x:frame.minX+80,y:frame.minY+80)); return true
            }
            if !moved { log(["test_error":"second display missing"]); return }
        }
        let state = observation(); log(state)
        guard let button = rows(state).first(where: { ($0["label"] as? String) == "Cua 本地测试按钮" }),
              let frame = button["screenshot_frame"] as? [String:Double],
              let capture = (state["result"] as? [String:Any])?["capture_id"] as? String,
              let x=frame["x"],let y=frame["y"],let w=frame["w"],let h=frame["h"] else { log(["test_error":"screenshot mapping missing"]); return }
        var action: [String:Any] = ["action":"move","pid":getpid(),"window_id":DispatchQueue.main.sync { window.windowNumber },"x":x+w/2,"y":y+h/2,"delivery_mode":"foreground"]
        log(call(action))
        action["action"]="click"
        action["capture_id"]=capture
        let before = DispatchQueue.main.sync { count }
        log(call(action)); log(observation())
        let after = DispatchQueue.main.sync { count }
        log(["coordinate_readback":["second_screen":second,"before":before,"after":after,"passed":after==before+1]])
    }
    func webTest() {
        let state=observation(); log(state)
        guard let fieldToken=rows(state).first(where: { ($0["label"] as? String)=="网页测试文本" && ($0["role"] as? String)=="AXTextField" })?["element_token"] as? String else { log(["test_error":"web field missing"]); return }
        let win=DispatchQueue.main.sync { window.windowNumber }
        log(call(["action":"type","pid":getpid(),"window_id":win,"element_token":fieldToken,"text":"Cua 网页输入验证","delivery_mode":"foreground"]))
        let next=observation(); log(next)
        guard let button=rows(next).first(where: { ($0["label"] as? String)=="网页测试按钮" })?["element_token"] as? String else { log(["test_error":"web button missing"]); return }
        log(call(["action":"click","pid":getpid(),"window_id":win,"element_token":button,"delivery_mode":"foreground"]))
        log(observation())
        DispatchQueue.main.async { self.web.evaluateJavaScript("({value:document.querySelector('input').value,button:document.querySelector('button').textContent})") { result,error in self.log(["web_readback":result ?? "missing","error":error.map { "\($0)" } ?? ""]) } }
    }
    func applicationWillTerminate(_ notification: Notification) { input?.closeFile(); process?.terminate() }
}
let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = Fixture(); app.delegate = delegate
app.run()
