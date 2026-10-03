import hashlib,importlib.util,json,os,plistlib,shutil,subprocess,tempfile,time
from pathlib import Path
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py')
w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
app=Path('/private/tmp/reasonix-d-fullscreen-installed-y5w4n0kj/Reasonix Tauri Preview.app')
host=app/'Contents/MacOS/reasonix-tauri';sidecar=app/'Contents/MacOS/reasonix-desktop-bridge'
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
expected={'reasonix-tauri':'22ee53cbfe6324f63a1f63d73d477cb3017be21cd5866ee4dfab1f50efc20e35','reasonix-desktop-bridge':'6f7f6352af9eed4043abe9600b6ad80712088d9363b4a94d69d824628035e447'}
assert not w.package.matching_package_is_running(identifier)
for p in (host,sidecar):assert hashlib.sha256(p.read_bytes()).hexdigest()==expected[p.name]
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
out=Path('/private/tmp/reasonix-minimize-path-control');out.mkdir(mode=0o700)
observer=out/'activation-timeline'
subprocess.run(['swiftc',str(repo/'tools/tauri/activation-timeline.swift'),'-o',str(observer)],check=True)
summary={'artifactSha256':expected,'scope':'Independent once-per-path native controls, not a full window acceptance or retry of the failed exercise','cases':[]}
phases=('menu-settings-minimized','menu-settings-direct-minimized','menu-settings-native-minimized','menu-settings-presented-minimized')
for managed in (True,False):
 for phase in phases:
  name=('managed' if managed else 'explicit')+'-'+phase
  receipt=out/name;receipt.mkdir(mode=0o700)
  root=Path(tempfile.mkdtemp(prefix='reasonix-minimize-path-',dir='/private/tmp'));root.chmod(0o700)
  for folder in ('home','tmp'):(root/folder).mkdir(mode=0o700)
  entry={'name':name,'root':str(root),'status':'running'};summary['cases'].append(entry)
  with (receipt/'activation.jsonl').open('wb') as stream,(receipt/'observer.log').open('wb') as errors:
   process=subprocess.Popen([str(observer)],stdout=stream,stderr=errors)
   try:
    deadline=time.monotonic()+5
    while (receipt/'activation.jsonl').stat().st_size==0:
     if process.poll() is not None or time.monotonic()>=deadline:raise RuntimeError('observer not ready')
     time.sleep(.01)
    w.launch(host,sidecar,root,identifier,managed,phase,verify_window_state=False,launch_services=True)
    entry['status']='passed'
   except Exception as error:
    entry.update(status='failed',error=str(error));print(name+': FAILED '+str(error),flush=True)
   finally:
    if process.poll() is None:process.terminate()
    process.wait(timeout=5)
    for file in (root/'tmp').glob('*'):
     if file.is_file() and not file.is_symlink() and file.name.startswith(('reasonix-native-window-','launch-services-')) and file.suffix in ('.json','.jsonl'):
      shutil.copy2(file,receipt/file.name)
    (out/'result.json').write_text(json.dumps(summary,indent=2)+'\n')
    if entry['status']=='passed':shutil.rmtree(root)
for p in (host,sidecar):assert hashlib.sha256(p.read_bytes()).hexdigest()==expected[p.name]
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
print(json.dumps({c['name']:c['status'] for c in summary['cases']}),flush=True)
