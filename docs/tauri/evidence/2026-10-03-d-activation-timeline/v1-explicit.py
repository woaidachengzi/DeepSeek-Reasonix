import importlib.util,json,plistlib,tempfile,shutil,sys,subprocess,time
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py');windows=importlib.util.module_from_spec(spec);spec.loader.exec_module(windows)
app=Path(json.loads(Path('/private/tmp/reasonix-post-review-install.json').read_text())['app'])
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
if windows.package.matching_package_is_running(identifier): raise RuntimeError('Preview is already running')
for managed in [False]:
 root=Path(tempfile.mkdtemp(prefix='reasonix-activation-timeline-control-',dir='/private/tmp'));root.chmod(0o700)
 (root/'home').mkdir(mode=0o700);(root/'tmp').mkdir(mode=0o700)
 completed=[]
 timeline=(root/'activation.jsonl').open('wb')
 observer=subprocess.Popen(['/private/tmp/reasonix-activation-timeline'],stdout=timeline,stderr=subprocess.PIPE)
 events=(root/'phase-events.jsonl').open('w')
 deadline=time.monotonic()+5
 while (root/'activation.jsonl').stat().st_size==0:
  if observer.poll() is not None or time.monotonic()>deadline: raise RuntimeError('read-only observer was not ready')
  time.sleep(.01)
 try:
  identities=[]
  for phase in ['exercise','restore-maximized','restore-normal','second-instance']:
   events.write(json.dumps({'phase':phase,'kind':'begin','unixMs':int(time.time()*1000)})+'\n');events.flush()
   identities.append(windows.launch(app/'Contents/MacOS/reasonix-tauri',app/'Contents/MacOS/reasonix-desktop-bridge',root,identifier,managed,phase,launch_services=True))
   completed.append(phase)
   events.write(json.dumps({'phase':phase,'kind':'passed','unixMs':int(time.time()*1000)})+'\n');events.flush()
  assert len(set(identities))==1
  (root/'control.json').write_text(json.dumps({'managed':managed,'passed':True,'completed':completed,'profileIdentity':identities[0]})+'\n')
  print(json.dumps({'root':str(root),'managed':managed,'passed':True}),flush=True)
 except BaseException as error:
  (root/'control.json').write_text(json.dumps({'managed':managed,'passed':False,'completed':completed,'error':str(error)})+'\n')
  print(json.dumps({'root':str(root),'managed':managed,'passed':False}),flush=True)
  events.write(json.dumps({'kind':'failed','unixMs':int(time.time()*1000)})+'\n');events.flush()
  raise
 finally:
  if observer.poll() is None: observer.terminate()
  observer.wait(timeout=5);timeline.close();events.close()
