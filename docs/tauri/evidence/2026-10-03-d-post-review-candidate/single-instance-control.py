import importlib.util,json,plistlib,tempfile,shutil,sys
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py');windows=importlib.util.module_from_spec(spec);spec.loader.exec_module(windows)
app=Path(json.loads(Path('/private/tmp/reasonix-post-review-install.json').read_text())['app'])
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
if windows.package.matching_package_is_running(identifier): raise RuntimeError('Preview is already running')
source=Path('/private/tmp/reasonix-native-window-smoke-a58fr_b0')
for managed in [True,False]:
 root=Path(tempfile.mkdtemp(prefix='reasonix-single-instance-control-',dir='/private/tmp'));root.chmod(0o700)
 (root/'home').mkdir(mode=0o700);(root/'tmp').mkdir(mode=0o700)
 data=root/'home/Library/Application Support'/identifier;data.mkdir(mode=0o700,parents=True)
 shutil.copyfile(source/'home/Library/Application Support'/identifier/'window-state.json',data/'window-state.json')
 shutil.copyfile(source/'tmp/reasonix-native-window-normal.json',root/'tmp/reasonix-native-window-normal.json')
 try:
  identity=windows.launch(app/'Contents/MacOS/reasonix-tauri',app/'Contents/MacOS/reasonix-desktop-bridge',root,identifier,managed,'second-instance',launch_services=True)
  (root/'control.json').write_text(json.dumps({'managed':managed,'passed':True,'profileIdentity':identity})+'\n')
  print(json.dumps({'root':str(root),'managed':managed,'passed':True}),flush=True)
 except BaseException as error:
  (root/'control.json').write_text(json.dumps({'managed':managed,'passed':False,'error':str(error)})+'\n')
  print(json.dumps({'root':str(root),'managed':managed,'passed':False}),flush=True)
  raise
