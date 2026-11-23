#!/usr/bin/env python3
"""Run real Gateway API demonstrations, fail on incorrect results, save fresh text logs."""
import collections
import datetime
import http.cookiejar
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import threading
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
os.chdir(ROOT)
NS = 'rooms-strategies'
URL = os.environ.get('DEMO_URL', 'http://127.0.0.1:8888')
OUT = ROOT / 'evidence' / 'logs'
lock = threading.Lock()
log = None
start = 0

def emit(message):
    with lock:
        line = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='milliseconds') + ' ' + str(message) + '\n'
        print(line, end='', flush=True)
        if log:
            log.write(line); log.flush()

def cmd(*args):
    emit('$ ' + shlex.join(args))
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    emit(result.stdout.strip())
    if result.returncode:
        raise RuntimeError(f'command exited {result.returncode}')
    return result.stdout

def k(*args): return cmd('kubectl', '-n', NS, *args)

def request(path='/version', audience=None, request_id=None, opener=None, data=None):
    headers = {'Connection':'close'}
    if audience: headers['X-Audience'] = audience
    if request_id: headers['X-Request-ID'] = request_id
    if data is not None: headers['Content-Type']='application/json'
    req = urllib.request.Request(URL+path, headers=headers, data=json.dumps(data).encode() if data is not None else None)
    with (opener or urllib.request.build_opener()).open(req, timeout=5) as response:
        body = json.load(response)
        return body, response.headers['X-App-Version'], response.headers['X-Pod-Name']

def sample(count=100, audience=None, expected=None, prefix='sample'):
    versions, pods = collections.Counter(), collections.Counter()
    for i in range(count):
        body, version, pod = request(audience=audience,request_id=f'{prefix}-{i}')
        assert body['version']==version and body['pod']==pod
        versions[version]+=1; pods[pod]+=1
    emit(f'{prefix}: requests={count}, versions={dict(versions)}, pods={dict(pods)}')
    if expected: assert set(versions)==set(expected), f'expected {expected}, got {versions}'
    return versions

def route(name, expected=None):
    k('apply','-f', f'deploy/strategies/{name}.yaml')
    deadline=time.monotonic()+45
    while time.monotonic()<deadline:
        raw=subprocess.check_output(['kubectl','-n',NS,'get','httproute','rooms','-o','json'],text=True)
        doc=json.loads(raw)
        conditions=[c for p in doc.get('status',{}).get('parents',[]) for c in p.get('conditions',[])]
        if all(any(c['type']==t and c['status']=='True' and c.get('observedGeneration')==doc['metadata']['generation'] for c in conditions) for t in ['Accepted','ResolvedRefs']):break
        time.sleep(.5)
    else: raise RuntimeError('HTTPRoute not accepted')
    # Status acknowledges the controller; allow the proxy to receive its xDS update.
    time.sleep(3)
    k('get','httproute','rooms','-o','yaml')
    if expected:
        for _ in range(30):
            observed=[request()[1] for _ in range(10)]
            if set(observed)=={expected}:break
            time.sleep(.5)
        else:raise AssertionError(f'route did not converge to {expected}')

def rollout(version):
    k('set','image','deployment/rooms-stable',f'app=meeting-rooms:deploy-{version}')
    k('rollout','status','deployment/rooms-stable','--timeout=180s')

def snapshot():
    k('get','deploy,pods,svc')

def rolling():
    route('stable','v1')
    sample(30,expected=['v1'],prefix='before')
    done=threading.Event(); errors=[]; versions=collections.Counter()
    def traffic():
        while not done.is_set():
            try:
                body,version,pod=request(); versions[version]+=1
                emit(f'live request: version={version} pod={pod}')
            except Exception as error:
                errors.append(str(error)); emit(f'ERROR {error}')
            done.wait(.25)
    thread=threading.Thread(target=traffic);thread.start()
    try:
        k('set','image','deployment/rooms-stable','app=meeting-rooms:deploy-v2')
        while True:
            state=json.loads(subprocess.check_output(['kubectl','-n',NS,'get','deploy','rooms-stable','-o','json'], text=True))
            k('get','pods','-l','app=rooms-stable','-L','pod-template-hash')
            status=state.get('status',{})
            if status.get('observedGeneration',0)>=state['metadata']['generation'] and status.get('updatedReplicas')==2 and status.get('availableReplicas')==2 and status.get('replicas')==2:break
            time.sleep(2)
            if time.monotonic()-start>180:raise RuntimeError('rollout timed out')
    finally:done.set();thread.join()
    assert not errors,errors
    assert set(versions)=={'v1','v2'},versions
    sample(40,expected=['v2'],prefix='after')
    emit(f'PASS rolling: live versions={dict(versions)}, request_errors=0')

def blue_green():
    route('stable','v1');sample(60,expected=['v1'],prefix='blue')
    route('green','v2');sample(60,expected=['v2'],prefix='green switched')
    route('stable','v1');sample(60,expected=['v1'],prefix='rollback blue')
    emit('PASS blue/green: single route switch, both deployments remain ready, rollback verified')

def canary():
    for old,new in [(90,10),(50,50),(0,100)]:
        route(f'canary-{old}-{new}','v2' if old==0 else None)
        counts=sample(1000,expected=['v1','v2'] if old else ['v2'],prefix=f'canary {old}/{new}')
        assert abs(counts['v2']/1000-new/100)<.08,counts
    emit('PASS canary: all three ramps within 8 percentage points of target')

def ab():
    route('ab')
    sample(100,expected=['v1'],prefix='audience default')
    sample(100,audience='beta',expected=['v2'],prefix='audience beta')
    sample(100,audience='other',expected=['v1'],prefix='audience other')
    emit('PASS A/B: only X-Audience: beta receives v2')

def shadow():
    route('shadow','v1')
    marker='shadow-'+str(time.time_ns())
    for i in range(20):
        _,v,p=request(audience=f'{marker}-{i}', request_id=f'{marker}-{i}')
        assert v=='v1'
        emit(f'client correlation_id(X-Audience)={marker}-{i} version={v} pod={p}')
    time.sleep(3)
    records={}
    for name in ['rooms-stable','rooms-green']:
        raw=subprocess.check_output(['kubectl','-n',NS,'logs','-l',f'app={name}','--all-containers=true','--prefix=true','--tail=-1','--max-log-requests=10'], text=True)
        lines=[line for line in raw.splitlines() if marker in line]
        (OUT/f'shadow-{name}.log').write_text('\n'.join(lines)+'\n')
        for line in lines: emit(line)
        ids={json.loads(line[line.index('{'):])['audience'] for line in lines}
        records[name]=ids
        emit(f'{name}: correlated unique requests={len(ids)}')
    expected={f'{marker}-{i}' for i in range(20)}
    assert records['rooms-stable']==expected and records['rooms-green']==expected
    emit('PASS shadow: all 20 correlation IDs (X-Audience) in both backends; all client responses v1')

def state():
    route('canary-50-50')
    opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    user,_,_=request('/api/register',opener=opener,data={'username':'demo-'+str(time.time_ns()),'password':'local-test-password'})
    pods=set();versions=set()
    for _ in range(80):
        body,v,p=request('/api/me',opener=opener)
        assert body==user
        pods.add(p);versions.add(v)
    assert len(pods)>=4 and versions=={'v1','v2'},(pods,versions)
    emit(f'PASS shared PostgreSQL session: same user authenticated across {len(pods)} pods, versions={sorted(versions)}')

STRATEGIES={'rolling':rolling,'blue-green':blue_green,'canary':canary,'ab':ab,'shadow':shadow,'state':state}
def main():
    global log,start
    OUT.mkdir(parents=True, exist_ok=True)
    selected=sys.argv[1:] or list(STRATEGIES)
    for name in selected:
        if name not in STRATEGIES:raise SystemExit('unknown strategy '+name)
    # Always restore a known blue release before demonstrations.
    rollout('v1')
    for name in selected:
        start=time.monotonic()
        with (OUT/f'{name}.log').open('w') as log:
            emit(f'BEGIN {name} real minikube / Envoy Gateway at {URL}')
            try:
                if name!='rolling':rollout('v1')
                STRATEGIES[name]()
                snapshot();emit(f'END {name}: PASS')
            except Exception as error:
                emit(f'END {name}: FAIL: {type(error).__name__}: {error}')
                raise
        log=None
    route('stable')
    emit('All requested demonstrations passed. Final route: stable.')

if __name__=='__main__':main()
