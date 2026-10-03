import importlib.util,json,plistlib,tempfile,shutil,sys,subprocess,time,threading,hashlib
from pathlib import Path
sys.dont_write_bytecode=True
repo=Path('/Users/jerry/temp/app/reasonix-tauri')
spec=importlib.util.spec_from_file_location('windows',repo/'tools/tauri/smoke-native-window.py');windows=importlib.util.module_from_spec(spec);spec.loader.exec_module(windows)
app=Path('/private/tmp/reasonix-d-post-review-installed-5o3uy087/Reasonix Tauri Preview.app')
identifier=plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']
if windows.package.matching_package_is_running(identifier):raise RuntimeError('Preview already running')
seed=repo/'docs/tauri/evidence/2026-10-03-d-post-review-candidate/window-failure'
output=Path('/private/tmp/reasonix-persistence-timeline');output.mkdir(mode=0o700)
source=repo/'tools/tauri/activation-timeline.swift';observer=output/'observer'
subprocess.run(['swiftc',str(source),'-o',str(observer)],check=True)
shutil.copy2(source,output/source.name)
(output/'identity.json').write_text(json.dumps({str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in [source,app/'Contents/MacOS/reasonix-tauri',app/'Contents/MacOS/reasonix-desktop-bridge',seed/'window-state.json',seed/'reasonix-native-window-normal.json']},indent=2)+'\n')
for managed in [True,False]:
 root=Path(tempfile.mkdtemp(prefix='reasonix-persistence-timeline-',dir='/private/tmp'));root.chmod(0o700)
 (root/'home').mkdir(mode=0o700);(root/'tmp').mkdir(mode=0o700)
 state=root/'home/Library/Application Support'/identifier/'window-state.json';state.parent.mkdir(parents=True)
 shutil.copy2(seed/'window-state.json',state)
 shutil.copy2(seed/'reasonix-native-window-normal.json',root/'tmp/reasonix-native-window-normal.json')
 for phase in ['restore-normal','second-instance']:
  receipt=output/('managed' if managed else 'explicit')/phase;receipt.mkdir(parents=True)
  shutil.copy2(state,receipt/'state-before.json')
  stop=threading.Event()
  def observe_disk():
   previous=None
   with (receipt/'persistence.jsonl').open('w') as trace:
    while not stop.is_set():
     try:
      value=state.read_bytes()
      if value!=previous:
       parsed=json.loads(value);previous=value
       trace.write(json.dumps({'unixMs':int(time.time()*1000),'state':parsed})+'\n');trace.flush()
     except (OSError,ValueError):pass
     stop.wait(.05)
  thread=threading.Thread(target=observe_disk);thread.start()
  events={'phase':phase,'managed':managed,'root':str(root),'startedUnixMs':int(time.time()*1000)}
  with (receipt/'activation.jsonl').open('wb') as trace,(receipt/'observer.log').open('wb') as errors:
   process=subprocess.Popen([str(observer)],stdout=trace,stderr=errors)
   try:
    deadline=time.monotonic()+5
    while (receipt/'activation.jsonl').stat().st_size==0:
     if process.poll() is not None or time.monotonic()>deadline:raise RuntimeError('observer not ready')
     time.sleep(.01)
    windows.launch(app/'Contents/MacOS/reasonix-tauri',app/'Contents/MacOS/reasonix-desktop-bridge',root,identifier,managed,phase,launch_services=True)
    if process.poll() is not None:raise RuntimeError('observer exited before phase completion')
    events['passed']=True
   except BaseException as error:
    events.update(passed=False,error=str(error));raise
   finally:
    if process.poll() is None:process.terminate()
    process.wait(timeout=5);stop.set();thread.join(timeout=2)
    events.update(finishedUnixMs=int(time.time()*1000),observerExit=process.returncode)
    shutil.copy2(state,receipt/'state-after.json')
    for p in (root/'tmp').glob('*.json*'):
     if p.name.startswith(('reasonix-native-window-','launch-services-')):shutil.copy2(p,receipt/p.name)
    (receipt/'receipt.json').write_text(json.dumps(events,indent=2)+'\n')
    print(json.dumps(events),flush=True)
