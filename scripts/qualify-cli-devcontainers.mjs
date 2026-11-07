// Sole-engine-mutation-lane qualification; explicit nonce fixtures only.
import {spawnSync} from 'node:child_process';
import {mkdirSync,readFileSync,writeFileSync,existsSync,copyFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {fileURLToPath} from 'node:url';
if(process.env.ORBWIN_QUALIFY_DEVCONTAINERS!=='1')throw Error('Explicit exclusive mutation-lane opt-in required');
const root=fileURLToPath(new URL('../',import.meta.url));
const token=randomUUID().replaceAll('-',''),name=`orbwin-devcontainer-${token}`;
const wslMode=process.env.ORBWIN_QUALIFY_DEVCONTAINERS_WSL==='1';
const composeMode=process.env.ORBWIN_QUALIFY_DEVCONTAINERS_COMPOSE==='1';
const candidateCompose=process.env.ORBWIN_QUALIFY_COMPOSE_CANDIDATE;
const sharedRoot='/mnt/wsl/orbwin/projects/710eb4f4d7f72b0e329a51d3';
const evidence=resolve(root,'artifacts/cli',name),workspace=wslMode?`\\\\wsl.localhost\\Ubuntu\\home\\vanyr\\orbwin-native-proof-20261024\\${name}`:join(process.env.LOCALAPPDATA,'Temp',name);
if(wslMode){const p=spawnSync('C:/Windows/System32/wsl.exe',['--distribution','Ubuntu','--user','vanyr','--exec','/bin/sh','-c',`test "$(id -u)" = 1000 && test "$(cat /etc/machine-id)" = 5563914b91c14609814e3657a227cff1 && test "$(stat -Lc '%d:%i' /home/vanyr/orbwin-native-proof-20261024)" = 2112:1374236 && test "$(stat -Lc '%d:%i' ${sharedRoot})" = 2112:1374236`],{encoding:'utf8',windowsHide:true,timeout:15000});if(p.error||p.status!==0)throw Error('WSL project root/machine/UID identity changed; no fixture directory created');}
mkdirSync(evidence);mkdirSync(workspace);mkdirSync(join(workspace,'.devcontainer'));
const wrapper=resolve(process.env.ORBWIN_QUALIFY_DOCKER_WRAPPER||join(root,'artifacts/cli/qualification-29247c23-aee8-485d-a391-eee3ca1060c9/orbwin-docker.exe'));
const cli=join(process.env.USERPROFILE,'.vscode/extensions/ms-vscode-remote.remote-containers-0.469.0/dist/spec-node/devContainersSpecCLI.js');
const docker=resolve(root,'artifacts/runtime-tools/bin/docker.exe');
const owned='npipe:////./pipe/docker-rancher-desktop-orbwin',desktop='npipe:////./pipe/dockerDesktopLinuxEngine';
const expected={owned:'d9c00194-ea9d-439a-8561-6a0ec580fbd5',desktop:'a307ff0e-581d-49d3-8aee-a0636dda9137',rancher:'741eb2cb-adf5-4676-bf28-f03f68d54f60'};
const label='dev.orbwin.qualification',image='sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63',alias=name+':cached';
let containerID;const proof={schema:1,token,workspace,mode:composeMode?'compose-build-windows':wslMode?'wsl-registered-project':'windows',startedAt:new Date().toISOString(),success:false,cleaned:false,steps:[],failures:[]};
const hash=p=>existsSync(p)?createHash('sha256').update(readFileSync(p)).digest('hex'):null;
const env={...process.env,DOCKER_CONTEXT:'',DOCKER_HOST:owned,DOCKER_TLS_VERIFY:'',DOCKER_CERT_PATH:'',DOCKER_API_VERSION:'',BUILDKIT_HOST:'',BUILDX_BUILDER:'default',COMPOSE_FILE:'',COMPOSE_PROFILES:'',COMPOSE_PROJECT_NAME:'',COMPOSE_ENV_FILES:'',COMPOSE_REMOVE_ORPHANS:'false'};
if(process.env.ORBWIN_QUALIFY_COMPOSE_BAKE==='false')env.COMPOSE_BAKE='false';
let composePath,devDocker=wrapper;
if(candidateCompose){
 if(process.env.ORBWIN_QUALIFY_DEVCONTAINERS_LOCAL_TEMP==='1'||process.env.ORBWIN_QUALIFY_COMPOSE_BAKE)throw Error('Candidate qualification requires ordinary temporary paths and no Bake workaround');
 const manifest=JSON.parse(readFileSync(resolve(root,'artifacts/compose/compose-manifest.json')));
 if(hash(candidateCompose)!==manifest.binarySha256)throw Error('Compose candidate checksum differs from reviewed manifest');
 composePath=join(evidence,'docker-compose.exe');copyFileSync(candidateCompose,composePath);copyFileSync(resolve(root,'artifacts/compose/compose-manifest.json'),join(evidence,'compose-manifest.json'));
 const configDir=join(evidence,'credential-free-docker'),plugins=join(configDir,'cli-plugins');mkdirSync(configDir);mkdirSync(plugins);
 const toolLock=JSON.parse(readFileSync(resolve(root,'runtime-tools.lock.json'))),buildxPin=toolLock.tools.find(t=>t.path==='docker-cli-plugins/docker-buildx.exe');
 const dockerPin=toolLock.tools.find(t=>t.path==='bin/docker.exe');if(hash(docker)!==dockerPin.sha256)throw Error('Pinned Docker checksum differs');proof.directDockerSha256=dockerPin.sha256;
 const buildx=resolve(root,'artifacts/runtime-tools/docker-cli-plugins/docker-buildx.exe');if(hash(buildx)!==buildxPin.sha256)throw Error('Pinned Buildx checksum differs');copyFileSync(buildx,join(plugins,'docker-buildx.exe'));
 writeFileSync(join(configDir,'config.json'),JSON.stringify({cliPluginsExtraDirs:[plugins]}));
 env.DOCKER_CONFIG=configDir;env.BUILDX_CONFIG=join(configDir,'buildx');
 for(const key of ['COMPOSE_BAKE','BUILDX_BAKE_ENTITLEMENTS_FS','BAKE_ALLOW_REMOTE_FS_ACCESS','BUILDX_BAKE_FILE','BUILDX_BAKE_FILE_RELATIVE_PATHS','BUILDX_BAKE_PATH_SEPARATOR','DOCKER_AUTH_CONFIG'])delete env[key];
 proof.composeCandidateSha256=manifest.binarySha256;proof.credentialFreeComposeConfig=true;proof.buildxSha256=buildxPin.sha256;proof.ordinaryTemporaryPaths=true;
 const adapterDir=join(evidence,'compose-selection-adapter');mkdirSync(adapterDir);
 const adapterSource=`package main\nimport("os";"os/exec";"fmt")\nfunc rejectProbe(a []string)bool{return len(a)==3&&a[0]=="compose"&&a[1]=="version"&&a[2]=="--short"}\nfunc forward(a []string)*exec.Cmd{return exec.Command(${JSON.stringify(wrapper)},a...)}\nfunc main(){a:=os.Args[1:];if rejectProbe(a){fmt.Fprintln(os.Stderr,"Nonce qualification adapter: use the explicitly supplied standalone Compose candidate");os.Exit(125)};c:=forward(a);c.Stdin=os.Stdin;c.Stdout=os.Stdout;c.Stderr=os.Stderr;if err:=c.Run();err!=nil{if e,ok:=err.(*exec.ExitError);ok{os.Exit(e.ExitCode())};fmt.Fprintln(os.Stderr,err);os.Exit(1)}}\n`;
 const adapterTest=`package main\nimport("testing";"slices")\nfunc TestOnlyExactDiscoveryProbeIsRejected(t *testing.T){if !rejectProbe([]string{"compose","version","--short"}){t.Fatal("exact probe accepted")};for _,a:=range [][]string{nil,{"compose","version"},{"compose","version","--short","extra"},{"compose","version","--short=true"},{"compose","VERSION","--short"},{"version","--short"},{"unknown",";","$(not-a-shell)"},{"exec","container","sh","-c","printf '%s' literal"}}{if rejectProbe(a){t.Fatal(a)};if !slices.Equal(forward(a).Args[1:],a){t.Fatalf("arguments changed: %#v",a)}}}\n`;
 writeFileSync(join(adapterDir,'main.go'),adapterSource);writeFileSync(join(adapterDir,'main_test.go'),adapterTest);
 const goLock=JSON.parse(readFileSync(resolve(root,'upstream.lock.json'))),go=resolve(root,`.tools/go${goLock.go.version}/go/bin/go.exe`);
 const goEnv={...env,CGO_ENABLED:'0',GOOS:'windows',GOARCH:'amd64',GOTOOLCHAIN:'local',GOMAXPROCS:'2'};
 for(const [tag,args]of [['test',['test',join(adapterDir,'main.go'),join(adapterDir,'main_test.go'),'-v']],['build',['build','-buildvcs=false','-trimpath','-o',join(adapterDir,'docker-selection.exe'),join(adapterDir,'main.go')]]]){const p=spawnSync(go,args,{cwd:adapterDir,env:goEnv,encoding:'utf8',windowsHide:true,timeout:60000,maxBuffer:2<<20});writeFileSync(join(adapterDir,tag+'.stdout'),p.stdout||'');writeFileSync(join(adapterDir,tag+'.stderr'),p.stderr||'');if(p.error||p.status!==0)throw Error('Compose selection adapter '+tag+' failed');}
 devDocker=join(adapterDir,'docker-selection.exe');proof.composeSelectionAdapter={purpose:'Force Dev Containers0.469.0 standalone Compose fallback without installed/plugin/config edits',rejectedProbe:['compose','version','--short'],otherArguments:'Literal forwarded to frozen OrbWin alias',sourceSha256:hash(join(adapterDir,'main.go')),testsSha256:hash(join(adapterDir,'main_test.go')),binarySha256:hash(devDocker),targetAliasSha256:hash(wrapper)};
}
function command(binary,args,timeout=30000){const p=spawnSync(binary,args,{encoding:'utf8',windowsHide:true,timeout,maxBuffer:4<<20,env});if(p.error||p.status!==0)throw Error(`${binary} ${args.join(' ')} failed: ${p.error||p.status}\n${p.stdout||''}\n${p.stderr||''}`);return p.stdout;}
const own=(...args)=>command(wrapper,args);
const foreign=(ep,...args)=>command(docker,['--host',ep,...args]);
const rancher=(...args)=>command('C:/Windows/System32/wsl.exe',['--distribution','Ubuntu','--user','root','--exec','/mnt/f/Codes/Orbwin/artifacts/runtime-tools/linux/docker','--host','unix:///mnt/wsl/rancher-desktop/run/docker.sock',...args]);
function identities(){const actual={owned:JSON.parse(own('info','--format','{{json .}}')).ID,desktop:JSON.parse(foreign(desktop,'info','--format','{{json .}}')).ID,rancher:JSON.parse(rancher('info','--format','{{json .}}')).ID};if(JSON.stringify(actual)!==JSON.stringify(expected))throw Error('Three-engine identity mismatch');return actual;}
const inventory=()=>({containers:own('ps','-a','--no-trunc','--format','{{.ID}} {{.State}}').trim().split('\n').filter(Boolean).sort(),images:[...new Set(own('image','ls','-aq','--no-trunc').trim().split(/\s+/).filter(Boolean))].sort(),volumes:own('volume','ls','-q').trim().split(/\s+/).filter(Boolean).sort(),networks:own('network','ls','-q','--no-trunc').trim().split(/\s+/).filter(Boolean).sort()});
const foreignInventory=client=>({containers:client('ps','-a','--no-trunc','--format','{{.ID}} {{.State}}').trim().split('\n').filter(Boolean).sort(),images:[...new Set(client('image','ls','-aq','--no-trunc').trim().split(/\s+/).filter(Boolean))].sort(),volumes:client('volume','ls','-q').trim().split(/\s+/).filter(Boolean).sort(),networks:client('network','ls','-q','--no-trunc').trim().split(/\s+/).filter(Boolean).sort()});
const identity=JSON.parse(readFileSync(join(process.env.LOCALAPPDATA,'OrbWinData/runtime.json')));
const configPaths=[join(process.env.USERPROFILE,'.docker/config.json'),join(process.env.USERPROFILE,'.kube/config'),join(identity.home,'.rdorbwin/docker/config.json')];
if(wslMode)configPaths.push(join(identity.home,'.rdorbwin/wsl-projects/710eb4f4d7f72b0e329a51d30a83377bc7ef6c0cca1a444bd8d2e70e72326e8f.json'));
const save=()=>writeFileSync(join(evidence,'proof.json'),JSON.stringify(proof,null,2)+'\n');
function dev(action,extra=[]){const args=[cli,action,'--docker-path',devDocker,...(composePath?['--docker-compose-path',composePath]:[]),'--workspace-folder',workspace,'--mount-workspace-git-root','false','--user-data-folder',join(workspace,'client-data'),'--default-user-env-probe','none','--log-level','debug','--log-format','json',...extra];const p=spawnSync(process.execPath,args,{encoding:'utf8',windowsHide:true,timeout:180000,maxBuffer:8<<20,env,cwd:workspace});writeFileSync(join(evidence,`${action}-${proof.steps.length}.stdout`),p.stdout||'');writeFileSync(join(evidence,`${action}-${proof.steps.length}.stderr`),p.stderr||'');if(p.error||p.status!==0)throw Error(`Dev Containers ${action} failed: ${p.error||p.status}`);if(composePath&&action==='up'){const commands=(p.stderr||'').split('\n').flatMap(line=>{try{const item=JSON.parse(line);return item.type==='start'&&item.text?.startsWith('Run: '+composePath)?[item.text]:[]}catch{return[]}});if(!commands.length)throw Error('Dev Containers did not invoke the explicit Compose candidate');proof.candidateComposeCommands=(proof.candidateComposeCommands||[]).concat(commands);}return p.stdout;}
try{
 proof.identitiesBefore=identities();proof.configBefore=configPaths.map(hash);proof.inventoryBefore=inventory();
 proof.foreignBefore={desktop:foreignInventory((...args)=>foreign(desktop,...args)),rancher:foreignInventory(rancher)};
 proof.harnessSha256=hash(fileURLToPath(import.meta.url));writeFileSync(join(evidence,'harness.mjs'),readFileSync(fileURLToPath(import.meta.url)));
 proof.clientManifestSha256=hash(resolve(root,'artifacts/cli/cli-manifest.json'));writeFileSync(join(evidence,'cli-manifest.json'),readFileSync(resolve(root,'artifacts/cli/cli-manifest.json')));
 proof.composeBake=env.COMPOSE_BAKE||'upstream default';
 if(composeMode)proof.cacheBefore=own('system','df','--format','{{json .}}').trim().split('\n').filter(Boolean).map(line=>JSON.parse(line));
 proof.wrapperSha256=hash(wrapper);proof.devContainersSha256=hash(cli);
 const base=JSON.parse(own('image','inspect',image))[0];if(base.Id!==image||base.Os!=='linux'||base.Architecture!=='amd64'||base.Config?.Volumes)throw Error('Cached base image profile differs');
 own('image','tag',image,alias);proof.alias=alias;save();
 writeFileSync(join(workspace,'directory-seed.txt'),'windows directory seed '+token+'\n');writeFileSync(join(workspace,'shared-file.txt'),'windows file seed '+token+'\n');
 const source=wslMode?`${sharedRoot}/${name}`:workspace;
 const fileSource=wslMode?`${source}/shared-file.txt`:join(workspace,'shared-file.txt');
 const config={name,image:alias,workspaceMount:`type=bind,source=${source},target=/workspace`,workspaceFolder:'/workspace',containerUser:'root',remoteUser:'root',userEnvProbe:'none',updateRemoteUserUID:false,runArgs:['--pull=never','--network=none',`--label=${label}=${token}`],mounts:[`type=bind,source=${fileSource},target=/shared-file.txt`],customizations:{vscode:{extensions:[]}}};
 if(composeMode){
  delete config.image;delete config.workspaceMount;delete config.runArgs;delete config.mounts;
  config.dockerComposeFile='compose.yaml';config.service='app';config.shutdownAction='none';config.overrideCommand=false;
  mkdirSync(join(workspace,'.devcontainer/build'));
  const dockerfile=`FROM ${alias}\nLABEL ${label}="${token}"\nRUN printf '%s\\n' 'built-${token}' > /fixture-build.txt\n`;
  writeFileSync(join(workspace,'.devcontainer/build/Dockerfile'),dockerfile);
  const compose={name,services:{app:{image:name+':built',build:{context:'./build',dockerfile:'Dockerfile',network:'none',pull:false,labels:{[label]:token}},pull_policy:'never',network_mode:'none',labels:{[label]:token},user:'root',command:['/bin/sh','-c','trap "exit 0" TERM; while sleep 1 & wait $!; do :; done'],volumes:[{type:'bind',source,target:'/workspace'},{type:'bind',source:fileSource,target:'/shared-file.txt'}]}}};
  writeFileSync(join(workspace,'.devcontainer/compose.yaml'),JSON.stringify(compose,null,2));proof.composeSha256=hash(join(workspace,'.devcontainer/compose.yaml'));proof.dockerfileSha256=hash(join(workspace,'.devcontainer/build/Dockerfile'));proof.buildAlias=name+':built';
  if(candidateCompose){
   const contextDir=join(workspace,'.devcontainer/build'),outside=join(workspace,'permission-probe');mkdirSync(outside);
   const selectedDir=join(outside,'[ab],selected'),siblingDir=join(outside,'a,selected');mkdirSync(selectedDir);mkdirSync(siblingDir);
   const selected=join(selectedDir,'Dockerfile'),sibling=join(siblingDir,'Dockerfile');
   writeFileSync(selected,'FROM scratch\nLABEL selected=literal\n');writeFileSync(sibling,'FROM scratch\nLABEL sibling=denied\n');
   const probe=(file,tag)=>{const bakeFile=join(contextDir,`probe-${tag}.json`);writeFileSync(bakeFile,JSON.stringify({target:{probe:{context:contextDir,dockerfile:file}}}));const args=['--config',env.DOCKER_CONFIG,'--host',owned,'buildx','bake','--file',bakeFile,'--allow',`fs.read=${selected}`,'--call=outline','probe'];const result=spawnSync(docker,args,{cwd:contextDir,env,encoding:'utf8',windowsHide:true,timeout:60000,maxBuffer:4<<20});writeFileSync(join(evidence,`bake-${tag}.stdout`),result.stdout||'');writeFileSync(join(evidence,`bake-${tag}.stderr`),result.stderr||'');return result;};
   const positive=probe(selected,'exact-file'),negative=probe(sibling,'sibling-file');
   if(positive.error||positive.status!==0)throw Error('Buildx literal bracket/comma exact-file grant did not succeed');
   if(negative.error||negative.status===0||!/additional privileges requested/.test(negative.stderr||''))throw Error('Buildx exact-file grant did not refuse the sibling Dockerfile');
   proof.exactFilePermission={literalBracketsAndComma:true,metacharacterPosition:'directory components',positiveExit:positive.status,siblingRefused:true,negativeExit:negative.status,scope:'Exact selected file outside current directory/context; no directory grant or filesystem bypass'};
  }
  if(process.env.ORBWIN_QUALIFY_DEVCONTAINERS_LOCAL_TEMP==='1'){const localTemp=join(workspace,'.devcontainer/build/temp');mkdirSync(localTemp);env.TEMP=localTemp;env.TMP=localTemp;proof.temporaryPathWorkaround={scope:'Dev Containers child process only',path:localTemp,reason:'Keep generated feature Dockerfile within the explicit fixture build context; no entitlement grant'};}
 }
 writeFileSync(join(workspace,'.devcontainer/devcontainer.json'),JSON.stringify(config,null,2));proof.configSha256=hash(join(workspace,'.devcontainer/devcontainer.json'));save();
 const result=dev('up',['--update-remote-user-uid-default','never','--skip-post-create']);proof.steps.push('actual-up');const response=JSON.parse(result.trim().split('\n').at(-1));containerID=response.containerId;if(!/^[a-f0-9]{64}$/.test(containerID||''))throw Error('No full container identity');proof.containerID=containerID;save();
 const inspect=JSON.parse(own('inspect',containerID))[0];if(inspect.Config.Labels?.[label]!==token||!inspect.Config.Image.startsWith(name)||inspect.HostConfig.NetworkMode!=='none')throw Error('Fixture container ownership differs');
 if(inspect.Mounts.some(m=>m.Type!=='bind'||!m.Source.includes(name)))throw Error('Unexpected or broad fixture mount');proof.mounts=inspect.Mounts.map(m=>({type:m.Type,source:m.Source,destination:m.Destination,rw:m.RW}));save();
 dev('exec',['--container-id',containerID,'/bin/sh','-c',`test "$(cat /workspace/directory-seed.txt)" = "windows directory seed ${token}" && test "$(cat /shared-file.txt)" = "windows file seed ${token}" && printf '%s\\n' 'container directory write ${token}' > /workspace/from-container.txt && printf '%s\\n' 'container file write ${token}' > /shared-file.txt`]);
 if(composeMode){dev('exec',['--container-id',containerID,'/bin/sh','-c',`test "$(cat /fixture-build.txt)" = 'built-${token}'`]);proof.steps.push('actual-compose-build-marker');}
 if(readFileSync(join(workspace,'from-container.txt'),'utf8').trim()!==`container directory write ${token}`||readFileSync(join(workspace,'shared-file.txt'),'utf8').trim()!==`container file write ${token}`)throw Error('Windows bind write did not reach host');proof.steps.push('directory-and-file-roundtrip');
 const reopened=JSON.parse(dev('up',['--expect-existing-container','--update-remote-user-uid-default','never','--skip-post-create']).trim().split('\n').at(-1));if(reopened.containerId!==containerID)throw Error('Reopen adopted another container');proof.steps.push('actual-reopen-same-container');proof.success=true;
}catch(e){proof.failures.push(String(e.stack||e));}
finally{
 try{
  identities();
  const labelled=own('ps','-aq','--no-trunc','--filter',`label=${label}=${token}`).trim().split(/\s+/).filter(Boolean);
  for(const id of labelled){const item=JSON.parse(own('inspect',id))[0];if(item.Id!==id||item.Config.Labels?.[label]!==token||!item.Config.Image.startsWith(name))throw Error('Cleanup adoption refused');own('rm','-f',id);}
  if(proof.buildAlias){const ids=own('image','ls','--filter',`reference=${proof.buildAlias}`,'-q','--no-trunc').trim().split(/\s+/).filter(Boolean);for(const id of new Set(ids)){const img=JSON.parse(own('image','inspect',proof.buildAlias))[0];if(img.Id!==id||img.Config.Labels?.[label]!==token||proof.inventoryBefore.images.includes(id))throw Error('Built image cleanup adoption refused');own('image','rm',proof.buildAlias);proof.builtImageRemoved=id;}}
  if(proof.alias){const current=JSON.parse(own('image','inspect',alias))[0];if(current.Id!==image)throw Error('Fixture image alias changed; cleanup refused');own('image','rm',alias);}
  proof.inventoryAfter=inventory();proof.inventoryRestored=JSON.stringify(proof.inventoryBefore)===JSON.stringify(proof.inventoryAfter);if(!proof.inventoryRestored)throw Error('Owned inventory differs after exact cleanup');
  proof.foreignAfter={desktop:foreignInventory((...args)=>foreign(desktop,...args)),rancher:foreignInventory(rancher)};proof.foreignInventoriesPreserved=JSON.stringify(proof.foreignBefore)===JSON.stringify(proof.foreignAfter);if(!proof.foreignInventoriesPreserved)throw Error('Foreign engine inventories changed during qualification');
  if(composeMode){proof.cacheAfter=own('system','df','--format','{{json .}}').trim().split('\n').filter(Boolean).map(line=>JSON.parse(line));proof.cacheResidue='BuildKit may retain fixture build records/layers. No global prune was performed; before/after system df is retained.';}
  proof.identitiesAfter=identities();proof.configAfter=configPaths.map(hash);proof.configPreserved=JSON.stringify(proof.configBefore)===JSON.stringify(proof.configAfter);if(!proof.configPreserved)throw Error('Client config changed');proof.cleaned=true;
 }catch(e){proof.failures.push('cleanup: '+String(e.stack||e));}
 proof.finishedAt=new Date().toISOString();save();console.log(join(evidence,'proof.json'));if(!proof.success||!proof.cleaned)process.exitCode=1;
}
