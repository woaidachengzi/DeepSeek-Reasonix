#!/usr/bin/env python3
"""Actual macOS Preview WKWebView IPC -> default browser loopback acceptance.

Opens two harmless local canary pages per private profile. Browser tabs may be
closed manually afterward; no existing tab/application is closed. Does not prove
physical clicks, mail composition, OAuth authentication or rendered page layout.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import secrets
import shutil
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location(
    "native_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


class BrowserReceipts:
    def __init__(self, temporary):
        nonce = secrets.token_hex(16)
        self.seen = set()
        self.errors = []
        owner = self
        commands = {"open_external_link", "open_external_url"}

        class Handler(BaseHTTPRequestHandler):
            def setup(self):
                super().setup()
                self.connection.settimeout(3)

            def log_message(self, *_):
                pass

            def do_GET(self):
                command = self.path.removeprefix(f"/{nonce}/")
                if command not in commands or self.path != f"/{nonce}/{command}":
                    self.send_response(204 if self.path == "/favicon.ico" else 404)
                    self.end_headers()
                    return
                if "Mozilla/" not in self.headers.get("User-Agent", ""):
                    owner.errors.append("loopback receipt lacks a browser user agent")
                    self.send_error(400)
                    return
                body = ("<!doctype html><meta charset=utf-8><title>Reasonix 浏览器验收</title>"
                        "<h1>Reasonix 浏览器打开链路已到达</h1>"
                        "<p>这是本机临时验收页，可以关闭此标签页。</p>").encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Content-Security-Policy", "default-src 'none'")
                self.send_header("Cache-Control", "no-store")
                self.end_headers()
                self.wfile.write(body)
                self.wfile.flush()
                receipt = temporary / f"reasonix-native-link-{command}.receipt"
                receipt.write_text(nonce)
                receipt.chmod(0o600)
                owner.seen.add(command)

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        self.server.timeout = 3
        control = temporary / "reasonix-native-link-control.json"
        control.write_text(json.dumps({"nonce": nonce, "port": self.server.server_port}))
        control.chmod(0o600)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        return self

    def pump(self, _inspect):
        # Receipts are handled by the server thread. Unlike a streaming task,
        # links may finish and exit between the runner's poll and a live check.
        # launch() still requires the result, normal exit and complete cleanup.
        pass

    def verify(self):
        if self.errors or self.seen != {"open_external_link", "open_external_url"}:
            raise RuntimeError("both actual browser receipts were not verified")

    def __exit__(self, *_):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


def smoke(app_path):
    app = Path(app_path).resolve()
    host = app / "Contents/MacOS/reasonix-tauri"
    sidecar = app / "Contents/MacOS/reasonix-desktop-bridge"
    if not host.is_file() or not sidecar.is_file():
        raise RuntimeError("packaged host/sidecar missing")
    with (app / "Contents/Info.plist").open("rb") as file:
        identifier = plistlib.load(file).get("CFBundleIdentifier")
    if not isinstance(identifier, str) or not identifier:
        raise RuntimeError("bundle identifier missing")
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("a Preview with this identifier is running; close it first")
    subprocess.run(["codesign", "--verify", "--deep", "--strict", str(app)], check=True)
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-native-links-", dir="/private/tmp"))
        root.chmod(0o700)
        (root / "home").mkdir(mode=0o700)
        (root / "tmp").mkdir(mode=0o700)
        success = False
        try:
            with BrowserReceipts(root / "tmp") as receipts:
                # A link acceptance has no geometry transitions. Keep the
                # existing window gate's geometry assertions unchanged.
                windows.launch(host, sidecar, root, identifier, managed,
                               "external-browser", receipts, verify_window_state=False,
                               environment={key: os.environ[key] for key in
                                            ("PATH", "USER", "LOGNAME", "LANG", "LC_ALL",
                                             "__CF_USER_TEXT_ENCODING") if key in os.environ})
            success = True
        finally:
            if success:
                shutil.rmtree(root)
            else:
                print(f"Private failure fixture retained: {root}", file=sys.stderr)
    print("Main WKWebView link validation, default browser receipts and cleanup: OK")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("this acceptance requires macOS")
    try:
        smoke(args.app)
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"native macOS link smoke failed: {error}") from error
