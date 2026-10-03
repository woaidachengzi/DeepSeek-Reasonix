import hashlib,json,subprocess,tempfile
from pathlib import Path
dmg=Path('/Users/jerry/temp/app/reasonix-tauri/desktop/tauri/target/release/bundle/dmg/Reasonix Tauri Preview_0.1.0_aarch64.dmg')
root=Path(tempfile.mkdtemp(prefix='reasonix-d-keychain-copy-installed-',dir='/private/tmp'))
mount=root/'mount';mount.mkdir()
subprocess.run(['hdiutil','attach','-readonly','-nobrowse','-mountpoint',str(mount),str(dmg)],check=True)
app=root/'Reasonix Tauri Preview.app'
try:
 subprocess.run(['ditto',str(mount/app.name),str(app)],check=True)
finally:
 subprocess.run(['hdiutil','detach',str(mount)],check=True)
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
def digest(path):
 h=hashlib.sha256()
 with path.open('rb') as f:
  for b in iter(lambda:f.read(1024*1024),b''):h.update(b)
 return h.hexdigest()
info={'app':str(app),'dmg':str(dmg),'dmgSha256':digest(dmg),'artifactSha256':{n:digest(app/'Contents/MacOS'/n) for n in ('reasonix-tauri','reasonix-desktop-bridge')}}
Path('/private/tmp/reasonix-keychain-copy-install.json').write_text(json.dumps(info,indent=2)+'\n')
print(json.dumps(info))
