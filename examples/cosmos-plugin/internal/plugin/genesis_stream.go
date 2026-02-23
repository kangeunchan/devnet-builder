package cosmos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	genesisinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/genesis"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

func (n *CosmosNetwork) streamModifyGenesis(dec *json.Decoder, w io.Writer, opts network.GenesisOptions) error {
	start, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read genesis start token: %w", err)
	}
	if d, ok := start.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("genesis root must be an object")
	}

	if _, err := io.WriteString(w, "{"); err != nil {
		return err
	}

	first := true
	cfg := n.GenesisConfig()
	hasChainID := false
	hasValidators := false
	hasAppState := false

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed to read genesis field name: %w", err)
		}

		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("invalid genesis field token type %T", keyTok)
		}

		if err := genesisinternal.WriteObjectKey(w, &first, key); err != nil {
			return err
		}

		switch key {
		case genesisFieldChainID:
			hasChainID = true
			if opts.ChainID == "" {
				if err := genesisinternal.WriteRawJSONValue(dec, w); err != nil {
					return fmt.Errorf("failed to copy %s: %w", genesisFieldChainID, err)
				}
				continue
			}

			if err := genesisinternal.DiscardJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard %s: %w", genesisFieldChainID, err)
			}
			if err := genesisinternal.WriteJSONValue(w, opts.ChainID); err != nil {
				return fmt.Errorf("failed to write %s: %w", genesisFieldChainID, err)
			}
		case genesisFieldValidators:
			hasValidators = true
			if err := genesisinternal.DiscardJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard %s: %w", genesisFieldValidators, err)
			}
			if err := genesisinternal.WriteJSONValue(w, []any{}); err != nil {
				return fmt.Errorf("failed to write %s: %w", genesisFieldValidators, err)
			}
		case genesisFieldAppState:
			hasAppState = true
			if err := n.streamModifyAppState(dec, w, opts, cfg); err != nil {
				return err
			}
		default:
			if err := genesisinternal.WriteRawJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy field %q: %w", key, err)
			}
		}
	}

	if !hasAppState {
		return fmt.Errorf("genesis missing %s", genesisFieldAppState)
	}

	if opts.ChainID != "" && !hasChainID {
		if err := genesisinternal.WriteObjectKey(w, &first, genesisFieldChainID); err != nil {
			return err
		}
		if err := genesisinternal.WriteJSONValue(w, opts.ChainID); err != nil {
			return fmt.Errorf("failed to write %s: %w", genesisFieldChainID, err)
		}
	}

	if !hasValidators {
		if err := genesisinternal.WriteObjectKey(w, &first, genesisFieldValidators); err != nil {
			return err
		}
		if err := genesisinternal.WriteJSONValue(w, []any{}); err != nil {
			return fmt.Errorf("failed to write %s: %w", genesisFieldValidators, err)
		}
	}

	end, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read genesis end token: %w", err)
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return fmt.Errorf("invalid genesis object terminator")
	}

	if _, err := io.WriteString(w, "}"); err != nil {
		return err
	}

	return nil
}

// streamModifyAppState keeps a streaming write path and defers bank emission until
// the end of app_state. This guarantees auth/staking-derived data is available when
// patching bank state without buffering the whole app_state in memory.
func (n *CosmosNetwork) streamModifyAppState(dec *json.Decoder, w io.Writer, opts network.GenesisOptions, cfg network.GenesisConfig) error {
	start, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read app_state start token: %w", err)
	}
	if d, ok := start.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("app_state must be an object")
	}

	if _, err := io.WriteString(w, "{"); err != nil {
		return err
	}

	validatorAccounts, bondedTotal := deriveValidatorAccountsAndBondedTotal(opts.Validators, n.Bech32Prefix())
	extraAccountAddresses := collectGenesisAccountAddresses(opts.AddAccounts)
	first := true

	var authModule map[string]any
	var deferredBank map[string]any
	var deferredBankPath string
	hasDeferredBank := false

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed to read app_state field name: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("invalid app_state field token type %T", keyTok)
		}
		switch key {
		case appModuleGov:
			module, err := decodeModuleObject(dec, appModuleGov)
			if err != nil {
				return err
			}
			n.patchGovParams(map[string]any{appModuleGov: module}, cfg)
			if err := n.applyAdditionalGenesisMutators(appModuleGov, module, opts, cfg); err != nil {
				return err
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case appModuleStaking:
			module, err := decodeModuleObject(dec, appModuleStaking)
			if err != nil {
				return err
			}
			actualValidatorAccounts, actualBondedTotal := n.patchStakingState(
				map[string]any{appModuleStaking: module}, opts, cfg)
			if actualValidatorAccounts != nil {
				validatorAccounts = actualValidatorAccounts
				bondedTotal = actualBondedTotal
			}
			if err := n.applyAdditionalGenesisMutators(appModuleStaking, module, opts, cfg); err != nil {
				return err
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case appModuleSlashing:
			module, err := decodeModuleObject(dec, appModuleSlashing)
			if err != nil {
				return err
			}
			n.patchSlashingState(map[string]any{appModuleSlashing: module})
			if err := n.applyAdditionalGenesisMutators(appModuleSlashing, module, opts, cfg); err != nil {
				return err
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case appModuleDistribution:
			module, err := decodeModuleObject(dec, appModuleDistribution)
			if err != nil {
				return err
			}
			n.patchDistributionState(map[string]any{appModuleDistribution: module}, opts)
			if err := n.applyAdditionalGenesisMutators(appModuleDistribution, module, opts, cfg); err != nil {
				return err
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case appModuleAuth:
			module, err := decodeModuleObject(dec, appModuleAuth)
			if err != nil {
				return err
			}
			genesisinternal.EnsureAuthBaseAccounts(module, mergeUniqueAddresses(validatorAccounts, extraAccountAddresses))
			authModule = module
			if err := n.applyAdditionalGenesisMutators(appModuleAuth, module, opts, cfg); err != nil {
				return err
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case appModuleBank:
			if len(n.hooks.AdditionalGenesisMutators) > 0 {
				module, err := decodeModuleObject(dec, appModuleBank)
				if err != nil {
					return err
				}
				deferredBank = module
				hasDeferredBank = true
				continue
			}

			tmpFile, err := os.CreateTemp("", "cosmos-bank-module-*.json")
			if err != nil {
				return fmt.Errorf("failed to create deferred bank temp file: %w", err)
			}

			tmpPath := tmpFile.Name()
			if err := genesisinternal.CopyJSONValue(dec, tmpFile); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpPath)
				return fmt.Errorf("failed to stream deferred bank module to temp file: %w", err)
			}
			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("failed to close deferred bank temp file: %w", err)
			}

			if deferredBankPath != "" {
				_ = os.Remove(deferredBankPath)
			}
			deferredBankPath = tmpPath
			hasDeferredBank = true
		case appModuleGenutil:
			module, err := decodeModuleObject(dec, appModuleGenutil)
			if err != nil {
				return err
			}
			module["gen_txs"] = []any{}
			if err := n.applyAdditionalGenesisMutators(appModuleGenutil, module, opts, cfg); err != nil {
				return err
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		default:
			if err := genesisinternal.WriteObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := genesisinternal.WriteRawJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy app_state module %q: %w", key, err)
			}
		}
	}

	if hasDeferredBank {
		if deferredBank != nil {
			if err := n.patchBankModule(deferredBank, authModule, validatorAccounts, bondedTotal, cfg, opts.AddAccounts); err != nil {
				return fmt.Errorf("failed to patch %s module: %w", appModuleBank, err)
			}
			if err := n.applyAdditionalGenesisMutators(appModuleBank, deferredBank, opts, cfg); err != nil {
				return err
			}
			if err := genesisinternal.WriteObjectKey(w, &first, appModuleBank); err != nil {
				return err
			}
			if err := genesisinternal.WriteJSONValue(w, deferredBank); err != nil {
				return fmt.Errorf("failed to write %s module: %w", appModuleBank, err)
			}
		} else if deferredBankPath != "" {
			defer os.Remove(deferredBankPath)
			if err := n.streamPatchDeferredBankModule(
				deferredBankPath,
				w,
				&first,
				authModule,
				validatorAccounts,
				bondedTotal,
				opts,
				cfg,
			); err != nil {
				return fmt.Errorf("failed to stream-patch %s module: %w", appModuleBank, err)
			}
		}
	}

	end, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read app_state end token: %w", err)
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return fmt.Errorf("invalid app_state object terminator")
	}

	_, err = io.WriteString(w, "}")
	return err
}

func (n *CosmosNetwork) applyAdditionalGenesisMutators(moduleName string, module map[string]any, opts network.GenesisOptions, cfg network.GenesisConfig) error {
	if n == nil || len(n.hooks.AdditionalGenesisMutators) == 0 {
		return nil
	}

	for _, mutator := range n.hooks.AdditionalGenesisMutators {
		if mutator == nil {
			continue
		}
		if err := mutator.MutateModule(moduleName, module, opts, cfg); err != nil {
			return fmt.Errorf("genesis mutator %q failed on %q: %w", mutator.Name(), moduleName, err)
		}
	}

	return nil
}

func decodeModuleObject(dec *json.Decoder, moduleName string) (map[string]any, error) {
	module, err := genesisinternal.ReadObjectValue(dec)
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s module: %w", moduleName, err)
	}
	return genesisinternal.EnsureMap(module), nil
}

func writeModuleObject(w io.Writer, first *bool, moduleName string, module map[string]any) error {
	if err := genesisinternal.WriteObjectKey(w, first, moduleName); err != nil {
		return err
	}
	if err := genesisinternal.WriteJSONValue(w, module); err != nil {
		return fmt.Errorf("failed to write %s module: %w", moduleName, err)
	}
	return nil
}
