import importlib.util,json,os,plistlib,tempfile,hashlib,traceback
from pathlib import Path
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py');w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
app=Path('/private/tmp/reasonix-d-explicit-boundary-installed-n0m21k7z/Reasonix Tauri Preview.app');host=app/'Contents/MacOS/reasonix-tauri';sidecar=app/'Contents/MacOS/reasonix-desktop-bridge'
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
assert hashlib.sha256(host.read_bytes()).hexdigest()=='d29503c849e5303eca92054bc8a6d4010e2bee1eaea90deb07d6ccc54a9c0822'
assert not w.package.matching_package_is_running(identifier)
output=Path(tempfile.mkdtemp(prefix='reasonix-d295-geometry-control-',dir='/private/tmp'));output.chmod(0o700)
print('Output: '+str(output),flush=True)
env={k:os.environ[k] for k in ('PATH','LANG','LC_ALL','USER','LOGNAME','SHELL','__CF_USER_TEXT_ENCODING') if k in os.environ}
geometry={'width':2000,'height':1400,'maximized':False,'x':920,'y':344,'scale_factor':2.0}
results=[]
for managed,seeded in [(True,False),(True,True),(False,True),(False,False)]:
 name=('managed' if managed else 'explicit')+('-seeded' if seeded else '-fresh');root=output/name;root.mkdir(mode=0o700);(root/'home').mkdir(mode=0o700);(root/'tmp').mkdir(mode=0o700)
 if seeded:
  data=root/'home/Library/Application Support'/identifier;data.mkdir(mode=0o700,parents=True);(data/'window-state.json').write_text(json.dumps(geometry))
 print('Case: '+name,flush=True)
 result={'name':name,'seed':geometry if seeded else None,'root':str(root)}
 try:
  w.launch(host,sidecar,root,identifier,managed,'menu-settings-minimized',verify_window_state=False,environment=env)
  result['status']='passed';w.print_window_trace(root/'tmp')
 except Exception as e:
  result.update(status='failed',error=str(e));print('Failed: '+str(e),flush=True)
 result['remainingSidecars']=w.package.own_sidecars(root/'tmp',sidecar)
 result['remainingReadiness']=len(list((root/'tmp').glob('reasonix-tauri-bridge-*/ready.json')))
 results.append(result);(output/'result.json').write_text(json.dumps(results,indent=2)+'\n')
print('Matrix: '+json.dumps([(r['name'],r['status']) for r in results]),flush=True)
print('Preview running: '+str(w.package.matching_package_is_running(identifier)),flush=True)
