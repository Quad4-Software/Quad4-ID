package bootstrap

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/italypaleale/francis/clusteradmin"
	"github.com/italypaleale/francis/components"

	"github.com/pocket-id/pocket-id/backend/internal/common"
	"github.com/pocket-id/pocket-id/backend/internal/service"
)

// AcquireExclusiveAccess takes an exclusive-access lease on the cluster so a restore can safely overwrite the database.
//
// It returns a release function that must be called once the operation is done, and a channel that is closed if the lease is lost while it is held.
func AcquireExclusiveAccess(ctx context.Context, providerOpts components.ProviderOptions, force bool) (release func(), lost <-chan struct{}, err error) {
	// New initializes the provider, applying the actor host's schema migrations, so this also works against a brand-new (empty) database
	admin, err := clusteradmin.New(ctx, providerOpts, clusteradmin.Options{
		// Match the actor host so the admin waits the right amount of time for hosts to drain
		HostHealthCheckDeadline: ActorsHostHealthCheckDeadline(common.EnvConfig.HAEnabled),
		Logger:                  slog.Default(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cluster admin: %w", err)
	}

	lost, err = admin.AcquireExclusive(ctx, clusteradmin.AcquireOptions{Force: force})
	if err != nil {
		_ = admin.Close()
		return nil, nil, err
	}

	release = func() {
		// The import preserves the actor host's "francis_" tables, including the lease row, so the lease must be released explicitly
		// Detach from ctx so the release still runs even if the operation was canceled
		releaseCtx, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancelRelease()
		rErr := admin.ReleaseExclusive(releaseCtx)
		if rErr != nil {
			slog.WarnContext(ctx, "Failed to release exclusive access", slog.Any("error", rErr))
		}
		_ = admin.Close()
	}
	return release, lost, nil
}

// EnsureNoActorsBackup rejects an archive that carries the actor data when a standalone Francis runtime owns it
// Such an archive comes from a deployment with an embedded runtime, and restoring only its Quad4 ID half would leave the runtime holding actor state belonging to a different deployment
func EnsureNoActorsBackup(zipReader *zip.Reader) error {
	for _, f := range zipReader.File {
		if f.Name == service.ActorsBackupFileName {
			return fmt.Errorf("this archive contains the actor data (%s) but FRANCIS_HOST points to a standalone Francis runtime, which owns that data instead: restore it into a deployment with an embedded runtime, or load the actor data into the runtime with 'francis runtime restore'", service.ActorsBackupFileName)
		}
	}

	return nil
}

// IsExclusiveAccessConflict reports whether err means the cluster refused exclusive access because other hosts are connected or another exclusive operation is running
func IsExclusiveAccessConflict(err error) bool {
	return errors.Is(err, components.ErrHostsConnected) || errors.Is(err, components.ErrExclusiveHeld)
}
