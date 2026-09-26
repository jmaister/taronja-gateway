package gateway

import (
	"context"
	"strings"
	"sync"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/notification"
)

// InitNotifications builds the notification.Service from cfg, stores it on
// d.NotificationService, and starts its background goroutines: the retry
// worker (unconditionally — retries apply to any channel) and, if
// Telegram is configured, its long-polling update loop. Called once at
// startup (see main.go), after deps.NewProduction (needs
// d.NotificationRepo/d.UserRepo) and before NewGatewayWithDependencies
// (registerOpenAPIRoutes reads d.NotificationService when wiring up
// StrictApiServer).
//
// Like InitTracing, notification config is fixed at startup and not
// reload-aware: a config reload that changes notification.* is stored but
// has no effect until a full restart (see doc/notifications.md's Notes
// section) — the underlying provider credentials rarely change at
// runtime, and reconstructing mid-flight would risk dropping whatever the
// (harmless, best-effort) Telegram poller was mid-request on.
func InitNotifications(ctx context.Context, cfg config.NotificationConfig, serverCfg config.ServerConfig, managementCfg config.ManagementConfig, d *deps.Dependencies) (shutdown func(context.Context) error, err error) {
	respondBaseURL := ""
	if serverCfg.URL != "" {
		respondBaseURL = strings.TrimSuffix(serverCfg.URL, "/") + managementCfg.Prefix + "/api/notifications/respond"
	}

	service := notification.NewService(cfg, d.NotificationRepo, d.UserRepo, respondBaseURL)
	d.NotificationService = service

	workerCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		service.RunRetryWorker(workerCtx)
	}()

	if poller := service.TelegramPoller(); poller != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			poller.Run(workerCtx)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	return func(shutdownCtx context.Context) error {
		cancel()
		select {
		case <-done:
			return nil
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}, nil
}
