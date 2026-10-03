from pathlib import Path
import subprocess,plistlib,json,hashlib
root=Path('/private/tmp/reasonix-pure-cocoa-phase-r47v83oh');app=root/'Private Identical Cocoa Control.app';binary=app/'Contents/MacOS/control';binary.parent.mkdir(parents=True,exist_ok=True)
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
subprocess.run(['xcrun','swiftc','-module-cache-path','/private/tmp/reasonix-browser-handler/modules','-target','arm64-apple-macos11.0',str(root/'control.swift'),'-o',str(binary)],check=True)
(app/'Contents/Info.plist').write_bytes(plistlib.dumps({'CFBundleIdentifier':'io.reasonix.desktop.cocoa-identical-control','CFBundleName':'Private Identical Cocoa Control','CFBundleExecutable':'control','CFBundlePackageType':'APPL','CFBundleVersion':'1','NSHighResolutionCapable':True}))
subprocess.run(['codesign','--force','--sign','-','--entitlements',str(repo/'desktop/tauri/Entitlements.plist'),str(app)],check=True)
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
def entitlements(path):return plistlib.loads(subprocess.run(['codesign','-d','--entitlements',':-',str(path)],check=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE).stdout)
effective=entitlements(app);assert effective==entitlements(repo/'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app')
(root/'build.json').write_text(json.dumps({'app':str(app),'binarySha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'effectiveEntitlements':effective},indent=2)+'\n')
