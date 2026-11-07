"""Source-only export. No Git mutation, build, package executable, or native API."""
if not __debug__:
    raise RuntimeError('Optimized Python refuses')
from pathlib import Path
import hashlib,json,difflib,re,tempfile

P=Path(__file__).resolve().parent
R=P.parents[1]
Q=R/'artifacts/qualification'
EXCLUDED={'pkg/guestagent/lima-guestagent.amd64.gz','upgrade_PROGRESS.md','upgrade_SDK_TRANSITION.md','upgrade_boot_fence_snapshot.json'}
INTO={'pkg/managementconfig/runtime_fresh_transport_windows.go','pkg/managementconfig/runtime_fresh_observation_windows.go'}
BASE=Q/'p2-root-fresh-codec-dial-convergence-source-20261107-1'
SCOPE=Q/'p2-initial-guest-native-scope-source-20261107-3'
GUEST=Q/'p2-selected-guest-product-entry-source-20261107-1'

def sha(b): return hashlib.sha256(b).hexdigest()
def dump(x): return (json.dumps(x,indent=2,ensure_ascii=False)+'\n').encode()
def emit(path,data):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('xb') as f: f.write(data)
def pin(path,expected):
    b=path.read_bytes()
    if sha(b)!=expected: raise RuntimeError('Pinned source input changed: '+str(path))
    return b
def load(directory,manifest,subdir,expected,count):
    rows=json.loads(pin(directory/manifest,expected));assert len(rows)==count
    out={}
    for row in rows:
        name=row['path'];assert name not in out and not name.startswith('/') and '..' not in Path(name).parts
        b=(directory/subdir/name).read_bytes()
        assert len(b)==row['bytes'] and sha(b)==row['sha256'],name
        out[name]=b
    return out
def rows(data): return [dict(path=n,bytes=len(b),sha256=sha(b)) for n,b in sorted(data.items())]
def canonical(data):
    assert EXCLUDED<=data.keys()
    return {n:(b if b'\0' in b else b.replace(b'\r\n',b'\n')) for n,b in data.items() if n not in EXCLUDED}
def patch(before,after):
    result=[]
    for name in sorted(before.keys()|after.keys()):
        a=before.get(name,b'');b=after.get(name,b'')
        if name in before and name in after and a==b: continue
        assert b'\0' not in a+b,'Changed binary source is not permitted'
        for line in difflib.diff_bytes(difflib.unified_diff,a.splitlines(True),b.splitlines(True),fromfile=('a/'+name).encode() if name in before else b'/dev/null',tofile=('b/'+name).encode() if name in after else b'/dev/null',n=3):
            if not line.endswith(b'\n'):
                result.extend([line+b'\n',b'\\ No newline at end of file\n'])
            else: result.append(line)
    return b''.join(result)
def replay(before,body):
    # Independent strict unified-hunk application; checks every removed/context
    # byte and original/new hunk counts, including final-newline markers.
    lines=body.splitlines(True);i=0;out=dict(before)
    while i<len(lines):
        assert lines[i].startswith(b'--- ')
        old=lines[i][4:].rstrip(b'\r\n').decode();i+=1
        assert lines[i].startswith(b'+++ ')
        new=lines[i][4:].rstrip(b'\r\n').decode();i+=1
        name=(new if new!='/dev/null' else old)[2:]
        assert (old=='/dev/null')==(name not in before)
        source=before.get(name,b'').splitlines(True);target=[];at=0
        while i<len(lines) and not lines[i].startswith(b'--- '):
            m=re.fullmatch(rb'@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@\n',lines[i]);assert m,lines[i]
            start=int(m[1]);remove=int(m[2] or b'1');add=int(m[4] or b'1');i+=1
            pos=start-1 if remove else start
            assert pos>=at;target.extend(source[at:pos]);at=pos;removed=added=0
            while removed<remove or added<add:
                line=lines[i];i+=1;tag=line[:1];text=line[1:]
                assert tag in (b' ',b'-',b'+')
                if i<len(lines) and lines[i]==b'\\ No newline at end of file\n':
                    text=text[:-1];i+=1
                if tag in (b' ',b'-'):
                    assert at<len(source) and source[at]==text,(name,at)
                    at+=1;removed+=1
                if tag in (b' ',b'+'):target.append(text);added+=1
            assert removed==remove and added==add
        target.extend(source[at:]);actual=b''.join(target)
        if new=='/dev/null':assert not actual;del out[name]
        else:out[name]=actual
    return out
def export(directory,label,before,after,raw_before,raw_after,patchname,parents,notes):
    assert not directory.exists();directory.mkdir()
    body=patch(before,after);emit(directory/patchname,body)
    recreated=replay(before,body);assert recreated==after
    # Fresh owned filesystem reconstruction, no source/tool execution.
    with tempfile.TemporaryDirectory(prefix='orbwin-source-export-') as tmp:
        t=Path(tmp)
        for name,b in recreated.items():
            q=t/name;q.parent.mkdir(parents=True,exist_ok=True);q.write_bytes(b)
        assert {q.relative_to(t).as_posix():q.read_bytes() for q in t.rglob('*') if q.is_file()}==after
    changes=[]
    for name in sorted(before.keys()|after.keys()):
        if before.get(name)==after.get(name):continue
        def fact(m):return None if name not in m else dict(bytes=len(m[name]),sha256=sha(m[name]))
        changes.append(dict(path=name,before=fact(before),after=fact(after),rawBefore=fact(raw_before),rawAfter=fact(raw_after)))
    emit(directory/(patchname+'.manifest.json'),dump(dict(schema=1,component=label,selected=False,orderedSeries=False,parents=parents,projection='LF for textual rdd source; four fixed historical exclusions omitted' if patchname=='rdd.patch' else 'byte-exact declared repository input subset',excluded=sorted(EXCLUDED) if patchname=='rdd.patch' else [],beforeFiles=len(before),afterFiles=len(after),beforeManifestSHA256=sha(dump(rows(before))),afterManifestSHA256=sha(dump(rows(after))),patchSHA256=sha(body),changedFiles=len(changes),changes=changes,strictPatchReplay=True,freshFilesystemExactTarget=True)))
    emit(directory/(patchname+'.before-manifest.json'),dump(rows(before)))
    emit(directory/(patchname+'.after-manifest.json'),dump(rows(after)))
    if notes:emit(directory/'NOTES.md',notes.encode())
    return dict(component=label,patch=patchname,changes=len(changes),before=len(before),after=len(after),patchSHA256=sha(body))

base=load(BASE,'source-manifest.json','rdd','c75ca01e63934fdd3af6b33fefe2947e99c22335452482449cf447e3d5dbc9a7',1214)
scope=load(SCOPE,'source-manifest.json','rdd','6591da268091dcf96759a46c9169cd4a13d7c04a3129da7d238e275ba222b322',1242)
guest=load(GUEST,'source-manifest.json','rdd','8a351c59cfaca07e4f0740b4f0ba947da3ebe7a235304a64988c57762aa0c190',1232)
restored=[]
for n in sorted(INTO):
    restored.append(dict(path=n,historicalScopeSHA256=sha(scope[n]),restoredCanonical23SHA256=sha(base[n])))
    scope[n]=base[n]
    assert guest[n]==base[n],n
pin(SCOPE/'receipt.json','aa286205b3c8f099fec0b9639e360bee753b90f7a831fafbc0a264fb0fad32b8')
pin(GUEST/'receipt.json','c839ea6d5989b6c32ad3ae294a6b104823e02f1a638231a7788e5c5f8acf5ecf')
parents=dict(canonical='23',canonicalGitTree='5300e3fff0cfa965301f261be311ba93241d2045',rawSourceManifestSHA256='c75ca01e63934fdd3af6b33fefe2947e99c22335452482449cf447e3d5dbc9a7',canonicalExclusions=sorted(EXCLUDED))
results=[]
results.append(export(P/'native-initial-scope3','native-initial-scope3',canonical(base),canonical(scope),base,scope,'rdd.patch',parents|dict(targetRawManifestSHA256='6591da268091dcf96759a46c9169cd4a13d7c04a3129da7d238e275ba222b322',targetRestorations=restored,sourceReceiptSHA256='aa286205b3c8f099fec0b9639e360bee753b90f7a831fafbc0a264fb0fad32b8',finiteSystemWalkPeerSHA256='93ec1e15f034268bfe9abf38ebc505a6fb3e6001c8046d184d4f0d12d7d14f05'),'''# Native initial scope3: isolated development proposal

Not selected by upstream.lock.json; no activation or native acceptance. Apply rdd.patch ONLY to the declared canonical23 rdd source projection, not after guest-entry or as an ordered series. Paths are relative to rdd/. Raw1242 Scope3 was based on older22; its two transport/observation callers are restored to EXACT common raw c75 Into2 before projection. Historical four exclusions are omitted from both. Raw and LF-projected delta identities are recorded; complete projected before/after manifests permit exact review/reconstruction. Copyright/license/test bytes are preserved except explicit textual LF projection.

Includes initial negative reader/schema/history/producer and post-real-Created initial scope. The reader intentionally refuses required-profile mutations while genuine continuation/association is absent; parsed DATA/argv/environment is not a capability. Scope source uses same actual Fresh/Created/image/authority, immutable archive, original native resources, fixed SystemWSL tool and one import role; local close/CLI0 cannot clear irreversible importEntered. No PID/reclaim/completion/guestauth activation.

Evidence: Source3 receipt aa286205b3c8f099fec0b9639e360bee753b90f7a831fafbc0a264fb0fad32b8;19 pure normal/race plus vet/three caller compile pass, check result ee9926b6c3858762e93535435892f5cfa337e296dea4ab9cad81cde2ee583925. Finite system-walk peer93ec1e15f034268bfe9abf38ebc505a6fb3e6001c8046d184d4f0d12d7d14f05; prior four FS fixes/command half have narrower evidence. No package PE/native execution is inferred. Source1/2 failed/blocked evidence stays historical, not rewritten.

Required gates: physical supported older-role denial/orphan preparation, genuine schema2 private-parent association, SAME-PE unknown retention at actual wiring, real driver/auth/worker joins, genuine NoGuest/checkpoint/completion, VHD SD/share qualification and paused native testing. Parent-source alignment and Root integration are required before any selection. This packet is source preservation, not Phase2 completion.
'''))
results.append(export(P/'guest-entry','guest-entry',canonical(base),canonical(guest),base,guest,'rdd.patch',parents|dict(targetRawManifestSHA256='8a351c59cfaca07e4f0740b4f0ba947da3ebe7a235304a64988c57762aa0c190',sourceReceiptSHA256='c839ea6d5989b6c32ad3ae294a6b104823e02f1a638231a7788e5c5f8acf5ecf'),'''# Guest mandatory entry: isolated development proposal

Not selected/activated. rdd.patch independently targets exact canonical23 rdd source after declared LF/four-exclusion projection. It is NOT ordered after native-initial-scope3. The separate repo.patch targets ONLY declared before-repo8 subset, producing repo13; it does not target an arbitrary current checkout and requires explicit parent/hash checks and Root composition. No selected lock or recipe is changed by this export.

Actual selected guest source has closed Docker/containerd role entry via private stdin; legacy entry is excluded under selected tag. Packaging source selects the tag, derives fixed unit masks/template and keeps identical guest output feeding overlay plus embedding. No real private host delivery, bootstrap/SSH/auth/driver connection is implemented by these DATA/source changes. Existing archives cannot be relabeled; require a coherent rebuild and NEW original immutable archive capture.

Source receipt c839ea6d5989b6c32ad3ae294a6b104823e02f1a638231a7788e5c5f8acf5ecf; checks summary38383542e89e47fbc1739cd1e5e1527b0d32446ce83d974f4fe52de728c51ad6 reports33 Go pure normal/race,25 Node,5 Rust pure tests plus selected/legacy guest and template compile-only checks. This export verifies reconstruction only, does not rerun those checks or execute archive/guest/native code. Packaging independent peer is pending/separate unless Root supplies a later exact receipt.

Required gates: independent packaging peer, coherent composition/reseal of actual build/SDK/offers, genuine archive rebuild/snapshot, native original delivery/network+SSH before provision and final role delivery after provision joins, real workload/native qualification and existing-root migration. No complete-protocol/authority/Phase2 claim. Preserve upstream copyright/license/tests; LF projection is explicit.
'''))
before_repo=load(GUEST,'before-repo-manifest.json','before-repo','6fc84490bf8506f461b4c5faa704af8dd5b8a5721b7db2e03da254603463c7e9',8)
after_repo=load(GUEST,'repo-source-manifest.json','repo','5c0284fccbf3e3be01460d19c4553047f601e91004c423280db9e854cbcbd867',13)
results.append(export(P/'guest-entry'/'repo-inputs','guest-entry-repo-inputs',before_repo,after_repo,before_repo,after_repo,'repo.patch',dict(declaredBeforeRepoManifestSHA256='6fc84490bf8506f461b4c5faa704af8dd5b8a5721b7db2e03da254603463c7e9',declaredAfterRepoManifestSHA256='5c0284fccbf3e3be01460d19c4553047f601e91004c423280db9e854cbcbd867'),None))
emit(P/'export-verification.json',dump(dict(schema=1,status='strict-source-reconstruction-pass',components=results,sourceOnly=True,selected=False,orderedSeries=False,gitMutations=False,buildOrNativeOrTargetRuns=False)))
print(json.dumps(results))
