package cosmos

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/altuslabsxyz/devnet-builder/pkg/network"
)

// ModifyGenesis applies genesis mutations through the same streaming pipeline used
// by ModifyGenesisFile to keep behavior identical across both code paths.
func (n *CosmosNetwork) ModifyGenesis(genesis []byte, opts network.GenesisOptions) ([]byte, error) {
	if err := validateGenesisOptions(opts); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(genesis))
	var streamOut bytes.Buffer
	if err := n.streamModifyGenesis(dec, &streamOut, opts); err != nil {
		return nil, fmt.Errorf("failed to modify genesis: %w", err)
	}

	var normalized map[string]any
	if err := json.Unmarshal(streamOut.Bytes(), &normalized); err != nil {
		return nil, fmt.Errorf("failed to parse modified genesis: %w", err)
	}

	out, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified genesis: %w", err)
	}
	return out, nil
}

// ModifyGenesisFile handles large genesis files using file paths.
// It avoids keeping both raw input bytes and output bytes in memory simultaneously.
func (n *CosmosNetwork) ModifyGenesisFile(inputPath, outputPath string, opts network.GenesisOptions) (int64, error) {
	if err := validateGenesisOptions(opts); err != nil {
		return 0, err
	}

	in, err := os.Open(inputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open input genesis %q: %w", inputPath, err)
	}
	defer in.Close()

	dec := json.NewDecoder(bufio.NewReader(in))

	tmpPath := outputPath + ".tmp"
	outFile, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output temp file %q: %w", tmpPath, err)
	}

	bufw := bufio.NewWriter(outFile)
	if err := n.streamModifyGenesis(dec, bufw, opts); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to stream-modify genesis %q: %w", inputPath, err)
	}
	if err := bufw.Flush(); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to flush modified genesis %q: %w", tmpPath, err)
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to close output file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to move %q to %q: %w", tmpPath, outputPath, err)
	}

	st, err := os.Stat(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat output genesis %q: %w", outputPath, err)
	}

	return st.Size(), nil
}
