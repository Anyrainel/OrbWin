import json,hashlib
from pathlib import Path
p=Path(__file__).resolve().parent;root=p.parents[2]
def sha(b):return hashlib.sha256(b).hexdigest()
def emit(n,x):(p/n).write_text(json.dumps(x,indent=2)+'\n',encoding='utf-8')
peer=root/'artifacts/qualification/p2-independent-setup-original-parent-bridge-source2-review-20261107-1/receipt.json'
expected='77b131d9098936823a7cf53cfd089cdd0d1f62a64bcdf4fed5d179a68a9a6f71'
assert sha(peer.read_bytes())==expected
emit('review-status.json',dict(source1='blocked-preserved',source1PeerSHA256='6c110ab20cfa2b7f8bb5c917ae5e0890de53d9802b2b009e86cc1be6c82879b3',source2='finite-source-peer-pass',source2PeerSHA256=expected,findingResolved='EDGE-A-PRECAPTURE-01',nativeOrActivationAcceptance=False))
files=[]
for q in sorted(p.rglob('*')):
 if not q.is_file() or q.name=='receipt.json':continue
 assert q.suffix not in ('.exe','.dll','.zip','.gz','.xz','.pfx','.p12','.key')
 b=q.read_bytes();files.append(dict(path=q.relative_to(p).as_posix(),bytes=len(b),sha256=sha(b)))
emit('receipt.json',dict(schema=1,status='isolated-source-export-reconstruction-pass-not-selected',sourceReceiptSHA256='593d0af514b877d52c7445fbbedc353af86a661047509e76c2a1db0363dba7c0',source2FinitePeerSHA256=expected,files=files,goProjectedSourceCounts=[1240,1244],desktopRequiredFiles=2,ignoredIconsSchemasRequired=False,orderedSeries=False,activeSelected=False,gitMutations=False,nativeBuildOrPERuns=False))
print(json.dumps(dict(receiptSHA256=sha((p/'receipt.json').read_bytes()),files=len(files),bytes=sum(x['bytes'] for x in files))))
