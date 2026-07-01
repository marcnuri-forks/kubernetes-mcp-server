package api

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// FilteringProvider provides tool filtering capabilities based on cluster capabilities
// (GVKs, features, etc.). Toolsets use this interface to determine which tools should
// be exposed.
type FilteringProvider interface {
	TargetCompatibilityToolFiltersEnabledProvider
	// IsMultiTarget reports whether the provider is configured for multiple targets.
	// Toolsets use it to keep the (potentially expensive) multi-target discovery
	// fan-out opt-in while still filtering single-target setups by default.
	IsMultiTarget() bool
	// AnyTargetHasGVKs reports whether every GVK in gvks is available on at least one target
	// exposed by this provider. Providers that have not opted in to GVK discovery
	// should return true so existing tools remain visible.
	AnyTargetHasGVKs(context.Context, []schema.GroupVersionKind) bool
}
