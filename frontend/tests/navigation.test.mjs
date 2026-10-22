import test from 'node:test';
import assert from 'node:assert/strict';
import {sampleApi, custom, settle, setup} from './helpers.mjs';

test('service search filters names, IDs and origins and survives settings refresh', async t => {
 const h = await setup(t); const d = h.window.document;
 h.setState({services:[sampleApi, custom], connections:[]}); h.emit();
 await settle(); await settle();
 const search = d.querySelector('#service-search');
 const find = query => { search.value = query; search.dispatchEvent(new h.window.Event('input')); return [...d.querySelectorAll('#services button:not([hidden])')]; };
 assert.equal(find('new SERVICE')[0].getAttribute('aria-label'), 'New Service, not connected');
 assert.equal(find('sample-api').length, 1);
 assert.equal(find('example.com')[0].getAttribute('aria-label'), 'New Service, not connected');
 h.setState({services:[sampleApi, custom], connections:[{service:'new-service',permissions:[]}]}); h.emit();
 await settle(); await settle();
 assert.equal(d.querySelectorAll('#services button:not([hidden])').length, 1);
 assert.equal(find('missing').length, 0);
 assert.equal(d.querySelector('#services-empty').hidden, false);
 search.dispatchEvent(new h.window.KeyboardEvent('keydown', {key:'Escape',bubbles:true}));
 assert.equal(search.value, '');
 assert.equal(d.querySelector('#services-empty').hidden, true);
 assert.equal(d.querySelectorAll('#services button:not([hidden])').length, 2);
});

test('keyboard navigation updates current page and focuses search and add service', async t => {
 const h = await setup(t, {Activity: async () => ({entries:[],warning:''})}); const d = h.window.document;
 const key = value => d.dispatchEvent(new h.window.KeyboardEvent('keydown', {key:value,metaKey:true,bubbles:true,cancelable:true}));
 key('2');
 assert.equal(d.querySelector('#activity').hidden, false);
 assert.equal(d.querySelector('nav [aria-current="page"]').dataset.tab, 'activity');
 assert.equal(d.activeElement, d.querySelector('#activity h1'));
 key('f');
 assert.equal(d.querySelector('#connections').hidden, false);
 assert.equal(d.activeElement.id, 'service-search');
 key('n');
 assert.equal(d.querySelector('#custom').hidden, false);
 assert.equal(d.activeElement.id, 'custom-name');
});

test('refreshes preserve service-list focus and scroll position', async t => {
 const h = await setup(t); const d = h.window.document;
 const list = d.querySelector('#services');
 list.querySelector('button').focus(); list.scrollTop = 120; list.scrollLeft = 80;
 h.setState({services:[sampleApi, custom], connections:[]}); h.emit();
 await settle(); await settle();
 assert.equal(d.activeElement.dataset.service, sampleApi.id);
 assert.equal(list.scrollTop, 120);
 assert.equal(list.scrollLeft, 80);
});

test('shortcuts leave modified, consumed, and modal key events alone', async t => {
 const h = await setup(t); const d = h.window.document;
 const key = options => d.dispatchEvent(new h.window.KeyboardEvent('keydown', {key:'n',metaKey:true,bubbles:true,cancelable:true,...options}));
 for (const options of [{shiftKey:true}, {altKey:true}, {isComposing:true}]) {
  assert.equal(key(options), true);
  assert.equal(d.querySelector('#custom').hidden, true);
 }
 const consumed = new h.window.KeyboardEvent('keydown', {key:'n',metaKey:true,bubbles:true,cancelable:true});
 consumed.preventDefault(); d.dispatchEvent(consumed);
 assert.equal(d.querySelector('#custom').hidden, true);
 d.querySelector('#delete-confirm').showModal();
 assert.equal(key({}), true);
 assert.equal(d.querySelector('#custom').hidden, true);
});

test('service arrow navigation skips search results that are hidden', async t => {
 const h = await setup(t); const d = h.window.document;
 h.setState({services:[sampleApi, custom],connections:[]}); h.emit(); await settle(); await settle();
 const search = d.querySelector('#service-search'); search.value = 'new'; search.dispatchEvent(new h.window.Event('input'));
 search.dispatchEvent(new h.window.KeyboardEvent('keydown', {key:'ArrowDown',bubbles:true,cancelable:true}));
 assert.match(d.activeElement.getAttribute('aria-label'), /New Service/);
 d.activeElement.dispatchEvent(new h.window.KeyboardEvent('keydown', {key:'Home',bubbles:true,cancelable:true}));
 assert.match(d.activeElement.getAttribute('aria-label'), /New Service/);
});
