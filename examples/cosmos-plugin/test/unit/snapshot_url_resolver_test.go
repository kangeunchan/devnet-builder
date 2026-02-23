package unit

import (
	"io"
	"net/http"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestResolveLatestPolkachuSnapshotURL_Mainnet(t *testing.T) {
<<<<<<<< HEAD:examples/cosmos-plugin/internal/plugin/snapshot_resolver_test.go
	originalClient := httpClient
	t.Cleanup(func() {
		httpClient = originalClient
	})

========
	mainnetIndexURL := "https://www.polkachu.com/tendermint_snapshots/cosmos"
>>>>>>>> 0ac5198 (test(examples/cosmos-plugin): move snapshot url resolver tests to unit suite):examples/cosmos-plugin/test/unit/snapshot_url_resolver_test.go
	mainnetHTML := `<html><body>
	<a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4">latest</a>
	<a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_29999999.tar.lz4">older</a>
	</body></html>`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
<<<<<<<< HEAD:examples/cosmos-plugin/internal/plugin/snapshot_resolver_test.go
			if req.URL.String() != mainnetSnapshotIndexURL {
========
			if req.URL.String() != mainnetIndexURL {
>>>>>>>> 0ac5198 (test(examples/cosmos-plugin): move snapshot url resolver tests to unit suite):examples/cosmos-plugin/test/unit/snapshot_url_resolver_test.go
				t.Fatalf("unexpected URL: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(mainnetHTML)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	got := cosmos.ResolveLatestPolkachuSnapshotURLWithClient("mainnet", client)
	want := "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4"
	if got != want {
		t.Fatalf("ResolveLatestPolkachuSnapshotURLWithClient(mainnet) = %q, want %q", got, want)
	}
}

func TestResolveLatestPolkachuSnapshotURL_TestnetAndNoMatch(t *testing.T) {
	mainnetIndexURL := "https://www.polkachu.com/tendermint_snapshots/cosmos"
	testnetIndexURL := "https://www.polkachu.com/testnets/cosmos/snapshots"

<<<<<<<< HEAD:examples/cosmos-plugin/internal/plugin/snapshot_resolver_test.go
	httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.String() {
			case testnetSnapshotIndexURL:
				body = `<a href="https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4">latest</a>`
			case mainnetSnapshotIndexURL:
========
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.String() {
			case testnetIndexURL:
				body = `<a href="https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4">latest</a>`
			case mainnetIndexURL:
>>>>>>>> 0ac5198 (test(examples/cosmos-plugin): move snapshot url resolver tests to unit suite):examples/cosmos-plugin/test/unit/snapshot_url_resolver_test.go
				body = `<html><body>no snapshot links</body></html>`
			default:
				body = ""
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if got := cosmos.ResolveLatestPolkachuSnapshotURLWithClient("testnet", client); got != "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4" {
		t.Fatalf("ResolveLatestPolkachuSnapshotURLWithClient(testnet) = %q", got)
	}
	if got := cosmos.ResolveLatestPolkachuSnapshotURLWithClient("mainnet", client); got != "" {
		t.Fatalf("ResolveLatestPolkachuSnapshotURLWithClient(mainnet) = %q, want empty", got)
	}
	if got := cosmos.ResolveLatestPolkachuSnapshotURLWithClient("unknown", client); got != "" {
		t.Fatalf("ResolveLatestPolkachuSnapshotURLWithClient(unknown) = %q, want empty", got)
	}
}
