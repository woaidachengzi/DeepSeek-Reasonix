import os, json, tempfile, pathlib, subprocess, secrets, urllib.request, urllib.error, time, http.server, threading, hashlib, sqlite3
app=pathlib.Path(__file__).resolve().parents[4]/'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
binary=pathlib.Path(os.environ.get('REASONIX_ARCHIVE_SMOKE_BIN',str(app/'Contents/MacOS/reasonix-desktop-bridge')))
output=pathlib.Path(os.environ.get('REASONIX_ARCHIVE_SMOKE_OUTPUT','/private/tmp/reasonix-session-archive-package-smoke.json'))
root=pathlib.Path(tempfile.mkdtemp(prefix='reasonix-session-archive-smoke-'));profile=root/'profile';profile.mkdir(mode=0o700)
entered, release=threading.Event(), threading.Event();release.set();calls=[]
class MockProvider(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def do_POST(self):
  body=json.loads(self.rfile.read(int(self.headers['Content-Length'])));assert body['model']=='model-one'
  calls.append(body['messages']);entered.set();release.wait(timeout=15)
  self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
  self.wfile.write(b'data: {"choices":[{"index":0,"delta":{"content":"Archive fixture reply"}}]}\n\ndata: [DONE]\n\n')
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),MockProvider);threading.Thread(target=server.serve_forever,daemon=True).start()
config='default_model = "archive-fixture/model-one"\n[[providers]]\nname = "archive-fixture"\nkind = "openai"\nbase_url = "http://127.0.0.1:'+str(server.server_port)+'/v1"\napi_key_env = "REASONIX_ARCHIVE_FIXTURE_KEY"\nmodels = ["model-one"]\ndefault = "model-one"\nno_proxy = true\n[desktop]\nprovider_access = ["archive-fixture"]\n'
(profile/'config.toml').write_text(config);(profile/'config.toml').chmod(0o600)
env=dict(os.environ,HOME=str(root),REASONIX_HOME=str(profile),REASONIX_STATE_HOME=str(profile),XDG_CACHE_HOME=str(root/'cache'),REASONIX_CREDENTIALS_STORE='file',REASONIX_ARCHIVE_FIXTURE_KEY='fixture-unused-secret')
proc=None;log=None;ready=root/'ready.json';exits=[];result={'nativeGUI':'not exercised; packaged sidecar real process','profile':'isolated disposable fixture','actualProviderNetwork':'not exercised; local mock only','cases':[]}
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
def history(id):
 data=expect('/v1/sessions/'+id+'/history');return data.get('history',data)['messages']
def wait_idle(id):
 for _ in range(250):
  if expect('/v1/sessions/'+id+'/snapshot')['session']['state']=='idle' and history(id)[-1]['role']=='assistant':return
  time.sleep(.05)
 raise AssertionError('turn did not finish')
def submit(id,text):expect('/v1/sessions/'+id+':submit',{'input':text},202);wait_idle(id)
def change(id,archived,status=200):return expect('/v1/sessions/archives',{'sessionId':id,'archived':archived},status)
try:
 start();expect('/v1/sessions/archives',auth=False,status=401)
 expect('/v1/sessions/archives',{'sessionId':'../outside','archived':True},400)
 opened=expect('/v1/sessions:open',{'sessionId':'archived-history','workspaceRoot':str(root/'workspace')})
 path=pathlib.Path(opened['session']['path']);expect('/v1/sessions/archived-history/title',{'title':'保留完整历史'},method='PATCH')
 submit('archived-history','Original history remains after archive')
 assert any(m.get('content')=='Original history remains after archive' for m in history('archived-history'))
 # Disposable marker files verify archive never invokes the artifact sweep.
 attachment=path.with_suffix('.attachments');attachment.mkdir(exist_ok=True);(attachment/'fixture.txt').write_text('preserved attachment marker')
 checkpoint=path.with_suffix('.ckpt');checkpoint.mkdir(exist_ok=True);(checkpoint/'fixture').write_text('preserved checkpoint marker')
 entered.clear();release.clear();expect('/v1/sessions/archived-history:submit',{'input':'Held running turn'},202);assert entered.wait(timeout=5)
 change('archived-history',True,409);assert expect('/v1/sessions/archives')['sessions']==[]
 release.set();wait_idle('archived-history');before=path.read_bytes();original_config=(profile/'config.toml').read_bytes()
 first=change('archived-history',True)['sessions'];assert len(first)==1 and first[0]['sessionId']=='archived-history'
 assert change('archived-history',True)['sessions']==first
 assert path.read_bytes()==before and (attachment/'fixture.txt').read_text()=='preserved attachment marker' and (checkpoint/'fixture').read_text()=='preserved checkpoint marker'
 expect('/v1/sessions:open',{'sessionId':'archived-history'},409)
 metadata=next(profile.rglob('session-archives-v1.json'));assert metadata.stat().st_mode&0o777==0o600
 identity=next(p for p in profile.rglob('*') if p.is_file() and p.read_bytes()[:16]==b'SQLite format 3\x00')
 with sqlite3.connect(identity) as db:assert db.execute('PRAGMA user_version').fetchone()[0]==9
 result['cases']+=['unauthorized and traversal requests rejected','running archive refused without state change','archive retry retains original date','history checkpoint and attachment paths/bytes retained','direct open of archived ID refused','owner-only overlay; identity schema 9 unchanged']
 stop()
 rollback=os.environ.get('REASONIX_ARCHIVE_ROLLBACK_BIN')
 if rollback:
  current_binary=binary;binary=pathlib.Path(rollback);start()
  expect('/v1/sessions:open',{'sessionId':'archived-history'})
  assert any(m.get('content')=='Original history remains after archive' for m in history('archived-history'))
  stop();result['rollbackBinarySha256']=hashlib.sha256(binary.read_bytes()).hexdigest();binary=current_binary
  result['cases'].append('previous packaged sidecar opens archived conversation and reads original history without data conversion')
 start();assert expect('/v1/sessions/archives')['sessions']==first
 change('archived-history',False);reopened=expect('/v1/sessions:open',{'sessionId':'archived-history'});assert reopened['session']['path']==str(path)
 assert any(m.get('content')=='Original history remains after archive' for m in history('archived-history'))
 submit('archived-history','Continue after restore')
 assert any(m.get('content')=='Original history remains after archive' for m in history('archived-history'))
 other=expect('/v1/sessions:switch',{'sessionId':'other-id'});expect('/v1/sessions/other-id/title',{'title':'Other empty conversation'},method='PATCH')
 expect('/v1/sessions:switch',{'sessionId':'archived-history'});change('other-id',True);expect('/v1/sessions:switch',{'sessionId':'other-id'},409)
 expect('/v1/sessions/archived-history/history');assert pathlib.Path(other['session']['path']+'.meta').is_file();change('other-id',False)
 saved=metadata.read_bytes();metadata.write_text('bad archive state')
 expect('/v1/sessions/archives',status=500);change('archived-history',True,500);expect('/v1/sessions:switch',{'sessionId':'other-id'},500)
 assert metadata.read_text()=='bad archive state';expect('/v1/sessions/archived-history/history');metadata.write_bytes(saved)
 submit('archived-history','Retry after archive metadata repair');assert (profile/'config.toml').read_bytes()==original_config
 result['cases']+=['archive survives sidecar restart','restore opens original ID/path/history and continues with same provider','inactive archive retains current owner','fresh empty conversation title survives archive','refused switch preserves current owner','corrupt overlay fails closed without overwrite','repair permits retry; config bytes unchanged']
 stop();result.update(processExits=exits,readyRemoved=True,providerRequests=len(calls),binarySha256=hashlib.sha256(binary.read_bytes()).hexdigest());output.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
finally:
 release.set()
 if proc and proc.poll() is None:
  proc.terminate()
  try:proc.wait(timeout=10)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 if log and not log.closed:log.close()
 server.shutdown();server.server_close()
