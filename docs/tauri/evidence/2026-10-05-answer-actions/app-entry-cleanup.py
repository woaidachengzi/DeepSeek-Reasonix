import pathlib,subprocess,json,hashlib,plistlib,time,os,shutil
repo=pathlib.Path('/Users/jerry/temp/app/reasonix-tauri');e=repo/'docs/tauri/evidence/2026-10-05-answer-actions'
ls='/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister'
query='kMDItemContentType == "com.apple.application-bundle" && (kMDItemFSName == "*Reasonix*"cd || kMDItemFSName == "*reasonix*"cd)'
formal=pathlib.Path('/Applications/Reasonix.app');formalExe=formal/'Contents/MacOS/reasonix-desktop'
formalBefore=hashlib.sha256(formalExe.read_bytes()).hexdigest()
previewPaths=[pathlib.Path('/Applications/Reasonix Tauri Preview.app'),repo/'desktop/tauri/target/release/bundle/portable/Reasonix Tauri Preview.app']
active=subprocess.check_output(['ps','-axo','pid=,comm='],text=True)
assert not any(str(p) in active for p in previewPaths),'preview is running; do not move it during use'
backup=repo/'desktop/tauri/target/preview-backups.noindex';backup.mkdir(parents=True,exist_ok=True)
before=subprocess.check_output(['mdfind',query],text=True).splitlines();moved=[]
for i,original in enumerate(previewPaths):
 for directory in backup.glob("old-"+str(i)+"-*"):
  restored=directory/"Reasonix Tauri Preview.app.rollback"
  if restored.exists():moved.append({"original":str(original),"backup":str(restored),"hostSha256":hashlib.sha256((restored/"Contents/MacOS/reasonix-tauri").read_bytes()).hexdigest(),"data":"application bundle only; profiles untouched"})
for i,p in enumerate(previewPaths):
 if not p.exists():continue
 info=plistlib.loads((p/'Contents/Info.plist').read_bytes());assert info['CFBundleIdentifier']=='io.reasonix.desktop.preview' and info['CFBundleExecutable']=='reasonix-tauri'
 destination=backup/('old-'+str(i)+'-'+str(time.time_ns()))/'Reasonix Tauri Preview.app.rollback';destination.parent.mkdir(mode=0o700)
 subprocess.run([ls,'-u',str(p)],check=True)
 oldSha=hashlib.sha256((p/'Contents/MacOS/reasonix-tauri').read_bytes()).hexdigest()
 shutil.move(str(p),str(destination));assert hashlib.sha256((destination/'Contents/MacOS/reasonix-tauri').read_bytes()).hexdigest()==oldSha
 moved.append({'original':str(p),'backup':str(destination),'hostSha256':oldSha,'data':'application bundle only; profiles untouched'})
new=repo/'desktop/tauri/target/release/bundle/macos.noindex/Reasonix Tauri Preview.app'
assert new.exists()
unregister=[]
for p in [repo/'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app',new]:
 result=subprocess.run([ls,'-u',str(p)],capture_output=True,text=True);unregister.append({'path':str(p),'exitCode':result.returncode,'output':result.stdout+result.stderr})
 if result.returncode and '-10814' not in result.stdout+result.stderr:result.check_returncode()
subprocess.run([ls,'-f',str(formal)],check=True)
assert hashlib.sha256(formalExe.read_bytes()).hexdigest()==formalBefore
for _ in range(20):
 after=subprocess.check_output(['mdfind',query],text=True).splitlines()
 if after==[str(formal)]:break
 time.sleep(.5)
result={'kept':str(formal),'keptVersion':'1.38.3','keptSha256':formalBefore,'beforeSpotlight':before,'previewUnregister':unregister,'afterSpotlight':after,'moves':moved,'newPreview':str(new),'compatibilityAlias':str(repo/'desktop/tauri/target/release/bundle/macos'),'noindexDirectory':True,'formalExecutableUnchanged':True,'userProfilesTouched':False,'formalReleasePerformed':False}
(e/'app-entry-cleanup.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False))
assert after==[str(formal)],'Spotlight index has not settled; keep concrete receipt and inspect remaining paths'
