import AppKit

final class FocusSink: NSView {
  override var acceptsFirstResponder: Bool { true }
}

final class Delegate: NSObject, NSApplicationDelegate, NSTextFieldDelegate {
  var window: NSWindow!
  var field: NSTextField!
  var label: NSTextField!
  var scroll: NSScrollView!
  var clicks = 0
  var scrollEvents = 0
  var lastScroll: [String: Any] = [:]
  func record() {
    let d: [String: Any] = [
      "pid": ProcessInfo.processInfo.processIdentifier, "clicks": clicks, "text": field.stringValue,
      "scroll_y": scroll.contentView.bounds.origin.y,
      "scroll_events": scrollEvents, "last_scroll": lastScroll,
    ]
    if let b = try? JSONSerialization.data(withJSONObject: d, options: [.sortedKeys]) {
      try? b.write(
        to: URL(
          fileURLWithPath: ProcessInfo.processInfo.environment["REASONIX_CONTROL_FIXTURE_STATE"]!),
        options: .atomic)
    }
  }
  @objc func clicked(_ sender: Any?) {
    clicks += 1
    label.stringValue = "Clicks: \(clicks)"
    record()
  }
  func controlTextDidChange(_ obj: Notification) { record() }
  func applicationDidFinishLaunching(_ n: Notification) {
    let menu = NSMenu()
    let editItem = NSMenuItem(title: "Edit", action: nil, keyEquivalent: "")
    let edit = NSMenu(title: "Edit")
    let selectAll = NSMenuItem(
      title: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
    selectAll.keyEquivalentModifierMask = .command
    edit.addItem(selectAll)
    editItem.submenu = edit
    menu.addItem(editItem)
    NSApp.mainMenu = menu
    window = NSWindow(
      contentRect: NSRect(x: 180, y: 200, width: 600, height: 500),
      styleMask: [.titled, .closable], backing: .buffered, defer: false)
    window.title = "Reasonix isolated desktop acceptance"
    let root = FocusSink(frame: window.contentView!.bounds)
    window.contentView = root
    let title = NSTextField(labelWithString: "Local test only — Reasonix computer_use")
    title.frame = NSRect(x: 24, y: 434, width: 550, height: 26)
    root.addSubview(title)
    let button = NSButton(title: "Increment test counter", target: self, action: #selector(clicked))
    button.frame = NSRect(x: 24, y: 376, width: 240, height: 36)
    root.addSubview(button)
    label = NSTextField(labelWithString: "Clicks: 0")
    label.frame = NSRect(x: 290, y: 380, width: 200, height: 26)
    root.addSubview(label)
    field = NSTextField(string: "")
    field.placeholderString = "Type acceptance text here"
    field.frame = NSRect(x: 24, y: 320, width: 550, height: 32)
    field.delegate = self
    root.addSubview(field)
    scroll = NSScrollView(frame: NSRect(x: 24, y: 24, width: 550, height: 270))
    scroll.hasVerticalScroller = true
    let text = NSTextView(frame: NSRect(x: 0, y: 0, width: 530, height: 1800))
    text.isEditable = false
    text.string = (1...60).map { "Fixture row \($0)" }.joined(separator: "\n")
    scroll.documentView = text
    root.addSubview(scroll)
    NotificationCenter.default.addObserver(
      forName: NSView.boundsDidChangeNotification, object: scroll.contentView, queue: .main
    ) { [weak self] _ in self?.record() }
    scroll.contentView.postsBoundsChangedNotifications = true
    NSEvent.addLocalMonitorForEvents(matching: .scrollWheel) { [weak self] event in
      guard let self else { return event }
      self.scrollEvents += 1
      self.lastScroll = [
        "delta": event.scrollingDeltaY, "window": event.windowNumber,
        "x": event.locationInWindow.x, "y": event.locationInWindow.y,
      ]
      self.record()
      return event
    }
    window.center()
    window.initialFirstResponder = root
    window.makeKeyAndOrderFront(nil)
    window.makeFirstResponder(root)
    NSApp.activate(ignoringOtherApps: true)
    record()
  }
  func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}
let app = NSApplication.shared
let delegate = Delegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
