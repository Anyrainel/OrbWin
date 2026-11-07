"""Export dormant Source2 source only; no Git/build/native/target operation."""
if not __debug__: raise RuntimeError('Optimized Python refuses')
from pathlib import Path
import ast,json,hashlib
P=Path(__file__).resolve().parent
R=P.parents[2]
Q=R/'artifacts/qualification'
helper=P.parent/'export-source-components.py'
assert hashlib.sha256(helper.read_bytes()).hexdigest()=='bd8ae3ef98d9ab819bc75f3b4557c28c25241d71b775a753f5376a0aa1ee3c78'
# Reuse only the pinned pure source-data functions/imports. Never execute the
# historical exporter's top-level frozen-source reads or output creation.
tree=ast.parse(helper.read_bytes());nodes=[n for n in tree.body if isinstance(n,(ast.Import,ast.ImportFrom,ast.FunctionDef))]
env={};exec(compile(ast.Module(body=nodes,type_ignores=[]),str(helper),'exec'),env)
env['EXCLUDED']={'pkg/guestagent/lima-guestagent.amd64.gz','upgrade_PROGRESS.md','upgrade_SDK_TRANSITION.md','upgrade_boot_fence_snapshot.json'}
load=env['load'];export=env['export'];canonical=env['canonical'];pin=env['pin'];emit=env['emit'];dump=env['dump'];sha=env['sha']
BASE=Q/'p2-root-guest-foundations-convergence-source-20261107-2'
SOURCE=Q/'p2-setup-original-parent-bridge-source-20261107-2'
pin(SOURCE/'receipt.json','593d0af514b877d52c7445fbbedc353af86a661047509e76c2a1db0363dba7c0')
before=load(BASE,'source-manifest.json','rdd','c43df8117061deee32a9927ed91aeaabfa9bdfb44f4e9329b8965867d09cbd8a',1244)
after=load(SOURCE,'rdd-source-manifest.json','rdd','f2e8e72e579f654dcfc64048964d8e82813a7897986cf212908317588528093c',1248)
if not (P/'go').exists():
 go=export(P/'go','setup-parent-source2-go',canonical(before),canonical(after),before,after,'rdd.patch',dict(parentRawManifestSHA256='c43df8117061deee32a9927ed91aeaabfa9bdfb44f4e9329b8965867d09cbd8a',targetRawManifestSHA256='f2e8e72e579f654dcfc64048964d8e82813a7897986cf212908317588528093c',sourceReceiptSHA256='593d0af514b877d52c7445fbbedc353af86a661047509e76c2a1db0363dba7c0',blockedSource1PeerSHA256='6c110ab20cfa2b7f8bb5c917ae5e0890de53d9802b2b009e86cc1be6c82879b3',source2Peer='pending-separate'),None)
else:
 # First attempt completed Go reconstruction, then caught the deliberate
 # Source2 zero-Rust-delta before selecting Source1's original two-file delta.
 body=(P/'go/rdd.patch').read_bytes();m=json.loads((P/'go/rdd.patch.manifest.json').read_bytes())
 assert sha(body)==m['patchSHA256'] and env['replay'](canonical(before),body)==canonical(after)
 go=dict(component='setup-parent-source2-go',patch='rdd.patch',changes=m['changedFiles'],before=m['beforeFiles'],after=m['afterFiles'],patchSHA256=m['patchSHA256'])
assert go['changes']==9
assert json.loads((SOURCE/'desktop-delta.json').read_bytes())==[]
first=Q/'p2-setup-original-parent-bridge-source-20261107-1'
delta=json.loads(pin(first/'desktop-delta.json','da77f0efab227019eff10f29ac2c4c8fb53e64ce3e16a1e8fd9c11794d2a241c'));assert len(delta)==2
expected={'src-tauri/src/native_persistent_setup.rs':'a2b8afedfc390d860ecf2436e5698575b8e28de444e320e0f7cb1b57d3627054','src-tauri/src/persistent_setup_model.rs':'031f26c35a7dee94269059875fe37c6ea8c356c95395af5cb066549a6ee3ae41'}
rawBefore={};rawAfter={}
for row in delta:
 name=row['path'];assert name in expected
 a=(R/name).read_bytes();b=(SOURCE/'desktop'/name).read_bytes()
 assert sha(a)==row['before']['sha256'] and len(a)==row['before']['bytes']
 assert sha(b)==expected[name]==row['after']['sha256'] and len(b)==row['after']['bytes']
 # The two actual original preimages already LF; Root's unrelated14 CRLF
 # normalization/icon/schema context is deliberately not an input requirement.
 assert b'\r\n' not in a and b'\r\n' not in b
 rawBefore[name]=a;rawAfter[name]=b
desktop=export(P/'desktop','setup-parent-source2-desktop',rawBefore,rawAfter,rawBefore,rawAfter,'repo.patch',dict(parentFullContextManifestSHA256='35cbe228d6a61216d021b994dfbca7accf1507230e2c0e96e46104b360368c76',targetHistoricalFullContextManifestSHA256='cb7508547183b2a7c68d74672528921a9d9794d8e2ed71beb248640d7f0dc974',requiredActualContext='only exact two LF Rust preimages; full ignored icons/schema context NOT required',sourceReceiptSHA256='593d0af514b877d52c7445fbbedc353af86a661047509e76c2a1db0363dba7c0',source2Peer='pending-separate'),None)
emit(P/'verification.json',dump(dict(schema=1,components=[go,desktop],strictUnifiedPatchReplay=True,freshOwnedFilesystemExactTarget=True,selected=False,orderedSeries=False,gitMutations=False,buildOrNativeOrPackagePERuns=False)))
print(json.dumps([go,desktop]))
