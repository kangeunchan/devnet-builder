package cosmos

import (
	"strings"

	snapshotresolver "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/snapshot"
)

const (
	invalidBalancePolicyFallback = "fallback"
	invalidBalancePolicyError    = "error"

	unsupportedNetworkEmpty = "empty"
)

// DefaultCustomization returns plugin defaults matching current behavior.
func DefaultCustomization() Customization {
	return Customization{
		NetworkProfiles: map[string]NetworkProfileConfig{
			networkMainnet: {
				ChainID:          "cosmoshub-4",
				RPCEndpoint:      "https://cosmoshub.rpc.kjnodes.com",
				RESTEndpoint:     "https://cosmoshub.api.kjnodes.com",
				SnapshotIndexURL: "https://www.polkachu.com/tendermint_snapshots/cosmos",
			},
			networkTestnet: {
				ChainID:          "provider",
				RPCEndpoint:      "https://cosmoshub-testnet.rpc.kjnodes.com",
				RESTEndpoint:     "https://cosmoshub-testnet.api.kjnodes.com",
				SnapshotIndexURL: "https://www.polkachu.com/testnets/cosmos/snapshots",
			},
		},
		Snapshot: SnapshotCustomization{
			ResolverTimeout:   defaultSnapshotResolverTimeout.String(),
			MainnetURLPattern: snapshotresolver.DefaultMainnetURLPattern,
			TestnetURLPattern: snapshotresolver.DefaultTestnetURLPattern,
		},
		Timeouts: TimeoutCustomization{
			RequestTimeout:   defaultRequestTimeout.String(),
			WaitBlockTimeout: defaultWaitTimeout.String(),
		},
		Funding: FundingCustomization{
			ValidatorBalance:      "1000000000000uatom",
			AccountBalance:        "100000000000uatom",
			InvalidBalancePolicy:  invalidBalancePolicyFallback,
			ValidatorStakeDefault: "100000000",
		},
		Runtime: RuntimeCustomization{
			BinaryName:    "gaiad",
			DockerImage:   "ghcr.io/cosmos/gaia:v25.3.2",
			DockerHomeDir: "/home/gaia",
			DefaultHome:   "/root/.gaia",
		},
		GenesisPolicy: GenesisPolicyCustomization{
			RequireValidators: boolPtr(true),
		},
		RPCPolicy: RPCPolicyCustomization{
			UnsupportedNetworkBehavior: unsupportedNetworkEmpty,
		},
	}
}

func (c Customization) mergedWith(override Customization) Customization {
	out := c

	out.NetworkProfiles = mergeNetworkProfiles(out.NetworkProfiles, override.NetworkProfiles)
	out.Endpoints = mergeEndpointsCustomization(out.Endpoints, override.Endpoints)
	out.Snapshot = mergeSnapshotCustomization(out.Snapshot, override.Snapshot)
	out.Timeouts = mergeTimeoutCustomization(out.Timeouts, override.Timeouts)
	out.Funding = mergeFundingCustomization(out.Funding, override.Funding)
	out.Runtime = mergeRuntimeCustomization(out.Runtime, override.Runtime)
	out.GenesisPolicy = mergeGenesisPolicyCustomization(out.GenesisPolicy, override.GenesisPolicy)
	out.RPCPolicy = mergeRPCPolicyCustomization(out.RPCPolicy, override.RPCPolicy)

	return out
}

func mergeNetworkProfiles(base map[string]NetworkProfileConfig, override map[string]NetworkProfileConfig) map[string]NetworkProfileConfig {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	out := make(map[string]NetworkProfileConfig, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		key := canonicalNetworkType(k)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(k))
		}
		if key == "" {
			continue
		}
		cur := out[key]
		if strings.TrimSpace(v.ChainID) != "" {
			cur.ChainID = strings.TrimSpace(v.ChainID)
		}
		if strings.TrimSpace(v.RPCEndpoint) != "" {
			cur.RPCEndpoint = strings.TrimSpace(v.RPCEndpoint)
		}
		if strings.TrimSpace(v.RESTEndpoint) != "" {
			cur.RESTEndpoint = strings.TrimSpace(v.RESTEndpoint)
		}
		if strings.TrimSpace(v.SnapshotIndexURL) != "" {
			cur.SnapshotIndexURL = strings.TrimSpace(v.SnapshotIndexURL)
		}
		out[key] = cur
	}

	return out
}

func mergeEndpointsCustomization(base EndpointsCustomization, override EndpointsCustomization) EndpointsCustomization {
	out := base
	if len(override.RESTToRPCHostMap) > 0 {
		out.RESTToRPCHostMap = mergeStringMap(base.RESTToRPCHostMap, override.RESTToRPCHostMap)
	}
	if len(override.RPCToRESTHostMap) > 0 {
		out.RPCToRESTHostMap = mergeStringMap(base.RPCToRESTHostMap, override.RPCToRESTHostMap)
	}
	return out
}

func mergeSnapshotCustomization(base SnapshotCustomization, override SnapshotCustomization) SnapshotCustomization {
	out := base
	if strings.TrimSpace(override.ResolverTimeout) != "" {
		out.ResolverTimeout = strings.TrimSpace(override.ResolverTimeout)
	}
	if strings.TrimSpace(override.MainnetURLPattern) != "" {
		out.MainnetURLPattern = strings.TrimSpace(override.MainnetURLPattern)
	}
	if strings.TrimSpace(override.TestnetURLPattern) != "" {
		out.TestnetURLPattern = strings.TrimSpace(override.TestnetURLPattern)
	}
	return out
}

func mergeTimeoutCustomization(base TimeoutCustomization, override TimeoutCustomization) TimeoutCustomization {
	out := base
	if strings.TrimSpace(override.RequestTimeout) != "" {
		out.RequestTimeout = strings.TrimSpace(override.RequestTimeout)
	}
	if strings.TrimSpace(override.WaitBlockTimeout) != "" {
		out.WaitBlockTimeout = strings.TrimSpace(override.WaitBlockTimeout)
	}
	return out
}

func mergeFundingCustomization(base FundingCustomization, override FundingCustomization) FundingCustomization {
	out := base
	if strings.TrimSpace(override.ValidatorBalance) != "" {
		out.ValidatorBalance = strings.TrimSpace(override.ValidatorBalance)
	}
	if strings.TrimSpace(override.AccountBalance) != "" {
		out.AccountBalance = strings.TrimSpace(override.AccountBalance)
	}
	if strings.TrimSpace(override.InvalidBalancePolicy) != "" {
		out.InvalidBalancePolicy = strings.TrimSpace(override.InvalidBalancePolicy)
	}
	if strings.TrimSpace(override.ValidatorStakeDefault) != "" {
		out.ValidatorStakeDefault = strings.TrimSpace(override.ValidatorStakeDefault)
	}
	return out
}

func mergeRuntimeCustomization(base RuntimeCustomization, override RuntimeCustomization) RuntimeCustomization {
	out := base
	if strings.TrimSpace(override.BinaryName) != "" {
		out.BinaryName = strings.TrimSpace(override.BinaryName)
	}
	if strings.TrimSpace(override.DockerImage) != "" {
		out.DockerImage = strings.TrimSpace(override.DockerImage)
	}
	if strings.TrimSpace(override.DockerHomeDir) != "" {
		out.DockerHomeDir = strings.TrimSpace(override.DockerHomeDir)
	}
	if strings.TrimSpace(override.DefaultHome) != "" {
		out.DefaultHome = strings.TrimSpace(override.DefaultHome)
	}
	return out
}

func mergeGenesisPolicyCustomization(base GenesisPolicyCustomization, override GenesisPolicyCustomization) GenesisPolicyCustomization {
	out := base
	if override.RequireValidators != nil {
		out.RequireValidators = boolPtr(*override.RequireValidators)
	}
	return out
}

func mergeRPCPolicyCustomization(base RPCPolicyCustomization, override RPCPolicyCustomization) RPCPolicyCustomization {
	out := base
	if strings.TrimSpace(override.UnsupportedNetworkBehavior) != "" {
		out.UnsupportedNetworkBehavior = strings.TrimSpace(override.UnsupportedNetworkBehavior)
	}
	return out
}

func mergeStringMap(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	for k, v := range override {
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}

	return out
}

func boolPtr(v bool) *bool {
	return &v
}
