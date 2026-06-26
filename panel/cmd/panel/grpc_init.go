package main

import (
	"context"
	"crypto/tls"
	"database/sql"

	"KorisPanel/panel/internal/alerts"
	"KorisPanel/panel/internal/config"
	"KorisPanel/panel/internal/dbstore"
	"KorisPanel/panel/internal/dbstore/mariadb"
	"KorisPanel/panel/internal/grpcclient"
	"KorisPanel/panel/internal/noderegistry"
	"KorisPanel/panel/internal/tui"
)

// grpcSubsystem holds references to all gRPC-related components initialized
// during startup. This allows the main function and other parts of the system
// to access the pool, traffic collector, etc.
type grpcSubsystem struct {
	Pool             grpcclient.Pool
	Store            dbstore.Store
	Registry         *noderegistry.DBRegistry
	StatusMonitor    *grpcclient.StatusMonitor
	MetricsConsumer  *grpcclient.MetricsConsumer
	UserSync         *grpcclient.UserSyncService
	TrafficCollector *grpcclient.TrafficCollector
	Thresholds       alerts.Thresholds
}

// initGRPCSubsystem initializes the gRPC client pool and all related services.
// It follows the startup sequence:
//  1. Create dbstore.Store wrapper around the existing database
//  2. Initialize node registry with credential encryption
//  3. Create and start the gRPC connection pool
//  4. Load enabled nodes from registry and connect (non-blocking)
//  5. Start the StatusMonitor (time-based status transitions)
//  6. Create the MetricsConsumer (processes incoming streams)
//  7. Create the UserSyncService and register reconnect-sync callback
//  8. Start the TrafficCollector (periodic GetTraffic calls)
//  9. Wire the alerter into pool's OnStatusChange
//
// Failed node connections do NOT block boot — they are marked offline and
// reconnection proceeds in the background.
func initGRPCSubsystem(ctx context.Context, database *sql.DB, cfg config.Config, log *tui.Logger) (*grpcSubsystem, error) {
	log.Info("grpc-client", "initializing gRPC subsystem")

	// 1. Wrap existing *sql.DB as a dbstore.Store for gRPC subsystem components.
	// The existing database connection is MariaDB-based; we wrap it to satisfy
	// the dbstore.Store interface needed by MetricsConsumer, TrafficCollector, etc.
	store := mariadb.NewFromDB(database)

	// 2. Initialize node registry with credential encryption.
	encryptor := noderegistry.NewEncryptor(cfg.SessionSecret)
	registry := noderegistry.NewDBRegistry(database, encryptor)
	log.Info("grpc-client", "node registry initialized")

	// 3. Create and start the gRPC connection pool.
	poolCfg := grpcclient.PoolConfig{
		ConnectTimeout:    cfg.GRPCConnectTimeout,
		KeepaliveInterval: cfg.GRPCKeepaliveInterval,
		Backoff:           grpcclient.DefaultBackoff(),
	}
	pool := grpcclient.NewPool(poolCfg)
	if err := pool.Start(ctx); err != nil {
		return nil, err
	}
	log.Info("grpc-client", "connection pool started", map[string]any{
		"connect_timeout":    cfg.GRPCConnectTimeout.String(),
		"keepalive_interval": cfg.GRPCKeepaliveInterval.String(),
	})

	// 4. Load enabled nodes from registry and attempt connections (non-blocking).
	nodes, err := registry.ListEnabled(ctx)
	if err != nil {
		log.Warn("grpc-client", "failed to load node registry", map[string]any{"error": err.Error()})
		// Don't block boot — continue with empty pool
		nodes = nil
	}

	connectedCount := 0
	for _, rec := range nodes {
		nodeCfg, cfgErr := buildNodeConfig(rec, registry)
		if cfgErr != nil {
			log.Warn("grpc-client", "skipping node (credential error)", map[string]any{
				"node":  rec.Name,
				"error": cfgErr.Error(),
			})
			continue
		}

		// Attempt connection with a short timeout — do NOT block boot.
		connCtx, connCancel := context.WithTimeout(ctx, cfg.GRPCConnectTimeout)
		connErr := pool.Connect(connCtx, nodeCfg)
		connCancel()

		if connErr != nil {
			log.Warn("grpc-client", "node connection failed (will retry in background)", map[string]any{
				"node":    rec.Name,
				"address": rec.Address,
				"port":    rec.Port,
				"error":   connErr.Error(),
			})
			// Mark offline in registry — pool already handles reconnection.
			_ = registry.UpdateStatus(ctx, rec.ID, "offline")
		} else {
			connectedCount++
			log.Info("grpc-client", "node connected", map[string]any{
				"node":    rec.Name,
				"address": rec.Address,
				"port":    rec.Port,
			})
			_ = registry.UpdateStatus(ctx, rec.ID, "online")
		}
	}

	log.Info("grpc-client", "node connections initialized", map[string]any{
		"total":     len(nodes),
		"connected": connectedCount,
		"offline":   len(nodes) - connectedCount,
	})

	// 5. Start the StatusMonitor (evaluates time-based status transitions).
	statusMonitor := grpcclient.NewStatusMonitorFromPool(pool, 0)
	statusMonitor.Start(ctx)
	log.Info("grpc-client", "status monitor started")

	// 6. Create the MetricsConsumer.
	metricsConsumer := grpcclient.NewMetricsConsumerFromPool(store, pool)
	log.Info("grpc-client", "metrics consumer ready")

	// 6b. Start metrics streams for all connected nodes.
	for _, rec := range nodes {
		if pool.Status(rec.ID) == grpcclient.StatusOnline {
			metricsConsumer.StartStreamWithInterval(ctx, rec.ID, cfg.GRPCMetricsInterval)
		}
	}

	// 6c. Register OnStatusChange callback to start metrics stream on reconnection.
	pool.OnStatusChange(func(nodeID int64, old, new grpcclient.NodeStatus) {
		if new == grpcclient.StatusOnline && old != grpcclient.StatusOnline {
			metricsConsumer.StartStreamWithInterval(ctx, nodeID, cfg.GRPCMetricsInterval)
		}
	})

	// 7. Create UserSyncService and register reconnect-sync callback.
	userSync := grpcclient.NewUserSyncService(pool, store)
	grpcclient.RegisterReconnectSync(pool, userSync, store)
	log.Info("grpc-client", "user sync service ready (reconnect-sync callback registered)")

	// 7b. Perform initial Health + AllCoreStatuses for nodes that connected successfully.
	// This satisfies Requirement 10.4: knode reports capabilities within 30s of startup.
	for _, rec := range nodes {
		if pool.Status(rec.ID) == grpcclient.StatusOnline {
			grpcclient.InitialNodeSync(ctx, pool, store, rec.ID)
		}
	}

	// 8. Start the TrafficCollector.
	quotaEnforcer := grpcclient.NewQuotaEnforcer(userSync, store)
	trafficCollector := grpcclient.NewTrafficCollector(pool, store, cfg.GRPCMetricsInterval, quotaEnforcer)
	trafficCollector.Start(ctx)
	log.Info("grpc-client", "traffic collector started", map[string]any{
		"interval": cfg.GRPCMetricsInterval.String(),
	})

	// 9. Wire the alerter into pool's OnStatusChange.
	thresholds := alerts.Thresholds{
		CPUPercent:  float64(cfg.AlertCPUThreshold),
		RAMPercent:  float64(cfg.AlertRAMThreshold),
		DiskPercent: float64(cfg.AlertDiskThreshold),
	}
	pool.OnStatusChange(func(nodeID int64, old, new grpcclient.NodeStatus) {
		alert := alerts.CheckStatusTransition(nodeID, string(old), string(new))
		if alert != nil {
			log.Warn("alerts", alert.Message, map[string]any{
				"node_id":    nodeID,
				"old_status": string(old),
				"new_status": string(new),
				"type":       string(alert.Type),
			})
		}

		// Also update registry status
		_ = registry.UpdateStatus(ctx, nodeID, string(new))
	})
	log.Info("grpc-client", "alerter wired to pool status changes", map[string]any{
		"cpu_threshold":  cfg.AlertCPUThreshold,
		"ram_threshold":  cfg.AlertRAMThreshold,
		"disk_threshold": cfg.AlertDiskThreshold,
	})

	return &grpcSubsystem{
		Pool:             pool,
		Store:            store,
		Registry:         registry,
		StatusMonitor:    statusMonitor,
		MetricsConsumer:  metricsConsumer,
		UserSync:         userSync,
		TrafficCollector: trafficCollector,
		Thresholds:       thresholds,
	}, nil
}

// buildNodeConfig converts a NodeRecord from the registry into a grpcclient.NodeConfig
// by decrypting the API key and client key.
func buildNodeConfig(rec *noderegistry.NodeRecord, registry *noderegistry.DBRegistry) (grpcclient.NodeConfig, error) {
	apiKey, err := registry.DecryptAPIKey(rec)
	if err != nil {
		return grpcclient.NodeConfig{}, err
	}

	cfg := grpcclient.NodeConfig{
		NodeID:  rec.ID,
		Name:    rec.Name,
		Address: rec.Address,
		Port:    rec.Port,
		APIKey:  string(apiKey),
		CACert:  rec.CACertPEM,
	}

	// Client cert is optional — only parse if both cert and key are present
	if len(rec.ClientCertPEM) > 0 && len(rec.ClientKeyEnc) > 0 {
		clientKeyPEM, err := registry.DecryptClientKey(rec)
		if err != nil {
			return grpcclient.NodeConfig{}, err
		}
		clientCert, err := tls.X509KeyPair(rec.ClientCertPEM, clientKeyPEM)
		if err != nil {
			return grpcclient.NodeConfig{}, err
		}
		cfg.ClientCert = clientCert
	}

	return cfg, nil
}

// stopGRPCSubsystem gracefully shuts down all gRPC subsystem components.
func stopGRPCSubsystem(sub *grpcSubsystem) {
	if sub == nil {
		return
	}
	sub.TrafficCollector.Stop()
	sub.StatusMonitor.Stop()
	_ = sub.Pool.Stop()
}
