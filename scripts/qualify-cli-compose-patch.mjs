// Pure source tests only: never calls Docker or changes engine/runtime state.
import {spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash,randomUUID} from 'node:crypto';
const root=fileURLToPath(new URL('../',import.meta.url)),cwd=resolve(root,'.upstream/compose');
const lock=JSON.parse(readFileSync(resolve(root,'upstream.lock.json'))),go=resolve(root,`.tools/go${lock.go.version}/go/bin/go.exe`);
const folder=resolve(root,'artifacts/compose',`tests-${randomUUID()}`);mkdirSync(folder,{recursive:true});
const hash=p=>createHash('sha256').update(readFileSync(p)).digest('hex');
const proof={schema:1,startedAt:new Date().toISOString(),patchSha256:hash(resolve(root,'patches/compose/0001-explicit-local-dockerfile-read.patch')),results:{},limits:['Full upstream Windows package tests cannot currently compile because pre-existing test helpers are excluded by !windows','Focused Windows tests compile every production GoFile plus selected upstream and new bake tests','Linux runs the selected tests from the complete package test binary; this is not the full upstream suite','No live engine or installed qualification']};
const baseEnv={...process.env,CGO_ENABLED:'0',GOTOOLCHAIN:'local',GOMAXPROCS:'2'};
function run(name,bin,args,env=baseEnv){const p=spawnSync(bin,args,{cwd,env,encoding:'utf8',windowsHide:true,timeout:240000,maxBuffer:8<<20});writeFileSync(join(folder,name+'.stdout'),p.stdout||'');writeFileSync(join(folder,name+'.stderr'),p.stderr||'');proof.results[name]={exitCode:p.status,error:p.error?String(p.error):null};return p;}
const required=(name,bin,args,env)=>{const p=run(name,bin,args,env);if(p.error||p.status!==0)throw Error(`${name} failed; inspect retained output`);return p.stdout;};
try{
 const windows={...baseEnv,GOOS:'windows',GOARCH:'amd64'};
 const upstream=run('windows-package-compile',go,['test','-mod=readonly','./pkg/compose','-run','^TestOrbWinDockerfile','-count=1'],windows);
 if(upstream.status!==0&&!/undefined: newStartTestService/.test(upstream.stderr))throw Error('Unexpected upstream Windows test compile failure');
 proof.windowsUpstreamCompileStatus=upstream.status===0?'passed':'pre-existing !windows helper compile failure retained';
 const pkg=JSON.parse(required('windows-package-files',go,['list','-json','./pkg/compose'],windows));
 required('windows-focused',go,['test','-mod=readonly',...pkg.GoFiles.map(p=>`pkg/compose/${p}`),'pkg/compose/build_bake_test.go','pkg/compose/build_bake_orbwin_test.go','-run','TestOrbWinDockerfile|TestBake|TestToBake','-count=1','-v'],windows);
 const binary=join(folder,'compose-bake-linux.test');
 required('linux-crosscompile',go,['test','-mod=readonly','-c','-o',binary,'./pkg/compose'],{...baseEnv,GOOS:'linux',GOARCH:'amd64'});
 const linuxPath='/mnt/'+binary[0].toLowerCase()+binary.slice(2).replaceAll('\\','/');
 const distributionID='{231810b0-fa21-48a8-b0a7-86f9a1360029}',wsl='C:/Windows/System32/wsl.exe';
 const uid=required('linux-uid',wsl,['--distribution-id',distributionID,'--user','vanyr','--exec','id','-u']).trim();if(uid!=='1000')throw Error('Exact WSL distribution is not running as UID1000');proof.distributionID=distributionID;proof.linuxUID=1000;
 required('linux-focused',wsl,['--distribution-id',distributionID,'--user','vanyr','--exec',linuxPath,'-test.run=TestOrbWinDockerfile|TestBake|TestToBake','-test.v=true','-test.count=1']);
 const manifest=JSON.parse(readFileSync(resolve(root,'artifacts/compose/compose-manifest.json'))),linuxRecord=manifest.binaries?.linuxAmd64;
 if(!linuxRecord)throw Error('Build the isolated Linux Compose candidate before qualifying it');
 const windowsBinary=resolve(root,'artifacts/compose/docker-compose.exe'),linuxBinary=resolve(root,'artifacts/compose',linuxRecord.path);
 if(hash(windowsBinary)!==manifest.binarySha256||hash(linuxBinary)!==linuxRecord.binarySha256)throw Error('Compose candidate checksum differs');
 const linuxComposePath='/mnt/'+linuxBinary[0].toLowerCase()+linuxBinary.slice(2).replaceAll('\\','/');
 const windowsVersion=required('windows-version',windowsBinary,['version','--short']).trim(),linuxVersion=required('linux-version',wsl,['--distribution-id',distributionID,'--user','vanyr','--exec',linuxComposePath,'version','--short']).trim();
 if(windowsVersion!=='5.6.1-orbwin.2'||linuxVersion!==windowsVersion)throw Error('Windows/Linux Compose version mismatch');
 proof.dependencyPatchSha256=hash(resolve(root,'patches/compose/0002-http2-dependency-floor.patch'));proof.manifestSha256=hash(resolve(root,'artifacts/compose/compose-manifest.json'));proof.candidateVersions={windows:windowsVersion,linux:linuxVersion};proof.binaryHashes={windows:hash(windowsBinary),linux:hash(linuxBinary)};proof.sourcePins=manifest.sourcePins;
 proof.success=true;
}catch(e){proof.success=false;proof.failure=String(e.stack||e);process.exitCode=1;}
finally{proof.finishedAt=new Date().toISOString();writeFileSync(join(folder,'proof.json'),JSON.stringify(proof,null,2)+'\n');console.log(join(folder,'proof.json'));}
