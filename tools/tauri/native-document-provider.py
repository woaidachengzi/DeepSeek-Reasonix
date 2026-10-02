"""One fixed loopback completion for ordinary local-document UI acceptance."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading

PROMPT = 'Show the two prepared local document links.'
CONTENTS = (b'Reasonix native document original A\n', b'Reasonix native document original B\n')


class DocumentProvider:
    def __init__(self, core, sources):
        self.requests, self.error = 0, None
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'

            def log_message(self, *_):
                pass

            def do_POST(self):
                try:
                    self.connection.settimeout(10)
                    length = int(self.headers.get('Content-Length', '0'))
                    if self.path != '/v1/chat/completions' or not 0 < length <= 2 << 20:
                        raise RuntimeError('unexpected request')
                    data = json.loads(self.rfile.read(length))
                    fixture.requests += 1
                    if (fixture.requests != 1 or data.get('model') != 'alpha'
                            or data.get('stream') is not True
                            or not any(message.get('role') == 'user' and message.get('content') == PROMPT
                                       for message in data.get('messages', []) if isinstance(message, dict))):
                        raise RuntimeError('unexpected prompt/retry')
                    self.send_response(200)
                    self.send_header('Content-Type', 'text/event-stream')
                    self.send_header('Connection', 'close')
                    self.end_headers()
                    text = '\n\n'.join(f'[Original {letter}]({path.as_uri()})'
                                       for letter, path in zip(('A', 'B'), sources))
                    for delta, finish in (({'role': 'assistant', 'content': text}, None), ({}, 'stop')):
                        payload = {'choices': [{'index': 0, 'delta': delta, 'finish_reason': finish}]}
                        self.wfile.write(('data: ' + json.dumps(payload) + '\n\n').encode())
                    self.wfile.write(b'data: [DONE]\n\n')
                    self.wfile.flush()
                except Exception:
                    # No request bodies, headers, credentials or socket diagnostics.
                    fixture.error = 'Bounded document provider failed'
                finally:
                    self.close_connection = True

        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.server.daemon_threads = True
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        config = core / 'config.toml'
        try:
            config.touch(mode=0o600, exist_ok=False)
            config.write_text('default_model = "local/alpha"\n\n[desktop]\nprovider_access = ["local"]\n\n'
                              '[[providers]]\nname = "local"\nkind = "openai"\n'
                              f'base_url = "http://127.0.0.1:{self.server.server_port}/v1"\n'
                              'models = ["alpha"]\ndefault = "alpha"\n')
        except Exception:
            self.server.server_close()
            raise
        self.thread.start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)
