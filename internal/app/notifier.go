package app

import (
	"context"

	"github.com/fromforgesoftware/go-kit/monitoring/logger"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// logNotifier records re-auth needs to the log. A Herald-backed notifier
// replaces it when notification delivery is wired.
type logNotifier struct {
	log logger.Logger
}

func NewLogNotifier() *logNotifier { return &logNotifier{log: logger.New()} }

func (n *logNotifier) NeedsReauth(ctx context.Context, conn domain.Connection) error {
	n.log.WarnContext(ctx, "connection needs re-authorization",
		"connection", conn.ID(), "owner", conn.Owner(), "connector", conn.Connector())
	return nil
}
