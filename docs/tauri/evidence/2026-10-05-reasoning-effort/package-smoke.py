import os,json,tempfile,pathlib,subprocess,secrets,urllib.request,urllib.error,time,http.server,threading,hashlib
binary=pathlib.Path(os.environ.get('REASONIX_USAGE_SMOKE_BIN','/private/tmp/reasonix-effort-bridge'))
output=pathlib.Path(os.environ.get('REASONIX_USAGE_SMOKE_OUTPUT','/private/tmp/reasonix-effort-source-smoke.json'))
root=pathlib.Path(tempfile.mkdtemp(prefix='reasonix-answer-usage-smoke-'));profile=root/'profile';profile.mkdir(mode=0o700);(root/'workspace').mkdir()
calls=[]
class MockProvider(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers['Content-Length'])));assert body['model'] in ['model-one','mimo-v2.6-flash','ordinary'];calls.append(body)
  assert not any('requestUsage' in message or 'createdAt' in message for message in body['messages'])
  self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
  self.wfile.write(b'data: {"choices":[{"index":0,"delta":{"content":"Usage fixture reply"}}]}\n\n')
  if len(calls)<=2:
   multiplier=len(calls);usage={'prompt_tokens':200*multiplier,'completion_tokens':25*multiplier,'total_tokens':225*multiplier,'prompt_cache_hit_tokens':100*multiplier,'prompt_cache_miss_tokens':100*multiplier,'completion_tokens_details':{'reasoning_tokens':5*multiplier}}
   self.wfile.write(('data: '+json.dumps({'choices':[], 'usage':usage})+'\n\n').encode())
  self.wfile.write(b'data: [DONE]\n\n')
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),MockProvider);threading.Thread(target=server.serve_forever,daemon=True).start()
config='default_model = "usage-fixture/model-one"\n[[providers]]\nname = "usage-fixture"\nkind = "openai"\nbase_url = "http://127.0.0.1:'+str(server.server_port)+'/v1"\napi_key_env = "REASONIX_USAGE_FIXTURE_KEY"\nmodels = ["model-one"]\ndefault = "model-one"\nno_proxy = true\n[desktop]\nprovider_access = ["usage-fixture"]\n'
config=config.replace('no_proxy = true\n[desktop]', 'no_proxy = true\nreasoning_protocol = "openai"\nsupported_efforts = ["low","high","max"]\n[[providers]]\nname = "mimo"\nkind = "openai"\nbase_url = "https://api.xiaomimimo.com/v1"\nrequest_url = "http://127.0.0.1:'+str(server.server_port)+'/v1/chat/completions"\napi_key_env = "REASONIX_MIMO_FIXTURE_KEY"\nmodels = ["mimo-v2.6-flash"]\nno_proxy = true\n[desktop]').replace('provider_access = ["usage-fixture"]','provider_access = ["usage-fixture","mimo"]')
(profile/'config.toml').write_text(config);(profile/'config.toml').chmod(0o600)
(profile/'.env').write_text('REASONIX_USAGE_FIXTURE_KEY=fixture-unused-secret\nREASONIX_MIMO_FIXTURE_KEY=second-unused-secret\n');(profile/'.env').chmod(0o600)
env=dict(os.environ,REASONIX_HOME=str(profile),REASONIX_STATE_HOME=str(profile),XDG_CACHE_HOME=str(root/'cache'),REASONIX_CREDENTIALS_STORE='file',REASONIX_USAGE_FIXTURE_KEY='fixture-unused-secret', REASONIX_MIMO_FIXTURE_KEY='second-unused-secret')
proc=None;log=None;ready=root/'ready.json';exits=[];result={'profile':'isolated disposable fixture','actualProviderNetwork':'local SSE mock only','nativeGUI':'not exercised','cases':[]}
def start():
 global proc,log,token,base
 token=secrets.token_hex(32);log=(root/'sidecar.log').open('ab')
 proc=subprocess.Popen([str(binary),'--ready-file',str(ready),'--launch-id',secrets.token_hex(32),'--host-pid',str(os.getpid())],stdin=subprocess.PIPE,stdout=log,stderr=log,env=env,cwd=root)
 proc.stdin.write((token+'\n').encode());proc.stdin.close()
 for _ in range(150):
  if ready.exists():break
  if proc.poll() is not None:raise RuntimeError('sidecar exited before ready')
  time.sleep(.1)
 else:raise RuntimeError('no sidecar readiness')
 base='http://'+json.loads(ready.read_text())['address']
def req(path,body=None,auth=True,method=None):
 headers={'Content-Type':'application/json'}
 if auth:headers['Authorization']='Bearer '+token
 if body is not None:headers['X-Reasonix-Request-ID']=secrets.token_hex(16)
 request=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:
  with urllib.request.urlopen(request,timeout=20) as response:return response.status,json.loads(response.read())
 except urllib.error.HTTPError as e:return e.code,json.loads(e.read())
def expect(path,body=None,status=200,**kw):
 code,data=req(path,body,**kw);assert code==status,(path,code,data);return data
def stop():
 expect('/v1:shutdown',{},202);exits.append(proc.wait(timeout=10));assert exits[-1]==0 and not ready.exists();log.close()

def history():
 data=expect('/v1/sessions/usage-history/history');return data.get('history',data)['messages']
def submit(text):
 expect('/v1/sessions/usage-history:submit',{'input':text},202)
 for _ in range(250):
  if expect('/v1/sessions/usage-history/snapshot')['session']['state']=='idle' and history()[-1]['role']=='assistant':return
  time.sleep(.05)
 raise AssertionError('turn did not finish')
try:
 start();expect('/v1/providers',status=401,auth=False)
 # Authenticated capability summary must advertise real options only.
 summary=expect('/v1/providers')
 providers=summary['providers']
 mimo=next(p for p in providers if p['name']=='mimo')
 assert mimo['reasoning'][0]['levels']==['auto','enabled','disabled'],mimo
 opened=expect('/v1/sessions:open',{'sessionId':'usage-history','workspaceRoot':str(root/'workspace'),'modelRef':'usage-fixture/model-one','effort':'max'})
 assert opened['session']['effort']=='max';submit('max fixture');assert calls[-1]['reasoning_effort']=='max'
 changed=expect('/v1/sessions/usage-history/model',{'model':'usage-fixture/model-one','effort':'low'});assert changed['session']['effort']=='low'
 submit('low fixture');assert calls[-1]['reasoning_effort']=='low'
 before=history();expect('/v1/sessions/usage-history/model',{'model':'usage-fixture/model-one','effort':'turbo'},500)
 assert expect('/v1/sessions/usage-history/snapshot')['session']['effort']=='low' and history()==before
 stop();start();restored=expect('/v1/sessions:open',{'sessionId':'usage-history','workspaceRoot':str(root/'workspace')})
 assert restored['session']['effort']=='low' and restored['session']['modelRef']=='usage-fixture/model-one';assert history()==before
 submit('restart fixture');assert calls[-1]['reasoning_effort']=='low'
 changed=expect('/v1/sessions/usage-history/model',{'model':'mimo/mimo-v2.6-flash'});assert changed['session']['effort']=='auto'
 for level in ['disabled','enabled']:
  expect('/v1/sessions/usage-history/model',{'model':'mimo/mimo-v2.6-flash','effort':level});submit('mimo '+level)
  assert calls[-1]['thinking']['type']==level and 'reasoning_effort' not in calls[-1],calls[-1]
 stop()
 rollback=os.environ.get('REASONIX_USAGE_ROLLBACK_BIN')
 if rollback:
  current=binary;binary=pathlib.Path(rollback);start();expect('/v1/sessions:open',{'sessionId':'usage-history','workspaceRoot':str(root/'workspace')});assert any(m['content']=='max fixture' for m in history());stop();binary=current
  start();expect('/v1/sessions:open',{'sessionId':'usage-history','workspaceRoot':str(root/'workspace')});assert any(m['content']=='max fixture' for m in history());stop();result['cases'].append('previous package can open new transcript; history survives return to candidate')
 result.update(cases=result['cases']+['capability summary redacted and authenticated','explicit first-turn max forwarded','current-session low forwarded','invalid effort keeps current model/effort/history','restart restores exact model/effort/history','model switch resets effort','MiMo Chat binary thinking type and no fictional depth'],processExits=exits,readyRemoved=True,providerRequests=len(calls),binarySha256=hashlib.sha256(binary.read_bytes()).hexdigest());output.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False))
finally:
 if proc and proc.poll() is None:proc.terminate();proc.wait(timeout=10)
 if log and not log.closed:log.close()
 server.shutdown();server.server_close()
