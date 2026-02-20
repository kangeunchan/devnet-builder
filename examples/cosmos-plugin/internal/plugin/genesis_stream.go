package cosmos

import (
	"encoding/json"
	"fmt"
	"io"

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

		if err := writeObjectKey(w, &first, key); err != nil {
			return err
		}

		switch key {
		case "chain_id":
			hasChainID = true
			if opts.ChainID == "" {
				if err := writeRawJSONValue(dec, w); err != nil {
					return fmt.Errorf("failed to copy chain_id: %w", err)
				}
				continue
			}

			if err := discardJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard chain_id: %w", err)
			}
			if err := writeJSONValue(w, opts.ChainID); err != nil {
				return fmt.Errorf("failed to write chain_id: %w", err)
			}
		case "validators":
			hasValidators = true
			if err := discardJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard validators: %w", err)
			}
			if err := writeJSONValue(w, []interface{}{}); err != nil {
				return fmt.Errorf("failed to write validators: %w", err)
			}
		case "app_state":
			hasAppState = true
			if err := n.streamModifyAppState(dec, w, opts, cfg); err != nil {
				return err
			}
		default:
			if err := writeRawJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy field %q: %w", key, err)
			}
		}
	}

	if !hasAppState {
		return fmt.Errorf("genesis missing app_state")
	}

	if opts.ChainID != "" && !hasChainID {
		if err := writeObjectKey(w, &first, "chain_id"); err != nil {
			return err
		}
		if err := writeJSONValue(w, opts.ChainID); err != nil {
			return fmt.Errorf("failed to write chain_id: %w", err)
		}
	}

	if !hasValidators {
		if err := writeObjectKey(w, &first, "validators"); err != nil {
			return err
		}
		if err := writeJSONValue(w, []interface{}{}); err != nil {
			return fmt.Errorf("failed to write validators: %w", err)
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

	var authModule map[string]interface{}
	var deferredBank map[string]interface{}
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
		case "gov":
			module, err := decodeModuleObject(dec, "gov")
			if err != nil {
				return err
			}
			n.patchGovParams(map[string]interface{}{"gov": module}, cfg)

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case "staking":
			module, err := decodeModuleObject(dec, "staking")
			if err != nil {
				return err
			}
			actualValidatorAccounts, actualBondedTotal := n.patchStakingState(
				map[string]interface{}{"staking": module}, opts, cfg)
			if actualValidatorAccounts != nil {
				validatorAccounts = actualValidatorAccounts
				bondedTotal = actualBondedTotal
			}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case "slashing":
			module, err := decodeModuleObject(dec, "slashing")
			if err != nil {
				return err
			}
			n.patchSlashingState(map[string]interface{}{"slashing": module})

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case "distribution":
			module, err := decodeModuleObject(dec, "distribution")
			if err != nil {
				return err
			}
			n.patchDistributionState(map[string]interface{}{"distribution": module}, opts)

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case "auth":
			module, err := decodeModuleObject(dec, "auth")
			if err != nil {
				return err
			}
			ensureAuthBaseAccounts(module, mergeUniqueAddresses(validatorAccounts, extraAccountAddresses))
			authModule = module

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		case "bank":
			module, err := decodeModuleObject(dec, "bank")
			if err != nil {
				return err
			}
			deferredBank = module
			hasDeferredBank = true
		case "genutil":
			module, err := decodeModuleObject(dec, "genutil")
			if err != nil {
				return err
			}
			module["gen_txs"] = []interface{}{}

			if err := writeModuleObject(w, &first, key, module); err != nil {
				return err
			}
		default:
			if err := writeObjectKey(w, &first, key); err != nil {
				return err
			}
			if err := writeRawJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy app_state module %q: %w", key, err)
			}
		}
	}

	if hasDeferredBank {
		n.patchBankModule(deferredBank, authModule, validatorAccounts, bondedTotal, cfg, opts.AddAccounts)
		if err := writeObjectKey(w, &first, "bank"); err != nil {
			return err
		}
		if err := writeJSONValue(w, deferredBank); err != nil {
			return fmt.Errorf("failed to write bank module: %w", err)
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

func decodeModuleObject(dec *json.Decoder, moduleName string) (map[string]interface{}, error) {
	module, err := readObjectValue(dec)
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s module: %w", moduleName, err)
	}
	return ensureMap(module), nil
}

func writeModuleObject(w io.Writer, first *bool, moduleName string, module map[string]interface{}) error {
	if err := writeObjectKey(w, first, moduleName); err != nil {
		return err
	}
	if err := writeJSONValue(w, module); err != nil {
		return fmt.Errorf("failed to write %s module: %w", moduleName, err)
	}
	return nil
}
