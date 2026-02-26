package manage

import (
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/interactive"
	"github.com/altuslabsxyz/devnet-builder/types"
)

type upgradeFlagSnapshot struct {
	upgradeName          string
	upgradeImage         string
	upgradeBinary        string
	upgradeMode          string
	upgradeNoInteractive bool
	upgradeVersion       string
	skipGovernance       bool
}

func snapshotUpgradeFlags() upgradeFlagSnapshot {
	return upgradeFlagSnapshot{
		upgradeName:          upgradeName,
		upgradeImage:         upgradeImage,
		upgradeBinary:        upgradeBinary,
		upgradeMode:          upgradeMode,
		upgradeNoInteractive: upgradeNoInteractive,
		upgradeVersion:       upgradeVersion,
		skipGovernance:       skipGovernance,
	}
}

func restoreUpgradeFlags(s upgradeFlagSnapshot) {
	upgradeName = s.upgradeName
	upgradeImage = s.upgradeImage
	upgradeBinary = s.upgradeBinary
	upgradeMode = s.upgradeMode
	upgradeNoInteractive = s.upgradeNoInteractive
	upgradeVersion = s.upgradeVersion
	skipGovernance = s.skipGovernance
}

func TestResolveUpgradeExecutionMode(t *testing.T) {
	snapshot := snapshotUpgradeFlags()
	t.Cleanup(func() { restoreUpgradeFlags(snapshot) })

	upgradeMode = ""
	mode, explicit, err := resolveUpgradeExecutionMode(types.ExecutionModeLocal)
	if err != nil {
		t.Fatalf("resolveUpgradeExecutionMode returned error: %v", err)
	}
	if mode != UpgradeModeLocal {
		t.Fatalf("expected mode %q, got %q", UpgradeModeLocal, mode)
	}
	if explicit {
		t.Fatalf("expected explicit=false, got true")
	}

	upgradeMode = "docker"
	mode, explicit, err = resolveUpgradeExecutionMode(types.ExecutionModeLocal)
	if err != nil {
		t.Fatalf("resolveUpgradeExecutionMode returned error: %v", err)
	}
	if mode != UpgradeModeDocker {
		t.Fatalf("expected mode %q, got %q", UpgradeModeDocker, mode)
	}
	if !explicit {
		t.Fatalf("expected explicit=true, got false")
	}

	upgradeMode = "invalid"
	_, _, err = resolveUpgradeExecutionMode(types.ExecutionModeLocal)
	if err == nil {
		t.Fatalf("expected error for invalid mode")
	}
}

func TestShouldRunUpgradeInteractiveSelection(t *testing.T) {
	snapshot := snapshotUpgradeFlags()
	t.Cleanup(func() { restoreUpgradeFlags(snapshot) })

	upgradeNoInteractive = false
	upgradeImage = ""
	upgradeBinary = ""
	if !shouldRunUpgradeInteractiveSelection(false) {
		t.Fatalf("expected interactive selection to run")
	}

	if shouldRunUpgradeInteractiveSelection(true) {
		t.Fatalf("expected interactive selection to be disabled in json mode")
	}

	upgradeNoInteractive = true
	if shouldRunUpgradeInteractiveSelection(false) {
		t.Fatalf("expected interactive selection to be disabled with --no-interactive")
	}
}

func TestResolveUpgradeName(t *testing.T) {
	snapshot := snapshotUpgradeFlags()
	t.Cleanup(func() { restoreUpgradeFlags(snapshot) })

	selection := &interactive.SelectionConfig{
		StartVersion:     "feat/gas-waiver",
		StartIsCustomRef: true,
		UpgradeName:      "prompt-name",
	}

	upgradeName = "flag-name"
	if got := resolveUpgradeName(selection); got != "flag-name" {
		t.Fatalf("expected flag name, got %q", got)
	}

	upgradeName = ""
	if got := resolveUpgradeName(selection); got != "prompt-name" {
		t.Fatalf("expected prompt name, got %q", got)
	}

	selection.UpgradeName = ""
	if got := resolveUpgradeName(selection); got != "gas-waiver-upgrade" {
		t.Fatalf("expected derived name, got %q", got)
	}
}

func TestValidateUpgradeSourceInputs(t *testing.T) {
	snapshot := snapshotUpgradeFlags()
	t.Cleanup(func() { restoreUpgradeFlags(snapshot) })

	upgradeBinary = "/tmp/custom"
	if err := validateUpgradeSourceInputs("v1.0.0", "upgrade-1"); err == nil {
		t.Fatalf("expected deprecated binary flag error")
	}

	upgradeBinary = ""
	upgradeImage = ""
	skipGovernance = false
	if err := validateUpgradeSourceInputs("", "upgrade-1"); err == nil {
		t.Fatalf("expected source validation error")
	}

	if err := validateUpgradeSourceInputs("v1.0.0", ""); err == nil {
		t.Fatalf("expected name validation error")
	}

	skipGovernance = true
	if err := validateUpgradeSourceInputs("v1.0.0", ""); err != nil {
		t.Fatalf("expected skip-gov to allow empty name, got error: %v", err)
	}
}

func TestBuildUpgradeTargets(t *testing.T) {
	snapshot := snapshotUpgradeFlags()
	t.Cleanup(func() { restoreUpgradeFlags(snapshot) })

	upgradeBinary = ""
	upgradeImage = "img:old"
	binary, image := buildUpgradeTargets("/tmp/selected", "")
	if binary != "/tmp/selected" {
		t.Fatalf("expected selected binary path, got %q", binary)
	}
	if image != "img:old" {
		t.Fatalf("expected original image, got %q", image)
	}

	binary, image = buildUpgradeTargets("", "img:resolved")
	if binary != "" {
		t.Fatalf("expected empty binary, got %q", binary)
	}
	if image != "img:resolved" {
		t.Fatalf("expected resolved image, got %q", image)
	}

	upgradeBinary = "/tmp/deprecated"
	binary, _ = buildUpgradeTargets("", "")
	if !strings.Contains(binary, "/tmp/deprecated") {
		t.Fatalf("expected fallback to raw binary path, got %q", binary)
	}
}
