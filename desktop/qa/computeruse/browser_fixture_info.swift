// Read-only identity of the disposable Chrome process supplied by our fixture.
import AppKit
import CoreGraphics
if CommandLine.arguments.count == 4 && CommandLine.arguments[1] == "--launch" {
    let profile = CommandLine.arguments[2]
    let page = URL(string:CommandLine.arguments[3])!
    guard profile.hasPrefix("/"), page.isFileURL else {exit(2)}
    let config = NSWorkspace.OpenConfiguration()
    config.createsNewApplicationInstance = true
    config.arguments = ["--user-data-dir="+profile,"--remote-debugging-port=0",
        "--no-first-run","--no-default-browser-check","--disable-background-networking",
        "--disable-sync","--disable-component-update",page.absoluteString]
    var done = false
    var result:[String:Any] = [:]
    NSWorkspace.shared.openApplication(at:URL(fileURLWithPath:"/Applications/Google Chrome.app"),configuration:config) {app,error in
        if let app = app {result=["pid":Int(app.processIdentifier)]}
        else {result=["error":String(describing:error)]}
        done=true
    }
    while !done {RunLoop.current.run(until:Date(timeIntervalSinceNow:0.1))}
    FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject:result))
    exit(result["pid"] == nil ? 2 : 0)
}
let pid = pid_t(CommandLine.arguments.dropFirst().first ?? "") ?? 0
guard pid > 0, let app = NSRunningApplication(processIdentifier:pid),
      let launch = app.launchDate, let bundle = app.bundleIdentifier else { exit(2) }
let entries = CGWindowListCopyWindowInfo([.optionOnScreenOnly,.excludeDesktopElements],kCGNullWindowID) as? [[String:Any]] ?? []
let windows = entries.filter { ($0[kCGWindowOwnerPID as String] as? Int) == Int(pid) && ($0[kCGWindowLayer as String] as? Int) == 0 }
let data = try JSONSerialization.data(withJSONObject:["target":["pid":Int(pid),"name":app.localizedName ?? "Chromium Fixture","bundle_id":bundle,"launch_time":launch.timeIntervalSince1970],"windows":windows])
FileHandle.standardOutput.write(data)
