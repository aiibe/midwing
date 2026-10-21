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

export {sampleApi, custom, settle, setup};
