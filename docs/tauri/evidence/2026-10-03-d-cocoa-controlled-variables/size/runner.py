import hashlib,importlib.util,json,os,select,signal,subprocess,sys,tempfile,time
from pathlib import Path
sys.dont_write_bytecode=True
s=importlib.util.spec_from_file_location('package',Path('/Users/jerry/temp/app/reasonix-tauri/tools/tauri/smoke-packaged-app.py'));package=importlib.util.module_from_spec(s);s.loader.exec_module(package)
if package.matching_package_is_running('io.reasonix.desktop.preview'):raise SystemExit('Preview running')
original=Path('/private/tmp/reasonix-cocoa-size-control/Private Size Cocoa Control.app')
subprocess.run(['codesign','--verify','--deep','--strict',str(original)],check=True)
signature=subprocess.run(['codesign','-d','--verbose=4',str(original)],text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True)
# This new control is compiled from the saved fixed source with explicit minos 11.0.
root=Path(tempfile.mkdtemp(prefix='reasonix-cocoa-size-control-',dir='/private/tmp'));root.chmod(0o700)
home=root/'home';home.mkdir(mode=0o700);tmp=root/'tmp';tmp.mkdir(mode=0o700);app=root/original.name
subprocess.run(['ditto',str(original),str(app)],check=True)
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
binary=app/'Contents/MacOS/control'
command=['/usr/bin/open','-n','-W',str(app),'--stdout',str(root/'host.log'),'--stderr',str(root/'host.log'),'--env','HOME='+str(home),'--env','TMPDIR='+str(tmp)]
opener=subprocess.Popen(command,env={k:os.environ[k] for k in ('PATH','LANG','USER','LOGNAME') if k in os.environ})
host=None;monitor=select.kqueue()
try:
 deadline=time.monotonic()+5
 while time.monotonic()<deadline:
  rows=[pid for pid,parent,cmd in package.processes() if cmd==str(binary)]
  if len(rows)==1:host=rows[0];break
  time.sleep(.02)
 assert host is not None,'control host unavailable'
 monitor.control([select.kevent(host,filter=select.KQ_FILTER_PROC,flags=select.KQ_EV_ADD|select.KQ_EV_ONESHOT,fflags=select.KQ_NOTE_EXIT|0x04000000)],0,0)
 exits=monitor.control([],1,20)
 assert len(exits)==1 and os.WIFEXITED(exits[0].data),'control exit unavailable'
 code=os.WEXITSTATUS(exits[0].data);opener_code=opener.wait(timeout=5)
 records=[json.loads(line) for line in (root/'host.log').read_text().splitlines() if line.startswith('{')]
 receipt={'root':str(root),'app':str(app),'pid':host,'kernelExitCode':code,'openExitCode':opener_code,'binarySha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'passed':code==0 and records[-1]['stage']=='passed','records':records}
 (root/'result.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt),flush=True)
 assert receipt['passed'],'independent Cocoa control failed'
finally:
 monitor.close()
 if host is not None and any(pid==host and cmd==str(binary) for pid,parent,cmd in package.processes()):os.kill(host,signal.SIGTERM)
 if opener.poll() is None:opener.terminate();opener.wait(timeout=5)
