// Preserve every ordered item/type without printing clipboard contents.
import AppKit
import Foundation
import Darwin

struct Entry: Codable, Equatable { let type: String; let data: Data }
struct Snapshot: Codable { let count: Int; let nonce: String; let items: [[Entry]]; let ownedPath: String? }
enum Failure: Error { case bounds, unavailable, changed, invalid, restore }

func privatePathSource(_ snapshotURL: URL, _ sourceURL: URL) throws -> (String, String) {
    let temporary = snapshotURL.deletingLastPathComponent()
    let mode = temporary.deletingLastPathComponent()
    let root = mode.deletingLastPathComponent()
    guard temporary.lastPathComponent == "tmp", ["managed", "explicit"].contains(mode.lastPathComponent),
          root.deletingLastPathComponent().standardizedFileURL == URL(fileURLWithPath: "/private/tmp").standardizedFileURL,
          sourceURL == temporary.appendingPathComponent("reasonix-native-ui-clipboard-path.json") else { throw Failure.invalid }
    for directory in [root, mode, temporary] {
        let attributes = try FileManager.default.attributesOfItem(atPath: directory.path)
        guard directory.resolvingSymlinksInPath().standardizedFileURL == directory.standardizedFileURL,
              attributes[.type] as? FileAttributeType == .typeDirectory,
              (attributes[.ownerAccountID] as? NSNumber)?.uint32Value == geteuid(),
              (attributes[.posixPermissions] as? NSNumber)?.intValue == 0o700 else { throw Failure.invalid }
    }
    let attributes = try FileManager.default.attributesOfItem(atPath: sourceURL.path)
    guard attributes[.type] as? FileAttributeType == .typeRegular,
          sourceURL.resolvingSymlinksInPath().standardizedFileURL == sourceURL.standardizedFileURL,
          (attributes[.ownerAccountID] as? NSNumber)?.uint32Value == geteuid(),
          (attributes[.posixPermissions] as? NSNumber)?.intValue == 0o600,
          ((attributes[.size] as? NSNumber)?.intValue ?? Int.max) <= 8192 else { throw Failure.invalid }
    let bytes = try Data(contentsOf: sourceURL)
    guard bytes.count <= 8192,
          let control = try JSONSerialization.jsonObject(with: bytes) as? [String: String],
          Set(control.keys) == Set(["nonce", "path"]), let nonce = control["nonce"],
          nonce.count == 32, nonce.utf8.allSatisfy({ (48...57).contains($0) || (97...102).contains($0) }),
          root.lastPathComponent == "reasonix-native-ui-clipboard-" + nonce,
          let path = control["path"], path.utf8.count <= 4096,
          path.hasPrefix(mode.path + "/"), !path.unicodeScalars.contains(where: { CharacterSet.controlCharacters.contains($0) }) else { throw Failure.invalid }
    let expected = URL(fileURLWithPath: path)
    guard expected.resolvingSymlinksInPath().standardizedFileURL == expected.standardizedFileURL else { throw Failure.invalid }
    let components = String(path.dropFirst(mode.path.count + 1)).split(separator: "/", omittingEmptySubsequences: false)
    var directory = mode
    for component in components {
        guard !component.isEmpty, component != ".", component != ".." else { throw Failure.invalid }
        directory.appendPathComponent(String(component))
        let attributes = try FileManager.default.attributesOfItem(atPath: directory.path)
        guard attributes[.type] as? FileAttributeType == .typeDirectory,
              (attributes[.ownerAccountID] as? NSNumber)?.uint32Value == geteuid() else { throw Failure.invalid }
    }
    return (nonce, path)
}

func items(_ board: NSPasteboard) throws -> [[Entry]] {
    let originals = board.pasteboardItems ?? []
    guard originals.count <= 32 else { throw Failure.bounds }
    var total = 0
    return try originals.map { item in
        guard item.types.count <= 64 else { throw Failure.bounds }
        return try item.types.map { type in
            // File promises cannot be restored by copying their bytes.
            guard type.rawValue.utf8.count <= 1024, !type.rawValue.lowercased().contains("promise"),
                  let data = item.data(forType: type) else { throw Failure.unavailable }
            total += data.count
            guard total <= 32 * 1024 * 1024 else { throw Failure.bounds }
            return Entry(type: type.rawValue, data: data)
        }
    }
}

func perform(_ args: [String], _ board: NSPasteboard) throws {
    guard args.count == 4 else { throw Failure.invalid }
    let path = URL(fileURLWithPath: args[2])
    if args[1] == "capture" || args[1] == "capture-path" {
        let pathSource = args[1] == "capture-path" ? try privatePathSource(path, URL(fileURLWithPath: args[3])) : nil
        let count = board.changeCount
        let content = try items(board)
        guard board.changeCount == count else { throw Failure.changed }
        let snapshot = Snapshot(count: count, nonce: pathSource?.0 ?? args[3], items: content, ownedPath: pathSource?.1)
        let encoder = PropertyListEncoder(); encoder.outputFormat = .binary
        try encoder.encode(snapshot).write(to: path, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: path.path)
        return
    }
    let bytes = try Data(contentsOf: path)
    guard bytes.count <= 40 * 1024 * 1024 else { throw Failure.bounds }
    let snapshot = try PropertyListDecoder().decode(Snapshot.self, from: bytes)
    let value = board.string(forType: .string)
    let expected = snapshot.ownedPath ?? "reasonix-native-clipboard-" + snapshot.nonce
    let ownValues = snapshot.ownedPath == nil ? [expected, expected + "-denied"] : [expected]
    let marker = URL(fileURLWithPath: args[3])
    let owned = try? JSONSerialization.jsonObject(with: Data(contentsOf: marker)) as? [String: Any]
    let checkpoint = marker.appendingPathExtension("before.json")
    if args[1] == "checkpoint" {
        // Record only the system generation immediately before a real UI
        // Copy/Cut. Keep the previous ownership receipt intact for recovery.
        let count = board.changeCount
        try JSONSerialization.data(withJSONObject: ["nonce": snapshot.nonce, "count": count])
            .write(to: checkpoint, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: checkpoint.path)
        guard board.changeCount == count else { throw Failure.changed }
        return
    }
    if args[1] == "claim" || args[1] == "claim-after" {
        // Physical UI copy/cut has no native smoke receipt. Claim only this
        // unique fixture value and its unchanged system generation, without
        // writing the pasteboard or printing any original clipboard data.
        let count = board.changeCount
        if args[1] == "claim-after" {
            guard let bytes = try? Data(contentsOf: checkpoint), bytes.count <= 8192,
                  let before = try JSONSerialization.jsonObject(with: bytes) as? [String: Any],
                  before["nonce"] as? String == snapshot.nonce,
                  let prior = before["count"] as? Int else { throw Failure.invalid }
            guard count > prior else { throw Failure.changed }
        }
        guard board.string(forType: .string) == ownValues[0], try items(board).count == 1,
              board.changeCount == count else { throw Failure.changed }
        try JSONSerialization.data(withJSONObject: ["nonce": snapshot.nonce, "count": count])
            .write(to: marker, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: marker.path)
        if args[1] == "claim-after" { try FileManager.default.removeItem(at: checkpoint) }
        return
    }
    if args[1] == "verify" {
        guard value == ownValues[0], owned?["nonce"] as? String == snapshot.nonce,
              owned?["count"] as? Int == board.changeCount else { throw Failure.changed }
        return
    }
    guard args[1] == "restore" else { throw Failure.invalid }
    if board.changeCount == snapshot.count { return }
    // A successful host supplies an exact generation. Interrupted runs may
    // restore only their unique, undisplayed canary text, never other content.
    guard ownValues.contains(value ?? "") else { throw Failure.changed }
    // Paths are displayed in the product. Require a separately claimed exact
    // system generation; never infer ownership from displayed text alone.
    if snapshot.ownedPath != nil {
        guard owned?["nonce"] as? String == snapshot.nonce,
              owned?["count"] as? Int == board.changeCount else { throw Failure.changed }
    }
    if let count = owned?["count"] as? Int, count != board.changeCount { throw Failure.changed }
    let restored = try snapshot.items.map { entries -> NSPasteboardItem in
        let item = NSPasteboardItem()
        for entry in entries {
            guard item.setData(entry.data, forType: .init(entry.type)) else { throw Failure.restore }
        }
        return item
    }
    board.clearContents()
    if !restored.isEmpty && !board.writeObjects(restored) { throw Failure.restore }
    guard try items(board) == snapshot.items else { throw Failure.restore }
}

func selftest(_ path: String) throws {
    let board = NSPasteboard(name: .init("io.reasonix.clipboard-acceptance." + UUID().uuidString))
    defer { board.releaseGlobally() }
    let first = NSPasteboardItem(), second = NSPasteboardItem()
    guard first.setString("first", forType: .string), first.setData(Data("<b>rich</b>".utf8), forType: .html),
          second.setString("second", forType: .string), second.setData(Data([0, 1, 255]), forType: .init("com.reasonix.fixture")) else { throw Failure.invalid }
    board.clearContents()
    guard board.writeObjects([first, second]) else { throw Failure.invalid }
    let original = try items(board), nonce = UUID().uuidString
    let marker = path + ".selftest-owned.json"
    let args = ["helper", "capture", path, nonce]
    try perform(args, board)
    board.clearContents(); guard board.setString("reasonix-native-clipboard-" + nonce, forType: .string) else { throw Failure.invalid }
    let claimCount = board.changeCount
    try perform(["helper", "claim", path, marker], board)
    guard board.changeCount == claimCount else { throw Failure.changed }
    try perform(["helper", "verify", path, marker], board)
    try perform(["helper", "restore", path, marker], board)
    guard try items(board) == original else { throw Failure.restore }
    try perform(args, board)
    board.clearContents(); guard board.setString("reasonix-native-clipboard-" + nonce, forType: .string) else { throw Failure.invalid }
    try JSONSerialization.data(withJSONObject: ["nonce": nonce, "count": board.changeCount]).write(to: URL(fileURLWithPath: marker))
    board.clearContents(); guard board.setString("external-change", forType: .string) else { throw Failure.invalid }
    do { try perform(["helper", "restore", path, marker], board); throw Failure.invalid }
    catch Failure.changed {}
    guard board.string(forType: .string) == "external-change" else { throw Failure.restore }
    do { try perform(["helper", "claim", path, marker], board); throw Failure.invalid }
    catch Failure.changed {}
    try perform(args, board)
    board.clearContents(); guard board.setString("reasonix-native-clipboard-" + nonce, forType: .string) else { throw Failure.invalid }
    try perform(["helper", "restore", path, path + ".absent"], board)
    guard board.string(forType: .string) == "external-change" else { throw Failure.restore }
    let promise = NSPasteboardItem()
    guard promise.setData(Data([1]), forType: .init("com.reasonix.promise")) else { throw Failure.invalid }
    board.clearContents(); guard board.writeObjects([promise]) else { throw Failure.invalid }
    let count = board.changeCount
    do { try perform(args, board); throw Failure.invalid }
    catch Failure.unavailable {}
    guard board.changeCount == count else { throw Failure.changed }

    // Path-button acceptance uses a separate, owner-only source and requires
    // a claimed generation even when the displayed path still matches.
    let pathNonce = UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased()
    let root = URL(fileURLWithPath: "/private/tmp/reasonix-native-ui-clipboard-" + pathNonce)
    let mode = root.appendingPathComponent("managed"), temporary = mode.appendingPathComponent("tmp")
    let expected = mode.appendingPathComponent("core"), source = temporary.appendingPathComponent("reasonix-native-ui-clipboard-path.json")
    for directory in [root, mode, temporary, expected] {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
    }
    defer { try? FileManager.default.removeItem(at: root) }
    func sourceValue(_ value: String) throws {
        try JSONSerialization.data(withJSONObject: ["nonce": pathNonce, "path": value]).write(to: source, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: source.path)
    }
    try sourceValue(expected.path)
    let backup = temporary.appendingPathComponent("path-original.plist").path
    let pathMarker = temporary.appendingPathComponent("path-owned.json").path
    let freshRich = try original.map { entries -> NSPasteboardItem in
        let item = NSPasteboardItem()
        for entry in entries {
            guard item.setData(entry.data, forType: .init(entry.type)) else { throw Failure.invalid }
        }
        return item
    }
    board.clearContents(); guard board.writeObjects(freshRich) else { throw Failure.invalid }
    let rich = try items(board)
    try perform(["helper", "capture-path", backup, source.path], board)
    board.clearContents(); guard board.setString(expected.path, forType: .string) else { throw Failure.invalid }
    do { try perform(["helper", "restore", backup, pathMarker], board); throw Failure.invalid }
    catch Failure.changed {}
    guard board.string(forType: .string) == expected.path else { throw Failure.restore }
    let pathCount = board.changeCount
    try perform(["helper", "claim", backup, pathMarker], board)
    guard board.changeCount == pathCount else { throw Failure.changed }
    try perform(["helper", "verify", backup, pathMarker], board)
    try perform(["helper", "checkpoint", backup, pathMarker], board)
    let noWriteCount = board.changeCount
    do { try perform(["helper", "claim-after", backup, pathMarker], board); throw Failure.invalid }
    catch Failure.changed {}
    guard board.changeCount == noWriteCount else { throw Failure.changed }
    // Same text only counts as another Copy/Cut after a real new generation.
    board.clearContents(); guard board.setString(expected.path, forType: .string) else { throw Failure.invalid }
    try perform(["helper", "claim-after", backup, pathMarker], board)
    try perform(["helper", "verify", backup, pathMarker], board)
    do { try perform(["helper", "claim-after", backup, pathMarker], board); throw Failure.changed }
    catch Failure.invalid {}
    try perform(["helper", "restore", backup, pathMarker], board)
    guard try items(board) == rich else { throw Failure.restore }
    try perform(["helper", "capture-path", backup, source.path], board)
    board.clearContents(); guard board.setString(expected.path, forType: .string) else { throw Failure.invalid }
    try perform(["helper", "claim", backup, pathMarker], board)
    board.clearContents(); guard board.setString(expected.path, forType: .string) else { throw Failure.invalid }
    do { try perform(["helper", "restore", backup, pathMarker], board); throw Failure.invalid }
    catch Failure.changed {}
    guard board.string(forType: .string) == expected.path else { throw Failure.restore }
    try sourceValue("/private/tmp")
    do { try perform(["helper", "capture-path", backup, source.path], board); throw Failure.changed }
    catch Failure.invalid {}
    let directoryLink = mode.appendingPathComponent("core-link")
    try FileManager.default.createSymbolicLink(at: directoryLink, withDestinationURL: expected)
    try sourceValue(directoryLink.path)
    do { try perform(["helper", "capture-path", backup, source.path], board); throw Failure.changed }
    catch Failure.invalid {}
    try sourceValue(mode.path + "/core/../core")
    do { try perform(["helper", "capture-path", backup, source.path], board); throw Failure.changed }
    catch Failure.invalid {}
    try sourceValue(expected.path)
    try FileManager.default.setAttributes([.posixPermissions: 0o644], ofItemAtPath: source.path)
    do { try perform(["helper", "capture-path", backup, source.path], board); throw Failure.changed }
    catch Failure.invalid {}
    try sourceValue(expected.path)
    let linked = temporary.appendingPathComponent("source-link.json")
    try FileManager.default.moveItem(at: source, to: linked)
    try FileManager.default.createSymbolicLink(at: source, withDestinationURL: linked)
    let invalidCount = board.changeCount
    do { try perform(["helper", "capture-path", backup, source.path], board); throw Failure.changed }
    catch Failure.invalid {}
    guard board.changeCount == invalidCount, board.string(forType: .string) == expected.path else { throw Failure.restore }
}

func execute() throws {
    umask(0o077)
    let args = CommandLine.arguments
    guard args.count == 4 else { throw Failure.invalid }
    if args[1] == "selftest" { try selftest(args[2]); return }
    try perform(args, NSPasteboard.general)
}

do { try execute() }
catch Failure.changed { fputs("clipboard ownership changed; current contents preserved\n", stderr); exit(2) }
catch Failure.restore { fputs("clipboard restoration could not be verified\n", stderr); exit(3) }
catch { fputs("clipboard snapshot unavailable or exceeds acceptance bounds\n", stderr); exit(1) }
