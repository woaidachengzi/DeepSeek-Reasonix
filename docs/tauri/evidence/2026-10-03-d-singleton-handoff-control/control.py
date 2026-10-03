import importlib.util,json,plistlib,tempfile,shutil,sys
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py');windows=importlib.util.module_from_spec(spec);spec.loader.exec_module(windows)
app=Path(json.loads(Path('/private/tmp/reasonix-singleton-handoff-install.json').read_text())['app'])
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
if windows.package.matching_package_is_running(identifier): raise RuntimeError('Preview is already running')
for managed in [True,False]:
 root=Path(tempfile.mkdtemp(prefix='reasonix-singleton-handoff-control-',dir='/private/tmp'));root.chmod(0o700)
 (root/'home').mkdir(mode=0o700);(root/'tmp').mkdir(mode=0o700)
 completed=[]
 try:
  identities=[]
  for phase in ['exercise','restore-maximized','restore-normal','second-instance']:
   identities.append(windows.launch(app/'Contents/MacOS/reasonix-tauri',app/'Contents/MacOS/reasonix-desktop-bridge',root,identifier,managed,phase,launch_services=True))
   completed.append(phase)
  assert len(set(identities))==1
  (root/'control.json').write_text(json.dumps({'managed':managed,'passed':True,'completed':completed,'profileIdentity':identities[0]})+'\n')
  print(json.dumps({'root':str(root),'managed':managed,'passed':True}),flush=True)
 except BaseException as error:
  (root/'control.json').write_text(json.dumps({'managed':managed,'passed':False,'completed':completed,'error':str(error)})+'\n')
  print(json.dumps({'root':str(root),'managed':managed,'passed':False}),flush=True)
  raise
