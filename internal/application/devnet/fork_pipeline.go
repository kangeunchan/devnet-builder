package devnet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	sdkmath "cosmossdk.io/math"
	"github.com/altuslabsxyz/devnet-builder/internal/application/dto"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	forkModeFull    = "fork-full"
	forkModeTrimmed = "fork-trimmed"

	// 3.5 GiB hard guardrail to fail before known runtime DB limits.
	genesisSizeGuardrailBytes = int64(3584 * 1024 * 1024)
)

var defaultTrimmedExportModules = []string{
	"auth",
	"bank",
	"distribution",
	"genutil",
	"gov",
	"mint",
	"params",
	"slashing",
	"staking",
}

type genesisPatchPolicy struct {
	ModuleOverrides map[string]any   `json:"module_overrides"`
	Patches         []genesisPatchOp `json:"patches"`
}

type genesisPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func normalizeForkMode(mode string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(mode))
	if normalized == "" {
		return forkModeTrimmed, nil
	}
	switch normalized {
	case forkModeFull, forkModeTrimmed:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid fork mode %q (must be %q or %q)", mode, forkModeFull, forkModeTrimmed)
	}
}

func normalizeExportModules(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		for _, entry := range strings.Split(raw, ",") {
			module := strings.TrimSpace(entry)
			if module == "" {
				continue
			}
			if _, ok := seen[module]; ok {
				continue
			}
			seen[module] = struct{}{}
			out = append(out, module)
		}
	}

	return out
}

func buildSnapshotExportOptions(input dto.ProvisionInput, defaults *ports.ExportOptions) (*ports.ExportOptions, error) {
	forkMode, err := normalizeForkMode(input.ForkMode)
	if err != nil {
		return nil, err
	}

	opts := ports.NewExportOptions()
	if defaults != nil {
		*opts = *defaults
	}
	opts.ForZeroHeight = true

	modules := normalizeExportModules(input.ExportModules)
	if len(modules) == 0 && forkMode == forkModeTrimmed {
		modules = append([]string(nil), defaultTrimmedExportModules...)
	}
	opts.ModulesToExport = modules

	return opts, nil
}

func applyForkTransform(genesis []byte, forkMode string) ([]byte, error) {
	if forkMode != forkModeTrimmed {
		return genesis, nil
	}

	root, err := decodeJSONMap(genesis)
	if err != nil {
		return nil, err
	}

	appState, ok := asMap(root["app_state"])
	if !ok {
		return nil, fmt.Errorf("missing app_state for fork transform")
	}

	moduleAccounts := collectModuleAccountAddresses(appState)
	if len(moduleAccounts) > 0 {
		trimAuthState(appState, moduleAccounts)
		trimBankState(appState, moduleAccounts)
	}
	trimGovState(appState)
	trimStakingState(appState)
	trimSlashingState(appState)
	trimDistributionState(appState)

	return marshalJSON(root)
}

func applyGenericPatchPolicy(genesis []byte, patchFile string) ([]byte, error) {
	patchFile = strings.TrimSpace(patchFile)
	if patchFile == "" {
		return genesis, nil
	}

	policy, err := loadGenesisPatchPolicy(patchFile)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return genesis, nil
	}

	root, err := decodeJSONMap(genesis)
	if err != nil {
		return nil, err
	}

	appState, ok := asMap(root["app_state"])
	if !ok {
		return nil, fmt.Errorf("missing app_state for generic patch")
	}

	for moduleName, override := range policy.ModuleOverrides {
		moduleName = strings.TrimSpace(moduleName)
		if moduleName == "" {
			continue
		}
		appState[moduleName] = override
	}

	for idx, op := range policy.Patches {
		if err := applyGenesisPatchOp(root, op); err != nil {
			return nil, fmt.Errorf("patch[%d] failed: %w", idx, err)
		}
	}

	return marshalJSON(root)
}

func recomputeDerivedGenesis(genesis []byte) ([]byte, error) {
	root, err := decodeJSONMap(genesis)
	if err != nil {
		return nil, err
	}

	appState, ok := asMap(root["app_state"])
	if !ok {
		return nil, fmt.Errorf("missing app_state")
	}
	bank, ok := asMap(appState["bank"])
	if !ok {
		return nil, fmt.Errorf("missing app_state.bank")
	}

	balances, _ := asSlice(bank["balances"])
	bank["supply"] = recomputeBankSupply(balances)

	return marshalJSON(root)
}

func validateGenesisForDeploy(genesis []byte) error {
	var envelope struct {
		ChainID  string         `json:"chain_id"`
		AppState map[string]any `json:"app_state"`
	}
	if err := json.Unmarshal(genesis, &envelope); err != nil {
		return fmt.Errorf("failed to parse final genesis: %w", err)
	}
	if strings.TrimSpace(envelope.ChainID) == "" {
		return fmt.Errorf("final genesis has empty chain_id")
	}
	if envelope.AppState == nil {
		return fmt.Errorf("final genesis missing app_state")
	}

	requiredModules := []string{"auth", "bank", "staking", "slashing", "gov"}
	for _, module := range requiredModules {
		if _, ok := envelope.AppState[module]; !ok {
			return fmt.Errorf("final genesis missing required module %q", module)
		}
	}

	return nil
}

func enforceGenesisSizeGuardrail(genesis []byte) error {
	return enforceGenesisSizeGuardrailSize(int64(len(genesis)), genesisSizeGuardrailBytes)
}

func enforceGenesisSizeGuardrailSize(size int64, limit int64) error {
	if size <= limit {
		return nil
	}

	return fmt.Errorf(
		"final genesis size %d bytes exceeds guardrail %d bytes (3.5GiB); use fork-trimmed and module export/patch policy to reduce state",
		size,
		limit,
	)
}

func decodeJSONMap(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}
	return out, nil
}

func marshalJSON(value any) ([]byte, error) {
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}
	return out, nil
}

func loadGenesisPatchPolicy(path string) (*genesisPatchPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis patch file %q: %w", path, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()

	var policy genesisPatchPolicy
	if err := dec.Decode(&policy); err != nil {
		return nil, fmt.Errorf("failed to parse genesis patch file %q: %w", path, err)
	}

	return &policy, nil
}

func collectModuleAccountAddresses(appState map[string]any) map[string]struct{} {
	result := map[string]struct{}{}
	auth, ok := asMap(appState["auth"])
	if !ok {
		return result
	}
	accounts, ok := asSlice(auth["accounts"])
	if !ok {
		return result
	}

	for _, raw := range accounts {
		account, ok := asMap(raw)
		if !ok {
			continue
		}
		typeURL, _ := account["@type"].(string)
		if !strings.Contains(typeURL, "ModuleAccount") {
			continue
		}
		if addr := extractGenesisAccountAddress(account); addr != "" {
			result[addr] = struct{}{}
		}
	}
	return result
}

func trimAuthState(appState map[string]any, moduleAccounts map[string]struct{}) {
	auth, ok := asMap(appState["auth"])
	if !ok {
		return
	}
	accounts, ok := asSlice(auth["accounts"])
	if !ok {
		return
	}

	filtered := make([]any, 0, len(moduleAccounts))
	for _, raw := range accounts {
		account, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, keep := moduleAccounts[extractGenesisAccountAddress(account)]; keep {
			filtered = append(filtered, raw)
		}
	}
	auth["accounts"] = filtered
}

func trimBankState(appState map[string]any, moduleAccounts map[string]struct{}) {
	bank, ok := asMap(appState["bank"])
	if !ok {
		return
	}
	balances, ok := asSlice(bank["balances"])
	if !ok {
		bank["supply"] = []any{}
		return
	}

	filtered := make([]any, 0, len(moduleAccounts))
	for _, raw := range balances {
		balance, ok := asMap(raw)
		if !ok {
			continue
		}
		addr, _ := balance["address"].(string)
		if _, keep := moduleAccounts[addr]; keep {
			filtered = append(filtered, balance)
		}
	}

	bank["balances"] = filtered
	bank["supply"] = recomputeBankSupply(filtered)
}

func trimGovState(appState map[string]any) {
	gov, ok := asMap(appState["gov"])
	if !ok {
		return
	}
	gov["proposals"] = []any{}
	gov["votes"] = []any{}
	gov["deposits"] = []any{}
}

func trimStakingState(appState map[string]any) {
	staking, ok := asMap(appState["staking"])
	if !ok {
		return
	}
	staking["validators"] = []any{}
	staking["delegations"] = []any{}
	staking["unbonding_delegations"] = []any{}
	staking["redelegations"] = []any{}
	staking["last_validator_powers"] = []any{}
	staking["last_total_power"] = "0"
	if pool, ok := asMap(staking["pool"]); ok {
		pool["bonded_tokens"] = "0"
	}
}

func trimSlashingState(appState map[string]any) {
	slashing, ok := asMap(appState["slashing"])
	if !ok {
		return
	}
	slashing["signing_infos"] = []any{}
	slashing["missed_blocks"] = []any{}
}

func trimDistributionState(appState map[string]any) {
	distribution, ok := asMap(appState["distribution"])
	if !ok {
		return
	}
	distribution["delegator_starting_infos"] = []any{}
	distribution["validator_slash_events"] = []any{}
	distribution["validator_historical_rewards"] = []any{}
	distribution["validator_current_rewards"] = []any{}
	distribution["validator_accumulated_commissions"] = []any{}
	distribution["outstanding_rewards"] = []any{}
}

func extractGenesisAccountAddress(account map[string]any) string {
	if addr, ok := account["address"].(string); ok && strings.TrimSpace(addr) != "" {
		return strings.TrimSpace(addr)
	}
	base, ok := asMap(account["base_account"])
	if !ok {
		return ""
	}
	addr, _ := base["address"].(string)
	return strings.TrimSpace(addr)
}

func recomputeBankSupply(balances []any) []any {
	total := sdk.NewCoins()
	for _, raw := range balances {
		balance, ok := asMap(raw)
		if !ok {
			continue
		}
		coins, ok := asSlice(balance["coins"])
		if !ok {
			continue
		}
		for _, coinRaw := range coins {
			coinMap, ok := asMap(coinRaw)
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
			amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(fmt.Sprint(coinMap["amount"])))
			if !ok || amount.IsNegative() {
				continue
			}
			total = total.Add(sdk.NewCoin(denom, amount))
		}
	}

	supply := make([]any, 0, len(total))
	for _, coin := range total.Sort() {
		supply = append(supply, map[string]any{
			"denom":  coin.Denom,
			"amount": coin.Amount.String(),
		})
	}
	return supply
}

func applyGenesisPatchOp(root map[string]any, op genesisPatchOp) error {
	kind := strings.TrimSpace(strings.ToLower(op.Op))
	if kind == "" {
		return fmt.Errorf("patch op is required")
	}
	switch kind {
	case "set":
		return setJSONPointer(root, op.Path, op.Value)
	case "delete":
		return deleteJSONPointer(root, op.Path)
	case "merge":
		return mergeJSONPointer(root, op.Path, op.Value)
	case "append":
		return appendJSONPointer(root, op.Path, op.Value)
	default:
		return fmt.Errorf("unsupported patch op %q", op.Op)
	}
}

func setJSONPointer(root map[string]any, pointer string, value any) error {
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("set on root is not supported")
	}

	parent, key, err := resolveJSONPointerParent(root, tokens)
	if err != nil {
		return err
	}

	switch container := parent.(type) {
	case map[string]any:
		container[key] = value
		return nil
	case []any:
		idx, err := parseJSONIndex(key, len(container), true)
		if err != nil {
			return err
		}
		if idx == len(container) {
			return fmt.Errorf("set at array append path %q requires explicit append op", pointer)
		}
		container[idx] = value
		return nil
	default:
		return fmt.Errorf("set target parent at %q is not a container", pointer)
	}
}

func deleteJSONPointer(root map[string]any, pointer string) error {
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return fmt.Errorf("delete on root is not supported")
	}

	parent, key, err := resolveJSONPointerParent(root, tokens)
	if err != nil {
		return err
	}

	switch container := parent.(type) {
	case map[string]any:
		delete(container, key)
		return nil
	case []any:
		idx, err := parseJSONIndex(key, len(container), false)
		if err != nil {
			return err
		}
		container = append(container[:idx], container[idx+1:]...)
		if err := replaceJSONPointer(root, tokens[:len(tokens)-1], container); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("delete target parent at %q is not a container", pointer)
	}
}

func mergeJSONPointer(root map[string]any, pointer string, value any) error {
	target, err := getJSONPointer(root, pointer)
	if err != nil {
		return err
	}
	targetMap, ok := asMap(target)
	if !ok {
		return fmt.Errorf("merge target at %q is not an object", pointer)
	}
	valueMap, ok := asMap(value)
	if !ok {
		return fmt.Errorf("merge value at %q is not an object", pointer)
	}
	deepMergeMap(targetMap, valueMap)
	return nil
}

func appendJSONPointer(root map[string]any, pointer string, value any) error {
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return err
	}

	target, err := getJSONPointer(root, pointer)
	if err != nil {
		return err
	}
	list, ok := asSlice(target)
	if !ok {
		return fmt.Errorf("append target at %q is not an array", pointer)
	}

	if values, ok := asSlice(value); ok {
		list = append(list, values...)
	} else {
		list = append(list, value)
	}
	return replaceJSONPointer(root, tokens, list)
}

func deepMergeMap(dst, src map[string]any) {
	for key, value := range src {
		srcChild, srcIsMap := asMap(value)
		dstChild, dstIsMap := asMap(dst[key])
		if srcIsMap && dstIsMap {
			deepMergeMap(dstChild, srcChild)
			continue
		}
		dst[key] = value
	}
}

func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, fmt.Errorf("patch path is required")
	}
	if pointer[0] != '/' {
		return nil, fmt.Errorf("patch path %q must be RFC6901 json-pointer format", pointer)
	}
	parts := strings.Split(pointer[1:], "/")
	tokens := make([]string, 0, len(parts))
	for _, token := range parts {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func resolveJSONPointerParent(root map[string]any, tokens []string) (any, string, error) {
	current := any(root)
	for _, token := range tokens[:len(tokens)-1] {
		next, err := getChildByToken(current, token)
		if err != nil {
			return nil, "", err
		}
		current = next
	}
	return current, tokens[len(tokens)-1], nil
}

func getJSONPointer(root map[string]any, pointer string) (any, error) {
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, err
	}

	current := any(root)
	for _, token := range tokens {
		next, err := getChildByToken(current, token)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func replaceJSONPointer(root map[string]any, tokens []string, value any) error {
	if len(tokens) == 0 {
		return fmt.Errorf("replace on root is not supported")
	}
	parent, key, err := resolveJSONPointerParent(root, tokens)
	if err != nil {
		return err
	}
	switch container := parent.(type) {
	case map[string]any:
		container[key] = value
		return nil
	case []any:
		idx, err := parseJSONIndex(key, len(container), false)
		if err != nil {
			return err
		}
		container[idx] = value
		return nil
	default:
		return fmt.Errorf("replace target parent is not a container")
	}
}

func getChildByToken(current any, token string) (any, error) {
	switch typed := current.(type) {
	case map[string]any:
		next, ok := typed[token]
		if !ok {
			return nil, fmt.Errorf("path segment %q not found", token)
		}
		return next, nil
	case []any:
		idx, err := parseJSONIndex(token, len(typed), false)
		if err != nil {
			return nil, err
		}
		return typed[idx], nil
	default:
		return nil, fmt.Errorf("path segment %q is not indexable", token)
	}
}

func parseJSONIndex(token string, length int, allowAppend bool) (int, error) {
	if token == "-" {
		if allowAppend {
			return length, nil
		}
		return 0, fmt.Errorf("'-' index is only valid for append")
	}
	idx, err := strconv.Atoi(token)
	if err != nil {
		return 0, fmt.Errorf("invalid array index %q", token)
	}
	if idx < 0 || idx >= length {
		if allowAppend && idx == length {
			return idx, nil
		}
		return 0, fmt.Errorf("array index %d out of range (len=%d)", idx, length)
	}
	return idx, nil
}

func asMap(value any) (map[string]any, bool) {
	v, ok := value.(map[string]any)
	return v, ok
}

func asSlice(value any) ([]any, bool) {
	v, ok := value.([]any)
	return v, ok
}
