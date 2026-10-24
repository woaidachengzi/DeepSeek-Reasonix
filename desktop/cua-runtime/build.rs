fn main() {
    let pin: serde_json::Value =
        serde_json::from_str(include_str!("upstream.json")).expect("valid upstream pin");
    let revision = pin["revision"].as_str().expect("source revision");
    assert!(revision.len() == 40 && revision.bytes().all(|b| b.is_ascii_hexdigit()));
    println!("cargo:rerun-if-changed=upstream.json");
    println!("cargo:rustc-env=REASONIX_CUA_SOURCE_REVISION={revision}");
    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("macos") {
        // ScreenCaptureKit's pinned apple-metal dependency links a Swift
        // bridge. Its rlib cannot propagate a final executable's LC_RPATH.
        println!("cargo:rustc-link-arg=-Wl,-rpath,/usr/lib/swift");
    }
}
