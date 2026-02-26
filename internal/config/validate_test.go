package config

import (
	"strings"
	"testing"

	"github.com/altuslabsxyz/devnet-builder/types"
)

func TestValidateFileConfig_ValidatorCountByMode(t *testing.T) {
	docker := types.ExecutionModeDocker
	local := types.ExecutionModeLocal

	tests := []struct {
		name       string
		cfg        *FileConfig
		wantErr    bool
		errContain string
	}{
		{
			name: "docker allows 100",
			cfg: &FileConfig{
				ExecutionMode: &docker,
				Validators:    intPtr(100),
			},
			wantErr: false,
		},
		{
			name: "local rejects above 4",
			cfg: &FileConfig{
				ExecutionMode: &local,
				Validators:    intPtr(5),
			},
			wantErr:    true,
			errContain: "1-4 for local mode",
		},
		{
			name: "empty mode defaults docker",
			cfg: &FileConfig{
				Validators: intPtr(50),
			},
			wantErr: false,
		},
		{
			name: "invalid mode rejected",
			cfg: &FileConfig{
				ExecutionMode: executionModePtr("k8s"),
				Validators:    intPtr(2),
			},
			wantErr:    true,
			errContain: "invalid mode in config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateFileConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("expected error to contain %q, got %q", tt.errContain, err.Error())
			}
		})
	}
}

func TestEffectiveConfigValidate_ValidatorCountByMode(t *testing.T) {
	cfg := NewEffectiveConfig("/tmp")
	cfg.BlockchainNetwork = NewStringValue("")
	cfg.Mode = NewStringValue("docker")
	cfg.Validators = NewIntValue(100)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should pass for docker validators=100: %v", err)
	}

	cfg.Mode = NewStringValue("local")
	cfg.Validators = NewIntValue(5)
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() should fail for local validators=5")
	}
}

func intPtr(v int) *int {
	return &v
}

func executionModePtr(v string) *types.ExecutionMode {
	mode := types.ExecutionMode(v)
	return &mode
}
