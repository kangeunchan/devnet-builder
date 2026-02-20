package cosmos

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestResolveLatestPolkachuSnapshotURL_Mainnet(t *testing.T) {
	originalClient := httpClient
	t.Cleanup(func() {
		httpClient = originalClient
	})

	mainnetHTML := `<html><body>
	<a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4">latest</a>
	<a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_29999999.tar.lz4">older</a>
	</body></html>`

	httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != mainnetSnapshotIndexURL {
				t.Fatalf("unexpected URL: %s", req.URL.String())
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(mainnetHTML)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	got := resolveLatestPolkachuSnapshotURL("mainnet")
	want := "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4"
	if got != want {
		t.Fatalf("resolveLatestPolkachuSnapshotURL(mainnet) = %q, want %q", got, want)
	}
}

func TestResolveLatestPolkachuSnapshotURL_TestnetAndNoMatch(t *testing.T) {
	originalClient := httpClient
	t.Cleanup(func() {
		httpClient = originalClient
	})

	httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.String() {
			case testnetSnapshotIndexURL:
				body = `<a href="https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4">latest</a>`
			case mainnetSnapshotIndexURL:
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

	if got := resolveLatestPolkachuSnapshotURL("testnet"); got != "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4" {
		t.Fatalf("resolveLatestPolkachuSnapshotURL(testnet) = %q", got)
	}
	if got := resolveLatestPolkachuSnapshotURL("mainnet"); got != "" {
		t.Fatalf("resolveLatestPolkachuSnapshotURL(mainnet) = %q, want empty", got)
	}
	if got := resolveLatestPolkachuSnapshotURL("unknown"); got != "" {
		t.Fatalf("resolveLatestPolkachuSnapshotURL(unknown) = %q, want empty", got)
	}
}
