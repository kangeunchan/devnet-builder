package network

import (
	"sort"
	"sync"
)

// DefaultNetworkName is empty - networks are loaded dynamically via plugins.
const DefaultNetworkName = ""

// Registry defines instance-scoped network registry operations.
// Callers should depend on this interface instead of package globals.
type Registry interface {
	Register(module NetworkModule) error
	MustRegister(module NetworkModule, panicOnError bool) error
	Get(name string) (NetworkModule, error)
	Has(name string) bool
	List() []string
	ListModules() []NetworkModule
	Default() (NetworkModule, error)
	SetDefault(name string) error
}

// Global registry instance
//
// Deprecated: Global registry is provided for backward compatibility.
// New code should use the NetworkRegistry wrapper from internal/di package
// for dependency injection. This allows for better testability and
// explicit dependency management.
var (
	globalRegistry = newRegistry()
)

// NewRegistry creates a new registry instance.
// Use this when you need an independent registry (e.g., for testing).
func NewRegistry() Registry {
	return newRegistry()
}

// GlobalRegistry returns the package-level registry instance.
// This exists for backward compatibility and explicit wiring into DI.
func GlobalRegistry() Registry {
	return globalRegistry
}

// registry holds registered network modules.
type registry struct {
	mu       sync.RWMutex
	modules  map[string]NetworkModule
	defaults string // default network name
}

// newRegistry creates a new registry instance.
func newRegistry() *registry {
	return &registry{
		modules:  make(map[string]NetworkModule),
		defaults: DefaultNetworkName,
	}
}

// Register adds a network module to the global registry.
// This function is typically called from init() in network module packages.
// It panics if:
//   - A module with the same name is already registered
//   - The module fails validation
func Register(module NetworkModule) {
	if err := globalRegistry.Register(module); err != nil {
		panic(err)
	}
}

// MustRegister is like Register but allows specifying whether to panic on error.
// If panicOnError is false, errors are silently ignored.
func MustRegister(module NetworkModule, panicOnError bool) error {
	return globalRegistry.MustRegister(module, panicOnError)
}

// Get retrieves a network module by name from the global registry.
// Returns an error if the network is not registered.
func Get(name string) (NetworkModule, error) {
	return globalRegistry.Get(name)
}

// MustGet retrieves a network module by name, panicking if not found.
func MustGet(name string) NetworkModule {
	m, err := Get(name)
	if err != nil {
		panic(err)
	}
	return m
}

// Has checks if a network is registered.
func Has(name string) bool {
	return globalRegistry.Has(name)
}

// List returns all registered network names in sorted order.
func List() []string {
	return globalRegistry.List()
}

// ListModules returns all registered network modules.
func ListModules() []NetworkModule {
	return globalRegistry.ListModules()
}

// Default returns the default network module ("stable").
// Returns an error if the default network is not registered.
func Default() (NetworkModule, error) {
	return globalRegistry.Default()
}

// SetDefault changes the default network name.
// Returns an error if the network is not registered.
func SetDefault(name string) error {
	return globalRegistry.SetDefault(name)
}

// Registry methods

// Register adds a module to this registry instance.
func (r *registry) Register(module NetworkModule) error {
	return r.register(module)
}

// MustRegister adds a module and optionally panics on error.
func (r *registry) MustRegister(module NetworkModule, panicOnError bool) error {
	err := r.register(module)
	if err != nil && panicOnError {
		panic(err)
	}
	return err
}

// Get returns a module by name from this registry instance.
func (r *registry) Get(name string) (NetworkModule, error) {
	return r.get(name)
}

// Has reports whether a module exists in this registry instance.
func (r *registry) Has(name string) bool {
	return r.has(name)
}

// List returns all module names in this registry instance.
func (r *registry) List() []string {
	return r.list()
}

// ListModules returns all modules in this registry instance.
func (r *registry) ListModules() []NetworkModule {
	return r.listModules()
}

// Default returns the default module in this registry instance.
func (r *registry) Default() (NetworkModule, error) {
	return r.defaults_()
}

// SetDefault changes the default module name in this registry instance.
func (r *registry) SetDefault(name string) error {
	return r.setDefault(name)
}

func (r *registry) register(module NetworkModule) error {
	if module == nil {
		return &ModuleValidationError{
			ModuleName: "<nil>",
			Reason:     "cannot register nil module",
		}
	}

	// Validate module before registration
	if err := ValidateModuleCompatibility(module); err != nil {
		return err
	}

	name := module.Name()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.modules[name]; exists {
		return &DuplicateNetworkError{NetworkName: name}
	}

	r.modules[name] = module
	return nil
}

func (r *registry) get(name string) (NetworkModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[name]
	if !ok {
		return nil, &UnknownNetworkError{
			RequestedNetwork:  name,
			AvailableNetworks: r.listLocked(),
		}
	}
	return module, nil
}

func (r *registry) has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.modules[name]
	return ok
}

func (r *registry) list() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listLocked()
}

func (r *registry) listLocked() []string {
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *registry) listModules() []NetworkModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	modules := make([]NetworkModule, 0, len(r.modules))
	names := r.listLocked()
	for _, name := range names {
		modules = append(modules, r.modules[name])
	}
	return modules
}

func (r *registry) defaults_() (NetworkModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaults == "" {
		return nil, ErrNoDefaultNetwork
	}

	module, ok := r.modules[r.defaults]
	if !ok {
		// Default is not registered yet - this is not an error during init
		// as modules may be registered in any order
		return nil, &UnknownNetworkError{
			RequestedNetwork:  r.defaults,
			AvailableNetworks: r.listLocked(),
		}
	}
	return module, nil
}

func (r *registry) setDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.modules[name]; !ok {
		return &UnknownNetworkError{
			RequestedNetwork:  name,
			AvailableNetworks: r.listLocked(),
		}
	}
	r.defaults = name
	return nil
}

func (r *registry) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules = make(map[string]NetworkModule)
	r.defaults = DefaultNetworkName
}

// ResetRegistry clears all registered modules. This is primarily for testing.
func ResetRegistry() {
	globalRegistry.reset()
}
