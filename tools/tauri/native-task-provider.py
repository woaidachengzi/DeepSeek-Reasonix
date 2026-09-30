"""Bounded loopback SSE fixture for packaged native task lifecycle acceptance."""

from contextlib import AbstractContextManager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import select
import threading
import time


class NativeTaskProvider(AbstractContextManager):
    def __init__(self, core_home, temporary):
        self.core_home, self.temporary = Path(core_home), Path(temporary)
        self.stop = threading.Event()
        self.closed = threading.Event()
        self.completed = threading.Event()
        self.requests = 0
        self.error = None
        self.lock = threading.Lock()
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_):
                pass

            def do_POST(self):
                try:
                    self.connection.settimeout(10)
                    if self.path != "/v1/chat/completions":
                        raise RuntimeError("unexpected local provider route")
                    length = int(self.headers.get("Content-Length", "0"))
                    if not 0 < length <= 2 * 1024 * 1024:
                        raise RuntimeError("invalid local provider request length")
                    request = json.loads(self.rfile.read(length))
                    if request.get("model") != "alpha" or request.get("stream") is not True:
                        raise RuntimeError("native task did not request a real streamed completion")
                    with fixture.lock:
                        fixture.requests += 1
                        number = fixture.requests
                    if number not in (1, 2):
                        raise RuntimeError("native task unexpectedly retried the provider")
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.send_header("Connection", "close")
                    self.end_headers()

                    def send(text, finished=False):
                        payload = {"choices": [{"delta": {"content": text}, "finish_reason": "stop" if finished else None}]}
                        self.wfile.write(("data: " + json.dumps(payload) + "\n\n").encode())
                        self.wfile.flush()

                    if number == 1:
                        send("native-before-close ")
                        fixture.wait_control("reasonix-native-task-allow-progress.json")
                        send("native-after-close ")
                        fixture.wait_control("reasonix-native-task-allow-finish.json")
                        send("native-completed", finished=True)
                        self.wfile.write(b"data: [DONE]\n\n")
                        self.wfile.flush()
                        fixture.completed.set()
                    else:
                        send("native-before-quit")
                        deadline = time.monotonic() + 30
                        while not fixture.stop.is_set() and time.monotonic() < deadline:
                            readable, _, _ = select.select([self.connection], [], [], 0.1)
                            if readable and self.connection.recv(1) == b"":
                                fixture.closed.set()
                                return
                        if not fixture.stop.is_set():
                            raise RuntimeError("native quit did not disconnect the active provider stream")
                except Exception as error:
                    # Never print request bodies, headers or socket diagnostics.
                    with fixture.lock:
                        fixture.error = str(error) if isinstance(error, RuntimeError) else "local native task provider failed"
                finally:
                    self.close_connection = True

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def wait_control(self, name):
        deadline = time.monotonic() + 30
        while not self.stop.is_set() and time.monotonic() < deadline:
            if (self.temporary / name).is_file():
                return
            time.sleep(0.05)
        raise RuntimeError("native task control was not delivered")

    def __enter__(self):
        self.core_home.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.config = self.core_home / "config.toml"
        self.original = self.config.read_bytes() if self.config.exists() else None
        port = self.server.server_port
        self.config.write_text(
            'default_model = "local/alpha"\n\n[desktop]\nprovider_access = ["local"]\n\n'
            '[[providers]]\nname = "local"\nkind = "openai"\n'
            f'base_url = "http://127.0.0.1:{port}/v1"\nmodels = ["alpha"]\ndefault = "alpha"\n'
        )
        self.config.chmod(0o600)
        for name in ("hidden", "progress", "allow-progress", "allow-finish"):
            (self.temporary / f"reasonix-native-task-{name}.json").unlink(missing_ok=True)
        self.thread.start()
        return self

    def pump(self, inspect_live):
        if self.error:
            raise RuntimeError(self.error)
        for marker, control in (("hidden", "allow-progress"), ("progress", "allow-finish")):
            target = self.temporary / f"reasonix-native-task-{control}.json"
            if not target.exists() and (self.temporary / f"reasonix-native-task-{marker}.json").is_file():
                inspect_live()
                target.write_text("{}")

    def verify(self):
        if self.error or self.requests != 2 or not self.completed.is_set() or not self.closed.wait(timeout=3):
            raise RuntimeError("native task did not complete in background and disconnect on quit")

    def __exit__(self, *_):
        self.stop.set()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)
        if self.original is None:
            self.config.unlink(missing_ok=True)
        else:
            self.config.write_bytes(self.original)
        return False
