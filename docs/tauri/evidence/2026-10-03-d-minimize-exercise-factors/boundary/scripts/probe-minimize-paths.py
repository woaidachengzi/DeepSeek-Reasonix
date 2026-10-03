#!/usr/bin/env python3
import argparse,sys,hashlib,importlib.util,json,os,plistlib,shutil,subprocess,tempfile,time
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py')
w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
parser=argparse.ArgumentParser(description='Once-per-path native minimize controls; not a complete window acceptance')
parser.add_argument('app',type=Path)
parser.add_argument('--output',required=True,type=Path)
parser.add_argument('--exercise-factors',action='store_true',help='2x2 trusted-page/initial-Show controls; original exercise retained')
options=parser.parse_args()
if sys.platform != 'darwin':parser.error('macOS required')
app=options.app.resolve()
host=app/'Contents/MacOS/reasonix-tauri';sidecar=app/'Contents/MacOS/reasonix-desktop-bridge'
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
expected={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (host,sidecar)}
assert not w.package.matching_package_is_running(identifier)
for p in (host,sidecar):assert hashlib.sha256(p.read_bytes()).hexdigest()==expected[p.name]
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
out=options.output.resolve();out.mkdir(mode=0o700,exist_ok=False)
observer=out/'activation-timeline'
subprocess.run(['swiftc',str(repo/'tools/tauri/activation-timeline.swift'),'-o',str(observer)],check=True)
summary={'artifactSha256':expected,'scope':'Independent 2x2 exercise factors' if options.exercise_factors else 'Independent native path controls','completeWindowAcceptance':False,'cases':[]}
phases=('exercise','exercise-loaded','exercise-single-show','exercise-loaded-single-show') if options.exercise_factors else ('menu-settings-minimized','menu-settings-direct-minimized','menu-settings-native-minimized','menu-settings-presented-minimized')
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

sys.exit(0 if all(case['status']=='passed' for case in summary['cases']) else 1)
