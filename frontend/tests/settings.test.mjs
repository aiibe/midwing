import test from 'node:test';
import assert from 'node:assert/strict';
import {sampleApi, custom, settle, setup} from './helpers.mjs';

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
 assert.equal(d.querySelector('#message').textContent, 'Sample API connected.');
 assert.equal(d.querySelector('#message').className, 'success');
 // A new interaction clears the prior mutation outcome.
 d.querySelector('[data-tab="activity"]').click();
 h.setFail(true); h.emit(); await settle(); await settle();
 assert.doesNotMatch(d.querySelector('#message').textContent, /Sample API connected/);
 assert.match(d.querySelector('#message').textContent, /last valid view/);
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
