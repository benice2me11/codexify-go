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
    querySelectorAll(selector) { assert.ok(['button','select'].includes(selector)); return all.filter(el => el.tag === selector); },
  };
  const listeners = new Map();
  const calls = [];
  let active = 0, maxActive = 0, exhausted = false;
  const sourceRoot = 'C:\\fixture';
  const defaultWorkspace = {projectRoot:sourceRoot,sourceProjectRoot:sourceRoot,managedWorktree:false,worktreeMode:'auto'};
  let currentWorkspace = options.initialWorkspace === undefined ? defaultWorkspace : options.initialWorkspace;
  let previousWorkspace = null, awaitingSelection = false;
  const fixtureGroups = [{
    name:'job-search',primary:'work',repositories:[
      {alias:'work',project:'work-search',checkout:'source',sourceBranch:'main',sourceProjectRoot:'C:\\fixture\\work'},
      {alias:'hh',project:'hh-mcp-server',checkout:'source',sourceBranch:'master',sourceProjectRoot:'C:\\fixture\\hh'},
      {alias:'habr',project:'habr-career-mcp',checkout:'source',sourceBranch:'master',sourceProjectRoot:'C:\\fixture\\habr'},
    ],
  }];
  const sourcePath = alias => 'C:\\fixture\\'+alias;
  const managedPath = alias => 'C:\\fixture\\managed-'+alias;
  const selectMember = alias => currentWorkspace.repositories.find(member=>member.alias===alias);
  function updateGroupActiveRoot(){
    const active=selectMember(currentWorkspace.activeRepository);
    currentWorkspace.projectRoot=active.projectRoot;
    currentWorkspace.sourceProjectRoot=active.sourceProjectRoot;
    currentWorkspace.managedWorktree=active.managedWorktree;
  }
  const window = {
    addEventListener(name, listener) { const list = listeners.get(name)||[]; list.push(listener); listeners.set(name,list); },
    dispatchEvent(event) { for (const listener of listeners.get(event.type)||[]) listener(event); return true; },
  };
  function emit(globals) {
    Object.assign(window.openai, globals);
    window.dispatchEvent({type:'openai:set_globals',detail:{globals}});
  }
  window.openai = {toolOutput:options.initialOutput||null,toolResponseMetadata:options.initialMetadata||null, async callTool(name, params) {
    if (calls.length >= 64) { exhausted = true; throw new Error('synthetic call budget exhausted'); }
    calls.push({name, params:JSON.parse(JSON.stringify(params))});
    active++; maxActive=Math.max(maxActive,active);
    try {
      await tick();
      if (options.fail) throw new Error('fixture failure');
      if (options.failObject) throw options.failObject;
      if (options.toolError) return {isError:true,content:options.toolError};
      let data;
      switch (name) {
        case 'setup_status': data={workspace:currentWorkspace,awaitingSelection,worktreeMode:'auto',
          projectGroupsEnabled:!!options.groups,groups:options.groups?fixtureGroups:[],
          ...(options.schemaStale?{conversationStale:true,connectorSchema:{status:'stale',refreshRecommended:true}}:{})}; break;
        case 'list_projects': data={projects:[{name:'Fixture',selector:'fixture',gitRepository:true},{name:'Plain project',selector:'plain',gitRepository:false}]}; break;
        case 'set_project_group':
          if(currentWorkspace)throw new Error('already bound');
          if(params.name!=='job-search')throw new Error('unknown fixture group');
          currentWorkspace={mode:'group',groupName:'job-search',primaryRepository:'work',activeRepository:'work',generation:1,worktreeMode:'auto',
            repositories:fixtureGroups[0].repositories.map(member=>{
              const checkout=params.checkouts?.[member.alias]||member.checkout;
              return {alias:member.alias,checkout,sourceProjectRoot:sourcePath(member.alias),
                projectRoot:checkout==='managed'?managedPath(member.alias):sourcePath(member.alias),
                sourceBranch:member.sourceBranch,
                branch:checkout==='managed'?'codex/fixture-'+member.alias:member.sourceBranch,
                managedWorktree:checkout==='managed',checkoutRevision:1};
            })};
          updateGroupActiveRoot();awaitingSelection=false;data=currentWorkspace;
          break;
        case 'set_active_repository':
          if(!currentWorkspace||currentWorkspace.mode!=='group')throw new Error('not bound to group');
          if(!selectMember(params.repository))throw new Error('unknown member');
          currentWorkspace.activeRepository=params.repository;currentWorkspace.generation++;
          updateGroupActiveRoot();data=currentWorkspace;
          break;
        case 'set_repository_checkout': {
          if(!currentWorkspace||currentWorkspace.mode!=='group')throw new Error('not bound to group');
          const member=selectMember(params.repository);
          if(!member)throw new Error('unknown member');
          if(!['source','managed'].includes(params.checkout))throw new Error('invalid checkout');
          if(member.checkout!==params.checkout){
            if(params.checkout==='source'){
              member.preservedManagedProjectRoot=member.projectRoot;member.projectRoot=member.sourceProjectRoot;
            }else{
              member.projectRoot=member.preservedManagedProjectRoot||managedPath(member.alias);
              member.preservedManagedProjectRoot='';
            }
            member.managedWorktree=params.checkout==='managed';member.checkout=params.checkout;
            member.branch=params.checkout==='managed'?'codex/fixture-'+member.alias:member.sourceBranch;
            member.checkoutRevision++;currentWorkspace.generation++;
            updateGroupActiveRoot();
          }
          data=currentWorkspace;
          break;
        }
        case 'chat_read': data={user_text:'Fixture unread message',state:'ready'}; break;
        case 'chat_write': data={state:'written'}; break;
        case 'self_update_status': data={status:'up_to_date',currentVersion:'fixture-v1',detail:'fixture status'}; break;
        case 'set_project_root':
          if (params.resumePath) {
            if (!awaitingSelection || !previousWorkspace || previousWorkspace.projectRoot!==params.resumePath) throw new Error('invalid resumePath');
            currentWorkspace=previousWorkspace;
          } else {
            if (currentWorkspace) throw new Error('already bound');
            if (params.withoutProject) {
              currentWorkspace={mode:'scratch',projectRoot:sourceRoot+'\\scratch',sourceProjectRoot:'',managedWorktree:false,worktreeMode:'auto'};
            } else {
              if (params.createWorktree===true && params.path==='plain') throw new Error('not a Git repository');
              const managed = params.createWorktree===true || (params.createWorktree===undefined && options.autoWorktree===true);
              currentWorkspace={projectRoot:managed?sourceRoot+'\\managed-fixture':sourceRoot,sourceProjectRoot:sourceRoot,managedWorktree:managed,worktreeMode:'auto'};
            }
          }
          awaitingSelection=false;
          data=currentWorkspace;
          break;
        case 'setup_ui_switch_project':
          if (!currentWorkspace || currentWorkspace.projectRoot!==params.expectedPath) throw new Error('wrong expectedPath');
          previousWorkspace=currentWorkspace;
          currentWorkspace=null;
          awaitingSelection=true;
          data={previousRoot:previousWorkspace.projectRoot,awaitingSelection:true};
          break;
        default: throw new Error('unexpected fixture tool '+name);
      }
      if (options.feedback !== false) {
        // A host may deliver a state change during a call, or after its promise
        // settles. The asynchronous case makes a mere in-flight flag inadequate.
        if (options.feedback === 'sync') emit({toolOutput:data});
        else setImmediate(() => emit({toolOutput:data}));
      }
      return {structuredContent:data,...(options.responseMeta?{_meta:options.responseMeta}:{})};
    } finally { active--; }
  }};
  const context = vm.createContext({window,document,console});
  for (const script of html.matchAll(/<script>([\s\S]*?)<\/script>/g)) vm.runInContext(script[1],context,{timeout:1000});
  return {calls,elements,all,emit,get maxActive(){return maxActive},get exhausted(){return exhausted},
    click(id) { const el=elements.get(id); assert.ok(el&&el.onclick,id); if(!el.disabled) return el.onclick(); },
  };
}

const setupToken = 'A'.repeat(43);
const setupMeta = {'io.github.devnoname120/codexify/setup-context':setupToken};
const fixtureProjects = [{name:'Fixture',selector:'fixture',gitRepository:true},{name:'Plain project',selector:'plain',gitRepository:false}];
const selectedWorkspace = {projectRoot:'C:\\fixture',sourceProjectRoot:'C:\\fixture',managedWorktree:false,worktreeMode:'auto'};
const unboundPayload = {workspace:null,selected:false,projects:{projects:fixtureProjects}};
const boundPayload = {workspace:selectedWorkspace,selected:true};
const expectedInitial = {SetupHTML:0,ChatHTML:1,UpdateHTML:1,DiffHTML:0};
const results = [];
for (const [name,html] of Object.entries(pages)) {
  const h=host(html,name==='SetupHTML'?{initialOutput:boundPayload,initialMetadata:setupMeta}:{});
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
    assert.equal(h.calls.length,name==='SetupHTML'?1:2*expectedInitial[name],name+' repeated overlapping clicks');
    assert.equal(h.maxActive,1,name+' concurrent widget calls');
    assert.equal(h.elements.get(id).disabled,false,name+' button remained disabled');
    if(name==='SetupHTML') {
      const before=h.calls.length;
      h.click('switch');
      await settle();
      assert.equal(h.calls.length,before+3,'switch must call once and refresh');
      const beforeScratch=h.calls.length;
      for(let i=0;i<20;i++) h.click('scratch');
      await settle();
      assert.equal(h.calls.length,beforeScratch+2,'scratch mutation should run once then status read');
      assert.equal(h.calls.filter(c=>c.name==='set_project_root').length,1);
      assert.ok(h.elements.get('status').innerHTML.includes('Scratch workspace'));
      assert.equal(h.elements.get('projects').children.length,0,'project list must hide while bound');
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
    if(name==='SetupHTML')failed.click('refresh');
    await settle();
    const count=failed.calls.length;
    for(let i=0;i<20;i++) failed.emit({toolOutput:{unrelated:'error event'}});
    await settle();
    assert.equal(failed.calls.length,count,'error response initiated retries');
    assert.ok(failed.elements.get('error').textContent.includes('fixture failure'));
    if (name==='SetupHTML') {
      const objectFailure=host(html,{failObject:{code:'MCP_BRIDGE_ERROR',message:'Linux setup call rejected'}});
      objectFailure.click('refresh');
      await settle();
      assert.ok(objectFailure.elements.get('error').textContent.includes('Linux setup call rejected'),'object-shaped bridge error must be readable');
      assert.ok(!objectFailure.elements.get('error').textContent.includes('[object Object]'));
      const mcpFailure=host(html,{toolError:[{type:'text',text:'Cached Linux v1 Setup request rejected'}]});
      mcpFailure.click('refresh');
      await settle();
      assert.ok(mcpFailure.elements.get('error').textContent.includes('Cached Linux v1 Setup request rejected'),'MCP isError text must be visible');
    }
    const sync=host(html,{feedback:'sync'});
    await settle();
    assert.equal(sync.calls.length,expectedInitial[name],'synchronous feedback loop');
  }
  results.push({name,initialCalls:expectedInitial[name],callsAfterInteractions:h.calls.length,maxConcurrent:h.maxActive,globalsTriggeredCalls:0});
}

if (!baseline) {
  // Exercise the actual shipped SetupHTML controller, not a substitute.
  const setup = pages.SetupHTML;
  const choose = (h,index=0) => {
    const btn = h.elements.get('projects').children[index].children[1];
    assert.ok(btn,'project Select button');
    return btn;
  };
  const setMode = (h,value) => {
    const picker=h.elements.get('workspace-mode');
    picker.value=value;
    picker.onchange();
  };
  const tool = (h,name) => h.calls.filter(call=>call.name===name);

  const auto=host(setup,{initialWorkspace:null,autoWorktree:true,initialOutput:unboundPayload,initialMetadata:setupMeta});
  await settle();
  assert.equal(auto.calls.length,0,'setup card should render mounted results without calling tools');
  assert.equal(auto.elements.get('workspace-mode').value,'auto');
  choose(auto).onclick();
  await settle();
  assert.equal(tool(auto,'set_project_root').length,1);
  assert.equal(Object.hasOwn(tool(auto,'set_project_root')[0].params,'createWorktree'),false,'Auto must omit bool entirely');
  assert.equal(tool(auto,'set_project_root')[0].params.uiContext,setupToken,'verified setup identity must travel with action');
  assert.ok(auto.elements.get('status').innerHTML.includes('Managed worktree'),'actual mode must be displayed');

  const shared=host(setup,{initialWorkspace:null,autoWorktree:true,initialOutput:unboundPayload,initialMetadata:setupMeta});
  await settle();
  setMode(shared,'source');
  assert.ok(shared.elements.get('mode-warning').textContent.includes('Concurrent writes'),'shared source warning missing');
  const select=choose(shared);
  for(let i=0;i<20;i++) if(!select.disabled) select.onclick();
  await settle();
  assert.equal(tool(shared,'set_project_root').length,1,'source rapid clicks duplicated mutation');
  assert.equal(tool(shared,'set_project_root')[0].params.createWorktree,false,'must pass boolean false');
  assert.ok(shared.elements.get('status').innerHTML.includes('Source checkout'));
  assert.ok(shared.elements.get('advanced-info').textContent.includes('Source:'),'technical paths belong in Advanced');
  assert.equal(shared.elements.get('status').innerHTML.includes('C:\\fixture'),false,'selected summary must not expose an absolute path');

  const managed=host(setup,{initialWorkspace:null,initialOutput:unboundPayload,initialMetadata:setupMeta});
  await settle();
  setMode(managed,'managed');
  assert.equal(choose(managed,1).disabled,true,'non-Git project must reject managed mode');
  choose(managed).onclick();
  await settle();
  assert.equal(tool(managed,'set_project_root')[0].params.createWorktree,true,'must pass boolean true');
  assert.ok(managed.elements.get('status').innerHTML.includes('Managed worktree'));

  // Same conversation: worktree -> source, then source -> worktree; no automatic rebinding.
  const managedWorkspace={projectRoot:'C:\\fixture\\managed-fixture',sourceProjectRoot:'C:\\fixture',managedWorktree:true,worktreeMode:'auto'};
  const swapped=host(setup,{initialWorkspace:managedWorkspace,initialOutput:{workspace:managedWorkspace,selected:true},initialMetadata:setupMeta});
  await settle();
  setMode(swapped,'source');
  assert.equal(swapped.elements.get('projects').children.length,0,'selected workspace should hide picker');
  swapped.click('switch');
  await settle();
  assert.ok(swapped.elements.get('status').innerHTML.includes('Previous source files'),'old worktree preservation notice missing');
  choose(swapped).onclick();
  await settle();
  assert.equal(tool(swapped,'set_project_root')[0].params.createWorktree,false);
  swapped.click('switch');
  await settle();
  setMode(swapped,'managed');
  choose(swapped).onclick();
  await settle();
  assert.equal(tool(swapped,'set_project_root')[1].params.createWorktree,true);
  assert.equal(tool(swapped,'setup_ui_switch_project').length,2);
  assert.equal(swapped.exhausted,false);

  const restore=host(setup,{initialOutput:boundPayload,initialMetadata:setupMeta});
  await settle();
  restore.click('switch');
  await settle();
  assert.equal(restore.elements.get('restore').hidden,false,'restore available after explicit switch');
  restore.click('restore');
  await settle();
  assert.equal(tool(restore,'set_project_root')[0].params.resumePath,'C:\\fixture','restore previous root supplied by backend');
  assert.equal(Object.hasOwn(tool(restore,'set_project_root')[0].params,'createWorktree'),false);
  assert.equal(restore.elements.get('restore').hidden,true);

  const stale=host(setup,{initialWorkspace:null,initialOutput:unboundPayload,initialMetadata:setupMeta});
  await settle();
  choose(stale).onclick();
  stale.emit({toolResponseMetadata:{'io.github.devnoname120/codexify/setup-context':'B'.repeat(43)}});
  await settle();
  assert.ok(stale.elements.get('error').textContent.includes('context changed'),'stale UI context must be rejected');
  assert.equal(tool(stale,'set_project_root').length,1,'stale reply must not retry mutation');

  // Host globals during selections must be passive, including after mutating tools.
  for(let i=0;i<100;i++) managed.emit({theme:i%2?'dark':'light',maxHeight:300+i});
  await settle();
  assert.equal(tool(managed,'set_project_root').length,1,'host globals triggered an extra selection');
  results.push({name:'WorkspaceModeSelection',auto:'omits createWorktree',source:'explicit false',managed:'explicit true',switch:'two directions',restore:'saved workspace',duplicateCalls:0});

  const groupCatalog=[{name:'job-search',primary:'work',repositories:[
    {alias:'work',project:'work-search',checkout:'source'},
    {alias:'hh',project:'hh-mcp-server',checkout:'source'},
    {alias:'habr',project:'habr-career-mcp',checkout:'source'},
  ]}];
  const groupMounted={workspace:null,selected:false,projectGroupsEnabled:true,groups:groupCatalog};
  const group=host(setup,{
    groups:true,initialWorkspace:null,initialOutput:groupMounted,initialMetadata:setupMeta,
  });
  await settle();
  assert.equal(group.calls.length,0,'group setup card should render host payload without calls');
  assert.equal(group.elements.get('groups-wrap').hidden,false);
  assert.equal(group.elements.get('workspace-kind').value,'group','configured groups should be discoverable by default');
  const groupCard=group.elements.get('groups').children[0];
  assert.ok(groupCard,'group option must exist');
  const initialHHMode=group.elements.get('group-settings').children[0].children[2].children[1];
  assert.equal(initialHHMode.value,'source');
  initialHHMode.value='managed';initialHHMode.onchange();
  const selectGroup=groupCard.children[0].children.at(-1);
  for(let i=0;i<20;i++)if(!selectGroup.disabled)selectGroup.onclick();
  await settle();
  assert.equal(tool(group,'set_project_group').length,1,'rapid group clicks created duplicate bindings');
  assert.equal(tool(group,'set_project_group')[0].params.name,'job-search');
  assert.equal(tool(group,'set_project_group')[0].params.checkouts.hh,'managed','initial member checkout not forwarded');
  assert.equal(tool(group,'set_project_group')[0].params.checkouts.habr,'source');
  assert.equal(tool(group,'set_project_group')[0].params.uiContext,setupToken);
  assert.ok(group.elements.get('status').innerHTML.includes('job-search'));
  assert.equal(group.elements.get('compact-members').children.length,3,'compact view must list the repositories');
  assert.equal(group.elements.get('compact-members').children[1].children[0].textContent,'hh-mcp-server');
  assert.equal(group.elements.get('compact-members').children[1].children[1].textContent,'Managed · codex/fixture-hh');
  assert.equal(group.elements.get('compact-members').children[0].children[1].textContent,'Source · main');
  assert.equal(group.elements.get('status').innerHTML.includes('C:\\fixture'),false,'paths must not appear in main group card');
  assert.ok(group.elements.get('advanced-info').textContent.includes('C:\\fixture'),'advanced must keep source and active identities');
  assert.equal(group.elements.get('members').children.length,4,'must render 3 independently managed members');
  assert.equal(group.elements.get('mode-row').hidden,true,'single workspace mode picker should not appear for group');

  function memberControls(h,index) {
    const row=h.elements.get('members').children[index+1];
    assert.ok(row,'missing group member '+index);
    return row.children.at(-1);
  }
  const hh=memberControls(group,1);
  assert.equal(hh.children[2].value,'managed','group summary lost HH managed kind');
  hh.children[0].onclick();
  await settle();
  assert.equal(tool(group,'set_active_repository').length,1);
  assert.equal(tool(group,'set_active_repository')[0].params.repository,'hh');
  assert.ok(group.elements.get('advanced-info').textContent.includes('hh'),'active default belongs in Advanced');
  const hhActive=memberControls(group,1);
  hhActive.children[2].value='source';
  hhActive.children[3].onclick();
  await settle();
  assert.equal(tool(group,'set_repository_checkout').length,1);
  assert.equal(tool(group,'set_repository_checkout')[0].params.checkout,'source');
  assert.equal(tool(group,'set_repository_checkout')[0].params.repository,'hh');
  const hhSource=memberControls(group,1);
  hhSource.children[2].value='managed';
  hhSource.children[3].onclick();
  await settle();
  assert.equal(tool(group,'set_repository_checkout').length,2);
  assert.equal(tool(group,'set_repository_checkout')[1].params.checkout,'managed');
  assert.equal(tool(group,'set_project_root').length,0,'member switch must not call global set_project_root');
  assert.equal(group.exhausted,false);
  const groupCalls=group.calls.length;
  for(let i=0;i<100;i++)group.emit({theme:i%2?'dark':'light',maxHeight:150+i});
  await settle();
  assert.equal(group.calls.length,groupCalls,'group host events caused an MCP tool burst');

  const staleGroup=host(setup,{
    groups:true,schemaStale:true,initialWorkspace:null,initialMetadata:setupMeta,
    initialOutput:{...groupMounted,conversationStale:true,connectorSchema:{status:'stale',refreshRecommended:true}},
  });
  await settle();
  assert.ok(staleGroup.elements.get('schema-warning').textContent.includes('Refresh/reconnect'));
  const staleChoose=staleGroup.elements.get('groups').children[0].children[0].children.at(-1);
  assert.equal(staleChoose.disabled,true,'stale connector must disable group selection');
  const staleCount=staleGroup.calls.length;
  staleChoose.onclick();
  await settle();
  assert.equal(staleGroup.calls.length,staleCount,'stale group selection should not call tools');

  results.push({name:'ProjectGroupsSetupV5',selection:'three members',active:'member alias',checkout:'managed → source → managed',stale:'blocked',duplicateCalls:0});
}

console.log(JSON.stringify({mode:baseline?'bounded-original-reproduction':'regression',host:'synthetic; no hosted MCP calls',results},null,2));
