from pathlib import Path
import subprocess,tempfile,json,hashlib
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
dmg=repo/'desktop/tauri/target/release/bundle/dmg/Reasonix Tauri Preview_0.1.0_aarch64.dmg'
root=Path(tempfile.mkdtemp(prefix='reasonix-d-identity-installed-',dir='/private/tmp'))
root.chmod(0o700)
mount=root/'mount'
mount.mkdir()
subprocess.run(['hdiutil','verify',str(dmg)],check=True)
try:
    subprocess.run(['hdiutil','attach','-readonly','-nobrowse','-mountpoint',str(mount),str(dmg)],check=True)
    app=root/'Reasonix Tauri Preview.app'
    subprocess.run(['ditto',str(mount/app.name),str(app)],check=True)
    subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
finally:
    subprocess.run(['hdiutil','detach',str(mount)],check=True)
manifest={'app':str(app),'dmg':str(dmg),'sha256':{}}
for path in [dmg,app/'Contents/MacOS/reasonix-tauri',app/'Contents/MacOS/reasonix-desktop-bridge']:
    manifest['sha256'][path.name]=hashlib.file_digest(path.open('rb'),'sha256').hexdigest()
Path('/private/tmp/reasonix-d-identity-installed.json').write_text(json.dumps(manifest,indent=2)+'\n')
print('Installed app: '+str(app),flush=True)
result=subprocess.run(['python3','-B',str(repo/'tools/tauri/verify-installed-d.py'),str(app),'--output','/private/tmp/reasonix-d-identity-evidence','--gates','boundary','identity','package','profile','startup','window','lifetime'])
raise SystemExit(result.returncode)
