// Offline host-event harness. No MCP, browser profile, or network is contacted.
// --baseline characterizes the original handlers under a bounded synthetic host
// that emits a globals notification after each tool result; it is not a claim
// about which events occurred in the historical hosted incident.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const args = process.argv.slice(2);
const baseline = args.includes('--baseline');
const sourceIndex = args.indexOf('--source');
let pages;
if (sourceIndex >= 0) {
  const source = fs.readFileSync(args[sourceIndex + 1], 'utf8');
  pages = Object.fromEntries(['SetupHTML', 'ChatHTML', 'UpdateHTML', 'DiffHTML'].map(name => {
    const match = source.match(new RegExp('const ' + name + ' = `([\\s\\S]*?)`'));
    assert.ok(match, name);
    return [name, match[1]];
  }));
} else {
  pages = JSON.parse(fs.readFileSync(0, 'utf8'));
}

const tick = () => new Promise(resolve => setImmediate(resolve));
async function settle() { for (let i = 0; i < 300; i++) await tick(); }

function host(html, options = {}) {
  const elements = new Map();
  const all = [];
  function element(tag = 'div') {
    const el = {tag, disabled:false, hidden:false, dataset:{}, children:[], value:'', onclick:null, _text:'', _html:'',
      append(...nodes) { this.children.push(...nodes); },
      set textContent(value) { this._text=String(value); this.children=[]; }, get textContent() { return this._text; },
      set innerHTML(value) { this._html=String(value); this.children=[]; }, get innerHTML() { return this._html; },
    };
    all.push(el); return el;
  }
  for (const match of html.matchAll(/<([a-z]+)[^>]*\bid="([^"]+)"/g)) elements.set(match[2], element(match[1]));
  const document = {
    getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
    createElement: element,
    querySelectorAll(selector) { assert.equal(selector, 'button'); return all.filter(el => el.tag === 'button'); },
  };
  const listeners = new Map();
  const calls = [];
  let active = 0, maxActive = 0, exhausted = false;
  const window = {
    addEventListener(name, listener) { const list = listeners.get(name)||[]; list.push(listener); listeners.set(name,list); },
    dispatchEvent(event) { for (const listener of listeners.get(event.type)||[]) listener(event); return true; },
  };
  function emit(globals) {
    Object.assign(window.openai, globals);
    window.dispatchEvent({type:'openai:set_globals',detail:{globals}});
  }
  window.openai = {toolOutput:options.initialOutput||null, async callTool(name, params) {
    if (calls.length >= 64) { exhausted = true; throw new Error('synthetic call budget exhausted'); }
    calls.push({name, params:JSON.parse(JSON.stringify(params))});
    active++; maxActive=Math.max(maxActive,active);
    try {
      await tick();
      if (options.fail) throw new Error('fixture failure');
      if (options.toolError?.name === name) return {isError:true,content:options.toolError.content};
      let data;
      switch (name) {
        case 'setup_status': data={workspace:{projectRoot:'C:\\fixture'},awaitingSelection:false}; break;
        case 'list_projects': data={projects:[{name:'Fixture',selector:'fixture'}]}; break;
        case 'chat_read': data={user_text:'Fixture unread message',state:'ready'}; break;
        case 'chat_write': data={state:'written'}; break;
        case 'self_update_status': data={status:'up_to_date',currentVersion:'fixture-v1',detail:'fixture status'}; break;
        case 'set_project_root': data={workspace:{projectRoot:'C:\\fixture'}}; break;
        case 'setup_ui_switch_project': data={awaitingSelection:true,workspace:null}; break;
        default: throw new Error('unexpected fixture tool '+name);
      }
      if (options.feedback !== false) {
        // A host may deliver a state change during a call, or after its promise
        // settles. The asynchronous case makes a mere in-flight flag inadequate.
        if (options.feedback === 'sync') emit({toolOutput:data});
        else setImmediate(() => emit({toolOutput:data}));
      }
      return {structuredContent:data};
    } finally { active--; }
  }};
  const context = vm.createContext({window,document,console});
  for (const script of html.matchAll(/<script>([\s\S]*?)<\/script>/g)) vm.runInContext(script[1],context,{timeout:1000});
  return {calls,elements,all,emit,get maxActive(){return maxActive},get exhausted(){return exhausted},
    click(id) { const el=elements.get(id); assert.ok(el&&el.onclick,id); if(!el.disabled) return el.onclick(); },
  };
}

const expectedInitial = {SetupHTML:2,ChatHTML:1,UpdateHTML:1,DiffHTML:0};
const results = [];
for (const [name,html] of Object.entries(pages)) {
  const h=host(html);
  await settle();
  if (baseline) {
    results.push({name,calls:h.calls.length,maxConcurrent:h.maxActive,budgetExhausted:h.exhausted});
    if (name==='DiffHTML') assert.equal(h.calls.length,0);
    else assert.equal(h.exhausted,true,'original widget failed to reproduce feedback: '+JSON.stringify({name,calls:h.calls,error:h.elements.get('error')?.textContent}));
    continue;
  }
  assert.equal(h.calls.length,expectedInitial[name],name+' amplified initial load');
  assert.equal(h.exhausted,false);
  for(let i=0;i<100;i++) h.emit({theme:i%2?'dark':'light',maxHeight:300+i});
  h.emit({toolOutput:{unrelated:'fixture'}});
  await settle();
  assert.equal(h.calls.length,expectedInitial[name],name+' initiated calls from globals');
  if(name==='DiffHTML') {
    h.emit({toolOutput:{output:'fixture diff'}});
    assert.equal(h.elements.get('diff').textContent,'fixture diff');
  } else {
    const id=name==='UpdateHTML'?'check':'refresh';
    for(let i=0;i<20;i++) h.click(id);
    await settle();
    assert.equal(h.calls.length,2*expectedInitial[name],name+' repeated overlapping clicks');
    assert.equal(h.maxActive,1,name+' concurrent widget calls');
    assert.equal(h.elements.get(id).disabled,false,name+' button remained disabled');
    if(name==='SetupHTML') {
      const before=h.calls.length;
      for(let i=0;i<20;i++) h.click('scratch');
      await settle();
      assert.equal(h.calls.length,before+3,'setup mutation should run once then two reads');
      assert.equal(h.calls.filter(c=>c.name==='set_project_root').length,1);
      assert.equal(h.elements.get('projects').children.length,1,'project list did not render');
      h.emit({toolOutput:{workspace:{projectRoot:'C:\\passive'},awaitingSelection:false}});
      assert.ok(h.elements.get('status').innerHTML.includes('passive'));
    } else if(name==='ChatHTML') {
      assert.equal(h.elements.get('messages').textContent,'Fixture unread message');
      h.elements.get('message').value='fixture outgoing';
      for(let i=0;i<20;i++) h.click('send');
      await settle();
      assert.equal(h.calls.filter(c=>c.name==='chat_write').length,1,'duplicate chat send');
      h.emit({toolOutput:{user_text:'Passive host text'}});
      assert.equal(h.elements.get('messages').textContent,'Passive host text');
    } else {
      assert.deepEqual(h.calls.map(c=>c.params.force),[false,true]);
      assert.ok(h.elements.get('status').textContent.includes('fixture-v1'));
    }
    const failed=host(html,{fail:true});
    await settle();
    const count=failed.calls.length;
    for(let i=0;i<20;i++) failed.emit({toolOutput:{unrelated:'error event'}});
    await settle();
    assert.equal(failed.calls.length,count,'error response initiated retries');
    assert.ok(failed.elements.get('error').textContent.includes('fixture failure'));
    const sync=host(html,{feedback:'sync'});
    await settle();
    assert.equal(sync.calls.length,expectedInitial[name],'synchronous feedback loop');
  }
  results.push({name,initialCalls:expectedInitial[name],callsAfterInteractions:h.calls.length,maxConcurrent:h.maxActive,globalsTriggeredCalls:0});
}
if (!baseline) {
  const errors = [
    {content:[{type:'text',text:'Selection rejected: <fixture identity missing>'}],expected:'Selection rejected: <fixture identity missing>'},
    {content:'Selection rejected by fixture',expected:'Selection rejected by fixture'},
    {content:[],expected:'The request failed.'},
  ];
  for (const fixture of errors) {
    const h=host(pages.SetupHTML,{feedback:false,toolError:{name:'set_project_root',content:fixture.content}});
    await settle();
    const previousStatus=h.elements.get('status').innerHTML;
    const before=h.calls.length;
    await h.elements.get('projects').children[0].children[1].onclick();
    await settle();
    assert.ok(h.elements.get('error').textContent.includes(fixture.expected),'Select must display MCP tool errors');
    assert.deepEqual(h.calls.slice(before).map(c=>c.name),['set_project_root'],'failed selection must not reload or retry');
    assert.equal(h.elements.get('status').innerHTML,previousStatus,'failed selection changed the displayed workspace');
    assert.ok(h.all.filter(el=>el.tag==='button').every(el=>!el.disabled),'failed selection left a disabled button');
  }
  results.push({name:'SetupToolErrors',cases:errors.length});
}
console.log(JSON.stringify({mode:baseline?'bounded-original-reproduction':'regression',host:'synthetic; no hosted MCP calls',results},null,2));
