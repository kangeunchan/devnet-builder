package config

import (
	"os"

	"github.com/altuslabsxyz/devnet-builder/types"
	"github.com/spf13/cobra"
)

// Resolver centralizes configuration precedence handling.
// Priority: default < config.toml < environment < CLI flag.
type Resolver struct{}

// NewResolver creates a new Resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// GlobalResolveInput contains current global flag values.
type GlobalResolveInput struct {
	Home    string
	Verbose bool
	JSON    bool
	NoColor bool
}

// GlobalResolveOutput contains resolved global values and sources.
type GlobalResolveOutput struct {
	Home    StringValue
	Verbose BoolValue
	JSON    BoolValue
	NoColor BoolValue
}

// RuntimeResolveInput contains runtime config values from command flags.
type RuntimeResolveInput struct {
	Network           string
	BlockchainNetwork string
	Validators        int
	Mode              types.ExecutionMode
	NetworkVersion    string
	NoCache           bool
	Accounts          int
}

// RuntimeResolveOutput contains resolved runtime file config and field sources.
type RuntimeResolveOutput struct {
	FileConfig *FileConfig
	Sources    map[string]ConfigSource
}

// ResolveGlobal resolves global values using unified precedence.
func (r *Resolver) ResolveGlobal(cmd *cobra.Command, fileCfg *FileConfig, in GlobalResolveInput) GlobalResolveOutput {
	out := GlobalResolveOutput{
		Home:    StringValue{Value: in.Home, Source: SourceDefault},
		Verbose: BoolValue{Value: in.Verbose, Source: SourceDefault},
		JSON:    BoolValue{Value: in.JSON, Source: SourceDefault},
		NoColor: BoolValue{Value: in.NoColor, Source: SourceDefault},
	}

	if fileCfg != nil {
		if fileCfg.Home != nil {
			out.Home = StringValue{Value: *fileCfg.Home, Source: SourceConfigFile}
		}
		if fileCfg.Verbose != nil {
			out.Verbose = BoolValue{Value: *fileCfg.Verbose, Source: SourceConfigFile}
		}
		if fileCfg.JSON != nil {
			out.JSON = BoolValue{Value: *fileCfg.JSON, Source: SourceConfigFile}
		}
		if fileCfg.NoColor != nil {
			out.NoColor = BoolValue{Value: *fileCfg.NoColor, Source: SourceConfigFile}
		}
	}

	if envHome := os.Getenv("DEVNET_HOME"); envHome != "" && !flagChanged(cmd, "home") {
		out.Home = StringValue{Value: envHome, Source: SourceEnvironment}
	}
	if os.Getenv("NO_COLOR") != "" && !flagChanged(cmd, "no-color") {
		out.NoColor = BoolValue{Value: true, Source: SourceEnvironment}
	}

	if flagChanged(cmd, "home") {
		out.Home = StringValue{Value: in.Home, Source: SourceFlag}
	}
	if flagChanged(cmd, "verbose") {
		out.Verbose = BoolValue{Value: in.Verbose, Source: SourceFlag}
	}
	if flagChanged(cmd, "json") {
		out.JSON = BoolValue{Value: in.JSON, Source: SourceFlag}
	}
	if flagChanged(cmd, "no-color") {
		out.NoColor = BoolValue{Value: in.NoColor, Source: SourceFlag}
	}

	return out
}

// ResolveRuntimeFileConfig resolves runtime settings into a FileConfig while
// preserving RunPartial behavior (unset/default fields remain nil).
func (r *Resolver) ResolveRuntimeFileConfig(cmd *cobra.Command, base *FileConfig, in RuntimeResolveInput) RuntimeResolveOutput {
	cfg := cloneFileConfig(base)
	sources := map[string]ConfigSource{
		"network":            sourceFromStringPtr(cfg.Network),
		"blockchain_network": sourceFromStringPtr(cfg.BlockchainNetwork),
		"validators":         sourceFromIntPtr(cfg.Validators),
		"mode":               sourceFromModePtr(cfg.ExecutionMode),
		"network_version":    sourceFromStringPtr(cfg.NetworkVersion),
		"no_cache":           sourceFromBoolPtr(cfg.NoCache),
		"accounts":           sourceFromIntPtr(cfg.Accounts),
	}

	if flagChanged(cmd, "network") {
		cfg.Network = stringPtr(in.Network)
		sources["network"] = SourceFlag
	} else if env := os.Getenv("DEVNET_NETWORK"); env != "" {
		cfg.Network = stringPtr(env)
		sources["network"] = SourceEnvironment
	}

	if flagChanged(cmd, "blockchain") {
		cfg.BlockchainNetwork = stringPtr(in.BlockchainNetwork)
		sources["blockchain_network"] = SourceFlag
	}

	if flagChanged(cmd, "validators") {
		cfg.Validators = intPtr(in.Validators)
		sources["validators"] = SourceFlag
	}

	if flagChanged(cmd, "mode") {
		cfg.ExecutionMode = modePtr(in.Mode)
		sources["mode"] = SourceFlag
	} else if env := os.Getenv("DEVNET_MODE"); env != "" {
		cfg.ExecutionMode = modePtr(types.ExecutionMode(env))
		sources["mode"] = SourceEnvironment
	}

	if flagChanged(cmd, "network-version") {
		cfg.NetworkVersion = stringPtr(in.NetworkVersion)
		sources["network_version"] = SourceFlag
	} else if env := os.Getenv("DEVNET_NETWORK_VERSION"); env != "" {
		cfg.NetworkVersion = stringPtr(env)
		sources["network_version"] = SourceEnvironment
	}

	if flagChanged(cmd, "no-cache") {
		cfg.NoCache = boolPtr(in.NoCache)
		sources["no_cache"] = SourceFlag
	}

	if flagChanged(cmd, "accounts") {
		cfg.Accounts = intPtr(in.Accounts)
		sources["accounts"] = SourceFlag
	}

	return RuntimeResolveOutput{
		FileConfig: cfg,
		Sources:    sources,
	}
}

func cloneFileConfig(base *FileConfig) *FileConfig {
	if base == nil {
		return &FileConfig{}
	}

	out := &FileConfig{}
	if base.Home != nil {
		out.Home = stringPtr(*base.Home)
	}
	if base.NoColor != nil {
		out.NoColor = boolPtr(*base.NoColor)
	}
	if base.Verbose != nil {
		out.Verbose = boolPtr(*base.Verbose)
	}
	if base.JSON != nil {
		out.JSON = boolPtr(*base.JSON)
	}
	if base.Network != nil {
		out.Network = stringPtr(*base.Network)
	}
	if base.BlockchainNetwork != nil {
		out.BlockchainNetwork = stringPtr(*base.BlockchainNetwork)
	}
	if base.Validators != nil {
		out.Validators = intPtr(*base.Validators)
	}
	if base.ExecutionMode != nil {
		out.ExecutionMode = modePtr(*base.ExecutionMode)
	}
	if base.NetworkVersion != nil {
		out.NetworkVersion = stringPtr(*base.NetworkVersion)
	}
	if base.NoCache != nil {
		out.NoCache = boolPtr(*base.NoCache)
	}
	if base.Accounts != nil {
		out.Accounts = intPtr(*base.Accounts)
	}
	if base.GitHubToken != nil {
		out.GitHubToken = stringPtr(*base.GitHubToken)
	}
	if base.CacheTTL != nil {
		out.CacheTTL = stringPtr(*base.CacheTTL)
	}
	return out
}

func flagChanged(cmd *cobra.Command, flagName string) bool {
	if cmd == nil || cmd.Flags() == nil || cmd.Flags().Lookup(flagName) == nil {
		return false
	}
	return cmd.Flags().Changed(flagName)
}

func sourceFromStringPtr(v *string) ConfigSource {
	if v == nil {
		return SourceDefault
	}
	return SourceConfigFile
}

func sourceFromIntPtr(v *int) ConfigSource {
	if v == nil {
		return SourceDefault
	}
	return SourceConfigFile
}

func sourceFromBoolPtr(v *bool) ConfigSource {
	if v == nil {
		return SourceDefault
	}
	return SourceConfigFile
}

func sourceFromModePtr(v *types.ExecutionMode) ConfigSource {
	if v == nil {
		return SourceDefault
	}
	return SourceConfigFile
}

func stringPtr(v string) *string { return &v }

func intPtr(v int) *int { return &v }

func boolPtr(v bool) *bool { return &v }

func modePtr(v types.ExecutionMode) *types.ExecutionMode { return &v }
