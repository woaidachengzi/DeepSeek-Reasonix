// Preserve every ordered item/type without printing clipboard contents.
import AppKit
import Foundation
import Darwin

struct Entry: Codable, Equatable { let type: String; let data: Data }
struct Snapshot: Codable { let count: Int; let nonce: String; let items: [[Entry]] }
enum Failure: Error { case bounds, unavailable, changed, invalid, restore }

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
    if args[1] == "capture" {
        let count = board.changeCount
        let content = try items(board)
        guard board.changeCount == count else { throw Failure.changed }
        let snapshot = Snapshot(count: count, nonce: args[3], items: content)
        let encoder = PropertyListEncoder(); encoder.outputFormat = .binary
        try encoder.encode(snapshot).write(to: path, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: path.path)
        return
    }
    let bytes = try Data(contentsOf: path)
    guard bytes.count <= 40 * 1024 * 1024 else { throw Failure.bounds }
    let snapshot = try PropertyListDecoder().decode(Snapshot.self, from: bytes)
    let value = board.string(forType: .string)
    let ownValues = ["reasonix-native-clipboard-" + snapshot.nonce,
                     "reasonix-native-clipboard-" + snapshot.nonce + "-denied"]
    let marker = URL(fileURLWithPath: args[3])
    let owned = try? JSONSerialization.jsonObject(with: Data(contentsOf: marker)) as? [String: Any]
    if args[1] == "claim" {
        // Physical UI copy/cut has no native smoke receipt. Claim only this
        // unique fixture value and its unchanged system generation, without
        // writing the pasteboard or printing any original clipboard data.
        let count = board.changeCount
        guard board.string(forType: .string) == ownValues[0], try items(board).count == 1,
              board.changeCount == count else { throw Failure.changed }
        try JSONSerialization.data(withJSONObject: ["nonce": snapshot.nonce, "count": count])
            .write(to: marker, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: marker.path)
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
