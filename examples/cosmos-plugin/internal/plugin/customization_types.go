package cosmos

// NetworkProfileConfig defines chain-specific endpoints and identifiers.
type NetworkProfileConfig struct {
	ChainID          string `yaml:"chain_id"`
	RPCEndpoint      string `yaml:"rpc_endpoint"`
	RESTEndpoint     string `yaml:"rest_endpoint"`
	SnapshotIndexURL string `yaml:"snapshot_index_url"`
}

// EndpointsCustomization controls host rewrite rules applied when users pass
// mixed RPC/REST endpoints into plugin RPC methods.
type EndpointsCustomization struct {
	RESTToRPCHostMap map[string]string `yaml:"rest_to_rpc_host_map"`
	RPCToRESTHostMap map[string]string `yaml:"rpc_to_rest_host_map"`
}

// SnapshotCustomization controls snapshot URL resolution behavior.
type SnapshotCustomization struct {
	ResolverTimeout   string `yaml:"resolver_timeout"`
	MainnetURLPattern string `yaml:"mainnet_url_pattern"`
	TestnetURLPattern string `yaml:"testnet_url_pattern"`
}

// TimeoutCustomization controls request-level timeout values.
type TimeoutCustomization struct {
	RequestTimeout   string `yaml:"request_timeout"`
	WaitBlockTimeout string `yaml:"wait_block_timeout"`
}

// FundingCustomization controls additional-account funding behavior.
type FundingCustomization struct {
	ValidatorBalance      string `yaml:"validator_balance"`
	AccountBalance        string `yaml:"account_balance"`
	InvalidBalancePolicy  string `yaml:"invalid_balance_policy"`
	ValidatorStakeDefault string `yaml:"validator_stake_default"`
}

// RuntimeCustomization controls runtime-specific defaults exposed by module methods.
type RuntimeCustomization struct {
	BinaryName    string `yaml:"binary_name"`
	DockerImage   string `yaml:"docker_image"`
	DockerHomeDir string `yaml:"docker_home_dir"`
	DefaultHome   string `yaml:"default_node_home"`
}

// GenesisPolicyCustomization controls genesis mutation policies.
type GenesisPolicyCustomization struct {
	RequireValidators *bool `yaml:"require_validators"`
}

// RPCPolicyCustomization controls unsupported network handling.
type RPCPolicyCustomization struct {
	UnsupportedNetworkBehavior string `yaml:"unsupported_network_behavior"`
}

// Customization is the user-facing plugin customization surface.
// It can be injected via API options or loaded from YAML.
type Customization struct {
	NetworkProfiles map[string]NetworkProfileConfig `yaml:"network_profiles"`
	Endpoints       EndpointsCustomization          `yaml:"endpoints"`
	Snapshot        SnapshotCustomization           `yaml:"snapshot"`
	Timeouts        TimeoutCustomization            `yaml:"timeouts"`
	Funding         FundingCustomization            `yaml:"funding"`
	Runtime         RuntimeCustomization            `yaml:"runtime"`
	GenesisPolicy   GenesisPolicyCustomization      `yaml:"genesis_policy"`
	RPCPolicy       RPCPolicyCustomization          `yaml:"rpc_policy"`
}
