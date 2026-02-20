package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/go-version"

	"github.com/altuslabsxyz/devnet-builder/cmd/devnet-builder/commands"
	"github.com/altuslabsxyz/devnet-builder/internal"
	"github.com/altuslabsxyz/devnet-builder/internal/di"
	domainversion "github.com/altuslabsxyz/devnet-builder/internal/domain/version"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/network"
	"github.com/altuslabsxyz/devnet-builder/internal/infrastructure/version/migrations"
	"github.com/altuslabsxyz/devnet-builder/internal/output"
	"github.com/altuslabsxyz/devnet-builder/pkg/network/plugin"
)

// Global plugin loader - shared across the application
var globalLoader *plugin.Loader

const defaultMigrationTargetVersion = "1.0.0"

func main() {
	// Enable color output
	color.NoColor = false

	// Load plugins from ~/.devnet-builder/plugins/
	globalLoader = plugin.NewLoader()

	// Load all discovered plugins with detailed error information
	loadResult, loadErr := globalLoader.LoadAllWithErrors()
	if loadErr != nil {
		output.DefaultLogger.Debug("Plugin loading error: %v", loadErr)
	}

	// Log which plugins were successfully loaded
	if loadResult != nil {
		loadedNames := make([]string, 0, len(loadResult.Loaded))
		for _, p := range loadResult.Loaded {
			loadedNames = append(loadedNames, p.Name())
		}
		output.DefaultLogger.Debug("Successfully loaded %d plugins: %v", len(loadResult.Loaded), loadedNames)

		// Log detailed errors for plugins that failed to load
		for _, loadErr := range loadResult.Errors {
			output.DefaultLogger.Warn("Failed to load plugin %q: %v", loadErr.PluginName, loadErr.Err)
		}
	}

	// Extract plugins from result for registration
	var plugins []*plugin.PluginClient
	if loadResult != nil {
		plugins = loadResult.Loaded
	}

	// Register loaded plugins with the network registry
	for _, p := range plugins {
		// Create an adapter to convert pkg/network.Module to internal/network.NetworkModule
		adapter := newPluginAdapter(p.Module())
		if err := network.MustRegister(adapter, false); err != nil {
			output.DefaultLogger.Warn("Failed to register plugin %q: %v", p.Name(), err)
		}
	}

	// Check and migrate version before executing commands
	// Respect DEVNET_HOME env var before flag parsing (matches behavior in root.go)
	homeDir := commands.DefaultHomeDir()
	if envHome := os.Getenv("DEVNET_HOME"); envHome != "" {
		homeDir = envHome
	}
	if err := checkAndMigrateVersion(homeDir); err != nil {
		fmt.Fprintf(os.Stderr, "Version migration failed: %v\n", err)
		globalLoader.Close()
		os.Exit(1)
	}

	// Initialize root command from commands package
	rootCmd := commands.NewRootCmd()

	// Enhance root command with binary passthrough commands
	// This is done after root command initialization but before execution
	// We pass a nil container here since the container will be initialized
	// lazily when commands are executed
	if err := enhanceRootWithBinaryPassthrough(rootCmd, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup binary passthrough: %v\n", err)
		os.Exit(1)
	}

	err := rootCmd.Execute()

	// Always close plugins before exit (os.Exit skips defers)
	globalLoader.Close()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// GetPluginLoader returns the global plugin loader.
// This is used by commands to access the plugin system.
func GetPluginLoader() *plugin.Loader {
	return globalLoader
}

// checkAndMigrateVersion checks the current version and applies migrations if needed.
func checkAndMigrateVersion(homeDir string) error {
	// Use default logger for migration
	logger := output.DefaultLogger

	// Create infrastructure factory
	factory := di.NewInfrastructureFactory(homeDir, logger)

	// Create migration service
	migrationSvc := factory.CreateMigrationService()

	// Register all migrations
	registered := []domainversion.Migration{
		migrations.NewCacheKeyMigration(),
		migrations.NewNoOpMigration(),
		migrations.NewV001ToV100Migration(),
		migrations.NewV010ToV100Migration(),
		migrations.NewV010DevToV100Migration(),
	}
	for _, m := range registered {
		migrationSvc.RegisterMigration(m)
	}

	targetVersion := resolveMigrationTargetVersion(internal.Version, registered, logger)

	// Check and migrate to current version
	ctx := context.Background()
	_, err := migrationSvc.CheckAndMigrate(ctx, homeDir, targetVersion)
	if err != nil {
		return fmt.Errorf("failed to migrate to version %s (build version %s): %w", targetVersion, internal.Version, err)
	}

	return nil
}

func resolveMigrationTargetVersion(buildVersion string, registered []domainversion.Migration, logger *output.Logger) string {
	versionString := strings.TrimSpace(buildVersion)
	if versionString == "" {
		if logger != nil {
			logger.Warn("Build version is empty; using migration target %s", defaultMigrationTargetVersion)
		}
		return defaultMigrationTargetVersion
	}

	if isSemverVersion(versionString) {
		return versionString
	}

	fallback := latestMigrationTargetVersion(registered)
	if fallback == "" {
		fallback = defaultMigrationTargetVersion
	}

	if logger != nil {
		logger.Warn("Build version %q is not semver-compatible; using migration target %q", versionString, fallback)
	}

	return fallback
}

func isSemverVersion(v string) bool {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return false
	}

	core := strings.TrimPrefix(trimmed, "v")
	if strings.Count(core, ".") < 2 {
		return false
	}

	if _, err := version.NewSemver(trimmed); err == nil {
		return true
	}

	if !strings.HasPrefix(trimmed, "v") {
		if _, err := version.NewSemver("v" + trimmed); err == nil {
			return true
		}
	}

	return false
}

func latestMigrationTargetVersion(registered []domainversion.Migration) string {
	var latestParsed *version.Version
	latestRaw := ""

	for _, m := range registered {
		candidate := strings.TrimSpace(m.ToVersion())
		if candidate == "" {
			continue
		}

		parsed, err := version.NewSemver(candidate)
		if err != nil {
			continue
		}

		if latestParsed == nil || latestParsed.LessThan(parsed) {
			latestParsed = parsed
			latestRaw = candidate
		}
	}

	return latestRaw
}
