import hashlib,importlib.util,json,os,plistlib,select,signal,subprocess,sys,tempfile,time
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
s=importlib.util.spec_from_file_location('package',repo/'tools/tauri/smoke-packaged-app.py');package=importlib.util.module_from_spec(s);s.loader.exec_module(package)
if package.matching_package_is_running('io.reasonix.desktop.preview'):
 raise SystemExit('Preview is running; no baseline started')
base=Path(tempfile.mkdtemp(prefix='reasonix-tauri-window-baseline-',dir='/private/tmp'));base.chmod(0o700)
app=base/'Reasonix Window Baseline.app';binary=app/'Contents/MacOS/window-baseline';binary.parent.mkdir(parents=True)
subprocess.run(['ditto',str(repo/'desktop/tauri/target/release/examples/window-baseline'),str(binary)],check=True)
info={'CFBundleIdentifier':'io.reasonix.desktop.window-baseline','CFBundleName':'Reasonix Window Baseline','CFBundleDisplayName':'Reasonix Window Baseline','CFBundleExecutable':'window-baseline','CFBundlePackageType':'APPL','CFBundleVersion':'1','NSHighResolutionCapable':True}
with (app/'Contents/Info.plist').open('wb') as f:plistlib.dump(info,f)
subprocess.run(['codesign','--force','--sign','-','--entitlements',str(repo/'desktop/tauri/Entitlements.plist'),str(binary)],check=True)
subprocess.run(['codesign','--force','--sign','-',str(app)],check=True)
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
print(json.dumps({'app':str(app),'binarySha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'base':str(base)}),flush=True)
kind=os.environ.get('REASONIX_WINDOW_BASELINE_KIND','tauri')
for style in (('standard',) if kind=='cocoa' else ('standard','overlay')):
 root=base/style;root.mkdir(mode=0o700);home=root/'home';home.mkdir(mode=0o700);tmp=root/'tmp';tmp.mkdir(mode=0o700)
 command=['/usr/bin/open','-n','-W',str(app),'--stdout',str(root/'host.log'),'--stderr',str(root/'host.log')]
 for key,value in {'HOME':str(home),'TMPDIR':str(tmp),'REASONIX_WINDOW_BASELINE_DIR':str(root),'REASONIX_WINDOW_BASELINE_STYLE':style,'REASONIX_WINDOW_BASELINE_KIND':kind}.items():command+=['--env',key+'='+value]
 opener=subprocess.Popen(command,stdin=subprocess.DEVNULL,env={k:os.environ[k] for k in ('PATH','LANG','USER','LOGNAME') if k in os.environ})
 host=None;monitor=select.kqueue()
 try:
  deadline=time.monotonic()+5
  while time.monotonic()<deadline:
   rows=[pid for pid,parent,cmd in package.processes() if cmd==str(binary)]
   if len(rows)==1:
    host=rows[0];break
   time.sleep(.02)
  if host is None:raise RuntimeError('baseline host unavailable')
  monitor.control([select.kevent(host,filter=select.KQ_FILTER_PROC,flags=select.KQ_EV_ADD|select.KQ_EV_ONESHOT,fflags=select.KQ_NOTE_EXIT|0x04000000)],0,0)
  exits=monitor.control([],1,20)
  if len(exits)!=1 or not os.WIFEXITED(exits[0].data):raise RuntimeError('baseline kernel exit unavailable')
  code=os.WEXITSTATUS(exits[0].data);open_code=opener.wait(timeout=5)
  result=json.loads((root/'result.json').read_text())
  expected=0 if result['restored'] else 2
  receipt={'style':style,'hostPid':host,'kernelExitCode':code,'openExitCode':open_code,'expectedCode':expected}
  (root/'exit-receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
  print(json.dumps(receipt),flush=True)
  assert result['precondition'] is True,'baseline activation/page precondition failed'
  assert code==expected,'baseline kernel exit receipt mismatch'
  print(json.dumps({'style':style,'root':str(root),'pid':host,'exitCode':code,'precondition':result['precondition'],'minimized':result['minimized'],'restored':result['restored']}),flush=True)
 finally:
  monitor.close()
  if host is not None and any(pid==host and cmd==str(binary) for pid,parent,cmd in package.processes()):os.kill(host,signal.SIGTERM)
  if opener.poll() is None:opener.terminate();opener.wait(timeout=5)
