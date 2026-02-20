// Package genesis provides genesis fetching and export implementations.
package genesis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
)

// FetcherAdapter implements ports.GenesisFetcher.
type FetcherAdapter struct {
	homeDir     string
	binaryPath  string
	dockerImage string
	useDocker   bool
	logger      *output.Logger
}

// NewFetcherAdapter creates a new FetcherAdapter.
func NewFetcherAdapter(homeDir, binaryPath, dockerImage string, useDocker bool, logger *output.Logger) *FetcherAdapter {
	if logger == nil {
		logger = output.DefaultLogger
	}
	return &FetcherAdapter{
		homeDir:     homeDir,
		binaryPath:  binaryPath,
		dockerImage: dockerImage,
		useDocker:   useDocker,
		logger:      logger,
	}
}

// ExportFromChain exports genesis from a running chain.
func (f *FetcherAdapter) ExportFromChain(ctx context.Context, nodeHomeDir string) ([]byte, error) {
	var exportOutput []byte
	var err error

	if f.useDocker && f.dockerImage != "" {
		exportOutput, err = f.exportFromDocker(ctx, nodeHomeDir)
	} else if f.binaryPath != "" {
		exportOutput, err = f.exportFromBinary(ctx, nodeHomeDir)
	} else {
		return nil, &GenesisError{
			Operation: "export",
			Message:   "no binary or docker image configured",
		}
	}

	if err != nil {
		return nil, &GenesisError{
			Operation: "export",
			Message:   err.Error(),
		}
	}

	return exportOutput, nil
}

func (f *FetcherAdapter) exportFromBinary(ctx context.Context, homeDir string) ([]byte, error) {
	f.logger.Debug("Exporting genesis using binary: %s export --home %s", f.binaryPath, homeDir)

	cmd := exec.CommandContext(ctx, f.binaryPath, "export", "--home", homeDir)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Include the actual error output in the error message
		return nil, fmt.Errorf("genesis export failed: %w\nOutput: %s", err, string(output))
	}

	// The export command writes genesis to stdout
	// But CombinedOutput includes stderr too, which might have warnings
	// We need to extract just the JSON part
	return extractGenesisJSON(output)
}

// extractGenesisJSON extracts valid JSON from command output.
// The export command might include warnings/logs before the actual JSON.
// It validates the extracted JSON is actually a genesis object by checking
// for characteristic fields (chain_id or app_state), skipping any JSON
// log lines that may precede the genesis output.
func extractGenesisJSON(output []byte) ([]byte, error) {
	searchFrom := 0
	for searchFrom < len(output) {
		// Find the next '{' starting from searchFrom
		jsonStart := -1
		for i := searchFrom; i < len(output); i++ {
			if output[i] == '{' {
				jsonStart = i
				break
			}
		}

		if jsonStart == -1 {
			break
		}

		jsonData := output[jsonStart:]

		// Validate it's valid JSON
		var js json.RawMessage
		if err := json.Unmarshal(jsonData, &js); err != nil {
			// Not valid JSON from this position, skip past this '{' and try next
			searchFrom = jsonStart + 1
			continue
		}

		// Check if this looks like a genesis object (has chain_id or app_state)
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(jsonData, &probe); err == nil {
			if _, hasChainID := probe["chain_id"]; hasChainID {
				return jsonData, nil
			}
			if _, hasAppState := probe["app_state"]; hasAppState {
				return jsonData, nil
			}
		}

		// Valid JSON but not genesis, skip past this object and try next
		searchFrom = jsonStart + len(jsonData)
	}

	return nil, fmt.Errorf("no genesis JSON found in export output (looked for chain_id or app_state): %s",
		string(output[:min(len(output), 500)]))
}

func (f *FetcherAdapter) exportFromDocker(ctx context.Context, homeDir string) ([]byte, error) {
	uid := os.Getuid()
	gid := os.Getgid()

	args := []string{
		"run", "--rm",
		"--user", fmt.Sprintf("%d:%d", uid, gid),
		"-e", "HOME=/data",
		"-v", homeDir + ":/data",
		f.dockerImage,
		"export", "--home", "/data",
	}

	f.logger.Debug("Exporting genesis using docker: docker %v", args)

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("genesis export failed: %w\nOutput: %s", err, string(output))
	}

	return extractGenesisJSON(output)
}

// FetchFromRPC fetches genesis from an RPC endpoint.
func (f *FetcherAdapter) FetchFromRPC(ctx context.Context, endpoint string) ([]byte, error) {
	destPath := filepath.Join(f.homeDir, "tmp", fmt.Sprintf("genesis-%d.json", time.Now().UnixNano()))

	// Ensure tmp directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, &GenesisError{
			Operation: "fetch_rpc",
			Message:   fmt.Sprintf("failed to create tmp directory: %v", err),
		}
	}

	// Fetch genesis from RPC
	if err := f.fetchGenesisFromRPC(ctx, endpoint, destPath); err != nil {
		return nil, &GenesisError{
			Operation: "fetch_rpc",
			Message:   err.Error(),
		}
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		return nil, &GenesisError{
			Operation: "fetch_rpc",
			Message:   fmt.Sprintf("failed to read fetched genesis: %v", err),
		}
	}

	// Clean up temp file
	os.Remove(destPath)

	return data, nil
}

// fetchGenesisFromRPC fetches genesis from an RPC endpoint and saves to destPath.
func (f *FetcherAdapter) fetchGenesisFromRPC(ctx context.Context, rpcEndpoint, destPath string) error {
	genesis, directErr := f.fetchGenesisDirect(ctx, rpcEndpoint)
	if directErr != nil {
		f.logger.Debug("Direct /genesis fetch failed (%v), trying /genesis_chunked fallback", directErr)
		chunkedGenesis, chunkedErr := f.fetchGenesisChunked(ctx, rpcEndpoint)
		if chunkedErr != nil {
			return fmt.Errorf("direct /genesis failed: %w; /genesis_chunked fallback failed: %v", directErr, chunkedErr)
		}
		genesis = chunkedGenesis
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write genesis file
	if err := os.WriteFile(destPath, genesis, 0o644); err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	return nil
}

func (f *FetcherAdapter) fetchGenesisDirect(ctx context.Context, rpcEndpoint string) ([]byte, error) {
	// Construct genesis endpoint URL
	genesisURL := strings.TrimSuffix(rpcEndpoint, "/") + "/genesis"
	f.logger.Debug("Fetching genesis from %s", genesisURL)

	body, err := f.fetchRPCBody(ctx, genesisURL)
	if err != nil {
		return nil, err
	}

	// Parse the RPC response
	var rpcResponse struct {
		Result struct {
			Genesis json.RawMessage `json:"genesis"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to parse RPC response: %w", err)
	}
	if len(rpcResponse.Result.Genesis) == 0 {
		return nil, fmt.Errorf("genesis response is empty")
	}
	return rpcResponse.Result.Genesis, nil
}

type genesisChunkResponse struct {
	Result struct {
		Chunk json.RawMessage `json:"chunk"`
		Total json.RawMessage `json:"total"`
		Data  string          `json:"data"`
	} `json:"result"`
}

type genesisChunk struct {
	Chunk int
	Total int
	Data  string
}

func (f *FetcherAdapter) fetchGenesisChunked(ctx context.Context, rpcEndpoint string) ([]byte, error) {
	// Fetch chunk 0 first to determine total count.
	first, err := f.fetchGenesisChunk(ctx, rpcEndpoint, 0)
	if err != nil {
		return nil, err
	}
	if first.Total <= 0 {
		return nil, fmt.Errorf("invalid genesis_chunked total: %d", first.Total)
	}

	decodedFirst, err := base64.StdEncoding.DecodeString(first.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode chunk 0: %w", err)
	}

	combined := make([]byte, 0, len(decodedFirst)*first.Total)
	combined = append(combined, decodedFirst...)

	for chunk := 1; chunk < first.Total; chunk++ {
		resp, err := f.fetchGenesisChunk(ctx, rpcEndpoint, chunk)
		if err != nil {
			return nil, err
		}
		if resp.Total != first.Total {
			return nil, fmt.Errorf("chunk %d total mismatch: got %d, expected %d", chunk, resp.Total, first.Total)
		}
		if resp.Chunk != chunk {
			return nil, fmt.Errorf("chunk index mismatch: got %d, expected %d", resp.Chunk, chunk)
		}

		decoded, err := base64.StdEncoding.DecodeString(resp.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode chunk %d: %w", chunk, err)
		}
		combined = append(combined, decoded...)
	}

	// Validate assembled genesis is valid JSON and appears to be a genesis object.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(combined, &probe); err != nil {
		return nil, fmt.Errorf("combined genesis_chunked payload is not valid JSON: %w", err)
	}
	if _, hasChainID := probe["chain_id"]; !hasChainID {
		if _, hasAppState := probe["app_state"]; !hasAppState {
			return nil, fmt.Errorf("combined genesis_chunked payload missing chain_id/app_state")
		}
	}

	return combined, nil
}

func (f *FetcherAdapter) fetchGenesisChunk(ctx context.Context, rpcEndpoint string, chunk int) (*genesisChunk, error) {
	chunkURL := fmt.Sprintf("%s/genesis_chunked?chunk=%d", strings.TrimSuffix(rpcEndpoint, "/"), chunk)
	f.logger.Debug("Fetching genesis chunk %d from %s", chunk, chunkURL)

	body, err := f.fetchRPCBody(ctx, chunkURL)
	if err != nil {
		return nil, err
	}

	var resp genesisChunkResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse genesis_chunked response: %w", err)
	}
	if strings.TrimSpace(resp.Result.Data) == "" {
		return nil, fmt.Errorf("genesis_chunked response is empty for chunk %d", chunk)
	}

	parsedChunk, err := parseChunkedIndex(resp.Result.Chunk, "chunk")
	if err != nil {
		return nil, fmt.Errorf("invalid genesis_chunked chunk field: %w", err)
	}
	parsedTotal, err := parseChunkedIndex(resp.Result.Total, "total")
	if err != nil {
		return nil, fmt.Errorf("invalid genesis_chunked total field: %w", err)
	}

	return &genesisChunk{
		Chunk: parsedChunk,
		Total: parsedTotal,
		Data:  resp.Result.Data,
	}, nil
}

func parseChunkedIndex(raw json.RawMessage, field string) (int, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("%s is missing", field)
	}

	var intValue int
	if err := json.Unmarshal(raw, &intValue); err == nil {
		return intValue, nil
	}

	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err != nil {
		return 0, fmt.Errorf("%s is neither integer nor string", field)
	}

	stringValue = strings.TrimSpace(stringValue)
	if stringValue == "" {
		return 0, fmt.Errorf("%s is empty", field)
	}

	parsedValue, err := strconv.Atoi(stringValue)
	if err != nil {
		return 0, fmt.Errorf("%s is not numeric: %w", field, err)
	}
	return parsedValue, nil
}

func (f *FetcherAdapter) fetchRPCBody(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch genesis: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch genesis: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis response: %w", err)
	}

	return body, nil
}

// ModifyGenesis applies modifications to a genesis file.
func (f *FetcherAdapter) ModifyGenesis(genesis []byte, opts ports.GenesisModifyOptions) ([]byte, error) {
	// Parse genesis
	var genesisMap map[string]interface{}
	if err := json.Unmarshal(genesis, &genesisMap); err != nil {
		return nil, &GenesisError{
			Operation: "modify",
			Message:   fmt.Sprintf("failed to parse genesis: %v", err),
		}
	}

	// Modify chain_id
	if opts.ChainID != "" {
		genesisMap["chain_id"] = opts.ChainID
	}

	// Reset initial_height to 1
	genesisMap["initial_height"] = "1"

	// Modify app_state for governance parameters
	if appState, ok := genesisMap["app_state"].(map[string]interface{}); ok {
		// Modify voting period
		if opts.VotingPeriod > 0 {
			if gov, ok := appState["gov"].(map[string]interface{}); ok {
				if params, ok := gov["params"].(map[string]interface{}); ok {
					params["voting_period"] = fmt.Sprintf("%dns", opts.VotingPeriod.Nanoseconds())
				}
			}
		}

		// Modify unbonding time
		if opts.UnbondingTime > 0 {
			if staking, ok := appState["staking"].(map[string]interface{}); ok {
				if params, ok := staking["params"].(map[string]interface{}); ok {
					params["unbonding_time"] = fmt.Sprintf("%dns", opts.UnbondingTime.Nanoseconds())
				}
			}
		}

		// Modify inflation rate
		if opts.InflationRate != "" {
			if mint, ok := appState["mint"].(map[string]interface{}); ok {
				if minter, ok := mint["minter"].(map[string]interface{}); ok {
					minter["inflation"] = opts.InflationRate
				}
			}
		}
	}

	// Marshal back
	modifiedGenesis, err := json.MarshalIndent(genesisMap, "", "  ")
	if err != nil {
		return nil, &GenesisError{
			Operation: "modify",
			Message:   fmt.Sprintf("failed to marshal genesis: %v", err),
		}
	}

	return modifiedGenesis, nil
}

// Ensure FetcherAdapter implements GenesisFetcher.
var _ ports.GenesisFetcher = (*FetcherAdapter)(nil)
