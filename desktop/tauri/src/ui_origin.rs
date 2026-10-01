//! Profile-bound origin for the packaged macOS UI. WKWebView's default store
//! does not follow a substituted HOME; different core profiles need different
//! origins before any frontend code can read persistent browser state.
use std::path::Path;

use tauri::{AppHandle, Manager, WebviewUrl};
use url::Url;

pub const SCHEME: &str = "reasonix-preview";

#[derive(Clone)]
pub struct PreviewUiOrigin {
    url: Url,
}

fn profile_url(id: &str) -> Result<Url, String> {
    if id.len() != 32
        || !id
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err("UI profile identity is invalid; restore the profile metadata backup".into());
    }
    Url::parse(&format!("{SCHEME}://{id}.localhost/"))
        .map_err(|_| "UI profile origin could not be created".into())
}

impl PreviewUiOrigin {
    pub fn for_profile(home: &Path) -> Result<Self, String> {
        let service = crate::credential_namespace::service_for_profile(home)?;
        let id = service
            .strip_prefix("com.reasonix.desktop.profile.")
            .ok_or("UI profile identity could not be resolved")?;
        Ok(Self {
            url: profile_url(id)?,
        })
    }

    pub fn webview_url(&self) -> WebviewUrl {
        WebviewUrl::CustomProtocol(self.url.clone())
    }

    pub(crate) fn matches(&self, url: &Url) -> bool {
        url.scheme() == self.url.scheme()
            && url.host_str() == self.url.host_str()
            && url.port().is_none()
            && url.username().is_empty()
            && url.password().is_none()
    }
}

pub fn matches(app: &AppHandle, url: &Url) -> bool {
    app.try_state::<PreviewUiOrigin>()
        .is_some_and(|origin| origin.matches(url))
}

pub fn webview_url(app: &AppHandle) -> Result<WebviewUrl, String> {
    app.try_state::<PreviewUiOrigin>()
        .map(|origin| origin.webview_url())
        .ok_or("UI profile origin is unavailable".into())
}

/// Serve only compiled application assets for this profile's own authority.
/// The public Tauri resolver retains its MIME inference and nonce/CSP handling.
/// No user filesystem, external proxy, or wider capability is exposed.
pub fn asset_response(
    app: &AppHandle,
    request: tauri::http::Request<Vec<u8>>,
) -> tauri::http::Response<Vec<u8>> {
    use tauri::http::{header, Method, Response, StatusCode};
    let rejected = || {
        Response::builder()
            .status(StatusCode::FORBIDDEN)
            .body(Vec::new())
            .unwrap()
    };
    if request.method() != Method::GET && request.method() != Method::HEAD {
        return rejected();
    }
    if request.uri().to_string().len() > 32_768 {
        return rejected();
    }
    let Ok(url) = Url::parse(&request.uri().to_string()) else {
        return rejected();
    };
    if !matches(app, &url) {
        return rejected();
    }
    let Some(asset) = app
        .asset_resolver()
        .get_for_scheme(url.path().to_string(), false)
    else {
        return Response::builder()
            .status(StatusCode::NOT_FOUND)
            .body(Vec::new())
            .unwrap();
    };
    let mut builder = Response::builder().header(header::CONTENT_TYPE, asset.mime_type);
    if let Some(csp) = asset.csp_header {
        builder = builder.header(header::CONTENT_SECURITY_POLICY, csp);
    }
    builder
        .body(if request.method() == Method::HEAD {
            Vec::new()
        } else {
            asset.bytes
        })
        .unwrap_or_else(|_| rejected())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn origin_is_distinct_durable_and_preserves_profile_identity_on_moves() {
        let root = tempfile::tempdir().unwrap();
        let first = root.path().join("first");
        let other = root.path().join("other");
        let a = PreviewUiOrigin::for_profile(&first).unwrap();
        let b = PreviewUiOrigin::for_profile(&other).unwrap();
        assert_ne!(a.url, b.url);
        assert_eq!(a.url, PreviewUiOrigin::for_profile(&first).unwrap().url);
        let moved = root.path().join("moved");
        std::fs::rename(first, &moved).unwrap();
        assert_eq!(a.url, PreviewUiOrigin::for_profile(&moved).unwrap().url);
        assert!(a.matches(&a.url.join("assets/app.js?native_result=ok").unwrap()));
        assert!(!a.matches(&b.url));
        assert!(!a.matches(&Url::parse("tauri://localhost/").unwrap()));
        let mut forged = a.url.clone();
        forged.set_username("foreign").unwrap();
        assert!(!a.matches(&forged));
        let mut forged = a.url.clone();
        forged.set_port(Some(80)).unwrap();
        assert!(!a.matches(&forged));
    }

    #[test]
    fn malformed_identity_cannot_become_a_url_authority() {
        for id in [
            "",
            "../shared",
            "localhost",
            "0000000000000000000000000000000g",
        ] {
            assert!(profile_url(id).is_err());
        }
    }
}
