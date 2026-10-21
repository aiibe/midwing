import test from 'node:test';
import assert from 'node:assert/strict';
import {settle, setup} from './helpers.mjs';

const fresh = {
 available:true, installed:false, canInstall:true, canRepair:false, canRemove:false,
 destination:'/Users/Test User/.local/bin/midwing',
 target:'/Applications/Midwing.app/Contents/MacOS/midwing', existingTarget:'',
 message:'Install CLI to create a link to this app.', warning:'',
 pathCommand:'export PATH="$HOME/.local/bin:$PATH"',
};

async function cli(t, state = fresh) {
 const h = await setup(t);
 h.app.CLISetupStatus = async () => structuredClone(state);
 h.window.dispatchEvent(new h.window.Event('focus'));
 await settle();
 h.window.document.querySelector('[aria-label="CLI setup"]').click();
 return h;
}

test('fresh setup offers installation with exact paths and terminal guidance', async t => {
 const h = await cli(t); const d = h.window.document;
 assert.equal(d.querySelector('#cli-setup').hidden, false);
 assert.equal(d.querySelector('#connections').hidden, true);
 assert.equal(d.querySelector('#cli-notice').hidden, false);
 assert.equal(d.querySelector('[data-cli-action="install"]').hidden, false);
 assert.equal(d.querySelector('[data-cli-action="repair"]').hidden, true);
 assert.equal(d.querySelector('#cli-destination').textContent, fresh.destination);
 assert.equal(d.querySelector('#cli-path').textContent, fresh.pathCommand);
 assert.match(d.querySelector('#cli-details').textContent, /fresh terminal/);
 assert.match(d.querySelector('#cli-details').textContent, /Restart coding agents/);
 let action;
 h.app.ChangeCLISetup = async value => { action = value; return {...fresh, installed:true, canInstall:false, canRemove:true, message:'CLI link is installed.'}; };
 d.querySelector('[data-cli-action="install"]').click(); await settle();
 assert.equal(action, 'install');
 assert.equal(d.querySelector('#cli-notice').hidden, true);
 assert.equal(d.querySelector('[data-cli-action="remove"]').hidden, false);
 assert.match(d.querySelector('#cli-status').textContent, /installed/);
});

test('repair shows old target and removal returns to installable state', async t => {
 const h = await cli(t, {...fresh, canInstall:false, canRepair:true, canRemove:true,
  existingTarget:'/Users/Test User/Applications/Old.app/Contents/MacOS/midwing', message:'Repair will replace the user link.'});
 const d = h.window.document;
 assert.match(d.querySelector('#cli-existing').textContent, /Old.app/);
 assert.equal(d.querySelector('#cli-existing-row').hidden, false);
 assert.equal(d.querySelector('#cli-notice').textContent, 'Repair CLI setup');
 assert.equal(d.querySelector('[data-cli-action="repair"]').hidden, false);
 const actions = [];
 h.app.ChangeCLISetup = async action => { actions.push(action); return action === 'repair'
  ? {...fresh, installed:true, canInstall:false, canRemove:true} : fresh; };
 d.querySelector('[data-cli-action="repair"]').click(); await settle();
 d.querySelector('[data-cli-action="remove"]').click(); await settle();
 assert.deepEqual(actions, ['repair', 'remove']);
 assert.equal(d.querySelector('[data-cli-action="install"]').hidden, false);
});

test('legacy setup avoids duplicate install; conflicts expose guidance without actions', async t => {
 for (const state of [
  {...fresh, installed:true, canInstall:false, message:'Existing /usr/local/bin/midwing works.',
   pathCommand:'export PATH="/usr/local/bin:$PATH"'},
  {...fresh, canInstall:false, warning:'Another command exists; run type -a midwing.'},
  {...fresh, available:false, canInstall:false, message:'Launch the installed copy.'},
 ]) {
  const h = await cli(t, state); const d = h.window.document;
  for (const button of d.querySelectorAll('[data-cli-action]')) assert.equal(button.hidden, true);
  assert.equal(d.querySelector('#cli-status').textContent, state.message);
  assert.equal(d.querySelector('#cli-details').hidden, !state.available);
  assert.equal(d.querySelector('#cli-path').textContent, state.pathCommand);
  if (state.warning) assert.match(d.querySelector('#cli-warning').textContent, /type -a/);
 }
});

test('failed operations remain retryable, and pending writes suppress overlapping calls', async t => {
 const h = await cli(t); const d = h.window.document;
 let reject, calls = 0;
 h.app.ChangeCLISetup = () => { calls++; return new Promise((_, fail) => { reject = fail; }); };
 const install = d.querySelector('[data-cli-action="install"]');
 install.click(); install.click();
 h.window.dispatchEvent(new h.window.Event('focus'));
 assert.equal(install.disabled, true);
 assert.equal(calls, 1);
 reject(Error('permission denied')); await settle();
 assert.match(d.querySelector('#cli-error').textContent, /permission denied/);
 assert.equal(install.disabled, false);
 assert.equal(d.querySelector('#cli-destination').textContent, fresh.destination);
 d.querySelector('#cli-refresh').click(); await settle();
 assert.equal(d.querySelector('#cli-error').textContent, '');
 h.app.CLISetupStatus = async () => { throw Error('unreadable destination'); };
 d.querySelector('#cli-refresh').click(); await settle();
 assert.match(d.querySelector('#cli-error').textContent, /unreadable destination/);
 assert.equal(d.querySelector('#cli-destination').textContent, fresh.destination);
});
