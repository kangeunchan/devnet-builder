package stateexport

import (
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
)

func TestDefaultExportOptions_ForZeroHeightEnabled(t *testing.T) {
	adapter := NewAdapter(t.TempDir(), nil)
	opts := adapter.DefaultExportOptions()
	if opts == nil {
		t.Fatalf("DefaultExportOptions returned nil")
	}
	if !opts.ForZeroHeight {
		t.Fatalf("expected ForZeroHeight=true by default")
	}
}

func TestBuildExportCommand_IncludesModulesToExport(t *testing.T) {
	adapter := NewAdapter(t.TempDir(), nil)
	args := adapter.buildExportCommand("/tmp/node0", adapter.DefaultExportOptions())
	if !containsToken(args, "--for-zero-height") {
		t.Fatalf("expected --for-zero-height in args: %#v", args)
	}

	args = adapter.buildExportCommand("/tmp/node0", &ports.ExportOptions{
		ForZeroHeight:   true,
		ModulesToExport: []string{"auth", "bank", "staking"},
	})

	idx := indexOfToken(args, "--modules-to-export")
	if idx < 0 || idx+1 >= len(args) {
		t.Fatalf("expected --modules-to-export argument, got %#v", args)
	}
	if args[idx+1] != "auth,bank,staking" {
		t.Fatalf("unexpected modules-to-export value: %q", args[idx+1])
	}
}

func TestBuildExportCommand_WithoutModulesToExportSkipsFlag(t *testing.T) {
	adapter := NewAdapter(t.TempDir(), nil)
	args := adapter.buildExportCommand("/tmp/node0", &ports.ExportOptions{
		ForZeroHeight:   true,
		ModulesToExport: nil,
	})
	if containsToken(args, "--modules-to-export") {
		t.Fatalf("did not expect --modules-to-export, got %#v", args)
	}
}

func TestBuildExportCommand_MapsHeightAndJailWhitelist(t *testing.T) {
	adapter := NewAdapter(t.TempDir(), nil)
	args := adapter.buildExportCommand("/tmp/node0", &ports.ExportOptions{
		ForZeroHeight: true,
		Height:        123,
		JailWhitelist: []string{"cosmosvaloper1a", "cosmosvaloper1b"},
	})

	heightIdx := indexOfToken(args, "--height")
	if heightIdx < 0 || heightIdx+1 >= len(args) || args[heightIdx+1] != "123" {
		t.Fatalf("unexpected --height args: %#v", args)
	}
	if countToken(args, "--jail-allowed-addrs") != 2 {
		t.Fatalf("expected two --jail-allowed-addrs flags, got %#v", args)
	}
}

func TestConvertToPkgExportOptions_MapsModulesToExport(t *testing.T) {
	pkgOpts := convertToPkgExportOptions(nil)
	if !pkgOpts.ForZeroHeight {
		t.Fatalf("expected nil options to default ForZeroHeight=true")
	}

	pkgOpts = convertToPkgExportOptions(&ports.ExportOptions{
		ForZeroHeight:   true,
		ModulesToExport: []string{"auth", "bank"},
	})
	if strings.Join(pkgOpts.ModulesToExport, ",") != "auth,bank" {
		t.Fatalf("expected modules to be mapped, got %#v", pkgOpts.ModulesToExport)
	}
}

func TestConvertToPkgExportOptions_MapsAllFields(t *testing.T) {
	pkgOpts := convertToPkgExportOptions(&ports.ExportOptions{
		ForZeroHeight:   true,
		ModulesToExport: []string{"auth"},
		ModulesToSkip:   []string{"mint"},
		JailWhitelist:   []string{"cosmosvaloper1x"},
		Height:          77,
		OutputPath:      "/tmp/genesis.json",
	})
	if !pkgOpts.ForZeroHeight {
		t.Fatalf("expected ForZeroHeight=true")
	}
	if strings.Join(pkgOpts.ModulesToExport, ",") != "auth" {
		t.Fatalf("unexpected ModulesToExport: %#v", pkgOpts.ModulesToExport)
	}
	if strings.Join(pkgOpts.ModulesToSkip, ",") != "mint" {
		t.Fatalf("unexpected ModulesToSkip: %#v", pkgOpts.ModulesToSkip)
	}
	if strings.Join(pkgOpts.JailWhitelist, ",") != "cosmosvaloper1x" {
		t.Fatalf("unexpected JailWhitelist: %#v", pkgOpts.JailWhitelist)
	}
	if pkgOpts.Height != 77 {
		t.Fatalf("unexpected Height: %d", pkgOpts.Height)
	}
	if pkgOpts.OutputPath != "/tmp/genesis.json" {
		t.Fatalf("unexpected OutputPath: %q", pkgOpts.OutputPath)
	}
}

func containsToken(args []string, token string) bool {
	return indexOfToken(args, token) >= 0
}

func indexOfToken(args []string, token string) int {
	for i := range args {
		if args[i] == token {
			return i
		}
	}
	return -1
}

func countToken(args []string, token string) int {
	count := 0
	for _, arg := range args {
		if arg == token {
			count++
		}
	}
	return count
}
