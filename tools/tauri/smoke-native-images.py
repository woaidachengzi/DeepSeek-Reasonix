#!/usr/bin/env python3
"""Packaged WKWebView native Paste -> real Go image input -> persisted reopen.

Uses installed AppKit Paste, not a synthetic ClipboardEvent or physical Cmd+V.
Only fresh private profiles, an owned PNG/TIFF and a bounded loopback model are
used. The clipboard guard restores all original formats unless ownership changes.
"""
import argparse
import base64
import binascii
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import struct
import sys
import tempfile
import threading
import uuid
import zlib

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("image_windows", Path(__file__).with_name("smoke-native-window.py"))
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)


def validate_request(body, nonce):
    """Return only an owned attachment path and image digest; never log payloads."""
    if not isinstance(body, dict) or body.get("model") != "alpha" or body.get("stream") is not True:
        raise ValueError("unexpected image model request")
    messages = body.get("messages")
    if not isinstance(messages, list):
        raise ValueError("missing image messages")
    users = [message.get("content") for message in messages if isinstance(message, dict) and message.get("role") == "user"]
    if not users or not isinstance(users[-1], list):
        raise ValueError("model did not receive multimodal user input")
    parts = users[-1]
    text = "\n".join(part.get("text", "") for part in parts if isinstance(part, dict) and part.get("type") == "text")
    images = [part.get("image_url") for part in parts if isinstance(part, dict) and part.get("type") == "image_url"]
    paths = set(re.findall(r"\.reasonix/attachments/[A-Za-z0-9_.-]+\.png", text))
    if f"reasonix-native-image-{nonce}" not in text:
        raise ValueError("owned image prompt missing")
    if len(paths) != 1:
        raise ValueError("owned image attachment path count differs")
    if len(images) != 1:
        raise ValueError("owned image payload count differs")
    value = images[0].get("url") if isinstance(images[0], dict) else None
    prefix = "data:image/png;base64,"
    if not isinstance(value, str) or not value.startswith(prefix) or len(value) > 65536:
        raise ValueError("model image is not the bounded PNG fixture")
    try:
        data = base64.b64decode(value[len(prefix):], validate=True)
    except (ValueError, binascii.Error):
        raise ValueError("invalid fixture image encoding") from None
    if len(data) < 33 or data[:8] != b"\x89PNG\r\n\x1a\n" or data[8:16] != b"\x00\x00\x00\rIHDR":
        raise ValueError("model image is not PNG")
    width, height, bits, color, compression, filtering, interlace = struct.unpack(">IIBBBBB", data[16:29])
    if (width, height, bits, compression, filtering, interlace) != (64, 40, 8, 0, 0, 0) or color not in (2, 6):
        raise ValueError("model fixture dimensions or pixel format differ")
    # Validate every chunk CRC and bounded decompression, not only a forged IHDR.
    offset, compressed, ended = 8, bytearray(), False
    while offset < len(data):
        if offset + 12 > len(data):
            raise ValueError("truncated fixture PNG")
        size = struct.unpack(">I", data[offset:offset + 4])[0]
        end = offset + size + 12
        if end > len(data):
            raise ValueError("truncated fixture PNG chunk")
        kind = data[offset + 4:offset + 8]
        payload = data[offset + 8:end - 4]
        if zlib.crc32(kind + payload) != struct.unpack(">I", data[end - 4:end])[0]:
            raise ValueError("invalid fixture PNG checksum")
        if kind == b"IDAT":
            compressed.extend(payload)
        if kind == b"IEND":
            ended = size == 0 and end == len(data)
            break
        offset = end
    length = 40 * (1 + 64 * (4 if color == 6 else 3))
    decoder = zlib.decompressobj()
    try:
        pixels = decoder.decompress(compressed, length + 1)
    except zlib.error:
        raise ValueError("invalid fixture PNG pixels") from None
    if not ended or len(pixels) != length or not decoder.eof or decoder.unconsumed_tail or decoder.unused_data:
        raise ValueError("model PNG pixel budget or stream differs")
    return paths.pop(), hashlib.sha256(data).hexdigest()


class ImageProvider:
    def __init__(self, nonce):
        self.nonce, self.requests, self.digest, self.path, self.error = nonce, 0, None, None, None
        self.completed = threading.Event()
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                try:
                    self.connection.settimeout(10)
                    length = int(self.headers.get("Content-Length", "0"))
                    if self.path != "/v1/chat/completions" or not 0 < length <= 2 << 20:
                        raise ValueError("unexpected image provider route or length")
                    fixture.requests += 1
                    if fixture.requests != 1:
                        raise ValueError("unexpected image provider resubmit")
                    body = json.loads(self.rfile.read(length))
                    try:
                        fixture.path, fixture.digest = validate_request(body, fixture.nonce)
                    except ValueError as error:
                        # validate_request raises only the fixed strings above;
                        # no body/header/socket diagnostic is retained.
                        fixture.error = fixture.error or str(error)
                        return
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.send_header("Connection", "close")
                    self.end_headers()
                    response = f"native-image-accepted\n\n![截图]({fixture.path})"
                    for delta, finish in (({"role": "assistant", "content": response}, None), ({}, "stop")):
                        self.wfile.write(("data: " + json.dumps({"choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}) + "\n\n").encode())
                    self.wfile.write(b"data: [DONE]\n\n")
                    self.wfile.flush()
                    fixture.completed.set()
                except Exception:
                    # No request text, headers, image data or socket error escapes.
                    fixture.error = fixture.error or "image-provider-failed"
                finally:
                    self.close_connection = True

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def verify(self):
        if self.error or self.requests != 1 or not self.digest or not self.path or not self.completed.is_set():
            raise RuntimeError("image provider rejected input, did not receive an image, or observed resubmit")

    def failure_status(self):
        return {"requests": min(self.requests, 10), "error": self.error,
                "imageAccepted": bool(self.digest), "replyCompleted": self.completed.is_set()}

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)


def smoke(app_path, template_path, profile, image_format):
    app = Path(app_path).resolve()
    identifier = plistlib.loads((app / "Contents/Info.plist").read_bytes())["CFBundleIdentifier"]
    if windows.package.matching_package_is_running(identifier):
        raise RuntimeError("Preview already running; isolated image smoke refused")
    placement = windows.package.placement_helper()
    geometry = placement.read_window_state_template(Path(template_path))
    if geometry["x"] >= 0:
        raise ValueError("image smoke requires a left-display window template")
    environment = {key: os.environ[key] for key in ("PATH", "LANG", "LC_ALL", "USER", "LOGNAME") if key in os.environ}
    formats = ("png", "tiff") if image_format == "both" else (image_format,)
    profiles = ("managed", "explicit") if profile == "both" else (profile,)
    evidence = Path(tempfile.mkdtemp(prefix="reasonix-native-image-evidence-", dir="/private/tmp"))
    evidence.chmod(0o700)
    for format_name in formats:
        for mode in profiles:
            nonce = uuid.uuid4().hex
            root = Path("/private/tmp") / ("reasonix-native-image-clipboard-" + nonce)
            root.mkdir(mode=0o700)
            owned = root / mode
            owned.mkdir(mode=0o700)
            for name in ("home", "tmp"):
                (owned / name).mkdir(mode=0o700)
            managed = mode == "managed"
            app_data = owned / "home/Library/Application Support" / identifier
            core = app_data / "reasonix-core" if managed else owned / "core"
            placement.seed_window_state(app_data, geometry)
            core.mkdir(parents=True, mode=0o700)
            provider = ImageProvider(nonce)
            provider.thread.start()
            success = False
            try:
                config = core / "config.toml"
                config.touch(mode=0o600, exist_ok=False)
                config.write_text('default_model = "local/alpha"\n[desktop]\nprovider_access = ["local"]\n'
                                  '[[providers]]\nname = "local"\nkind = "openai"\n'
                                  f'base_url = "http://127.0.0.1:{provider.server.server_port}/v1"\n'
                                  'models = ["alpha"]\ndefault = "alpha"\nvision_models = ["alpha"]\n')
                if windows.package.matching_package_is_running(identifier):
                    raise RuntimeError("Preview started before image acceptance; clipboard untouched")
                with windows.clipboard_fixture.NativeClipboardFixture(owned / "tmp", nonce=nonce, image_format=format_name) as clipboard:
                    if windows.package.matching_package_is_running(identifier):
                        raise RuntimeError("Preview started while preparing image acceptance; clipboard restoration required")
                    identity = windows.launch(app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge",
                                              owned, identifier, managed, "ui-image-paste", clipboard,
                                              verify_window_state=False, environment=environment, launch_services=True)
                    provider.verify()
                    receipt = json.loads((owned / "tmp/reasonix-native-image-paste-result.json").read_text())
                    if any(receipt.get(key) is not True for key in (
                        "ok", "nativeMenuPaste", "draftPreview", "draftRemovalCleanup", "imageSend",
                        "historyImage", "markdownImage", "existingSessionPreview", "stagingCleanup", "noEarlySession", "pendingImageBeforeExit",
                    )):
                        raise RuntimeError("native image Paste receipt incomplete")
                    if list((owned / "tmp").glob("pasted-image-*")):
                        raise RuntimeError("native image host left staged files after exit")
                    saved = json.loads((app_data / "window-state.json").read_text())
                    if saved != geometry:
                        raise RuntimeError("native image app escaped left-display placement")
                if windows.package.matching_package_is_running(identifier):
                    raise RuntimeError("Preview started before image reopen; existing app untouched")
                reopened_identity = windows.launch(app / "Contents/MacOS/reasonix-tauri", app / "Contents/MacOS/reasonix-desktop-bridge",
                                                   owned, identifier, managed, "ui-image-reopen",
                                                   verify_window_state=False, environment=environment, launch_services=True)
                provider.verify()
                reopened = json.loads((owned / "tmp/reasonix-native-image-reopen-result.json").read_text())
                if reopened_identity != identity or any(reopened.get(key) is not True for key in (
                    "ok", "historyImage", "markdownImage", "noDraftReplay", "stagingCleanup",
                )):
                    raise RuntimeError("native image reopen receipt or credential identity differs")
                # Only new isolated Global data is read. Assert bytes received by
                # the real model equal the owned file copied by the Go controller.
                copied = core / "global-workspace" / provider.path
                if not copied.is_file() or hashlib.sha256(copied.read_bytes()).hexdigest() != provider.digest:
                    raise RuntimeError("model input differs from copied Global image")
                record = {"profile": mode, "clipboardFormat": format_name, "paste": receipt, "reopen": reopened,
                          "providerRequests": provider.requests, "providerImageDigest": provider.digest,
                          "modelBytesEqualWorkspace": True, "clipboardRestored": True, "leftDisplay": True, "pendingImageDeletedOnExit": True,
                          "hostSha256": hashlib.sha256((app / "Contents/MacOS/reasonix-tauri").read_bytes()).hexdigest()}
                output = evidence / f"{mode}-{format_name}.json"
                output.touch(mode=0o600, exist_ok=False)
                output.write_text(json.dumps(record, indent=2) + "\n")
                print(f"Native image {mode}/{format_name}: Paste, real model bytes, history reopen, private cleanup and clipboard restore PASS", flush=True)
                success = True
            finally:
                provider.close()
                if success:
                    shutil.rmtree(root)
                else:
                    print("Bounded image provider status: " + json.dumps(provider.failure_status()), file=sys.stderr, flush=True)
                    print(f"Failed private image fixture retained: {root}", file=sys.stderr, flush=True)
    print(f"Native image bounded receipts: {evidence}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("app")
    parser.add_argument("--window-state-template", required=True)
    parser.add_argument("--profile", choices=("both", "managed", "explicit"), default="both")
    parser.add_argument("--format", choices=("both", "png", "tiff"), default="both")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("macOS required")
    smoke(args.app, args.window_state_template, args.profile, args.format)
