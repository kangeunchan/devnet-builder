package cosmos

import (
	"context"
	"fmt"
	"strings"
)

// WithCustomization merges customization values on top of defaults.
func WithCustomization(cfg Customization) Option {
	return func(n *CosmosNetwork) {
		if n == nil {
			return
		}
		n.customizationFromOption = n.customizationFromOption.mergedWith(cfg)
	}
}

// WithCustomizationFile loads customization from YAML and merges it on top of defaults.
func WithCustomizationFile(path string) Option {
	return func(n *CosmosNetwork) {
		if n == nil {
			return
		}

		cfg, err := LoadCustomizationFile(path)
		if err != nil {
			if n.initErr == nil {
				n.initErr = fmt.Errorf("load customization file: %w", err)
			}
			return
		}
		n.customizationFromFile = n.customizationFromFile.mergedWith(cfg)
	}
}

// WithHooks registers advanced extension hooks.
func WithHooks(h Hooks) Option {
	return func(n *CosmosNetwork) {
		if n == nil {
			return
		}
		n.hooks = mergeHooks(n.hooks, h)
	}
}

func (n *CosmosNetwork) applyConfiguration() {
	if n == nil {
		return
	}

	cfg := DefaultCustomization()
	n.customization = cfg
	n.refreshDerivedConfiguration()

	merged := cfg.mergedWith(n.customizationFromFile)
	merged = merged.mergedWith(n.customizationFromOption)

	if err := validateCustomization(merged); err != nil {
		n.initErr = fmt.Errorf("invalid customization: %w", err)
		return
	}

	n.customization = merged
	n.refreshDerivedConfiguration()
}

func (n *CosmosNetwork) refreshDerivedConfiguration() {
	if n == nil {
		return
	}

	n.profiles = buildNetworkProfiles(n.customization.NetworkProfiles)
	n.restToRPCHostMap = buildRESTToRPCHostMapWithOverrides(n.profiles, n.customization.Endpoints.RESTToRPCHostMap)
	n.rpcToRESTHostMap = buildRPCToRESTHostMapWithOverrides(n.profiles, n.customization.Endpoints.RPCToRESTHostMap)

	n.requestTimeout = parseDurationWithFallback(n.customization.Timeouts.RequestTimeout, defaultRequestTimeout)
	n.waitBlockTimeout = parseDurationWithFallback(n.customization.Timeouts.WaitBlockTimeout, defaultWaitTimeout)
	n.snapshotResolveTimeout = parseDurationWithFallback(n.customization.Snapshot.ResolverTimeout, defaultSnapshotResolverTimeout)
}

func (n *CosmosNetwork) networkProfileByType(networkType string) (networkProfile, bool) {
	normalized := canonicalNetworkType(networkType)
	if normalized == "" {
		return networkProfile{}, false
	}

	profile, ok := n.profiles[normalized]
	return profile, ok
}

func (n *CosmosNetwork) requireNetworkProfile(networkType string) (networkProfile, error) {
	profile, ok := n.networkProfileByType(networkType)
	if ok {
		return profile, nil
	}
	return networkProfile{}, fmt.Errorf("unsupported network type %q", strings.TrimSpace(networkType))
}

func (n *CosmosNetwork) shouldRequireValidators() bool {
	if n == nil {
		return true
	}
	if n.customization.GenesisPolicy.RequireValidators == nil {
		return true
	}
	return *n.customization.GenesisPolicy.RequireValidators
}

func (n *CosmosNetwork) invalidBalancePolicy() string {
	policy := strings.ToLower(strings.TrimSpace(n.customization.Funding.InvalidBalancePolicy))
	if policy == invalidBalancePolicyError {
		return invalidBalancePolicyError
	}
	return invalidBalancePolicyFallback
}

func (n *CosmosNetwork) getJSON(ctx context.Context, endpoint string, out any) error {
	if n != nil && n.hooks.RPCRequestMiddleware != nil {
		return n.hooks.RPCRequestMiddleware(ctx, endpoint, out, getJSON)
	}
	return getJSON(ctx, endpoint, out)
}
