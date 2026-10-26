use sha2::{Digest, Sha256};

fn start_ticks(stat: &str) -> Result<u64, String> {
    let (_, fields) = stat.rsplit_once(')').ok_or("invalid Linux process stat")?;
    let fields: Vec<_> = fields.split_whitespace().collect();
    if fields.len() < 20 || matches!(fields[0], "Z" | "X" | "x") {
        return Err("Linux process unavailable".into());
    }
    let ticks = fields[19]
        .parse::<u64>()
        .map_err(|_| "invalid start time")?;
    if ticks == 0 || ticks > (1u64 << 53) - 1 {
        return Err("invalid Linux process start time".into());
    }
    Ok(ticks)
}

fn executable_identity(
    boot: &str,
    namespace: &str,
    uid: u64,
    dev: u64,
    ino: u64,
    executable: &[u8],
) -> String {
    let hex_path: String = executable.iter().map(|b| format!("{b:02x}")).collect();
    let canonical = format!(
        "reasonix-linux-process-v1\n{}\n{namespace}\n{uid}\n{dev}\n{ino}\n{hex_path}\n",
        boot.trim()
    );
    format!("linux:{:x}", Sha256::digest(canonical.as_bytes()))
}

#[cfg(target_os = "linux")]
pub fn resolve(pid: i32) -> Result<(String, u64), String> {
    use std::{
        fs,
        os::{
            fd::AsRawFd,
            unix::{ffi::OsStrExt, fs::MetadataExt},
        },
    };
    let read = || -> Result<(String, u64), Box<dyn std::error::Error>> {
        if pid < 1 {
            return Err("invalid Linux app PID".into());
        }
        let process = fs::File::open(format!("/proc/{pid}"))?;
        let uid = process.metadata()?.uid();
        if uid != fs::metadata("/proc/self")?.uid() {
            return Err("Linux app must belong to the current desktop user".into());
        }
        // Pin the same process directory used by the Go host, not a reusable PID.
        let root = format!("/proc/self/fd/{}", process.as_raw_fd());
        let ticks = start_ticks(&fs::read_to_string(format!("{root}/stat"))?)?;
        let executable = fs::read_link(format!("{root}/exe"))?;
        let path = executable.as_os_str().as_bytes();
        if !executable.is_absolute() || path.ends_with(b" (deleted)") {
            return Err("Linux app executable unavailable".into());
        }
        let meta = fs::metadata(format!("{root}/exe"))?;
        if !meta.is_file() {
            return Err("invalid Linux executable".into());
        }
        let boot = fs::read_to_string("/proc/sys/kernel/random/boot_id")?;
        if boot.trim().is_empty() {
            return Err("Linux boot identity unavailable".into());
        }
        let namespace = fs::read_link(format!("{root}/ns/pid"))?;
        let namespace = namespace.to_str().ok_or("invalid PID namespace")?;
        let again = start_ticks(&fs::read_to_string(format!("{root}/stat"))?)?;
        let current = fs::metadata(format!("{root}/exe"))?;
        if again != ticks
            || current.dev() != meta.dev()
            || current.ino() != meta.ino()
            || fs::read_link(format!("{root}/exe"))? != executable
        {
            return Err("Linux app changed during identity read".into());
        }
        Ok((
            executable_identity(&boot, namespace, uid.into(), meta.dev(), meta.ino(), path),
            ticks,
        ))
    };
    read().map_err(|e| format!("approved Linux app unavailable: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn identity_contract_matches_go_host() {
        let cases: serde_json::Value = serde_json::from_str(include_str!(
            "../../../../internal/computeruse/testdata/linux_identity.json"
        ))
        .unwrap();
        for case in cases["stat"].as_array().unwrap() {
            assert_eq!(
                start_ticks(case["text"].as_str().unwrap()).ok(),
                case["ticks"].as_u64(),
                "{}",
                case["name"]
            );
        }
        let f = &cases["fingerprint"];
        assert_eq!(
            executable_identity(
                f["boot"].as_str().unwrap(),
                f["namespace"].as_str().unwrap(),
                f["uid"].as_u64().unwrap(),
                f["dev"].as_u64().unwrap(),
                f["ino"].as_u64().unwrap(),
                f["exe"].as_str().unwrap().as_bytes()
            ),
            f["expected"].as_str().unwrap()
        );
    }
    #[cfg(target_os = "linux")]
    #[test]
    fn verifies_live_process_and_rejects_changed_grant() {
        let pid = std::process::id() as i32;
        let (bundle_id, ticks) = resolve(pid).unwrap();
        let mut target = crate::request::Target {
            pid,
            name: "fixture".into(),
            bundle_id,
            launch_time: ticks as f64,
        };
        assert!(crate::identity::verify(&target).is_ok());
        target.launch_time += 1.;
        assert!(crate::identity::verify(&target).is_err());
        target.launch_time = ticks as f64;
        target.bundle_id.push('x');
        assert!(crate::identity::verify(&target).is_err());
        assert!(resolve(0).is_err());
    }
}
