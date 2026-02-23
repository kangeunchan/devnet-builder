package main

import (
	"fmt"
	"log"

	cosmos "github.com/altuslabsxyz/devnet-builder/examples/cosmos-plugin/plugin"
)

func main() {
	networkModule := cosmos.New(
		cosmos.WithCustomizationFile("examples/cosmos-plugin/customization/example.yaml"),
	)

	if err := networkModule.Validate(); err != nil {
		log.Fatalf("invalid customization: %v", err)
	}

	fmt.Printf("mainnet rpc: %s\n", networkModule.RPCEndpoint("mainnet"))
	fmt.Printf("mainnet snapshot: %s\n", networkModule.SnapshotURL("mainnet"))
}
