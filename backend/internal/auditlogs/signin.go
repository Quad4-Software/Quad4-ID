package auditlogs

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/pocket-id/pocket-id/backend/internal/appconfig"
	"github.com/pocket-id/pocket-id/backend/internal/model"
)

// CreateSignIn records a successful login and applies the global notification policy
// The caller must send the notification only after the login commits
// When the sign-in looks anomalous and prevention is enabled, no sign-in row is written and the result is marked Blocked
// A blocked caller must abort the login first and then persist the attempt through RecordBlockedSignIn, because writing on a second connection while the login transaction holds a lock deadlocks SQLite
func (s *service) CreateSignIn(ctx context.Context, event Event, ipAddress, userAgent, userID, browserToken string, tx *gorm.DB, dbConfig *appconfig.AppConfigModel) SignInResult {
	result := SignInResult{Event: event}

	if dbConfig.SignInAnomalyMode != appconfig.SignInAnomalyDisabled {
		// The location is resolved up front so detection and a possible blocked entry share it
		country, city, geoErr := s.ipLocator.GetLocationByIP(ctx, ipAddress)
		if geoErr != nil {
			slog.WarnContext(ctx, "Failed to get IP location", slog.String("ip", ipAddress), slog.Any("error", geoErr))
		}

		if anomalies := s.detectSignInAnomalies(ctx, userID, country, dbConfig.SignInAnomalyMode, tx); len(anomalies) > 0 {
			result.Anomalies = anomalies
			if dbConfig.SignInAnomalyMode == appconfig.SignInAnomalyPrevent {
				// Nothing is persisted here because the login transaction has to roll back first
				result.Blocked = true
				result.Notify = true
				result.AuditLog = AuditLog{
					Event:     EventAnomalousSignInBlocked,
					Country:   country,
					City:      city,
					UserAgent: userAgent,
					UserID:    userID,
					Data:      Data{DataKeyAnomalies: strings.Join(anomalies, ",")},
				}
				if ipAddress != "" {
					result.AuditLog.IpAddress = &ipAddress
				}
				result.Created = true
				return result
			}
		}
	}

	entry, created := s.Create(ctx, event, ipAddress, userAgent, userID, Data{}, tx)
	result.AuditLog = entry
	result.Created = created
	if !created {
		return result
	}

	// Detection without prevention annotates the sign-in row so the audit trail shows the flag
	if len(result.Anomalies) > 0 {
		if entry.Data == nil {
			entry.Data = Data{}
		}
		entry.Data[DataKeyAnomalies] = strings.Join(result.Anomalies, ",")
		if err := tx.WithContext(ctx).Model(&AuditLog{}).Where("id = ?", entry.ID).Update("data", entry.Data).Error; err != nil {
			slog.ErrorContext(ctx, "Failed to annotate sign-in anomalies", slog.Any("error", err))
		}
		result.AuditLog = entry
		result.Notify = true
	}

	// Only browser recognition uses a persistent cookie; other modes rely on the selected notification policy
	switch dbConfig.EmailLoginNotificationMode {
	case appconfig.LoginNotificationAlways:
		result.Notify = true
		return result
	case appconfig.LoginNotificationBrowserRecognition:
		known, token, err := s.rememberBrowser(userID, browserToken)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to remember sign-in browser", slog.Any("error", err))
		}
		result.KnownBrowserToken = token
		if known {
			return result
		}
	case appconfig.LoginNotificationIPAndUserAgent:
		// Check sign-in history without reading or renewing the browser token
	default:
		return result
	}

	// Only earlier successful sign-ins for this user can satisfy the fallback
	var count int64
	query := tx.WithContext(ctx).Model(&AuditLog{}).
		Where("user_id = ? AND user_agent = ? AND id <> ?", userID, userAgent, entry.ID).
		Where("event IN ?", []Event{EventSignIn, EventOneTimeAccessTokenSignIn, EventRemoteSignIn})
	if ipAddress == "" {
		query = query.Where("ip_address IS NULL")
	} else {
		query = query.Where("ip_address = ?", ipAddress)
	}
	if err := query.Count(&count).Error; err != nil {
		slog.ErrorContext(ctx, "Failed to check sign-in history", slog.Any("error", err))
		return result
	}
	result.Notify = result.Notify || count == 0
	return result
}

// RecordBlockedSignIn persists an anomalous attempt that prevention refused
// It runs on the primary connection so callers must only invoke it after the login transaction has been rolled back
func (s *service) RecordBlockedSignIn(ctx context.Context, result SignInResult) {
	if !result.Blocked || result.AuditLog.Event != EventAnomalousSignInBlocked {
		return
	}
	entry := result.AuditLog
	ipAddress := ""
	if entry.IpAddress != nil {
		ipAddress = *entry.IpAddress
	}
	s.Create(ctx, EventAnomalousSignInBlocked, ipAddress, entry.UserAgent, entry.UserID, entry.Data, s.db)
}

// detectSignInAnomalies compares the new sign-in against the account's successful sign-in geography history
func (s *service) detectSignInAnomalies(ctx context.Context, userID, country string, mode appconfig.AppConfigValue, tx *gorm.DB) []string {
	if mode == appconfig.SignInAnomalyDisabled || country == "" {
		return nil
	}

	signInEvents := []Event{EventSignIn, EventOneTimeAccessTokenSignIn, EventRemoteSignIn}

	// Two sign-ins from different countries inside the window cannot be the same person travelling
	var latest AuditLog
	latestErr := tx.WithContext(ctx).
		Where("user_id = ? AND country <> ''", userID).
		Where("event IN ?", signInEvents).
		Order("created_at DESC").
		First(&latest).
		Error
	if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
		slog.ErrorContext(ctx, "Failed to check latest sign-in location", slog.Any("error", latestErr))
		return nil
	}
	impossibleTravel := latestErr == nil && latest.Country != country && time.Since(latest.CreatedAt.ToTime()) < impossibleTravelWindow

	// A country the account has never signed in from is only suspicious once the account has a geography baseline
	var countries []string
	if err := tx.WithContext(ctx).Model(&AuditLog{}).
		Where("user_id = ? AND country <> ''", userID).
		Where("event IN ?", signInEvents).
		Distinct().
		Pluck("country", &countries).
		Error; err != nil {
		slog.ErrorContext(ctx, "Failed to check sign-in country history", slog.Any("error", err))
		return nil
	}
	newCountry := len(countries) > 0 && !slices.Contains(countries, country)

	var anomalies []string
	if impossibleTravel {
		anomalies = append(anomalies, AnomalyImpossibleTravel)
	}
	if newCountry {
		anomalies = append(anomalies, AnomalyNewCountry)
	}
	return anomalies
}

// SendSignInNotification sends only after the caller has completed the login successfully
func (s *service) SendSignInNotification(ctx context.Context, result SignInResult) {
	if !result.Created || !result.Notify {
		return
	}
	entry := result.AuditLog
	ipAddress := ""
	if entry.IpAddress != nil {
		ipAddress = *entry.IpAddress
	}
	go func() {
		// This runs in background, so use a context without cancellation (or it would be stopped when the request ends)
		// We still want to have a context derived from the request's to carry over tracing info
		innerCtx := context.WithoutCancel(ctx)

		// This runs after the request has completed, so we resolve the current config rather than threading the request's snapshot into the goroutine
		dbConfig, innerErr := s.appConfigService.GetConfig(innerCtx)
		if innerErr != nil {
			slog.ErrorContext(innerCtx, "Failed to load app configuration to send notification email", slog.Any("error", innerErr))
			return
		}

		// Note we don't use the transaction here because this is running in background
		var user model.User
		innerErr = s.db.
			WithContext(innerCtx).
			Where("id = ?", entry.UserID).
			First(&user).
			Error
		if innerErr != nil {
			slog.ErrorContext(innerCtx, "Failed to load user from database to send notification email", slog.Any("error", innerErr))
			return
		}

		if user.Email == nil {
			return
		}

		if result.Blocked {
			innerErr = s.emailSender.SendBlockedLogin(
				innerCtx,
				dbConfig,
				user.FullName(),
				*user.Email,
				ipAddress,
				entry.Country,
				entry.City,
				s.DeviceStringFromUserAgent(entry.UserAgent),
				signInMethod(result.Event),
				entry.CreatedAt.UTC(),
			)
		} else {
			innerErr = s.emailSender.SendNewLogin(
				innerCtx,
				dbConfig,
				user.FullName(),
				*user.Email,
				ipAddress,
				entry.Country,
				entry.City,
				s.DeviceStringFromUserAgent(entry.UserAgent),
				signInMethod(result.Event),
				entry.CreatedAt.UTC(),
			)
		}
		if innerErr != nil {
			slog.ErrorContext(innerCtx, "Failed to send notification email", slog.Any("error", innerErr), slog.String("address", *user.Email))
			return
		}
	}()
}

// signInMethod describes the successful login rather than the credential used to approve another device
func signInMethod(event Event) string {
	switch event { //nolint:exhaustive // Other audit events are not sign-ins
	case EventSignIn:
		return "Passkey"
	case EventOneTimeAccessTokenSignIn:
		return "One-time code"
	case EventRemoteSignIn:
		return "Another device (QR code)"
	default:
		return "Unknown"
	}
}

// SignInResult defers notification delivery until the login has committed
type SignInResult struct {
	AuditLog          AuditLog
	Event             Event
	Created           bool
	Notify            bool
	Blocked           bool
	Anomalies         []string
	KnownBrowserToken string
}
