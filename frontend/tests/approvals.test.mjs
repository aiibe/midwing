import test from 'node:test';
import assert from 'node:assert/strict';
import {custom, settle, setup} from './helpers.mjs';

for (const authKind of ['token', 'password']) {
 test(`${authKind} approval submits the displayed revision and survives a stale rejection`, async t => {
  const h = await setup(t);
  const d = h.window.document;
  const reviewed = {...custom, authKind};
  h.setState({services:[reviewed], connections:[]});
  h.emit(); await settle(); await settle();
  // The store changes before its watcher notification reaches the app.
  const updated = {...reviewed, revision:'new-contract'};
  h.setState({services:[updated], connections:[]});
  let submitted;
  const method = authKind === 'password' ? 'ConnectPassword' : 'SaveConnection';
  h.app[method] = async (...args) => {
   submitted = args;
   throw Error('service changed; reload and review permissions before reconnecting');
  };
  d.querySelector('#token').value = 'secret-token';
  d.querySelector('#email').value = 'me@example.com';
  d.querySelector('#password').value = 'secret-password';
  d.querySelector('[value="repos"]').checked = false;
  const submit = () => d.querySelector('#form').dispatchEvent(new h.window.Event('submit', {bubbles:true,cancelable:true}));
  submit(); await settle();
  assert.deepEqual(structuredClone(submitted), authKind === 'password'
   ? [reviewed.id, reviewed.revision, 'me@example.com', 'secret-password', ['profile']]
   : [reviewed.id, reviewed.revision, 'secret-token', ['profile']]);
  assert.match(d.querySelector('#message').textContent, /review permissions/);
  assert.equal(d.querySelector('#badge').textContent, 'Not connected');
  assert.equal(d.querySelector('#token').value, '');
  assert.equal(d.querySelector('#password').value, '');
  // Refresh requires a fresh review, then sends the new revision.
  h.emit(); await settle(); await settle();
  assert.equal(d.querySelector('[value="repos"]').checked, true);
  h.app[method] = async (...args) => { submitted = args; return {}; };
  d.querySelector('#token').value = 'new-token';
  d.querySelector('#password').value = 'new-password';
  submit(); await settle(); await settle();
  assert.equal(submitted[1], updated.revision);
 });
}
