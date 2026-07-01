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
// target-compatibility filtering, centralizing the "any target has the GVKs; assume
// yes on error" semantics that were previously duplicated in per-tool closures.
//
// A tool with no RequiredGVKs is always included. Otherwise:
//   - Single-target providers always evaluate the GVKs, restoring the pre-refactor
//     behavior where OpenShift-only tools are hidden on vanilla Kubernetes.
//   - Multi-target providers only evaluate when the feature is enabled, keeping the
//     (potentially expensive) cross-target discovery fan-out opt-in.
//
// The enabled flag is sourced from the applied Configuration (not the provider) so a
// SIGHUP reload that toggles it takes effect.
func ShouldIncludeByTargetCompatibility(ctx context.Context, p api.FilteringProvider, isMultiTarget, enabled bool) ToolFilter {
	return func(tool api.ServerTool) bool {
		if len(tool.RequiredGVKs) == 0 {
			return true
		}
		if isMultiTarget && !enabled {
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
