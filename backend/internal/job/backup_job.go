package job

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/italypaleale/francis/builtin/cronjob"

	"github.com/pocket-id/pocket-id/backend/internal/service"
)

// BackupJob runs scheduled backups through the actor host's cron system
// The BackupService is attached after the services are initialized, which happens after the cron actor is registered but always before the host starts running jobs
type BackupJob struct {
	svc atomic.Pointer[service.BackupService]
}

// NewBackupJob creates a BackupJob with no service attached yet
func NewBackupJob() *BackupJob {
	return &BackupJob{}
}

// SetService attaches the BackupService the job invokes
func (j *BackupJob) SetService(svc *service.BackupService) {
	j.svc.Store(svc)
}

// CronJob returns the built-in actor that runs the job every hour, so backups happen within an hour of their configured interval after restarts
func (j *BackupJob) CronJob() (*cronjob.CronJob, error) {
	cj, err := cronjob.New(
		"AutomaticBackup",
		cronjob.WithJob(j.run),
		cronjob.WithInterval(time.Hour),
		cronjob.WithLogger(slog.Default()),
		cronjob.WithJitter(15*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating AutomaticBackup job: %w", err)
	}

	return cj, nil
}

func (j *BackupJob) run(ctx context.Context) error {
	svc := j.svc.Load()
	if svc == nil {
		return nil
	}

	return svc.RunScheduledBackup(ctx)
}
