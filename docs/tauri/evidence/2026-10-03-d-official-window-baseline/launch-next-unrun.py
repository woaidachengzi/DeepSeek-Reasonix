from pathlib import Path
import hashlib,importlib.util,json,os,plistlib,select,signal,subprocess,sys,tempfile,time
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
s=importlib.util.spec_from_file_location('profiles',repo/'tools/tauri/smoke-profile-import.py');profiles=importlib.util.module_from_spec(s);s.loader.exec_module(profiles);package=profiles.package
if any('.app/Contents/MacOS/reasonix-desktop' in command for pid,parent,command in package.processes()):raise SystemExit('Existing Wails GUI running; no baseline started')
app=Path('/private/tmp/reasonix-official-window-baseline/extracted/Reasonix.app');binary=app/'Contents/MacOS/reasonix-desktop'
assert hashlib.sha256(binary.read_bytes()).hexdigest()=='869b02d8f8a92f5847c1728923fde7153d931e105f26fd2926bfa8feeb863650'
subprocess.run(['codesign','--verify','--deep','--strict',str(app)],check=True)
root=Path(tempfile.mkdtemp(prefix='reasonix-official-physical-window-',dir='/private/tmp'));root.chmod(0o700)
for name in ('home','tmp','core','cache'):(root/name).mkdir(mode=0o700)
configuration=profiles.CONFIG.replace(b'[desktop]\n',b'[desktop]\nmetrics = false\ntelemetry = false\n');(root/'core/config.toml').write_bytes(configuration);(root/'core/config.toml').chmod(0o600)
env={'HOME':str(root/'home'),'TMPDIR':str(root/'tmp'),'REASONIX_HOME':str(root/'core'),'REASONIX_STATE_HOME':str(root/'core'),'REASONIX_CACHE_HOME':str(root/'cache'),'HTTP_PROXY':'http://127.0.0.1:9','HTTPS_PROXY':'http://127.0.0.1:9','NO_PROXY':'127.0.0.1,localhost'}
command=['/usr/bin/open','-n','-W',str(app),'--stdout',str(root/'host.log'),'--stderr',str(root/'host.log')]
for key,value in env.items():command+=['--env',key+'='+value]
opener=subprocess.Popen(command,env={k:os.environ[k] for k in ('PATH','LANG','USER','LOGNAME') if k in os.environ});monitor=select.kqueue();pid=None
try:
 deadline=time.monotonic()+20
 while time.monotonic()<deadline:
  rows=[p for p,parent,c in package.processes() if c==str(binary)]
  if len(rows)==1:pid=rows[0];break
  time.sleep(.05)
 assert pid is not None,'Official host not found'
 monitor.control([select.kevent(pid,filter=select.KQ_FILTER_PROC,flags=select.KQ_EV_ADD|select.KQ_EV_ONESHOT,fflags=select.KQ_NOTE_EXIT|0x04000000)],0,0)
 info={'app':str(app),'binary':str(binary),'root':str(root),'pid':pid,'openerPid':opener.pid};(root/'launch.json').write_text(json.dumps(info,indent=2)+'\n');print(json.dumps(info),flush=True)
 exits=monitor.control([],1,180);assert len(exits)==1 and os.WIFEXITED(exits[0].data),'No normal kernel exit receipt'
 info.update(kernelExitCode=os.WEXITSTATUS(exits[0].data),openExitCode=opener.wait(timeout=5));(root/'result.json').write_text(json.dumps(info,indent=2)+'\n');print(json.dumps(info),flush=True);assert info['kernelExitCode']==info['openExitCode']==0
except BaseException as error:
 info={'app':str(app),'root':str(root),'pid':pid,'openerPid':opener.pid,'status':'failed','errorType':type(error).__name__,'error':str(error)}
 (root/'failure.json').write_text(json.dumps(info,indent=2)+'\n');print(json.dumps(info),flush=True)
 raise
finally:
 monitor.close()
 if pid is not None and any(p==pid and c==str(binary) for p,parent,c in package.processes()):os.kill(pid,signal.SIGTERM)
 if opener.poll() is None:opener.terminate();opener.wait(timeout=5)
