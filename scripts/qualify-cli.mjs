// Read-only qualification. Dedicated staging directory, no installation edits,
// engine lifecycle requests, context selection, profile edits or PATH writes.
import { execFileSync } from 'node:child_process';
import { readFileSync, mkdirSync, copyFileSync, symlinkSync, existsSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { createHash, randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../', import.meta.url));
const installed = join(process.env.LOCALAPPDATA, 'OrbWin');
const identity = JSON.parse(readFileSync(join(process.env.LOCALAPPDATA, 'OrbWinData/runtime.json')));
if (!identity.identityVerified || identity.endpoint !== 'npipe:////./pipe/docker-rancher-desktop-orbwin') throw Error('No qualified owned identity');
const out = resolve(root, `artifacts/cli/qualification-${randomUUID()}`);
mkdirSync(out);
copyFileSync(resolve(root, 'artifacts/cli/orbwin-cli.exe'), join(out, 'orbwin-cli.exe'));
copyFileSync(resolve(root, 'artifacts/cli/orbwin-docker.exe'), join(out, 'orbwin-docker.exe'));
symlinkSync(join(installed, 'runtime'), join(out, 'runtime'), 'junction');
const hash = p => existsSync(p) ? createHash('sha256').update(readFileSync(p)).digest('hex') : null;
const preserved = [join(process.env.USERPROFILE,'.docker/config.json'), join(process.env.USERPROFILE,'.kube/config'), join(identity.home,'.rdorbwin/docker/config.json')];
const before = preserved.map(hash);
const cli = args => execFileSync(join(out,'orbwin-cli.exe'),args,{encoding:'utf8',windowsHide:true,timeout:25000});
const status = JSON.parse(cli(['status']));
if (status.ID !== identity.daemonId || status.OSType !== 'linux') throw Error('Wrong engine');
const dockerInfo = JSON.parse(cli(['docker','info','--format','{{json .}}']));
if (dockerInfo.ID !== identity.daemonId) throw Error('Docker wrapper target mismatch');
const aliasInfo=JSON.parse(execFileSync(join(out,'orbwin-docker.exe'),['info','--format','{{json .}}'],{encoding:'utf8',windowsHide:true,timeout:25000}));
if(aliasInfo.ID!==identity.daemonId)throw Error('Docker alias target mismatch');
const alias = args => execFileSync(join(out,'orbwin-docker.exe'),args,{encoding:'utf8',windowsHide:true,timeout:25000});
const buildxVersion=alias(['buildx','version']).trim();
const builder=alias(['buildx','inspect','default']);
const defaultContext=JSON.parse(execFileSync(join(installed,'runtime/tools/bin/docker.exe'),['--config',join(identity.home,'.rdorbwin/docker'),'--host',identity.endpoint,'context','inspect','default'],{encoding:'utf8',windowsHide:true,timeout:15000,env:{...process.env,DOCKER_CONTEXT:'',DOCKER_HOST:identity.endpoint}}));
if(!/^Driver:\s+docker\s*$/m.test(builder)||!/^Endpoint:\s+default\s*$/m.test(builder)||defaultContext[0]?.Endpoints?.docker?.Host!==identity.endpoint)throw Error('Default Buildx builder context is not the explicitly targeted Docker driver');
const logs = JSON.parse(cli(['logs']));
const config = JSON.parse(cli(['config','get']));
if (typeof config.running !== 'boolean' || typeof config['kubernetes.enabled'] !== 'boolean') throw Error('Settings projection invalid');
const snapshot = join(out,'snapshot.json');cli(['snapshot',snapshot]);
const snap = JSON.parse(readFileSync(snapshot));
if (snap.engine.ID !== identity.daemonId || snap.status !== 'available' || snap.schema !== 1) throw Error('Snapshot mismatch');
let refused = false;try {cli(['snapshot',snapshot]);} catch {refused = true;}if (!refused) throw Error('Snapshot overwrite accepted');
for (const forbidden of [['docker','--host=foreign','ps'],['docker','context','use','default'],['config','set','application.addPath=front']]) {
  let rejected = false;try {cli(forbidden);}catch {rejected=true;}if (!rejected) throw Error('Unsafe request accepted');
}
const after = preserved.map(hash);if (JSON.stringify(before)!==JSON.stringify(after)) throw Error('Persistent client configuration changed');
writeFileSync(join(out,'proof.json'),JSON.stringify({schema:1,createdAt:new Date().toISOString(),daemonId:status.ID,status,config,logsDirectoryCorrect:logs.directory===join(identity.home,'AppData/Local/rancher-desktop-orbwin-logs'),snapshot:true,snapshotNoOverwrite:true,dockerAlias:true,buildxVersion,defaultBuilderDriver:'docker',defaultBuilderEndpointVerified:true,unsafeRequestsRefused:true,configBefore:before,configAfter:after,cliSha256:hash(join(out,'orbwin-cli.exe')),aliasSha256:hash(join(out,'orbwin-docker.exe')),limits:['Read-only development CLI staged against installed runtime','No lifecycle or config mutations qualified','No installed packaging acceptance']},null,2)+'\n');
console.log(join(out,'proof.json'));
