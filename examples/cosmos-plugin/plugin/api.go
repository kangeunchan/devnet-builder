package cosmos

import (
	"context"

	internal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
)

type (
	CosmosNetwork = internal.CosmosNetwork
	Option        = internal.Option

	Hooks                    = internal.Hooks
	GenesisMutator           = internal.GenesisMutator
	JSONRequestFunc          = internal.JSONRequestFunc
	SnapshotURLResolverHook  = internal.SnapshotURLResolverHook
	RPCRequestMiddlewareHook = internal.RPCRequestMiddlewareHook

	Customization              = internal.Customization
	NetworkProfileConfig       = internal.NetworkProfileConfig
	EndpointsCustomization     = internal.EndpointsCustomization
	SnapshotCustomization      = internal.SnapshotCustomization
	TimeoutCustomization       = internal.TimeoutCustomization
	FundingCustomization       = internal.FundingCustomization
	RuntimeCustomization       = internal.RuntimeCustomization
	GenesisPolicyCustomization = internal.GenesisPolicyCustomization
	RPCPolicyCustomization     = internal.RPCPolicyCustomization
)

func New(opts ...Option) *CosmosNetwork {
	return internal.New(opts...)
}

func WithCommandRunner(runner func(context.Context, string, ...string) ([]byte, error)) Option {
	return internal.WithCommandRunner(runner)
}

func WithCustomization(cfg Customization) Option {
	return internal.WithCustomization(cfg)
}

func WithCustomizationFile(path string) Option {
	return internal.WithCustomizationFile(path)
}

func WithHooks(h Hooks) Option {
	return internal.WithHooks(h)
}

func DefaultCustomization() Customization {
	return internal.DefaultCustomization()
}

func LoadCustomizationFile(path string) (Customization, error) {
	return internal.LoadCustomizationFile(path)
}

func DecodeCustomizationYAML(data []byte) (Customization, error) {
	return internal.DecodeCustomizationYAML(data)
}

func EncodeCustomizationYAML(cfg Customization) ([]byte, error) {
	return internal.EncodeCustomizationYAML(cfg)
}
