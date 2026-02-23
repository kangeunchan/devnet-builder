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
	mainnetIndexURL := "https://www.polkachu.com/tendermint_snapshots/cosmos"
	mainnetHTML := `<html><body>
	<a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4">latest</a>
	<a href="https://snapshots.polkachu.com/snapshots/cosmos/cosmos_29999999.tar.lz4">older</a>
	</body></html>`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != mainnetIndexURL {
				t.Fatalf("unexpected URL: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(mainnetHTML)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	networkModule := cosmos.New(cosmos.WithSnapshotHTTPClient(client))
	got := networkModule.SnapshotURL("mainnet")
	want := "https://snapshots.polkachu.com/snapshots/cosmos/cosmos_30000001.tar.lz4"
	if got != want {
		t.Fatalf("SnapshotURL(mainnet) = %q, want %q", got, want)
	}
}

func TestResolveLatestPolkachuSnapshotURL_TestnetAndNoMatch(t *testing.T) {
	mainnetIndexURL := "https://www.polkachu.com/tendermint_snapshots/cosmos"
	testnetIndexURL := "https://www.polkachu.com/testnets/cosmos/snapshots"

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch req.URL.String() {
			case testnetIndexURL:
				body = `<a href="https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4">latest</a>`
			case mainnetIndexURL:
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

	networkModule := cosmos.New(cosmos.WithSnapshotHTTPClient(client))
	if got := networkModule.SnapshotURL("testnet"); got != "https://snapshots.polkachu.com/testnet-snapshots/cosmos/cosmos_16000000.tar.lz4" {
		t.Fatalf("SnapshotURL(testnet) = %q", got)
	}
	if got := networkModule.SnapshotURL("mainnet"); got != "" {
		t.Fatalf("SnapshotURL(mainnet) = %q, want empty", got)
	}
	if got := networkModule.SnapshotURL("unknown"); got != "" {
		t.Fatalf("SnapshotURL(unknown) = %q, want empty", got)
	}
}
