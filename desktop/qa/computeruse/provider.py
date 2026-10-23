"""Deterministic, loopback-only model for packaged desktop acceptance."""
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class FixtureModel:
    def __init__(self, root):
        self.root = root
        self.step = 0
        self.stale = 0
        self.waits = 0
        self.actions = ['status', 'apps', 'observe', 'activate', 'observe',
                        'click_button', 'observe', 'click_field', 'observe',
                        'type', 'observe', 'select_all', 'observe', 'replace',
                        'observe', 'scroll', 'observe']

    def reply(self, request):
        tools = [t.get('function', {}).get('name') for t in request.get('tools', [])]
        if 'use_capability' not in tools:
            return {'role': 'assistant', 'content': 'QA blocked: capability router unavailable.'}
        recent = next((m for m in reversed(request.get('messages', [])) if m['role'] == 'tool'), None)
        prior = {}
        if recent:
            content = recent.get('content', '')
            with (self.root / 'tool-results.jsonl').open('a') as f:
                f.write(json.dumps({'step': self.step, 'content': content}) + '\n')
            try:
                prior = json.JSONDecoder().raw_decode(content[content.index('{'):])[0]
            except (ValueError, TypeError):
                return {'role': 'assistant', 'content': 'QA stopped: malformed or blocked tool result.'}
            if prior.get('state_changed'):
                if self.stale >= 4:
                    return {'role': 'assistant', 'content': 'QA stopped: fixture did not settle after fresh observations.'}
                self.step -= 1
                self.stale += 1
            elif content.startswith('error:'):
                if 'app state changed; no input sent' in content and self.stale < 4:
                    self.step -= 1
                    self.stale += 1
                else:
                    return {'role': 'assistant', 'content': 'QA stopped: inspect the saved tool error.'}
            else:
                self.stale = 0
        fixture = json.loads((self.root / 'fixture-state.json').read_text())
        if self.step >= len(self.actions):
            return {'role': 'assistant', 'content': 'QA complete; independently verify fixture postconditions.'}
        action = self.actions[self.step]
        state = prior.get('state') or {}
        if state.get('capture_available'):
            images = [part for message in request.get('messages', [])
                      if isinstance(message.get('content'), list)
                      for part in message['content'] if part.get('type') == 'image_url']
            if not images:
                return {'role': 'assistant', 'content': 'QA stopped: screenshot did not reach the vision model.'}
        if self.step == 1 and not (state.get('accessibility') and state.get('screen_recording')):
            return {'role': 'assistant', 'content': 'QA stopped: grant the packaged app Accessibility and Screen Recording.'}
        if self.step == 2 and not any(a['pid'] == fixture['pid'] and a['bundle_id'] == 'io.reasonix.control.fixture' for a in state.get('apps', [])):
            return {'role': 'assistant', 'content': 'QA stopped: fixture identity not in real app catalog.'}
        if self.step == 11 and fixture['text'] != 'Reasonix 验收 🧪':
            return {'role': 'assistant', 'content': 'QA stopped: Unicode input postcondition failed.'}
        args = {'action': action, 'pid': fixture['pid']}
        if action in ('status', 'apps'):
            args = {'action': action}
        elif action != 'observe':
            args['observation'] = prior.get('observation', '')
            elements = state.get('elements', [])
            if action == 'click_button':
                button = next((e for e in elements if 'Increment test counter' in (e.get('title', '') + e.get('description', ''))), None)
                if button is None:
                    if self.waits < 4:
                        self.waits += 1
                        return self.call({'action': 'observe', 'pid': fixture['pid']})
                    return {'role': 'assistant', 'content': 'QA stopped: fixture button unavailable.'}
                args.update(action='click', element=button['element'])
            if action == 'click_field':
                field = next((e for e in elements if e.get('role') == 'AXTextField'), None)
                if field is None:
                    return {'role': 'assistant', 'content': 'QA stopped: fixture text field unavailable.'}
                b, w = field['bounds'], state['bounds']
                args.update(action='click', x=b['x'] - w['x'] + b['width'] / 2,
                            y=b['y'] - w['y'] + b['height'] / 2)
            if action == 'type':
                args['text'] = 'Reasonix 验收 🧪'
            if action == 'select_all':
                args.update(action='key', key='a', modifiers=['command'])
            if action == 'replace':
                args.update(action='type', text='Desktop acceptance passed ✅')
            if action == 'scroll':
                args['delta'] = -8
        self.step += 1
        return self.call(args)

    @staticmethod
    def call(args):
        return {'role': 'assistant', 'content': None, 'tool_calls': [{
            'id': f'qa_{time.time_ns()}', 'type': 'function',
            'function': {'name': 'use_capability', 'arguments': json.dumps({
                'action': 'call', 'capability_id': 'tool:computer_use', 'arguments': args})}}]}


def serve(root):
    model = FixtureModel(root)

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_GET(self):
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({'object': 'list', 'data': [{'id': 'desktop-qa', 'object': 'model'}]}).encode())

        def do_POST(self):
            request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            time.sleep(0.3)  # Simulate a model response instead of hammering window animations.
            reply = model.reply(request)
            self.send_response(200)
            if request.get('stream'):
                self.send_header('Content-Type', 'text/event-stream')
                self.end_headers()
                if 'tool_calls' in reply:
                    for i, call in enumerate(reply['tool_calls']):
                        call['index'] = i
                for delta, reason in [(reply, None), ({}, 'tool_calls' if 'tool_calls' in reply else 'stop')]:
                    chunk = {'id': 'qa', 'object': 'chat.completion.chunk', 'model': 'desktop-qa',
                             'choices': [{'index': 0, 'delta': delta, 'finish_reason': reason}]}
                    self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
                self.wfile.write(b'data: [DONE]\n\n')
            else:
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({'id': 'qa', 'object': 'chat.completion', 'model': 'desktop-qa',
                    'choices': [{'index': 0, 'message': reply, 'finish_reason': 'tool_calls' if 'tool_calls' in reply else 'stop'}]}).encode())

    return ThreadingHTTPServer(('127.0.0.1', 0), Handler)
