// Isolated production-renderer probe. No native input, real profile, or TCC changes.
import AppKit
import WebKit
import Foundation

final class Host: NSObject, WKNavigationDelegate, WKScriptMessageHandler {
    var window: NSWindow!
    var view: WKWebView!
    let script: String
    let output: String
    var finished = false
    init(script: String, output: String) { self.script = script; self.output = output }
    func finish(_ text: String, passed: Bool) {
        if finished { return }; finished = true
        try? text.write(toFile: output, atomically: true, encoding: .utf8)
        DispatchQueue.main.asyncAfter(deadline:.now()+5) { exit(passed ? 0 : 1) }
        let config = WKSnapshotConfiguration()
        view.takeSnapshot(with: config) { image, _ in
                if let data = image?.tiffRepresentation, let bitmap = NSBitmapImageRep(data: data), let png = bitmap.representation(using: .png, properties: [:]) {
                    try? png.write(to: URL(fileURLWithPath: self.output + ".png"))
                }
                self.window.close(); exit(passed ? 0 : 1)
            }
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let text = message.body as? String, let data = text.data(using: .utf8), let result = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
        finish(text, passed: result["passed"] as? Bool == true)
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        webView.evaluateJavaScript(script + "\n'probe-started'") { _, error in
            if let error { self.finish("{\"passed\":false,\"error\":\"injection failed\"}", passed:false); print(error) }
        }
    }
}
let arguments=CommandLine.arguments
guard arguments.count == 4, let url = URL(string: arguments[3]), url.scheme == "http", url.host == "127.0.0.1" else {
    fputs("usage: native-transcript-probe SCRIPT OUTPUT LOOPBACK_URL\n", stderr); exit(2)
}
try String(ProcessInfo.processInfo.processIdentifier).write(toFile: arguments[2] + ".pid", atomically:true, encoding:.utf8)
let app=NSApplication.shared
app.setActivationPolicy(.regular)
app.finishLaunching()
let host=Host(script:try String(contentsOfFile:arguments[1],encoding:.utf8),output:arguments[2])
let config=WKWebViewConfiguration()
config.websiteDataStore = .nonPersistent()
config.userContentController.add(host,name:"result")
config.userContentController.addUserScript(WKUserScript(source:"window.__qaRaf=0;function qaFrame(){window.__qaRaf++;requestAnimationFrame(qaFrame)};requestAnimationFrame(qaFrame);window.__qaConsole=[];for(const level of ['warn','error']){const old=console[level].bind(console);console[level]=(...args)=>{if(window.__qaConsole.length<50)window.__qaConsole.push({level,text:args.map(String).join(' ')});old(...args)}};window.__qaErrors=[];window.addEventListener('error',e=>window.__qaErrors.push(e.message));window.addEventListener('unhandledrejection',e=>window.__qaErrors.push(String(e.reason)));",injectionTime:.atDocumentStart,forMainFrameOnly:true))
host.view=WKWebView(frame:NSRect(x:0,y:0,width:1200,height:800),configuration:config)
host.view.navigationDelegate=host
host.window=NSWindow(contentRect:NSRect(x:0,y:0,width:1200,height:800),styleMask:[.titled,.closable],backing:.buffered,defer:false)
host.window.isReleasedWhenClosed=false
host.window.contentView=host.view
host.window.center();host.window.makeKeyAndOrderFront(nil);app.activate(ignoringOtherApps:true)
host.view.load(URLRequest(url:url))
DispatchQueue.main.asyncAfter(deadline:.now()+90){host.view.evaluateJavaScript("JSON.stringify({phase:window.__navigationPhase,ready:document.readyState,visibility:document.visibilityState,focused:document.hasFocus(),rows:document.querySelector('.transcript')?.dataset.transcriptRowCount,topics:document.querySelectorAll('.project-tree__topic-main').length,raf:window.__qaRaf})") { value, error in
    print("probe state", value ?? "nil", "visible",host.window.isVisible,"key",host.window.isKeyWindow,"active",app.isActive)
    host.finish("{\"passed\":false,\"error\":\"native probe deadline\"}",passed:false)
}}
DispatchQueue.main.asyncAfter(deadline:.now()+0.2){host.window.makeKeyAndOrderFront(nil);app.activate(ignoringOtherApps:true)}
app.run()
