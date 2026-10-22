import test from 'node:test';
import assert from 'node:assert/strict';
import {settle, setup} from './helpers.mjs';

const ready = {
 available:true, installed:true, autoSetupDisabled:false, setupError:'',
 destination:'/Users/Test User/.local/bin/midwing',
 target:'/Applications/Midwing.app/Contents/MacOS/midwing', existingTarget:'',
 message:'CLI link installed. Check command discovery in your terminal.',
 pathCommand:'export PATH="$HOME/.local/bin:$PATH"',
};
const disabled = {...ready, installed:false, autoSetupDisabled:true,
 message:'Automatic CLI setup is disabled.'};
const missing = {...ready, installed:false, message:'The CLI link needs setup.'};

test('CLI menu badge explains non-ready states from any screen and clears on recovery', async t => {
 for (const [name, state, description] of [
  ['missing', missing, /Set up the CLI/],
  ['disabled', disabled, /CLI is disabled/],
  ['unavailable', {...missing, available:false}, /unavailable in this build/],
  ['failed setup', {...ready, setupError:'permission denied'}, /CLI setup failed/],
 ]) {
  await t.test(name, async t => {
   const h = await setup(t, {CLISetupStatus: async () => state}); const d = h.window.document;
   const badge = d.querySelector('#cli-warning');
   const menu = d.querySelector('nav [data-tab="cli-setup"]');
   assert.equal(d.querySelector('#connections').hidden, false);
   assert.equal(badge.hidden, false);
   assert.match(badge.title, description);
   assert.equal(menu.getAttribute('aria-describedby'), 'cli-warning-text');
   assert.equal(d.querySelector('#cli-warning-text').textContent, badge.title);
   menu.click(); await settle();
   assert.equal(d.querySelector('#cli-setup').hidden, false);
   h.app.CLISetupStatus = async () => ready;
   h.window.dispatchEvent(new h.window.Event('focus')); await settle();
   assert.equal(badge.hidden, true);
   assert.equal(menu.hasAttribute('aria-describedby'), false);
  });
 }
});

test('CLI status read failures show the menu badge until a successful refresh', async t => {
 const h = await setup(t, {CLISetupStatus: async () => { throw Error('status unavailable'); }});
 const badge = h.window.document.querySelector('#cli-warning');
 assert.equal(badge.hidden, false);
 assert.match(badge.title, /needs attention/);
 h.app.CLISetupStatus = async () => ready;
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.equal(badge.hidden, true);
});

async function cli(t, state = ready) {
 const h = await setup(t, {CLISetupStatus: async () => structuredClone(state)});
 h.window.document.querySelector('[aria-label="CLI setup"]').click();
 await settle();
 return {...h, d:h.window.document};
}

test('ready setup stays quiet and keeps terminal help and technical details collapsed', async t => {
 const {d} = await cli(t);
 assert.equal(d.querySelector('#cli-setup').hidden, false);
 assert.equal(d.querySelector('#connections').hidden, true);
 assert.equal(d.querySelector('#cli-status').textContent, 'Ready');
 assert.equal(d.querySelector('#cli-notice').hidden, true);
 assert.equal(d.querySelector('#cli-retry').hidden, true);
 assert.equal(d.querySelector('#cli-enabled').checked, true);
 assert.equal(d.querySelector('#cli-destination').textContent, ready.destination);
 assert.equal(d.querySelector('#cli-path').textContent, ready.pathCommand);
 for (const details of d.querySelectorAll('#cli-setup details')) assert.equal(details.open, false);
 assert.equal(d.querySelector('#cli-setup').textContent.includes('App executable:'), false);
 assert.match(d.querySelector('#cli-controls').textContent, /Restart coding agents/);
});

test('Enable CLI toggles removal and opt-in without a disabled sidebar prompt', async t => {
 const h = await cli(t); const {d} = h;
 const actions = [];
 h.app.ChangeCLISetup = async action => {
  actions.push(action);
  return action === 'disable' ? disabled : ready;
 };
 const toggle = d.querySelector('#cli-enabled');
 toggle.click(); await settle();
 assert.equal(toggle.checked, false);
 assert.equal(d.querySelector('#cli-status').textContent, 'Disabled');
 assert.equal(d.querySelector('#cli-notice').hidden, true);
 assert.equal(d.querySelector('#cli-retry').hidden, true);
 h.app.CLISetupStatus = async () => disabled;
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.equal(toggle.checked, false);
 toggle.click(); await settle();
 assert.deepEqual(actions, ['disable', 'enable']);
 assert.equal(toggle.checked, true);
 assert.equal(d.querySelector('#cli-status').textContent, 'Ready');
});

test('missing or moved links use the same retry action', async t => {
 for (const state of [missing, {...missing, existingTarget:'/Applications/Old.app/Contents/MacOS/midwing'}]) {
  const h = await cli(t, state); const {d} = h;
  assert.equal(d.querySelector('#cli-status').textContent, 'Needs attention');
  assert.equal(d.querySelector('#cli-notice').hidden, false);
  assert.equal(d.querySelector('#cli-retry').hidden, false);
  let action;
  h.app.ChangeCLISetup = async value => { action = value; return ready; };
  d.querySelector('#cli-retry').click(); await settle();
  assert.equal(action, 'ensure');
  assert.equal(d.querySelector('#cli-notice').hidden, true);
 }
});

test('conflicts explain preservation; unavailable builds hide setup controls', async t => {
 const h = await cli(t, {...missing, message:'Another command occupies the CLI location. It has been preserved.'});
 assert.match(h.d.querySelector('#cli-message').textContent, /preserved/);
 assert.equal(h.d.querySelector('#cli-notice').hidden, false);
 const unavailable = {...missing, available:false, message:'Launch the installed copy.'};
 h.app.CLISetupStatus = async () => unavailable;
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.equal(h.d.querySelector('#cli-status').textContent, 'Unavailable');
 assert.equal(h.d.querySelector('#cli-controls').hidden, true);
 assert.equal(h.d.querySelector('#cli-notice').hidden, true);
 assert.equal(h.d.querySelector('#cli-retry').hidden, true);
});

test('page opening and focus refresh status without a refresh button', async t => {
 const h = await cli(t, missing); const {d} = h;
 h.app.CLISetupStatus = async () => ready;
 d.querySelector('[aria-label="Connections"]').click();
 d.querySelector('[aria-label="CLI setup"]').click(); await settle();
 assert.equal(d.querySelector('#cli-status').textContent, 'Ready');
 h.app.CLISetupStatus = async () => disabled;
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.equal(d.querySelector('#cli-status').textContent, 'Disabled');
});

test('startup errors survive refresh and successful retry clears the notice', async t => {
 const h = await cli(t, {...missing, setupError:'permission denied'}); const {d} = h;
 assert.match(d.querySelector('#cli-error').textContent, /permission denied/);
 assert.equal(d.querySelector('#cli-status').textContent, 'Needs attention');
 h.window.dispatchEvent(new h.window.Event('focus')); await settle();
 assert.match(d.querySelector('#cli-error').textContent, /permission denied/);
 h.app.ChangeCLISetup = async () => ready;
 d.querySelector('#cli-retry').click(); await settle();
 assert.equal(d.querySelector('#cli-error').textContent, '');
 assert.equal(d.querySelector('#cli-notice').hidden, true);
});

test('pending writes prevent overlapping actions and failed removal preserves the saved opt-out', async t => {
 const h = await cli(t); const {d} = h;
 let reject, calls = 0;
 h.app.ChangeCLISetup = () => { calls++; return new Promise((_, fail) => { reject = fail; }); };
 const toggle = d.querySelector('#cli-enabled');
 toggle.click(); toggle.click();
 h.window.dispatchEvent(new h.window.Event('focus'));
 assert.equal(toggle.disabled, true);
 assert.equal(calls, 1);
 h.app.CLISetupStatus = async () => ({...disabled, installed:true, setupError:'permission denied'});
 reject(Error('permission denied')); await settle();
 assert.equal(toggle.disabled, false);
 assert.equal(toggle.checked, false);
 assert.match(d.querySelector('#cli-error').textContent, /permission denied/);
 assert.equal(d.querySelector('#cli-retry').hidden, false);
 h.app.ChangeCLISetup = async () => disabled;
 d.querySelector('#cli-retry').click(); await settle();
 assert.equal(d.querySelector('#cli-status').textContent, 'Disabled');
 assert.equal(d.querySelector('#cli-notice').hidden, true);
});

test('an unreadable initial status leaves retry accessible', async t => {
 const h = await setup(t, {CLISetupStatus: async () => { throw Error('unreadable destination'); }});
 const d = h.window.document;
 d.querySelector('[aria-label="CLI setup"]').click(); await settle();
 assert.equal(d.querySelector('#cli-notice').hidden, false);
 const retry = d.querySelector('#cli-retry');
 assert.equal(retry.hidden, false);
 assert.equal(retry.closest('[hidden]'), null);
 assert.match(d.querySelector('#cli-error').textContent, /unreadable destination/);
 h.app.ChangeCLISetup = async () => ready;
 retry.click(); await settle();
 assert.equal(d.querySelector('#cli-status').textContent, 'Ready');
});

test('refreshes requested during an in-flight read run after it finishes', async t => {
 const h = await cli(t, missing); const {d} = h;
 let finish, reads = 0;
 h.app.CLISetupStatus = async () => {
  reads++;
  return reads === 1 ? new Promise(resolve => { finish = resolve; }) : ready;
 };
 h.window.dispatchEvent(new h.window.Event('focus'));
 d.querySelector('[aria-label="Connections"]').click();
 d.querySelector('[aria-label="CLI setup"]').click();
 finish(missing); await settle();
 assert.equal(reads, 2);
 assert.equal(d.querySelector('#cli-status').textContent, 'Ready');
});

test('inspection failures retain the saved opt-out and allow retry', async t => {
 const h = await cli(t, {...disabled, setupError:'Could not inspect CLI link: permission denied'});
 const {d} = h;
 assert.equal(d.querySelector('#cli-controls').hidden, false);
 assert.equal(d.querySelector('#cli-enabled').checked, false);
 assert.equal(d.querySelector('#cli-enabled').disabled, false);
 assert.equal(d.querySelector('#cli-status').textContent, 'Needs attention');
 assert.equal(d.querySelector('#cli-retry').hidden, false);
 h.app.ChangeCLISetup = async () => disabled;
 d.querySelector('#cli-retry').click(); await settle();
 assert.equal(d.querySelector('#cli-status').textContent, 'Disabled');
});

test('copy buttons send the exact command path and PATH line to the native clipboard', async t => {
 const h = await cli(t); const {d} = h;
 const copied = [];
 h.window.runtime.ClipboardSetText = async value => { copied.push(value); return true; };
 d.querySelector('[data-cli-copy="cli-destination"]').click(); await settle();
 d.querySelector('[data-cli-copy="cli-path"]').click(); await settle();
 assert.deepEqual(copied, [ready.destination, ready.pathCommand]);
 assert.equal(d.querySelector('#cli-copy-status').textContent, 'Copied.');
 h.window.runtime.ClipboardSetText = async () => false;
 d.querySelector('[data-cli-copy="cli-path"]').click(); await settle();
 assert.match(d.querySelector('#cli-copy-status').textContent, /Select and copy/);
});
