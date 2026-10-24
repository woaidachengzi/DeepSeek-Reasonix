// Explicit opt-in fixture: creates only its own window; never posts input.
#[cfg(target_os = "macos")]
#[allow(dead_code)]
#[path = "../src/computer_use_cursor.rs"]
mod computer_use_cursor;
#[cfg(target_os = "macos")]
#[allow(dead_code)]
#[path = "../src/protocol_generated.rs"]
mod protocol_generated;
#[cfg(target_os = "macos")]
fn main() {
    let output = std::env::args()
        .nth(1)
        .expect("supply own-view PNG output path");
    computer_use_cursor::acceptance_fixture(&output).expect("native cursor acceptance failed");
}

#[cfg(not(target_os = "macos"))]
fn main() {
    println!("The whale cursor native fixture is macOS-only.");
}
