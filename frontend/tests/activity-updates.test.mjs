import test from 'node:test';
import assert from 'node:assert/strict';
import {settle, setup} from './helpers.mjs';

test('activity ignores older responses and older failures', async t => {
 for (const rejectOld of [false, true]) {
  await t.test(rejectOld ? 'stale failure' : 'stale result', async t => {
   const h = await setup(t); const d = h.window.document;
   const requests = [];
   h.app.Activity = () => new Promise((resolve, reject) => requests.push({resolve, reject}));
   d.querySelector('[data-tab="activity"]').click();
   d.querySelector('#refresh').click();
   requests[1].resolve({entries:[{time: '2026-01-02T00:00:00Z', operation: 'latest', outcome: 'success'}]});
   await settle();
   if (rejectOld) requests[0].reject(Error('obsolete failure'));
   else requests[0].resolve({entries:[{time: '2026-01-01T00:00:00Z', operation: 'obsolete', outcome: 'success'}],warning:'obsolete warning'});
   await settle();
   assert.match(d.querySelector('#history').textContent, /latest/);
   assert.doesNotMatch(d.querySelector('#history').textContent, /obsolete/);
   assert.doesNotMatch(d.querySelector('#message').textContent, /obsolete failure/);
   assert.equal(d.querySelector('#history-warning').hidden, true);
  });
 }
});

test('activity displays recovered entries with a warning and clears it after recovery', async t => {
 const h = await setup(t); const d = h.window.document;
 const entry = {time:'2026-01-02T00:00:00Z', operation:'recovered request', outcome:'success'};
 h.app.Activity = async () => ({entries:[entry], warning:'some activity records were damaged and skipped'});
 d.querySelector('[data-tab="activity"]').click(); await settle();
 assert.match(d.querySelector('#history').textContent, /recovered request/);
 assert.match(d.querySelector('#history-warning').textContent, /damaged and skipped/);
 assert.equal(d.querySelector('#history-warning').hidden, false);
 h.app.Activity = async () => ({entries:[entry]});
 d.querySelector('#refresh').click(); await settle();
 assert.equal(d.querySelector('#history-warning').hidden, true);
 assert.equal(d.querySelector('#history-warning').textContent, '');
 h.app.Activity = async () => ({entries:[], warning:'some activity records were damaged and skipped'});
 d.querySelector('#refresh').click(); await settle();
 assert.equal(d.querySelector('#history-warning').hidden, false);
 assert.match(d.querySelector('#history').textContent, /No readable activity/);
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
