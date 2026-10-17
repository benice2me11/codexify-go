package ui

import "github.com/modelcontextprotocol/go-sdk/mcp"

const (
	// A distinct resource URI invalidates cached v2 Setup cards after the
	// explicit checkout-mode and verified conversation-context redesign.
	SetupURI            = "ui://codexify-go/setup/v3/mcp-app.html"
	GroupSetupURI       = "ui://codexify-go/setup/v5/mcp-app.html"
	LegacyGroupSetupURI = "ui://codexify-go/setup/v4/mcp-app.html"
	LegacySetupURI      = "ui://codexify-go/setup/v2/mcp-app.html"
	// Older Linux Setup cards may still be mounted after an upgrade.
	LegacyLinuxSetupURI = "ui://codexify-go/setup/v1/mcp-app.html"
	DiffURI             = "ui://codexify-go/diff/v1/mcp-app.html"
	ChatURI             = "ui://codexify-go/markdown-chat/v2/mcp-app.html"
	UpdateURI           = "ui://codexify-go/self-update/v2/mcp-app.html"
	MIMEType            = "text/html;profile=mcp-app"
)

func SetupToolMeta() mcp.Meta {
	return SetupToolMetaForGroups(false)
}

func SetupToolMetaForGroups(enabled bool) mcp.Meta {
	resource := SetupURI
	if enabled {
		resource = GroupSetupURI
	}
	return mcp.Meta{
		"ui": map[string]any{
			"resourceUri": resource,
			"visibility":  []string{"model", "app"},
		},
		"ui/resourceUri":          resource,
		"openai/outputTemplate":   resource,
		"openai/widgetAccessible": true,
	}
}

// App-callable tools are visible to the model and the Setup App, but do not
// mount a duplicate Setup widget for each individual action.
func AppCallableToolMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"visibility": []string{"model", "app"},
		},
		"openai/widgetAccessible": true,
	}
}

func AppOnlyToolMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"visibility": []string{"app"},
		},
		"openai/visibility":       "private",
		"openai/widgetAccessible": true,
	}
}

func DiffToolMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"resourceUri": DiffURI,
			"visibility":  []string{"model"},
		},
		"ui/resourceUri": DiffURI,
	}
}

func ChatToolMeta() mcp.Meta {
	return toolMeta(ChatURI, []string{"model", "app"})
}

func UpdateToolMeta() mcp.Meta {
	return toolMeta(UpdateURI, []string{"app"})
}

func toolMeta(uri string, visibility []string) mcp.Meta {
	return mcp.Meta{
		"ui":                      map[string]any{"resourceUri": uri, "visibility": visibility},
		"ui/resourceUri":          uri,
		"openai/outputTemplate":   uri,
		"openai/widgetAccessible": true,
	}
}

func ResourceMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"prefersBorder": false,
			"csp": map[string]any{
				"connectDomains":  []string{},
				"resourceDomains": []string{},
			},
		},
		"openai/widgetPrefersBorder": false,
		"openai/widgetCSP": map[string]any{
			"connect_domains":  []string{},
			"resource_domains": []string{},
		},
	}
}

const SetupHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{font-family:system-ui,sans-serif;color-scheme:light dark}body{margin:0;padding:12px;font-size:14px}
.card{border:1px solid color-mix(in srgb,currentColor 19%,transparent);border-radius:13px;padding:16px;max-width:760px}
.header,.row,.compact-member,.group-option-head{display:flex;align-items:center;justify-content:space-between;gap:10px}
.row{justify-content:flex-start;flex-wrap:wrap}.header strong{font-size:1.02em}
.badge{font-size:.77em;border-radius:20px;padding:4px 9px;background:color-mix(in srgb,currentColor 9%,transparent)}
.workspace-summary{padding:20px 2px 13px}.workspace-name{font-size:1.1em;font-weight:600}.workspace-subtitle{margin-top:4px;font-size:.88em;opacity:.75}
.compact-member{padding:11px 2px;gap:12px;border-top:1px solid color-mix(in srgb,currentColor 9%,transparent)}
.repo-name{font-weight:550;overflow-wrap:anywhere}.repo-state{font-size:.82em;opacity:.8;text-align:right;white-space:nowrap}
.advanced{margin-top:13px;border-top:1px solid color-mix(in srgb,currentColor 9%,transparent);padding-top:12px}
.advanced>summary{cursor:pointer;font-size:.9em;opacity:.8;padding:5px 0;list-style:revert}
.advanced-content{margin-top:12px}.selection-tabs{margin:4px 0 10px}.selection-tabs label{display:flex;align-items:center;justify-content:space-between;gap:8px}
button{font:inherit;padding:7px 10px;border-radius:8px;border:1px solid color-mix(in srgb,currentColor 25%,transparent);background:transparent;cursor:pointer}
button:hover:enabled{background:color-mix(in srgb,currentColor 7%,transparent)}
select{font:inherit;padding:6px;border-radius:8px;max-width:100%}
.project{display:flex;justify-content:space-between;gap:8px;padding:9px 2px;border-top:1px solid color-mix(in srgb,currentColor 10%,transparent)}
.muted{opacity:.72;font-size:.89em}.warning{margin:7px 0;font-size:.88em}.notice{font-size:.9em;margin:7px 0}
.mode-row{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin:12px 0 6px}
.group-option{padding:11px 2px;border-top:1px solid color-mix(in srgb,currentColor 10%,transparent)}
.group-option-head{align-items:start}.group-option-members{font-size:.85em;opacity:.8;margin:7px 0}
.group-heading{font-weight:650;margin:14px 0 5px}
.group-member{padding:9px 0;border-top:1px solid color-mix(in srgb,currentColor 11%,transparent)}
.group-member .row{margin-top:6px}.group-controls{display:flex;gap:7px;flex-wrap:wrap;align-items:center}
.group-controls label{font-size:.88em}.group-path{font-size:.78em;word-break:break-all}
code{font-family:ui-monospace,monospace;font-size:.9em;overflow-wrap:anywhere}
#error{color:#c44;white-space:pre-wrap;padding-top:6px}
@media(max-width:420px){.repo-state{white-space:normal}.card{padding:12px}}
</style>
</head>
<body>
<div class="card">
  <div class="header"><strong>Workspace</strong><span id="workspace-badge" class="badge">Not selected</span></div>
  <div id="status" class="workspace-summary" aria-live="polite">Choose a workspace.</div>
  <div id="schema-warning" class="warning" role="status"></div>
  <div id="compact-members"></div>
  <div id="workspace-choice" class="selection-tabs"><label for="workspace-kind">Workspace type <select id="workspace-kind"><option value="single">Single repository</option><option value="group">Project group</option><option value="scratch">Scratch</option></select></label></div>
  <div id="groups-wrap" hidden><div id="groups"></div></div>
  <div id="projects"></div>
  <button id="scratch" hidden>Use scratch workspace</button>
  <details class="advanced" id="advanced">
    <summary>Advanced settings</summary>
    <div class="advanced-content">
      <div id="advanced-info" class="group-path muted"></div>
      <div id="mode-row" class="mode-row"><label for="workspace-mode">Checkout policy</label><select id="workspace-mode"><option value="auto" selected>Auto (configured policy)</option><option value="source">Source checkout</option><option value="managed">Managed worktree</option></select></div>
      <div id="mode-warning" class="warning muted"></div>
      <div id="group-settings"></div>
      <div id="members"></div>
      <div class="row"><button id="refresh">Refresh</button><button id="switch" hidden>Change workspace</button><button id="restore" hidden>Restore previous workspace</button></div>
    </div>
  </details>
  <div id="error" role="alert"></div>
</div>
<script>
const statusEl=document.getElementById("status"),projectsEl=document.getElementById("projects"),errorEl=document.getElementById("error"),switchBtn=document.getElementById("switch"),restoreBtn=document.getElementById("restore"),modeSelect=document.getElementById("workspace-mode"),warningEl=document.getElementById("mode-warning"),groupsWrap=document.getElementById("groups-wrap"),groupsEl=document.getElementById("groups"),membersEl=document.getElementById("members"),schemaEl=document.getElementById("schema-warning"),modeRow=document.getElementById("mode-row"),workspaceKind=document.getElementById("workspace-kind"),workspaceChoice=document.getElementById("workspace-choice"),compactMembers=document.getElementById("compact-members"),workspaceBadge=document.getElementById("workspace-badge"),advancedInfo=document.getElementById("advanced-info"),groupSettings=document.getElementById("group-settings"),scratchBtn=document.getElementById("scratch");
let uiContext="",observedHostContext="",hostContextRevision=0;
function setupContext(value){
  if(!value||typeof value!=="object")return "";
  const key="io.github.devnoname120/codexify/setup-context";
  for(const meta of [value,value._meta,value.call_tool_result&&value.call_tool_result._meta,value.mcp_tool_result&&value.mcp_tool_result._meta]){
    const token=meta&&meta[key];
    if(typeof token==="string"&&/^[A-Za-z0-9_-]{43}$/.test(token))return token;
  }
  return "";
}
function applyHostContext(current){
  if(current!==uiContext){uiContext=current;hostContextRevision++;}
}
function syncHostContext(){
  const current=setupContext(window.openai&&window.openai.toolResponseMetadata);
  if(current!==observedHostContext){observedHostContext=current;applyHostContext(current);}
}
function structured(r){return r&&((r.structuredContent)||(r.structured_content)||(r.result&&r.result.structuredContent))||null}
async function call(name,args={}){
  if(!(window.openai&&window.openai.callTool))throw new Error("Tool calls are unavailable in this host");
  syncHostContext();
  const revision=hostContextRevision;
  const result=await window.openai.callTool(name,uiContext?{...args,uiContext}:args);
  syncHostContext();
  if(revision!==hostContextRevision)throw new Error("Workspace context changed while this request was running. Refresh to continue.");
  const nextContext=setupContext(result);if(nextContext)uiContext=nextContext;
  if(result&&result.isError===true){
    const message=typeof result.content==="string"?result.content:Array.isArray(result.content)?result.content.filter(c=>c&&c.type==="text"&&typeof c.text==="string").map(c=>c.text).join("\n"):"";
    throw new Error(message.trim()||"The request failed.");
  }
  return result;
}
let busy=false,currentWorkspace=null,restorePath="",knownProjects=[],knownGroups=[],groupsEnabled=false,schemaStale=false,workspaceKindInitialized=false;
modeSelect.value="auto";
function selectionMode(){return modeSelect.value||"auto"}
function errorText(e){
  const value=e&&typeof e==="object"?(e.message||(e.data&&e.data.message)||JSON.stringify(e)):String(e);
  return String(value||"Unknown error").replace(/\s+/g," ").trim().slice(0,512);
}
function renderModeWarning(){
  const mode=selectionMode();
  warningEl.textContent=mode==="source"?"Shared checkout: other conversations or local services may use these files. Concurrent writes can conflict.":mode==="managed"?"Requires a Git repository. Creates a separate checkout and preserves the source directory.":"Uses the configured auto / always / never worktree policy.";
}
function renderChoice(){
  const bound=!!currentWorkspace;
  if(!workspaceKindInitialized&&groupsEnabled&&knownGroups.length){
    workspaceKind.value="group";workspaceKindInitialized=true;
  }
  if(!groupsEnabled&&workspaceKind.value==="group")workspaceKind.value="single";
  const kind=workspaceKind.value;
  workspaceChoice.hidden=bound;
  groupsWrap.hidden=bound||kind!=="group"||!groupsEnabled||knownGroups.length===0;
  projectsEl.hidden=bound||kind!=="single";
  scratchBtn.hidden=bound||kind!=="scratch";
  modeRow.hidden=bound||kind!=="single";
  groupSettings.hidden=bound||kind!=="group";
}
function sourceName(path){
  return String(path||"").replace(/\\/g,"/").split("/").filter(Boolean).pop()||"Repository";
}
function renderStatus(s){
  const w=s.workspace||null;
  currentWorkspace=w;
  if(w&&w.mode==="group"){
    workspaceBadge.textContent="Project group";
    statusEl.innerHTML="<div class=workspace-name>"+escapeHTML(w.groupName)+"</div><div class=workspace-subtitle>"+(w.repositories||[]).length+" repositories · Primary: "+escapeHTML(w.primaryRepository)+"</div>";
    advancedInfo.textContent="Default alias: "+(w.activeRepository||w.primaryRepository)+" · Active checkout: "+(w.projectRoot||"")+". Repository aliases route each tool call independently; changing the default is optional.";
  }else if(w){
    const actual=w.mode==="scratch"?"Scratch":w.managedWorktree?"Managed worktree":"Source checkout";
    workspaceBadge.textContent=w.mode==="scratch"?"Scratch":"Single repository";
    statusEl.innerHTML="<div class=workspace-name>"+escapeHTML(w.mode==="scratch"?"Scratch workspace":sourceName(w.sourceProjectRoot||w.projectRoot))+"</div><div class=workspace-subtitle>"+actual+"</div>";
    advancedInfo.textContent="Active: "+(w.projectRoot||w.workspaceRoot||"")+" · Source: "+(w.sourceProjectRoot||"")+" · Policy: "+(w.worktreeMode||s.worktreeMode||"unknown");
  }else{
    workspaceBadge.textContent="Not selected";
    statusEl.innerHTML="<div class=workspace-subtitle>"+(s.awaitingSelection?"Choose a workspace. Previous source files and managed worktrees have not been removed.":"Choose a workspace to begin.")+"</div>";
    advancedInfo.textContent="";
  }
  switchBtn.hidden=!w;
  switchBtn.textContent="Change workspace";
  switchBtn.dataset.path=w&&w.projectRoot||"";
  restoreBtn.hidden=!!w||!s.awaitingSelection||!restorePath;
  if(w)projectsEl.textContent="";
  renderChoice();
  renderGroupMembers(w);
}
function renderCompactMembers(w){
  compactMembers.textContent="";
  if(!w||w.mode!=="group"||!Array.isArray(w.repositories))return;
  const group=knownGroups.find(item=>item.name.toLowerCase()===String(w.groupName).toLowerCase());
  for(const member of w.repositories){
    const row=document.createElement("div");row.className="compact-member";
    const name=document.createElement("span");name.className="repo-name";
    const catalogMember=group&&(group.repositories||[]).find(item=>item.alias===member.alias);
    name.textContent=catalogMember?catalogMember.project:member.alias;
    const state=document.createElement("span");state.className="repo-state";
    const checkout=member.checkout==="managed"?"Managed":"Source";
    state.textContent=checkout+" · "+(member.branch||member.sourceBranch||"unknown branch");
    row.append(name,state);compactMembers.append(row);
  }
}
function checkoutSelect(id,kind){
  const select=document.createElement("select");select.id=id;
  for(const value of ["source","managed"]){
    const option=document.createElement("option");option.value=value;
    option.textContent=value==="source"?"Source checkout":"Managed worktree";
    select.append(option);
  }
  select.value=kind==="managed"?"managed":"source";
  return select;
}
function renderGroupMembers(w){
  membersEl.textContent="";
  renderCompactMembers(w);
  if(!w||w.mode!=="group"||!Array.isArray(w.repositories))return;
  const heading=document.createElement("div");heading.className="group-heading";heading.textContent="Group members";
  membersEl.append(heading);
  for(const member of w.repositories){
    const div=document.createElement("div");div.className="group-member";
    const label=document.createElement("strong");label.textContent=member.alias+(member.alias===w.primaryRepository?" (primary)":"");
    const state=document.createElement("div");state.className="notice";
    state.textContent="Checkout: "+(member.checkout==="managed"?"Managed worktree":"Source checkout")+(member.alias===w.activeRepository?" · active default":"");
    const source=document.createElement("div");source.className="group-path muted";source.textContent="Source: "+(member.sourceProjectRoot||"");
    const active=document.createElement("div");active.className="group-path muted";active.textContent="Active: "+(member.projectRoot||"");
    div.append(label,state,source,active);
    if(member.preservedManagedProjectRoot){
      const preserved=document.createElement("div");preserved.className="group-path muted";preserved.textContent="Preserved managed: "+member.preservedManagedProjectRoot;
      div.append(preserved);
    }
    const controls=document.createElement("div");controls.className="group-controls";
    const activeBtn=document.createElement("button");activeBtn.id="member-active-"+member.alias;activeBtn.textContent=member.alias===w.activeRepository?"Default":"Make default";
    activeBtn.disabled=busy||schemaStale||member.alias===w.activeRepository;
    activeBtn.onclick=()=>run(async()=>{
      if(schemaStale)throw new Error("Refresh the connector schema before changing repositories.");
      await call("set_active_repository",{repository:member.alias});await load();
    });
    const kindLabel=document.createElement("label");kindLabel.textContent="Checkout type";kindLabel.htmlFor="member-mode-"+member.alias;
    const select=checkoutSelect(kindLabel.htmlFor,member.checkout);
    const typeBtn=document.createElement("button");typeBtn.id="member-switch-"+member.alias;typeBtn.textContent="Switch type";
    typeBtn.disabled=busy||schemaStale;
    typeBtn.onclick=()=>run(async()=>{
      if(schemaStale)throw new Error("Refresh the connector schema before changing checkout types.");
      if(select.value===member.checkout)return;
      try{await call("set_repository_checkout",{repository:member.alias,checkout:select.value})}finally{await load()}
    });
    controls.append(activeBtn,kindLabel,select,typeBtn);div.append(controls);membersEl.append(div);
  }
}
function renderGroups(list){
  if(Array.isArray(list))knownGroups=list;
  groupsEl.textContent="";
  groupSettings.textContent="";
  renderChoice();
  if(!groupsEnabled||currentWorkspace)return;
  for(const group of knownGroups){
    const card=document.createElement("div");card.className="group-option";
    const heading=document.createElement("div");heading.className="group-option-head";
    const title=document.createElement("div");
    const name=document.createElement("strong");name.textContent=group.name;
    const summary=document.createElement("div");summary.className="workspace-subtitle";
    summary.textContent=(group.repositories||[]).length+" repositories · Primary: "+group.primary;
    title.append(name,summary);heading.append(title);
    const checkouts={};
    const advanced=document.createElement("div");advanced.className="group-member";
    const advancedTitle=document.createElement("strong");advancedTitle.textContent=group.name+" checkout overrides";
    advanced.append(advancedTitle);
    const names=document.createElement("div");names.className="group-option-members";
    names.textContent=(group.repositories||[]).map(member=>member.project).join(" · ");
    for(const member of group.repositories||[]){
      const controls=document.createElement("div");controls.className="group-controls";
      const label=document.createElement("label");label.textContent=member.alias+" · "+member.project;
      label.htmlFor="new-group-"+group.name+"-"+member.alias;
      const select=checkoutSelect(label.htmlFor,member.checkout);
      select.onchange=()=>{checkouts[member.alias]=select.value};
      checkouts[member.alias]=select.value;
      controls.append(label,select);advanced.append(controls);
    }
    const note=document.createElement("div");note.className="warning muted";
    note.textContent="Managed selection here uses the current local HEAD. For a new feature task, select Source and let the model prepare a worktree from a fetched remote branch.";
    advanced.append(note);
    groupSettings.append(advanced);
    const choose=document.createElement("button");choose.id="group-select-"+group.name;choose.textContent="Select group";
    choose.disabled=busy||schemaStale;
    choose.onclick=()=>run(async()=>{
      if(schemaStale)throw new Error("Refresh the connector schema before selecting a project group.");
      if(currentWorkspace)throw new Error("Change workspace / group before selecting another binding.");
      try{await call("set_project_group",{name:group.name,checkouts})}finally{await load()}
      restorePath="";
    });
    heading.append(choose);card.append(heading,names);groupsEl.append(card);
  }
}
function renderProjects(list){
  if(list)knownProjects=list.projects||[];
  projectsEl.textContent="";
  for(const p of knownProjects){
    const div=document.createElement("div");div.className="project";
    const label=document.createElement("span");
    label.innerHTML="<b>"+escapeHTML(p.name||p.selector)+"</b><br><span class=muted>"+escapeHTML(p.selector)+(p.gitRepository===false?" (not a Git repository)":"")+"</span>";
    const b=document.createElement("button");b.textContent="Select";
    b.disabled=busy||schemaStale||!!currentWorkspace||(selectionMode()==="managed"&&p.gitRepository===false);
    if(selectionMode()==="managed"&&p.gitRepository===false)b.title="Managed worktree requires Git";
    else if(currentWorkspace)b.title="Choose Change workspace / type first";
    b.onclick=()=>selectProject(p);
    div.append(label,b);projectsEl.append(div);
  }
}
function renderPayload(value){
  const p=structured(value)||value;
  if(!p||typeof p!=="object")return;
  if(Object.hasOwn(p,"projectGroupsEnabled"))groupsEnabled=!!p.projectGroupsEnabled;
  if(Array.isArray(p.groups))knownGroups=p.groups;
  if(Object.hasOwn(p,"conversationStale")||Object.hasOwn(p,"connectorSchema")){
    schemaStale=!!(p.conversationStale||(p.connectorSchema&&p.connectorSchema.refreshRecommended));
    schemaEl.textContent=schemaStale?"The connector schema is stale. Refresh/reconnect the Codexify Go connector before changing workspace or group bindings.":"";
  }
  if(Object.hasOwn(p,"projectRoot"))renderStatus({workspace:p});
  else if(Object.hasOwn(p,"workspace")||Object.hasOwn(p,"awaitingSelection")||Object.hasOwn(p,"selected"))renderStatus(p);
  if(Array.isArray(p.groups))renderGroups(p.groups);
  if(p.projects&&!currentWorkspace)renderProjects(Array.isArray(p.projects)?{projects:p.projects}:p.projects);
}
// Host globals are state notifications, never an instruction to call tools.
function renderHost(event){
  const globals=event&&event.detail&&event.detail.globals;
  if(globals&&Object.hasOwn(globals,"toolResponseMetadata")){
    observedHostContext=setupContext(window.openai&&window.openai.toolResponseMetadata);
    applyHostContext(setupContext(globals.toolResponseMetadata));
  }else syncHostContext();
  if(globals&&!Object.hasOwn(globals,"toolOutput"))return;
  renderPayload(globals?globals.toolOutput:window.openai&&window.openai.toolOutput);
}
async function run(action){
  if(busy)return;
  const focusId=document.activeElement&&document.activeElement.id||"";
  busy=true;errorEl.textContent="";
  for(const button of document.querySelectorAll("button"))button.disabled=true;
  for(const select of document.querySelectorAll("select"))select.disabled=true;
  try{await action()}catch(e){errorEl.textContent=errorText(e)}finally{
    busy=false;
    for(const select of document.querySelectorAll("select"))select.disabled=false;
    for(const button of document.querySelectorAll("button"))button.disabled=false;
    renderProjects();renderGroups(knownGroups);renderGroupMembers(currentWorkspace);renderChoice();
    for(const id of ["scratch","switch","restore"])document.getElementById(id).disabled=schemaStale;
    if(focusId){
      const element=document.getElementById(focusId);
      if(element&&typeof element.focus==="function"&&!element.disabled)element.focus();
      else if(currentWorkspace&&typeof statusEl.focus==="function"){statusEl.tabIndex=-1;statusEl.focus()}
    }
  }
}
async function load(){
  renderPayload(structured(await call("setup_status",{}))||{});
  if(currentWorkspace){renderProjects({projects:[]});return;}
  renderProjects(structured(await call("list_projects",{limit:40}))||{});
}
function refresh(){return run(load)}
function selectProject(project){
  return run(async()=>{
    if(schemaStale)throw new Error("Refresh/reconnect the connector before changing workspace bindings.");
    if(currentWorkspace)throw new Error("Already bound. Click Change workspace / type before selecting another checkout.");
    const mode=selectionMode();
    if(mode==="managed"&&project.gitRepository===false)throw new Error("Managed worktree requires a Git repository.");
    const args={path:project.selector};
    if(mode==="source")args.createWorktree=false;
    if(mode==="managed")args.createWorktree=true;
    const selected=structured(await call("set_project_root",args))||{};
    await load();
    if(mode==="source"&&selected.managedWorktree!==false)throw new Error("Source checkout was requested but the connector did not confirm it. Check the active path.");
    if(mode==="managed"&&selected.managedWorktree!==true)throw new Error("Managed worktree was requested but the connector did not confirm it. Check the active path.");
    restorePath="";
    restoreBtn.hidden=true;
  });
}
modeSelect.onchange=()=>{renderModeWarning();renderProjects()};
workspaceKind.onchange=()=>{workspaceKindInitialized=true;renderChoice();renderProjects();renderGroups(knownGroups)};
document.getElementById("refresh").onclick=refresh;
document.getElementById("scratch").onclick=()=>run(async()=>{if(schemaStale)throw new Error("Refresh/reconnect the connector before changing workspaces.");await call("set_project_root",{withoutProject:true});restorePath="";await load()});
switchBtn.onclick=()=>run(async()=>{
  if(schemaStale)throw new Error("Refresh/reconnect the connector before changing workspaces.");
  const oldRoot=switchBtn.dataset.path||"";
  if(!oldRoot)throw new Error("No selected workspace to switch.");
  const wasGroup=!!currentWorkspace&&currentWorkspace.mode==="group";
  const change=structured(await call("setup_ui_switch_project",{expectedPath:oldRoot}))||{};
  if(wasGroup)restorePath="";
  else if(change.awaitingSelection&&change.previousRoot)restorePath=change.previousRoot;
  await load();
});
restoreBtn.onclick=()=>run(async()=>{
  if(schemaStale)throw new Error("Refresh/reconnect the connector before restoring workspaces.");
  if(!restorePath)throw new Error("The previous workspace path is unavailable. Use a saved resumePath.");
  const result=structured(await call("set_project_root",{resumePath:restorePath}))||{};
  await load();
  if(!result.projectRoot)throw new Error("Restore did not confirm the previous workspace.");
  restorePath="";
  restoreBtn.hidden=true;
});
function escapeHTML(v){return String(v||"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]))}
window.addEventListener("openai:set_globals",renderHost);
renderModeWarning();renderHost();
</script>
</body>
</html>`

const DiffHTML = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{font-family:system-ui,sans-serif;color-scheme:light dark}body{margin:0;padding:10px}
pre{margin:0;padding:12px;border-radius:10px;background:color-mix(in srgb,currentColor 6%,transparent);overflow:auto;white-space:pre;font:12px/1.5 ui-monospace,monospace}
</style>
</head>
<body><pre id="diff">No diff output.</pre>
<script>
function render(){
 const o=(window.openai&&window.openai.toolOutput)||{};
 const v=o.output||(o.structuredContent&&o.structuredContent.output)||"No diff output.";
 document.getElementById("diff").textContent=String(v);
}
window.addEventListener("openai:set_globals",render);render();
</script></body>
</html>`

const ChatHTML = `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>:root{font-family:system-ui,sans-serif;color-scheme:light dark}body{margin:0;padding:10px}.card{border:1px solid color-mix(in srgb,currentColor 18%,transparent);border-radius:12px;padding:10px}.row{display:flex;gap:8px}textarea{box-sizing:border-box;width:100%;min-height:76px;margin-top:8px;font:inherit}button{font:inherit;padding:7px 10px}.messages{white-space:pre-wrap;max-height:260px;overflow:auto}.muted{opacity:.65;font-size:.9em}#error{color:#c44}</style></head>
<body><div class="card"><div class="row"><strong>Codexify chat</strong><button id="refresh">Refresh</button></div><div id="messages" class="messages muted">No unread user text.</div><textarea id="message" placeholder="Message to append to CHAT.md"></textarea><div class="row"><button id="send">Send</button><span id="status" class="muted"></span></div><div id="error"></div></div>
<script>
const messages=document.getElementById("messages"),message=document.getElementById("message"),status=document.getElementById("status"),error=document.getElementById("error");
function structured(r){return r&&((r.structuredContent)||(r.structured_content)||(r.result&&r.result.structuredContent))||{}}
async function call(name,args={}){if(!(window.openai&&window.openai.callTool))throw new Error("Tool calls are unavailable in this host");return window.openai.callTool(name,args)}
let busy=false;
function renderPayload(value){const r=(value&&(value.structuredContent||value.structured_content))||value;if(!r||typeof r!=="object")return;if(Object.hasOwn(r,"user_text")||Object.hasOwn(r,"userText"))messages.textContent=r.user_text||r.userText||"No unread user text.";if(Object.hasOwn(r,"state"))status.textContent=r.state||""}
function renderHost(event){const g=event&&event.detail&&event.detail.globals;if(g&&!Object.hasOwn(g,"toolOutput"))return;renderPayload(g?g.toolOutput:window.openai&&window.openai.toolOutput)}
async function run(action){if(busy)return;busy=true;error.textContent="";document.getElementById("refresh").disabled=true;document.getElementById("send").disabled=true;try{await action()}catch(e){error.textContent=String(e)}finally{busy=false;document.getElementById("refresh").disabled=false;document.getElementById("send").disabled=false}}
function refresh(){return run(async()=>renderPayload(structured(await call("chat_read",{}))))}
document.getElementById("refresh").onclick=refresh;document.getElementById("send").onclick=()=>run(async()=>{const value=message.value.trim();if(!value)return;await call("chat_write",{message:value});if(message.value.trim()===value)message.value="";status.textContent="Sent"});
window.addEventListener("openai:set_globals",renderHost);renderHost();refresh();
</script></body></html>`

const UpdateHTML = `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>:root{font-family:system-ui,sans-serif;color-scheme:light dark}body{margin:0;padding:10px}.card{border:1px solid color-mix(in srgb,currentColor 18%,transparent);border-radius:12px;padding:12px}.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}button{font:inherit;padding:7px 10px}.muted{opacity:.65}code{font-family:ui-monospace,monospace}#error{color:#c44}</style></head>
<body><div class="card"><div class="row"><strong>Codexify Go update</strong><button id="check">Check again</button></div><p id="status" class="muted">Checking…</p><p id="detail"></p><p class="muted">Installation is intentionally explicit. When an update is available, run <code>codexify-go update apply --config &lt;config&gt;</code> on the host.</p><div id="error"></div></div>
<script>
const status=document.getElementById("status"),detail=document.getElementById("detail"),error=document.getElementById("error");function structured(r){return r&&((r.structuredContent)||(r.structured_content)||(r.result&&r.result.structuredContent))||{}}async function call(name,args={}){if(!(window.openai&&window.openai.callTool))throw new Error("Tool calls are unavailable in this host");return window.openai.callTool(name,args)}
let busy=false;
function renderPayload(value){const p=(value&&(value.structuredContent||value.structured_content))||value;if(!p||typeof p!=="object"||!Object.hasOwn(p,"currentVersion"))return;status.textContent=(p.status||"unknown")+" — current "+(p.currentVersion||"?")+(p.latestVersion?", latest "+p.latestVersion:"");detail.textContent=p.detail||""}
function renderHost(event){const g=event&&event.detail&&event.detail.globals;if(g&&!Object.hasOwn(g,"toolOutput"))return;renderPayload(g?g.toolOutput:window.openai&&window.openai.toolOutput)}
async function check(force=false){if(busy)return;busy=true;document.getElementById("check").disabled=true;error.textContent="";try{renderPayload(structured(await call("self_update_status",{force})))}catch(e){error.textContent=String(e)}finally{busy=false;document.getElementById("check").disabled=false}}
document.getElementById("check").onclick=()=>check(true);window.addEventListener("openai:set_globals",renderHost);renderHost();check(false);
</script></body></html>`
