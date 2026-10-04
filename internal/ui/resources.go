package ui

import "github.com/modelcontextprotocol/go-sdk/mcp"

const (
	SetupURI  = "ui://codexify-go/setup/v4/mcp-app.html"
	DiffURI   = "ui://codexify-go/diff/v1/mcp-app.html"
	ChatURI   = "ui://codexify-go/markdown-chat/v2/mcp-app.html"
	UpdateURI = "ui://codexify-go/self-update/v2/mcp-app.html"
	MIMEType  = "text/html;profile=mcp-app"
)

func SetupToolMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"resourceUri": SetupURI,
			"visibility":  []string{"model", "app"},
		},
		"ui/resourceUri":          SetupURI,
		"openai/outputTemplate":   SetupURI,
		"openai/widgetAccessible": true,
	}
}

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
:root{font-family:system-ui,sans-serif;color-scheme:light dark}body{margin:0;padding:12px}
.card{border:1px solid color-mix(in srgb,currentColor 18%,transparent);border-radius:12px;padding:12px}
.row{display:flex;gap:8px;flex-wrap:wrap;align-items:center}button{font:inherit;padding:7px 10px;border-radius:8px;border:1px solid color-mix(in srgb,currentColor 25%,transparent);background:transparent;cursor:pointer}
.project{display:flex;justify-content:space-between;gap:8px;padding:7px 0;border-top:1px solid color-mix(in srgb,currentColor 12%,transparent)}
.muted{opacity:.7;font-size:.9em}code{font-family:ui-monospace,monospace;font-size:.9em;overflow-wrap:anywhere}#error{color:#c44;white-space:pre-wrap}
</style>
</head>
<body>
<div class="card">
  <div class="row"><strong>Codexify Go</strong><button id="refresh">Refresh</button><button id="scratch">Scratch</button><button id="switch" hidden>Switch project</button></div>
  <div id="status" class="muted">No live status loaded. Press Refresh to fetch it.</div>
  <div id="projects"></div>
  <div id="error"></div>
</div>
<script>
const statusEl=document.getElementById("status"),projectsEl=document.getElementById("projects"),errorEl=document.getElementById("error"),switchBtn=document.getElementById("switch");
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
  if(revision!==hostContextRevision)throw new Error("The workspace changed while this request was running. Refresh to continue.");
  const nextContext=setupContext(result);if(nextContext)uiContext=nextContext;
  if(result&&result.isError===true){
    const message=typeof result.content==="string"?result.content:Array.isArray(result.content)?result.content.filter(c=>c&&c.type==="text"&&typeof c.text==="string").map(c=>c.text).join("\n"):"";
    throw new Error(message.trim()||"The request failed.");
  }
  return result;
}
let busy=false;
function renderStatus(s){
    const w=s.workspace||null;
    statusEl.innerHTML=w?("Selected: <code>"+escapeHTML(w.projectRoot||w.workspaceRoot||"")+"</code>"+(w.managedWorktree?" (worktree)":"")):(s.awaitingSelection?"Choose a workspace.":"No workspace selected.");
    switchBtn.hidden=!w;
    switchBtn.dataset.path=w&&w.projectRoot||"";
}
function renderProjects(list){
    projectsEl.textContent="";
    for(const p of list.projects||[]){
      const div=document.createElement("div");div.className="project";
      const label=document.createElement("span");label.innerHTML="<b>"+escapeHTML(p.name||p.selector)+"</b><br><span class=muted>"+escapeHTML(p.selector)+"</span>";
      const b=document.createElement("button");b.textContent="Select";b.disabled=busy;b.onclick=()=>selectProject(p.selector);
      div.append(label,b);projectsEl.append(div);
    }
}
function renderPayload(value){
  const p=structured(value)||value;
  if(!p||typeof p!=="object")return;
  if(Object.hasOwn(p,"projectRoot"))renderStatus({workspace:p});
  else if(Object.hasOwn(p,"workspace")||Object.hasOwn(p,"awaitingSelection")||Object.hasOwn(p,"selected"))renderStatus(p);
  if(Array.isArray(p.projects))renderProjects(p);
}
// Host globals are state notifications, never an instruction to call tools.
function renderHost(event){
  const globals=event&&event.detail&&event.detail.globals;
  if(globals&&Object.hasOwn(globals,"toolResponseMetadata")){observedHostContext=setupContext(window.openai&&window.openai.toolResponseMetadata);applyHostContext(setupContext(globals.toolResponseMetadata));}
  else syncHostContext();
  if(globals&&!Object.hasOwn(globals,"toolOutput"))return;
  renderPayload(globals?globals.toolOutput:window.openai&&window.openai.toolOutput);
}
async function run(action){
  if(busy)return;
  busy=true;errorEl.textContent="";
  for(const button of document.querySelectorAll("button"))button.disabled=true;
  try{await action()}catch(e){errorEl.textContent=String(e)}finally{
    busy=false;for(const button of document.querySelectorAll("button"))button.disabled=false;
  }
}
async function load(){renderStatus(structured(await call("setup_status",{}))||{});renderProjects(structured(await call("list_projects",{limit:40}))||{})}
function refresh(){return run(load)}
function selectProject(path){return run(async()=>{await call("set_project_root",{path});await load()})}
document.getElementById("refresh").onclick=refresh;
document.getElementById("scratch").onclick=()=>run(async()=>{await call("set_project_root",{withoutProject:true});await load()});
switchBtn.onclick=()=>run(async()=>{await call("setup_ui_switch_project",{expectedPath:switchBtn.dataset.path||""});await load()});
function escapeHTML(v){return String(v||"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]))}
window.addEventListener("openai:set_globals",renderHost);
renderHost();
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
