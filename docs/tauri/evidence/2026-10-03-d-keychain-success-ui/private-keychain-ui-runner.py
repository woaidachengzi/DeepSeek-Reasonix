import hashlib,importlib.util,json,os,plistlib,subprocess,sys
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
s=importlib.util.spec_from_file_location('probe',repo/'tools/tauri/probe-launch-services-profile.py');probe=importlib.util.module_from_spec(s);s.loader.exec_module(probe)
app=Path('/private/tmp/reasonix-d-theme-boundary-installed-b4f0dmtp/Reasonix Tauri Preview.app');identifier='io.reasonix.desktop.preview'
if probe.package.matching_package_is_running(identifier):raise SystemExit('Preview running; fixture not started')
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
real_popen=subprocess.Popen
seeded=False
def fixture_popen(command,*args,**kwargs):
 global seeded
 if isinstance(command,list) and command[:4]==['/usr/bin/open','-n','-W',str(app)]:
  if seeded:raise RuntimeError('unexpected second fixture launch')
  env={command[i+1].split('=',1)[0]:command[i+1].split('=',1)[1] for i,x in enumerate(command) if x=='--env'}
  home=Path(env['HOME']);root=home.parent
  assert root.parent==Path('/private/tmp') and root.name.startswith('reasonix-launch-services-')
  prefs=home/'Library/Preferences';prefs.mkdir(parents=True,mode=0o700)
  chains=home/'Library/Keychains';chains.mkdir(parents=True,mode=0o700)
  keychain=chains/'fixture.keychain'
  entry={'DbName':str(keychain),'GUID':'{87191ca3-0fc9-11d4-849a-000502b52122}','SubserviceType':6}
  preference=prefs/'com.apple.security.plist'
  preference.write_bytes(plistlib.dumps({'DefaultKeychain':[entry],'DLDBSearchList':[entry]}));preference.chmod(0o600)
  private_env={k:os.environ[k] for k in ('PATH','USER','LOGNAME','LANG') if k in os.environ};private_env.update(HOME=str(home),TMPDIR=str(root/'tmp'))
  result=subprocess.run(['/usr/bin/security','create-keychain','-p','reasonix-owned-test-fixture-only',str(keychain)],env=private_env,text=True,capture_output=True,timeout=20)
  (root/'keychain-create.json').write_text(json.dumps({'exitCode':result.returncode,'stdout':result.stdout,'stderr':result.stderr,'path':str(keychain)},indent=2)+'\n')
  if result.returncode:raise RuntimeError('private keychain creation failed')
  verify=subprocess.run(['/usr/bin/security','default-keychain','-d','user'],env=private_env,text=True,capture_output=True,timeout=20)
  if verify.returncode or str(keychain) not in verify.stdout:raise RuntimeError('private default unresolved')
  data=home/'Library/Application Support'/identifier;data.mkdir(parents=True,mode=0o700)
  legacy=data/'keychain.dat';value=json.dumps({'api_key_deepseek-flash':'reasonix-private-dummy-no-real-provider-access'})+'\n'
  legacy.write_text(value);legacy.chmod(0o600)
  info={'path':str(legacy),'sha256':hashlib.sha256(legacy.read_bytes()).hexdigest(),'size':legacy.stat().st_size,'mode':oct(legacy.stat().st_mode & 0o777),'mtimeNs':legacy.stat().st_mtime_ns,'provider':'deepseek-flash','dummyOnly':True}
  (root/'legacy-before.json').write_text(json.dumps(info,indent=2)+'\n');seeded=True
 return real_popen(command,*args,**kwargs)
subprocess.Popen=fixture_popen
try:probe.launch(app,identifier,False,interactive=True,observe=True,wait_seconds=900)
finally:subprocess.Popen=real_popen
