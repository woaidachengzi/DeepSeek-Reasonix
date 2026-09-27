fn main() {
    println!("cargo:rerun-if-env-changed=REASONIX_PREVIEW_COMMIT");
    println!("cargo:rerun-if-env-changed=REASONIX_PREVIEW_DIRTY");
    let commit = std::env::var("REASONIX_PREVIEW_COMMIT")
        .ok()
        .filter(|value| {
            (value.len() == 40 || value.len() == 64)
                && value.bytes().all(|byte| byte.is_ascii_hexdigit())
        })
        .unwrap_or_else(|| "unknown".to_string());
    println!("cargo:rustc-env=REASONIX_PREVIEW_COMMIT={commit}");
    let dirty = if std::env::var("REASONIX_PREVIEW_DIRTY").as_deref() == Ok("1") {
        "1"
    } else {
        "0"
    };
    println!("cargo:rustc-env=REASONIX_PREVIEW_DIRTY={dirty}");
    tauri_build::build()
}
