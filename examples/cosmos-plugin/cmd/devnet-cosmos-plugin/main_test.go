package main

import (
	"bytes"
	"strings"
	"testing"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/internal/plugin"
)

func TestRootCmdValidate(t *testing.T) {
	cmd := newRootCmd(cosmos.New())

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := out.String(); got != "ok\n" {
		t.Fatalf("expected output %q, got %q", "ok\n", got)
	}
}

func TestRootCmdValidateRejectsArgs(t *testing.T) {
	cmd := newRootCmd(cosmos.New())

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate", "unexpected"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unexpected argument")
	}

	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error: %v", err)
	}
}
