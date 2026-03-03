(function() {
  'use strict';

  var hp = window.htmPreact;
  var html = hp.html;
  var render = hp.render;
  var useState = hp.useState;
  var useEffect = hp.useEffect;

  var protocol = window.mcpProtocol;
  var components = window.mcpComponents;

  // Tool name injected by the server via ViewerHTMLForTool
  var embeddedToolName = window.__mcpToolName || '';

  function App() {
    var stateArr = useState('loading');
    var state = stateArr[0];
    var setState = stateArr[1];

    var toolNameArr = useState(embeddedToolName);
    var toolName = toolNameArr[0];
    var setToolName = toolNameArr[1];

    var resultArr = useState(null);
    var result = resultArr[0];
    var setResult = resultArr[1];

    var errorArr = useState(null);
    var appError = errorArr[0];
    var setError = errorArr[1];

    useEffect(function() {
      // Register notification handlers BEFORE initializing to avoid any timing window

      // Spec-compliant flow: host sends tool-result after calling the tool
      protocol.onNotification('ui/notifications/tool-result', function(params) {
        console.log('[mcp-app] tool-result received');
        setResult(params);
        setState('result');
      });

      // Fallback flow: host sends tool-input, viewer calls the tool via serverTools
      protocol.onNotification('ui/notifications/tool-input', function(params) {
        console.log('[mcp-app] tool-input received, toolName:', toolName);
        if (toolName) {
          var args = (params && params.arguments) || {};
          console.log('[mcp-app] calling tool via serverTools:', toolName);
          protocol.sendRequest('tools/call', { name: toolName, arguments: args }).then(function(callResult) {
            console.log('[mcp-app] tool call result received');
            setResult(callResult);
            setState('result');
          }).catch(function(err) {
            console.error('[mcp-app] tool call failed:', err);
            setError('Tool call failed: ' + (err.message || err));
            setState('error');
          });
        }
      });

      protocol.onNotification('ui/notifications/host-context-changed', function(params) {
        console.log('[mcp-app] host-context-changed');
      });

      protocol.initialize().then(function(initResult) {
        // Prefer hostContext.toolInfo if available (spec-compliant hosts)
        var name = embeddedToolName;
        if (initResult && initResult.hostContext && initResult.hostContext.toolInfo && initResult.hostContext.toolInfo.tool) {
          name = initResult.hostContext.toolInfo.tool.name;
        }
        console.log('[mcp-app] initialized, toolName:', name);
        setToolName(name);
        setState('ready');

      }).catch(function(err) {
        console.error('[mcp-app] initialize error:', err);
        setError(err.message || 'Failed to initialize');
        setState('error');
      });
    }, []);

    if (state === 'loading') {
      return html`<p class="status">Initializing...</p>`;
    }
    if (state === 'error') {
      return html`<p class="status">Error: ${appError}</p>`;
    }
    if (state === 'ready') {
      return html`<p class="status">Waiting for tool result...</p>`;
    }

    // state === 'result'
    if (!result) {
      return html`<p class="status">No result received</p>`;
    }

    // Extract structured content or fall back to text.
    // The server wraps array data in {"items": [...]} because the MCP spec
    // requires structuredContent to be a JSON object. Unwrap it here.
    var raw = result.structuredContent;
    var structured = (raw && raw.items && Array.isArray(raw.items)) ? raw.items : raw;
    var textContent = '';
    if (result.content && Array.isArray(result.content)) {
      for (var i = 0; i < result.content.length; i++) {
        if (result.content[i] && result.content[i].text) {
          textContent += result.content[i].text;
        }
      }
    }

    // Route to the appropriate view based on tool name and data shape
    if (structured && toolName === 'pods_top') {
      return html`<${components.MetricsTable} data=${structured} />`;
    }

    // Generic table for any structured array data (list operations)
    if (structured && Array.isArray(structured) && structured.length > 0 && typeof structured[0] === 'object') {
      return html`<${components.TableView} data=${structured} />`;
    }

    // Fallback: try to render structured as JSON, or raw text
    if (structured) {
      return html`<${components.GenericView} text=${JSON.stringify(structured, null, 2)} />`;
    }
    return html`<${components.GenericView} text=${textContent} />`;
  }

  // Mount
  render(html`<${App} />`, document.getElementById('app'));
})();
