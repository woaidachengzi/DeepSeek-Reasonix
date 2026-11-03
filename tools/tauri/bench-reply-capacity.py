#!/usr/bin/env python3
"""Compare 0.1.73 encoding with capacity hints on synthetic replies only."""
import argparse, hashlib, json, os, re, statistics, subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SOURCES = ['desktop/cua-runtime/src/reply.rs', 'desktop/cua-runtime/examples/reply_pipeline.rs',
           'internal/computeruse/cua_reply_bench_test.go', 'tools/tauri/bench-reply-capacity.py']

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    out=args.output.resolve();out.mkdir(parents=True,exist_ok=False)
    hashes={p:hashlib.sha256((ROOT/p).read_bytes()).hexdigest() for p in SOURCES}
    env=dict(os.environ,REASONIX_HOME=str(out/'profile'),GOPROXY='off',GOSUMDB='off')
    def run(argv,name):
        with (out/name).open('w') as log:
            subprocess.run(list(map(str,argv)),cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,check=True)
        return (out/name).read_text()
    run(['cargo','build','--release','--locked','--offline','--manifest-path','desktop/cua-runtime/Cargo.toml','--example','reply_pipeline'],'build.log')
    binary=ROOT/'desktop/cua-runtime/target/release/examples/reply_pipeline'
    go=out/'pipe.test'
    run(['go','test','-c','-o',go,'./internal/computeruse'],'go-build.log')
    env['REASONIX_CUA_REPLY_BENCH_BIN']=str(binary)
    rust=[];pipe=[]
    for index in range(5):
        for route in (['previous','borrowed'] if index%2==0 else ['borrowed','previous']):
            for size in [65536,1048576,8388608]:
                row=json.loads(run([binary,'--bench',route,size,50],f'rust-{route}-{size}-{index}.json'))
                rust.append(dict(round=index,**row))
            raw=run([go,'-test.run=^$',f'-test.bench=^BenchmarkCuaReplyPipeline/.*/{route}$','-test.benchmem','-test.benchtime=50x','-test.count=1'],f'pipe-{route}-{index}.log')
            rows=re.findall(r'^BenchmarkCuaReplyPipeline/(\d+)KiB/\w+(?:-\d+)?\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',raw,re.M)
            assert len(rows)==3,raw
            pipe.extend(dict(round=index,route=route,base64_bytes=int(k)*1024,ns_per_op=float(ns),go_bytes_per_op=int(b),go_allocs_per_op=int(a)) for k,ns,b,a in rows)
    medians={}
    for name,rows,metrics in [('rust',rust,['ns_per_op','rust_allocated_bytes_per_op','rust_allocations_per_op']),('pipe',pipe,['ns_per_op','go_bytes_per_op','go_allocs_per_op'])]:
        medians[name]=[dict(route=route,base64_bytes=size,**{m:statistics.median(r[m] for r in rows if r['route']==route and r['base64_bytes']==size) for m in metrics}) for route in ['previous','borrowed'] for size in [65536,1048576,8388608]]
    assert hashes=={p:hashlib.sha256((ROOT/p).read_bytes()).hexdigest() for p in SOURCES}
    report=dict(baseline_commit='128999ab5ff83439effd1d1d087e676db783a2e1',baseline_route='previous: exact 0.1.73 serde_json::to_vec normal path, fixtures below cap',rounds=5,source_sha256=hashes,rust=rust,pipe=pipe,medians=medians,
                limits=['Synthetic replies; no capture, thumbnails, model or input.','Rust counts allocation requests separately from timing; not peak RSS.','Pipe includes warm worker, full Go decode and image identity validation.'])
    (out/'benchmark.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(medians,indent=2))

if __name__=='__main__':main()
