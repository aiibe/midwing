import test from 'node:test';
import assert from 'node:assert/strict';
import {build} from 'vite';
import {fileURLToPath} from 'node:url';
import {JSDOM} from 'jsdom';

// Bundle the real entry point so imports, raw HTML, and Wails bindings are exercised.
const bundle = await build({
 root: fileURLToPath(new URL('..', import.meta.url)),
 configFile: false,
 logLevel: 'silent',
 build: {
  write: false,
  minify: false,
  lib: {entry: fileURLToPath(new URL('../src/main.ts', import.meta.url)), name: 'MidwingTest', formats: ['iife']},
 },
});
const output = Array.isArray(bundle) ? bundle[0] : bundle;
const script = output.output.find(chunk => chunk.type === 'chunk' && chunk.isEntry).code;
const sampleApi = {id:'sample-api',name:'Sample API',description:'Repositories',origin:'https://api.example.test',authKind:'token',revision:'',custom:false,examples:['/user'],permissions:[{id:'profile',label:'Profile',description:'Read profile'},{id:'repos',label:'Repositories',description:'Read repositories'}]};
const custom = {...sampleApi,id:'new-service',revision:'custom-v1',name:'New Service',custom:true,origin:'https://example.com'};
const settle = () => new Promise(resolve => setTimeout(resolve, 20));
async function setup(t) {
 const dom = new JSDOM('<div id="app"></div>', {runScripts:'outside-only',pretendToBeVisual:true});
 t.after(() => dom.window.close());
 const {window} = dom;
 // JSDOM does not implement the native dialog methods.
 window.HTMLDialogElement.prototype.showModal = function() { this.open = true; };
 window.HTMLDialogElement.prototype.close = function() { this.open = false; };
 let listener;
 let state = {services:[sampleApi],connections:[]};
 let fail = false;
 let calls = 0;
 let active = 0;
 let maxActive = 0;
 const app = {async Version() { return '0.1.0'; }, async State() {
  calls++; active++; maxActive = Math.max(maxActive, active);
  try { await settle(); if(fail) throw Error('bad settings'); return structuredClone(state); } finally { active--; }
 }};
 window.go = {main:{App:app}};
 window.runtime = {};
 window.runtime.EventsOnMultiple = (name, callback) => { assert.equal(name,'settings-changed'); listener = callback; return () => {listener = undefined;}; };
 window.eval(script);
 await settle(); await settle();
 return {window,app,emit:() => listener?.(),setState:value=>{state=value;},setFail:value=>{fail=value;},calls:()=>calls,maxActive:()=>maxActive};
}

test('external creation appears and preserves credentials, selection, and permission edits', async t => {
 const h = await setup(t); const d = h.window.document;
 d.querySelector('#token').value='draft-secret';
 const permission = d.querySelector('[value="repos"]'); permission.checked=false; permission.dispatchEvent(new h.window.Event('change',{bubbles:true}));
 h.setState({services:[sampleApi,custom],connections:[]}); h.emit();
 await settle(); await settle();
 assert.match(d.querySelector('#services').textContent,/New Service/);
 assert.equal(d.querySelector('#service-name').textContent,'Sample API');
 assert.equal(d.querySelector('#token').value,'draft-secret');
 assert.equal(d.querySelector('[value="repos"]').checked,false);
});

test('unchanged snapshots do not rerender; failed reload keeps last valid view',async t=>{
 const h=await setup(t); const d=h.window.document;
 const original = d.querySelector('[value="profile"]');
 h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('[value="profile"]'),original);
 h.setFail(true); h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('[value="profile"]'),original);
 assert.match(d.querySelector('#message').textContent,/last valid view/);
 h.setFail(false);h.setState({services:[sampleApi,custom],connections:[]});
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();await settle();
 assert.match(d.querySelector('#services').textContent,/New Service/);
});

test('connection changes update untouched permissions; definition changes clear credentials',async t=>{
 const h=await setup(t); const d=h.window.document;
 h.setState({services:[sampleApi],connections:[{service:'sample-api',permissions:['profile']}]});h.emit();await settle();await settle();
 assert.equal(d.querySelector('[value="repos"]').checked,false);
 assert.equal(d.querySelector('#badge').textContent,'Connected');
 d.querySelector('#token').value='old-destination-secret';
 h.setState({services:[{...sampleApi,revision:'changed',origin:'https://different.example.com'}],connections:[]});h.emit();await settle();await settle();
 assert.equal(d.querySelector('#token').value,'');
});

test('service revisions preserve edits across display changes and clear them when the contract changes', async t => {
 const h = await setup(t); const d = h.window.document;
 d.querySelector('#token').value = 'draft-secret';
 const permission = d.querySelector('[value="repos"]');
 permission.checked = false;
 permission.dispatchEvent(new h.window.Event('change', {bubbles:true}));
 h.setState({services:[{...sampleApi,description:'Updated display text'}],connections:[]});
 h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('#service-description').textContent, 'Updated display text');
 assert.equal(d.querySelector('#token').value, 'draft-secret');
 assert.equal(d.querySelector('[value="repos"]').checked, false);
 h.setState({services:[{...sampleApi,revision:'new-contract'}],connections:[]});
 h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('#token').value, '');
 assert.equal(d.querySelector('[value="repos"]').checked, true);
 d.querySelector('#token').value = 'another-draft';
 h.setState({services:[{...sampleApi,id:'replacement',revision:'new-contract'}],connections:[]});
 h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('#token').value, '');
});

test('custom form survives refresh and concurrent refreshes are serialized',async t=>{
 const h=await setup(t);const d=h.window.document;
 d.querySelector('#add-service').click();
 d.querySelector('#custom-name').value='Draft';
 h.setState({services:[sampleApi,custom],connections:[]});
 h.emit();await new Promise(resolve=>setTimeout(resolve,5));h.emit();h.window.dispatchEvent(new h.window.Event('focus'));
 await settle();await settle();await settle();
 assert.equal(d.querySelector('#custom-name').value,'Draft');
 assert.equal(d.querySelector('#custom').hidden,false);
 assert.equal(h.maxActive(),1);
 assert.match(d.querySelector('#services').textContent,/New Service/);
});

test('refresh waits for connection save to finish',async t=>{
 const h=await setup(t);const d=h.window.document;
 let finish;
 h.app.SaveConnection=()=>new Promise(resolve=>{finish=resolve;});
 d.querySelector('#token').value='token';
 d.querySelector('#form').dispatchEvent(new h.window.Event('submit',{bubbles:true,cancelable:true}));
 const count=h.calls();
 h.setState({services:[sampleApi,custom],connections:[{service:'sample-api',permissions:['profile']}]});h.emit();
 await settle();assert.equal(h.calls(),count);
 finish();await settle();await settle();await settle();
 assert.match(d.querySelector('#services').textContent,/New Service/);
 assert.equal(d.querySelector('#badge').textContent,'Connected');
 assert.equal(h.maxActive(),1);
});

test('custom service creation uses generated bindings for token and password definitions', async t => {
 for (const authKind of ['token', 'password']) {
  await t.test(authKind, async t => {
   const h = await setup(t); const d = h.window.document;
   d.querySelector('#add-service').click();
   const values = {name: 'New Service', id: 'new-service', url: 'https://example.com', header: 'Authorization', prefix: 'Bearer', 'login-path': '/login', 'token-field': 'session.token', 'refresh-path': '/refresh'};
   for (const [key, value] of Object.entries(values)) d.querySelector(`#custom-${key}`).value = value;
   const auth = d.querySelector('#custom-auth'); auth.value = authKind;
   auth.dispatchEvent(new h.window.Event('change'));
   d.querySelector('.endpoint-path').value = '/items/{id}';
   d.querySelector('.endpoint-query').value = 'limit, cursor';
   d.querySelector('.endpoint-description').value = 'Read item details';
   let created;
   h.app.CreateCustomService = async definition => {
    created = JSON.parse(JSON.stringify(definition));
    h.setState({services: [sampleApi, {...custom, authKind}], connections: []});
   };
   d.querySelector('#custom-form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
   await settle(); await settle(); await settle();
   assert.equal(created.id, 'new-service');
   assert.equal(created.authKind, authKind);
   assert.equal(created.prefix, 'Bearer ');
   assert.deepEqual(created.endpoints, [{path: '/items/{id}', description: 'Read item details', queryParameters: ['limit', 'cursor']}]);
   if (authKind === 'password') {
    assert.deepEqual(created.password, {loginPath: '/login', emailField: 'email', passwordField: 'password', tokenField: 'session.token', refreshPath: '/refresh'});
   } else assert.equal(created.password, undefined);
   assert.equal(d.querySelector('#service-name').textContent, 'New Service');
   assert.equal(d.querySelector('#connections').hidden, false);
   assert.equal(d.querySelector('#password-fields').hidden, authKind !== 'password');
  });
 }
});

test('connection save reload proceeds after an earlier background read fails', async t => {
 const h = await setup(t); const d = h.window.document;
 let rejectRead;
 let calls = 0;
 h.app.State = async () => {
  calls++;
  if (calls === 1) await new Promise((_, reject) => { rejectRead = reject; });
  return {services: [sampleApi], connections: [{service: 'sample-api', permissions: ['profile']}]};
 };
 h.emit(); await settle();
 h.app.SaveConnection = async () => {};
 d.querySelector('#token').value = 'token';
 d.querySelector('#form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
 await settle();
 assert.equal(calls, 1);
 rejectRead(Error('background read failed'));
 await settle(); await settle();
 assert.equal(calls, 2);
 assert.equal(d.querySelector('#badge').textContent, 'Connected');
 assert.match(d.querySelector('#message').textContent, /Sample API connected/);
 assert.equal(d.querySelector('#token').disabled, true);
 assert.equal(d.querySelector('#email').disabled, true);
 assert.equal(d.querySelector('#password').disabled, true);
});

test('successful connection changes remain reported when their reload fails', async t => {
 for (const operation of ['save', 'disconnect', 'delete']) {
  await t.test(operation, async t => {
   const h = await setup(t); const d = h.window.document;
   h.setState({services: [{...sampleApi, custom: true}], connections: [{service: 'sample-api', permissions: ['profile']}]});
   h.emit(); await settle(); await settle();
   h.setFail(true);
   let changes = 0;
   const change = async () => { changes++; };
   if (operation === 'save') {
    d.querySelector('#reconnect').click();
    h.app.SaveConnection = change;
    d.querySelector('#token').value = 'token';
    d.querySelector('#form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
   } else if (operation === 'disconnect') {
    h.app.RemoveConnection = change;
    d.querySelector('#remove').click();
   } else {
    h.app.DeleteCustomService = change;
    d.querySelector('#delete-service').click();
    d.querySelector('#confirm-delete').click();
   }
   await settle(); await settle();
   assert.equal(changes, 1);
   assert.match(d.querySelector('#message').textContent, new RegExp(`Sample API ${operation === 'save' ? 'connected' : operation === 'disconnect' ? 'disconnected' : 'deleted'}`));
   assert.match(d.querySelector('#message').textContent, /Could not refresh the view/);
   assert.equal(d.querySelector('#token').disabled, operation !== 'save');
  });
 }
});

test('created service reports success and leaves the creation form when reload fails', async t => {
 const h = await setup(t); const d = h.window.document;
 d.querySelector('#add-service').click();
 for (const [key, value] of Object.entries({name: 'New Service', id: 'new-service', url: 'https://example.com'})) d.querySelector(`#custom-${key}`).value = value;
 d.querySelector('.endpoint-path').value = '/items';
 let changes = 0;
 h.app.CreateCustomService = async () => { changes++; h.setFail(true); };
 d.querySelector('#custom-form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
 await settle(); await settle();
 assert.equal(changes, 1);
 assert.equal(d.querySelector('#custom').hidden, true);
 assert.equal(d.querySelector('#connections').hidden, false);
 assert.equal(d.querySelector('#custom-error').textContent, '');
 assert.match(d.querySelector('#message').textContent, /New Service created.*Could not refresh the view/);
 h.setFail(false);
 h.setState({services: [sampleApi, custom], connections: []});
 h.window.dispatchEvent(new h.window.Event('focus'));
 await settle(); await settle();
 assert.equal(d.querySelector('#service-name').textContent, 'New Service');
});

test('deferred background failure preserves saved change and recovery clears its warning', async t => {
 const h = await setup(t); const d = h.window.document;
 let finish;
 h.app.SaveConnection = () => new Promise(resolve => { finish = resolve; });
 d.querySelector('#token').value = 'token';
 d.querySelector('#form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
 h.emit();
 h.setFail(true);
 finish();
 await settle(); await settle(); await settle();
 assert.match(d.querySelector('#message').textContent, /Sample API connected.*Could not refresh the view/);
 h.setFail(false);
 h.setState({services: [sampleApi], connections: [{service: 'sample-api', permissions: ['profile']}]});
 h.window.dispatchEvent(new h.window.Event('focus'));
 await settle(); await settle();
 assert.equal(d.querySelector('#message').textContent, 'Sample API connected. Your tools can now call its API.');
 assert.equal(d.querySelector('#message').className, 'success');
 // A new interaction clears the prior mutation outcome.
 d.querySelector('[data-tab="activity"]').click();
 h.setFail(true); h.emit(); await settle(); await settle();
 assert.doesNotMatch(d.querySelector('#message').textContent, /Sample API connected/);
 assert.match(d.querySelector('#message').textContent, /last valid view/);
});

test('creation defers stale snapshots and selects the created service', async t => {
 const h = await setup(t); const d = h.window.document;
 d.querySelector('#add-service').click();
 for (const [key, value] of Object.entries({name: 'New Service', id: 'new-service', url: 'https://example.com'})) d.querySelector(`#custom-${key}`).value = value;
 d.querySelector('.endpoint-path').value = '/items';
 let finishRead, finishCreate;
 let reads = 0, creates = 0, created = false;
 h.app.State = async () => {
  reads++;
  if (reads === 1) return new Promise(resolve => { finishRead = () => resolve({services: [sampleApi], connections: [{service: 'sample-api', permissions: ['profile']}]}); });
  return {services: created ? [sampleApi, custom] : [sampleApi], connections: []};
 };
 h.app.CreateCustomService = () => {
  creates++;
  return new Promise(resolve => { finishCreate = () => { created = true; resolve(); }; });
 };
 h.emit(); await settle();
 const submit = () => d.querySelector('#custom-form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
 submit(); submit();
 assert.equal(creates, 1);
 finishCreate(); await settle(); finishRead();
 await settle(); await settle(); await settle();
 assert.equal(d.querySelector('#service-name').textContent, 'New Service');
 assert.equal(d.querySelector('#connections').hidden, false);
 assert.match(d.querySelector('#message').textContent, /New Service created/);
 assert.equal(d.querySelector('#token').disabled, false);
});

test('activity ignores older responses and older failures', async t => {
 for (const rejectOld of [false, true]) {
  await t.test(rejectOld ? 'stale failure' : 'stale result', async t => {
   const h = await setup(t); const d = h.window.document;
   const requests = [];
   h.app.Activity = () => new Promise((resolve, reject) => requests.push({resolve, reject}));
   d.querySelector('[data-tab="activity"]').click();
   d.querySelector('#refresh').click();
   requests[1].resolve([{time: '2026-01-02T00:00:00Z', operation: 'latest', outcome: 'success'}]);
   await settle();
   if (rejectOld) requests[0].reject(Error('obsolete failure'));
   else requests[0].resolve([{time: '2026-01-01T00:00:00Z', operation: 'obsolete', outcome: 'success'}]);
   await settle();
   assert.match(d.querySelector('#history').textContent, /latest/);
   assert.doesNotMatch(d.querySelector('#history').textContent, /obsolete/);
   assert.doesNotMatch(d.querySelector('#message').textContent, /obsolete failure/);
  });
 }
});

test('committed cleanup warnings refresh the UI and survive later refreshes', async t => {
 for (const operation of ['save', 'disconnect', 'delete']) {
  await t.test(operation, async t => {
   const h = await setup(t); const d = h.window.document;
   h.setState({services: [{...sampleApi, custom: true}], connections: [{service: 'sample-api', permissions: ['profile']}]});
   h.emit(); await settle(); await settle();
   const change = async () => {
    h.setState({services: operation === 'delete' ? [custom] : [{...sampleApi, custom: true}], connections: operation === 'save' ? [{service: 'sample-api', permissions: ['repos']}] : []});
    return {warning: 'Credential cleanup is pending.'};
   };
   if (operation === 'save') {
    d.querySelector('#reconnect').click();
    h.app.SaveConnection = change;
    d.querySelector('#token').value = 'new-token';
    d.querySelector('#form').dispatchEvent(new h.window.Event('submit', {bubbles: true, cancelable: true}));
   } else if (operation === 'disconnect') {
    h.app.RemoveConnection = change; d.querySelector('#remove').click();
   } else {
    h.app.DeleteCustomService = change; d.querySelector('#delete-service').click(); d.querySelector('#confirm-delete').click();
   }
   await settle(); await settle();
   assert.match(d.querySelector('#message').textContent, /Credential cleanup is pending/);
   if (operation === 'save') assert.equal(d.querySelector('[value="repos"]').checked, true);
   else assert.equal(d.querySelector('#badge').textContent, 'Not connected');
   h.window.dispatchEvent(new h.window.Event('focus')); await settle(); await settle();
   assert.match(d.querySelector('#message').textContent, /Credential cleanup is pending/);
   assert.equal(d.querySelector('#message').className, 'error');
  });
 }
});

test('delete confirmation opens a dialog and cancellation keeps the service', async t => {
 const h = await setup(t); const d = h.window.document;
 h.setState({services:[{...sampleApi,custom:true}],connections:[]}); h.emit();
 await settle(); await settle();
 let deletes = 0;
 h.app.DeleteCustomService = async () => { deletes++; return {}; };
 d.querySelector('#delete-service').click();
 assert.equal(d.querySelector('#delete-confirm').open, true);
 assert.equal(d.querySelector('#delete-title').textContent, 'Delete Sample API?');
 d.querySelector('#cancel-delete').click();
 assert.equal(d.querySelector('#delete-confirm').open, false);
 assert.equal(deletes, 0);
});

test('connected service hides credentials until reconnect and cancel restores saved permissions', async t => {
 const h = await setup(t); const d = h.window.document;
 const endpoint = {id:'items',label:'GET /items/{id}',description:'Read /items/{id}'};
 h.setState({services:[{...sampleApi,permissions:[endpoint]}],connections:[{service:'sample-api',permissions:['items']}]});
 h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('#token-fields').hidden, true);
 assert.equal(d.querySelector('#connected-overview'), null);
 assert.equal(d.querySelector('#badge').textContent, 'Connected');
 assert.equal(d.querySelector('#save-connection').hidden, true);
 assert.equal(d.querySelector('[name="permission"]').disabled, true);
 assert.equal(d.querySelector('#permissions small'), null);
 assert.equal(d.querySelector('#permission-count').textContent, '1 of 1 enabled');
 d.querySelector('#reconnect').click();
 assert.equal(d.querySelector('#token-fields').hidden, false);
 assert.equal(d.querySelector('#token').required, true);
 assert.equal(d.querySelector('[name="permission"]').disabled, false);
 d.querySelector('#token').value = 'unsaved';
 d.querySelector('[name="permission"]').checked = false;
 d.querySelector('#cancel-reconnect').click();
 assert.equal(d.querySelector('#token').value, '');
 assert.equal(d.querySelector('[name="permission"]').checked, true);
 assert.equal(d.querySelector('#token-fields').hidden, true);
});

test('custom permission descriptions can be edited without reconnecting', async t => {
 const h = await setup(t); const d = h.window.document;
 const service = {...custom, permissions:[{id:'endpoint-1',label:'GET /items',description:'Read /items'}]};
 h.setState({services:[service],connections:[{service:service.id,permissions:['endpoint-1']}]}); h.emit(); await settle(); await settle();
 let saved;
 h.app.UpdatePermissionDescriptions = async (id, descriptions) => {
  saved = {id, descriptions};
  h.setState({services:[{...service,permissions:[{...service.permissions[0],description:descriptions['/items']}]}],connections:[{service:service.id,permissions:['endpoint-1']}]});
  return {};
 };
 d.querySelector('.permission-edit').click();
 assert.equal(d.querySelector('.permission-editor').hidden, false);
 const input = d.querySelector('.permission-description'); assert.equal(input.value,''); input.value='Inventory access';
 d.querySelector('.permission-editor .primary').click();
 await settle(); await settle();
 assert.deepEqual(JSON.parse(JSON.stringify(saved)), {id:service.id,descriptions:{'/items':'Inventory access'}});
 assert.equal(d.querySelector('.permission-editor').hidden,true);
 assert.equal(d.querySelector('#permissions small').textContent,'Inventory access');
 assert.equal(d.querySelector('[name="permission"]').checked,true);
 assert.equal(d.querySelector('#token-fields').hidden,true);
});

test('inline description editing preserves permission selection and supports cancel and retry', async t => {
 const h = await setup(t); const d = h.window.document;
 const service = {...custom,permissions:[{id:'endpoint-1',label:'GET /items',description:'Read inventory'}]};
 h.setState({services:[service],connections:[]});h.emit();await settle();await settle();
 const check = d.querySelector('[name="permission"]'); check.checked=false;check.dispatchEvent(new h.window.Event('change',{bubbles:true}));
 d.querySelector('.permission-edit').click();
 const input=d.querySelector('.permission-description'); input.value='Unsaved';input.dispatchEvent(new h.window.Event('input',{bubbles:true}));
 d.querySelector('.permission-editor .secondary').click();
 assert.equal(check.checked,false);
 assert.equal(d.querySelector('.permission-editor').hidden,true);
 d.querySelector('.permission-edit').click();assert.equal(input.value,'Read inventory');
 h.app.UpdatePermissionDescriptions=async()=>{throw Error('Could not save');};
 input.value='Try again';input.dispatchEvent(new h.window.Event('input',{bubbles:true}));
 input.dispatchEvent(new h.window.KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));await settle();
 assert.equal(d.querySelector('.permission-editor').hidden,false);
 assert.equal(input.value,'Try again');
 assert.match(d.querySelector('.permission-editor .error').textContent,/Could not save/);
 assert.equal(check.checked,false);
 d.querySelector('#token').value='draft-token';
 h.app.UpdatePermissionDescriptions=async()=>{
  h.setState({services:[{...service,permissions:[{...service.permissions[0],description:'Try again'}]}],connections:[]});
  return {};
 };
 d.querySelector('.permission-editor .primary').click();await settle();await settle();
 assert.equal(d.querySelector('[name="permission"]').checked,false);
 assert.equal(d.querySelector('#token').value,'draft-token');
 assert.equal(d.querySelector('#permissions small').textContent,'Try again');
});

test('feedback starts empty and disconnect feedback sits beneath the service header', async t => {
 const h = await setup(t); const d = h.window.document;
 const banner = d.querySelector('#message');
 assert.equal(banner.matches(':empty'), true);
 h.setState({services:[sampleApi],connections:[{service:'sample-api',permissions:['profile']}]});h.emit();await settle();await settle();
 h.app.RemoveConnection = async () => {h.setState({services:[sampleApi],connections:[]});return {};};
 d.querySelector('#remove').click();await settle();await settle();
 assert.equal(banner.textContent,'Sample API disconnected.');
 assert.equal(banner.parentElement.id,'service-content');
 assert.equal(banner.previousElementSibling.className,'connection');
 assert.equal(banner.nextElementSibling.className,'access-panel');
 h.app.Activity = async () => [];
 d.querySelector('[data-tab="activity"]').click();await settle();
 assert.equal(banner.matches(':empty'),true);
 assert.equal(banner.parentElement.id,'activity');
});

test('empty service library offers creation and recovers after adding or deleting a service', async t => {
 const h = await setup(t); const d = h.window.document;
 h.setState({services:[],connections:[]}); h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('#service-count').textContent,'0');
 assert.equal(d.querySelector('#connection-count').textContent,'0 connected');
 assert.equal(d.querySelector('#services').children.length,0);
 assert.equal(d.querySelector('#service-content').hidden,true);
 assert.equal(d.querySelector('#custom').hidden,false);
 assert.equal(d.querySelectorAll('#custom-endpoints .endpoint-row').length,1);
 assert.equal(d.querySelector('#custom-login input').disabled,true);
 d.querySelector('#cancel-custom').click();
 assert.equal(d.querySelector('#custom').hidden,false);
 h.setState({services:[custom],connections:[]}); h.emit(); await settle(); await settle();
 d.querySelector('#services button').click();
 assert.equal(d.querySelector('#service-content').hidden,false);
 assert.equal(d.querySelector('#custom').hidden,true);
 assert.equal(d.querySelector('#service-name').textContent,'New Service');
 h.setState({services:[],connections:[]}); h.emit(); await settle(); await settle();
 assert.equal(d.querySelector('#service-content').hidden,true);
 assert.equal(d.querySelector('#custom').hidden,false);
});

test('update notice links to the release, preserves offline notices, and avoids overlapping checks', async t => {
 const h = await setup(t); const d = h.window.document;
 const notice = d.querySelector('#update-notice');
 assert.equal(d.querySelector('#app-version').textContent,'v0.1.0');
 assert.equal(d.querySelector('#app-version').hidden,false);
 assert.equal(notice.hidden,true);
 let finish, calls = 0, opened;
 h.app.CheckForUpdate = () => { calls++; return new Promise(resolve => { finish = resolve; }); };
 h.window.runtime.BrowserOpenURL = url => { opened = url; };
 h.window.dispatchEvent(new h.window.Event('focus'));
 h.window.dispatchEvent(new h.window.Event('focus'));
 assert.equal(calls,1);
 finish({version:'0.2.0',downloadURL:'https://github.com/example/midwing/releases/tag/v0.2.0'});
 await settle();
 assert.equal(notice.hidden,false);
 assert.match(notice.textContent,/0\.2\.0/);
 notice.click();
 assert.equal(opened,'https://github.com/example/midwing/releases/tag/v0.2.0');
 h.app.CheckForUpdate = async () => { throw Error('offline'); };
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.equal(notice.hidden,false);
 h.app.CheckForUpdate = async () => ({version:'',downloadURL:''});
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.equal(notice.hidden,true);
});
