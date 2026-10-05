import os,json,tempfile,pathlib,subprocess,secrets,urllib.request,urllib.error,time,shutil,http.server,threading
app=pathlib.Path('/Users/jerry/temp/app/reasonix-tauri/desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app')
root=pathlib.Path(tempfile.mkdtemp(prefix='reasonix-direct-key-package-'));profile=root/'profile';profile.mkdir(mode=0o700)
fixture_key='fixture-package-api-key';upstream_calls=[]
class MockProvider(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
  if self.headers.get('api-key')!=fixture_key or body['model'] not in ['mimo-v2.6-flash','mimo-v2.6-pro']:
   self.send_error(401);return
  upstream_calls.append({"model":body["model"],"credential":self.headers.get("api-key"),"messages":body.get("messages",[])});self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
  self.wfile.write(b'data: {"choices":[{"index":0,"delta":{"content":"OK"}}]}\n\ndata: [DONE]\n\n')
server=http.server.HTTPServer(('127.0.0.1',0),MockProvider);threading.Thread(target=server.serve_forever,daemon=True).start()
(profile/'config.toml').write_text('default_model = "mimo-token-plan-cn/mimo-v2.5-pro"\n[[providers]]\nname = "mimo-token-plan-cn"\npreset_id = "mimo-token-plan-cn"\nkind = "openai"\nbase_url = "https://token-plan-cn.xiaomimimo.com/v1"\napi_key_env = "PACKAGE_DIRECT_MIMO_KEY"\nmodels = ["mimo-v2.5-pro", "mimo-v2.5"]\ndefault = "mimo-v2.5-pro"\nno_proxy = true\n[desktop]\nprovider_access = ["mimo-token-plan-cn"]\n');(profile/'config.toml').chmod(0o600)
env=dict(os.environ,HOME=str(root),REASONIX_HOME=str(profile),REASONIX_CREDENTIALS_STORE='file');env.pop('PACKAGE_DIRECT_MIMO_KEY',None)
proc=None;log=None;ready=root/'ready.json';receipt={'nativeGUI':'not exercised','profile':'isolated disposable fixture','actualMiMoNetwork':'not exercised; local mock provider only','cases':[]}
def start():
 global proc,log,token,base
 token=secrets.token_hex(32);log=(root/'sidecar.log').open('ab')
 proc=subprocess.Popen([str(app/'Contents/MacOS/reasonix-desktop-bridge'),'--ready-file',str(ready),'--launch-id',secrets.token_hex(32),'--host-pid',str(os.getpid())],stdin=subprocess.PIPE,stdout=log,stderr=log,env=env,cwd=root)
 proc.stdin.write((token+'\n').encode());proc.stdin.close()
 for _ in range(150):
  if ready.exists():break
  if proc.poll() is not None:raise RuntimeError('sidecar exited before ready')
  time.sleep(.1)
 base='http://'+json.loads(ready.read_text())['address']
def request(path,body=None,auth=True):
 headers={'Content-Type':'application/json'}
 if auth:headers['Authorization']='Bearer '+token
 if body is not None:headers['X-Reasonix-Request-ID']=secrets.token_hex(16)
 req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers)
 try:
  with urllib.request.urlopen(req,timeout=15) as r:
   raw=r.read();assert b'fixture-package-api-key' not in raw;return r.status,json.loads(raw)
 except urllib.error.HTTPError as e:return e.code,None
def stop():
 assert request('/v1:shutdown',{})[0]==202;assert proc.wait(timeout=10)==0 and not ready.exists();log.close()
try:
 start()
 original=(profile/'config.toml').read_text()
 config_view=request('/v1/settings/provider-configs')[1]
 entry=next(p for p in config_view['providers'] if p['name']=='mimo-token-plan-cn')
 assert entry['models'][:2]==['mimo-v2.6-flash','mimo-v2.6-pro'] and entry['default']=='mimo-v2.5-pro'
 preset=next(p for p in config_view['presets'] if p['id']=='mimo-token-plan-cn')
 assert preset['routes'][0]['default']=='mimo-v2.6-flash'
 assert (profile/'config.toml').read_text()==original
 summary=request('/v1/providers')[1]
 entry=next(p for p in summary['providers'] if p['name']=='mimo-token-plan-cn')
 assert 'mimo-v2.6-flash' in entry['models']
 receipt['cases'].extend(['old preset editor and composer expose v2.6 flash/pro','existing default retained and configuration not written by reads','new preset defaults to v2.6 flash'])
 # Route only the disposable fixture's requests to the local mock. Preserve
 # the just-upgraded catalog explicitly before adding a custom request URL.
 upgraded=original.replace('models = ["mimo-v2.5-pro", "mimo-v2.5"]','models = ["mimo-v2.6-flash", "mimo-v2.6-pro", "mimo-v2.5-pro", "mimo-v2.5"]')
 upgraded=upgraded.replace('kind = "openai"','kind = "openai"\nrequest_url = "http://127.0.0.1:'+str(server.server_port)+'/v1/chat/completions"')
 (profile/'config.toml').write_text(upgraded)
 assert request('/v1/settings/provider-api-key',{'providerName':'mimo-token-plan-cn','apiKey':fixture_key},False)[0]==401
 assert not request('/v1/providers')[1]['providers'][0]['configured']
 assert request('/v1/settings/provider-api-key',{'providerName':'mimo-token-plan-cn','apiKey':fixture_key})[0]==200
 assert request('/v1/providers')[1]['providers'][0]['configured']
 assert request('/v1/sessions:open',{'sessionId':'key-smoke','workspaceRoot':str(root/'workspace')})[0]==200
 assert request('/v1/sessions/key-smoke/model',{'model':'mimo-token-plan-cn/mimo-v2.6-flash'})[0]==200
 def turn(text):
  before=len(upstream_calls)
  status,response=request('/v1/sessions/key-smoke:submit',{'input':text})
  assert status==202,(status,response)
  for _ in range(200):
   history=request('/v1/sessions/key-smoke/history')[1]
   messages=history['history']['messages'] if 'history' in history else history['messages']
   if len(upstream_calls)>before and messages and messages[-1]['role']=='assistant' and request('/v1/sessions/key-smoke/snapshot')[1]['session']['state']=='idle':break
   time.sleep(.05)
  else:raise AssertionError('turn did not finish')
  return messages
 messages=turn('first message')
 assert upstream_calls[-1]['credential']==fixture_key
 assert request('/v1/sessions/key-smoke/approval-mode',{'mode':'auto'})[0]==200
 fixture_key='fixture-package-api-key-rotated'
 assert request('/v1/settings/provider-api-key',{'providerName':'mimo-token-plan-cn','apiKey':fixture_key})[0]==200
 messages=turn('second message after saving')
 assert upstream_calls[-1]['credential']==fixture_key
 assert any(m['role']=='user' and m['content']=='first message' for m in messages)
 assert request('/v1/sessions/key-smoke/approval-mode')[1]['mode']=='auto'
 original_config=(profile/'config.toml').read_text()
 (profile/'config.toml').write_text('this is invalid toml [')
 before=len(upstream_calls)
 status,_=request('/v1/sessions/key-smoke:submit',{'input':'must not send'})
 assert status==409 and len(upstream_calls)==before
 assert request('/v1/sessions/key-smoke/history')[0]==200
 (profile/'config.toml').write_text(original_config)
 turn('retry after restoring config')
 assert request('/v1/sessions/key-smoke/model',{'model':'mimo-token-plan-cn/mimo-v2.6-pro'})[0]==200
 turn('message with selected model')
 assert upstream_calls[-1]['model']=='mimo-v2.6-pro' and upstream_calls[-1]['credential']==fixture_key
 receipt['cases'].extend(['active conversation uses rotated key without restarting','history and approval mode retained','settings read failure rejects request and retains history','retry after restoring settings succeeds','explicit model switch uses saved key'])
 files=[p for p in profile.rglob('*') if p.is_file() and fixture_key.encode() in p.read_bytes()]
 assert len(files)==1 and files[0].name=='.env' and files[0].stat().st_mode&0o777==0o600
 receipt['cases'].extend(['unauthorized write refused','saved API key immediately ready','actual conversation uses saved API key','secret stored only in owner-only profile .env'])
 stop();start()
 assert request('/v1/providers')[1]['providers'][0]['configured']
 assert request('/v1/settings/provider-model-probe',{'name':'mimo-token-plan-cn','model':'mimo-v2.6-pro'})[0]==200
 receipt['cases'].append('new sidecar restores local API key and probes second model')
 assert request('/v1/settings/provider-api-key',{'providerName':'mimo-token-plan-cn','delete':True})[0]==200
 assert not request('/v1/providers')[1]['providers'][0]['configured']
 assert fixture_key.encode() not in files[0].read_bytes()
 stop();receipt.update(processExits=[0,0],readyRemoved=True,localProviderRequests=len(upstream_calls));receipt['upstreamModels']=[c['model'] for c in upstream_calls]
 pathlib.Path('/private/tmp/reasonix-mimo-v26-package-smoke.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt))
finally:
 if proc and proc.poll() is None:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 if log and not log.closed:log.close()
 server.shutdown();server.server_close();shutil.rmtree(root)
