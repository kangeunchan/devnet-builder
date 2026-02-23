package cosmos

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	sdkmath "cosmossdk.io/math"
	genesisinternal "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/genesis"
	"github.com/altuslabsxyz/devnet-builder/pkg/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (n *CosmosNetwork) streamPatchDeferredBankModule(
	bankModulePath string,
	w io.Writer,
	first *bool,
	authModule map[string]any,
	validatorAccounts []string,
	bondedTotal sdkmath.Int,
	opts network.GenesisOptions,
	cfg network.GenesisConfig,
) error {
	in, err := os.Open(bankModulePath)
	if err != nil {
		return fmt.Errorf("failed to open deferred bank module %q: %w", bankModulePath, err)
	}
	defer in.Close()

	moduleAddrs := map[string]string{}
	if authModule != nil {
		moduleAddrs = genesisinternal.CollectModuleAccountAddresses(authModule)
	}
	targets, err := n.buildBankFundingTargets(validatorAccounts, opts.AddAccounts, bondedTotal, moduleAddrs, cfg.BaseDenom)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bufio.NewReader(in))
	start, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read deferred bank module start token: %w", err)
	}
	if d, ok := start.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("bank module must be an object")
	}

	if err := genesisinternal.WriteObjectKey(w, first, appModuleBank); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "{"); err != nil {
		return err
	}

	moduleFirst := true
	hasBalances := false
	totalSupply := sdk.NewCoins()

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed to read bank module field name: %w", err)
		}

		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("invalid bank module field token type %T", keyTok)
		}

		switch key {
		case "balances":
			hasBalances = true
			if err := genesisinternal.WriteObjectKey(w, &moduleFirst, "balances"); err != nil {
				return err
			}
			supply, err := n.streamPatchBankBalances(dec, w, cfg.BaseDenom, targets)
			if err != nil {
				return err
			}
			totalSupply = totalSupply.Add(supply...)
		case "supply":
			if err := genesisinternal.SkipJSONValue(dec); err != nil {
				return fmt.Errorf("failed to discard bank supply: %w", err)
			}
		default:
			if err := genesisinternal.WriteObjectKey(w, &moduleFirst, key); err != nil {
				return err
			}
			if err := genesisinternal.CopyJSONValue(dec, w); err != nil {
				return fmt.Errorf("failed to copy bank field %q: %w", key, err)
			}
		}
	}

	if !hasBalances {
		if err := genesisinternal.WriteObjectKey(w, &moduleFirst, "balances"); err != nil {
			return err
		}
		supply, err := writeOnlyTargetBalances(w, cfg.BaseDenom, targets)
		if err != nil {
			return err
		}
		totalSupply = totalSupply.Add(supply...)
	}

	if err := genesisinternal.WriteObjectKey(w, &moduleFirst, "supply"); err != nil {
		return err
	}
	if err := genesisinternal.WriteJSONValue(w, sdkCoinsToInterfaces(totalSupply.Sort())); err != nil {
		return fmt.Errorf("failed to write recomputed bank supply: %w", err)
	}

	end, err := dec.Token()
	if err != nil {
		return fmt.Errorf("failed to read deferred bank module end token: %w", err)
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return fmt.Errorf("invalid bank module object terminator")
	}

	_, err = io.WriteString(w, "}")
	return err
}

func (n *CosmosNetwork) streamPatchBankBalances(
	dec *json.Decoder,
	w io.Writer,
	baseDenom string,
	targets map[string]sdkmath.Int,
) (sdk.Coins, error) {
	start, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read bank balances start token: %w", err)
	}
	if d, ok := start.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("bank balances must be an array")
	}

	if _, err := io.WriteString(w, "["); err != nil {
		return nil, err
	}

	workerCount := genesisBalanceWorkers()
	jobs := make(chan []any, workerCount*2)
	results := make(chan sdk.Coins, workerCount)

	var wg sync.WaitGroup
	for idx := 0; idx < workerCount; idx++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := sdk.NewCoins()
			for coins := range jobs {
				local = addCoinsFromInterfaces(local, coins)
			}
			results <- local
		}()
	}

	stopWorkers := func() sdk.Coins {
		close(jobs)
		wg.Wait()
		close(results)

		total := sdk.NewCoins()
		for local := range results {
			if len(local) == 0 {
				continue
			}
			total = total.Add(local...)
		}
		return total.Sort()
	}

	firstEntry := true
	seen := make(map[string]struct{}, len(targets))

	for dec.More() {
		balance, err := genesisinternal.ReadObjectValue(dec)
		if err != nil {
			_ = stopWorkers()
			return nil, fmt.Errorf("failed to decode bank balance entry: %w", err)
		}
		balance = genesisinternal.EnsureMap(balance)

		addr := strings.TrimSpace(fmt.Sprint(balance["address"]))
		if desired, ok := targets[addr]; ok {
			genesisinternal.EnsureCoinAmount(balance, baseDenom, desired)
			seen[addr] = struct{}{}
		}

		coins, _ := asSlice(balance["coins"])
		if coins == nil {
			coins = []any{}
			balance["coins"] = coins
		}

		if !firstEntry {
			if _, err := io.WriteString(w, ","); err != nil {
				_ = stopWorkers()
				return nil, err
			}
		}
		firstEntry = false

		if err := genesisinternal.WriteJSONValue(w, balance); err != nil {
			_ = stopWorkers()
			return nil, fmt.Errorf("failed to write bank balance entry: %w", err)
		}

		jobs <- coins
	}

	end, err := dec.Token()
	if err != nil {
		_ = stopWorkers()
		return nil, fmt.Errorf("failed to read bank balances end token: %w", err)
	}
	if d, ok := end.(json.Delim); !ok || d != ']' {
		_ = stopWorkers()
		return nil, fmt.Errorf("invalid bank balances array terminator")
	}

	missingTargets := make([]string, 0, len(targets))
	for addr := range targets {
		if _, ok := seen[addr]; ok {
			continue
		}
		missingTargets = append(missingTargets, addr)
	}
	sort.Strings(missingTargets)

	for _, addr := range missingTargets {
		amount := targets[addr]
		entry := map[string]any{
			"address": addr,
			"coins": []any{
				map[string]any{
					"denom":  baseDenom,
					"amount": amount.String(),
				},
			},
		}

		if !firstEntry {
			if _, err := io.WriteString(w, ","); err != nil {
				_ = stopWorkers()
				return nil, err
			}
		}
		firstEntry = false

		if err := genesisinternal.WriteJSONValue(w, entry); err != nil {
			_ = stopWorkers()
			return nil, fmt.Errorf("failed to write synthesized bank balance entry: %w", err)
		}

		coins, _ := asSlice(entry["coins"])
		jobs <- coins
	}

	totalSupply := stopWorkers()

	if _, err := io.WriteString(w, "]"); err != nil {
		return nil, err
	}

	return totalSupply, nil
}

func writeOnlyTargetBalances(w io.Writer, baseDenom string, targets map[string]sdkmath.Int) (sdk.Coins, error) {
	if _, err := io.WriteString(w, "["); err != nil {
		return nil, err
	}

	addrs := make([]string, 0, len(targets))
	for addr := range targets {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	total := sdk.NewCoins()
	first := true

	for _, addr := range addrs {
		amount := targets[addr]
		entry := map[string]any{
			"address": addr,
			"coins": []any{
				map[string]any{
					"denom":  baseDenom,
					"amount": amount.String(),
				},
			},
		}

		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return nil, err
			}
		}
		first = false

		if err := genesisinternal.WriteJSONValue(w, entry); err != nil {
			return nil, fmt.Errorf("failed to write synthesized bank balance entry: %w", err)
		}

		total = total.Add(sdk.NewCoin(baseDenom, amount))
	}

	if _, err := io.WriteString(w, "]"); err != nil {
		return nil, err
	}

	return total.Sort(), nil
}

func addCoinsFromInterfaces(total sdk.Coins, coins []any) sdk.Coins {
	for _, raw := range coins {
		coinMap, ok := asMap(raw)
		if !ok {
			continue
		}

		denom := strings.TrimSpace(fmt.Sprint(coinMap["denom"]))
		if denom == "" {
			continue
		}
		if err := sdk.ValidateDenom(denom); err != nil {
			continue
		}

		amount := parseAmountIntOrZero(fmt.Sprint(coinMap["amount"]))
		total = total.Add(sdk.NewCoin(denom, amount))
	}

	return total
}

func sdkCoinsToInterfaces(coins sdk.Coins) []any {
	supply := make([]any, 0, len(coins))
	for _, c := range coins {
		supply = append(supply, map[string]any{
			"denom":  c.Denom,
			"amount": c.Amount.String(),
		})
	}
	return supply
}
