package validation

import "testing"

func TestValidateMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "docker", mode: "docker", wantErr: false},
		{name: "local", mode: "local", wantErr: false},
		{name: "invalid", mode: "k8s", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateMode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateNetworkSource(t *testing.T) {
	tests := []struct {
		name    string
		network string
		wantErr bool
	}{
		{name: "mainnet", network: "mainnet", wantErr: false},
		{name: "testnet", network: "testnet", wantErr: false},
		{name: "invalid", network: "devnet", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkSource(tt.network)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateNetworkSource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateValidatorsForMode(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		validators int
		wantErr    bool
	}{
		{name: "docker valid min", mode: "docker", validators: 1, wantErr: false},
		{name: "docker valid max", mode: "docker", validators: 100, wantErr: false},
		{name: "docker invalid", mode: "docker", validators: 101, wantErr: true},
		{name: "local valid min", mode: "local", validators: 1, wantErr: false},
		{name: "local valid max", mode: "local", validators: 4, wantErr: false},
		{name: "local invalid", mode: "local", validators: 5, wantErr: true},
		{name: "mode invalid", mode: "k8s", validators: 3, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateValidatorsForMode(tt.mode, tt.validators)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateValidatorsForMode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateValidatorsRange(t *testing.T) {
	tests := []struct {
		name       string
		validators int
		min        int
		max        int
		wantErr    bool
	}{
		{name: "in range", validators: 2, min: 1, max: 4, wantErr: false},
		{name: "below min", validators: 0, min: 1, max: 4, wantErr: true},
		{name: "above max", validators: 5, min: 1, max: 4, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateValidatorsRange(tt.validators, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateValidatorsRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAtLeast(t *testing.T) {
	if err := ValidateAtLeast("validators", 1, 1); err != nil {
		t.Fatalf("ValidateAtLeast() unexpected error: %v", err)
	}
	if err := ValidateAtLeast("validators", 0, 1); err == nil {
		t.Fatalf("ValidateAtLeast() expected error for below minimum")
	}
}

func TestValidateNonNegative(t *testing.T) {
	if err := ValidateNonNegative("full-nodes", 0); err != nil {
		t.Fatalf("ValidateNonNegative() unexpected error: %v", err)
	}
	if err := ValidateNonNegative("full-nodes", -1); err == nil {
		t.Fatalf("ValidateNonNegative() expected error for negative value")
	}
}

func TestRequireDaemonConnected(t *testing.T) {
	if err := RequireDaemonConnected(true); err != nil {
		t.Fatalf("RequireDaemonConnected(true) unexpected error: %v", err)
	}
	if err := RequireDaemonConnected(false); err == nil {
		t.Fatalf("RequireDaemonConnected(false) expected error")
	}
}
