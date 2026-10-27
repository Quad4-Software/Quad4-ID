package controller

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/pocket-id/pocket-id/backend/internal/authz"
	"github.com/pocket-id/pocket-id/backend/internal/backup"
	"github.com/pocket-id/pocket-id/backend/internal/httpserver"
	"github.com/pocket-id/pocket-id/backend/internal/service"
)

// NewBackupController creates a new controller for backup endpoints
// @Summary Create a new backup controller
// @Description Initialize routes for managing backups
// @Tags Backups
func NewBackupController(
	r *authz.Router,
	backupService *service.BackupService,
) {
	bc := &BackupController{
		backupService: backupService,
	}

	r.GET("/backups", authz.ConfigRead, httpserver.Handle(bc.listBackupsHandler))
	r.POST("/backups", authz.ConfigWrite, httpserver.Handle(bc.createBackupHandler))
	r.GET("/backups/pending-restore", authz.ConfigRead, httpserver.Handle(bc.pendingRestoreHandler))
	r.DELETE("/backups/pending-restore", authz.ConfigWrite, httpserver.Handle(bc.cancelPendingRestoreHandler))
	r.GET("/backups/:name/download", authz.ConfigRead, httpserver.Handle(bc.downloadBackupHandler))
	r.DELETE("/backups/:name", authz.ConfigWrite, httpserver.Handle(bc.deleteBackupHandler))
	r.POST("/backups/restore", authz.ConfigWrite, httpserver.Handle(bc.restoreBackupHandler))
}

type BackupController struct {
	backupService *service.BackupService
}

type pendingRestoreDto struct {
	Pending *service.PendingRestore `json:"pending"`
	Result  *service.RestoreResult  `json:"result"`
}

// listBackupsHandler godoc
// @Summary List backups
// @Description List the backup archives at the configured destination
// @Tags Backups
// @Produce json
// @Success 200 {array} service.BackupInfo
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups [get]
func (bc *BackupController) listBackupsHandler(c *gin.Context) error {
	backups, err := bc.backupService.ListBackups(c.Request.Context())
	if err != nil {
		return err
	}

	c.JSON(http.StatusOK, backups)
	return nil
}

// createBackupHandler godoc
// @Summary Create a backup
// @Description Create a new backup archive at the configured destination
// @Tags Backups
// @Produce json
// @Success 201 {object} service.BackupInfo
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups [post]
func (bc *BackupController) createBackupHandler(c *gin.Context) error {
	info, err := bc.backupService.CreateBackup(c.Request.Context())
	if err != nil {
		return err
	}

	c.JSON(http.StatusCreated, info)
	return nil
}

// pendingRestoreHandler godoc
// @Summary Get the staged restore
// @Description Get the restore staged for the next restart and the outcome of the last one
// @Tags Backups
// @Produce json
// @Success 200 {object} pendingRestoreDto
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups/pending-restore [get]
func (bc *BackupController) pendingRestoreHandler(c *gin.Context) error {
	pending, err := service.ReadPendingRestore()
	if err != nil {
		return err
	}

	result, err := service.ReadRestoreResult()
	if err != nil {
		return err
	}

	c.JSON(http.StatusOK, pendingRestoreDto{Pending: pending, Result: result})
	return nil
}

// cancelPendingRestoreHandler godoc
// @Summary Cancel the staged restore
// @Description Remove the restore staged for the next restart
// @Tags Backups
// @Success 204 "No Content"
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups/pending-restore [delete]
func (bc *BackupController) cancelPendingRestoreHandler(c *gin.Context) error {
	if err := service.ClearPendingRestore(); err != nil {
		return err
	}

	c.Status(http.StatusNoContent)
	return nil
}

// downloadBackupHandler godoc
// @Summary Download a backup
// @Description Download a backup archive from the configured destination
// @Tags Backups
// @Produce application/octet-stream
// @Success 200 {file} binary
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups/{name}/download [get]
func (bc *BackupController) downloadBackupHandler(c *gin.Context) error {
	rc, size, err := bc.backupService.OpenBackup(c.Request.Context(), c.Param("name"))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("backup not found: %w", err)
	}
	if err != nil {
		return err
	}
	defer rc.Close()

	c.Header("Content-Disposition", "attachment; filename="+strconv.Quote(c.Param("name")))
	c.DataFromReader(http.StatusOK, size, "application/octet-stream", rc, nil)
	return nil
}

// deleteBackupHandler godoc
// @Summary Delete a backup
// @Description Delete a backup archive from the configured destination
// @Tags Backups
// @Success 204 "No Content"
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups/{name} [delete]
func (bc *BackupController) deleteBackupHandler(c *gin.Context) error {
	err := bc.backupService.DeleteBackup(c.Request.Context(), c.Param("name"))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("backup not found: %w", err)
	}
	if err != nil {
		return err
	}

	c.Status(http.StatusNoContent)
	return nil
}

// restoreBackupHandler godoc
// @Summary Stage a restore
// @Description Stage a backup archive to be restored on the next restart
// @Tags Backups
// @Accept multipart/form-data
// @Param name formData string false "Name of a backup at the configured destination"
// @Param file formData file false "Backup archive to upload"
// @Param encryptionKey formData string false "Passphrase for an encrypted archive"
// @Produce json
// @Success 200 {object} service.PendingRestore
// @Failure default {object} dto.ErrorDto "Error"
// @Router /api/backups/restore [post]
func (bc *BackupController) restoreBackupHandler(c *gin.Context) error {
	ctx := c.Request.Context()
	encryptionKey := c.PostForm("encryptionKey")

	var (
		name string
		open func() (io.ReadCloser, error)
	)

	if fh, err := c.FormFile("file"); err == nil && fh != nil {
		name = fh.Filename
		open = func() (io.ReadCloser, error) {
			return fh.Open()
		}
	} else {
		name = c.PostForm("name")
		if name == "" {
			return errors.New("either a backup name or an uploaded file is required")
		}
		if !backup.IsBackupName(name) {
			return fmt.Errorf("invalid backup name: %s", name)
		}
		open = func() (io.ReadCloser, error) {
			rc, _, err := bc.backupService.OpenBackup(ctx, name)
			return rc, err
		}
	}

	err := bc.backupService.StageRestore(name, open, []byte(encryptionKey))
	if err != nil {
		return err
	}

	pending, err := service.ReadPendingRestore()
	if err != nil {
		return err
	}

	c.JSON(http.StatusOK, pending)
	return nil
}
