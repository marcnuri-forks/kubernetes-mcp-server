package mcpapps

import _ "embed"

const ResourceMIMEType = "text/html;profile=mcp-app"
const ViewerResourceURI = "ui://kubernetes-mcp-server/viewer.html"

//go:embed viewer.html
var viewerHTML string

func ViewerHTML() string {
	return viewerHTML
}

func ToolMeta() map[string]any {
	return map[string]any{
		"ui": map[string]any{"resourceUri": ViewerResourceURI},
	}
}
