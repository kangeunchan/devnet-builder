package unit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	snapshotresolver "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin/snapshot"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func httpResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestResolveLatestPolkachuSnapshotURL_MainnetSuccess(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", req.Method)
			}
			return httpResponse(http.StatusOK, `
<html>
  <body>
    <a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_99999999.tar.lz4">latest</a>
  </body>
</html>`), nil
		}),
	}

	got, err := snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		context.Background(),
		"mainnet",
		snapshotresolver.Profile{SnapshotIndexURL: "https://example.com/mainnet"},
		10*time.Second,
		client,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("ResolveLatestPolkachuSnapshotURL returned error: %v", err)
	}
	if got != "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_99999999.tar.lz4" {
		t.Fatalf("unexpected snapshot url: %q", got)
	}
}

func TestResolveLatestPolkachuSnapshotURL_AppliesTimeoutWhenMissingDeadline(t *testing.T) {
	var sawDeadline bool
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_, sawDeadline = req.Context().Deadline()
			return httpResponse(http.StatusOK, `
<a href="https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_11111111.tar.lz4">latest</a>`), nil
		}),
	}

	_, err := snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		context.Background(),
		"testnet",
		snapshotresolver.Profile{SnapshotIndexURL: "https://example.com/testnet"},
		5*time.Second,
		client,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("ResolveLatestPolkachuSnapshotURL returned error: %v", err)
	}
	if !sawDeadline {
		t.Fatal("expected request context to have deadline from timeout")
	}
}

func TestResolveLatestPolkachuSnapshotURL_UnsupportedNetwork(t *testing.T) {
	_, err := snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		context.Background(),
		"devnet",
		snapshotresolver.Profile{SnapshotIndexURL: "https://example.com/devnet"},
		time.Second,
		&http.Client{},
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected unsupported network error")
	}
	if !strings.Contains(err.Error(), "unsupported network type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLatestPolkachuSnapshotURL_InvalidPattern(t *testing.T) {
	_, err := snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		context.Background(),
		"mainnet",
		snapshotresolver.Profile{SnapshotIndexURL: "https://example.com/mainnet"},
		time.Second,
		&http.Client{},
		"[invalid",
		"",
	)
	if err == nil {
		t.Fatal("expected invalid pattern error")
	}
	if !strings.Contains(err.Error(), "compile mainnet snapshot pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLatestPolkachuSnapshotURL_RequestAndMatchErrors(t *testing.T) {
	reqErrClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_ = req
			return nil, errors.New("network down")
		}),
	}

	_, err := snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		context.Background(),
		"mainnet",
		snapshotresolver.Profile{SnapshotIndexURL: "https://example.com/mainnet"},
		time.Second,
		reqErrClient,
		"",
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "request snapshot index") {
		t.Fatalf("expected request error, got: %v", err)
	}

	noMatchClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_ = req
			return httpResponse(http.StatusOK, `<html>no links</html>`), nil
		}),
	}

	_, err = snapshotresolver.ResolveLatestPolkachuSnapshotURL(
		context.Background(),
		"mainnet",
		snapshotresolver.Profile{SnapshotIndexURL: "https://example.com/mainnet"},
		time.Second,
		noMatchClient,
		"",
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "no snapshot URL matched") {
		t.Fatalf("expected no-match error, got: %v", err)
	}
}
