package mcp

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// ToolFilter is a function that takes a ServerTool and returns a boolean indicating whether to include the tool
type ToolFilter func(tool api.ServerTool) bool

func CompositeFilter(filters ...ToolFilter) ToolFilter {
	return func(tool api.ServerTool) bool {
		for _, f := range filters {
			if !f(tool) {
				return false
			}
		}

		return true
	}
}

// ShouldIncludeByTargetCompatibility gates tools that declare RequiredGVKs behind
// target-compatibility filtering. A tool is included when it declares no RequiredGVKs,
// when the feature is disabled, or when at least one target provides all declared GVKs.
// This centralizes the "any target has the GVKs; assume yes on error" semantics that
// were previously duplicated in per-tool closures. The enabled flag is sourced from the
// applied Configuration (not the provider) so a SIGHUP reload that toggles it takes effect.
func ShouldIncludeByTargetCompatibility(ctx context.Context, p api.FilteringProvider, enabled bool) ToolFilter {
	return func(tool api.ServerTool) bool {
		if len(tool.RequiredGVKs) == 0 || !enabled {
			return true
		}
		return p.AnyTargetHasGVKs(ctx, tool.RequiredGVKs)
	}
}

func ShouldIncludeTargetListTool(targetName string, isMultiTarget bool) ToolFilter {
	return func(tool api.ServerTool) bool {
		if !tool.IsTargetListProvider() {
			return true
		}
		if !isMultiTarget {
			// there is no need to provide a tool to list the single available target
			return false
		}

		// Mutual exclusivity between configuration_contexts_list and targets_list:
		// - configuration_contexts_list: only for kubeconfig provider (targetName == "context")
		// - targets_list: only for non-kubeconfig providers (targetName != "context")
		// Note: targets_list gets mutated to "{targetName}_list" before this filter runs,
		// so we check for the mutated name pattern
		if tool.Tool.Name == "configuration_contexts_list" && targetName != kubernetes.KubeConfigTargetParameterName {
			return false
		}
		mutatedTargetsListName := targetName + "_list"
		if tool.Tool.Name == mutatedTargetsListName && targetName == kubernetes.KubeConfigTargetParameterName {
			return false
		}

		return true
	}
}
