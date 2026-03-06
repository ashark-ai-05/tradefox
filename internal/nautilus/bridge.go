package nautilus

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	pb "github.com/ashark-ai-05/tradefox/internal/nautilus/proto"
)

// NautilusBridge manages the gRPC connection and process lifecycle.
type NautilusBridge struct {
	conn       *grpc.ClientConn
	process    *NautilusProcess
	strategies pb.StrategyServiceClient
	execution  pb.ExecutionServiceClient
	portfolio  pb.PortfolioServiceClient
	backtest   pb.BacktestServiceClient
	data       pb.DataServiceClient
	eventBus   *eventbus.Bus
	config     NautilusConfig
	logger     *slog.Logger
	connected  bool
}

// NewBridge creates a new NautilusBridge.
func NewBridge(config NautilusConfig, bus *eventbus.Bus, logger *slog.Logger) *NautilusBridge {
	return &NautilusBridge{
		config:   config,
		eventBus: bus,
		logger:   logger,
	}
}

// Start launches the Nautilus process (if autostart) and connects via gRPC.
func (b *NautilusBridge) Start(ctx context.Context) error {
	if b.config.AutoStart {
		b.process = NewProcess(b.config.PythonPath, b.config.GRPCPort, b.logger)
		if err := b.process.Start(); err != nil {
			return fmt.Errorf("start nautilus process: %w", err)
		}
		// Give the server a moment to start
		time.Sleep(2 * time.Second)
	}

	addr := fmt.Sprintf("%s:%d", b.config.GRPCAddress, b.config.GRPCPort)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("grpc connect to %s: %w", addr, err)
	}
	b.conn = conn

	b.strategies = pb.NewStrategyServiceClient(conn)
	b.execution = pb.NewExecutionServiceClient(conn)
	b.portfolio = pb.NewPortfolioServiceClient(conn)
	b.backtest = pb.NewBacktestServiceClient(conn)
	b.data = pb.NewDataServiceClient(conn)

	// Verify health
	healthClient := grpc_health_v1.NewHealthClient(conn)
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := healthClient.Check(healthCtx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		b.logger.Warn("nautilus health check failed", "error", err)
	} else {
		b.logger.Info("nautilus connected", "status", resp.Status.String())
		b.connected = true
	}

	return nil
}

// Stop gracefully shuts down the connection and process.
func (b *NautilusBridge) Stop() error {
	b.connected = false

	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			b.logger.Warn("error closing grpc connection", "error", err)
		}
	}

	if b.process != nil {
		return b.process.Stop()
	}
	return nil
}

// IsConnected returns whether the gRPC connection is active.
func (b *NautilusBridge) IsConnected() bool {
	return b.connected
}
