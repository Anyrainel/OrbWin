// Owned finite IPv4-published fixture for actual Windows browser observation.
// Browser navigation is separate; importing/running never drives a browser.
import {spawnSync} from 'node:child_process';
import {createHash,randomUUID} from 'node:crypto';
import {readFileSync,writeFileSync,mkdirSync,existsSync,openSync,writeSync,fsyncSync,closeSync,renameSync} from 'node:fs';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
assert.equal(process.env.ORBWIN_QUALIFY_OWNED,'1');
const root=fileURLToPath(new URL('../',import.meta.url));
const mode=process.argv[2];assert.ok(['prepare','cleanup'].includes(mode));
const pin=JSON.parse(readFileSync(resolve(root,'tests/registry/images.lock.json'))).base;
const wrapper=resolve(root,'artifacts/package/orbwin-docker.exe');
const label='dev.orbwin.qualification';
const hash=p=>existsSync(p)?createHash('sha256').update(readFileSync(p)).digest('hex'):null;
let state,file;
function save(){const temp=`${file}.${randomUUID()}.partial`;const fd=openSync(temp,'wx');try{writeSync(fd,JSON.stringify(state,null,2));fsyncSync(fd);}finally{closeSync(fd);}renameSync(temp,file);}
function run(binary,args){const r=spawnSync(binary,args,{encoding:'utf8',windowsHide:true,timeout:30000,maxBuffer:4<<20,env:{...process.env,DOCKER_CONTEXT:'',DOCKER_TLS_VERIFY:'',DOCKER_CERT_PATH:'',DOCKER_API_VERSION:'',BUILDKIT_HOST:''}});state.events.push({args,status:r.status,error:r.error?.message,stdoutSha256:createHash('sha256').update(r.stdout??'').digest('hex'),stderrSha256:createHash('sha256').update(r.stderr??'').digest('hex')});save();assert.equal(r.error,undefined);assert.equal(r.status,0,'Fixture command failed; output omitted');return r.stdout.trim();}
const cli=(...args)=>run(wrapper,args);
const linux=p=>`/mnt/${p[0].toLowerCase()}/${p.slice(3).replaceAll('\\','/')}`;
const lines=s=>s.split(/\r?\n/).filter(Boolean).sort();
function snapshot(){
 const docker=resolve(root,'artifacts/runtime-tools/bin/docker.exe');
 const raw=(host,...args)=>run(docker,['--config',state.emptyConfig,'--host',host,...args]);
 const rancher=(...args)=>run(resolve(process.env.SystemRoot,'System32/wsl.exe'),['--distribution-id','{231810b0-fa21-48a8-b0a7-86f9a1360029}','--user','root','--exec',linux(resolve(root,'artifacts/runtime-tools/linux/docker')),'--config',linux(state.emptyConfig),'--host','unix:///mnt/wsl/rancher-desktop/run/docker.sock',...args]);
 const inventories={};const ids={};
 for(const [name,read,expected] of [['owned',(...a)=>raw('npipe:////./pipe/docker-rancher-desktop-orbwin',...a),'d9c00194-ea9d-439a-8561-6a0ec580fbd5'],['desktop',(...a)=>raw('npipe:////./pipe/dockerDesktopLinuxEngine',...a),'a307ff0e-581d-49d3-8aee-a0636dda9137'],['rancher',rancher,'741eb2cb-adf5-4676-bf28-f03f68d54f60']]){
  ids[name]=read('info','--format','{{.ID}}');assert.equal(ids[name],expected);
  const containers=lines(read('ps','-a','--no-trunc','--format','{{.ID}}'));
  assert.ok(containers.length<=256&&containers.every(x=>/^[a-f0-9]{64}$/.test(x)));
  inventories[name]={containers,states:containers.length?lines(read('container','inspect',...containers,'--format','{{.Id}}|{{.State.Running}}|{{.State.Paused}}|{{.RestartCount}}')):[],images:[...new Set(lines(read('image','ls','-aq','--no-trunc')))],volumes:lines(read('volume','ls','-q')),networks:lines(read('network','ls','--no-trunc','-q'))};
 }
 const home=resolve(process.env.LOCALAPPDATA,'OrbWinData/home');
 const configs=Object.fromEntries([['docker',resolve(process.env.USERPROFILE,'.docker/config.json')],['kube',resolve(process.env.USERPROFILE,'.kube/config')],['wsl',resolve(process.env.USERPROFILE,'.wslconfig')],['windowsPrivate',resolve(home,'.rdorbwin/docker/config.json')],['wslPrivate',resolve(home,'.rdorbwin/wsl-docker/config.json')],['project',resolve(home,'.rdorbwin/wsl-projects/710eb4f4d7f72b0e329a51d30a83377bc7ef6c0cca1a444bd8d2e70e72326e8f.json')]].map(([name,path])=>[name,hash(path)]));
 return {ids,inventories,configs};
}
function verify(){
 assert.equal(cli('info','--format','{{.ID}}'),state.before.ids.owned);
 const value=JSON.parse(cli('container','inspect',state.id))[0];
 assert.equal(value.Id,state.id);assert.match(value.Id,/^[a-f0-9]{64}$/);
 assert.ok(!state.before.inventories.owned.containers.includes(value.Id));
 assert.equal(value.Name,`/${state.name}`);assert.equal(value.Config.Labels?.[label],state.nonce);assert.equal(value.Image,state.imageId);
 assert.equal(value.Config.Entrypoint?.[0],'/proof/server');assert.deepEqual(value.Config.Cmd,[state.nonce]);
 assert.equal(value.HostConfig.ReadonlyRootfs,true);assert.equal(value.HostConfig.Privileged,false);assert.equal(value.HostConfig.NanoCpus,250000000);assert.equal(value.HostConfig.Memory,67108864);assert.equal(value.HostConfig.PidsLimit,32);
 assert.ok(value.HostConfig.CapDrop?.includes('ALL'));assert.ok(value.HostConfig.SecurityOpt?.includes('no-new-privileges:true'));assert.equal(value.HostConfig.RestartPolicy.Name,'no');
 assert.equal(value.Mounts.length,1);const m=value.Mounts[0];assert.equal(m.Type,'bind');assert.equal(m.Destination,'/proof/server');assert.equal(m.RW,false);assert.ok(m.Source.replaceAll('\\','/').toLowerCase().endsWith('/p2-browser-server-20261025/server'));
 assert.equal(hash(state.server),state.serverSha256);
 return value;
}
function cleanup(){
 if(!state.id&&state.intent){const found=lines(cli('ps','-aq','--no-trunc','--filter',`name=^/${state.name}$`));assert.ok(found.length<=1);if(found.length){state.id=found[0];save();}}
 if(state.id){const value=verify();assert.equal(value.State.Paused,false);if(value.State.Running)cli('stop','--time','5',state.id);verify();cli('rm',state.id);}
 state.after=snapshot();assert.deepEqual(state.after,state.before);state.cleaned=true;save();
}
if(mode==='prepare'){
 const nonce=randomUUID().replaceAll('-','');const name=`orbwin-browser-${nonce}`;const folder=resolve(root,'artifacts/qualification',name);mkdirSync(folder);file=resolve(folder,'proof.json');const emptyConfig=resolve(folder,'empty-docker');mkdirSync(emptyConfig);writeFileSync(resolve(emptyConfig,'config.json'),'{}',{flag:'wx'});
 state={schema:1,nonce,name,folder,emptyConfig,events:[],started:new Date().toISOString(),cleaned:false,server:resolve(root,'artifacts/qualification/p2-browser-server-20261025/server')};save();
 try{
  const manifest=JSON.parse(readFileSync(resolve(root,'artifacts/cli/cli-manifest.json')));assert.equal(hash(wrapper),manifest.alias.sha256);state.wrapperSha256=hash(wrapper);state.serverSha256=hash(state.server);assert.equal(state.serverSha256,'967891d67d954f94d170cb55101f70edf106fc8f38e687e42e0760b3b05ecee7');state.sourceSha256=hash(resolve(root,'tests/network/browser-loopback/main.go'));state.before=snapshot();save();
  const image=JSON.parse(cli('image','inspect',pin))[0];assert.equal(image.Os,'linux');assert.equal(image.Architecture,'amd64');state.imageId=image.Id;
  assert.equal(cli('ps','-aq','--filter',`name=^/${name}$`),'');state.intent={kind:'container',name,image:pin,time:new Date().toISOString()};save();
  state.id=cli('create','--name',name,'--pull=never','--label',`${label}=${nonce}`,'--read-only','--network=bridge','--cpus=0.25','--memory=64m','--pids-limit=32','--cap-drop=ALL','--security-opt=no-new-privileges:true','--restart=no','--publish','127.0.0.1::8080','--mount',`type=bind,source=${state.server},target=/proof/server,readonly`,'--env','GOMAXPROCS=2','--env','GOMEMLIMIT=32MiB','--entrypoint','/proof/server',pin,nonce);save();verify();cli('start',state.id);const value=verify();assert.equal(value.State.Running,true);const port=value.NetworkSettings.Ports['8080/tcp'];assert.equal(port.length,1);assert.equal(port[0].HostIp,'127.0.0.1');state.port=Number(port[0].HostPort);assert.ok(state.port>=1&&state.port<=65535);state.urls=[`http://localhost:${state.port}/${nonce}`,`http://network-check.orbwin.localhost:${state.port}/${nonce}`];save();console.log(JSON.stringify({file,urls:state.urls,nonce,deadlineSeconds:180}));
 }catch(e){state.error=String(e);save();if(state.before)try{cleanup();}catch(c){state.cleanupError=String(c);save();}throw e;}
}else{
 file=resolve(process.argv[3]??'');state=JSON.parse(readFileSync(file));assert.match(state.nonce,/^[a-f0-9]{32}$/);assert.equal(state.name,`orbwin-browser-${state.nonce}`);assert.equal(dirname(file),resolve(root,'artifacts/qualification',state.name));
 try {verify();state.serverRequests=cli('logs','--tail=100',state.id).split(/\r?\n/).filter(Boolean).map(x=>JSON.parse(x));assert.ok(state.serverRequests.length<=100);}catch(e){state.readbackError='Fixture request-log readback failed';save();process.exitCode=1;}finally{cleanup();}console.log(JSON.stringify({file,cleaned:state.cleaned,serverRequests:state.serverRequests}));
}
