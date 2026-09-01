package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

const sessionCookie = "genesis_session"

type server struct {
	db                      *pgxpool.Pool
	webRoot                 string
	transferRoot            string
	publicURL               string
	cookieSecure            bool
	actionSigner            *actionSigner
	loginMu                 sync.Mutex
	loginFails              map[string]*loginAttempt
	authSessions            sync.Map
	csrfMu                  sync.Mutex
	csrfTokens              sync.Map
	desktopAgentCredentials sync.Map
	desktopFrames           sync.Map
	desktopFrameLanes       sync.Map
	desktopFrameLocksMu     sync.Mutex
	desktopFrameLocks       sync.Map
	desktopFrameDiagnostics sync.Map
	desktopFrameSignals     sync.Map
	desktopAgentSeen        sync.Map
	desktopInputAcks        sync.Map
	desktopClipboardAcks    sync.Map
	desktopClipboardImages  sync.Map
	desktopViewerTouches    sync.Map
	desktopSessionAccess    sync.Map
	desktopSessionRuntime   sync.Map
	desktopDeviceSessions   sync.Map
	desktopInputQueues      sync.Map
	networkTunnels          sync.Map
	transferProgressSignals sync.Map
	transferOfferSignals    sync.Map
	transferChunkLocks      sync.Map
}

type loginAttempt struct {
	count       int
	windowStart time.Time
}

type authState struct {
	UserID             string
	Username           string
	Role               string
	MustChangePassword bool
	SessionID          string
	CSRFHash           []byte
	cacheKey           string
}

type cachedAuthState struct {
	Auth            authState
	ValidatedAt     time.Time
	LastUsedTouchAt time.Time
}

// csrfTokens keeps the recoverable CSRF value for an authenticated session.
// The database intentionally stores only the hash.  Without this small
// process-local cache every GET /auth/me had to rotate the token; two tabs (or
// two concurrent application boot requests) could then return tokens in the
// opposite order and leave the client holding an already invalid value.
// A server restart simply makes the first /auth/me request mint one new value.
type cachedCSRFToken struct {
	Token     string
	ExpiresAt time.Time
}

func (s *server) invalidateAuthSession(sessionID string) {
	s.csrfTokens.Delete(sessionID)
	s.authSessions.Range(func(key, value any) bool {
		if entry, ok := value.(cachedAuthState); !ok || entry.Auth.SessionID == sessionID {
			s.authSessions.Delete(key)
		}
		return true
	})
}

func (s *server) invalidateAuthUser(userID string) {
	s.authSessions.Range(func(key, value any) bool {
		if entry, ok := value.(cachedAuthState); !ok || entry.Auth.UserID == userID {
			if ok {
				s.csrfTokens.Delete(entry.Auth.SessionID)
			}
			s.authSessions.Delete(key)
		}
		return true
	})
}

type agentJobPayload struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload"`
	TimeoutSeconds int            `json:"timeoutSeconds"`
}

type contextKey string

const authKey contextKey = "auth"

var schemaStatements = []string{
	`CREATE EXTENSION IF NOT EXISTS pgcrypto`,
	`CREATE TABLE IF NOT EXISTS users (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        username text NOT NULL,
        password_hash text NOT NULL,
        role text NOT NULL DEFAULT 'technician' CHECK (role IN ('owner','admin','technician','viewer')),
        must_change_password boolean NOT NULL DEFAULT false,
        disabled boolean NOT NULL DEFAULT false,
        created_at timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now()
    )`,
	`CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users (lower(username))`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT ''`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at timestamptz`,
	`CREATE TABLE IF NOT EXISTS sessions (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        token_hash bytea NOT NULL UNIQUE,
        csrf_hash bytea NOT NULL,
        ip_address inet,
        user_agent text NOT NULL DEFAULT '',
        created_at timestamptz NOT NULL DEFAULT now(),
        last_used_at timestamptz NOT NULL DEFAULT now(),
        expires_at timestamptz NOT NULL
    )`,
	`CREATE INDEX IF NOT EXISTS sessions_token_hash_idx ON sessions(token_hash)`,
	`CREATE TABLE IF NOT EXISTS enrollment_tokens (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        name text NOT NULL,
        token_hash bytea NOT NULL UNIQUE,
        device_group text NOT NULL DEFAULT 'Основная группа',
        max_uses integer NOT NULL DEFAULT 1 CHECK (max_uses > 0 AND max_uses <= 1000),
        uses integer NOT NULL DEFAULT 0,
        expires_at timestamptz NOT NULL,
        disabled boolean NOT NULL DEFAULT false,
        created_by uuid REFERENCES users(id) ON DELETE SET NULL,
        created_at timestamptz NOT NULL DEFAULT now()
    )`,
	`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS token_value text`,
	`CREATE TABLE IF NOT EXISTS devices (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        connection_code varchar(12) NOT NULL UNIQUE,
        name text NOT NULL,
        hostname text NOT NULL DEFAULT '',
        device_group text NOT NULL DEFAULT 'Основная группа',
        os text NOT NULL DEFAULT 'unknown',
        os_version text NOT NULL DEFAULT '',
        arch text NOT NULL DEFAULT '',
        agent_version text NOT NULL DEFAULT '',
        public_ip inet,
        local_ips text[] NOT NULL DEFAULT '{}',
        logged_in_user text NOT NULL DEFAULT '',
        cpu_model text NOT NULL DEFAULT '',
        cpu_load_percent double precision NOT NULL DEFAULT 0,
        memory_bytes bigint NOT NULL DEFAULT 0,
        memory_used_bytes bigint NOT NULL DEFAULT 0,
        disk_total_bytes bigint NOT NULL DEFAULT 0,
        disk_free_bytes bigint NOT NULL DEFAULT 0,
        uptime_seconds bigint NOT NULL DEFAULT 0,
        secret_hash bytea NOT NULL,
        enrolled_at timestamptz NOT NULL DEFAULT now(),
        last_seen timestamptz NOT NULL DEFAULT now(),
        updated_at timestamptz NOT NULL DEFAULT now()
    )`,
	`CREATE INDEX IF NOT EXISTS devices_last_seen_idx ON devices(last_seen DESC)`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS desired_name text NOT NULL DEFAULT ''`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS install_mode text NOT NULL DEFAULT 'unknown'`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS privileged boolean NOT NULL DEFAULT false`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS pending_removal boolean NOT NULL DEFAULT false`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS enrollment_token_id uuid REFERENCES enrollment_tokens(id) ON DELETE SET NULL`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS desktop_secret_hash bytea`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS access_password_hash text NOT NULL DEFAULT ''`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS access_protected_at timestamptz`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS cpu_load_percent double precision NOT NULL DEFAULT 0`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS memory_used_bytes bigint NOT NULL DEFAULT 0`,
	`ALTER TABLE devices ADD COLUMN IF NOT EXISTS uptime_seconds bigint NOT NULL DEFAULT 0`,
	`CREATE INDEX IF NOT EXISTS devices_enrollment_token_idx ON devices(enrollment_token_id,enrolled_at DESC)`,
	`CREATE TABLE IF NOT EXISTS device_access_unlocks (
        device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
        session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
        unlocked_at timestamptz NOT NULL DEFAULT now(),
        expires_at timestamptz NOT NULL,
        PRIMARY KEY(device_id,session_id)
    )`,
	`CREATE INDEX IF NOT EXISTS device_access_unlocks_expires_idx ON device_access_unlocks(expires_at)`,
	`CREATE TABLE IF NOT EXISTS agent_jobs (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
        created_by uuid REFERENCES users(id) ON DELETE SET NULL,
		job_type text NOT NULL CHECK (job_type IN ('shell','inventory','uninstall','files_list','files_read','files_write','action','tunnel')),
        payload jsonb NOT NULL DEFAULT '{}'::jsonb,
        status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed','cancelled','expired')),
        timeout_seconds integer NOT NULL DEFAULT 30 CHECK (timeout_seconds BETWEEN 5 AND 60),
        output text NOT NULL DEFAULT '',
        error_text text NOT NULL DEFAULT '',
        exit_code integer,
        created_at timestamptz NOT NULL DEFAULT now(),
        started_at timestamptz,
        completed_at timestamptz,
        expires_at timestamptz NOT NULL DEFAULT (now() + interval '30 minutes'),
        updated_at timestamptz NOT NULL DEFAULT now()
    )`,
	`CREATE INDEX IF NOT EXISTS agent_jobs_device_created_idx ON agent_jobs(device_id,created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS agent_jobs_queue_idx ON agent_jobs(device_id,status,created_at)`,
	`DO $$
    BEGIN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conrelid='agent_jobs'::regclass
              AND conname='agent_jobs_job_type_check'
              AND pg_get_constraintdef(oid) LIKE '%tunnel%'
        ) THEN
            ALTER TABLE agent_jobs DROP CONSTRAINT IF EXISTS agent_jobs_job_type_check;
            ALTER TABLE agent_jobs ADD CONSTRAINT agent_jobs_job_type_check CHECK (job_type IN ('shell','inventory','uninstall','files_list','files_read','files_write','action','tunnel'));
        END IF;
    END $$`,
	`DO $$
    BEGIN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conrelid='agent_jobs'::regclass
              AND conname='agent_jobs_timeout_seconds_check'
              AND pg_get_constraintdef(oid) LIKE '%900%'
        ) THEN
            ALTER TABLE agent_jobs DROP CONSTRAINT IF EXISTS agent_jobs_timeout_seconds_check;
            ALTER TABLE agent_jobs ADD CONSTRAINT agent_jobs_timeout_seconds_check CHECK (timeout_seconds BETWEEN 5 AND 900);
        END IF;
    END $$`,
	`CREATE TABLE IF NOT EXISTS integration_tokens (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        name text NOT NULL,
        token_hash bytea NOT NULL UNIQUE,
        scopes text[] NOT NULL DEFAULT '{}',
        expires_at timestamptz NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        last_used_at timestamptz,
        revoked_at timestamptz
    )`,
	`CREATE INDEX IF NOT EXISTS integration_tokens_user_idx ON integration_tokens(user_id,created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS network_tunnels (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
        created_by uuid REFERENCES users(id) ON DELETE SET NULL,
        protocol text NOT NULL CHECK (protocol IN ('rdp','ssh')),
        target_host inet NOT NULL,
        target_port integer NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
        client_token_hash bytea NOT NULL,
        status text NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting','connected','ended','failed','expired')),
        created_at timestamptz NOT NULL DEFAULT now(),
        agent_connected_at timestamptz,
        client_connected_at timestamptz,
        connected_at timestamptz,
        ended_at timestamptz,
        expires_at timestamptz NOT NULL DEFAULT (now() + interval '2 hours'),
        error_text text NOT NULL DEFAULT ''
    )`,
	`CREATE INDEX IF NOT EXISTS network_tunnels_device_created_idx ON network_tunnels(device_id,created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS network_tunnels_expires_idx ON network_tunnels(expires_at)`,
	`CREATE TABLE IF NOT EXISTS action_jobs (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
        requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
        requested_via text NOT NULL DEFAULT 'web' CHECK (requested_via IN ('web','mcp')),
        action_type text NOT NULL,
        parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
        risk_level text NOT NULL CHECK (risk_level IN ('read','high','critical')),
        status text NOT NULL CHECK (status IN ('awaiting_approval','queued','running','succeeded','failed','cancelled','expired')),
        approval_required boolean NOT NULL DEFAULT true,
        approved_by uuid REFERENCES users(id) ON DELETE SET NULL,
        approved_at timestamptz,
        execution_job_id uuid UNIQUE REFERENCES agent_jobs(id) ON DELETE SET NULL,
        plan jsonb NOT NULL DEFAULT '{}'::jsonb,
        rollback_plan jsonb NOT NULL DEFAULT '{}'::jsonb,
        idempotency_key text NOT NULL DEFAULT '',
        request_hash text NOT NULL,
        nonce text NOT NULL UNIQUE,
        signature text NOT NULL DEFAULT '',
        output text NOT NULL DEFAULT '',
        error_text text NOT NULL DEFAULT '',
        exit_code integer,
        created_at timestamptz NOT NULL DEFAULT now(),
        expires_at timestamptz NOT NULL,
        started_at timestamptz,
        completed_at timestamptz,
        updated_at timestamptz NOT NULL DEFAULT now()
    )`,
	`CREATE INDEX IF NOT EXISTS action_jobs_device_created_idx ON action_jobs(device_id,created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS action_jobs_status_idx ON action_jobs(status,created_at DESC)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS action_jobs_idempotency_idx ON action_jobs(requested_by,idempotency_key) WHERE idempotency_key<>''`,
	`CREATE TABLE IF NOT EXISTS audit_events (
        id bigserial PRIMARY KEY,
        actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
        actor_device_id uuid REFERENCES devices(id) ON DELETE SET NULL,
        event_type text NOT NULL,
        target_type text NOT NULL DEFAULT '',
        target_id text NOT NULL DEFAULT '',
        ip_address inet,
        details jsonb NOT NULL DEFAULT '{}'::jsonb,
        created_at timestamptz NOT NULL DEFAULT now()
    )`,
	`CREATE INDEX IF NOT EXISTS audit_events_created_at_idx ON audit_events(created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS remote_desktop_sessions (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
        created_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','ended','expired')),
		control_enabled boolean NOT NULL DEFAULT true,
		target_fps integer NOT NULL DEFAULT 0,
		cursor_visible boolean NOT NULL DEFAULT false,
        frame bytea,
        frame_width integer NOT NULL DEFAULT 0,
        frame_height integer NOT NULL DEFAULT 0,
        frame_at timestamptz,
        agent_seen_at timestamptz,
        agent_error text NOT NULL DEFAULT '',
        viewer_seen_at timestamptz NOT NULL DEFAULT now(),
        expires_at timestamptz NOT NULL DEFAULT (now()+interval '30 minutes'),
        created_at timestamptz NOT NULL DEFAULT now(),
        ended_at timestamptz
    )`,
	`ALTER TABLE remote_desktop_sessions ADD COLUMN IF NOT EXISTS agent_error text NOT NULL DEFAULT ''`,
	`ALTER TABLE remote_desktop_sessions ADD COLUMN IF NOT EXISTS target_fps integer NOT NULL DEFAULT 0`,
	`ALTER TABLE remote_desktop_sessions ALTER COLUMN target_fps SET DEFAULT 0`,
	`ALTER TABLE remote_desktop_sessions ADD COLUMN IF NOT EXISTS cursor_visible boolean NOT NULL DEFAULT false`,
	`CREATE INDEX IF NOT EXISTS remote_desktop_device_idx ON remote_desktop_sessions(device_id,status,created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS remote_desktop_inputs (
        id bigserial PRIMARY KEY,
        session_id uuid NOT NULL REFERENCES remote_desktop_sessions(id) ON DELETE CASCADE,
        event jsonb NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now(),
        delivered_at timestamptz
    )`,
	`CREATE INDEX IF NOT EXISTS remote_desktop_inputs_queue_idx ON remote_desktop_inputs(session_id,id) WHERE delivered_at IS NULL`,
	`CREATE TABLE IF NOT EXISTS remote_file_transfers (
        id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
        created_by uuid REFERENCES users(id) ON DELETE SET NULL,
        direction text NOT NULL CHECK (direction IN ('to_device','from_device')),
        file_name text NOT NULL,
        remote_path text NOT NULL,
		size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 0 AND 53687091200),
        received_bytes bigint NOT NULL DEFAULT 0,
        status text NOT NULL CHECK (status IN ('uploading','queued','transferring','ready','completed','failed','cancelled','expired')),
        error_text text NOT NULL DEFAULT '',
        created_at timestamptz NOT NULL DEFAULT now(),
        started_at timestamptz,
        completed_at timestamptz,
        expires_at timestamptz NOT NULL DEFAULT (now()+interval '24 hours'),
        updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`ALTER TABLE remote_file_transfers ADD COLUMN IF NOT EXISTS source_ready boolean NOT NULL DEFAULT false`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid='remote_file_transfers'::regclass
			  AND conname='remote_file_transfers_size_bytes_check'
			  AND pg_get_constraintdef(oid) LIKE '%53687091200%'
		) THEN
			ALTER TABLE remote_file_transfers DROP CONSTRAINT IF EXISTS remote_file_transfers_size_bytes_check;
			ALTER TABLE remote_file_transfers ADD CONSTRAINT remote_file_transfers_size_bytes_check CHECK (size_bytes BETWEEN 0 AND 53687091200);
		END IF;
	END $$`,
	`CREATE INDEX IF NOT EXISTS remote_file_transfers_queue_idx ON remote_file_transfers(device_id,status,created_at)`,
}

func main() {
	ctx := context.Background()
	databaseURL := mustEnv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	s := &server{
		db:           pool,
		webRoot:      envOr("WEB_ROOT", "../web/dist"),
		transferRoot: envOr("TRANSFER_ROOT", "/tmp/remoteit-transfers"),
		publicURL:    strings.TrimRight(envOr("REMOTEIT_PUBLIC_URL", "https://supportgenesis.ru"), "/"),
		cookieSecure: envOr("GENESIS_COOKIE_SECURE", "true") != "false",
		loginFails:   make(map[string]*loginAttempt),
	}
	s.actionSigner, err = newActionSignerFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(s.transferRoot, 0o700); err != nil {
		log.Fatal(err)
	}
	if err := s.migrate(ctx); err != nil {
		log.Fatal(err)
	}
	if err := s.ensureAdmin(ctx); err != nil {
		log.Fatal(err)
	}
	go s.runMaintenance(ctx)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Ordinary HTTP requests remain bounded, but upgraded WebSocket streams are
	// intentionally long lived. Applying chi's Timeout middleware to an upgraded
	// connection cancels an otherwise healthy remote-control session after two
	// hours and then tries to write an HTTP timeout response to a hijacked socket.
	r.Use(timeoutUnlessWebSocket(2 * time.Hour))
	r.Use(s.securityHeaders)
	r.Get("/healthz", s.health)
	r.Post("/api/auth/login", s.login)
	r.Post("/api/public/install/resolve", s.resolvePublicInstallCode)
	r.Post("/api/public/install/windows-agent", s.downloadPublicWindowsAgent)
	r.Post("/api/public/install/unix-agent", s.downloadPublicUnixAgent)
	r.Post("/api/public/install/macos-agent", s.downloadPublicMacOSAgent)
	r.Post("/api/agent/enroll", s.enrollAgent)
	r.Post("/api/agent/verify", s.verifyAgentRegistration)
	r.Post("/api/agent/heartbeat", s.agentHeartbeat)
	r.Post("/api/agent/uninstall-complete", s.confirmAgentUninstall)
	r.Post("/api/agent/jobs/{id}/result", s.agentJobResult)
	r.Get("/api/desktop/agent/session", s.desktopAgentSession)
	r.Post("/api/desktop/agent/sessions/{id}/frame", s.desktopAgentFrame)
	r.Get("/api/desktop/agent/sessions/{id}/stream", s.desktopAgentFrameStream)
	r.Post("/api/desktop/agent/sessions/{id}/status", s.desktopAgentStatus)
	r.Get("/api/desktop/agent/sessions/{id}/inputs", s.desktopAgentInputs)
	r.Post("/api/desktop/agent/sessions/{id}/clipboard-image", s.desktopAgentUploadClipboardImage)
	r.Get("/api/desktop/agent/sessions/{id}/clipboard-image/{sequence}", s.desktopAgentDownloadClipboardImage)
	r.Get("/api/agent/file-transfers/next", s.agentNextFileTransfer)
	r.Get("/api/agent/file-transfers/{id}", s.agentFileTransferStatus)
	r.Get("/api/agent/file-transfers/{id}/data", s.agentDownloadTransferChunk)
	r.Put("/api/agent/file-transfers/{id}/data", s.agentUploadTransferChunk)
	r.Post("/api/agent/file-transfers/{id}/complete", s.agentCompleteFileTransfer)
	r.Post("/api/agent/file-transfers/{id}/fail", s.agentFailFileTransfer)
	r.Get("/api/network-tunnels/{id}/agent", s.networkTunnelAgent)
	r.Get("/api/network-tunnels/{id}/client", s.networkTunnelClient)
	r.Route("/api", func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/auth/me", s.me)
		r.Get("/auth/sessions", s.listSessions)
		r.With(s.requireCSRF).Post("/auth/logout", s.logout)
		r.With(s.requireCSRF).Post("/auth/change-password", s.changePassword)
		r.With(s.requireCSRF).Delete("/auth/sessions/{id}", s.revokeSession)
		r.Get("/devices", s.listDevices)
		r.With(s.requireCSRF).Put("/devices/{id}/access-protection", s.setDeviceAccessProtection)
		r.With(s.requireCSRF).Delete("/devices/{id}/access-protection", s.removeDeviceAccessProtection)
		r.With(s.requireCSRF).Post("/devices/{id}/unlock", s.unlockDeviceAccess)
		r.With(s.requireCSRF).Delete("/devices/{id}/unlock", s.lockDeviceAccess)
		r.With(s.requireCSRF).Patch("/devices/{id}", s.renameDevice)
		r.With(s.requireCSRF).Delete("/devices/{id}", s.deleteDevice)
		r.With(s.requireCSRF).Delete("/devices/{id}/forget", s.forgetDevice)
		r.With(s.requireCSRF).Post("/devices/{id}/uninstall", s.requestDeviceUninstall)
		r.With(s.requireCSRF).Post("/devices/{id}/desktop-sessions", s.startDesktopSession)
		r.Get("/devices/{id}/jobs", s.listDeviceJobs)
		r.With(s.requireCSRF).Post("/devices/{id}/jobs", s.createDeviceJob)
		r.With(s.requireCSRF).Post("/jobs/{id}/cancel", s.cancelDeviceJob)
		r.Get("/desktop-sessions/{id}", s.desktopSessionStatus)
		r.Get("/desktop-sessions/{id}/frame", s.desktopSessionFrame)
		r.Get("/desktop-sessions/{id}/clipboard-image", s.desktopSessionClipboardImage)
		r.With(s.requireCSRF).Post("/desktop-sessions/{id}/clipboard-image", s.desktopSessionUploadClipboardImage)
		r.Get("/desktop-sessions/{id}/stream", s.desktopSessionStream)
		r.With(s.requireCSRF).Patch("/desktop-sessions/{id}", s.updateDesktopSession)
		r.With(s.requireCSRF).Post("/desktop-sessions/{id}/input", s.desktopSessionInput)
		r.With(s.requireCSRF).Delete("/desktop-sessions/{id}", s.endDesktopSession)
		r.With(s.requireCSRF).Post("/devices/{id}/file-transfers", s.createFileTransfer)
		r.Get("/file-transfers/{id}", s.fileTransferStatus)
		r.With(s.requireCSRF).Put("/file-transfers/{id}/data", s.uploadTransferChunk)
		r.With(s.requireCSRF).Post("/file-transfers/{id}/ready", s.readyFileTransfer)
		r.Get("/file-transfers/{id}/download", s.downloadFileTransfer)
		r.With(s.requireCSRF).Delete("/file-transfers/{id}", s.cancelFileTransfer)
		r.With(s.requireCSRF).Post("/network-tunnels", s.createNetworkTunnel)
		r.Get("/network-tunnels/{id}", s.networkTunnelStatus)
		r.Get("/users", s.listUsers)
		r.With(s.requireCSRF).Post("/users", s.createUser)
		r.With(s.requireCSRF).Patch("/users/{id}", s.updateUser)
		r.With(s.requireCSRF).Post("/users/{id}/reset-password", s.resetUserPassword)
		r.Get("/enrollment-tokens", s.listEnrollmentTokens)
		r.Get("/enrollment-tokens/{id}/windows-agent", s.downloadBoundWindowsAgent)
		r.Get("/enrollment-tokens/{id}/unix-agent", s.downloadBoundUnixAgent)
		r.Get("/enrollment-tokens/{id}/macos-agent", s.downloadBoundMacOSAgent)
		r.With(s.requireCSRF).Post("/enrollment-tokens", s.createEnrollmentToken)
		r.With(s.requireCSRF).Patch("/enrollment-tokens/{id}", s.updateEnrollmentToken)
		r.With(s.requireCSRF).Delete("/enrollment-tokens/{id}", s.deleteEnrollmentToken)
		r.Get("/audit", s.listAudit)
		r.Get("/integration-tokens", s.listIntegrationTokens)
		r.With(s.requireCSRF).Post("/integration-tokens", s.createIntegrationToken)
		r.With(s.requireCSRF).Delete("/integration-tokens/{id}", s.revokeIntegrationToken)
		r.Get("/action-jobs", s.listActionJobs)
		r.Get("/action-jobs/{id}", s.getActionJob)
		r.With(s.requireCSRF).Post("/action-jobs/plan", s.planActionJob)
		r.With(s.requireCSRF).Post("/action-jobs", s.createActionJobFromWeb)
		r.With(s.requireCSRF).Post("/action-jobs/{id}/approve", s.approveActionJob)
		r.With(s.requireCSRF).Post("/action-jobs/{id}/cancel", s.cancelActionJob)
		r.With(s.requireCSRF).Post("/ai/analyze", s.aiAnalyze)
	})
	r.Route("/api/integration/v1", func(r chi.Router) {
		r.Use(s.requireIntegrationToken)
		r.Get("/devices", s.integrationListDevices)
		r.Get("/devices/{id}", s.integrationGetDevice)
		r.Post("/actions/plan", s.integrationPlanAction)
		r.Post("/actions", s.integrationCreateAction)
		r.Get("/actions/{id}", s.integrationGetAction)
		r.Post("/actions/{id}/cancel", s.integrationCancelAction)
	})
	r.NotFound(s.serveSPA)

	port := envOr("PORT", "8080")
	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      0,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Printf("RemoteIt server listening on :%s", port)
	log.Fatal(httpServer.ListenAndServe())
}

func timeoutUnlessWebSocket(timeout time.Duration) func(http.Handler) http.Handler {
	withTimeout := middleware.Timeout(timeout)
	return func(next http.Handler) http.Handler {
		timed := withTimeout(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if websocketUpgradeRequested(r) {
				next.ServeHTTP(w, r)
				return
			}
			timed.ServeHTTP(w, r)
		})
	}
}

func websocketUpgradeRequested(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet || !desktopWebSocketPath(r.URL.Path) || !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, value := range r.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

func desktopWebSocketPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 6 && parts[0] == "api" && parts[1] == "desktop" && parts[2] == "agent" && parts[3] == "sessions" && parts[4] != "" && parts[5] == "stream" {
		return true
	}
	return len(parts) == 4 && parts[0] == "api" && parts[1] == "desktop-sessions" && parts[2] != "" && parts[3] == "stream"
}

func (s *server) migrate(ctx context.Context) error {
	for _, statement := range schemaStatements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

func (s *server) ensureAdmin(ctx context.Context) error {
	username := envOr("GENESIS_ADMIN_USERNAME", "Admin")
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE lower(username)=lower($1))`, username).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	password := os.Getenv("GENESIS_ADMIN_PASSWORD")
	if password == "" {
		return errors.New("GENESIS_ADMIN_PASSWORD is required for the first startup")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO users (username,password_hash,role,must_change_password) VALUES ($1,$2,'owner',true)`, username, hash)
	return err
}

func (s *server) runMaintenance(ctx context.Context) {
	run := func() {
		statements := []string{
			`DELETE FROM sessions WHERE expires_at<=now()`,
			`DELETE FROM device_access_unlocks WHERE expires_at<=now()`,
			`DELETE FROM agent_jobs WHERE completed_at<now()-interval '90 days'`,
			`DELETE FROM enrollment_tokens WHERE expires_at<now()-interval '90 days'`,
			`UPDATE remote_desktop_sessions SET status='expired',frame=NULL,ended_at=now() WHERE status='active' AND (expires_at<=now() OR viewer_seen_at<now()-interval '45 seconds')`,
			`DELETE FROM remote_desktop_inputs WHERE delivered_at<now()-interval '1 hour'`,
			`DELETE FROM remote_desktop_sessions WHERE status<>'active' AND ended_at<now()-interval '7 days'`,
			`DELETE FROM audit_events WHERE created_at<now()-interval '180 days'`,
		}
		for _, statement := range statements {
			if _, err := s.db.Exec(ctx, statement); err != nil {
				log.Printf("maintenance query failed: %v", err)
			}
		}
		s.removeExpiredTransferFiles()
		s.removeExpiredNetworkTunnels()
		s.pruneDesktopRuntimeState(time.Now().Add(-2 * time.Hour))
		s.loginMu.Lock()
		for ip, attempt := range s.loginFails {
			if time.Since(attempt.windowStart) > 15*time.Minute {
				delete(s.loginFails, ip)
			}
		}
		s.loginMu.Unlock()
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (s *server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.loginBlocked(ip) {
		writeError(w, http.StatusTooManyRequests, "Слишком много попыток. Повторите позже.")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var user authState
	var passwordHash string
	var disabled bool
	err := s.db.QueryRow(r.Context(), `SELECT id,username,password_hash,role,must_change_password,disabled FROM users WHERE lower(username)=lower($1)`, strings.TrimSpace(in.Username)).Scan(
		&user.UserID, &user.Username, &passwordHash, &user.Role, &user.MustChangePassword, &disabled,
	)
	if err != nil || disabled || !verifyPassword(in.Password, passwordHash) {
		s.recordLoginFailure(ip)
		s.audit(r.Context(), nil, nil, "auth.login_failed", "user", truncate(strings.TrimSpace(in.Username), 64), ip, map[string]any{})
		writeError(w, http.StatusUnauthorized, "Неверный логин или пароль")
		return
	}
	s.clearLoginFailures(ip)
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE expires_at<=now()`)
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id IN (SELECT id FROM sessions WHERE user_id=$1 ORDER BY created_at DESC OFFSET 9)`, user.UserID)
	token := randomToken(32)
	csrf := randomToken(32)
	var sessionID string
	err = s.db.QueryRow(r.Context(), `INSERT INTO sessions (user_id,token_hash,csrf_hash,ip_address,user_agent,expires_at) VALUES ($1,$2,$3,NULLIF($4,'')::inet,$5,now()+interval '12 hours') RETURNING id`,
		user.UserID, tokenHash(token), tokenHash(csrf), ip, truncate(r.UserAgent(), 512)).Scan(&sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать сессию")
		return
	}
	s.csrfTokens.Store(sessionID, cachedCSRFToken{Token: csrf, ExpiresAt: time.Now().UTC().Add(12 * time.Hour)})
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 60 * 60})
	_, _ = s.db.Exec(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, user.UserID)
	s.audit(r.Context(), &user, nil, "auth.login", "user", user.UserID, ip, map[string]any{})
	writeJSON(w, http.StatusOK, map[string]any{"user": userResponse(user), "csrfToken": csrf})
}

func (s *server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "Требуется авторизация")
			return
		}
		now := time.Now().UTC()
		hash := tokenHash(cookie.Value)
		cacheKey := string(hash)
		var a authState
		cached, cacheHit := s.authSessions.Load(cacheKey)
		entry, cacheValid := cached.(cachedAuthState)
		if cacheHit && cacheValid && now.Sub(entry.ValidatedAt) < time.Second {
			a = entry.Auth
		} else {
			err = s.db.QueryRow(r.Context(), `SELECT u.id,u.username,u.role,u.must_change_password,s.id,s.csrf_hash FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.disabled=false`, hash).Scan(
				&a.UserID, &a.Username, &a.Role, &a.MustChangePassword, &a.SessionID, &a.CSRFHash,
			)
			if err != nil {
				s.authSessions.Delete(cacheKey)
				http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteStrictMode})
				writeError(w, http.StatusUnauthorized, "Сессия истекла")
				return
			}
			entry = cachedAuthState{Auth: a, ValidatedAt: now, LastUsedTouchAt: now}
		}
		a.cacheKey = cacheKey
		entry.Auth = a
		if a.MustChangePassword && r.URL.Path != "/api/auth/me" && r.URL.Path != "/api/auth/change-password" && r.URL.Path != "/api/auth/logout" {
			writeError(w, http.StatusForbidden, "Перед продолжением необходимо заменить временный пароль")
			return
		}
		if now.Sub(entry.LastUsedTouchAt) >= 30*time.Second {
			_, _ = s.db.Exec(r.Context(), `UPDATE sessions SET last_used_at=now() WHERE id=$1`, a.SessionID)
			entry.LastUsedTouchAt = now
		}
		s.authSessions.Store(cacheKey, entry)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey, &a)))
	})
}

func (s *server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := currentAuth(r)
		provided := r.Header.Get("X-CSRF-Token")
		if provided == "" || subtle.ConstantTimeCompare(tokenHash(provided), a.CSRFHash) != 1 {
			writeError(w, http.StatusForbidden, "Недействительный защитный токен")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	// Serialise the cache miss so parallel boot requests receive the same token,
	// not two different values where the last database update invalidates the
	// response that happens to reach the browser last.
	s.csrfMu.Lock()
	cached, exists := s.csrfTokens.Load(a.SessionID)
	csrf := ""
	if entry, ok := cached.(cachedCSRFToken); exists && ok && entry.ExpiresAt.After(time.Now().UTC()) {
		csrf = entry.Token
	}
	if csrf == "" {
		csrf = randomToken(32)
		if _, err := s.db.Exec(r.Context(), `UPDATE sessions SET csrf_hash=$1,last_used_at=now() WHERE id=$2`, tokenHash(csrf), a.SessionID); err != nil {
			s.csrfMu.Unlock()
			writeError(w, http.StatusInternalServerError, "Не удалось обновить сессию")
			return
		}
		s.csrfTokens.Store(a.SessionID, cachedCSRFToken{Token: csrf, ExpiresAt: time.Now().UTC().Add(12 * time.Hour)})
	}
	s.csrfMu.Unlock()
	// requireAuth may have populated its one-second hash cache just before a
	// concurrent /auth/me rotated the database value. Always evict it here.
	s.authSessions.Delete(a.cacheKey)
	writeJSON(w, http.StatusOK, map[string]any{"user": userResponse(*a), "csrfToken": csrf})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id=$1`, a.SessionID)
	s.invalidateAuthSession(a.SessionID)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteStrictMode})
	s.audit(r.Context(), a, nil, "auth.logout", "user", a.UserID, clientIP(r), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !validAccountPassword(in.NewPassword) {
		writeError(w, http.StatusBadRequest, "Пароль должен содержать от 4 до 256 символов")
		return
	}
	var currentHash string
	if err := s.db.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, a.UserID).Scan(&currentHash); err != nil || !verifyPassword(in.CurrentPassword, currentHash) {
		writeError(w, http.StatusUnauthorized, "Текущий пароль указан неверно")
		return
	}
	newHash, err := hashPassword(in.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось обновить пароль")
		return
	}
	_, err = s.db.Exec(r.Context(), `UPDATE users SET password_hash=$1,must_change_password=false,updated_at=now() WHERE id=$2`, newHash, a.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось обновить пароль")
		return
	}
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1 AND id<>$2`, a.UserID, a.SessionID)
	s.invalidateAuthUser(a.UserID)
	s.audit(r.Context(), a, nil, "auth.password_changed", "user", a.UserID, clientIP(r), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	rows, err := s.db.Query(r.Context(), `SELECT id::text,COALESCE(host(ip_address),''),user_agent,created_at,last_used_at,expires_at FROM sessions WHERE user_id=$1 AND expires_at>now() ORDER BY last_used_at DESC`, a.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить активные сессии")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, ip, userAgent string
		var createdAt, lastUsedAt, expiresAt time.Time
		if err := rows.Scan(&id, &ip, &userAgent, &createdAt, &lastUsedAt, &expiresAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Не удалось прочитать активные сессии")
			return
		}
		items = append(items, map[string]any{
			"id": id, "ip": ip, "userAgent": userAgent, "createdAt": createdAt,
			"lastUsedAt": lastUsedAt, "expiresAt": expiresAt, "current": id == a.SessionID,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить активные сессии")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": items})
}

func (s *server) revokeSession(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	sessionID := chi.URLParam(r, "id")
	if sessionID == a.SessionID {
		writeError(w, http.StatusBadRequest, "Текущую сессию завершите кнопкой выхода")
		return
	}
	result, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id::text=$1 AND user_id=$2`, sessionID, a.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить сессию")
		return
	}
	count := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusNotFound, "Сессия не найдена")
		return
	}
	s.invalidateAuthSession(sessionID)
	s.audit(r.Context(), a, nil, "auth.session_revoked", "session", sessionID, clientIP(r), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listDevices(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	rows, err := s.db.Query(r.Context(), `SELECT id,connection_code,name,hostname,device_group,os,os_version,arch,agent_version,COALESCE(host(public_ip),''),local_ips,logged_in_user,cpu_model,cpu_load_percent,memory_bytes,memory_used_bytes,disk_total_bytes,disk_free_bytes,uptime_seconds,install_mode,privileged,pending_removal,enrolled_at,last_seen,(last_seen>now()-interval '90 seconds'),(access_password_hash<>''),($1='owner' OR access_password_hash='' OR EXISTS(SELECT 1 FROM device_access_unlocks u WHERE u.device_id=devices.id AND u.session_id=$2 AND u.expires_at>now())) FROM devices ORDER BY name`, a.Role, a.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить устройства")
		return
	}
	defer rows.Close()
	devices := make([]map[string]any, 0)
	for rows.Next() {
		var id, code, name, hostname, group, osName, osVersion, arch, agentVersion, publicIP, currentUser, cpu, installMode string
		var localIPs []string
		var memory, memoryUsed, diskTotal, diskFree, uptime int64
		var cpuLoad float64
		var enrolled, lastSeen time.Time
		var online, privileged, pendingRemoval, accessProtected, accessGranted bool
		if err := rows.Scan(&id, &code, &name, &hostname, &group, &osName, &osVersion, &arch, &agentVersion, &publicIP, &localIPs, &currentUser, &cpu, &cpuLoad, &memory, &memoryUsed, &diskTotal, &diskFree, &uptime, &installMode, &privileged, &pendingRemoval, &enrolled, &lastSeen, &online, &accessProtected, &accessGranted); err != nil {
			continue
		}
		devices = append(devices, map[string]any{"id": id, "connectionCode": code, "name": name, "hostname": hostname, "group": group, "os": osName, "osVersion": osVersion, "arch": arch, "agentVersion": agentVersion, "publicIp": publicIP, "localIps": localIPs, "currentUser": currentUser, "cpuModel": cpu, "cpuLoadPercent": cpuLoad, "memoryBytes": memory, "memoryUsedBytes": memoryUsed, "diskTotalBytes": diskTotal, "diskFreeBytes": diskFree, "uptimeSeconds": uptime, "installMode": installMode, "privileged": privileged, "pendingRemoval": pendingRemoval, "enrolledAt": enrolled, "lastSeen": lastSeen, "online": online, "accessProtected": accessProtected, "accessGranted": accessGranted})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *server) renameDevice(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	if !s.requireDeviceAccess(w, r, chi.URLParam(r, "id")) {
		return
	}
	var in struct {
		Name  *string `json:"name"`
		Group *string `json:"group"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.Name == nil && in.Group == nil {
		writeError(w, http.StatusBadRequest, "Не указаны изменения")
		return
	}
	var name, group any
	details := map[string]any{}
	if in.Name != nil {
		value := strings.TrimSpace(*in.Name)
		if len([]rune(value)) < 1 || len([]rune(value)) > 64 {
			writeError(w, http.StatusBadRequest, "Название должно содержать от 1 до 64 символов")
			return
		}
		name = value
		details["name"] = value
	}
	if in.Group != nil {
		value := strings.TrimSpace(*in.Group)
		if len([]rune(value)) < 1 || len([]rune(value)) > 100 {
			writeError(w, http.StatusBadRequest, "Группа должна содержать от 1 до 100 символов")
			return
		}
		group = value
		details["group"] = value
	}
	result, err := s.db.Exec(r.Context(), `UPDATE devices SET name=COALESCE($1,name),desired_name=CASE WHEN $1::text IS NULL THEN desired_name ELSE $1 END,device_group=COALESCE($2,device_group),updated_at=now() WHERE id=$3`, name, group, chi.URLParam(r, "id"))
	if err != nil || result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	s.audit(r.Context(), a, nil, "device.updated", "device", chi.URLParam(r, "id"), clientIP(r), details)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	s.requestDeviceUninstall(w, r)
}

func (s *server) forgetDevice(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Удаление устройства доступно только владельцу и администраторам")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	s.removeDeviceTransferFiles(deviceID)
	result, err := s.db.Exec(r.Context(), `DELETE FROM devices WHERE id=$1`, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось удалить устройство")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	s.desktopAgentCredentials.Delete(deviceID)
	s.deleteDesktopFramesForDevice(deviceID)
	s.audit(r.Context(), a, nil, "device.forgotten", "device", deviceID, clientIP(r), map[string]any{"localAgentRemoved": false, "credentialsRevoked": true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) requestDeviceUninstall(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Удалённая деинсталляция доступна только владельцу и администраторам")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось подготовить удаление агента")
		return
	}
	defer tx.Rollback(r.Context())
	var pending bool
	var agentVersion string
	if err = tx.QueryRow(r.Context(), `SELECT pending_removal,agent_version FROM devices WHERE id=$1 FOR UPDATE`, deviceID).Scan(&pending, &agentVersion); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить устройство")
		return
	}
	if pending {
		writeError(w, http.StatusConflict, "Удаление агента уже ожидает подключения устройства")
		return
	}
	if !semanticVersionAtLeast(agentVersion, "0.6.0") {
		writeError(w, http.StatusConflict, "Для подтверждённого полного удаления сначала обновите RemoteIt Agent до версии 0.6.0")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE devices SET pending_removal=true,updated_at=now() WHERE id=$1`, deviceID); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE agent_jobs SET status='cancelled',error_text='Отменено перед удалением агента',completed_at=now(),updated_at=now() WHERE device_id=$1 AND status='queued'`, deviceID)
	}
	var jobID string
	var expires time.Time
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO agent_jobs (device_id,created_by,job_type,payload,timeout_seconds,expires_at) VALUES ($1,$2,'uninstall','{"requested":true}'::jsonb,30,now()+interval '7 days') RETURNING id,expires_at`, deviceID, a.UserID).Scan(&jobID, &expires)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось поставить удаление агента в очередь")
		return
	}
	s.audit(r.Context(), a, nil, "device.uninstall_requested", "device", deviceID, clientIP(r), map[string]any{"jobId": jobID})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": jobID, "expiresAt": expires})
}

func semanticVersionAtLeast(actual, required string) bool {
	parse := func(value string) []int {
		parts := strings.Split(strings.TrimSpace(value), ".")
		result := make([]int, len(parts))
		for index, part := range parts {
			part = strings.TrimLeftFunc(part, func(character rune) bool { return character < '0' || character > '9' })
			digits := strings.TrimRightFunc(part, func(character rune) bool { return character < '0' || character > '9' })
			result[index], _ = strconv.Atoi(digits)
		}
		return result
	}
	left, right := parse(actual), parse(required)
	length := max(len(left), len(right))
	for index := 0; index < length; index++ {
		leftValue, rightValue := 0, 0
		if index < len(left) {
			leftValue = left[index]
		}
		if index < len(right) {
			rightValue = right[index]
		}
		if leftValue != rightValue {
			return leftValue > rightValue
		}
	}
	return true
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT id,username,display_name,role,must_change_password,disabled,created_at,last_login_at FROM users ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, lower(username)`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить пользователей")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, username, displayName, role string
		var mustChange, disabled bool
		var created time.Time
		var lastLogin *time.Time
		if rows.Scan(&id, &username, &displayName, &role, &mustChange, &disabled, &created, &lastLogin) == nil {
			items = append(items, map[string]any{"id": id, "username": username, "displayName": displayName, "role": role, "mustChangePassword": mustChange, "disabled": disabled, "createdAt": created, "lastLoginAt": lastLogin})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": items})
}

var usernamePattern = regexp.MustCompile(`^[\p{L}\p{N}._-]{3,64}$`)

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	var in struct {
		Username          string `json:"username"`
		DisplayName       string `json:"displayName"`
		Role              string `json:"role"`
		TemporaryPassword string `json:"temporaryPassword"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = truncate(in.DisplayName, 100)
	if !usernamePattern.MatchString(in.Username) {
		writeError(w, http.StatusBadRequest, "Логин должен содержать 3–64 буквы, цифры, точки, дефисы или подчёркивания")
		return
	}
	if !validAccountPassword(in.TemporaryPassword) {
		writeError(w, http.StatusBadRequest, "Временный пароль должен содержать от 4 до 256 символов")
		return
	}
	if !validRole(in.Role) || in.Role == "owner" || (a.Role == "admin" && in.Role == "admin") {
		writeError(w, http.StatusForbidden, "Эту роль назначить нельзя")
		return
	}
	hash, err := hashPassword(in.TemporaryPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось защитить пароль")
		return
	}
	var id string
	err = s.db.QueryRow(r.Context(), `INSERT INTO users (username,display_name,password_hash,role,must_change_password) VALUES ($1,$2,$3,$4,true) RETURNING id`, in.Username, in.DisplayName, hash, in.Role).Scan(&id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "Такой логин уже существует")
			return
		}
		writeError(w, http.StatusInternalServerError, "Не удалось создать пользователя")
		return
	}
	s.audit(r.Context(), a, nil, "user.created", "user", id, clientIP(r), map[string]any{"username": in.Username, "role": in.Role})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "username": in.Username, "displayName": in.DisplayName, "role": in.Role, "mustChangePassword": true, "disabled": false})
}

func (s *server) updateUser(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	targetID := chi.URLParam(r, "id")
	if targetID == a.UserID {
		writeError(w, http.StatusBadRequest, "Нельзя изменить роль или состояние собственной учётной записи")
		return
	}
	var in struct {
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !validRole(in.Role) || in.Role == "owner" || (a.Role == "admin" && in.Role == "admin") {
		writeError(w, http.StatusForbidden, "Эту роль назначить нельзя")
		return
	}
	var currentRole string
	if err := s.db.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, targetID).Scan(&currentRole); err != nil {
		writeError(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if currentRole == "owner" || (a.Role == "admin" && currentRole == "admin") {
		writeError(w, http.StatusForbidden, "Недостаточно прав для изменения этой учётной записи")
		return
	}
	_, err := s.db.Exec(r.Context(), `UPDATE users SET role=$1,disabled=$2,updated_at=now() WHERE id=$3`, in.Role, in.Disabled, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось обновить пользователя")
		return
	}
	if in.Disabled {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, targetID)
	}
	s.invalidateAuthUser(targetID)
	s.audit(r.Context(), a, nil, "user.updated", "user", targetID, clientIP(r), map[string]any{"role": in.Role, "disabled": in.Disabled})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	targetID := chi.URLParam(r, "id")
	if targetID == a.UserID {
		writeError(w, http.StatusBadRequest, "Для собственной учётной записи используйте смену пароля")
		return
	}
	var in struct {
		TemporaryPassword string `json:"temporaryPassword"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !validAccountPassword(in.TemporaryPassword) {
		writeError(w, http.StatusBadRequest, "Временный пароль должен содержать от 4 до 256 символов")
		return
	}
	var targetRole string
	if err := s.db.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, targetID).Scan(&targetRole); err != nil {
		writeError(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if targetRole == "owner" || (a.Role == "admin" && targetRole == "admin") {
		writeError(w, http.StatusForbidden, "Недостаточно прав для сброса пароля этой учётной записи")
		return
	}
	hash, err := hashPassword(in.TemporaryPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось защитить пароль")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сбросить пароль")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `UPDATE users SET password_hash=$1,must_change_password=true,updated_at=now() WHERE id=$2`, hash, targetID); err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, targetID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сбросить пароль")
		return
	}
	s.invalidateAuthUser(targetID)
	s.audit(r.Context(), a, nil, "user.password_reset", "user", targetID, clientIP(r), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func validRole(role string) bool {
	switch role {
	case "owner", "admin", "technician", "viewer":
		return true
	default:
		return false
	}
}

func validAccountPassword(password string) bool {
	length := len([]rune(password))
	return length >= 4 && length <= 256
}

func (s *server) createEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	var in struct {
		Name       string `json:"name"`
		Group      string `json:"group"`
		MaxUses    int    `json:"maxUses"`
		ValidHours int    `json:"validHours"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.Name == "" {
		in.Name = "Установщик агента"
	}
	if in.Group == "" {
		in.Group = "Основная группа"
	}
	if in.MaxUses < 1 || in.MaxUses > 300 {
		writeError(w, http.StatusBadRequest, "Допустимо от 1 до 300 установок")
		return
	}
	if in.ValidHours < 1 || in.ValidHours > 24*30 {
		writeError(w, http.StatusBadRequest, "Срок действия — от 1 часа до 30 дней")
		return
	}
	token := randomToken(32)
	var id string
	var expires time.Time
	err := s.db.QueryRow(r.Context(), `INSERT INTO enrollment_tokens (name,token_hash,token_value,device_group,max_uses,expires_at,created_by) VALUES ($1,$2,$3,$4,$5,now()+make_interval(hours => $6),$7) RETURNING id,expires_at`,
		truncate(in.Name, 100), tokenHash(token), token, truncate(in.Group, 100), in.MaxUses, in.ValidHours, a.UserID).Scan(&id, &expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать токен")
		return
	}
	s.audit(r.Context(), a, nil, "enrollment.created", "enrollment_token", id, clientIP(r), map[string]any{"group": in.Group, "maxUses": in.MaxUses})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "token": token, "expiresAt": expires, "group": in.Group, "maxUses": in.MaxUses})
}

func (s *server) listEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT t.id,t.name,COALESCE(t.token_value,''),t.device_group,t.max_uses,t.uses,t.expires_at,t.disabled,t.created_at,
		       COALESCE(u.username,''),
		       COALESCE(jsonb_agg(jsonb_build_object(
		           'id',d.id::text,'name',d.name,'connectionCode',d.connection_code,'enrolledAt',d.enrolled_at
		       ) ORDER BY d.enrolled_at DESC) FILTER (WHERE d.id IS NOT NULL),'[]'::jsonb)
		FROM enrollment_tokens t
		LEFT JOIN users u ON u.id=t.created_by
		LEFT JOIN devices d ON d.enrollment_token_id=t.id
		GROUP BY t.id,u.username
		ORDER BY t.created_at DESC
		LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить токены")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, token, group, createdBy string
		var maxUses, uses int
		var expires, created time.Time
		var disabled bool
		var devices json.RawMessage
		if rows.Scan(&id, &name, &token, &group, &maxUses, &uses, &expires, &disabled, &created, &createdBy, &devices) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "token": token, "group": group, "maxUses": maxUses, "uses": uses, "expiresAt": expires, "disabled": disabled, "createdAt": created, "createdBy": createdBy, "devices": devices})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": items})
}

func (s *server) updateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	var in struct {
		Disabled bool `json:"disabled"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	tokenID := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `UPDATE enrollment_tokens SET disabled=$1 WHERE id=$2`, in.Disabled, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось обновить токен")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Токен не найден")
		return
	}
	s.audit(r.Context(), a, nil, "enrollment.updated", "enrollment_token", tokenID, clientIP(r), map[string]any{"disabled": in.Disabled})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) deleteEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	tokenID := chi.URLParam(r, "id")
	result, err := s.db.Exec(r.Context(), `DELETE FROM enrollment_tokens WHERE id=$1`, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось удалить токен")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Токен не найден")
		return
	}
	s.audit(r.Context(), a, nil, "enrollment.deleted", "enrollment_token", tokenID, clientIP(r), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) enrollAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token        string   `json:"token"`
		Name         string   `json:"name"`
		Hostname     string   `json:"hostname"`
		OS           string   `json:"os"`
		OSVersion    string   `json:"osVersion"`
		Arch         string   `json:"arch"`
		AgentVersion string   `json:"agentVersion"`
		LocalIPs     []string `json:"localIps"`
		InstallMode  string   `json:"installMode"`
		Privileged   bool     `json:"privileged"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.Token == "" || strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "Не указан токен или название устройства")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось зарегистрировать устройство")
		return
	}
	defer tx.Rollback(r.Context())
	var tokenID, group string
	var uses, maxUses int
	var expires time.Time
	var disabled bool
	err = tx.QueryRow(r.Context(), `SELECT id,device_group,uses,max_uses,expires_at,disabled FROM enrollment_tokens WHERE token_hash=$1 FOR UPDATE`, tokenHash(in.Token)).Scan(&tokenID, &group, &uses, &maxUses, &expires, &disabled)
	if err != nil || disabled || time.Now().After(expires) || uses >= maxUses {
		writeError(w, http.StatusUnauthorized, "Токен регистрации недействителен")
		return
	}
	secret := randomToken(32)
	desktopSecret := randomToken(32)
	var deviceID, code string
	for attempt := 0; attempt < 5; attempt++ {
		code = connectionCode()
		err = tx.QueryRow(r.Context(), `INSERT INTO devices (connection_code,name,hostname,device_group,os,os_version,arch,agent_version,public_ip,local_ips,install_mode,privileged,secret_hash,desktop_secret_hash,enrollment_token_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::inet,$10,$11,$12,$13,$14,$15) RETURNING id`,
			code, truncate(strings.TrimSpace(in.Name), 64), truncate(in.Hostname, 255), group, truncate(in.OS, 50), truncate(in.OSVersion, 100), truncate(in.Arch, 30), truncate(in.AgentVersion, 30), clientIP(r), sanitizeIPs(in.LocalIPs), sanitizeInstallMode(in.InstallMode), in.Privileged, tokenHash(secret), tokenHash(desktopSecret), tokenID).Scan(&deviceID)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Printf("agent enrollment insert failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Не удалось зарегистрировать устройство")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE enrollment_tokens SET uses=uses+1 WHERE id=$1`, tokenID); err != nil {
		log.Printf("agent enrollment token update failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Не удалось активировать устройство")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO audit_events (actor_device_id,event_type,target_type,target_id,ip_address,details) VALUES ($1,'device.enrolled','device',$2,NULLIF($3,'')::inet,jsonb_build_object('name',$4::text,'group',$5::text))`, deviceID, deviceID, clientIP(r), in.Name, group); err != nil {
		log.Printf("agent enrollment audit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Не удалось завершить регистрацию")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		log.Printf("agent enrollment commit failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Не удалось завершить регистрацию")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deviceId": deviceID, "deviceSecret": secret, "desktopSecret": desktopSecret, "connectionCode": code, "heartbeatSeconds": 30})
}

func (s *server) verifyAgentRegistration(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.Header.Get("X-Genesis-Device-Id"))
	authorization := r.Header.Get("Authorization")
	if deviceID == "" || !strings.HasPrefix(authorization, "Device ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Device "))
	var stored []byte
	if err := s.db.QueryRow(r.Context(), `SELECT secret_hash FROM devices WHERE id=$1`, deviceID).Scan(&stored); err != nil || subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Устройство больше не зарегистрировано в RemoteIt")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	deviceID := r.Header.Get("X-Genesis-Device-Id")
	authz := r.Header.Get("Authorization")
	if deviceID == "" || !strings.HasPrefix(authz, "Device ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	secret := strings.TrimPrefix(authz, "Device ")
	var stored, desktopStored []byte
	var previousAgentVersion string
	if err := s.db.QueryRow(r.Context(), `SELECT secret_hash,desktop_secret_hash,agent_version FROM devices WHERE id=$1`, deviceID).Scan(&stored, &desktopStored, &previousAgentVersion); err != nil || subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	var in struct {
		Name            string   `json:"name"`
		Hostname        string   `json:"hostname"`
		OS              string   `json:"os"`
		OSVersion       string   `json:"osVersion"`
		Arch            string   `json:"arch"`
		AgentVersion    string   `json:"agentVersion"`
		LocalIPs        []string `json:"localIps"`
		CurrentUser     string   `json:"currentUser"`
		CPUModel        string   `json:"cpuModel"`
		CPULoadPercent  float64  `json:"cpuLoadPercent"`
		MemoryBytes     int64    `json:"memoryBytes"`
		MemoryUsedBytes int64    `json:"memoryUsedBytes"`
		DiskTotalBytes  int64    `json:"diskTotalBytes"`
		DiskFreeBytes   int64    `json:"diskFreeBytes"`
		UptimeSeconds   int64    `json:"uptimeSeconds"`
		InstallMode     string   `json:"installMode"`
		Privileged      bool     `json:"privileged"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "Не указано название устройства")
		return
	}
	var desiredName, connectionCode string
	err := s.db.QueryRow(r.Context(), `UPDATE devices SET name=CASE WHEN desired_name<>'' THEN desired_name ELSE $1 END,desired_name=CASE WHEN desired_name<>'' AND desired_name=$1 THEN '' ELSE desired_name END,hostname=$2,os=$3,os_version=$4,arch=$5,agent_version=$6,public_ip=NULLIF($7,'')::inet,local_ips=$8,logged_in_user=$9,cpu_model=$10,cpu_load_percent=$11,memory_bytes=$12,memory_used_bytes=$13,disk_total_bytes=$14,disk_free_bytes=$15,uptime_seconds=$16,install_mode=$17,privileged=$18,last_seen=now(),updated_at=now() WHERE id=$19 RETURNING desired_name,connection_code`,
		truncate(in.Name, 64), truncate(in.Hostname, 255), truncate(in.OS, 50), truncate(in.OSVersion, 100), truncate(in.Arch, 30), truncate(in.AgentVersion, 30), clientIP(r), sanitizeIPs(in.LocalIPs), truncate(in.CurrentUser, 255), truncate(in.CPUModel, 255), clampFloat(in.CPULoadPercent, 0, 100), maxZero(in.MemoryBytes), maxZero(in.MemoryUsedBytes), maxZero(in.DiskTotalBytes), maxZero(in.DiskFreeBytes), maxZero(in.UptimeSeconds), sanitizeInstallMode(in.InstallMode), in.Privileged, deviceID).Scan(&desiredName, &connectionCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось обновить состояние")
		return
	}
	if strings.TrimSpace(previousAgentVersion) != strings.TrimSpace(in.AgentVersion) {
		// A desktop worker is replaced together with the Agent. End any viewer
		// session and erase its last JPEG so a frame captured by an older worker
		// (including the pre-1.0.6 multi-user VDI race) can never remain visible
		// after the privacy boundary changes. The administrator starts a fresh
		// session once the new, SID-bound worker is ready.
		if _, clearErr := s.db.Exec(r.Context(), `UPDATE remote_desktop_sessions SET status='ended',frame=NULL,ended_at=now(),agent_error='Agent updated; start a new remote session' WHERE device_id=$1 AND status='active'`, deviceID); clearErr != nil {
			log.Printf("clear stale desktop frame after Agent update for %s: %v", deviceID, clearErr)
		}
	}
	desktopSecret := ""
	if len(desktopStored) == 0 {
		candidate := randomToken(32)
		if result, updateErr := s.db.Exec(r.Context(), `UPDATE devices SET desktop_secret_hash=$1 WHERE id=$2 AND desktop_secret_hash IS NULL`, tokenHash(candidate), deviceID); updateErr == nil && result.RowsAffected() == 1 {
			desktopSecret = candidate
		}
	}
	job, jobErr := s.claimNextAgentJob(r.Context(), deviceID)
	if jobErr != nil {
		log.Printf("claim agent job failed for %s: %v", deviceID, jobErr)
	}
	update := s.agentUpdateFor(in.OS, in.Arch, in.AgentVersion)
	// Fifteen seconds keeps public/local IP and online state responsive after a
	// VPN route or network location changes without materially loading the API
	// at the supported fleet size (up to 300 agents).
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "nextHeartbeatSeconds": 15, "desiredName": desiredName, "connectionCode": connectionCode, "desktopSecret": desktopSecret, "actionSigningPublicKey": s.actionSigner.publicKey(), "job": job, "agentUpdate": update})
}

func (s *server) claimNextAgentJob(ctx context.Context, deviceID string) (*agentJobPayload, error) {
	_, _ = s.db.Exec(ctx, `UPDATE agent_jobs SET status='expired',error_text='Время выполнения задания истекло',completed_at=now(),updated_at=now() WHERE device_id=$1 AND status IN ('queued','running') AND (expires_at<=now() OR (status='running' AND started_at < now()-make_interval(secs => timeout_seconds+90)))`, deviceID)
	var job agentJobPayload
	var payload []byte
	err := s.db.QueryRow(ctx, `UPDATE agent_jobs SET status='running',started_at=now(),updated_at=now() WHERE id=(SELECT id FROM agent_jobs WHERE device_id=$1 AND status='queued' AND expires_at>now() ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING id,job_type,payload,timeout_seconds`, deviceID).Scan(&job.ID, &job.Type, &payload, &job.TimeoutSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payload, &job.Payload); err != nil {
		return nil, err
	}
	if job.Type == "action" {
		_, _ = s.db.Exec(ctx, `UPDATE action_jobs SET status='running',started_at=COALESCE(started_at,now()),updated_at=now() WHERE execution_job_id=$1 AND status='queued'`, job.ID)
	}
	return &job, nil
}

func (s *server) listDeviceJobs(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE agent_jobs SET status='expired',error_text='Время выполнения задания истекло',completed_at=now(),updated_at=now() WHERE device_id=$1 AND status IN ('queued','running') AND (expires_at<=now() OR (status='running' AND started_at < now()-make_interval(secs => timeout_seconds+90)))`, deviceID)
	canSeeShell := a.Role == "owner" || a.Role == "admin"
	rows, err := s.db.Query(r.Context(), `SELECT j.id,j.job_type,j.payload,j.status,j.timeout_seconds,j.output,j.error_text,j.exit_code,j.created_at,j.started_at,j.completed_at,COALESCE(u.username,'') FROM agent_jobs j LEFT JOIN users u ON u.id=j.created_by WHERE j.device_id=$1 AND ($2 OR j.job_type IN ('inventory','files_list','files_read','files_write')) ORDER BY j.created_at DESC LIMIT 50`, deviceID, canSeeShell)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить задания")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, jobType, status, output, errorText, actor string
		var payload []byte
		var timeout int
		var exitCode *int
		var created time.Time
		var started, completed *time.Time
		if rows.Scan(&id, &jobType, &payload, &status, &timeout, &output, &errorText, &exitCode, &created, &started, &completed, &actor) != nil {
			continue
		}
		var parsed map[string]any
		_ = json.Unmarshal(payload, &parsed)
		items = append(items, map[string]any{"id": id, "type": jobType, "payload": parsed, "status": status, "timeoutSeconds": timeout, "output": output, "error": errorText, "exitCode": exitCode, "createdAt": created, "startedAt": started, "completedAt": completed, "createdBy": actor})
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}

func (s *server) createDeviceJob(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role == "viewer" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	deviceID := chi.URLParam(r, "id")
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	var in struct {
		Type           string `json:"type"`
		Command        string `json:"command"`
		Shell          string `json:"shell"`
		Path           string `json:"path"`
		Name           string `json:"name"`
		DataBase64     string `json:"dataBase64"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.TimeoutSeconds == 0 {
		in.TimeoutSeconds = 30
	}
	if in.TimeoutSeconds < 5 || in.TimeoutSeconds > 60 {
		writeError(w, http.StatusBadRequest, "Тайм-аут должен быть от 5 до 60 секунд")
		return
	}
	payload := map[string]any{}
	switch in.Type {
	case "inventory":
		payload["requested"] = true
	case "shell":
		if a.Role != "owner" && a.Role != "admin" {
			writeError(w, http.StatusForbidden, "Удалённые команды доступны только владельцу и администраторам")
			return
		}
		in.Command = strings.TrimSpace(in.Command)
		if in.Command == "" || len([]rune(in.Command)) > 8192 {
			writeError(w, http.StatusBadRequest, "Команда должна содержать от 1 до 8192 символов")
			return
		}
		payload["command"] = in.Command
		var deviceOS string
		if err := s.db.QueryRow(r.Context(), `SELECT os FROM devices WHERE id=$1`, deviceID).Scan(&deviceOS); err != nil {
			writeError(w, http.StatusNotFound, "Устройство не найдено")
			return
		}
		var shellErr error
		in.Shell, shellErr = shellForDeviceOS(deviceOS, in.Shell)
		if shellErr != nil {
			writeError(w, http.StatusBadRequest, shellErr.Error())
			return
		}
		payload["shell"] = in.Shell
	case "files_list", "files_read":
		in.Path = strings.TrimSpace(in.Path)
		if len([]rune(in.Path)) > 4096 {
			writeError(w, http.StatusBadRequest, "Путь не должен превышать 4096 символов")
			return
		}
		if in.Type == "files_read" && in.Path == "" {
			writeError(w, http.StatusBadRequest, "Не указан путь к файлу")
			return
		}
		payload["path"] = in.Path
	case "files_write":
		in.Path = strings.TrimSpace(in.Path)
		in.Name = strings.TrimSpace(in.Name)
		if in.Path == "" || len([]rune(in.Path)) > 4096 {
			writeError(w, http.StatusBadRequest, "Не указана допустимая папка назначения")
			return
		}
		if in.Name == "" || in.Name == "." || in.Name == ".." || len([]rune(in.Name)) > 255 || strings.ContainsAny(in.Name, `/\\`) {
			writeError(w, http.StatusBadRequest, "Недопустимое имя файла")
			return
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(in.DataBase64)
		if decodeErr != nil || len(decoded) > 512*1024 {
			writeError(w, http.StatusBadRequest, "Файл должен быть корректным Base64 размером не более 512 КБ")
			return
		}
		payload["path"] = in.Path
		payload["name"] = in.Name
		payload["dataBase64"] = in.DataBase64
	default:
		writeError(w, http.StatusBadRequest, "Неизвестный тип задания")
		return
	}
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)`, deviceID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "Устройство не найдено")
		return
	}
	payloadJSON, _ := json.Marshal(payload)
	var id string
	var created, expires time.Time
	err := s.db.QueryRow(r.Context(), `INSERT INTO agent_jobs (device_id,created_by,job_type,payload,timeout_seconds,expires_at) VALUES ($1,$2,$3,$4,$5,now()+interval '30 minutes') RETURNING id,created_at,expires_at`, deviceID, a.UserID, in.Type, payloadJSON, in.TimeoutSeconds).Scan(&id, &created, &expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать задание")
		return
	}
	s.audit(r.Context(), a, nil, "agent_job.created", "agent_job", id, clientIP(r), map[string]any{"deviceId": deviceID, "type": in.Type, "timeoutSeconds": in.TimeoutSeconds})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "deviceId": deviceID, "type": in.Type, "status": "queued", "timeoutSeconds": in.TimeoutSeconds, "createdAt": created, "expiresAt": expires})
}

func shellForDeviceOS(deviceOS, requested string) (string, error) {
	osName := strings.ToLower(strings.TrimSpace(deviceOS))
	shell := strings.ToLower(strings.TrimSpace(requested))
	isWindows := strings.Contains(osName, "windows")
	isMac := strings.Contains(osName, "mac") || strings.Contains(osName, "darwin")
	if shell == "" {
		if isWindows {
			return "powershell", nil
		}
		if isMac {
			return "zsh", nil
		}
		return "bash", nil
	}
	allowed := (isWindows && (shell == "powershell" || shell == "cmd")) ||
		(isMac && (shell == "zsh" || shell == "bash" || shell == "sh")) ||
		(!isWindows && !isMac && (shell == "bash" || shell == "sh"))
	if !allowed {
		return "", fmt.Errorf("оболочка %s недоступна для %s", shell, strings.TrimSpace(deviceOS))
	}
	return shell, nil
}

func (s *server) cancelDeviceJob(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	jobID := chi.URLParam(r, "id")
	var deviceID string
	if err := s.db.QueryRow(r.Context(), `SELECT device_id FROM agent_jobs WHERE id=$1`, jobID).Scan(&deviceID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Задание не найдено")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось проверить задание")
		return
	}
	if !s.requireDeviceAccess(w, r, deviceID) {
		return
	}
	result, err := s.db.Exec(r.Context(), `UPDATE agent_jobs SET status='cancelled',completed_at=now(),updated_at=now() WHERE id=$1 AND status='queued'`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось отменить задание")
		return
	}
	if result.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "Можно отменить только задание в очереди")
		return
	}
	s.audit(r.Context(), a, nil, "agent_job.cancelled", "agent_job", jobID, clientIP(r), map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) agentJobResult(w http.ResponseWriter, r *http.Request) {
	deviceID := r.Header.Get("X-Genesis-Device-Id")
	authz := r.Header.Get("Authorization")
	if deviceID == "" || !strings.HasPrefix(authz, "Device ") {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	secret := strings.TrimPrefix(authz, "Device ")
	var stored []byte
	if err := s.db.QueryRow(r.Context(), `SELECT secret_hash FROM devices WHERE id=$1`, deviceID).Scan(&stored); err != nil || subtle.ConstantTimeCompare(tokenHash(secret), stored) != 1 {
		writeError(w, http.StatusUnauthorized, "Недействительные данные устройства")
		return
	}
	var in struct {
		Success  bool   `json:"success"`
		Output   string `json:"output"`
		Error    string `json:"error"`
		ExitCode int    `json:"exitCode"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	status := "failed"
	if in.Success {
		status = "succeeded"
	}
	jobID := chi.URLParam(r, "id")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить результат задания")
		return
	}
	defer tx.Rollback(r.Context())
	var jobType string
	var jobPayload []byte
	err = tx.QueryRow(r.Context(), `UPDATE agent_jobs SET status=$1,output=$2,error_text=$3,exit_code=$4,completed_at=now(),updated_at=now() WHERE id=$5 AND device_id=$6 AND status='running' RETURNING job_type,payload`, status, truncate(in.Output, 900000), truncate(in.Error, 4096), in.ExitCode, jobID, deviceID).Scan(&jobType, &jobPayload)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Задание уже завершено или недоступно")
		return
	}
	if err == nil && jobType == "uninstall" {
		if in.Success {
			_, err = tx.Exec(r.Context(), `UPDATE devices SET pending_removal=true,updated_at=now() WHERE id=$1`, deviceID)
		} else {
			_, err = tx.Exec(r.Context(), `UPDATE devices SET pending_removal=false,updated_at=now() WHERE id=$1`, deviceID)
		}
	}
	if err == nil && jobType == "action" {
		_, err = tx.Exec(r.Context(), `UPDATE action_jobs SET status=$1,output=$2,error_text=$3,exit_code=$4,started_at=COALESCE(started_at,now()),completed_at=now(),updated_at=now() WHERE execution_job_id=$5 AND device_id=$6`, status, truncate(in.Output, 900000), truncate(in.Error, 4096), in.ExitCode, jobID, deviceID)
	}
	if err == nil && jobType == "tunnel" && !in.Success {
		var payload struct {
			TunnelID string `json:"tunnelId"`
		}
		if json.Unmarshal(jobPayload, &payload) == nil && validTransferID(payload.TunnelID) {
			_, err = tx.Exec(r.Context(), `UPDATE network_tunnels SET status='failed',error_text=$1,ended_at=now() WHERE id=$2 AND device_id=$3 AND status='waiting'`, truncate(in.Error, 4096), payload.TunnelID, deviceID)
		}
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить задание")
		return
	}
	if jobType == "uninstall" && in.Success {
		s.audit(r.Context(), nil, nil, "agent_job.completed", "agent_job", jobID, clientIP(r), map[string]any{"success": in.Success, "exitCode": in.ExitCode})
		s.audit(r.Context(), nil, &deviceID, "device.uninstall_scheduled", "device", deviceID, clientIP(r), map[string]any{})
	} else {
		s.audit(r.Context(), nil, &deviceID, "agent_job.completed", "agent_job", jobID, clientIP(r), map[string]any{"success": in.Success, "exitCode": in.ExitCode})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) listAudit(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if a.Role != "owner" && a.Role != "admin" {
		writeError(w, http.StatusForbidden, "Недостаточно прав")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT a.id,COALESCE(u.username,''),a.event_type,a.target_type,a.target_id,COALESCE(host(a.ip_address),''),a.details,a.created_at FROM audit_events a LEFT JOIN users u ON u.id=a.actor_user_id ORDER BY a.created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить журнал")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var actor, eventType, targetType, targetID, ip string
		var details []byte
		var created time.Time
		if rows.Scan(&id, &actor, &eventType, &targetType, &targetID, &ip, &details, &created) == nil {
			var parsed any
			_ = json.Unmarshal(details, &parsed)
			items = append(items, map[string]any{"id": id, "actor": actor, "eventType": eventType, "targetType": targetType, "targetId": targetID, "ip": ip, "details": parsed, "createdAt": created})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": items})
}

func (s *server) audit(ctx context.Context, user *authState, deviceID *string, eventType, targetType, targetID, ip string, details map[string]any) {
	data, _ := json.Marshal(details)
	var userID any
	if user != nil {
		userID = user.UserID
	}
	var device any
	if deviceID != nil {
		device = *deviceID
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO audit_events (actor_user_id,actor_device_id,event_type,target_type,target_id,ip_address,details) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::inet,$7)`, userID, device, eventType, targetType, targetID, ip, data)
}

func (s *server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "Маршрут не найден")
		return
	}
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		http.NotFound(w, r)
		return
	}
	if clean == "." {
		clean = "index.html"
	}
	// Some desktop shells request /favicon.ico even when the page declares a
	// PNG icon explicitly. Serve the centered RemoteIt mark instead of falling
	// back to the SPA HTML document.
	if clean == "favicon.ico" {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, filepath.Join(s.webRoot, "icons", "icon-64.png"))
		return
	}
	path := filepath.Join(s.webRoot, clean)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		switch {
		case strings.HasPrefix(clean, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case clean == "sw.js" || strings.HasSuffix(clean, ".webmanifest"):
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(clean, "downloads/"):
			w.Header().Set("Cache-Control", "public, max-age=300")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		if strings.HasSuffix(clean, ".webmanifest") {
			w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
		}
		http.ServeFile(w, r, path)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(s.webRoot, "index.html"))
}

func (s *server) loginBlocked(ip string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	a := s.loginFails[ip]
	if a == nil || time.Since(a.windowStart) > 15*time.Minute {
		return false
	}
	return a.count >= 10
}

func (s *server) recordLoginFailure(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	a := s.loginFails[ip]
	if a == nil || time.Since(a.windowStart) > 15*time.Minute {
		s.loginFails[ip] = &loginAttempt{count: 1, windowStart: time.Now()}
		return
	}
	a.count++
}

func (s *server) clearLoginFailures(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	delete(s.loginFails, ip)
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	memory := uint32(64 * 1024)
	iterations := uint32(3)
	parallelism := uint8(2)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func randomToken(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func connectionCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000000))
	if err != nil {
		panic(err)
	}
	return strconv.FormatInt(100000000+n.Int64(), 10)
}

func currentAuth(r *http.Request) *authState {
	return r.Context().Value(authKey).(*authState)
}

func userResponse(a authState) map[string]any {
	return map[string]any{"id": a.UserID, "username": a.Username, "role": a.Role, "mustChangePassword": a.MustChangePassword}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "Некорректный запрос")
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Некорректный запрос")
		return errors.New("multiple json values")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	return ""
}

func sanitizeIPs(values []string) []string {
	result := make([]string, 0, min(len(values), 16))
	for _, value := range values {
		if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
			result = append(result, ip.String())
		}
		if len(result) == 16 {
			break
		}
	}
	return result
}

func sanitizeInstallMode(value string) string {
	switch value {
	case "system", "user":
		return value
	default:
		return "unknown"
	}
}

func truncate(value string, maxLen int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return string(runes)
}

func maxZero(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func clampFloat(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func mustEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
