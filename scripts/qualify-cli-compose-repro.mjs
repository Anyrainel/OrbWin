// Rebuild only isolated candidates; never installs or runs an engine command.
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash,randomUUID} from 'node:crypto';
const root=fileURLToPath(new URL('../',import.meta.url)),output=resolve(root,'artifacts/compose');
const folder=join(output,`repro-${randomUUID()}`);mkdirSync(folder);
const hash=p=>createHash('sha256').update(readFileSync(p)).digest('hex');
const manifestPath=join(output,'compose-manifest.json'),before=JSON.parse(readFileSync(manifestPath));
const proof={schema:1,startedAt:new Date().toISOString(),before:{},limits:['Same-host repeat-build evidence for the pinned compiler/source recipe','No engine, plugin discovery or installed acceptance claim']};
try{
 for(const [key,record]of Object.entries(before.binaries)){const actual=hash(join(output,record.path));if(actual!==record.binarySha256)throw Error('Existing candidate differs from its manifest');proof.before[key]=actual;}
 if(hash(resolve(root,before.sourcePins.buildScript))!==before.sourcePins.buildScriptSha256)throw Error('Recipe source changed before reproducibility check');
 writeFileSync(join(folder,'manifest-before.json'),JSON.stringify(before,null,2)+'\n');
 const p=spawnSync(process.execPath,['scripts/build-compose.mjs'],{cwd:root,encoding:'utf8',windowsHide:true,timeout:240000,maxBuffer:4<<20});
 writeFileSync(join(folder,'build.stdout'),p.stdout||'');writeFileSync(join(folder,'build.stderr'),p.stderr||'');if(p.error||p.status!==0)throw Error('Repeat build failed: '+(p.error||p.status));
 const after=JSON.parse(readFileSync(manifestPath));writeFileSync(join(folder,'manifest-after.json'),JSON.stringify(after,null,2)+'\n');
 proof.after={};for(const [key,record]of Object.entries(after.binaries))proof.after[key]=hash(join(output,record.path));
 if(JSON.stringify(proof.before)!==JSON.stringify(proof.after)||JSON.stringify(before.binaries)!==JSON.stringify(after.binaries)||JSON.stringify(before.sourcePins)!==JSON.stringify(after.sourcePins))throw Error('Repeat build changed candidate or source/license manifest identity');
 proof.sourcePins=after.sourcePins;proof.success=true;
}catch(e){proof.success=false;proof.failure=String(e.stack||e);process.exitCode=1;}
finally{proof.finishedAt=new Date().toISOString();writeFileSync(join(folder,'proof.json'),JSON.stringify(proof,null,2)+'\n');console.log(join(folder,'proof.json'));}
