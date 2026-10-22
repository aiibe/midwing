import test from 'node:test';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
import {bundleFrontend, script, settle} from './helpers.mjs';

const previewScript = await bundleFrontend({'import.meta.env.DEV':'true'});

test('production bundles exclude the sample backend', () => {
 assert.doesNotMatch(script, /setupPreview|api\.github\.com|api\.linear\.app|preview-secret/);
});

test('browser without the desktop bridge shows guidance instead of a credential form', async t => {
 const dom = new JSDOM('<div id="app"></div>', {runScripts:'outside-only',pretendToBeVisual:true});
 t.after(() => dom.window.close());
 dom.window.eval(script); await settle();
 const d = dom.window.document;
 assert.equal(d.querySelector('#service-content').hidden, true);
 assert.match(d.querySelector('#message').textContent, /Open the desktop app/);
 assert.equal(d.querySelector('#message').closest('[hidden]'), null);
});

test('development preview boots and edits sample connections without keeping credentials', async t => {
 const dom = new JSDOM('<div id="app"></div>', {
  url:'http://127.0.0.1:5173/?preview', runScripts:'outside-only', pretendToBeVisual:true,
 });
 t.after(() => dom.window.close());
 const {window} = dom; const d = window.document;
 window.structuredClone = structuredClone;
 window.HTMLDialogElement.prototype.close = function() { this.open = false; };
 window.eval(previewScript);
 await settle(); await settle();
 assert.equal(d.querySelector('#preview-label').hidden, false);
 assert.equal(d.querySelector('#nav-count').textContent, '4');
 assert.equal(d.querySelector('#connection-count').textContent, '2 connected');
 assert.equal(d.querySelector('#cli-warning').hidden, true);
 assert.equal(d.querySelector('#app').classList.contains('desktop'), false);
 d.querySelector('#reconnect').click();
 d.querySelector('#token').value = 'preview-secret';
 d.querySelector('#form').dispatchEvent(new window.Event('submit', {bubbles:true,cancelable:true}));
 await settle(); await settle();
 assert.equal(d.querySelector('#badge').textContent, 'Connected');
 assert.equal(d.querySelector('#token').value, '');
 assert.doesNotMatch(JSON.stringify(await window.go.main.App.State()), /preview-secret/);
 assert.match(d.querySelector('#message').textContent, /GitHub connected/);
});
