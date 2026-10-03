import hashlib,json,os,plistlib,subprocess,tempfile
from pathlib import Path
root=Path(tempfile.mkdtemp(prefix='reasonix-private-keychain-',dir='/private/tmp'));root.chmod(0o700)
home=root/'home';home.mkdir(mode=0o700)
prefs=home/'Library/Preferences';prefs.mkdir(parents=True,mode=0o700)
chains=home/'Library/Keychains';chains.mkdir(parents=True,mode=0o700)
path=chains/'fixture.keychain'
normal_env={k:os.environ[k] for k in ('PATH','HOME','USER','LOGNAME','LANG') if k in os.environ}
private_env=dict(normal_env,HOME=str(home),TMPDIR=str(root))
def run(args,env):
 p=subprocess.run(['/usr/bin/security']+args,env=env,text=True,capture_output=True,timeout=20)
 return {'exitCode':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
def snapshot():
 realprefs=Path(normal_env['HOME'])/'Library/Preferences/com.apple.security.plist'
 return {'default':run(['default-keychain','-d','user'],normal_env),'search':run(['list-keychains','-d','user'],normal_env),'preferencesSha256':hashlib.sha256(realprefs.read_bytes()).hexdigest() if realprefs.is_file() else None}
before=snapshot();(root/'normal-before.json').write_text(json.dumps(before,indent=2)+'\n')
identifier={'DbName':str(path),'GUID':'{87191ca3-0fc9-11d4-849a-000502b52122}','SubserviceType':6}
privateprefs=prefs/'com.apple.security.plist'
privateprefs.write_bytes(plistlib.dumps({'DefaultKeychain':[identifier],'DLDBSearchList':[identifier]}));privateprefs.chmod(0o600)
results={}
try:
 results['create']=run(['create-keychain','-p','reasonix-owned-test-fixture-only',str(path)],private_env)
 results['privateDefault']=run(['default-keychain','-d','user'],private_env)
 results['privateSearch']=run(['list-keychains','-d','user'],private_env)
 if results['create']['exitCode']!=0:raise RuntimeError('private keychain creation failed')
 if str(path) not in results['privateDefault']['stdout'] and str(path)+'-db' not in results['privateDefault']['stdout']:raise RuntimeError('private default did not resolve fixture')
 results['addDummy']=run(['add-generic-password','-s','reasonix-private-probe-only','-a','dummy','-w','reasonix-dummy-not-provider'],private_env)
 if results['addDummy']['exitCode']!=0:raise RuntimeError('private default write failed')
 results['findDummyInFixture']=run(['find-generic-password','-s','reasonix-private-probe-only','-a','dummy',str(path)],private_env)
 if results['findDummyInFixture']['exitCode']!=0:raise RuntimeError('dummy item missing from exact fixture')
 results['deleteDummy']=run(['delete-generic-password','-s','reasonix-private-probe-only','-a','dummy',str(path)],private_env)
 results['missingDummy']=run(['find-generic-password','-s','reasonix-private-probe-only','-a','dummy',str(path)],private_env)
 if results['deleteDummy']['exitCode']!=0 or results['missingDummy']['exitCode']==0:raise RuntimeError('dummy cleanup not proven')
 results['passed']=True
finally:
 after=snapshot();results['normalUnchanged']=before==after
 (root/'normal-after.json').write_text(json.dumps(after,indent=2)+'\n')
 (root/'result.json').write_text(json.dumps(results,indent=2)+'\n')
 print(json.dumps({'root':str(root),'passed':results.get('passed',False),'normalUnchanged':results['normalUnchanged']}),flush=True)
 if before!=after:raise RuntimeError('normal user keychain preferences changed')
