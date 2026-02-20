package genesis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractGenesisJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantField string // field that should be present in result
	}{
		{
			name:      "clean genesis output",
			input:     `{"chain_id":"test-1","app_state":{}}`,
			wantErr:   false,
			wantField: "chain_id",
		},
		{
			name:      "genesis with preceding log lines",
			input:     "WARNING: some log message\nINFO: starting export\n" + `{"chain_id":"test-1","app_state":{}}`,
			wantErr:   false,
			wantField: "chain_id",
		},
		{
			name:      "genesis with preceding JSON log line",
			input:     `{"level":"info","msg":"starting export"}` + "\n" + `{"chain_id":"test-1","app_state":{}}`,
			wantErr:   false,
			wantField: "chain_id",
		},
		{
			name:    "only non-genesis JSON",
			input:   `{"level":"info","msg":"export complete"}`,
			wantErr: true,
		},
		{
			name:    "no JSON at all",
			input:   "Some random output\nwith no JSON\n",
			wantErr: true,
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:      "genesis with only app_state (no chain_id at top level)",
			input:     `{"app_state":{"bank":{},"staking":{}}}`,
			wantErr:   false,
			wantField: "app_state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractGenesisJSON([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil with result: %s", string(result))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantField != "" && !strings.Contains(string(result), tt.wantField) {
				t.Errorf("result should contain %q, got: %s", tt.wantField, string(result))
			}
		})
	}
}

func TestFetcherAdapter_FetchFromRPC_DirectFetchFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/genesis" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "mainnet /genesis unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()

	homeDir := t.TempDir()
	fetcher := NewFetcherAdapter(homeDir, "", "", false, nil)

	_, err := fetcher.FetchFromRPC(context.Background(), server.URL)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetcherAdapter_FetchFromRPC_WritesAndCleansTempFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/genesis" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":-1,"result":{"genesis":{"chain_id":"provider","app_state":{}}}}`))
	}))
	defer server.Close()

	homeDir := t.TempDir()
	fetcher := NewFetcherAdapter(homeDir, "", "", false, nil)

	got, err := fetcher.FetchFromRPC(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchFromRPC returned error: %v", err)
	}
	if !strings.Contains(string(got), `"chain_id":"provider"`) {
		t.Fatalf("unexpected fetched genesis: %s", string(got))
	}

	tmpDir := filepath.Join(homeDir, "tmp")
	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("failed to read tmp dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected temp genesis files to be cleaned up, found %d file(s)", len(entries))
	}
}
