package auditlogs

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/pocket-id/pocket-id/backend/internal/appconfig"
	"github.com/pocket-id/pocket-id/backend/internal/model"
	testutils "github.com/pocket-id/pocket-id/backend/internal/utils/testing"
)

type signInLocationResolver struct{}

func (signInLocationResolver) GetLocationByIP(context.Context, string) (string, string, error) {
	return "Switzerland", "Zurich", nil
}

func TestSignInBrowserRecognition(t *testing.T) {
	for _, tt := range []struct {
		name          string
		ip            string
		agent         string
		cookie        string
		otherUser     bool
		previousEvent Event
		wantNotify    bool
	}{
		{name: "cookie recognizes changed IPv6 prefix and browser version", ip: "2001:db8:2::1", agent: "browser/2", cookie: "known"},
		{name: "missing cookie falls back to exact IP and agent", ip: "2001:db8:1::1", agent: "browser/1"},
		{name: "unknown cookie falls back to exact IP and agent", ip: "2001:db8:1::1", agent: "browser/1", cookie: "forged"},
		{name: "same IPv6 subnet is not an exact match", ip: "2001:db8:1::2", agent: "browser/1", wantNotify: true},
		{name: "same IP with changed agent is unknown", ip: "2001:db8:1::1", agent: "browser/2", wantNotify: true},
		{name: "forged cookie does not suppress notification", ip: "192.0.2.1", agent: "browser/1", cookie: "forged", wantNotify: true},
		{name: "cookie and history are scoped to user", ip: "2001:db8:1::1", agent: "browser/1", cookie: "known", otherUser: true, wantNotify: true},
		{name: "unrelated audit event does not establish familiarity", ip: "2001:db8:1::1", agent: "browser/1", previousEvent: EventClientAuthorization, wantNotify: true},
		{name: "login code history establishes familiarity", ip: "2001:db8:1::1", agent: "browser/1", previousEvent: EventOneTimeAccessTokenSignIn},
		{name: "QR login history establishes familiarity", ip: "2001:db8:1::1", agent: "browser/1", previousEvent: EventRemoteSignIn},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testutils.NewDatabaseForTest(t)
			s := newService(db, nil, signInLocationResolver{}, nil, browserTokenStub{})
			user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
			require.NoError(t, db.Create(&user).Error)
			historyUserID := user.ID
			if tt.otherUser {
				other := model.User{Base: model.Base{ID: "other"}, Username: "other"}
				require.NoError(t, db.Create(&other).Error)
				historyUserID = other.ID
			}
			event := tt.previousEvent
			if event == "" {
				event = EventSignIn
			}
			_, created := s.Create(t.Context(), event, "2001:db8:1::1", "browser/1", historyUserID, Data{}, db)
			require.True(t, created)
			s.browserTokens = browserTokenStub{userID: historyUserID}

			result := s.CreateSignIn(t.Context(), EventSignIn, tt.ip, tt.agent, user.ID, tt.cookie, db, &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition})
			require.True(t, result.Created)
			require.Equal(t, tt.wantNotify, result.Notify)

			require.Equal(t, "renewed:"+user.ID, result.KnownBrowserToken)
			require.Equal(t, tt.ip, *result.AuditLog.IpAddress)
			require.Equal(t, tt.agent, result.AuditLog.UserAgent)
		})
	}
}

func TestSignInWithoutAddress(t *testing.T) {
	db := testutils.NewDatabaseForTest(t)
	s := newService(db, nil, signInLocationResolver{}, nil, browserTokenStub{})
	user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
	require.NoError(t, db.Create(&user).Error)
	first := s.CreateSignIn(t.Context(), EventSignIn, "", "browser", user.ID, "", db, &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition})
	require.True(t, first.Notify)
	require.Nil(t, first.AuditLog.IpAddress)
	second := s.CreateSignIn(t.Context(), EventSignIn, "", "browser", user.ID, "", db, &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition})
	require.False(t, second.Notify)
}

func TestSignInAuditRollsBackWithLogin(t *testing.T) {
	db := testutils.NewDatabaseForTest(t)
	s := newService(db, nil, signInLocationResolver{}, nil, browserTokenStub{})
	user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
	require.NoError(t, db.Create(&user).Error)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	result := s.CreateSignIn(t.Context(), EventSignIn, "192.0.2.1", "browser", user.ID, "", tx, &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition})
	require.True(t, result.Created)
	require.True(t, result.Notify)
	require.NoError(t, tx.Rollback().Error)
	var count int64
	require.NoError(t, db.Model(&AuditLog{}).Count(&count).Error)
	require.Zero(t, count)
}

type notificationConfig struct{ config *appconfig.AppConfigModel }

func (c notificationConfig) GetConfig(context.Context) (*appconfig.AppConfigModel, error) {
	return c.config, nil
}

type loginNotification struct{ recipient, ip, method string }
type loginNotificationSender struct{ sent chan loginNotification }

func (s loginNotificationSender) SendNewLogin(_ context.Context, _ *appconfig.AppConfigModel, _, email, ip, _, _, _, method string, _ time.Time) error {
	s.sent <- loginNotification{recipient: email, ip: ip, method: method}
	return nil
}

func (s loginNotificationSender) SendBlockedLogin(_ context.Context, _ *appconfig.AppConfigModel, _, email, ip, _, _, _, method string, _ time.Time) error {
	s.sent <- loginNotification{recipient: email, ip: ip, method: method}
	return nil
}

func TestSignInNotificationsForEveryMethod(t *testing.T) {
	for _, tt := range []struct {
		event  Event
		method string
	}{
		{EventSignIn, "Passkey"},
		{EventOneTimeAccessTokenSignIn, "One-time code"},
		{EventRemoteSignIn, "Another device (QR code)"},
	} {
		t.Run(tt.method, func(t *testing.T) {
			db := testutils.NewDatabaseForTest(t)
			email := "user@example.test"
			user := model.User{Base: model.Base{ID: "user"}, Username: "user", Email: &email}
			require.NoError(t, db.Create(&user).Error)
			sender := loginNotificationSender{sent: make(chan loginNotification, 1)}
			config := &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition}
			service := newService(db, sender, signInLocationResolver{}, notificationConfig{config}, browserTokenStub{})

			// Each successful sign-in method delivers a notification for an unfamiliar browser
			result := service.CreateSignIn(t.Context(), tt.event, "192.0.2.1", "browser", user.ID, "", db, &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition})
			require.True(t, result.Created)
			require.True(t, result.Notify)
			service.SendSignInNotification(t.Context(), result)
			select {
			case sent := <-sender.sent:
				require.Equal(t, loginNotification{recipient: email, ip: "192.0.2.1", method: tt.method}, sent)
			case <-time.After(5 * time.Second):
				t.Fatal("sign-in notification was not delivered")
			}
		})
	}
}

// browserTokenStub exercises sign-in decisions while JWT validation is covered by the token service tests
type browserTokenStub struct {
	userID  string
	signErr error
}

func (s browserTokenStub) generate(userID string, _ time.Duration) (string, error) {
	if s.signErr != nil {
		return "", s.signErr
	}
	return "renewed:" + userID, nil
}

func (s browserTokenStub) verify(token, userID string) error {
	if token == "renewed:"+userID || (token == "known" && userID == s.userID) {
		return nil
	}
	return errors.New("invalid browser token")
}

func TestSignInBrowserSigningFailurePreservesLogin(t *testing.T) {
	for _, token := range []string{"", "invalid-token", "known"} {
		t.Run(token, func(t *testing.T) {
			db := testutils.NewDatabaseForTest(t)
			s := newService(db, nil, signInLocationResolver{}, nil, browserTokenStub{
				userID: "user", signErr: errors.New("signing failed"),
			})
			user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
			require.NoError(t, db.Create(&user).Error)
			tx := db.Begin()
			require.NoError(t, tx.Error)
			result := s.CreateSignIn(t.Context(), EventSignIn, "192.0.2.1", "browser", user.ID, token, tx, &appconfig.AppConfigModel{EmailLoginNotificationMode: appconfig.LoginNotificationBrowserRecognition})
			require.NoError(t, tx.Commit().Error)
			require.True(t, result.Created)
			require.Equal(t, token, result.KnownBrowserToken)
			require.Equal(t, token != "known", result.Notify)
			require.NoError(t, db.First(&AuditLog{}, "id = ?", result.AuditLog.ID).Error)
		})
	}
}

func TestSignInNotificationModes(t *testing.T) {
	for _, event := range []Event{EventSignIn, EventOneTimeAccessTokenSignIn, EventRemoteSignIn} {
		for _, tt := range []struct {
			mode         appconfig.AppConfigValue
			knownHistory bool
			wantNotify   bool
		}{
			{appconfig.LoginNotificationDisabled, false, false},
			{appconfig.LoginNotificationDisabled, true, false},
			{appconfig.LoginNotificationAlways, false, true},
			{appconfig.LoginNotificationAlways, true, true},
			{appconfig.LoginNotificationIPAndUserAgent, false, true},
			{appconfig.LoginNotificationIPAndUserAgent, true, false},
			{appconfig.LoginNotificationBrowserRecognition, false, false},
			{appconfig.LoginNotificationBrowserRecognition, true, false},
		} {
			t.Run(string(event)+"/"+string(tt.mode)+"/"+strconv.FormatBool(tt.knownHistory), func(t *testing.T) {
				db := testutils.NewDatabaseForTest(t)
				// A nil token service proves cookie-free modes never verify or issue browser tokens
				s := newService(db, nil, signInLocationResolver{}, nil, nil)
				if tt.mode == appconfig.LoginNotificationBrowserRecognition {
					s.browserTokens = browserTokenStub{userID: "user"}
				}
				user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
				require.NoError(t, db.Create(&user).Error)
				if tt.knownHistory {
					_, created := s.Create(t.Context(), EventSignIn, "192.0.2.1", "browser", user.ID, Data{}, db)
					require.True(t, created)
				}
				result := s.CreateSignIn(t.Context(), event, "192.0.2.1", "browser", user.ID, "known", db, &appconfig.AppConfigModel{EmailLoginNotificationMode: tt.mode})
				require.True(t, result.Created)
				require.Equal(t, tt.wantNotify, result.Notify)
				if tt.mode == appconfig.LoginNotificationBrowserRecognition {
					require.Equal(t, "renewed:user", result.KnownBrowserToken)
				} else {
					require.Empty(t, result.KnownBrowserToken)
				}
			})
		}
	}
}

type countryResolver map[string]string

func (r countryResolver) GetLocationByIP(_ context.Context, ip string) (string, string, error) {
	return r[ip], "", nil
}

func insertSignInHistory(t *testing.T, db *gorm.DB, userID, ip, country string, age time.Duration) {
	t.Helper()
	ipPtr := &ip
	row := AuditLog{
		Event:     EventSignIn,
		IpAddress: ipPtr,
		Country:   country,
		UserAgent: "browser",
		UserID:    userID,
	}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, db.Model(&row).UpdateColumn("created_at", time.Now().Add(-age)).Error)
}

func anomalyConfig(mode string) *appconfig.AppConfigModel {
	return &appconfig.AppConfigModel{SignInAnomalyMode: appconfig.AppConfigValue(mode)}
}

func TestSignInAnomalyDetection(t *testing.T) {
	const usIP = "198.51.100.1"
	const chIP = "203.0.113.7"
	resolver := countryResolver{usIP: "United States", chIP: "Switzerland"}

	for _, tt := range []struct {
		name    string
		mode    string
		history []struct {
			ip, country string
			age         time.Duration
		}
		signInIP      string
		wantAnomalies []string
		wantBlocked   bool
	}{
		{name: "disabled mode never flags", mode: appconfig.SignInAnomalyDisabled, history: []struct {
			ip, country string
			age         time.Duration
		}{{usIP, "United States", 24 * time.Hour}}, signInIP: chIP},
		{name: "first sign-in is never anomalous", mode: appconfig.SignInAnomalyDetect, signInIP: chIP},
		{name: "same country is clean", mode: appconfig.SignInAnomalyDetect, history: []struct {
			ip, country string
			age         time.Duration
		}{{chIP, "Switzerland", time.Hour}}, signInIP: chIP},
		{name: "new country after baseline", mode: appconfig.SignInAnomalyDetect, history: []struct {
			ip, country string
			age         time.Duration
		}{{usIP, "United States", 24 * time.Hour}}, signInIP: chIP, wantAnomalies: []string{AnomalyNewCountry}},
		{name: "rapid country switch is impossible travel", mode: appconfig.SignInAnomalyDetect, history: []struct {
			ip, country string
			age         time.Duration
		}{{usIP, "United States", time.Hour}}, signInIP: chIP, wantAnomalies: []string{AnomalyImpossibleTravel, AnomalyNewCountry}},
		{name: "commuting between two known countries is still impossible travel without new country", mode: appconfig.SignInAnomalyDetect, history: []struct {
			ip, country string
			age         time.Duration
		}{{usIP, "United States", 24 * time.Hour}, {chIP, "Switzerland", 48 * time.Hour}, {usIP, "United States", time.Hour}}, signInIP: chIP, wantAnomalies: []string{AnomalyImpossibleTravel}},
		{name: "prevent mode blocks a new country sign-in", mode: appconfig.SignInAnomalyPrevent, history: []struct {
			ip, country string
			age         time.Duration
		}{{usIP, "United States", 24 * time.Hour}}, signInIP: chIP, wantAnomalies: []string{AnomalyNewCountry}, wantBlocked: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testutils.NewDatabaseForTest(t)
			s := newService(db, nil, resolver, nil, browserTokenStub{})
			user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
			require.NoError(t, db.Create(&user).Error)
			for _, h := range tt.history {
				insertSignInHistory(t, db, user.ID, h.ip, h.country, h.age)
			}

			result := s.CreateSignIn(t.Context(), EventSignIn, tt.signInIP, "browser", user.ID, "", db, anomalyConfig(tt.mode))
			require.True(t, result.Created)
			require.Equal(t, tt.wantAnomalies, result.Anomalies)
			require.Equal(t, tt.wantBlocked, result.Blocked)
			if len(tt.wantAnomalies) > 0 && !tt.wantBlocked {
				require.Equal(t, strings.Join(tt.wantAnomalies, ","), result.AuditLog.Data[DataKeyAnomalies])
				require.True(t, result.Notify)
			}
		})
	}
}

func TestSignInAnomalyPreventSurvivesRollback(t *testing.T) {
	db := testutils.NewDatabaseForTest(t)
	resolver := countryResolver{"198.51.100.1": "United States", "203.0.113.7": "Switzerland"}
	s := newService(db, nil, resolver, nil, browserTokenStub{})
	user := model.User{Base: model.Base{ID: "user"}, Username: "user"}
	require.NoError(t, db.Create(&user).Error)
	insertSignInHistory(t, db, user.ID, "198.51.100.1", "United States", 24*time.Hour)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	result := s.CreateSignIn(t.Context(), EventSignIn, "203.0.113.7", "browser", user.ID, "", tx, anomalyConfig(appconfig.SignInAnomalyPrevent))
	require.True(t, result.Blocked)
	require.NoError(t, tx.Rollback().Error)
	s.RecordBlockedSignIn(t.Context(), result)

	// The blocked attempt stays in the audit trail even though the login transaction was rolled back
	var blocked AuditLog
	err := db.Where("event = ?", EventAnomalousSignInBlocked).First(&blocked).Error
	require.NoError(t, err)
	require.Equal(t, AnomalyNewCountry, blocked.Data[DataKeyAnomalies])

	// A blocked attempt must not establish the country as known for the next sign-in, so only the seeded history row may exist
	var signIns int64
	require.NoError(t, db.Model(&AuditLog{}).Where("event = ?", EventSignIn).Count(&signIns).Error)
	require.Equal(t, int64(1), signIns)
}

func TestSignInAnomalyBlockedNotification(t *testing.T) {
	db := testutils.NewDatabaseForTest(t)
	resolver := countryResolver{"198.51.100.1": "United States", "203.0.113.7": "Switzerland"}
	email := "user@example.test"
	user := model.User{Base: model.Base{ID: "user"}, Username: "user", Email: &email}
	require.NoError(t, db.Create(&user).Error)
	sender := &blockedNotificationSender{blocked: make(chan string, 1)}
	config := &appconfig.AppConfigModel{SignInAnomalyMode: appconfig.SignInAnomalyPrevent}
	service := newService(db, sender, resolver, notificationConfig{config}, browserTokenStub{})
	insertSignInHistory(t, db, user.ID, "198.51.100.1", "United States", 24*time.Hour)

	result := service.CreateSignIn(t.Context(), EventSignIn, "203.0.113.7", "browser", user.ID, "", db, config)
	require.True(t, result.Blocked)
	service.SendSignInNotification(t.Context(), result)
	select {
	case <-sender.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("expected a blocked sign-in notification")
	}
}

type blockedNotificationSender struct {
	loginNotificationSender
	blocked chan string
}

func (s *blockedNotificationSender) SendBlockedLogin(_ context.Context, _ *appconfig.AppConfigModel, _, email, _, _, _, _, _ string, _ time.Time) error {
	s.blocked <- email
	return nil
}
