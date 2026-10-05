import pathlib,os,subprocess,hashlib,json,tempfile,secrets,time,urllib.request,urllib.error
repo=pathlib.Path('/Users/jerry/temp/app/reasonix-tauri');e=repo/'docs/tauri/evidence/2026-10-05-answer-actions'
app=repo/'desktop/tauri/target/release/bundle/macos.noindex/Reasonix Tauri Preview.app'
head=subprocess.check_output(['git','rev-parse','3f4f0a7b0'],cwd=repo,text=True).strip()
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
metadata=subprocess.check_output(['go','version','-m',str(app/'Contents/MacOS/reasonix-desktop-bridge')],text=True)
assert 'vcs.revision='+head in metadata and 'vcs.modified=false' in metadata
assert (repo/'desktop/tauri/target/release/bundle/macos').is_symlink()
assert head.encode() in (app/'Contents/MacOS/reasonix-tauri').read_bytes()
root=pathlib.Path(tempfile.mkdtemp(prefix='reasonix-answer-package-check-'));profile=root/'profile';profile.mkdir(mode=0o700)
(profile/'config.toml').write_text('');(profile/'config.toml').chmod(0o600)
ready=root/'ready.json';token=secrets.token_hex(32);env=dict(os.environ,REASONIX_HOME=str(profile),REASONIX_STATE_HOME=str(profile),XDG_CACHE_HOME=str(root/'cache'),REASONIX_CREDENTIALS_STORE='file')
log=(root/'sidecar.log').open('wb');proc=subprocess.Popen([str(app/'Contents/MacOS/reasonix-desktop-bridge'),'--ready-file',str(ready),'--launch-id',secrets.token_hex(32),'--host-pid',str(os.getpid())],stdin=subprocess.PIPE,stdout=log,stderr=log,env=env,cwd=root)
try:
 proc.stdin.write((token+'\n').encode());proc.stdin.close()
 for _ in range(150):
  if ready.exists():break
  assert proc.poll() is None,'early exit'
  time.sleep(.1)
 else:raise AssertionError('no readiness')
 base='http://'+json.loads(ready.read_text())['address']
 def req(path,body=None,auth=True):
  headers={'Content-Type':'application/json'}
  if auth:headers['Authorization']='Bearer '+token
  if body is not None:headers['X-Reasonix-Request-ID']=secrets.token_hex(16)
  request=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers)
  try:
   with urllib.request.urlopen(request,timeout=10) as r:return r.status,json.loads(r.read())
  except urllib.error.HTTPError as error:return error.code,json.loads(error.read())
 denied=req('/v1/health',auth=False);assert denied[0]==401,denied
 health=req('/v1/health');assert health[0]==200 and health[1]['status']=='ok',health
 shutdown=req('/v1:shutdown',{});assert shutdown[0]==202,shutdown
 exitCode=proc.wait(timeout=10);assert exitCode==0 and not ready.exists()
finally:
 if proc.poll() is None:proc.terminate();proc.wait(timeout=10)
 log.close()
rollback=json.loads(pathlib.Path('/private/tmp/reasonix-answer-actions-rollback.json').read_text())
receipt=dict(sourceCommit=head,sourceDirty=False,buildCommand='pnpm tauri:build --bundles app',artifact=str(app),bundleIdentifier='io.reasonix.desktop.preview',architecture='macOS arm64',version='0.1.0',signing='ad-hoc; codesign --verify --deep --strict passed; not notarized',sha256={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (app/'Contents/MacOS').iterdir() if p.is_file()},**rollback,packageChecks={'embeddedHostRevision':True,'sidecarGoRevision':True,'sidecarVcsModified':False,'health':health,'unauthenticatedHealthStatus':denied[0],'shutdownStatus':shutdown[0],'exitCode':exitCode,'readyFileRemoved':not ready.exists()},nativeGUI='not exercised; headless actual UI component browser QA and real packaged sidecar only',actualProviderNetwork='not exercised',packaging='direct app only; no ZIP/DMG generated; Applications not replaced',rollback='Close new app; rename saved .app.rollback to .app and run it. This display-only change does not migrate data.')
(e/'package-receipt.json').write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+'\n')
(e/'package-go-metadata.txt').write_text(metadata)
print(json.dumps(receipt,ensure_ascii=False))
