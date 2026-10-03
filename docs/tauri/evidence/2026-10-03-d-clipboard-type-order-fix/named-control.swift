import AppKit
import Foundation
let board=NSPasteboard(name:NSPasteboard.Name("io.reasonix.order-control."+UUID().uuidString))
defer { board.releaseGlobally() }
let entries:[(NSPasteboard.PasteboardType,Data)]=[(.init("com.apple.notes.richtext"),Data("dummy".utf8)),(.rtf,Data("{\\rtf1 dummy}".utf8)),(.string,Data("dummy".utf8)),(.init("public.utf16-external-plain-text"),"dummy".data(using:.utf16)!)]
let expected=entries.map{$0.0.rawValue}
func state(_ label:String){print(label+": "+String(data:try! JSONSerialization.data(withJSONObject:board.pasteboardItems!.map{$0.types.map{$0.rawValue}}),encoding:.utf8)!)}
let item=NSPasteboardItem();for (type,bytes) in entries{assert(item.setData(bytes,forType:type))};print("in-memory: \(item.types.map{$0.rawValue})")
board.clearContents();assert(board.writeObjects([item]));state("writeObjects")
board.declareTypes(entries.map{$0.0},owner:nil);for(type,bytes)in entries{assert(board.setData(bytes,forType:type))};state("declareTypes+setData")
board.clearContents();board.addTypes(entries.map{$0.0},owner:nil);for(type,bytes)in entries{assert(board.setData(bytes,forType:type))};state("addTypes+setData")
