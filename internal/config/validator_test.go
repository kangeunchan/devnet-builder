package config

import "testing"

func TestValidateValidatorCount(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		validators int
		wantErr    bool
	}{
		{name: "docker min", mode: "docker", validators: 1, wantErr: false},
		{name: "docker max", mode: "docker", validators: 100, wantErr: false},
		{name: "docker below min", mode: "docker", validators: 0, wantErr: true},
		{name: "docker above max", mode: "docker", validators: 101, wantErr: true},
		{name: "local min", mode: "local", validators: 1, wantErr: false},
		{name: "local max", mode: "local", validators: 4, wantErr: false},
		{name: "local below min", mode: "local", validators: 0, wantErr: true},
		{name: "local above max", mode: "local", validators: 5, wantErr: true},
		{name: "empty mode defaults docker", mode: "", validators: 10, wantErr: false},
		{name: "invalid mode", mode: "kubernetes", validators: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateValidatorCount(tt.mode, tt.validators)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateValidatorCount(%q, %d) error = %v, wantErr %v", tt.mode, tt.validators, err, tt.wantErr)
			}
		})
	}
}

func TestValidatorCountRangeForMode(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		wantMin      int
		wantMax      int
		wantResolved string
		wantErr      bool
	}{
		{name: "docker", mode: "docker", wantMin: 1, wantMax: 100, wantResolved: "docker"},
		{name: "local", mode: "local", wantMin: 1, wantMax: 4, wantResolved: "local"},
		{name: "empty defaults docker", mode: "", wantMin: 1, wantMax: 100, wantResolved: "docker"},
		{name: "invalid", mode: "k8s", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			min, max, resolvedMode, err := ValidatorCountRangeForMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatorCountRangeForMode(%q) error = %v, wantErr %v", tt.mode, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if min != tt.wantMin || max != tt.wantMax || resolvedMode != tt.wantResolved {
				t.Fatalf("ValidatorCountRangeForMode(%q) = (%d, %d, %q), want (%d, %d, %q)",
					tt.mode, min, max, resolvedMode, tt.wantMin, tt.wantMax, tt.wantResolved)
			}
		})
	}
}
