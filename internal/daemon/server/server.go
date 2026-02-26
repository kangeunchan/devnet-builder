// internal/daemon/server/server.go
package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	v1 "github.com/altuslabsxyz/devnet-builder/api/proto/gen/v1"
	"github.com/altuslabsxyz/devnet-builder/internal/application/ports"
	"github.com/altuslabsxyz/devnet-builder/internal/auth"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/checker"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/controller"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/provisioner"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/runtime"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/server/ante"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/store"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/subnet"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/types"
	"github.com/altuslabsxyz/devnet-builder/internal/daemon/upgrader"
	"go.uber.org/multierr"
	"google.golang.org/grpc"
)

// Config holds server configuration.
type Config struct {
	// SocketPath is the Unix socket path.
	SocketPath string
	// DataDir is the data directory.
	DataDir string
	// Foreground runs in foreground (don't daemonize).
	Foreground bool
	// Workers is the number of workers per controller.
	Workers int
	// LogLevel is the log level (debug, info, warn, error).
	LogLevel string
	// RuntimeMode selects the node process runtime: "process" (default), "service", "docker".
	RuntimeMode string
	// EnableDocker enables Docker container runtime for nodes.
	EnableDocker bool
	// DockerImage is the default Docker image for nodes.
	DockerImage string
	// ShutdownTimeout is the graceful shutdown timeout.
	ShutdownTimeout time.Duration
	// HealthCheckTimeout is the RPC health check timeout.
	HealthCheckTimeout time.Duration
	// GitHubToken is the GitHub API token.
	GitHubToken string

	// Remote listener settings (optional - enables remote access)
	// Listen is the TCP address to listen on (e.g., "0.0.0.0:9000").
	// Empty means local-only mode (Unix socket only).
	Listen string
	// TLSCert is the path to the TLS certificate file.
	TLSCert string
	// TLSKey is the path to the TLS private key file.
	TLSKey string

	// Authentication settings
	// AuthEnabled enables API key authentication for remote connections.
	AuthEnabled bool
	// AuthKeysFile is the path to the API keys file.
	AuthKeysFile string
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".devnet-builder")
	return &Config{
		SocketPath:         filepath.Join(dataDir, "devnetd.sock"),
		DataDir:            dataDir,
		Foreground:         false,
		Workers:            2,
		LogLevel:           "info",
		ShutdownTimeout:    30 * time.Second,
		HealthCheckTimeout: 5 * time.Second,
		GitHubToken:        "",
	}
}

// Server is the devnetd daemon server.
type Server struct {
	config          *Config
	store           store.Store
	manager         *controller.Manager
	healthCtrl      *controller.HealthController
	pluginManager   *PluginManager
	subnetAllocator *subnet.Allocator
	nodeRuntime     runtime.NodeRuntime // Node runtime for process management
	grpcServer      *grpc.Server
	listener        net.Listener // Unix socket listener
	tcpListener     net.Listener // TCP/TLS listener (optional)
	logger          *slog.Logger
	logFile         *os.File // Log file handle for cleanup

	// shutdownCtx is cancelled during server shutdown to terminate long-running
	// streaming RPCs (like log streaming) that would otherwise block GracefulStop.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

type ServerBuilder struct {
	config *Config
	server *Server

	orchFactory *OrchestratorFactory
	devnetProv  *provisioner.DevnetProvisioner

	cleanupStack []func() error
	err          error
}

// NewServerBuilder creates a new server builder.
func NewServerBuilder(config *Config) *ServerBuilder {
	return &ServerBuilder{
		config: config,
		server: &Server{config: config},
	}
}

// New creates a new server.
func New(config *Config) (*Server, error) {
	return NewServerBuilder(config).
		WithDataDir().
		WithLogger().
		WithPlugins().
		WithStoreAndSubnet().
		WithControllers().
		WithRuntime().
		WithGRPCServices().
		Build()
}

// WithDataDir ensures the data directory exists.
func (b *ServerBuilder) WithDataDir() *ServerBuilder {
	if b.err != nil {
		return b
	}

	if err := os.MkdirAll(b.config.DataDir, 0755); err != nil {
		b.err = fmt.Errorf("failed to create data directory: %w", err)
	}
	return b
}

// WithLogger initializes persistent logging.
func (b *ServerBuilder) WithLogger() *ServerBuilder {
	if b.err != nil {
		return b
	}

	level := slog.LevelInfo
	switch b.config.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logFilePath := filepath.Join(b.config.DataDir, "daemon.log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		b.err = fmt.Errorf("failed to open daemon log file: %w", err)
		return b
	}

	multiWriter := io.MultiWriter(os.Stdout, logFile)
	b.server.logger = slog.New(slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: level}))
	b.server.logFile = logFile
	b.pushCleanup(func() error { return logFile.Close() })
	return b
}

// WithPlugins loads and registers network plugins.
func (b *ServerBuilder) WithPlugins() *ServerBuilder {
	if b.err != nil {
		return b
	}

	pluginMgr := NewPluginManager(PluginManagerConfig{
		PluginDirs: []string{filepath.Join(b.config.DataDir, "plugins")},
		Logger:     b.server.logger,
	})

	result, err := pluginMgr.LoadAndRegister()
	if err != nil {
		b.err = fmt.Errorf("failed to load plugins: %w", err)
		return b
	}

	if len(result.Loaded) > 0 {
		b.server.logger.Info("network plugins loaded",
			"count", len(result.Loaded),
			"plugins", result.Loaded)
	}
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			b.server.logger.Warn("plugin load error",
				"plugin", e.Name,
				"error", e.Error)
		}
	}

	b.server.pluginManager = pluginMgr
	b.pushCleanup(func() error {
		pluginMgr.Close()
		return nil
	})
	return b
}

// WithStoreAndSubnet initializes state storage and subnet allocation.
func (b *ServerBuilder) WithStoreAndSubnet() *ServerBuilder {
	if b.err != nil {
		return b
	}

	dbPath := filepath.Join(b.config.DataDir, "devnetd.db")
	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		b.err = fmt.Errorf("failed to open state store: %w", err)
		return b
	}

	b.server.store = st
	b.pushCleanup(func() error { return st.Close() })

	subnetAllocatorPath := filepath.Join(b.config.DataDir, "subnets.json")
	subnetAlloc, err := subnet.LoadOrCreate(subnetAllocatorPath)
	if err != nil {
		b.err = fmt.Errorf("failed to initialize subnet allocator: %w", err)
		return b
	}
	b.server.subnetAllocator = subnetAlloc
	b.server.logger.Info("subnet allocator initialized", "path", subnetAllocatorPath)
	return b
}

// WithControllers registers controller stack.
func (b *ServerBuilder) WithControllers() *ServerBuilder {
	if b.err != nil {
		return b
	}

	b.setupControllerManager()
	b.setupOrchestratorAndProvisioner()
	b.registerDevnetController()
	b.registerHealthController()
	b.registerUpgradeController()
	b.registerTransactionController()
	return b
}

func (b *ServerBuilder) setupControllerManager() {
	mgr := controller.NewManager()
	mgr.SetLogger(b.server.logger)
	b.server.manager = mgr
}

func (b *ServerBuilder) setupOrchestratorAndProvisioner() {
	b.orchFactory = NewOrchestratorFactory(b.config.DataDir, b.server.logger)
	b.devnetProv = provisioner.NewDevnetProvisioner(b.server.store, provisioner.Config{
		DataDir:             b.config.DataDir,
		Logger:              b.server.logger,
		OrchestratorFactory: b.orchFactory,
		SubnetAllocator:     b.server.subnetAllocator,
	})
}

func (b *ServerBuilder) registerDevnetController() {
	devnetCtrl := controller.NewDevnetController(b.server.store, b.devnetProv)
	devnetCtrl.SetLogger(b.server.logger)
	devnetCtrl.SetManager(b.server.manager)
	b.server.manager.Register("devnets", devnetCtrl)
	b.attachProvisionProgressReporter(devnetCtrl)
}

func (b *ServerBuilder) attachProvisionProgressReporter(devnetCtrl *controller.DevnetController) {
	b.devnetProv.SetStepProgressReporterFactory(func(namespace, name string) ports.ProgressReporter {
		return ports.ProgressFunc(func(step ports.StepProgress) {
			devnetCtrl.BroadcastProvisionLog(namespace, name, &controller.ProvisionLogEntry{
				Timestamp:       time.Now(),
				Level:           "info",
				Message:         step.Name,
				Phase:           "genesis-fork",
				StepName:        step.Name,
				StepStatus:      step.Status,
				ProgressCurrent: step.Current,
				ProgressTotal:   step.Total,
				ProgressUnit:    step.Unit,
				StepDetail:      step.Detail,
				Speed:           step.Speed,
			})
		})
	})
}

func (b *ServerBuilder) registerHealthController() {
	healthChecker := checker.NewRPCHealthChecker(checker.Config{
		Logger:  b.server.logger,
		Timeout: b.config.HealthCheckTimeout,
	})
	healthConfig := controller.DefaultHealthControllerConfig()
	healthCtrl := controller.NewHealthController(b.server.store, healthChecker, b.server.manager, healthConfig)
	healthCtrl.SetLogger(b.server.logger)
	b.server.manager.Register("health", healthCtrl)
	b.server.healthCtrl = healthCtrl
}

func (b *ServerBuilder) registerUpgradeController() {
	upgradeRuntime := upgrader.NewRuntime(b.server.store, upgrader.Config{Logger: b.server.logger})
	upgradeCtrl := controller.NewUpgradeController(b.server.store, upgradeRuntime)
	upgradeCtrl.SetLogger(b.server.logger)
	b.server.manager.Register("upgrades", upgradeCtrl)
}

func (b *ServerBuilder) registerTransactionController() {
	txCtrl := controller.NewTxController(b.server.store, nil)
	txCtrl.SetLogger(b.server.logger)
	b.server.manager.Register("transactions", txCtrl)
}

// WithRuntime initializes node runtime and registers node controller.
func (b *ServerBuilder) WithRuntime() *ServerBuilder {
	if b.err != nil {
		return b
	}

	runtimeMode := b.config.RuntimeMode
	if runtimeMode == "" {
		runtimeMode = "process"
	}
	if b.config.EnableDocker {
		runtimeMode = "docker"
	}

	var nodeRuntime runtime.NodeRuntime
	switch runtimeMode {
	case "docker":
		dockerRuntime, err := runtime.NewDockerRuntime(runtime.DockerConfig{
			DefaultImage: b.config.DockerImage,
			Logger:       b.server.logger,
		})
		if err != nil {
			b.err = fmt.Errorf("failed to create docker runtime: %w", err)
			return b
		}
		nodeRuntime = dockerRuntime
		b.server.logger.Info("docker runtime enabled", "image", b.config.DockerImage)
	case "service":
		svcRuntime, err := runtime.NewServiceRuntime(runtime.ServiceRuntimeConfig{
			DataDir:               b.config.DataDir,
			Logger:                b.server.logger,
			PluginRuntimeProvider: b.orchFactory.AsPluginRuntimeProvider(),
		})
		if err != nil {
			b.err = fmt.Errorf("failed to create service runtime: %w", err)
			return b
		}
		nodeRuntime = svcRuntime
		b.server.logger.Info("service runtime enabled (OS service manager)")
	default:
		nodeRuntime = runtime.NewProcessRuntime(runtime.ProcessRuntimeConfig{
			DataDir:               b.config.DataDir,
			Logger:                b.server.logger,
			PluginRuntimeProvider: b.orchFactory.AsPluginRuntimeProvider(),
		})
		b.server.logger.Info("process runtime enabled for local mode")
	}

	b.server.nodeRuntime = nodeRuntime
	nodeCtrl := controller.NewNodeController(b.server.store, nodeRuntime)
	nodeCtrl.SetLogger(b.server.logger)
	b.server.manager.Register("nodes", nodeCtrl)
	return b
}

// WithGRPCServices initializes gRPC server and registers services.
func (b *ServerBuilder) WithGRPCServices() *ServerBuilder {
	if b.err != nil {
		return b
	}

	grpcServer := b.buildGRPCServer()
	b.server.grpcServer = grpcServer

	networkSvc := b.newNetworkService()
	anteHandler := ante.New(b.server.store, networkSvc)
	shutdownCtx := b.initShutdownContext()
	b.registerGRPCServices(grpcServer, networkSvc, anteHandler, shutdownCtx)
	return b
}

func (b *ServerBuilder) buildGRPCServer() *grpc.Server {
	if b.config.Listen == "" || !b.config.AuthEnabled {
		return grpc.NewServer()
	}

	keysFile := b.config.AuthKeysFile
	if keysFile == "" {
		keysFile = filepath.Join(b.config.DataDir, "api-keys.yaml")
	}
	keyStore := auth.NewFileKeyStore(keysFile)
	if err := keyStore.Load(); err != nil {
		b.server.logger.Warn("failed to load API keys, starting with empty key store", "error", err)
	}

	b.server.logger.Info("authentication enabled for remote connections")
	return grpc.NewServer(
		grpc.ChainUnaryInterceptor(auth.NewAuthInterceptor(keyStore, IsLocalConnection)),
		grpc.ChainStreamInterceptor(auth.NewStreamAuthInterceptor(keyStore, IsLocalConnection)),
	)
}

func (b *ServerBuilder) newNetworkService() *NetworkService {
	githubFactory := NewDefaultGitHubClientFactory(b.config.DataDir, b.server.logger)
	networkSvc := NewNetworkService(githubFactory)
	networkSvc.SetLogger(b.server.logger)
	return networkSvc
}

func (b *ServerBuilder) initShutdownContext() context.Context {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	b.server.shutdownCtx = shutdownCtx
	b.server.shutdownCancel = shutdownCancel
	b.pushCleanup(func() error {
		shutdownCancel()
		return nil
	})
	return shutdownCtx
}

func (b *ServerBuilder) registerGRPCServices(
	grpcServer *grpc.Server,
	networkSvc *NetworkService,
	anteHandler *ante.AnteHandler,
	shutdownCtx context.Context,
) {
	devnetSvc := NewDevnetServiceWithAnte(b.server.store, b.server.manager, anteHandler, b.server.subnetAllocator, b.devnetProv)
	devnetSvc.SetLogger(b.server.logger)
	v1.RegisterDevnetServiceServer(grpcServer, devnetSvc)

	nodeSvc := NewNodeServiceWithAnte(b.server.store, b.server.manager, b.server.nodeRuntime, anteHandler, shutdownCtx)
	nodeSvc.SetLogger(b.server.logger)
	v1.RegisterNodeServiceServer(grpcServer, nodeSvc)

	upgradeSvc := NewUpgradeServiceWithAnte(b.server.store, b.server.manager, anteHandler)
	upgradeSvc.SetLogger(b.server.logger)
	v1.RegisterUpgradeServiceServer(grpcServer, upgradeSvc)

	txSvc := NewTransactionService(b.server.store, b.server.manager)
	txSvc.SetLogger(b.server.logger)
	v1.RegisterTransactionServiceServer(grpcServer, txSvc)

	v1.RegisterNetworkServiceServer(grpcServer, networkSvc)
	v1.RegisterAuthServiceServer(grpcServer, NewAuthService())
}

// Build finalizes server construction.
func (b *ServerBuilder) Build() (*Server, error) {
	if b.err != nil {
		return nil, b.cleanupOnError(b.err)
	}
	b.cleanupStack = nil
	return b.server, nil
}

func (b *ServerBuilder) pushCleanup(fn func() error) {
	b.cleanupStack = append(b.cleanupStack, fn)
}

func (b *ServerBuilder) cleanupOnError(buildErr error) error {
	var cleanupErr error
	for i := len(b.cleanupStack) - 1; i >= 0; i-- {
		cleanupErr = multierr.Append(cleanupErr, b.cleanupStack[i]())
	}
	b.cleanupStack = nil
	return multierr.Append(buildErr, cleanupErr)
}

// Run starts the server and blocks until shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Remove stale socket
	os.Remove(s.config.SocketPath)

	// Create Unix socket listener (always available for local access)
	listener, err := net.Listen("unix", s.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket: %w", err)
	}
	s.listener = listener

	// Create TCP/TLS listener if configured (for remote access)
	if s.config.Listen != "" {
		tcpListener, err := s.createTLSListener()
		if err != nil {
			s.listener.Close()
			return fmt.Errorf("failed to create TCP/TLS listener: %w", err)
		}
		s.tcpListener = tcpListener
	}

	// Write PID file
	pidPath := filepath.Join(s.config.DataDir, "devnetd.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	defer os.Remove(pidPath)

	logAttrs := []any{
		"socket", s.config.SocketPath,
		"dataDir", s.config.DataDir,
		"pid", os.Getpid(),
		"workers", s.config.Workers,
	}
	if s.config.Listen != "" {
		logAttrs = append(logAttrs, "listen", s.config.Listen)
	}
	s.logger.Info("devnetd started", logAttrs...)

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Reconnect to running processes from previous session (orphaned processes)
	if err := s.reconnectExistingProcesses(ctx); err != nil {
		s.logger.Warn("partial process reconnection", "error", err)
		// Continue anyway - failed nodes will be restarted by controllers
	}

	// Start controller manager in background
	go s.manager.Start(ctx, s.config.Workers)

	// Start health controller's periodic health check loop
	s.healthCtrl.Start(ctx)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start gRPC server on Unix socket in background
	errCh := make(chan error, 2) // Buffer for both listeners
	go func() {
		errCh <- s.grpcServer.Serve(listener)
	}()

	// Start gRPC server on TCP/TLS listener if configured
	if s.tcpListener != nil {
		go func() {
			errCh <- s.grpcServer.Serve(s.tcpListener)
		}()
	}

	// Wait for shutdown
	select {
	case <-ctx.Done():
		s.logger.Info("context cancelled, shutting down")
	case sig := <-sigCh:
		s.logger.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		if err != nil {
			s.logger.Error("gRPC server error", "error", err)
			return err
		}
	}

	// Cancel context BEFORE shutdown to allow workers to exit gracefully.
	// This is critical: Manager.Start() waits on ctx.Done() before signaling
	// that workers have stopped. If we don't cancel here, Shutdown() will
	// deadlock waiting for workers that are waiting for context cancellation.
	cancel()

	return s.Shutdown()
}

// Shutdown gracefully shuts down the server.
// Detaches from running processes so they continue as orphans.
func (s *Server) Shutdown() error {
	s.logger.Info("shutting down")

	// Stop health controller
	if s.healthCtrl != nil {
		s.healthCtrl.Stop()
	}

	// Stop controller manager and wait for all workers to complete.
	// This MUST happen before closing the store to prevent "database not open" errors.
	// Use a timeout to prevent hanging if workers are blocked on external processes
	// (e.g., a Cosmos SDK binary deadlocked during genesis export).
	if s.manager != nil {
		s.logger.Debug("waiting for controller workers to stop", "timeout", s.config.ShutdownTimeout)
		graceful := s.manager.StopWithTimeout(s.config.ShutdownTimeout)
		if graceful {
			s.logger.Debug("controller workers stopped gracefully")
		} else {
			s.logger.Warn("controller workers did not stop within timeout, proceeding with shutdown")
		}
	}

	// Handle runtime-specific shutdown behavior
	switch rt := s.nodeRuntime.(type) {
	case *runtime.ProcessRuntime:
		// Detach from running processes - they will continue as orphans
		// and can be reconnected on next startup
		s.logger.Info("detaching from running processes (will persist as orphans)")
		if err := rt.Detach(); err != nil {
			s.logger.Warn("failed to detach from processes", "error", err)
		}
	case *runtime.ServiceRuntime:
		// No-op: launchd/systemd owns the processes. They persist independently.
		s.logger.Info("nodes persist under OS service manager")
	}

	// Cancel shutdown context to terminate long-running streaming RPCs (e.g., log streaming).
	// This MUST happen before GracefulStop() to unblock streams that would otherwise
	// prevent graceful shutdown from completing.
	if s.shutdownCancel != nil {
		s.logger.Debug("cancelling streaming RPCs for graceful shutdown")
		s.shutdownCancel()
	}

	// Graceful gRPC shutdown
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// Close listeners
	if s.listener != nil {
		s.listener.Close()
	}
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}

	// Close store (safe now that all workers have stopped)
	if s.store != nil {
		s.store.Close()
	}

	// Close plugin manager
	if s.pluginManager != nil {
		s.pluginManager.Close()
	}

	// Close log file
	if s.logFile != nil {
		s.logFile.Close()
	}

	// Clean up socket
	os.Remove(s.config.SocketPath)

	s.logger.Info("devnetd stopped")
	return nil
}

// reconnectExistingProcesses attempts to reconnect to orphaned node processes
// from a previous daemon session. This is called at startup.
func (s *Server) reconnectExistingProcesses(ctx context.Context) error {
	// Determine reconnection strategy based on runtime type
	switch rt := s.nodeRuntime.(type) {
	case *runtime.ProcessRuntime:
		allNodes, err := s.collectAllNodes(ctx)
		if err != nil {
			return err
		}
		if len(allNodes) == 0 {
			s.logger.Debug("no nodes to reconnect")
			return nil
		}

		reconnected, err := rt.ReconnectAll(ctx, allNodes)
		if err != nil {
			return fmt.Errorf("reconnection failed: %w", err)
		}
		s.logger.Info("process reconnection summary",
			"reconnected", reconnected,
			"totalNodes", len(allNodes))
		return nil

	case *runtime.ServiceRuntime:
		allNodes, err := s.collectAllNodes(ctx)
		if err != nil {
			return err
		}
		if len(allNodes) == 0 {
			s.logger.Debug("no nodes to discover")
			return nil
		}

		discovered, err := rt.DiscoverExisting(ctx, allNodes)
		if err != nil {
			return fmt.Errorf("service discovery failed: %w", err)
		}
		s.logger.Info("service discovery summary",
			"discovered", discovered,
			"totalNodes", len(allNodes))
		return nil

	default:
		// Docker or other runtimes don't need reconnection
		return nil
	}
}

// collectAllNodes gathers all nodes from all devnets in the store.
func (s *Server) collectAllNodes(ctx context.Context) ([]*types.Node, error) {
	devnets, err := s.store.ListDevnets(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list devnets: %w", err)
	}

	var allNodes []*types.Node
	for _, devnet := range devnets {
		nodes, err := s.store.ListNodes(ctx, devnet.Metadata.Namespace, devnet.Metadata.Name)
		if err != nil {
			s.logger.Warn("failed to list nodes for devnet",
				"namespace", devnet.Metadata.Namespace,
				"devnet", devnet.Metadata.Name,
				"error", err)
			continue
		}
		allNodes = append(allNodes, nodes...)
	}
	return allNodes, nil
}

// createTLSListener creates a TCP listener with TLS configured.
// NOTE: TLS certificates are loaded once at startup. For certificate rotation,
// the server must be restarted. For production deployments requiring zero-downtime
// rotation, consider using tls.Config.GetCertificate callback or a reverse proxy.
func (s *Server) createTLSListener() (net.Listener, error) {
	// Load TLS certificate and key
	cert, err := tls.LoadX509KeyPair(s.config.TLSCert, s.config.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Create TCP listener with TLS
	listener, err := tls.Listen("tcp", s.config.Listen, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", s.config.Listen, err)
	}

	s.logger.Info("TCP/TLS listener started", "address", s.config.Listen)
	return listener, nil
}
