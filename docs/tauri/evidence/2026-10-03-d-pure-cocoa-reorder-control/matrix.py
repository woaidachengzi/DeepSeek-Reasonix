import subprocess,sys
from pathlib import Path
root=Path(__file__).parent
for index,mode in enumerate(("loaded","none","loaded","none")):
 with (root/(str(index)+"-"+mode+".log")).open("w") as log:
  result=subprocess.run([sys.executable,str(root/"runner.py"),mode],stdout=log,stderr=subprocess.STDOUT)
 print(index,mode,result.returncode,flush=True)
 if result.returncode:raise SystemExit(result.returncode)
