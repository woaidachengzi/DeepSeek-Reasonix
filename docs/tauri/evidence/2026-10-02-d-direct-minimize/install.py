from pathlib import Path
import subprocess,tempfile,json,hashlib
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
dmg=repo/'desktop/tauri/target/release/bundle/dmg/Reasonix Tauri Preview_0.1.0_aarch64.dmg'
root=Path(tempfile.mkdtemp(prefix='reasonix-d-direct-installed-',dir='/private/tmp'))
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
Path('/private/tmp/reasonix-d-direct-installed.json').write_text(json.dumps(manifest,indent=2)+'\n')
print('Installed app: '+str(app),flush=True)
evidence=Path('/private/tmp/reasonix-d-direct-evidence')
evidence.mkdir(mode=0o700)
manifest.update(sourceHead=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip(),dirty=True,gates=[])
patch=subprocess.check_output(['git','diff','--binary','HEAD','--'],cwd=repo)
(evidence/'source.patch').write_bytes(patch)
manifest['sourcePatchSha256']=hashlib.sha256(patch).hexdigest()
for name,script,args in [('direct-control','smoke-native-menu-window.py',['--direct-only']),('package','smoke-packaged-app.py',[])]:
    log=evidence/(name+'.log')
    with log.open('w') as out:
        result=subprocess.run(['python3','-B',str(repo/'tools/tauri'/script),str(app),*args],stdout=out,stderr=subprocess.STDOUT)
    manifest['gates'].append({'name':name,'script':script,'args':args,'exitCode':result.returncode,'log':log.name})
    (evidence/'result.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(name+': exit '+str(result.returncode),flush=True)
    subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
raise SystemExit(1 if any(g['exitCode'] for g in manifest['gates']) else 0)
