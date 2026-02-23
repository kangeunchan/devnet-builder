package cosmos

import (
	"context"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// GenesisMutator allows users to inject additional module-level genesis mutations.
type GenesisMutator interface {
	Name() string
	MutateModule(moduleName string, module map[string]any, opts network.GenesisOptions, cfg network.GenesisConfig) error
}

// JSONRequestFunc is the low-level RPC JSON request function used by middleware.
type JSONRequestFunc func(ctx context.Context, endpoint string, out any) error

// SnapshotURLResolverHook resolves snapshot URLs with access to the effective profile and customization.
type SnapshotURLResolverHook func(ctx context.Context, networkType string, profile NetworkProfileConfig, cfg Customization) (string, error)

// RPCRequestMiddlewareHook intercepts JSON RPC/REST requests made by this plugin.
type RPCRequestMiddlewareHook func(ctx context.Context, endpoint string, out any, next JSONRequestFunc) error

// Hooks contains optional advanced extension points.
type Hooks struct {
	SnapshotURLResolver       SnapshotURLResolverHook
	AdditionalGenesisMutators []GenesisMutator
	RPCRequestMiddleware      RPCRequestMiddlewareHook
}

func mergeHooks(base Hooks, override Hooks) Hooks {
	out := base
	if override.SnapshotURLResolver != nil {
		out.SnapshotURLResolver = override.SnapshotURLResolver
	}
	if override.RPCRequestMiddleware != nil {
		out.RPCRequestMiddleware = override.RPCRequestMiddleware
	}
	if len(override.AdditionalGenesisMutators) > 0 {
		out.AdditionalGenesisMutators = append([]GenesisMutator{}, override.AdditionalGenesisMutators...)
	}
	return out
}
