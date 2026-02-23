package cosmos

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadCustomizationFile loads plugin customization from a YAML file.
func LoadCustomizationFile(path string) (Customization, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return Customization{}, fmt.Errorf("customization file path is required")
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return Customization{}, fmt.Errorf("read customization file %q: %w", cleanPath, err)
	}

	cfg, err := DecodeCustomizationYAML(data)
	if err != nil {
		return Customization{}, fmt.Errorf("decode customization file %q: %w", cleanPath, err)
	}

	return cfg, nil
}

// DecodeCustomizationYAML decodes and validates customization from YAML bytes.
func DecodeCustomizationYAML(data []byte) (Customization, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg Customization
	if err := dec.Decode(&cfg); err != nil {
		return Customization{}, fmt.Errorf("decode customization yaml: %w", err)
	}

	if err := validateCustomization(cfg); err != nil {
		return Customization{}, err
	}

	return cfg, nil
}

// EncodeCustomizationYAML renders customization as YAML.
func EncodeCustomizationYAML(cfg Customization) ([]byte, error) {
	if err := validateCustomization(cfg); err != nil {
		return nil, err
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal customization yaml: %w", err)
	}
	return out, nil
}

func validateCustomization(cfg Customization) error {
	if err := validateDurationString(cfg.Snapshot.ResolverTimeout, "snapshot.resolver_timeout"); err != nil {
		return err
	}
	if err := validateRegexString(cfg.Snapshot.MainnetURLPattern, "snapshot.mainnet_url_pattern"); err != nil {
		return err
	}
	if err := validateRegexString(cfg.Snapshot.TestnetURLPattern, "snapshot.testnet_url_pattern"); err != nil {
		return err
	}
	if err := validateDurationString(cfg.Timeouts.RequestTimeout, "timeouts.request_timeout"); err != nil {
		return err
	}
	if err := validateDurationString(cfg.Timeouts.WaitBlockTimeout, "timeouts.wait_block_timeout"); err != nil {
		return err
	}

	policy := strings.ToLower(strings.TrimSpace(cfg.Funding.InvalidBalancePolicy))
	if policy != "" && policy != invalidBalancePolicyFallback && policy != invalidBalancePolicyError {
		return fmt.Errorf("funding.invalid_balance_policy must be one of %q, %q", invalidBalancePolicyFallback, invalidBalancePolicyError)
	}

	unsupportedPolicy := strings.ToLower(strings.TrimSpace(cfg.RPCPolicy.UnsupportedNetworkBehavior))
	if unsupportedPolicy != "" && unsupportedPolicy != unsupportedNetworkEmpty {
		return fmt.Errorf("rpc_policy.unsupported_network_behavior must be %q", unsupportedNetworkEmpty)
	}

	return nil
}

func validateDurationString(raw string, field string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if _, err := time.ParseDuration(value); err != nil {
		return fmt.Errorf("%s is invalid duration: %w", field, err)
	}
	return nil
}

func validateRegexString(raw string, field string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if _, err := regexp.Compile(value); err != nil {
		return fmt.Errorf("%s is invalid regex: %w", field, err)
	}
	return nil
}
