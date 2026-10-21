import test from 'node:test';
import assert from 'node:assert/strict';
import {sampleApi, custom, settle, setup} from './helpers.mjs';

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
   d.querySelector('.endpoint-method[value="POST"]').checked = true;
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
   assert.deepEqual(created.endpoints, [{path: '/items/{id}', description: 'Read item details', methods: ['GET', 'POST'], queryParameters: ['limit', 'cursor']}]);
   if (authKind === 'password') {
    assert.deepEqual(created.password, {loginPath: '/login', emailField: 'email', passwordField: 'password', tokenField: 'session.token', refreshPath: '/refresh'});
   } else assert.equal(created.password, undefined);
   assert.equal(d.querySelector('#service-name').textContent, 'New Service');
   assert.equal(d.querySelector('#connections').hidden, false);
   assert.equal(d.querySelector('#password-fields').hidden, authKind !== 'password');
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
