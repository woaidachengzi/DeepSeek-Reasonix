#!/usr/bin/env python3
"""Actual packaged message render -> programmatic context menu -> OS clipboard.

Private fake provider, exact original clipboard backup and owned generation.
Does not substitute for physical pointer/keyboard, IME or denial UI acceptance.
"""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
from pathlib import Path
import plistlib
import shutil
import sys
import tempfile
import threading
import uuid

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def smoke(app_path):
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview already running")
    for managed in (True, False):
        root = Path(tempfile.mkdtemp(prefix="reasonix-native-message-copy-", dir="/private/tmp"))
        root.chmod(0o700)
        for name in ("home", "tmp"):
            (root / name).mkdir(mode=0o700)
        core = root / "home/Library/Application Support" / identifier / "reasonix-core" if managed else root / "core"
        core.mkdir(parents=True, mode=0o700)
        nonce = uuid.uuid4().hex
        expected = "reasonix-native-clipboard-" + nonce
        state = {"requests": 0, "error": False}

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                try:
                    self.connection.settimeout(10)
                    length = int(self.headers.get("Content-Length", "0"))
                    if self.path != "/v1/chat/completions" or not 0 < length <= 2 << 20:
                        raise RuntimeError("unexpected request")
                    body = json.loads(self.rfile.read(length))
                    state["requests"] += 1
                    if state["requests"] != 1 or body.get("model") != "alpha" or body.get("stream") is not True or not any(
                        message.get("role") == "user" and message.get("content") == expected
                        for message in body.get("messages", []) if isinstance(message, dict)
                    ):
                        raise RuntimeError("unexpected prompt/retry")
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.end_headers()
                    for delta, finish in (({"role": "assistant", "content": expected}, None), ({}, "stop")):
                        self.wfile.write(("data: " + json.dumps({"choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}) + "\n\n").encode())
                    self.wfile.write(b"data: [DONE]\n\n")
                    self.wfile.flush()
                except Exception:
                    state["error"] = True
                finally:
                    self.close_connection = True

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        server.daemon_threads = True
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        config = core / "config.toml"
        config.touch(mode=0o600)
        config.write_text('default_model = "local/alpha"\n[desktop]\nprovider_access = ["local"]\n[[providers]]\nname = "local"\nkind = "openai"\n'
                          f'base_url = "http://127.0.0.1:{server.server_port}/v1"\nmodels = ["alpha"]\ndefault = "alpha"\n')
        thread.start()
        success = False
        try:
            with windows.clipboard_fixture.NativeClipboardFixture(root / "tmp", nonce=nonce) as fixture:
                windows.launch(app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge",
                               root, identifier, managed, "ui-message-copy", fixture,
                               verify_window_state=False, launch_services=True)
                if state != {"requests": 1, "error": False}:
                    raise RuntimeError("owned provider request count or response failed")
            success = True
            print(f"Actual {'managed' if managed else 'explicit'} rendered message/menu/OS text, one provider call and clipboard restoration: OK", flush=True)
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=3)
            if success:
                shutil.rmtree(root)
            else:
                print(f"Failed private message fixture retained: {root}", file=sys.stderr, flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("macOS required")
    smoke(args.app)
