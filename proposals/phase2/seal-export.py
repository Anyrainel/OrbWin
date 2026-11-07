"""Read-only source/data closure seal; writes only the NEW export receipt."""
import json,hashlib
from pathlib import Path
p=Path(__file__).resolve().parent;root=p.parents[1]
def sha(b):return hashlib.sha256(b).hexdigest()
# Crosscheck declared LF parent against actual canonical23 source manifest.
canonical=json.loads((root/'artifacts/qualification/p2-root-canonical-0023-generation-20261107-1/candidate/canonical-source-manifest.json').read_bytes())
expected={r['path'].removeprefix('rdd/'):r['sha256'] for r in canonical}
base=json.loads((p/'native-initial-scope3/rdd.patch.before-manifest.json').read_bytes())
assert expected=={r['path']:r['sha256'] for r in base}
files=[]
for q in sorted(p.rglob('*')):
 if not q.is_file() or q.name=='receipt.json':continue
 assert q.suffix not in ('.exe','.dll','.gz','.zip','.xz','.msi','.pfx','.p12','.key')
 b=q.read_bytes();files.append(dict(path=q.relative_to(p).as_posix(),bytes=len(b),sha256=sha(b)))
result=dict(schema=1,status='isolated-source-exports-reconstruction-pass-not-selected',canonicalParentProjectionMatched=True,canonicalGitTree='5300e3fff0cfa965301f261be311ba93241d2045',files=files,orderedSeries=False,activeLocksChanged=False,gitMutations=False,nativeOrBuildOrTargetRuns=False,packagingPeer='pending-or-separate-no-acceptance-claim')
(p/'receipt.json').write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8')
print(json.dumps(dict(receiptSHA256=sha((p/'receipt.json').read_bytes()),files=len(files),bytes=sum(r['bytes'] for r in files))))
