import importlib.util,json,os,hashlib,subprocess
from pathlib import Path
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py')
w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
app=Path('/private/tmp/reasonix-d-message-menu-installed-crpdlm9f/Reasonix Tauri Preview.app')
root=Path('/private/tmp/reasonix-native-window-smoke-56bucily')
identifier='io.reasonix.preview'
import plistlib
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
for folder in (root,root/'home',root/'tmp'):
 assert folder==folder.resolve() and not folder.is_symlink() and folder.is_dir()
 assert folder.stat().st_uid==os.getuid() and folder.stat().st_mode&0o777==0o700
assert (root/'tmp/reasonix-native-window-normal.json').is_file()
assert not w.package.matching_package_is_running(identifier)
host=app/'Contents/MacOS/reasonix-tauri';sidecar=app/'Contents/MacOS/reasonix-desktop-bridge'
expected={'reasonix-tauri':'f259ddc90191a09d7f5418b30579c6c9ca89e754e9ad456ccf5001d241ee7ca3','reasonix-desktop-bridge':'156f1d447b179408fd2a803ee496d1be96d31a8c9b870f0deb637d02b3335311'}
for p in (host,sidecar):assert hashlib.sha256(p.read_bytes()).hexdigest()==expected[p.name]
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
identity=w.package.check_credential_profile(root/'core')
out=Path('/private/tmp/reasonix-message-menu-window-remaining');out.mkdir(mode=0o700)
summary={'scope':'Independent remaining nonclipboard phases on retained explicit fixture; not a full uninterrupted 48-stage pass','root':str(root),'artifactSha256':expected,'phases':[]}
for phase in ('second-instance','close-quit','restore-close-quit'):
 entry={'phase':phase,'status':'running'};summary['phases'].append(entry)
 try:
  actual=w.launch(host,sidecar,root,identifier,False,phase,launch_services=True)
  assert actual==identity
  entry['status']='passed'
 except BaseException as error:
  entry.update(status='failed',error=str(error));raise
 finally:
  receipt=out/phase;receipt.mkdir()
  for name in ('reasonix-native-window-result.json','reasonix-native-window-trace.jsonl','reasonix-native-window-normal.json',f'launch-services-{phase}-exit.json'):
   source=root/'tmp'/name
   if source.is_file():(receipt/name).write_bytes(source.read_bytes())
  (out/'result.json').write_text(json.dumps(summary,indent=2)+'\n')
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
for p in (host,sidecar):assert hashlib.sha256(p.read_bytes()).hexdigest()==expected[p.name]
print('Independent explicit remaining nonclipboard phases and identity/signature: OK')
