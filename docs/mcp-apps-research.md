# MCP Apps Integration Research — Issue #753

This document captures all research findings, architectural decisions, and implementation
guidance for adding MCP Apps support to kubernetes-mcp-server.

**Issue**: [containers/kubernetes-mcp-server#753](https://github.com/containers/kubernetes-mcp-server/issues/753)
**Branch**: `feat/mcp-apps`
**Date**: 2026-03-03

## Table of Contents

- [1. What Are MCP Apps](#1-what-are-mcp-apps)
- [2. Go-SDK Support](#2-go-sdk-support)
- [3. PoC Reference (openshift-mcp-server PR #143)](#3-poc-reference-openshift-mcp-server-pr-143)
- [4. Styling and CSS Isolation](#4-styling-and-css-isolation)
- [5. Multiple Widgets and Tool Call Accumulation](#5-multiple-widgets-and-tool-call-accumulation)
- [6. Frontend Stack Decision](#6-frontend-stack-decision)
- [7. MCP postMessage Protocol](#7-mcp-postmessage-protocol)
- [8. Configuration: Opt-In Feature](#8-configuration-opt-in-feature)
- [9. Codebase Readiness](#9-codebase-readiness)
- [10. Architecture and Implementation Plan](#10-architecture-and-implementation-plan)

---

## 1. What Are MCP Apps

MCP Apps is the first official extension to the Model Context Protocol (spec version 2026-01-26).
It enables MCP tools to return interactive HTML-based UIs rendered in sandboxed iframes by the
MCP host (VS Code, Claude Desktop, ChatGPT, etc.).

The flow:

1. **Tool definition** declares `_meta.ui.resourceUri` pointing to a `ui://` resource
2. **Model calls the tool** — host executes it on the server
3. **Host fetches the UI resource** — gets HTML via MCP `resources/read`
4. **Host renders in sandboxed iframe** — passes tool data via `postMessage`
5. **Bidirectional communication** — app can call server tools via JSON-RPC over `postMessage`

Extension identifier: `io.modelcontextprotocol/ui`

Supported hosts: VS Code (Insiders and Stable), Claude (web/desktop), ChatGPT, Goose, Postman, MCPJam.

Key specification sources:
- [MCP Apps Specification (2026-01-26)](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)
- [MCP Apps Documentation](https://modelcontextprotocol.io/docs/extensions/apps)
- [ext-apps SDK Repository](https://github.com/modelcontextprotocol/ext-apps)

## 2. Go-SDK Support

### PR #794 (merged 2026-02-17, included in v1.4.0)

[PR](https://github.com/modelcontextprotocol/go-sdk/pull/794) added `Extensions` field
to both `ServerCapabilities` and `ClientCapabilities` per SEP-2133.

New API surface available in our go-sdk v1.4.0 dependency:

```go
// On ServerCapabilities
Extensions map[string]any `json:"extensions,omitempty"`
func (c *ServerCapabilities) AddExtension(name string, settings map[string]any)

// On ClientCapabilities
Extensions map[string]any `json:"extensions,omitempty"`
func (c *ClientCapabilities) AddExtension(name string, settings map[string]any)
```

Both `AddExtension` methods normalize `nil` settings to `map[string]any{}` (spec requires
an object, not null).

### Usage for MCP Apps capability negotiation

**Server declares support** (in `initialize` response):
```json
{
  "capabilities": {
    "extensions": {
      "io.modelcontextprotocol/ui": {}
    }
  }
}
```

**Client declares support** (in `initialize` request):
```json
{
  "capabilities": {
    "extensions": {
      "io.modelcontextprotocol/ui": {
        "mimeTypes": ["text/html;profile=mcp-app"]
      }
    }
  }
}
```

In Go (to add in `pkg/mcp/mcp.go`):
```go
caps := &mcp.ServerCapabilities{...}
caps.AddExtension("io.modelcontextprotocol/ui", nil)
```

## 3. PoC Reference (openshift-mcp-server PR #143)

[PR #143](https://github.com/openshift/openshift-mcp-server/pull/143) is a rough PoC
implementing a Pods Top dashboard with MCP Apps.

### What it does

- Adds a `mcp-app/` directory with a Vite + TypeScript frontend build
- Uses `vite-plugin-singlefile` to produce a single HTML with all JS/CSS inlined
- Bundles the HTML into Go binary via `//go:embed`
- Uses `@modelcontextprotocol/ext-apps` SDK (v1.0.0) for host communication
- Implements column sorting, refresh via `app.callServerTool()`, dark/light theme support

### Architecture (3 layers)

1. **Go embed package** (`mcp-app/mcpapp.go`):
   - `//go:embed dist/pods-top-app.html`
   - `Resources()` returns a registry of all app resources
   - `ToolMeta(resourceURI)` helper generates `_meta` map
   - MIME type: `text/html;profile=mcp-app`
   - Resource URI: `ui://kubernetes-mcp-server/pods-top.html`

2. **MCP resource registration** (`pkg/mcp/mcp.go`):
   - Iterates `mcpapp.Resources()` and registers each as an MCP resource
   - Enables `Resources` capability (was `nil` before)

3. **Tool linkage** (`pkg/toolsets/core/pods.go`):
   - `pods_top` tool gets `Meta: mcpapp.ToolMeta(mcpapp.PodsTopResourceURI)`
   - Tool handler returns both text and `structuredContent`

### Concerns noted

- **Build step required**: `npm install && vite build` as dependency of `make build`
- **Committed build artifacts**: `dist/pods-top-app.html` in git (should be .gitignored)
- **innerHTML without sanitization**: XSS risk if pod names contain HTML-special characters
- **SDK size**: `@modelcontextprotocol/ext-apps` is 314 KB (mostly Zod validation)
- **Tight Go/TypeScript coupling**: Matching struct/interface with no shared schema

### Patterns worth reusing

- `//go:embed` for bundling HTML into Go binary
- `Resources()` registry pattern for app resources
- `ToolMeta()` helper for consistent `_meta` structure
- Dual content return (text + structured) for backward compatibility
- Theme support via `onhostcontextchanged`

## 4. Styling and CSS Isolation

### Spec guarantees

**CSS isolation is not a concern.** The MCP Apps specification mandates that all app views
run inside **sandboxed iframes**. This provides complete CSS isolation by design:

- Views have no access to the host's DOM, cookies, or storage
- All communication passes through `postMessage`
- App CSS cannot leak into the host; host CSS cannot reach the app
- Each app runs in its own isolated context

This means Shadow DOM, `@scope`, `all: initial`, and other CSS isolation techniques are
**unnecessary** for MCP Apps.

### Theming

Hosts inject standardized CSS custom properties into the iframe for visual integration:

- `--mcp-background`, `--mcp-foreground`, `--mcp-primary-background`, etc.
- Extended set: `--color-background-*`, `--color-text-*`, `--color-border-*`
- Font variables: `--font-sans`, `--font-mono`, `--font-text-*-size`

**Apps SHOULD use these with fallbacks:**
```css
.container {
  background: var(--color-background-primary, #ffffff);
  color: var(--color-text-primary, #1a1a1a);
}
```

**Theme changes** are communicated via `ui/notifications/host-context-changed` notification
with a `theme` field (`"dark"` or `"light"`).

The `prefersBorder` metadata allows apps to request border/background treatment from the host.

### Known issues

- Different hosts provide inconsistent CSS variable subsets
  ([ext-apps #382](https://github.com/modelcontextprotocol/ext-apps/issues/382))
- Platform-specific styling pressure — Claude, ChatGPT, VS Code each have their own guidelines
  ([ext-apps #467](https://github.com/modelcontextprotocol/ext-apps/issues/467))
- `autoResize: true` causes layout bugs including infinite growth loops
  ([ext-apps #502](https://github.com/modelcontextprotocol/ext-apps/issues/502))

### Recommendation

- Always provide CSS variable fallback values
- Treat all host-provided variables as optional
- Use `autoResize: false` + explicit height + manual `sendSizeChanged()` for predictable sizing
- Set `prefersBorder` explicitly

### Default Content Security Policy

When `ui.csp` is omitted, the default CSP is:
```
default-src 'none';
script-src 'self' 'unsafe-inline';
style-src 'self' 'unsafe-inline';
img-src 'self' data:;
media-src 'self' data:;
connect-src 'none';
```

This means:
- Inline `<script>` and `<style>` tags work (`'unsafe-inline'`)
- No external network requests (`connect-src 'none'`)
- No CDN imports possible without declaring them in `_meta.ui.csp`

## 5. Multiple Widgets and Tool Call Accumulation

### Spec behavior

**Each tool call with `_meta.ui.resourceUri` creates a separate iframe** (1:1 mapping).
The spec does NOT address widget accumulation or consolidation.

Evidence from the specification:
- The lifecycle diagram shows "Host renders View in an iframe" for each tool call
- `McpUiHostContext.toolInfo` is singular (not an array) — each iframe knows its one tool
- `ontoolresult` is called exactly once per iframe lifecycle
- The `AppBridge` (host-side SDK) manages one view per instance

If an agent calls 5 tools with UI in one turn, the host creates 5 separate iframes.

### Consolidation mechanism: `callServerTool()` + `visibility: ["app"]`

The spec provides a pull-based pattern for consolidation:

- **`visibility: ["app"]`** — hides a tool from the model, making it callable only from within
  an app iframe
- **`app.callServerTool()`** — allows an app to call any server tool, including app-only tools

**Pattern from the system-monitor example:**
```
1. Model calls get-system-info (model-visible, has _meta.ui) → creates widget
2. Widget calls poll-system-stats (app-only) via callServerTool() → gets live data
3. Widget renders a consolidated dashboard
```

### Impact assessment for "app per tool"

With every tool having `_meta.ui.resourceUri`, typical agent behavior:
- 1-3 tool calls per turn → 1-3 inline widgets (acceptable UX)
- 5-10 tool calls per turn → 5-10 iframes (potentially noisy but each replaces text output)

**Mitigation strategies:**
- Keep widgets compact (controlled height via `sendSizeChanged()`)
- Use `IntersectionObserver` to pause off-screen widgets (performance)
- A widget can call `callServerTool()` to show related data (reducing need for separate calls)

### Display modes

Apps can request different display modes:
- **`inline`** — embedded in chat flow (hosts MUST support this)
- **`fullscreen`** — takes over window
- **`panel`** — side panel
- **`pip`** — picture-in-picture overlay

## 6. Frontend Stack Decision

### Decision: Preact + HTM + Signals-core (no build step)

| Evaluated | Size (gzipped) | Verdict |
|-----------|---------------|---------|
| Alpine.js | ~7-8 KB | Good for enhancing HTML, poor fit for data-driven widgets |
| Preact + HTM | ~4.5 KB | Component model, virtual DOM, React-like API |
| Preact + HTM + Signals | ~6 KB | Adds fine-grained reactivity |
| `@modelcontextprotocol/ext-apps` SDK | ~74 KB | Overkill — 314 KB for ~100 lines of protocol |
| HTMX | ~14 KB | Expects HTTP, not postMessage — architectural mismatch |
| Vanilla JS | 0 KB | Always viable but more boilerplate |

**Why Preact over Alpine.js:**
- Component model matches "data in, UI out" widget pattern
- Signals provide fine-grained DOM updates for async data (postMessage events)
- Better composability for complex UIs (tables, dashboards)
- Smaller total footprint
- React ecosystem knowledge transfers directly

**Why not HTMX:**
HTMX expects HTTP requests (`hx-get="/endpoint"`) and server-returned HTML fragments.
MCP Apps communicate via postMessage/JSON-RPC through the host. Fundamental architecture mismatch.

**Why implement MCP protocol directly (not SDK):**
- SDK is 314 KB (74 KB gzipped), mostly Zod schema validation
- The actual protocol is ~100 lines of vanilla JS
- Spec explicitly states the SDK is optional
- Spec includes a working inline implementation example
- No schema validation needed on the app side (host validates)

### Single-file constraint

MCP `resources/read` returns HTML as a text string via JSON-RPC. The host renders it
in an iframe (via `srcdoc` or blob URL). Therefore:

- **Import maps with relative paths won't work** (no origin to resolve against)
- **All JS must be inline** in `<script>` tags
- **CDN imports blocked** by default CSP (`connect-src 'none'`)

### Libraries to inline

| Library | Raw Size | Purpose |
|---------|----------|---------|
| `htm@3.1.1/preact/standalone.umd.js` | ~13 KB | Preact + HTM + Hooks in one UMD bundle |
| `@preact/signals-core@1.8.0` | ~4 KB | `signal()`, `computed()`, `effect()` |
| MCP protocol (custom implementation) | ~2 KB | JSON-RPC over postMessage |
| **Total** | **~19 KB** | |

**Tradeoff**: Using `htm/preact/standalone` + `@preact/signals-core` means we get the core
signal primitives (`signal()`, `computed()`, `effect()`, `batch()`) but NOT the Preact
integration hooks (`useSignal()`, `useComputed()`). Those require separate `preact` +
`@preact/signals` modules that can't be used with the standalone bundle. The core primitives
are sufficient — signal changes can be wired to Preact rerenders with a small helper.

### Vendoring strategy

- **Makefile target** (`make vendor-js`) downloads minified files from jsdelivr CDN
- **Files committed to repo** — always available for airgapped builds
- **Makefile used for version bumps** — re-run when updating library versions

Example Makefile targets:

```makefile
PREACT_HTM_VERSION ?= 3.1.1
SIGNALS_CORE_VERSION ?= 1.8.0
VENDOR_DIR ?= pkg/mcpapps/vendor

.PHONY: vendor-js
vendor-js:
	@mkdir -p $(VENDOR_DIR)
	curl -sL "https://cdn.jsdelivr.net/npm/htm@$(PREACT_HTM_VERSION)/preact/standalone.umd.js" \
		-o $(VENDOR_DIR)/htm-preact-standalone.umd.js
	curl -sL "https://cdn.jsdelivr.net/npm/@preact/signals-core@$(SIGNALS_CORE_VERSION)/dist/signals-core.module.js" \
		-o $(VENDOR_DIR)/signals-core.module.js
```

## 7. MCP postMessage Protocol

### Protocol overview

JSON-RPC 2.0 over `window.parent.postMessage()`. The spec includes a working inline
implementation (~30 lines).

### Essential messages

**Initialization handshake (required):**
1. App → Host: `ui/initialize` request (with appInfo, capabilities, protocolVersion)
2. Host → App: response with hostInfo, hostCapabilities, hostContext (includes toolInfo)
3. App → Host: `ui/notifications/initialized` notification

**Data flow (receive):**
- `ui/notifications/tool-input` — tool call arguments (sent once after init)
- `ui/notifications/tool-result` — tool execution result (sent once)
- `ui/notifications/host-context-changed` — theme changes
- `ui/resource-teardown` — cleanup before iframe destruction

**App-initiated (send):**
- `tools/call` — call a server tool (`callServerTool()`)
- `ui/notifications/size-changed` — resize notification
- `ping` — respond to health checks

### Minimal implementation (~100 lines)

```javascript
// JSON-RPC primitives
let nextId = 1;
const pending = new Map();

function sendRequest(method, params) {
  const id = nextId++;
  window.parent.postMessage({ jsonrpc: "2.0", id, method, params }, '*');
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
  });
}

function sendNotification(method, params) {
  window.parent.postMessage({ jsonrpc: "2.0", method, params }, '*');
}

const handlers = {};
function onNotification(method, handler) {
  handlers[method] = handler;
}

// Message router
window.addEventListener('message', (event) => {
  const msg = event.data;
  if (!msg || msg.jsonrpc !== '2.0') return;
  // Response to our request
  if (msg.id && pending.has(msg.id)) {
    const { resolve, reject } = pending.get(msg.id);
    pending.delete(msg.id);
    if (msg.error) reject(new Error(msg.error.message));
    else resolve(msg.result);
    return;
  }
  // Incoming notification
  if (msg.method && handlers[msg.method]) {
    handlers[msg.method](msg.params);
  }
  // Incoming request (ping, teardown)
  if (msg.method && msg.id) {
    if (msg.method === 'ping') {
      window.parent.postMessage({ jsonrpc: "2.0", id: msg.id, result: {} }, '*');
    }
  }
});

// Initialization
async function initialize() {
  const result = await sendRequest('ui/initialize', {
    appInfo: { name: 'kubernetes-mcp-viewer', version: '1.0.0' },
    protocolVersion: '2026-01-26',
    appCapabilities: {},
  });
  sendNotification('ui/notifications/initialized', {});
  return result; // Contains hostContext with toolInfo, theme, etc.
}

// Tool call
function callServerTool(name, args) {
  return sendRequest('tools/call', { name, arguments: args || {} });
}
```

### Data available after initialization

```javascript
const { hostContext } = await initialize();
// hostContext.toolInfo.tool.name → which tool created this iframe
// hostContext.toolInfo.tool.inputSchema → tool's parameter schema
// hostContext.toolInfo.id → JSON-RPC request ID of the tools/call
// hostContext.theme → "dark" or "light"
// hostContext.styleVariables → CSS custom properties from host
```

## 8. Configuration: Opt-In Feature

MCP Apps is an **opt-in feature** controlled by configuration, disabled by default.

### Configuration flag

Following the pattern of `validation_enabled` in `StaticConfig` (`pkg/config/config.go`),
a new boolean field gates the entire MCP Apps feature:

```go
// In StaticConfig
AppsEnabled bool // Enable MCP Apps interactive UI extensions
```

TOML configuration:
```toml
apps_enabled = true
```

CLI flag (in `cmd/kubernetes-mcp-server/`):
```
--apps    Enable MCP Apps interactive UI extensions
```

### What the flag controls

**When `apps_enabled = false` (default):**
- No `io.modelcontextprotocol/ui` extension in server capabilities
- `Resources` capability remains `nil` (unchanged from current behavior)
- No `ui://` resource registered
- Tools have no `_meta.ui` field (Meta stays nil)
- Behavior is identical to current server — zero overhead, zero side effects

**When `apps_enabled = true`:**
- Server declares `io.modelcontextprotocol/ui` extension via `AddExtension()`
- `Resources` capability enabled (`&mcp.ResourceCapabilities{}`)
- `ui://kubernetes-mcp-server/viewer.html` resource registered
- All tools get `_meta.ui.resourceUri` pointing to the viewer
- Tools return `structuredContent` alongside text for UI consumption

### Implementation approach

The flag is checked at two points:

1. **Server initialization** (`pkg/mcp/mcp.go`): Conditionally add the extension and
   Resources capability to `ServerCapabilities`, and register the UI resource:
   ```go
   if configuration.AppsEnabled {
       caps.AddExtension("io.modelcontextprotocol/ui", nil)
       caps.Resources = &mcp.ResourceCapabilities{}
       // Register UI resource after server creation
   }
   ```

2. **Tool registration** (`pkg/mcp/mcp.go`): When building applicable tools, conditionally
   populate `Meta` with the UI resource URI. This can be done in the existing
   `collectApplicableTools()` or `registerTool()` flow rather than in each individual
   toolset file. This avoids touching every toolset definition and keeps the opt-in
   logic centralized:
   ```go
   func (s *Server) registerTool(tool api.ServerTool) {
       if s.configuration.AppsEnabled && tool.Tool.Meta == nil {
           tool.Tool.Meta = mcpapps.ToolMeta(mcpapps.ViewerResourceURI)
       }
       // ... existing registration logic
   }
   ```

   This centralized approach means **no changes to individual toolset files** are needed
   just for the Meta field. The feature is entirely self-contained in the MCP server layer
   and the new `pkg/mcpapps/` package.

### Configuration flow

The existing configuration loading chain supports this naturally:

```
BaseDefault() → config.toml → conf.d/*.toml → CLI flags
                (apps_enabled = true)           (--apps)
```

The flag is also compatible with dynamic reload via `SIGHUP` — when toggled at runtime,
the server can re-register tools with or without `_meta.ui` on the next `reloadToolsets()`.

### Interaction with other config options

- **`read_only = true`**: MCP Apps still works, showing only read-only tool results
- **`disable_destructive = true`**: MCP Apps works for non-destructive tools
- **`enabled_tools` / `disabled_tools`**: Only applicable tools get the UI; filtered
  tools are not affected
- **`stateless = true`**: Compatible — MCP Apps uses Resources (static), not tool
  list change notifications

## 9. Codebase Readiness

### Already implemented

| Feature | Location | Status |
|---------|----------|--------|
| `Tool.Meta map[string]any` | `pkg/api/toolsets.go:168` | Defined, not yet used by any tools |
| Meta → go-sdk conversion | `pkg/mcp/gosdk.go:27` | `Meta: mcp.Meta(tool.Tool.Meta)` |
| `ToolCallResult.StructuredContent` | `pkg/api/toolsets.go:58-61` | Defined and actively used |
| `NewToolCallResultStructured()` | `pkg/api/toolsets.go:86-98` | Helper that JSON-serializes to Content field |
| `NewStructuredResult()` | `pkg/mcp/mcp.go:416-438` | Converts to go-sdk CallToolResult |
| Go-SDK v1.4.0 | `go.mod` | Includes Extensions support from PR #794 |

### Needs implementation

| Feature | Where | What |
|---------|-------|------|
| Config flag | `pkg/config/config.go` | Add `AppsEnabled bool` to `StaticConfig` |
| CLI flag | `cmd/kubernetes-mcp-server/` | Add `--apps` flag |
| Server capabilities (conditional) | `pkg/mcp/mcp.go` | When `AppsEnabled`: add extension, enable Resources |
| UI resource registration | `pkg/mcp/mcp.go` | When `AppsEnabled`: register `ui://` resource via `server.AddResource()` |
| Centralized Meta injection | `pkg/mcp/mcp.go` | When `AppsEnabled`: set `_meta.ui` on tools during `registerTool()` |
| HTML viewer | New package `pkg/mcpapps/` | Single HTML file with inlined JS, embedded via `//go:embed` |
| Structured content from tools | Each tool handler | Return `structuredContent` alongside text (many already do) |
| Makefile vendor target | `Makefile` | `vendor-js` target to download Preact/HTM/Signals-core |

### Key files to modify

| File | Changes |
|------|---------|
| `pkg/config/config.go` | Add `AppsEnabled bool` field to `StaticConfig` |
| `cmd/kubernetes-mcp-server/` | Add `--apps` CLI flag |
| `pkg/mcp/mcp.go` | Conditional extension/Resources/resource registration + centralized Meta injection |
| `pkg/mcpapps/mcpapps.go` | **New file**: `//go:embed`, resource registry, `ToolMeta()`, `ViewerResourceURI` |
| `pkg/mcpapps/viewer.html` | **New file**: single generic viewer (HTML + inlined JS + CSS) |
| `pkg/mcpapps/vendor/` | **New dir**: vendored JS files (committed, updated via `make vendor-js`) |
| `Makefile` | Add `vendor-js` target |
| `docs/configuration.md` | Document `apps_enabled` option |

Note: **Individual toolset files do NOT need changes** for Meta — it's injected centrally
when `AppsEnabled` is true. Toolset files only need changes if/when adding `structuredContent`
to their tool handlers.

## 10. Architecture and Implementation Plan

### High-level architecture

```
┌─────────────────────────────────────────────────────┐
│  MCP Host (Claude, VS Code, ChatGPT, etc.)          │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Sandboxed iframe (one per tool call)          │  │
│  │                                                │  │
│  │  viewer.html (single generic viewer)           │  │
│  │  ├── Inline: htm/preact/standalone (13 KB)     │  │
│  │  ├── Inline: @preact/signals-core  (4 KB)      │  │
│  │  ├── Inline: MCP protocol impl     (2 KB)      │  │
│  │  └── App code: reads toolInfo.tool.name        │  │
│  │      → renders appropriate UI per tool type    │  │
│  │      → can call other tools via callServerTool │  │
│  └────────────────────────────────────────────────┘  │
│                     ↕ postMessage (JSON-RPC 2.0)     │
└─────────────────────────────────────────────────────┘
                      ↕ MCP protocol
┌─────────────────────────────────────────────────────┐
│  kubernetes-mcp-server (Go binary)                   │
│                                                      │
│  ServerCapabilities:                                 │
│    extensions: {"io.modelcontextprotocol/ui": {}}    │
│    resources: {}                                     │
│                                                      │
│  Resource:                                           │
│    uri: ui://kubernetes-mcp-server/viewer.html       │
│    mimeType: text/html;profile=mcp-app               │
│    content: //go:embed viewer.html                   │
│                                                      │
│  Every tool:                                         │
│    _meta.ui.resourceUri → viewer.html                │
│    Handler returns text + structuredContent           │
└─────────────────────────────────────────────────────┘
```

### Generic viewer behavior

The single `viewer.html` file:

1. **Initializes**: Calls `ui/initialize`, receives `hostContext`
2. **Reads tool identity**: `hostContext.toolInfo.tool.name` determines what to render
3. **Receives tool result**: `ontoolresult` callback gets `structuredContent` (preferred)
   or falls back to `content[].text`
4. **Renders per-tool UI**: Switch on tool name to render appropriate component
   (table for list operations, detail view for get operations, etc.)
5. **Supports theme**: Applies `--mcp-*` CSS variables from host context
6. **Can refresh**: Calls `callServerTool()` to re-fetch data
7. **Can fetch related data**: Calls other tools to show contextual information

### Package structure

```
pkg/mcpapps/
├── mcpapps.go          # Go embed, resource registry, ToolMeta() helper
├── viewer.html         # Single generic viewer (HTML + inlined JS + CSS)
└── vendor/
    ├── htm-preact-standalone.umd.js   # ~13 KB, committed
    └── signals-core.module.js         # ~4 KB, committed
```

The `viewer.html` file uses Go's `embed` directive. The vendor JS files are inlined
into the HTML either at build time (via a Go template) or as literal `<script>` tag
content in the HTML file itself.

### Implementation phases

**Phase 1: Infrastructure + Configuration**
- Add `AppsEnabled` to `StaticConfig` and `--apps` CLI flag
- Create `pkg/mcpapps/` package with `//go:embed` and resource registry
- Add `make vendor-js` Makefile target, download and commit vendor JS files
- Conditionally add extension + Resources capability to server (gated by `AppsEnabled`)
- Register the `ui://` resource (gated by `AppsEnabled`)
- Add centralized Meta injection in `registerTool()` (gated by `AppsEnabled`)

**Phase 2: Protocol + Viewer skeleton**
- Implement MCP postMessage protocol (~100 lines JS) in `viewer.html`
- Create `viewer.html` with Preact + theme support
- Implement basic "tool result display" for one tool type (e.g., `pods_list`)
- Ensure that tool returns `structuredContent` alongside text

**Phase 3: All tools**
- Ensure every tool handler returns appropriate `structuredContent`
- Add per-tool rendering logic to the generic viewer
- Group similar tools for shared rendering (e.g., all list operations share a table component)

**Phase 4: Polish**
- Sorting, filtering, pagination for table views
- Refresh capability via `callServerTool()`
- Compact height management via `sendSizeChanged()`
- Error states and loading indicators
- Edge case handling (empty results, large datasets)
- Documentation: update `docs/configuration.md` with `apps_enabled` option
