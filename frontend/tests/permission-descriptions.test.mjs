import test from 'node:test';
import assert from 'node:assert/strict';
import {custom, settle, setup} from './helpers.mjs';

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

test('POST description editing sends the endpoint path', async t => {
 const h = await setup(t); const d = h.window.document;
 const service = {...custom,permissions:[{id:'endpoint-1-post',label:'POST /items',description:'Create an item'}]};
 h.setState({services:[service],connections:[]});h.emit();await settle();await settle();
 let saved;
 h.app.UpdatePermissionDescriptions = async (id, descriptions) => {saved = {id, descriptions}; return {};};
 d.querySelector('.permission-edit').click();
 const input = d.querySelector('.permission-description'); input.value = 'Submit an item';
 assert.equal(input.getAttribute('aria-label'), 'Description for POST /items');
 assert.equal(input.placeholder, 'Submit a request to /items');
 input.dispatchEvent(new h.window.Event('input',{bubbles:true}));
 d.querySelector('.permission-editor .primary').click();await settle();await settle();
 assert.equal(saved.id, service.id);
 assert.deepEqual(JSON.parse(JSON.stringify(saved.descriptions)), {'/items':'Submit an item'});
});

test('GET and POST share one description editor and preserve its draft across refreshes', async t => {
 const h = await setup(t); const d = h.window.document;
 const service = {...custom, permissions:[
  {id:'endpoint-1',label:'GET /items',description:'Read /items'},
  {id:'endpoint-1-post',label:'POST /items',description:'Submit a request to /items'},
 ]};
 h.setState({services:[service],connections:[]});h.emit();await settle();await settle();
 assert.equal(d.querySelectorAll('.permission-edit').length, 1);
 d.querySelector('.permission-edit').click();
 assert.match(d.querySelector('.permission-editor').textContent, /applies to GET and POST/);
 const input = d.querySelector('.permission-description'); input.value = 'Manage items';
 input.dispatchEvent(new h.window.Event('input',{bubbles:true}));
 h.setState({services:[{...service,name:'Renamed'}],connections:[]});h.emit();await settle();await settle();
 assert.equal(d.querySelector('.permission-description').value, 'Manage items');
 h.app.UpdatePermissionDescriptions = async (id, descriptions) => {
  assert.equal(id, service.id);
  assert.deepEqual(JSON.parse(JSON.stringify(descriptions)), {'/items':'Manage items'});
  h.setState({services:[{...service,permissions:service.permissions.map(p=>({...p,description:'Manage items'}))}],connections:[]});
  return {};
 };
 d.querySelector('.permission-editor .primary').click();await settle();await settle();
 assert.deepEqual([...d.querySelectorAll('#permissions small')].map(el=>el.textContent), ['Manage items','Manage items']);
 assert.equal(d.querySelectorAll('.permission-edit').length, 1);
});
