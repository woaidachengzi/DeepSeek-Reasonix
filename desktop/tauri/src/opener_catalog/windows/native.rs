//! Win32 calls are isolated here. Handles and COM apartments never leave their
//! owning thread; only bounded PNG bytes and application paths are returned.
use super::{composites_rgba, png_data_url, registered_executable, Plan, Verb};
use std::{
    ffi::c_void,
    path::{Path, PathBuf},
    ptr::{null, null_mut},
};
use windows_sys::Win32::{
    Graphics::Gdi::{
        CreateCompatibleDC, CreateDIBSection, DeleteDC, DeleteObject, GdiFlush, GetDC, ReleaseDC,
        SelectObject, BITMAPINFO, BI_RGB, DIB_RGB_COLORS, HBITMAP, HDC, HGDIOBJ,
    },
    System::{
        Com::{CoInitializeEx, CoUninitialize, COINIT_APARTMENTTHREADED, COINIT_DISABLE_OLE1DDE},
        Environment::ExpandEnvironmentStringsW,
        Registry::{
            RegCloseKey, RegOpenKeyExW, RegQueryValueExW, HKEY, HKEY_CURRENT_USER,
            HKEY_LOCAL_MACHINE, KEY_QUERY_VALUE, KEY_WOW64_32KEY, KEY_WOW64_64KEY, REG_EXPAND_SZ,
            REG_SZ,
        },
    },
    UI::{
        Shell::{
            SHGetFileInfoW, ShellExecuteExW, SEE_MASK_FLAG_NO_UI, SEE_MASK_NOASYNC,
            SHELLEXECUTEINFOW, SHFILEINFOW, SHGFI_ICON, SHGFI_LARGEICON,
        },
        WindowsAndMessaging::{DestroyIcon, DrawIconEx, DI_NORMAL, HICON, SW_SHOWNORMAL},
    },
};

fn wide(text: &str) -> Option<Vec<u16>> {
    if text.contains('\0') {
        return None;
    }
    let mut text: Vec<_> = text.encode_utf16().collect();
    if text.len() >= 32768 {
        return None;
    }
    text.push(0);
    Some(text)
}
fn wide_path(path: &Path) -> Option<Vec<u16>> {
    wide(path.to_str()?)
}
struct RegistryKey(HKEY);
impl Drop for RegistryKey {
    fn drop(&mut self) {
        // Safety: this wrapper exclusively owns an opened registry key.
        unsafe {
            RegCloseKey(self.0);
        }
    }
}
struct Apartment;
impl Apartment {
    fn new() -> Option<Self> {
        // Safety: calls run on a new native worker, and every success has one matching uninitialize.
        let result = unsafe {
            CoInitializeEx(
                null(),
                (COINIT_APARTMENTTHREADED | COINIT_DISABLE_OLE1DDE) as u32,
            )
        };
        (result >= 0).then_some(Self)
    }
}
impl Drop for Apartment {
    fn drop(&mut self) {
        // Safety: this thread initialized COM successfully above.
        unsafe {
            CoUninitialize();
        }
    }
}

pub(crate) fn app_path(name: &str) -> Option<PathBuf> {
    if name.is_empty()
        || name
            .chars()
            .any(|c| c.is_control() || matches!(c, '/' | '\\'))
    {
        return None;
    }
    let subkey = wide(&format!(
        "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\{name}"
    ))?;
    for root in [HKEY_CURRENT_USER, HKEY_LOCAL_MACHINE] {
        for view in [KEY_WOW64_64KEY, KEY_WOW64_32KEY, 0] {
            if let Some(path) = read_registered_path(root, &subkey, view)
                .and_then(|raw| registered_executable(&raw))
            {
                return Some(path);
            }
        }
    }
    None
}
fn read_registered_path(root: HKEY, subkey: &[u16], view: u32) -> Option<String> {
    let mut handle = null_mut();
    // Safety: strings are NUL terminated, the output pointer is live, and only query access is requested.
    if unsafe {
        RegOpenKeyExW(
            root,
            subkey.as_ptr(),
            0,
            KEY_QUERY_VALUE | view,
            &mut handle,
        )
    } != 0
    {
        return None;
    }
    let key = RegistryKey(handle);
    let mut kind = 0;
    let mut size = 0;
    // Safety: a null data buffer queries the default value's byte length.
    if unsafe { RegQueryValueExW(key.0, null(), null(), &mut kind, null_mut(), &mut size) } != 0
        || size == 0
        || size > 65536
        || size % 2 != 0
        || ![REG_SZ, REG_EXPAND_SZ].contains(&kind)
    {
        return None;
    }
    let mut buffer = vec![0u16; size as usize / 2];
    // Safety: the allocation matches size bytes. A concurrently grown value returns an error and is rejected.
    if unsafe {
        RegQueryValueExW(
            key.0,
            null(),
            null(),
            &mut kind,
            buffer.as_mut_ptr().cast(),
            &mut size,
        )
    } != 0
        || size == 0
        || size as usize > buffer.len() * 2
        || size % 2 != 0
        || ![REG_SZ, REG_EXPAND_SZ].contains(&kind)
    {
        return None;
    }
    buffer.truncate(size as usize / 2);
    if buffer.last() == Some(&0) {
        buffer.pop();
    }
    let raw = String::from_utf16(&buffer).ok()?;
    if kind != REG_EXPAND_SZ {
        return Some(raw);
    }
    let source = wide(&raw)?;
    // Safety: source is a bounded NUL terminated UTF-16 string; this call requests required capacity.
    let length = unsafe { ExpandEnvironmentStringsW(source.as_ptr(), null_mut(), 0) };
    if length == 0 || length > 32768 {
        return None;
    }
    let mut expanded = vec![0u16; length as usize];
    // Safety: destination capacity equals the advertised capacity. Growth is detected and rejected.
    let written =
        unsafe { ExpandEnvironmentStringsW(source.as_ptr(), expanded.as_mut_ptr(), length) };
    if written == 0 || written > length {
        return None;
    }
    expanded.truncate(written as usize);
    if expanded.last() == Some(&0) {
        expanded.pop();
    }
    String::from_utf16(&expanded).ok()
}

pub(crate) fn launch(plan: Plan) -> Result<(), String> {
    match plan {
        Plan::Process { target, args, dir } => {
            let mut child = std::process::Command::new(target).args(args).current_dir(dir)
                .stdin(std::process::Stdio::null()).stdout(std::process::Stdio::null()).stderr(std::process::Stdio::null())
                .spawn().map_err(|_| "selected application could not start; check installation and retry")?;
            std::thread::spawn(move || { let _ = child.wait(); }); Ok(())
        }
        Plan::Shell { file, dir, verb } => std::thread::spawn(move || {
            let _apartment = Apartment::new().ok_or("native application launch is unavailable; restart Preview and retry")?;
            let file = wide_path(&file).ok_or("application path is invalid; select another application")?;
            let dir = wide_path(&dir).ok_or("workspace path is invalid; choose another workspace")?;
            let verb = wide(match verb { Verb::Open => "open", Verb::Explore => "explore" }).unwrap();
            let mut info = SHELLEXECUTEINFOW { cbSize: std::mem::size_of::<SHELLEXECUTEINFOW>() as u32,
                fMask: SEE_MASK_FLAG_NO_UI | SEE_MASK_NOASYNC, lpVerb: verb.as_ptr(),
                lpFile: file.as_ptr(), lpDirectory: dir.as_ptr(), nShow: SW_SHOWNORMAL,
                ..Default::default() };
            // Safety: strings live until synchronous submission finishes. No
            // parameter string, cmd /c, start, elevation or process handle is requested.
            let result = unsafe { ShellExecuteExW(&mut info) };
            if result == 0 { Err("selected application rejected the launch; check its installation or file association, then retry".to_string()) } else { Ok(()) }
        }).join().map_err(|_| "native application launch failed; restart Preview and retry")?,
    }
}

struct Icon(HICON);
impl Drop for Icon {
    fn drop(&mut self) {
        // Safety: SHGetFileInfo transferred this icon's ownership to the wrapper.
        unsafe {
            DestroyIcon(self.0);
        }
    }
}
struct Screen(HDC);
impl Drop for Screen {
    fn drop(&mut self) {
        // Safety: DC was acquired for the null screen window on this thread.
        unsafe {
            ReleaseDC(null_mut(), self.0);
        }
    }
}
struct Memory(HDC);
impl Drop for Memory {
    fn drop(&mut self) {
        // Safety: this is an owned compatible memory DC.
        unsafe {
            DeleteDC(self.0);
        }
    }
}
struct Bitmap(HBITMAP);
impl Drop for Bitmap {
    fn drop(&mut self) {
        // Safety: the selection guard has already restored the previous object.
        unsafe {
            DeleteObject(self.0);
        }
    }
}
struct Selection {
    dc: HDC,
    previous: HGDIOBJ,
}
impl Drop for Selection {
    fn drop(&mut self) {
        // Safety: DC and previous object still exist when this guard drops.
        unsafe {
            SelectObject(self.dc, self.previous);
        }
    }
}

const SIZE: i32 = 32;
pub(crate) fn icon_data_url(path: &Path) -> Option<String> {
    let path = path.to_path_buf();
    std::thread::spawn(move || {
        let _apartment = Apartment::new()?;
        let path = wide_path(&path)?;
        let mut info = SHFILEINFOW::default();
        // Safety: the structure's exact byte size and live output storage are supplied.
        let result = unsafe {
            SHGetFileInfoW(
                path.as_ptr(),
                0,
                &mut info,
                std::mem::size_of::<SHFILEINFOW>() as u32,
                SHGFI_ICON | SHGFI_LARGEICON,
            )
        };
        if info.hIcon.is_null() {
            return None;
        }
        let icon = Icon(info.hIcon);
        if result == 0 {
            return None;
        }
        let black = render(&icon, 0)?;
        let white = render(&icon, 255)?;
        png_data_url(
            &composites_rgba(&black, &white, SIZE as u32, SIZE as u32)?,
            SIZE as u32,
            SIZE as u32,
        )
    })
    .join()
    .ok()
    .flatten()
}
fn render(icon: &Icon, background: u8) -> Option<Vec<u8>> {
    // Safety: every acquired handle has a scope guard; bitmap storage remains
    // valid until pixels are copied, and selected objects are restored first.
    unsafe {
        let screen = GetDC(null_mut());
        if screen.is_null() {
            return None;
        }
        let screen = Screen(screen);
        let memory = CreateCompatibleDC(screen.0);
        if memory.is_null() {
            return None;
        }
        let memory = Memory(memory);
        let mut info = BITMAPINFO::default();
        info.bmiHeader.biSize = std::mem::size_of_val(&info.bmiHeader) as u32;
        info.bmiHeader.biWidth = SIZE;
        info.bmiHeader.biHeight = -SIZE;
        info.bmiHeader.biPlanes = 1;
        info.bmiHeader.biBitCount = 32;
        info.bmiHeader.biCompression = BI_RGB;
        let mut bits: *mut c_void = null_mut();
        let bitmap = CreateDIBSection(memory.0, &info, DIB_RGB_COLORS, &mut bits, null_mut(), 0);
        if bitmap.is_null() {
            return None;
        }
        let bitmap = Bitmap(bitmap);
        if bits.is_null() {
            return None;
        }
        let previous = SelectObject(memory.0, bitmap.0);
        if previous.is_null() || previous as isize == -1 {
            return None;
        }
        let _selection = Selection {
            dc: memory.0,
            previous,
        };
        {
            let pixels = std::slice::from_raw_parts_mut(
                bits.cast::<u8>(),
                SIZE as usize * SIZE as usize * 4,
            );
            for pixel in pixels.chunks_exact_mut(4) {
                pixel.copy_from_slice(&[background, background, background, 255]);
            }
        }
        if DrawIconEx(memory.0, 0, 0, icon.0, SIZE, SIZE, 0, null_mut(), DI_NORMAL) == 0
            || GdiFlush() == 0
        {
            return None;
        }
        Some(
            std::slice::from_raw_parts(bits.cast::<u8>(), SIZE as usize * SIZE as usize * 4)
                .to_vec(),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use windows_sys::Win32::System::Registry::{
        RegCreateKeyExW, RegDeleteKeyExW, RegSetValueExW, KEY_SET_VALUE, REG_CREATED_NEW_KEY,
        REG_OPTION_NON_VOLATILE,
    };

    struct TestRegistration {
        key: RegistryKey,
        subkey: Vec<u16>,
    }
    impl Drop for TestRegistration {
        fn drop(&mut self) {
            // Safety: this test created this unique key and only removes it.
            // The owned handle closes immediately after the deletion request.
            unsafe {
                RegDeleteKeyExW(HKEY_CURRENT_USER, self.subkey.as_ptr(), KEY_WOW64_64KEY, 0);
            }
        }
    }

    #[test]
    fn native_app_paths_expands_registered_environment_and_preserves_registry_after_cleanup() {
        let temp =
            PathBuf::from(std::env::var_os("TEMP").expect("Windows runner must provide TEMP"));
        let root = tempfile::tempdir_in(temp).unwrap();
        let file = root.path().join("Custom Editor.exe");
        std::fs::write(&file, b"test fixture").unwrap();
        let leaf = root.path().file_name().unwrap().to_str().unwrap();
        let name = format!("reasonix-opener-test-{leaf}.exe");
        let subkey = wide(&format!(
            "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\{name}"
        ))
        .unwrap();
        let mut handle = null_mut();
        let mut disposition = 0;
        // Safety: writes are limited to a new, unique HKCU test key. Existing
        // keys are refused before any value is changed; no administrator access.
        let result = unsafe {
            RegCreateKeyExW(
                HKEY_CURRENT_USER,
                subkey.as_ptr(),
                0,
                null(),
                REG_OPTION_NON_VOLATILE,
                KEY_SET_VALUE | KEY_QUERY_VALUE | KEY_WOW64_64KEY,
                null(),
                &mut handle,
                &mut disposition,
            )
        };
        assert_eq!(result, 0);
        let key = RegistryKey(handle);
        assert_eq!(
            disposition, REG_CREATED_NEW_KEY,
            "refuse to overwrite an existing registry key"
        );
        let registration = TestRegistration { key, subkey };
        let value = wide(&format!("\"%TEMP%\\{leaf}\\Custom Editor.exe\"")).unwrap();
        // Safety: a live bounded UTF-16 buffer is written to the owned test key only.
        let result = unsafe {
            RegSetValueExW(
                registration.key.0,
                null(),
                0,
                REG_EXPAND_SZ,
                value.as_ptr().cast(),
                (value.len() * 2) as u32,
            )
        };
        assert_eq!(result, 0);
        let found = app_path(&name).expect("registered custom path must resolve");
        assert_eq!(found.canonicalize().unwrap(), file.canonicalize().unwrap());
        drop(registration);
        assert!(app_path(&name).is_none());
    }

    #[test]
    fn native_explorer_icon_is_bounded_png() {
        let root = std::env::var_os("WINDIR").expect("Windows runner must provide WINDIR");
        let path = PathBuf::from(root).join("explorer.exe");
        assert!(path.is_file(), "native Explorer binary must exist");
        let image = icon_data_url(&path).expect("Explorer icon should render");
        assert!(image.starts_with("data:image/png;base64,"));
        assert!(image.len() < 90 * 1024);
    }
    #[test]
    fn shell_execute_failure_is_reported_without_launching_console_or_showing_paths() {
        let missing = PathBuf::from(std::env::var_os("WINDIR").unwrap())
            .join("reasonix-nonexistent-opener-test.exe");
        assert!(!missing.exists());
        let plan = Plan::Shell {
            file: missing,
            dir: std::env::temp_dir(),
            verb: Verb::Open,
        };
        let error = launch(plan).unwrap_err();
        assert!(error.contains("check its installation"));
        assert!(!error.contains("reasonix-nonexistent"));
    }
}
