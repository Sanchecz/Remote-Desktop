import { ClipboardEvent as ReactClipboardEvent, CSSProperties, DragEvent as ReactDragEvent, FormEvent, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent, WheelEvent as ReactWheelEvent, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  AlertTriangle,
	Apple,
  Ban,
  Boxes,
	Camera,
  CheckCircle2,
  ChevronDown,
	ChevronLeft,
	ChevronUp,
	CircleHelp,
  CircleUserRound,
  Clock3,
  Copy,
	Cpu,
	Database,
  Download,
	Eye,
  FileCode2,
  File as FileIcon,
  Folder,
  FolderOpen,
	HardDrive,
	Hand,
  KeyRound,
	Link2,
  ListFilter,
  LockKeyhole,
  LogOut,
	Maximize2,
  Menu,
  Monitor,
  MoreHorizontal,
  Moon,
	MousePointer2,
  Palette,
  Play,
  Plus,
  RefreshCw,
  Search,
  Server,
	SlidersHorizontal,
  Save,
  Send,
  Settings,
  ShieldCheck,
  Sparkles,
	ScreenShare,
  Sun,
  TerminalSquare,
	Trash2,
  Upload,
  UserPlus,
  Users,
	Wifi,
	Star,
	Clipboard,
	Crown,
	Keyboard,
	Power,
  X
} from "lucide-react";
import { browserCodeToVirtualKey, chunkRemoteText, planRemoteBoundaryDeletion, planRemoteKeyboardInput, planRemoteMobileBeforeInput, planRemoteTextReconciliation } from "./remoteKeyboard";
import { advanceRemotePinch, advanceRemoteTrackpadCursor, authoritativeRemoteFrameSize, cameraFollowingRemotePoint, canReleaseRemoteTouchSuppression, clampRemoteCamera, clampRemotePoint, classifyRemoteTouchGesture, fillRemoteFrame, fitRemoteFrame, isRemoteTwoFingerTap, remoteCursorVisualPointForLayer, remotePointerTapActions, remotePointFromClient, reprojectRemotePoint, shouldPresentDecodedRemoteFrame, stabilizeRemoteTrackpadMotion, stableRemoteTrackpadSamples } from "./remoteCamera";
import { LatestPointerCadence, remotePointerCadenceMillis } from "./remotePointerCadence";
import { bindRemoteInputCoordinates, remoteInputAckID, remoteInputBatchID, remoteInputClientID, restoreRemoteInputBatch, shouldRetryRemoteInputDelivery, takePendingRemoteInputBatches, type PendingRemoteInputBatch } from "./remoteInputDelivery";
import { buildCodexOperatorInstruction, buildWindowsMCPInstaller } from "./codexSetup";
import { REMOTE_VIEWPORT_SETTLE_DELAYS, remoteViewportChanged, remoteViewportWithStableOrientation, resolveRemoteLayoutLandscape, resolveRemoteViewport, shouldApplyRemoteOrientationTransition, shouldRebaseRemotePointerViewport, shouldUseCompactRemoteControls, shouldUseRemoteTrackpad, type RemoteViewport } from "./remoteViewport";
import { isCurrentRemoteFallbackGeneration, isRecoverableRemoteStatusFailure, isRemoteFrameStreamStalled, remoteReconnectDelay, shouldUseRemoteFrameFallback } from "./remoteReconnect";
import { abortableDelay, browserTransferChunkLength, fileTransferProgress, isAbortError, uploadTransferChunk, validateTransferCheckpoint } from "./fileTransfers";
import { createRemoteFrameRetirementScheduler } from "./remoteFramePresentation";

type User = {
  id: string;
  username: string;
  role: string;
  mustChangePassword: boolean;
};

type ManagedUser = User & {
  displayName: string;
  disabled: boolean;
  createdAt: string;
  lastLoginAt: string | null;
};

type Theme = "dark" | "light" | "blue";
type RemoteScaleMode = "fit" | "fill" | "actual";

type Device = {
  id: string;
  connectionCode: string;
  name: string;
  hostname: string;
  group: string;
  os: string;
  osVersion: string;
  arch: string;
  agentVersion: string;
  publicIp: string;
  localIps: string[];
  currentUser: string;
  cpuModel: string;
  cpuLoadPercent: number;
  memoryBytes: number;
  memoryUsedBytes: number;
  diskTotalBytes: number;
  diskFreeBytes: number;
  uptimeSeconds: number;
  installMode: "system" | "user" | "unknown";
  privileged: boolean;
  pendingRemoval: boolean;
  accessProtected: boolean;
  accessGranted: boolean;
  lastSeen: string;
  online: boolean;
};

type AgentJob = {
  id: string;
  type: "shell" | "inventory" | "uninstall" | "files_list" | "files_read" | "files_write";
  payload: { command?: string; shell?: string; path?: string };
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled" | "expired";
  timeoutSeconds: number;
  output: string;
  error: string;
  exitCode: number | null;
  createdAt: string;
  startedAt: string | null;
  completedAt: string | null;
  createdBy: string;
};

type AICommandResult = {
  id: string;
  command: string;
  shell: string;
  output: string;
  error: string;
  exitCode: number | null;
};

type AIAnalysis = {
  mode: "local" | "openai";
  modelConfigured: boolean;
  summary: string;
  findings: { level: "ok" | "warning" | "error"; title: string; details: string }[];
  commands: { id: string; title: string; explanation: string; shell: string; command: string; risk: "read" | "change"; requiresConfirmation: boolean }[];
  evidence: AICommandResult[];
  privacyNote: string;
};

type AuditEvent = {
  id: number;
  actor: string;
  eventType: string;
  targetType: string;
  targetId: string;
  ip: string;
  details: Record<string, unknown>;
  createdAt: string;
};

type EnrollmentTokenInfo = {
	id: string;
	name: string;
	token: string;
	group: string;
	maxUses: number;
	uses: number;
	expiresAt: string;
	disabled: boolean;
	createdAt: string;
	createdBy: string;
	devices: { id: string; name: string; connectionCode: string; enrolledAt: string }[];
};

type AuthSession = {
  id: string;
  ip: string;
  userAgent: string;
  createdAt: string;
  lastUsedAt: string;
  expiresAt: string;
  current: boolean;
};

type IntegrationToken = {
  id: string;
  name: string;
  scopes: string[];
  expiresAt: string;
  createdAt: string;
  lastUsedAt: string | null;
  revokedAt: string | null;
};

type ActionJob = {
  id: string;
  deviceId: string;
  deviceName: string;
  remoteId: string;
  action: string;
  parameters: Record<string, unknown>;
  risk: "read" | "high" | "critical";
  status: "awaiting_approval" | "queued" | "running" | "succeeded" | "failed" | "cancelled" | "expired";
  approvalRequired: boolean;
  requestedVia: "web" | "mcp";
  plan: {
    title?: string;
    description?: string;
    steps?: string[];
    rollback?: string;
  };
  output: string;
  error: string;
  exitCode: number | null;
  requestHash: string;
  createdAt: string;
  expiresAt: string;
  startedAt: string | null;
  completedAt: string | null;
  approvedAt: string | null;
};

type Section = "devices" | "remote" | "sessions" | "terminal" | "scripts" | "tokens" | "users" | "audit" | "settings";

type ApiError = { error?: string };

const LATEST_AGENT_VERSION = "1.0.31";

async function api<T>(path: string, options: RequestInit = {}, csrf = ""): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body) headers.set("Content-Type", "application/json");
  if (csrf) headers.set("X-CSRF-Token", csrf);
  const response = await fetch(path, { ...options, headers, credentials: "same-origin" });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ApiError;
    throw new Error(body.error || `Ошибка ${response.status}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`brand ${compact ? "brand-compact" : ""}`}>
      <span className="brand-mark"><img src="/icons/icon-64.png" alt="" /></span>
      {!compact && <span className="brand-name">Remote<span>It</span></span>}
    </div>
  );
}

const themeOptions: { id: Theme; label: string; icon: typeof Moon }[] = [
  { id: "dark", label: "Тёмная", icon: Moon },
  { id: "light", label: "Светлая", icon: Sun },
  { id: "blue", label: "Бело-синяя", icon: Palette }
];

function ThemeSwitcher({ theme, onChange, compact = false }: { theme: Theme; onChange: (theme: Theme) => void; compact?: boolean }) {
  return <div className={`theme-switcher ${compact ? "theme-switcher-compact" : ""}`} aria-label="Тема интерфейса">{themeOptions.map(({ id, label, icon: Icon }) => <button key={id} className={theme === id ? "active" : ""} onClick={() => onChange(id)} type="button" aria-label={label} title={label}><Icon size={15} />{!compact && <span>{label}</span>}</button>)}</div>;
}

function Login({ onLogin, theme, onTheme }: { onLogin: (user: User, csrf: string) => void; theme: Theme; onTheme: (theme: Theme) => void }) {
  const [username, setUsername] = useState("Admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
	const [installCode, setInstallCode] = useState(() => new URLSearchParams(window.location.hash.replace(/^#/, "")).get("install") || "");
	const [installInfo, setInstallInfo] = useState<{ name: string; group: string; remaining: number; expiresAt: string } | null>(null);
	const [installError, setInstallError] = useState("");
	const [installLoading, setInstallLoading] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      const result = await api<{ user: User; csrfToken: string }>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password })
      });
      onLogin(result.user, result.csrfToken);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось войти");
    } finally {
      setLoading(false);
    }
  }

	async function resolveInstallCode(event: FormEvent) {
		event.preventDefault();
		setInstallLoading(true); setInstallError(""); setInstallInfo(null);
		try {
			const result = await api<{ name: string; group: string; remaining: number; expiresAt: string }>("/api/public/install/resolve", { method: "POST", body: JSON.stringify({ code: installCode.trim() }) });
			setInstallInfo(result);
		} catch (reason) {
			setInstallError(reason instanceof Error ? reason.message : "Код установки недействителен");
		} finally { setInstallLoading(false); }
	}

	async function downloadPublicAgent(platform: "windows" | "macos" | "linux") {
		setInstallLoading(true); setInstallError("");
		try {
			const endpoint = platform === "windows" ? "/api/public/install/windows-agent" : "/api/public/install/unix-agent";
			const response = await fetch(endpoint, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code: installCode.trim() }) });
			if (!response.ok) {
				const body = await response.json().catch(() => ({ error: `HTTP ${response.status}` })) as { error?: string };
				throw new Error(body.error || `HTTP ${response.status}`);
			}
			const url = URL.createObjectURL(await response.blob());
			const anchor = document.createElement("a");
			anchor.href = url;
			anchor.download = platform === "windows" ? "RemoteIt-Agent.exe" : platform === "macos" ? "RemoteIt-Agent-macOS.sh" : "RemoteIt-Agent-Linux.sh";
			anchor.click();
			window.setTimeout(() => URL.revokeObjectURL(url), 1500);
		} catch (reason) {
			setInstallError(reason instanceof Error ? reason.message : "Не удалось скачать Agent");
		} finally { setInstallLoading(false); }
	}

	async function downloadPublicAndroidAgent() {
		setInstallLoading(true); setInstallError("");
		try {
			await navigator.clipboard?.writeText(installCode.trim()).catch(() => undefined);
			const anchor = document.createElement("a");
			anchor.href = "/downloads/RemoteIt-Agent-Android.apk";
			anchor.download = "RemoteIt-Agent-Android.apk";
			anchor.click();
		} catch (reason) {
			setInstallError(reason instanceof Error ? reason.message : "Не удалось скачать Android Agent");
		} finally { setInstallLoading(false); }
	}

  return (
    <main className="login-page">
      <div className="login-glow login-glow-one" />
      <div className="login-glow login-glow-two" />
      <section className="login-card">
        <Brand />
        <div className="login-copy">
          <span className="eyebrow"><ShieldCheck size={14} /> Защищённый доступ</span>
          <h1>Панель управления</h1>
          <p>Компьютеры, серверы и сеансы вашей инфраструктуры в одном месте.</p>
        </div>
        <form onSubmit={submit}>
          <label>
            <span>Логин</span>
            <div className="input-wrap"><CircleUserRound size={18} /><input autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required /></div>
          </label>
          <label>
            <span>Пароль</span>
            <div className="input-wrap"><LockKeyhole size={18} /><input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required autoFocus={!installCode} /></div>
          </label>
          {error && <div className="form-error">{error}</div>}
          <button className="primary-button login-button" disabled={loading}>{loading ? <RefreshCw className="spin" size={18} /> : <KeyRound size={18} />} Войти</button>
        </form>
		<div className="login-divider"><span>ИЛИ УСТАНОВИТЬ AGENT</span></div>
		<form className="public-install-form" onSubmit={resolveInstallCode}>
			<label><span>Код установки</span><div className="input-wrap"><Download size={18} /><input value={installCode} onChange={(event) => { setInstallCode(event.target.value); setInstallInfo(null); setInstallError(""); }} placeholder="Вставьте код от администратора" autoFocus={Boolean(installCode)} required /></div></label>
			{installError && <div className="form-error">{installError}</div>}
			{installInfo ? <div className="public-install-result"><div className="public-install-result-head"><span className="status-dot" /><div><strong>{installInfo.name}</strong><small>Группа «{installInfo.group}» · осталось установок: {installInfo.remaining}</small></div></div><div className="public-install-platforms"><button type="button" onClick={() => void downloadPublicAgent("windows")} disabled={installLoading}><DeviceOSIcon os="Windows" size={25} /><span>Windows<small>готовый EXE</small></span></button><button type="button" onClick={() => void downloadPublicAgent("macos")} disabled={installLoading}><DeviceOSIcon os="macOS" size={25} /><span>macOS<small>установщик SH</small></span></button><button type="button" onClick={() => void downloadPublicAgent("linux")} disabled={installLoading}><DeviceOSIcon os="Linux" size={25} /><span>Linux<small>установщик SH</small></span></button><button type="button" onClick={() => void downloadPublicAndroidAgent()} disabled={installLoading}><DeviceOSIcon os="Android" size={25} /><span>Android Agent<small>APK · код скопирован</small></span></button></div><p>Agent запросит имя устройства и автоматически появится в панели администратора. Для Android вставьте скопированный код и разрешите показ экрана.</p></div> : <button className="secondary-button public-install-submit" disabled={installLoading}>{installLoading ? <RefreshCw className="spin" size={17} /> : <Download size={17} />} Получить Agent</button>}
		</form>
        <footer><span><span className="status-dot" /> supportgenesis.ru</span><ThemeSwitcher theme={theme} onChange={onTheme} compact /></footer>
      </section>
    </main>
  );
}

function ChangePassword({ user, csrf, onDone, theme, onTheme }: { user: User; csrf: string; onDone: () => void; theme: Theme; onTheme: (theme: Theme) => void }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (next !== confirm) return setError("Новые пароли не совпадают");
    setLoading(true);
    setError("");
    try {
      await api("/api/auth/change-password", { method: "POST", body: JSON.stringify({ currentPassword: current, newPassword: next }) }, csrf);
      onDone();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось обновить пароль");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card password-card">
        <Brand />
        <div className="login-copy">
          <span className="eyebrow"><ShieldCheck size={14} /> Первый вход</span>
          <h1>Защитите аккаунт {user.username}</h1>
          <p>Временный пароль необходимо заменить. Используйте не менее 4 символов.</p>
        </div>
        <form onSubmit={submit}>
          <label><span>Текущий пароль</span><div className="input-wrap"><LockKeyhole size={18} /><input type="password" value={current} onChange={(e) => setCurrent(e.target.value)} required autoFocus /></div></label>
          <label><span>Новый пароль</span><div className="input-wrap"><KeyRound size={18} /><input type="password" minLength={4} maxLength={256} value={next} onChange={(e) => setNext(e.target.value)} required /></div></label>
          <label><span>Повторите новый пароль</span><div className="input-wrap"><KeyRound size={18} /><input type="password" minLength={4} maxLength={256} value={confirm} onChange={(e) => setConfirm(e.target.value)} required /></div></label>
          {error && <div className="form-error">{error}</div>}
          <button className="primary-button login-button" disabled={loading}>{loading ? <RefreshCw className="spin" size={18} /> : <ShieldCheck size={18} />} Сохранить пароль</button>
        </form>
        <footer><span><span className="status-dot" /> Защищённая сессия</span><ThemeSwitcher theme={theme} onChange={onTheme} compact /></footer>
      </section>
    </main>
  );
}

const navigation = [
  { id: "devices", label: "Устройства", icon: Monitor, enabled: true },
  { id: "remote", label: "Удалённый доступ", icon: ScreenShare, enabled: true },
  { id: "sessions", label: "Сеансы", icon: Activity, enabled: true },
  { id: "terminal", label: "Терминал", icon: TerminalSquare, enabled: true },
  { id: "scripts", label: "Сценарии", icon: FileCode2, enabled: true },
  { id: "tokens", label: "Токены", icon: KeyRound, enabled: true },
  { id: "users", label: "Пользователи", icon: Users, enabled: true },
  { id: "audit", label: "Журнал", icon: Clock3, enabled: true },
  { id: "settings", label: "Настройки", icon: Settings, enabled: true }
];

function Dashboard({ user, csrf, onLogout, theme, onTheme }: { user: User; csrf: string; onLogout: () => void; theme: Theme; onTheme: (theme: Theme) => void }) {
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "online" | "offline" | "attention">("all");
  const [menuOpen, setMenuOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [aiOpen, setAIOpen] = useState(false);
  const [enrollOpen, setEnrollOpen] = useState(false);
	const [tokenRefreshKey, setTokenRefreshKey] = useState(0);
  const [error, setError] = useState("");
  const [section, setSection] = useState<Section>(() => {
    const requested = new URLSearchParams(window.location.search).get("section");
    if (requested === "settings" || requested === "sessions") return requested;
    if (requested === "remote" && user.role !== "viewer") return "remote";
    if (requested === "terminal" && user.role !== "viewer") return "terminal";
    if (requested === "scripts" && (user.role === "owner" || user.role === "admin")) return "scripts";
    if ((requested === "users" || requested === "audit" || requested === "tokens") && (user.role === "owner" || user.role === "admin")) return requested;
    return "devices";
  });
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
	const [remoteDeviceId, setRemoteDeviceId] = useState("");
	const [remoteNavigationKey, setRemoteNavigationKey] = useState(0);
	const searchRef = useRef<HTMLInputElement>(null);
  const canManageUsers = user.role === "owner" || user.role === "admin";
  const roleLabel = user.role === "owner" ? "Владелец" : user.role === "admin" ? "Администратор" : user.role === "technician" ? "Техник" : "Наблюдатель";

  const loadDevices = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const result = await api<{ devices: Device[] }>("/api/devices");
      setDevices(result.devices);
      setError("");
      return result.devices;
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить устройства");
      return [] as Device[];
    } finally { if (!silent) setLoading(false); }
  }, []);

  const updateDeviceAccess = useCallback((deviceId: string, accessProtected: boolean, accessGranted: boolean) => {
    setDevices((current) => current.map((device) => device.id === deviceId ? { ...device, accessProtected, accessGranted } : device));
    setSelectedDevice((current) => current?.id === deviceId ? { ...current, accessProtected, accessGranted } : current);
  }, []);

  useEffect(() => { void loadDevices(); }, [loadDevices]);
  useEffect(() => {
    const id = window.setInterval(() => void loadDevices(true), 30_000);
    return () => window.clearInterval(id);
  }, [loadDevices]);
	useEffect(() => {
		const handleShortcut = (event: KeyboardEvent) => {
			if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
				event.preventDefault(); searchRef.current?.focus();
			}
		};
		window.addEventListener("keydown", handleShortcut);
		return () => window.removeEventListener("keydown", handleShortcut);
	}, []);

  const filtered = useMemo(() => {
    const q = query.toLowerCase().trim();
    return devices.filter((d) => {
      const matchesQuery = !q || [d.name, d.hostname, d.connectionCode, d.publicIp, d.os, d.group].some((v) => v?.toLowerCase().includes(q));
      const matchesStatus = statusFilter === "all" || (statusFilter === "online" && d.online) || (statusFilter === "offline" && !d.online) || (statusFilter === "attention" && (!d.online || !versionAtLeast(d.agentVersion, LATEST_AGENT_VERSION)));
      return matchesQuery && matchesStatus;
    });
  }, [devices, query, statusFilter]);
  const online = devices.filter((d) => d.online).length;

  async function logout() {
    await api("/api/auth/logout", { method: "POST" }, csrf).catch(() => undefined);
    onLogout();
  }

  function navigateSection(next: Section, remoteTarget = "") {
		// Normal navigation opens Remote Access without a preselected preview.
		// Only an explicit Connect action from Devices hands a target across.
		setRemoteDeviceId(next === "remote" ? remoteTarget : "");
		if (next === "remote") setRemoteNavigationKey((current) => current + 1);
    setSection(next);
    setMenuOpen(false);
    const url = new URL(window.location.href);
    if (next === "devices") url.searchParams.delete("section");
    else url.searchParams.set("section", next);
    window.history.replaceState(null, "", url);
  }

  return (
    <div className="app-shell">
      <aside className={`sidebar ${menuOpen ? "sidebar-open" : ""}`}>
        <div className="sidebar-head"><Brand /><button className="mobile-close" onClick={() => setMenuOpen(false)} aria-label="Закрыть меню"><X /></button></div>
        <nav>{navigation.map(({ id, label, icon: Icon, enabled }) => {
          const allowed = enabled && (!["users", "audit", "scripts", "tokens"].includes(id) || canManageUsers) && (!["terminal", "remote"].includes(id) || user.role !== "viewer");
          return <button key={id} className={section === id ? "active" : ""} disabled={!allowed} onClick={() => allowed && navigateSection(id as Section)}><Icon size={19} /><span>{label}</span></button>;
        })}</nav>
        <div className="sidebar-foot">
          <ThemeSwitcher theme={theme} onChange={onTheme} />
          <div className="server-health"><span className="status-dot" /><div><strong>Сервер работает</strong><small>supportgenesis.ru</small></div></div>
          <button className="user-block" onClick={logout}><span className="avatar">{user.username.slice(0, 1).toUpperCase()}</span><div><strong>{user.username}</strong><small>{roleLabel}</small></div><LogOut size={17} /></button>
        </div>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <button className="mobile-menu" onClick={() => setMenuOpen(true)} aria-label="Открыть меню"><Menu /></button>
		  <div className="mobile-top-brand"><Brand /></div>
          <div className="search-box"><Search size={18} /><input ref={searchRef} placeholder="Поиск по имени, ID или IP" value={query} onChange={(e) => setQuery(e.target.value)} /><kbd>Ctrl K</kbd></div>
		  {user.role !== "viewer" && <button className="ai-top-button" aria-label="Открыть AI-администратора" onClick={() => setAIOpen(true)}><Sparkles size={17} /><span>AI</span></button>}
          <button className="icon-button" aria-label="Обновить" onClick={() => void loadDevices()}><RefreshCw size={18} className={loading ? "spin" : ""} /></button>
			<ThemeSwitcher theme={theme} onChange={onTheme} compact />
			<button className="icon-button topbar-help" aria-label="Справка" title="Справка и настройки" onClick={() => navigateSection("settings")}><CircleHelp size={18} /></button>
          <div className="profile-menu-wrap">
            <button className="profile-button" aria-expanded={profileOpen} onClick={() => setProfileOpen((open) => !open)}><span className="avatar small">{user.username.slice(0, 1).toUpperCase()}</span><span>{user.username}</span><ChevronDown size={15} className={profileOpen ? "rotate" : ""} /></button>
            {profileOpen && <div className="profile-menu">
              <div className="profile-menu-head"><strong>{user.username}</strong><small>{roleLabel}</small></div>
              <button onClick={() => { setProfileOpen(false); navigateSection("settings"); }}><Settings size={16} /> Настройки</button>
              <button onClick={() => { setProfileOpen(false); navigateSection("sessions"); }}><Activity size={16} /> Активные сеансы</button>
              <button className="profile-logout" onClick={() => void logout()}><LogOut size={16} /> Выйти</button>
            </div>}
          </div>
        </header>

        <div className="content">
          {section === "users" ? <UsersPage currentUser={user} csrf={csrf} /> : section === "remote" ? <RemoteControlPage key={`remote-${remoteNavigationKey}`} devices={devices} currentUser={user} csrf={csrf} initialDeviceId={remoteDeviceId} onAccessChanged={updateDeviceAccess} onOpenDevice={setSelectedDevice} /> : section === "sessions" ? <SessionsPage devices={devices} onOpen={setSelectedDevice} /> : section === "terminal" ? <TerminalPage devices={devices} currentUser={user} csrf={csrf} onAccessChanged={updateDeviceAccess} /> : section === "scripts" ? <ScriptsPage devices={devices} currentUser={user} csrf={csrf} onAccessChanged={updateDeviceAccess} /> : section === "tokens" ? <TokensPage csrf={csrf} refreshKey={tokenRefreshKey} onCreate={() => setEnrollOpen(true)} /> : section === "audit" ? <AuditPage /> : section === "settings" ? <SettingsPage currentUser={user} csrf={csrf} theme={theme} onTheme={onTheme} /> : <>
          <section className="page-heading">
            <div><span className="eyebrow">ИНФРАСТРУКТУРА</span><h1>Устройства</h1><p>Все компьютеры и серверы, подключённые к RemoteIt.</p></div>
            <div className="heading-actions"><a className="secondary-button apk-download" href="/downloads/RemoteIt.apk" download><Download size={17} /> Android APK</a><button className="primary-button" onClick={() => setEnrollOpen(true)}><Plus size={18} /> Добавить устройство</button></div>
          </section>

          <section className="stats-grid">
            <Stat icon={Activity} label="В сети" value={String(online)} note={`из ${devices.length} устройств`} tone="green" />
            <Stat icon={Boxes} label="Всего устройств" value={String(devices.length)} note="лимит 300" tone="blue" />
            <Stat icon={AlertTriangle} label="Требуют внимания" value={String(devices.filter((d) => !d.online || !versionAtLeast(d.agentVersion, LATEST_AGENT_VERSION)).length)} note="не в сети или старый агент" tone="amber" />
            <Stat icon={CheckCircle2} label="Актуальный агент" value={String(devices.filter((d) => versionAtLeast(d.agentVersion, LATEST_AGENT_VERSION)).length)} note={`версия ${LATEST_AGENT_VERSION}`} tone="violet" />
          </section>

          <section className="device-panel devices-panel">
            <div className="panel-head"><div><h2>Все устройства</h2><span>{filtered.length} показано</span></div><label className="panel-filter"><ListFilter size={17} /><select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as typeof statusFilter)}><option value="all">Все устройства</option><option value="online">Только в сети</option><option value="offline">Не в сети</option><option value="attention">Требуют внимания</option></select></label></div>
            {error && <div className="panel-error">{error}</div>}
            <div className="table-wrap">
              <table>
                <thead><tr><th>Устройство</th><th>Remote ID</th><th>Система</th><th>IP-адрес</th><th>Пользователь</th><th>Последняя активность</th><th>Статус</th><th /></tr></thead>
                <tbody>
                  {loading && devices.length === 0 ? <LoadingRows /> : filtered.map((device) => <DeviceRow key={device.id} device={device} onOpen={() => setSelectedDevice(device)} onRemote={() => navigateSection("remote", device.id)} />)}
                </tbody>
              </table>
              {!loading && filtered.length === 0 && <div className="empty-state"><div className="empty-icon"><Monitor size={30} /></div><h3>{query || statusFilter !== "all" ? "Ничего не найдено" : "Добавьте первое устройство"}</h3><p>{query || statusFilter !== "all" ? "Измените поиск или фильтр состояния." : "Создайте токен установки — агент автоматически появится здесь."}</p>{devices.length === 0 && !query && statusFilter === "all" && <button className="secondary-button" onClick={() => setEnrollOpen(true)}><Plus size={17} /> Создать токен</button>}</div>}
            </div>
          </section>
          </>}
        </div>
		<nav className="mobile-bottom-nav" aria-label="Основная навигация"><button className={section === "devices" ? "active" : ""} onClick={() => navigateSection("devices")}><Monitor size={20} /><span>Устройства</span></button><button className={section === "sessions" ? "active" : ""} onClick={() => navigateSection("sessions")}><Activity size={20} /><span>Сеансы</span></button><button className={section === "remote" ? "active" : ""} disabled={user.role === "viewer"} onClick={() => navigateSection("remote")}><ScreenShare size={20} /><span>Доступ</span></button><button className={section === "scripts" ? "active" : ""} disabled={!canManageUsers} onClick={() => canManageUsers && navigateSection("scripts")}><FileCode2 size={20} /><span>Автоматизация</span></button><button className={menuOpen || ["terminal","tokens","users","audit","settings"].includes(section) ? "active" : ""} onClick={() => setMenuOpen(true)}><MoreHorizontal size={21} /><span>Ещё</span></button></nav>
      </main>
      {menuOpen && <div className="mobile-overlay" onClick={() => setMenuOpen(false)} />}
      {enrollOpen && <EnrollmentModal csrf={csrf} onClose={() => { setEnrollOpen(false); setTokenRefreshKey((value) => value + 1); }} />}
      {aiOpen && <AIAssistant devices={devices} currentUser={user} csrf={csrf} initialDevice={selectedDevice} onClose={() => setAIOpen(false)} />}
      {selectedDevice && <DeviceDrawer device={selectedDevice} currentUser={user} csrf={csrf} onClose={() => setSelectedDevice(null)} onAccessChanged={updateDeviceAccess} onRenamed={() => void loadDevices()} onDeleted={() => { setSelectedDevice(null); void loadDevices(); }} />}
    </div>
  );
}

function AIAssistant({ devices, currentUser, csrf, initialDevice, onClose }: { devices: Device[]; currentUser: User; csrf: string; initialDevice: Device | null; onClose: () => void }) {
  const [deviceId, setDeviceId] = useState(initialDevice?.id || devices.find((device) => device.online && device.accessGranted)?.id || devices.find((device) => device.accessGranted)?.id || "");
  const [question, setQuestion] = useState("");
  const [analysis, setAnalysis] = useState<AIAnalysis | null>(null);
  const [results, setResults] = useState<AICommandResult[]>([]);
  const [busy, setBusy] = useState(false);
  const [running, setRunning] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const selected = devices.find((device) => device.id === deviceId) || null;
  const canExecute = currentUser.role === "owner" || currentUser.role === "admin";
  const suggestions = ["Почему компьютер тормозит?", "Почему нет интернета?", "Что занимает место на диске?", "Проверь последние системные ошибки"];

  useEffect(() => {
    if (!deviceId && devices.length) setDeviceId(devices.find((device) => device.online && device.accessGranted)?.id || devices.find((device) => device.accessGranted)?.id || "");
  }, [deviceId, devices]);

  async function analyze(nextResults = results, requestedQuestion = question) {
    const prompt = requestedQuestion.trim();
    if (!deviceId || !prompt) return;
    setBusy(true); setError("");
    try {
      const response = await api<AIAnalysis>("/api/ai/analyze", { method: "POST", body: JSON.stringify({ deviceId, question: prompt, results: nextResults }) }, csrf);
      setAnalysis(response);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось выполнить анализ");
    } finally { setBusy(false); }
  }

  async function waitForJob(id: string): Promise<AgentJob> {
    for (let attempt = 0; attempt < 75; attempt += 1) {
      const response = await api<{ jobs: AgentJob[] }>(`/api/devices/${deviceId}/jobs`);
      const job = response.jobs.find((item) => item.id === id);
      if (job && ["succeeded", "failed", "cancelled", "expired"].includes(job.status)) return job;
      await new Promise((resolve) => window.setTimeout(resolve, 800));
    }
    throw new Error("Agent не вернул результат за отведённое время");
  }

  async function runCommand(command: AIAnalysis["commands"][number]) {
    if (!canExecute) return setError("Запуск команд доступен владельцу и администраторам");
    if (!selected?.online) return setError("Устройство не в сети — дождитесь подключения агента");
    if (command.requiresConfirmation && !window.confirm(`RemoteIt выполнит изменяющую команду на ${selected.name}.\n\n${command.title}\n\n${command.command}\n\nПродолжить?`)) return;
    setRunning((current) => ({ ...current, [command.id]: "running" })); setError("");
    try {
      const created = await api<{ id: string }>(`/api/devices/${deviceId}/jobs`, { method: "POST", body: JSON.stringify({ type: "shell", command: command.command, shell: command.shell, timeoutSeconds: 60 }) }, csrf);
      const job = await waitForJob(created.id);
      const result: AICommandResult = { id: command.id, command: command.command, shell: command.shell, output: job.output || "", error: job.error || "", exitCode: job.exitCode };
      setResults((current) => [...current.filter((item) => item.id !== command.id), result]);
      setRunning((current) => ({ ...current, [command.id]: job.status }));
      return result;
    } catch (reason) {
      setRunning((current) => ({ ...current, [command.id]: "failed" }));
      setError(reason instanceof Error ? reason.message : "Команда не выполнена");
      return null;
    }
  }

  async function runDiagnostics() {
    if (!analysis) return;
    const safeCommands = analysis.commands.filter((command) => command.risk === "read");
    if (!safeCommands.length) return;
    const completed: AICommandResult[] = [];
    for (const command of safeCommands) {
      const result = await runCommand(command);
      if (result) completed.push(result);
    }
    if (completed.length) {
      const merged = [...results.filter((item) => !completed.some((next) => next.id === item.id)), ...completed];
      setResults(merged);
      await analyze(merged);
    }
  }

  function askSuggestion(value: string) {
    setQuestion(value);
    setResults([]);
    void analyze([], value);
  }

  return <div className="drawer-backdrop ai-drawer-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><aside className="ai-assistant" aria-label="AI-администратор RemoteIt">
    <header className="ai-assistant-head"><div className="ai-assistant-brand"><span><Sparkles size={21} /></span><div><small>REMOTEIT INTELLIGENCE</small><h2>AI-администратор</h2><p>Диагностика выбранного устройства с подтверждением действий</p></div></div><button className="icon-button" onClick={onClose} aria-label="Закрыть AI-администратора"><X size={20} /></button></header>
    <div className="ai-device-picker"><label><span>Контекст устройства</span><select value={deviceId} onChange={(event) => { setDeviceId(event.target.value); setAnalysis(null); setResults([]); setError(""); }}>{devices.filter((device) => device.accessGranted).map((device) => <option key={device.id} value={device.id}>{device.online ? "●" : "○"} {device.name} · {device.os}</option>)}</select></label>{selected && <div className="ai-device-context"><span className={`status-dot ${selected.online ? "" : "offline"}`} /><strong>{selected.online ? "В сети" : "Не в сети"}</strong><small>CPU {Math.round(selected.cpuLoadPercent || 0)}% · RAM {selected.memoryBytes ? Math.round(selected.memoryUsedBytes / selected.memoryBytes * 100) : 0}% · диск {formatBytes(selected.diskFreeBytes)} свободно</small></div>}</div>
    <div className="ai-assistant-body">
      {!analysis && <section className="ai-welcome"><span className="ai-orbit"><Sparkles size={28} /></span><h3>Что проверить на {selected?.name || "устройстве"}?</h3><p>Опишите симптом обычными словами. RemoteIt сначала покажет план и не изменит систему без вашего подтверждения.</p><div className="ai-suggestions">{suggestions.map((item) => <button key={item} type="button" onClick={() => askSuggestion(item)}>{item}</button>)}</div></section>}
      {analysis && <div className="ai-response"><div className="ai-response-mode"><span><Sparkles size={15} />{analysis.mode === "openai" ? "OpenAI · контекстный анализ" : "Встроенная диагностика RemoteIt"}</span>{analysis.modelConfigured && analysis.mode !== "openai" && <em>AI временно недоступен — применены локальные правила</em>}</div><section className="ai-summary"><h3>Результат анализа</h3><p>{analysis.summary}</p></section><div className="ai-findings">{analysis.findings.map((finding, index) => <article key={`${finding.title}-${index}`} className={`ai-finding ${finding.level}`}><span>{finding.level === "ok" ? <CheckCircle2 size={18} /> : <AlertTriangle size={18} />}</span><div><strong>{finding.title}</strong><p>{finding.details}</p></div></article>)}</div>{analysis.commands.length > 0 && <section className="ai-plan"><div className="ai-plan-head"><div><h3>Предлагаемый план</h3><p>Сначала безопасные проверки, изменяющие команды помечены отдельно.</p></div>{canExecute && analysis.commands.some((command) => command.risk === "read") && <button className="primary-button" disabled={busy || Object.values(running).includes("running") || !selected?.online} onClick={() => void runDiagnostics()}>{Object.values(running).includes("running") ? <RefreshCw size={16} className="spin" /> : <Play size={16} />} Запустить диагностику</button>}</div><div className="ai-command-list">{analysis.commands.map((command, index) => { const result = results.find((item) => item.id === command.id); const state = running[command.id]; return <article className={`ai-command ${command.risk}`} key={command.id}><div className="ai-command-number">{index + 1}</div><div className="ai-command-main"><div><strong>{command.title}</strong><span className={`ai-risk ${command.risk}`}>{command.risk === "read" ? "Только чтение" : "Изменяет систему"}</span></div><p>{command.explanation}</p><details><summary>Показать команду</summary><pre><code>{command.command}</code></pre></details>{result && <details className={`ai-command-result ${result.error || result.exitCode ? "failed" : "success"}`} open><summary>{result.error || result.exitCode ? "Результат требует внимания" : "Проверка выполнена"}</summary><pre>{result.error || result.output || "Команда завершена без текстового вывода"}</pre></details>}</div><button className={command.risk === "change" ? "danger-outline-button" : "secondary-button"} disabled={!canExecute || !selected?.online || state === "running"} onClick={() => void runCommand(command)}>{state === "running" ? <RefreshCw size={15} className="spin" /> : state === "succeeded" ? <CheckCircle2 size={15} /> : <Play size={15} />}{command.risk === "change" ? "Подтвердить" : "Выполнить"}</button></article>; })}</div></section>}<div className="ai-privacy"><ShieldCheck size={16} /><span>{analysis.privacyNote}</span></div></div>}
    </div>
    {error && <div className="ai-error"><AlertTriangle size={16} />{error}</div>}
    <form className="ai-composer" onSubmit={(event) => { event.preventDefault(); setResults([]); void analyze([]); }}><textarea value={question} onChange={(event) => setQuestion(event.target.value)} placeholder="Например: почему на этом компьютере нет интернета?" rows={2} maxLength={2000} /><button className="primary-button" disabled={busy || !deviceId || !question.trim()}>{busy ? <RefreshCw size={18} className="spin" /> : <Send size={18} />}<span>{analysis ? "Спросить ещё" : "Анализировать"}</span></button></form>
  </aside></div>;
}

function Stat({ icon: Icon, label, value, note, tone }: { icon: typeof Activity; label: string; value: string; note: string; tone: string }) {
  return <article className={`stat-card stat-${tone}`}><span className={`stat-icon ${tone}`}><Icon size={20} /></span><div><span>{label}</span><strong>{value}{tone === "green" && <em><i /> В сети</em>}</strong><small>{note}</small></div>{tone === "green" && <svg className="stat-sparkline" viewBox="0 0 120 56" aria-hidden="true"><path d="M2 47 L18 40 L31 44 L46 34 L61 36 L76 29 L89 31 L102 20 L110 8 L118 3" /></svg>}</article>;
}

function DeviceOSIcon({ os, size = 19 }: { os: string; size?: number }) {
	const normalized = os.toLowerCase();
	if (normalized.includes("windows")) return <span className="os-glyph os-windows" aria-label="Windows"><i /><i /><i /><i /></span>;
	if (["mac", "darwin", "os x", "ios", "iphone", "ipad"].some((name) => normalized.includes(name))) return <Apple size={size} aria-label="Apple" />;
	if (normalized.includes("android")) return <svg className="os-brand-svg os-android" width={size} height={size} viewBox="0 0 24 24" role="img" aria-label="Android"><path d="M7 9h10v7.2a1.8 1.8 0 0 1-1.8 1.8H8.8A1.8 1.8 0 0 1 7 16.2V9Z" /><path d="M8 9a4 4 0 0 1 8 0M9 5.2 7.8 3.8M15 5.2l1.2-1.4M5 10v6M19 10v6M9.5 18v2.2M14.5 18v2.2" /><circle cx="9.8" cy="7.3" r=".65" /><circle cx="14.2" cy="7.3" r=".65" /></svg>;
	if (normalized.includes("chrome")) return <span className="os-glyph os-chrome" role="img" aria-label="ChromeOS"><i /></span>;
	if (normalized.includes("ubuntu")) return <svg className="os-brand-svg os-ubuntu" width={size} height={size} viewBox="0 0 24 24" role="img" aria-label="Ubuntu"><circle cx="12" cy="12" r="4.2" /><circle cx="12" cy="3.4" r="1.8" /><circle cx="4.55" cy="16.3" r="1.8" /><circle cx="19.45" cy="16.3" r="1.8" /><path d="M8 5.1a8.2 8.2 0 0 0-4 7M6.4 19.2a8.2 8.2 0 0 0 11.2 0M20 12.1a8.2 8.2 0 0 0-4-7" /></svg>;
	if (["linux", "debian", "centos", "fedora", "red hat", "rhel", "alpine", "suse"].some((name) => normalized.includes(name))) return <svg className="os-brand-svg os-linux" width={size} height={size} viewBox="0 0 24 24" role="img" aria-label="Linux"><path d="M12 2.2c-2.6 0-4 2.3-4 5.3 0 1.2.2 2.2-.5 3.4-1.4 2.4-2.5 4.6-1.2 7.4.7 1.6 2.1 2.5 3.8 2.5.7 0 1.3-.2 1.9-.5.6.3 1.2.5 1.9.5 1.7 0 3.1-.9 3.8-2.5 1.3-2.8.2-5-1.2-7.4-.7-1.2-.5-2.2-.5-3.4 0-3-1.4-5.3-4-5.3Z" /><path className="os-linux-belly" d="M12 10.2c-2 0-3.5 2.2-3.5 5 0 2.4 1.2 4.1 3.5 4.1s3.5-1.7 3.5-4.1c0-2.8-1.5-5-3.5-5Z" /><circle className="os-linux-eye" cx="10.4" cy="7" r=".65" /><circle className="os-linux-eye" cx="13.6" cy="7" r=".65" /><path className="os-linux-beak" d="m10.5 8.7 1.5 1 1.5-1-1.5-.8-1.5.8Z" /></svg>;
	if (normalized.includes("server") || normalized.includes("freebsd") || normalized.includes("unix")) return <Server size={size} aria-label="Сервер" />;
	return <Monitor size={size} aria-label={os || "Компьютер"} />;
}

function remoteScreenSupportedOS(os: string) {
	const normalized = os.toLowerCase();
	return normalized.includes("windows") || normalized.includes("android");
}

function DeviceRow({ device, onOpen, onRemote }: { device: Device; onOpen: () => void; onRemote: () => void }) {
  const remoteAvailable = device.accessGranted && device.online && remoteScreenSupportedOS(device.os) && versionAtLeast(device.agentVersion, "0.6.0");
  const remoteTitle = !device.accessGranted ? "Сначала разблокируйте устройство" : remoteAvailable ? "Открыть удалённый доступ" : "Удалённый доступ сейчас недоступен";
  const oldAgent = !versionAtLeast(device.agentVersion, LATEST_AGENT_VERSION);
  return <tr><td><div className="device-name"><span className={`device-icon ${device.online ? "online" : ""}`}><DeviceOSIcon os={device.os} /></span><div><strong>{device.name}{device.accessProtected && <LockKeyhole className="inline-device-lock" size={13} aria-label="Защищено паролем" />}</strong><small>{device.hostname || device.group}</small></div></div></td><td><code>{device.connectionCode}</code></td><td><div className="stacked"><strong>{device.os || "Неизвестно"}</strong><small>{device.osVersion || device.arch || "—"}{oldAgent ? ` · агент ${device.agentVersion || "старый"}` : ""}</small></div></td><td><div className="stacked"><strong>{device.publicIp || "—"}</strong><small>{device.localIps?.[0] || "нет локального IP"}</small></div></td><td>{device.currentUser || "—"}</td><td>{device.online ? "сейчас" : formatRelative(device.lastSeen)}</td><td><div className="device-status-stack"><span className={`status-pill ${device.online ? "is-online" : "is-offline"}`}><span />{device.online ? "В сети" : "Не в сети"}</span>{oldAgent && <span className="agent-version-warning">{device.agentVersion ? `Старый Agent ${device.agentVersion}` : "Версия Agent неизвестна"}</span>}</div></td><td><div className="row-actions"><button className="row-remote" aria-label={`Удалённый доступ — ${device.name}`} title={remoteTitle} disabled={!remoteAvailable} onClick={onRemote}><ScreenShare size={16} /><span>Подключиться</span></button><button className="row-menu" aria-label="Открыть устройство" onClick={onOpen}><MoreHorizontal size={18} /></button></div></td></tr>;
}

function LoadingRows() {
  return <tr className="loading-row"><td colSpan={8}><span className="loading-inline"><RefreshCw size={17} className="spin" /> Загрузка данных…</span></td></tr>;
}

function PanelLoader() {
	return <div className="panel-loading"><RefreshCw size={18} className="spin" /><span>Загрузка данных…</span></div>;
}

function SessionsPage({ devices, onOpen }: { devices: Device[]; onOpen: (device: Device) => void }) {
  const [query, setQuery] = useState("");
  const [platform, setPlatform] = useState("all");
  const [status, setStatus] = useState("all");
  const active = devices.filter((device) => device.online);
  const visible = devices.filter((device) => {
    const text = query.trim().toLowerCase();
    const isWindows = device.os.toLowerCase().includes("windows");
    const matchesText = !text || [device.name, device.hostname, device.connectionCode, device.publicIp, device.currentUser].some((value) => value?.toLowerCase().includes(text));
    return matchesText && (platform === "all" || (platform === "windows" ? isWindows : !isWindows)) && (status === "all" || (status === "online" ? device.online : !device.online));
  });
  return <>
    <section className="page-heading"><div><span className="eyebrow">ПОДКЛЮЧЕНИЯ АГЕНТОВ</span><h1>Сеансы</h1><p>Текущие подключения устройств, состояние Agent и доступные административные действия.</p></div></section>
    <section className="stats-grid">
      <Stat icon={Activity} label="Активные сеансы" value={String(active.length)} note={`из ${devices.length} устройств`} tone="green" />
      <Stat icon={Monitor} label="Windows" value={String(active.filter((d) => d.os.toLowerCase().includes("windows")).length)} note="сейчас подключены" tone="blue" />
      <Stat icon={Server} label="Linux / macOS" value={String(active.filter((d) => !d.os.toLowerCase().includes("windows")).length)} note="сейчас подключены" tone="violet" />
      <Stat icon={Clock3} label="Ожидают подключения" value={String(devices.length - active.length)} note="Agent не в сети" tone="amber" />
    </section>
    <section className="device-panel sessions-panel"><div className="session-toolbar"><label className="panel-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск по устройству, пользователю или ID" /></label><label><span>Платформа</span><select value={platform} onChange={(event) => setPlatform(event.target.value)}><option value="all">Все</option><option value="windows">Windows</option><option value="other">Linux / macOS</option></select></label><label><span>Статус</span><select value={status} onChange={(event) => setStatus(event.target.value)}><option value="all">Все</option><option value="online">Активные</option><option value="offline">Ожидают</option></select></label><span className="filter-count"><ListFilter size={15} /> {visible.length}</span></div><div className="table-wrap"><table className="sessions-table"><thead><tr><th>Устройство</th><th>Remote ID</th><th>Пользователь ПК</th><th>Последняя активность</th><th>Режим</th><th>Права</th><th>Agent</th><th>Статус</th><th>Действия</th></tr></thead><tbody>{visible.map((device) => <tr key={device.id}><td><div className="device-name"><span className={`device-icon ${device.online ? "online" : ""}`}><DeviceOSIcon os={device.os} size={18} /></span><div><strong>{device.name}{device.accessProtected && <LockKeyhole className="inline-device-lock" size={13} />}</strong><small>{device.os} · {device.publicIp || device.localIps?.[0] || "IP неизвестен"}</small></div></div></td><td><code>{device.connectionCode}</code></td><td><div className="stacked"><strong>{device.currentUser || "—"}</strong><small>{device.hostname || "пользователь не определён"}</small></div></td><td>{device.online ? "сейчас" : formatRelative(device.lastSeen)}</td><td><div className="session-mode"><Monitor size={15} /><span><strong>{device.online ? "Доступен" : "Ожидание"}</strong><small>{device.online && device.os.toLowerCase().includes("windows") ? "удалённый доступ готов" : "агентский канал"}</small></span></div></td><td><div className="session-rights"><ShieldCheck size={15} /><span><strong>{device.privileged ? "Системные" : "Пользовательские"}</strong><small>{device.privileged ? "полные права Agent" : "ограниченный режим"}</small></span></div></td><td><div className="stacked"><strong>{device.agentVersion || "—"}</strong><small>{device.agentVersion === LATEST_AGENT_VERSION ? "актуальная" : `нужна ${LATEST_AGENT_VERSION}`}</small></div></td><td><span className={`status-pill ${device.online ? "is-online" : "is-offline"}`}><span />{device.online ? "Активен" : "Не в сети"}</span></td><td><button className="secondary-button compact-action" onClick={() => onOpen(device)}><MoreHorizontal size={15} /> Подробнее</button></td></tr>)}</tbody></table>{visible.length === 0 && <div className="empty-state"><div className="empty-icon"><Activity size={30} /></div><h3>{devices.length ? "Сеансы не найдены" : "Сеансов пока нет"}</h3><p>{devices.length ? "Измените фильтры или строку поиска." : "После установки Agent устройство появится здесь автоматически."}</p></div>}</div><div className="compact-list-footer"><span>Показано {visible.length} из {devices.length}</span><small>{active.length} активных подключений</small></div></section>
  </>;
}

function RemoteControlPage({ devices, currentUser, csrf, initialDeviceId, onAccessChanged, onOpenDevice }: { devices: Device[]; currentUser: User; csrf: string; initialDeviceId: string; onAccessChanged: (deviceId: string, accessProtected: boolean, accessGranted: boolean) => void; onOpenDevice: (device: Device) => void }) {
	const [deviceId, setDeviceId] = useState(() => initialDeviceId || "");
	const [controlSessionId, setControlSessionId] = useState("");
	const appliedInitialDeviceId = useRef("");

  useEffect(() => {
		// initialDeviceId is a one-time hand-off from the Devices page. Reapplying
		// the same value after every local selection made the list appear clickable
		// while immediately snapping back to the first computer.
		if (initialDeviceId !== appliedInitialDeviceId.current && initialDeviceId && devices.some((item) => item.id === initialDeviceId)) {
			appliedInitialDeviceId.current = initialDeviceId;
      setDeviceId(initialDeviceId);
      return;
    }
		if (deviceId && !devices.some((item) => item.id === deviceId)) {
			setControlSessionId("");
			setDeviceId("");
		}
  }, [devices, deviceId, initialDeviceId]);

	const switchDevice = (nextDeviceId: string) => {
		// A preview owns and deletes its session in effect cleanup. Once a preview
		// has been handed to the control workspace, the page owns that session and
		// ends it explicitly so the old Agent stops immediately instead of waiting
		// for the 45 second viewer timeout.
		const previousControlSession = controlSessionId;
		setControlSessionId("");
		// Repeating the click is an explicit collapse action. Unmounting the
		// preview also closes its server-side session immediately.
		setDeviceId(nextDeviceId === deviceId ? "" : nextDeviceId);
		if (previousControlSession) {
			void fetch(`/api/desktop-sessions/${previousControlSession}`, { method: "DELETE", credentials: "same-origin", headers: { "X-CSRF-Token": csrf } });
		}
	};

  const device = devices.find((item) => item.id === deviceId);
  return <>
    <section className="page-heading remote-control-heading">
      <div><span className="eyebrow">УДАЛЁННЫЙ РАБОЧИЙ СТОЛ</span><h1>Удалённый доступ</h1><p>Живой экран компьютера, управление мышью и клавиатурой в одном защищённом сеансе.</p></div>
      {device && <div className="remote-control-current"><span className={`status-pill ${device.online ? "is-online" : "is-offline"}`}><span />{device.online ? "В сети" : "Не в сети"}</span><strong>{device.name}</strong><small>Remote ID {device.connectionCode}</small></div>}
    </section>
    {devices.length === 0 ? <section className="device-panel empty-state"><div className="empty-icon"><ScreenShare /></div><h3>Нет устройств для подключения</h3><p>Установите RemoteIt Agent — компьютер автоматически появится в разделе удалённого доступа.</p></section> : <section className="remote-control-layout">
      <aside className="remote-device-list">
        <header><strong>Компьютеры</strong><small>{devices.filter((item) => item.online).length} в сети</small></header>
        <div>{devices.map((item) => {
          const available = item.accessGranted && item.online && remoteScreenSupportedOS(item.os) && versionAtLeast(item.agentVersion, "0.6.0");
          const availability = !item.accessGranted ? "нужен пароль" : available ? "готов к управлению" : item.online ? "удалённый экран недоступен" : "не в сети";
          return <button key={item.id} className={item.id === deviceId ? "active" : ""} aria-current={item.id === deviceId ? "true" : undefined} onClick={() => switchDevice(item.id)}><span className={`device-icon ${item.online ? "online" : ""}`}><DeviceOSIcon os={item.os} size={17} /></span><span><strong>{item.name}{item.accessProtected && <LockKeyhole size={12} />}</strong><small>{item.connectionCode} · {availability}</small></span><span className={`remote-device-dot ${available ? "ready" : ""}`} /></button>;
        })}</div>
      </aside>
      <main className="remote-control-stage">
        {device && (!device.accessGranted ? <DeviceAccessPanel key={`access-${device.id}`} device={device} currentUser={currentUser} csrf={csrf} onChanged={onAccessChanged} gate /> : controlSessionId ? <RemoteDesktopModal key={`control-${device.id}-${controlSessionId}`} device={device} csrf={csrf} initialSessionId={controlSessionId} embedded onClose={() => setControlSessionId("")} /> : <RemoteDesktopPreview key={`preview-${device.id}`} device={device} csrf={csrf} onConnect={setControlSessionId} />)}
		{!device && <div className="remote-control-placeholder"><span className="remote-control-placeholder-icon"><ScreenShare size={30} /></span><h2>Выберите компьютер</h2><p>Предпросмотр не запускается автоматически. Нажмите на компьютер слева, чтобы открыть живой экран; нажмите повторно, чтобы свернуть его.</p></div>}
		{device && !controlSessionId && <div className="remote-control-help"><div><MousePointer2 size={18} /><span><strong>Мышь</strong><small>Нажмите на предпросмотр, затем управляйте курсором и колёсиком.</small></span></div><div><TerminalSquare size={18} /><span><strong>Клавиатура</strong><small>Физическая клавиатура работает сразу; на Android используйте кнопку клавиатуры в сеансе.</small></span></div><div><ShieldCheck size={18} /><span><strong>Контроль доступа</strong><small>Пассивный предпросмотр не беспокоит пользователя. При первом управляющем действии Agent покажет уведомление, а события сохранятся в журнале.</small></span></div></div>}
      </main>
    </section>}
  </>;
}

function ScriptsPage({ devices, currentUser, csrf, onAccessChanged }: { devices: Device[]; currentUser: User; csrf: string; onAccessChanged: (deviceId: string, accessProtected: boolean, accessGranted: boolean) => void }) {
  const [deviceId, setDeviceId] = useState(() => devices.find((device) => device.online)?.id || devices[0]?.id || "");
  const [sending, setSending] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
	const [query, setQuery] = useState("");
  useEffect(() => {
    if (!devices.some((device) => device.id === deviceId)) setDeviceId(devices.find((device) => device.online)?.id || devices[0]?.id || "");
  }, [devices, deviceId]);
  const device = devices.find((item) => item.id === deviceId);
  const scripts = diagnosticCommands(device?.os || "Windows");
	const categoryMeta = [
		{ icon: Monitor, description: "Диагностика операционной системы", tone: "violet" },
		{ icon: Wifi, description: "Сетевые интерфейсы, адреса и маршруты", tone: "green" },
		{ icon: Cpu, description: "Анализ запущенных процессов и нагрузки", tone: "amber" },
		{ icon: Settings, description: "Состояние системных служб", tone: "blue" },
		{ icon: HardDrive, description: "Диски, разделы и свободное место", tone: "violet" },
	];
	const visibleScripts = scripts.map((script, index) => ({ ...script, ...categoryMeta[index] })).filter((script) => !query.trim() || `${script.label} ${script.description} ${script.command}`.toLowerCase().includes(query.trim().toLowerCase()));

  async function runScript(label: string, command: string) {
    if (!device) return;
    setSending(label); setMessage(""); setError("");
    try {
      await api(`/api/devices/${device.id}/jobs`, { method: "POST", body: JSON.stringify({ type: "shell", command, timeoutSeconds: 45 }) }, csrf);
      setMessage(`Сценарий «${label}» отправлен на ${device.name}. Результат появится в терминале.`);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось запустить сценарий"); }
    finally { setSending(""); }
  }

  return <>
    <section className="page-heading terminal-heading"><div><span className="eyebrow">АВТОМАТИЗАЦИЯ</span><h1>Сценарии</h1><p>Готовые безопасные автоматизации для диагностики, обслуживания и поиска проблем.</p></div><label className="device-picker"><span>Устройство</span><select value={deviceId} onChange={(event) => setDeviceId(event.target.value)}>{devices.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.connectionCode}{!item.accessGranted ? " · нужен пароль" : item.online ? " · в сети" : " · не в сети"}</option>)}</select></label></section>
    {device ? !device.accessGranted ? <DeviceAccessPanel device={device} currentUser={currentUser} csrf={csrf} onChanged={onAccessChanged} gate /> : <>
      <section className="scenario-stats"><article><span className="stat-icon amber"><Star size={20} /></span><div><small>Доступно</small><strong>{scripts.length}</strong><p>готовых сценариев</p></div></article><article><span className="stat-icon blue"><Activity size={20} /></span><div><small>Диагностики</small><strong>3</strong><p>система, сеть, процессы</p></div></article><article><span className="stat-icon violet"><Settings size={20} /></span><div><small>Обслуживание</small><strong>1</strong><p>службы устройства</p></div></article><article><span className="stat-icon green"><HardDrive size={20} /></span><div><small>Хранилище</small><strong>1</strong><p>диски и свободное место</p></div></article><article><span className="stat-icon blue"><DeviceOSIcon os={device.os} size={20} /></span><div><small>Платформа</small><strong className="scenario-os-value">{device.os}</strong><p>{device.privileged ? "системные права" : "права пользователя"}</p></div></article></section>
      <section className="scenario-library"><div className="scenario-toolbar"><label className="panel-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск сценариев и действий" /></label><span><SlidersHorizontal size={15} /> Безопасные команды</span></div><div className="scenario-library-title"><h2>Категории сценариев</h2><small>{visibleScripts.length} из {scripts.length}</small></div><div className="scenario-categories">{visibleScripts.map((script) => { const Icon = script.icon; return <article key={script.label} className={`scenario-category scenario-${script.tone}`}><span className="scenario-category-icon"><Icon size={22} /></span><div className="scenario-category-copy"><strong>{script.label}</strong><small>{script.description}</small></div><span className="scenario-count">1 сценарий</span><div className="scenario-popular"><small>Готовая команда</small><button disabled={Boolean(sending) || !device.online} onClick={() => void runScript(script.label, script.command)}><Activity size={14} />{sending === script.label ? "Выполняется…" : script.label}</button><code title={script.command}>{script.command}</code></div><ChevronDown size={17} /></article>; })}{visibleScripts.length === 0 && <div className="empty-state"><Search size={28} /><h3>Сценарии не найдены</h3><p>Измените строку поиска.</p></div>}</div></section>
      {message && <div className="notice scenario-feedback"><CheckCircle2 size={17} /><span>{message}</span></div>}{error && <div className="panel-error scenario-feedback">{error}</div>}{!device.online && <div className="notice limited-notice scenario-feedback"><AlertTriangle size={17} /><span>Устройство не в сети. Подключите Agent перед запуском сценария.</span></div>}
    </> : <section className="device-panel empty-state"><div className="empty-icon"><FileCode2 /></div><h3>Нет подключённых устройств</h3><p>Установите Agent, чтобы запускать готовые сценарии.</p></section>}
  </>;
}

function TerminalPage({ devices, currentUser, csrf, onAccessChanged }: { devices: Device[]; currentUser: User; csrf: string; onAccessChanged: (deviceId: string, accessProtected: boolean, accessGranted: boolean) => void }) {
  const [deviceId, setDeviceId] = useState(() => devices.find((device) => device.online)?.id || devices[0]?.id || "");
  useEffect(() => {
    if (!devices.some((device) => device.id === deviceId)) setDeviceId(devices.find((device) => device.online)?.id || devices[0]?.id || "");
  }, [devices, deviceId]);
  const device = devices.find((item) => item.id === deviceId);
  return <>
    <section className="page-heading terminal-heading">
      <div><span className="eyebrow">УДАЛЁННОЕ АДМИНИСТРИРОВАНИЕ</span><h1>Терминал</h1><p>Команды выполняются в контексте установленного агента и сохраняются в журнале.</p></div>
      <label className="device-picker"><span>Устройство</span><select value={deviceId} onChange={(event) => setDeviceId(event.target.value)}>{devices.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.connectionCode}{!item.accessGranted ? " · нужен пароль" : item.online ? " · в сети" : " · не в сети"}</option>)}</select></label>
    </section>
    {device ? !device.accessGranted ? <DeviceAccessPanel device={device} currentUser={currentUser} csrf={csrf} onChanged={onAccessChanged} gate /> : <RemoteConsole device={device} currentUser={currentUser} csrf={csrf} /> : <section className="device-panel empty-state"><div className="empty-icon"><TerminalSquare /></div><h3>Нет подключённых устройств</h3><p>Сначала установите агент на компьютер или сервер.</p></section>}
  </>;
}

function RemoteConsole({ device, currentUser, csrf, compact = false }: { device: Device; currentUser: User; csrf: string; compact?: boolean }) {
  const [jobs, setJobs] = useState<AgentJob[]>([]);
  const [command, setCommand] = useState("");
	const normalizedOS = device.os.toLowerCase();
	const isWindows = normalizedOS.includes("windows");
	const isMac = normalizedOS.includes("mac") || normalizedOS.includes("darwin");
	const defaultShell = isWindows ? "powershell" : isMac ? "zsh" : "bash";
	const [shell, setShell] = useState(defaultShell);
  const [timeoutSeconds, setTimeoutSeconds] = useState(30);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const canShell = currentUser.role === "owner" || currentUser.role === "admin";
	const quickActions = diagnosticCommands(device.os, shell);
  const visibleJobs = jobs.filter((job) => !["files_list", "files_read", "files_write"].includes(job.type));
	const shellOptions = isWindows ? [{ id: "powershell", label: "PowerShell" }, { id: "cmd", label: "CMD" }] : isMac ? [{ id: "zsh", label: "Zsh" }, { id: "bash", label: "Bash" }] : [{ id: "bash", label: "Bash" }];
	const shellLabel = shellOptions.find((item) => item.id === shell)?.label || "Shell";
	const terminalUser = (device.currentUser || (isWindows ? "Administrator" : "admin")).replace(/^.*[\\/]/, "").replace(/\$$/, "") || "admin";
	const terminalHost = device.hostname || device.name;
	const platformClass = isWindows ? "windows" : isMac ? "macos" : "linux";
	const prompt = shell === "powershell" ? "PS C:\\>" : shell === "cmd" ? "C:\\>" : isMac && shell === "zsh" ? `${terminalUser}@${terminalHost} ~ %` : `${terminalUser}@${terminalHost}:~$`;
	const terminalTitle = shell === "powershell" ? `Windows PowerShell — ${device.name}` : shell === "cmd" ? `Командная строка — ${device.name}` : `${terminalUser}@${terminalHost}: ~ — ${shellLabel}`;
	const placeholder = shell === "powershell" ? "Get-ComputerInfo | Select-Object WindowsProductName, OsVersion" : shell === "cmd" ? "systeminfo" : isMac ? "sw_vers && uptime" : "uname -a && uptime";
	const memoryPercent = device.memoryBytes ? Math.min(100, Math.round(device.memoryUsedBytes / device.memoryBytes * 100)) : 0;
	const diskUsed = Math.max(0, device.diskTotalBytes - device.diskFreeBytes);
	const diskPercent = device.diskTotalBytes ? Math.min(100, Math.round(diskUsed / device.diskTotalBytes * 100)) : 0;

	useEffect(() => { setShell(defaultShell); setCommand(""); }, [device.id, defaultShell]);

  const loadJobs = useCallback(async () => {
    try {
      const result = await api<{ jobs: AgentJob[] }>(`/api/devices/${device.id}/jobs`);
      setJobs(result.jobs);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось загрузить задания");
    } finally {
      setLoading(false);
    }
  }, [device.id]);

  useEffect(() => {
    setLoading(true); setJobs([]); setExpanded(null); void loadJobs();
    const timer = window.setInterval(() => void loadJobs(), 3000);
    return () => window.clearInterval(timer);
  }, [loadJobs]);

  async function createJob(type: "shell" | "inventory", shellCommand = "") {
    setSending(true); setError("");
    try {
      await api(`/api/devices/${device.id}/jobs`, { method: "POST", body: JSON.stringify({ type, command: shellCommand, shell: type === "shell" ? shell : undefined, timeoutSeconds }) }, csrf);
      if (type === "shell") setCommand("");
      await loadJobs();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось создать задание");
    } finally {
      setSending(false);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!command.trim()) return;
    await createJob("shell", command);
  }

  async function cancelJob(jobId: string) {
    try {
      await api(`/api/jobs/${jobId}/cancel`, { method: "POST" }, csrf);
      await loadJobs();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось отменить задание");
    }
  }

  return <section className={`remote-console ${compact ? "compact" : ""}`}>
    <div className="console-toolbar"><div className="console-device"><span className={`device-icon ${device.online ? "online" : ""}`}><DeviceOSIcon os={device.os} size={20} /></span><span><span className={`status-pill ${device.online ? "is-online" : "is-offline"}`}><span />{device.online ? "Агент в сети" : "Задание будет ждать агента"}</span><strong>{device.name}</strong><small>{device.os} {device.osVersion} · Remote ID {device.connectionCode}</small></span></div><button className="secondary-button" disabled={sending} onClick={() => void createJob("inventory")}><RefreshCw size={16} className={sending ? "spin" : ""} /> Обновить данные</button></div>
    <div className="terminal-resource-strip"><TerminalMetric icon={Cpu} label="Загрузка ЦП" value={`${Math.round(device.cpuLoadPercent || 0)}%`} percent={device.cpuLoadPercent || 0} /><TerminalMetric icon={Database} label="Память" value={device.memoryBytes ? `${memoryPercent}% · ${formatBytes(device.memoryUsedBytes)}` : "Ожидаем данные"} percent={memoryPercent} /><TerminalMetric icon={HardDrive} label="Диск" value={device.diskTotalBytes ? `${diskPercent}% · ${formatBytes(device.diskFreeBytes)} свободно` : "Ожидаем данные"} percent={diskPercent} /><TerminalMetric icon={Clock3} label="Время работы" value={device.uptimeSeconds ? formatUptime(device.uptimeSeconds) : "Ожидаем данные"} /></div>
    {canShell ? <form className="terminal-form" onSubmit={submit}>
      <div className="terminal-shell-bar">{shellOptions.length > 1 ? <div className="terminal-shell-tabs" role="tablist" aria-label="Командная оболочка">{shellOptions.map((item) => <button type="button" role="tab" aria-selected={shell === item.id} className={shell === item.id ? "active" : ""} key={item.id} onClick={() => { setShell(item.id); setCommand(""); }}><TerminalSquare size={15} />{item.label}</button>)}</div> : <span className="terminal-shell-single"><TerminalSquare size={15} /> Bash</span>}<span><ShieldCheck size={14} /> UTF-8</span></div>
      <div className="terminal-workspace"><div className="terminal-main"><div className={`terminal-screen terminal-platform-${platformClass} terminal-shell-${shell}`}><div className="terminal-title">{isMac ? <><span /><span /><span /></> : <TerminalSquare size={14} />}<strong>{terminalTitle}</strong></div><div className="terminal-input"><span>{prompt}</span><textarea value={command} onChange={(event) => setCommand(event.target.value)} placeholder={placeholder} maxLength={8192} rows={compact ? 3 : 11} spellCheck={false} /></div></div><div className="terminal-quick-row">{quickActions.map((action) => <button type="button" key={action.label} onClick={() => setCommand(action.command)}><FileCode2 size={14} />{action.label}</button>)}</div></div><aside className="terminal-command-library"><header><span>Быстрые команды</span><small>{shellLabel}</small></header>{quickActions.map((action) => <button type="button" key={action.label} onClick={() => setCommand(action.command)}><span><FileCode2 size={14} /><strong>{action.label}</strong></span><code>{action.command}</code></button>)}</aside></div>
      <div className="terminal-actions"><label><span>Тайм-аут</span><select value={timeoutSeconds} onChange={(event) => setTimeoutSeconds(Number(event.target.value))}><option value={15}>15 секунд</option><option value={30}>30 секунд</option><option value={60}>60 секунд</option></select></label><button className="primary-button" disabled={sending || !command.trim()}>{sending ? <RefreshCw size={17} className="spin" /> : <Play size={17} />} Выполнить</button></div>
      <div className={`notice terminal-notice ${device.privileged ? "" : "limited-notice"}`}><ShieldCheck size={17} /><span>{device.privileged ? "Команда выполняется с системными правами." : "Агент установлен без прав администратора: команда ограничена правами текущего пользователя."} Её запуск, автор и результат фиксируются в RemoteIt.</span></div>
    </form> : <div className="notice"><ShieldCheck size={17} /><span>Удалённые команды доступны владельцу и администраторам. Техник может запрашивать диагностические данные.</span></div>}
    {error && <div className="panel-error">{error}</div>}
    <div className="job-history"><div className="panel-head"><div><h2>История заданий</h2><span>{visibleJobs.length} последних операций</span></div><button className="icon-button" onClick={() => void loadJobs()} aria-label="Обновить историю"><RefreshCw size={17} className={loading ? "spin" : ""} /></button></div>
      {visibleJobs.length === 0 && !loading ? <div className="empty-jobs">Заданий для этого устройства пока нет.</div> : visibleJobs.map((job) => {
        const open = expanded === job.id;
        const jobLabel = job.type === "shell" ? job.payload.command || "Команда" : job.type === "uninstall" ? "Удаление агента" : "Обновление инвентаризации";
        return <article className={`job-card job-${job.status}`} key={job.id}><button className="job-summary" onClick={() => setExpanded(open ? null : job.id)}><span className="job-type">{job.type === "shell" ? <TerminalSquare size={17} /> : job.type === "uninstall" ? <Ban size={17} /> : <Activity size={17} />}</span><span className="job-main"><strong>{jobLabel}</strong><small>{job.type === "shell" ? `${job.payload.shell || "shell"} · ` : ""}{job.createdBy || "агент"} · {formatRelative(job.createdAt)}</small></span><span className={`job-status ${job.status}`}>{jobStatusLabel(job.status)}</span><ChevronDown size={17} className={open ? "rotate" : ""} /></button>{open && <div className="job-details">{job.output && <pre>{job.output}</pre>}{job.error && <div className="job-error">{job.error}</div>}{(job.output || job.error) && <div className="result-actions"><button onClick={() => void navigator.clipboard.writeText(job.output || job.error)}><Copy size={14} /> Копировать</button><button onClick={() => downloadText(`remoteit-${device.name}-${job.id.slice(0, 8)}.txt`, job.output || job.error)}><Download size={14} /> Скачать TXT</button></div>}<div className="job-meta"><span>Тайм-аут: {job.timeoutSeconds} сек.</span><span>Код выхода: {job.exitCode ?? "—"}</span>{job.status === "queued" && canShell && <button className="text-danger" onClick={() => void cancelJob(job.id)}>Отменить</button>}</div></div>}</article>;
      })}
    </div>
  </section>;
}

function TerminalMetric({ icon: Icon, label, value, percent }: { icon: typeof Cpu; label: string; value: string; percent?: number }) {
	return <article className="terminal-metric"><span><Icon size={16} /></span><div><small>{label}</small><strong>{value}</strong>{typeof percent === "number" && <i><b style={{ width: `${Math.max(0, Math.min(100, percent))}%` }} /></i>}</div></article>;
}

function diagnosticCommands(osName: string, shell = "") {
	const normalizedOS = osName.toLowerCase();
	if (normalizedOS.includes("windows") && shell === "cmd") return [
		{ label: "Система", command: "systeminfo" },
		{ label: "Сеть", command: "ipconfig /all" },
		{ label: "Процессы", command: "tasklist" },
		{ label: "Службы", command: "sc query state= all" },
		{ label: "Диски", command: "wmic logicaldisk get Caption,FileSystem,FreeSpace,Size 2>nul || fsutil volume diskfree C:" }
	];
	if (normalizedOS.includes("windows")) return [
		{ label: "Система", command: "Get-ComputerInfo | Select-Object WindowsProductName,WindowsVersion,OsArchitecture,CsName,CsTotalPhysicalMemory | Format-List" },
		{ label: "Сеть", command: "Get-NetIPConfiguration | Format-List InterfaceAlias,IPv4Address,IPv4DefaultGateway,DNSServer" },
		{ label: "Процессы", command: "Get-Process | Sort-Object CPU -Descending | Select-Object -First 20 Name,Id,CPU,WorkingSet | Format-Table -AutoSize" },
		{ label: "Службы", command: "Get-Service | Where-Object Status -eq 'Stopped' | Select-Object -First 40 Name,DisplayName,Status | Format-Table -AutoSize" },
		{ label: "Диски", command: "Get-Volume | Select-Object DriveLetter,FileSystemLabel,FileSystem,HealthStatus,SizeRemaining,Size | Format-Table -AutoSize" }
	];
	if (normalizedOS.includes("mac") || normalizedOS.includes("darwin")) return [
		{ label: "Система", command: "sw_vers; echo; uname -m; echo; uptime" },
		{ label: "Сеть", command: "ifconfig; echo; netstat -rn" },
		{ label: "Процессы", command: "ps aux -r | head -n 21" },
		{ label: "Службы", command: "launchctl list | head -n 50" },
		{ label: "Диски", command: "df -h; echo; diskutil list" }
	];
	return [
		{ label: "Система", command: "uname -a; echo; uptime; echo; cat /etc/os-release 2>/dev/null" },
		{ label: "Сеть", command: "ip -brief address; echo; ip route" },
		{ label: "Процессы", command: "ps aux --sort=-%cpu | head -n 21" },
		{ label: "Службы", command: "systemctl --failed --no-pager" },
		{ label: "Диски", command: "df -h; echo; lsblk" }
	];
}

function DeviceAccessPanel({ device, currentUser, csrf, onChanged, gate = false }: { device: Device; currentUser: User; csrf: string; onChanged: (deviceId: string, accessProtected: boolean, accessGranted: boolean) => void; gate?: boolean }) {
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const isOwner = currentUser.role === "owner";

  useEffect(() => {
    setPassword(""); setConfirmation(""); setError("");
    setEditing(false);
  }, [device.id, device.accessProtected]);

  async function submit(event: FormEvent) {
    event.preventDefault(); setError("");
    if (isOwner && password !== confirmation) { setError("Пароли не совпадают"); return; }
    setBusy(true);
    try {
      if (isOwner) {
        await api(`/api/devices/${device.id}/access-protection`, { method: "PUT", body: JSON.stringify({ password }) }, csrf);
        onChanged(device.id, true, true);
        setEditing(false);
      } else {
        await api(`/api/devices/${device.id}/unlock`, { method: "POST", body: JSON.stringify({ password }) }, csrf);
        onChanged(device.id, true, true);
      }
      setPassword(""); setConfirmation("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось изменить доступ к устройству");
    } finally { setBusy(false); }
  }

  async function removeProtection() {
    if (!window.confirm(`Снять парольную защиту с устройства «${device.name}»? После этого администраторы и техники снова получат доступ согласно своей роли.`)) return;
    setBusy(true); setError("");
    try {
      await api(`/api/devices/${device.id}/access-protection`, { method: "DELETE" }, csrf);
      onChanged(device.id, false, true);
      setEditing(false);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось снять защиту"); }
    finally { setBusy(false); }
  }

  async function lockNow() {
    setBusy(true); setError("");
    try {
      await api(`/api/devices/${device.id}/unlock`, { method: "DELETE" }, csrf);
      onChanged(device.id, true, false);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось заблокировать устройство"); }
    finally { setBusy(false); }
  }

  if (!device.accessProtected && !isOwner) return null;
  const title = isOwner ? device.accessProtected ? "Пароль устройства включён" : "Доступ открыт по ролям" : device.accessGranted ? "Доступ разблокирован" : "Требуется пароль устройства";
  const description = isOwner
    ? device.accessProtected
      ? "Владелец подключается без пароля. Другим администраторам и техникам потребуется пароль этого устройства."
      : "Сейчас владельцы, администраторы и техники могут подключаться согласно своей роли. При необходимости владелец может задать отдельный пароль."
    : device.accessGranted ? "Доступ действует четыре часа только в этой сессии панели." : "Владелец RemoteIt ограничил доступ. Пароль не передаётся агенту и не хранится на компьютере.";

  return <section className={`device-access-card ${device.accessProtected ? "is-protected" : "is-open"} ${gate ? "access-gate" : ""}`}>
    <div className="device-access-heading"><span className="device-access-icon">{device.accessProtected ? <LockKeyhole size={21} /> : <ShieldCheck size={21} />}</span><div><span className="eyebrow">ДОСТУП К УСТРОЙСТВУ</span><h3>{title}</h3><p>{description}</p></div></div>
    {currentUser.role === "viewer" ? <div className="notice limited-notice"><ShieldCheck size={17} /><span>У вашей учётной записи нет прав на разблокировку и управление устройствами.</span></div> : isOwner && !device.accessProtected && !editing ? <div className="device-access-actions"><span className="open-access-state"><CheckCircle2 size={17} /> Отдельный пароль не задан</span><button type="button" className="secondary-button" disabled={busy} onClick={() => setEditing(true)}><KeyRound size={16} /> Настроить пароль</button></div> : isOwner && device.accessProtected && !editing ? <div className="device-access-actions"><span className="protected-state"><CheckCircle2 size={17} /> Пароль включён</span><button type="button" className="secondary-button" disabled={busy} onClick={() => setEditing(true)}><KeyRound size={16} /> Сменить пароль</button><button type="button" className="danger-link" disabled={busy} onClick={() => void removeProtection()}>Снять защиту</button></div> : !isOwner && device.accessGranted ? <div className="device-access-actions"><span className="protected-state"><CheckCircle2 size={17} /> Разблокировано для {currentUser.username}</span><button type="button" className="secondary-button" disabled={busy} onClick={() => void lockNow()}><LockKeyhole size={16} /> Заблокировать сейчас</button></div> : <form className="device-access-form" onSubmit={submit}><label><span>{isOwner ? device.accessProtected ? "Новый пароль устройства" : "Пароль устройства" : "Пароль устройства"}</span><input type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} maxLength={128} autoComplete="new-password" required placeholder="От 8 до 128 символов" /></label>{isOwner && <label><span>Повторите пароль</span><input type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} minLength={8} maxLength={128} autoComplete="new-password" required placeholder="Введите пароль ещё раз" /></label>}<div className="device-access-submit"><button className="primary-button" disabled={busy || password.length < 8 || (isOwner && password !== confirmation)}>{busy ? <RefreshCw size={16} className="spin" /> : <KeyRound size={16} />} {isOwner ? device.accessProtected ? "Сохранить новый пароль" : "Включить защиту" : "Разблокировать"}</button>{isOwner && <button type="button" className="secondary-button" onClick={() => { setEditing(false); setPassword(""); setConfirmation(""); setError(""); }}>Отмена</button>}</div></form>}
    {error && <div className="form-error">{error}</div>}
    {isOwner && <small className="device-access-footnote">Главный владелец не вводит этот пароль и не может случайно заблокировать собственный доступ.</small>}
  </section>;
}

function DeviceDrawer({ device, currentUser, csrf, onClose, onAccessChanged, onRenamed, onDeleted }: { device: Device; currentUser: User; csrf: string; onClose: () => void; onAccessChanged: (deviceId: string, accessProtected: boolean, accessGranted: boolean) => void; onRenamed: () => void; onDeleted: () => void }) {
  const [name, setName] = useState(device.name);
	const [group, setGroup] = useState(device.group);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
	const [desktopSessionId, setDesktopSessionId] = useState("");
  const canOperate = currentUser.role !== "viewer" && device.accessGranted;
	const canDelete = currentUser.role === "owner" || currentUser.role === "admin";
	const supportsAgentJobs = !device.os.toLowerCase().includes("android");
	const supportsConfirmedUninstall = supportsAgentJobs && versionAtLeast(device.agentVersion, "0.6.0");

  async function rename(event: FormEvent) {
    event.preventDefault(); setSaving(true); setError("");
    try {
      await api(`/api/devices/${device.id}`, { method: "PATCH", body: JSON.stringify({ name, group }) }, csrf);
      onRenamed();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось переименовать устройство");
    } finally { setSaving(false); }
  }

	async function removeDevice() {
		if (!window.confirm(`Полностью удалить RemoteIt Agent с компьютера «${device.name}» и затем убрать устройство из панели? Если компьютер сейчас выключен, команда дождётся его следующего подключения.`)) return;
		setSaving(true); setError("");
		try {
			await api(`/api/devices/${device.id}/uninstall`, { method: "POST" }, csrf);
			onDeleted();
		} catch (reason) {
			setError(reason instanceof Error ? reason.message : "Не удалось поставить удаление Agent в очередь");
		} finally { setSaving(false); }
	}

	async function forgetDevice() {
		if (!window.confirm(`Удалить «${device.name}» только из панели RemoteIt? Локальный Agent на выключенном компьютере останется установленным, но его текущие данные доступа будут отозваны и устройство сразу исчезнет из панели.`)) return;
		setSaving(true); setError("");
		try {
			await api(`/api/devices/${device.id}/forget`, { method: "DELETE" }, csrf);
			onDeleted();
		} catch (reason) {
			setError(reason instanceof Error ? reason.message : "Не удалось удалить устройство из панели");
		} finally { setSaving(false); }
	}

  return <div className="drawer-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><aside className="device-drawer"><header><div><span className="eyebrow">REMOTE ID {device.connectionCode}</span><h2>{device.name}</h2><span className={`status-pill ${device.pendingRemoval ? "waiting" : device.online ? "is-online" : "is-offline"}`}><span />{device.pendingRemoval ? "Ожидает удаления агента" : device.online ? "В сети" : `Не в сети · ${formatRelative(device.lastSeen)}`}</span></div><button className="icon-button" onClick={onClose}><X size={19} /></button></header><div className="drawer-scroll">
    <section className="device-facts"><div><span>Система</span><strong>{device.os}</strong><small>{device.osVersion || device.arch}</small></div><div><span>Публичный IP</span><strong>{device.publicIp || "—"}</strong><small>{device.localIps?.join(", ") || "локальный IP неизвестен"}</small></div><div><span>Пользователь</span><strong>{device.currentUser || "—"}</strong><small>{device.hostname || "имя хоста неизвестно"}</small></div><div><span>Права агента</span><strong>{device.privileged ? "Системные" : device.installMode === "user" ? "Пользовательские" : "Не определены"}</strong><small>{device.installMode === "user" ? "Работает только в сеансе пользователя" : "Фоновая системная служба"}</small></div><div><span>Версия агента</span><strong>{device.agentVersion || "—"}</strong><small>{device.agentVersion === LATEST_AGENT_VERSION ? "актуальная версия" : `доступна ${LATEST_AGENT_VERSION}`}</small></div><div><span>Ресурсы</span><strong>{device.memoryBytes ? formatBytes(device.memoryBytes) + " RAM" : "—"}</strong><small>{device.diskTotalBytes ? `${formatBytes(device.diskFreeBytes)} свободно из ${formatBytes(device.diskTotalBytes)}` : device.cpuModel || "данные ожидаются"}</small></div></section>
    {(currentUser.role === "owner" || device.accessProtected) && <DeviceAccessPanel device={device} currentUser={currentUser} csrf={csrf} onChanged={onAccessChanged} />}
    {canOperate && <RemoteDesktopPreview device={device} csrf={csrf} onConnect={setDesktopSessionId} />}
    {desktopSessionId && <RemoteDesktopModal device={device} csrf={csrf} initialSessionId={desktopSessionId} onClose={() => setDesktopSessionId("")} />}
    {canOperate && <form className="rename-form" onSubmit={rename}><label><span>Название в RemoteIt</span><input value={name} onChange={(event) => setName(event.target.value)} maxLength={64} required /></label><label><span>Группа</span><input value={group} onChange={(event) => setGroup(event.target.value)} maxLength={100} required /></label><div className="device-edit-actions"><button className="secondary-button" disabled={saving || (name.trim() === device.name && group.trim() === device.group)}><Save size={16} /> Сохранить</button></div>{error && <div className="form-error">{error}</div>}</form>}
    {canDelete && device.accessGranted && <section className="device-removal-card"><div><strong>Удаление устройства</strong><small>{supportsConfirmedUninstall ? "Можно дождаться подтверждённой деинсталляции Agent либо сразу убрать недоступный компьютер только из панели." : "Эту версию Agent нельзя деинсталлировать с подтверждением, но устройство можно сразу удалить только из панели."}</small></div>{device.pendingRemoval && <div className="notice limited-notice"><Clock3 size={17} /><span>Удаление Agent ожидает следующего подключения. Это не загрузка: команду можно оставить в очереди либо удалить запись из панели прямо сейчас.</span></div>}<div className="device-removal-actions"><button type="button" className="danger-button" disabled={saving || device.pendingRemoval || !supportsConfirmedUninstall} onClick={() => void removeDevice()}><Ban size={16} /> {supportsConfirmedUninstall ? "Удалить Agent и устройство" : "Подтверждённое удаление недоступно"}</button><button type="button" className="secondary-button forget-device-button" disabled={saving} onClick={() => void forgetDevice()}><Trash2 size={16} /> Удалить только из панели</button></div></section>}
    {canOperate && supportsAgentJobs && <RemoteFiles device={device} csrf={csrf} />}
    {canOperate && supportsAgentJobs && <RemoteConsole device={device} currentUser={currentUser} csrf={csrf} compact />}
  </div></aside></div>;
}

type DesktopSession = {
  id: string;
  deviceId: string;
  status: "active" | "ended" | "expired";
	controlEnabled: boolean;
	targetFps: number;
	cursorVisible: boolean;
  frameWidth: number;
  frameHeight: number;
  frameAt: string | null;
  agentConnected: boolean;
  agentError: string;
	inputAck?: {
		id: number;
		type: string;
		error: string;
		at: string;
	} | null;
	clipboardAck?: {
		id: number;
		type: string;
		error: string;
		value?: string;
		mime?: string;
		at: string;
	} | null;
  captureDiagnostics?: {
    captureMillis: number;
    copyMillis: number;
    scaleMillis: number;
    encodeMillis: number;
    captureBackend: string;
  };
};

async function desktopFrameBlob(response: Response): Promise<Blob | null> {
  if (response.status === 204) return null;
  if (!response.ok) throw new Error(`Кадр экрана: HTTP ${response.status}`);
  const contentType = response.headers.get("Content-Type")?.toLowerCase() || "";
  if (!contentType.startsWith("image/jpeg")) throw new Error("Agent вернул некорректный формат кадра");
  const frame = await response.blob();
  if (frame.size < 100) throw new Error("Agent вернул пустой кадр");
  return frame;
}

function RemoteDesktopPreview({ device, csrf, onConnect }: { device: Device; csrf: string; onConnect: (sessionId: string) => void }) {
  const desktopCompatible = versionAtLeast(device.agentVersion, "0.6.0");
  const supported = device.online && remoteScreenSupportedOS(device.os) && desktopCompatible;
  const [sessionId, setSessionId] = useState("");
  const [frameURL, setFrameURL] = useState("");
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState("");
	const handedOff = useRef(false);
	const previewImageRef = useRef<HTMLImageElement>(null);

  useEffect(() => {
    if (!supported) return;
		handedOff.current = false;
    let disposed = false; let statusTimer = 0; let frameTimer = 0; let streamWatchdogTimer = 0; let currentURL = ""; let createdId = ""; let lastFrameAt = ""; let frameSocket: WebSocket | null = null; let fallbackActive = false; let fallbackGeneration = 0; let lastFrameReceivedAt = performance.now();
		const presentPreview = (frame: Blob) => {
			if (disposed || frame.size < 100) return;
			lastFrameReceivedAt = performance.now();
			const next = URL.createObjectURL(frame);
			if (currentURL) URL.revokeObjectURL(currentURL);
			currentURL = next;
			if (previewImageRef.current) previewImageRef.current.src = next;
			else setFrameURL(next);
		};
    const start = async () => {
      try {
        const created = await api<{ id: string }>(`/api/devices/${device.id}/desktop-sessions`, { method: "POST", body: JSON.stringify({ controlEnabled: false, targetFps: 15, cursorVisible: false }) }, csrf);
        if (disposed) return; createdId = created.id; setSessionId(created.id);
        const refreshStatus = async () => {
          if (disposed) return;
          try {
            const status = await api<DesktopSession>(`/api/desktop-sessions/${created.id}`); setConnected(status.agentConnected); setError(status.agentError || "");
          } catch (reason) { if (!disposed) setError(reason instanceof Error ? reason.message : "Предпросмотр временно недоступен"); }
          if (!disposed) statusTimer = window.setTimeout(() => void refreshStatus(), 900);
        };
				const refreshFrame = async (generation: number) => {
					if (disposed || !fallbackActive || generation !== fallbackGeneration) return;
					try {
            const after = lastFrameAt ? `&after=${encodeURIComponent(lastFrameAt)}` : "";
            const response = await fetch(`/api/desktop-sessions/${created.id}/frame?t=${Date.now()}${after}`, { credentials: "same-origin", cache: "no-store" });
            const receivedAt = response.headers.get("X-RemoteIt-Frame-At") || "";
            const frame = await desktopFrameBlob(response);
            if (frame) {
							lastFrameAt = receivedAt || lastFrameAt;
							presentPreview(frame);
						}
					} catch (reason) { if (!disposed) setError(reason instanceof Error ? reason.message : "Предпросмотр временно недоступен"); }
					if (!disposed && fallbackActive && generation === fallbackGeneration) frameTimer = window.setTimeout(() => void refreshFrame(generation), 5);
				};
				const startFallback = () => {
					if (disposed || fallbackActive) return;
					fallbackActive = true;
					const generation = ++fallbackGeneration;
					void refreshFrame(generation);
				};
				const stopFallback = () => {
					fallbackActive = false;
					fallbackGeneration += 1;
					window.clearTimeout(frameTimer);
					frameTimer = 0;
				};
				void refreshStatus();
				const streamProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
				frameSocket = new WebSocket(`${streamProtocol}//${window.location.host}/api/desktop-sessions/${created.id}/stream`);
				frameSocket.binaryType = "blob";
				frameSocket.onmessage = (event) => {
					if (event.data instanceof Blob) { stopFallback(); presentPreview(event.data); }
					else if (event.data instanceof ArrayBuffer) { stopFallback(); presentPreview(new Blob([event.data], { type: "image/jpeg" })); }
				};
				frameSocket.onerror = () => frameSocket?.close();
				frameSocket.onclose = () => startFallback();
				window.setTimeout(() => {
					if (!disposed && frameSocket && frameSocket.readyState !== WebSocket.OPEN) {
						frameSocket.close();
						startFallback();
					}
				}, 2_500);
				// OPEN only confirms a WebSocket handshake. A proxy, VPN transition or
				// suspended Android browser can leave an open stream that never delivers
				// video. Start the HTTP long-poll safety lane whenever no fresh frame has
				// arrived; a real WebSocket frame immediately stops it again.
				const watchStream = () => {
					if (disposed) return;
					if (isRemoteFrameStreamStalled(lastFrameReceivedAt, performance.now(), 900)) startFallback();
					streamWatchdogTimer = window.setTimeout(watchStream, 450);
				};
				streamWatchdogTimer = window.setTimeout(watchStream, 450);
      } catch (reason) { if (!disposed) setError(reason instanceof Error ? reason.message : "Не удалось запустить предпросмотр"); }
    };
    void start();
    return () => { disposed = true; frameSocket?.close(); window.clearTimeout(statusTimer); window.clearTimeout(frameTimer); window.clearTimeout(streamWatchdogTimer); if (currentURL) URL.revokeObjectURL(currentURL); if (createdId && !handedOff.current) void fetch(`/api/desktop-sessions/${createdId}`, { method: "DELETE", credentials: "same-origin", headers: { "X-CSRF-Token": csrf } }); setSessionId(""); setFrameURL(""); setConnected(false); };
  }, [device.id, csrf, supported]);

  const unavailableReason = !device.online ? "Агент не в сети" : !remoteScreenSupportedOS(device.os) ? "Доступно для Windows и Android" : !desktopCompatible ? `Обновите Agent ${device.agentVersion || "старой версии"} до 0.6.0` : "Предпросмотр недоступен";
  return <section className="remote-preview-card"><header><div><span className="eyebrow">УДАЛЁННЫЙ ДОСТУП</span><strong><ScreenShare size={18} /> Живой экран</strong><small>{supported ? frameURL ? "Предпросмотр онлайн · управление выключено" : connected ? "Agent подключён · ожидаем первый кадр" : "Подключаем защищённый предпросмотр…" : unavailableReason}</small></div><span className={`preview-live ${connected && !!frameURL ? "active" : ""}`}><span />{connected && frameURL ? "LIVE" : "WAIT"}</span></header><button type="button" className="remote-preview-screen" disabled={!supported || !sessionId || !frameURL} onClick={() => { if (device.os.toLowerCase().includes("windows")) primeRemoteClipboardFromConnectionGesture(); handedOff.current = true; onConnect(sessionId); }}>{frameURL ? <img ref={previewImageRef} src={frameURL} draggable={false} onError={() => { setFrameURL(""); setError("Получен повреждённый кадр — ожидаем следующий"); }} /> : <span><Monitor size={38} /><strong>{error || (supported ? "Ожидаем изображение от Agent" : unavailableReason)}</strong><small>{desktopCompatible ? "Диагностика обновляется автоматически" : "Скачайте новый агент из раздела токенов"}</small></span>}<b><MousePointer2 size={17} /> Открыть подключение</b></button><footer><ShieldCheck size={14} /> Пассивный предпросмотр журналируется, но не показывает всплывающее уведомление. Оно появится при первом управляющем действии.</footer></section>;
}

function capturePointerSafely(target: Element, pointerId: number) {
	try {
		target.setPointerCapture(pointerId);
	} catch {
		// Pointer capture is an optimisation, not a prerequisite. Older Android
		// WebViews and Safari can reject it while rotating, zooming or transferring
		// capture between two fingers; the gesture must continue instead of throwing.
	}
}

async function clipboardBlobFingerprint(blob: Blob): Promise<string> {
	const data = await blob.arrayBuffer();
	if (globalThis.crypto?.subtle) {
		const digest = await globalThis.crypto.subtle.digest("SHA-256", data);
		return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join("");
	}
	// Secure production contexts always expose SubtleCrypto. Keep a deterministic
	// bounded fallback for older embedded WebViews so unchanged images do not loop.
	const bytes = new Uint8Array(data);
	let hash = 2166136261;
	for (let index = 0; index < bytes.length; index += Math.max(1, Math.floor(bytes.length / 4096))) {
		hash = Math.imul(hash ^ bytes[index], 16777619);
	}
	return `${blob.size}:${hash >>> 0}`;
}

async function writeClipboardPNG(blob: Blob): Promise<void> {
	if (!navigator.clipboard?.write || typeof ClipboardItem === "undefined") {
		throw new Error("Этот браузер не поддерживает изображения в буфере");
	}
	await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
}

type LocalClipboardSnapshot = {
	text: string;
	image: Blob | null;
};

type PendingRemoteClipboardWrite = {
	resolve: (blob: Blob) => void;
	reject: (reason?: unknown) => void;
	timer: number;
};

let primedRemoteClipboardSnapshot: Promise<LocalClipboardSnapshot> | null = null;

async function readLocalClipboardSnapshot(): Promise<LocalClipboardSnapshot> {
	if (!navigator.clipboard?.readText) throw new Error("Этот браузер не поддерживает системный буфер");
	let image: Blob | null = null;
	let text = "";
	if (navigator.clipboard.read) {
		const items = await navigator.clipboard.read();
		for (const item of items) {
			if (item.types.includes("image/png")) {
				image = await item.getType("image/png");
				break;
			}
		}
		if (!image) {
			const textItem = items.find((item) => item.types.includes("text/plain"));
			if (textItem) text = await (await textItem.getType("text/plain")).text();
		}
	} else {
		text = await navigator.clipboard.readText();
	}
	return { text, image };
}

// Start the protected clipboard read inside the same click that opens the
// remote connection. Safari/WebKit discards that grant as soon as the event
// handler returns, so waiting for the modal and session API would lose the
// administrator's already copied command or link.
function primeRemoteClipboardFromConnectionGesture() {
	primedRemoteClipboardSnapshot = readLocalClipboardSnapshot();
}

function takePrimedRemoteClipboardSnapshot() {
	const snapshot = primedRemoteClipboardSnapshot;
	primedRemoteClipboardSnapshot = null;
	return snapshot;
}

function releasePointerSafely(target: Element, pointerId: number) {
	try {
		if (target.hasPointerCapture(pointerId)) target.releasePointerCapture(pointerId);
	} catch {
		// A browser may already have released capture after pointercancel or a visual
		// viewport change. Local gesture ownership is cleared by the caller.
	}
}

function RemoteDesktopModal({ device, csrf, initialSessionId = "", onClose, embedded = false }: { device: Device; csrf: string; initialSessionId?: string; onClose: () => void; embedded?: boolean }) {
	const targetWindows = device.os.toLowerCase().includes("windows");
  const [sessionId, setSessionId] = useState("");
  const [status, setStatus] = useState<DesktopSession | null>(null);
  const [frameURL, setFrameURL] = useState("");
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(true);
	const [latencyMs, setLatencyMs] = useState(0);
	const [frameFPS, setFrameFPS] = useState(0);
	// Android WebView sometimes advertises an emulated fine pointer even though
	// the administrator is holding a phone. Trust the native bridge / mobile UA
	// first, then fall back to the pointer media queries. A Windows hybrid laptop
	// still keeps normal desktop controls because it has neither mobile marker.
	const [coarsePointerClient] = useState(() => {
		if (typeof window === "undefined") return false;
		const androidBridge = Boolean((window as unknown as { RemoteItAndroid?: unknown }).RemoteItAndroid);
		const mobileRuntime = /RemoteIt-Android|Android|iPhone|iPad|Mobile/i.test(window.navigator.userAgent);
		return androidBridge || mobileRuntime || (window.matchMedia("(pointer: coarse)").matches && !window.matchMedia("(any-pointer: fine)").matches);
	});
	const [keyboardOpen, setKeyboardOpen] = useState(false);
	// Windows sessions use one clipboard from the moment they are opened. Browsers
	// that permit background reads keep it fully automatic; Safari/WebKit still
	// receives a trusted Cmd+C/Cmd+V path below without an extra enable switch.
	const [clipboardSyncEnabled] = useState(targetWindows);
	const [mobileText, setMobileText] = useState("");
	const [sasFeedback, setSASFeedback] = useState("");
	const [sasFeedbackError, setSASFeedbackError] = useState(false);
	const [pointerMode, setPointerMode] = useState<"direct" | "trackpad">("trackpad");
	const [filesOpen, setFilesOpen] = useState(false);
	const [desktopDropActive, setDesktopDropActive] = useState(false);
	const [desktopDropProgress, setDesktopDropProgress] = useState<ActiveFileTransfer | null>(null);
	const [screenScale, setScreenScale] = useState<RemoteScaleMode>("fit");
	const [streamStatsCollapsed, setStreamStatsCollapsed] = useState(false);
	// Auto is the default on every client. The Agent starts Auto at 30 FPS,
	// raises it to 60 on a consistently fast channel and falls back to 15 when
	// capture or upload cost cannot sustain the target without queueing frames.
	const [targetFPS, setTargetFPS] = useState<"auto" | "15" | "30" | "60">("auto");
	const [renderedFrameSize, setRenderedFrameSize] = useState({ width: 0, height: 0 });
	const [viewportSize, setViewportSize] = useState({ width: 0, height: 0 });
	const [remoteViewport, setRemoteViewport] = useState<RemoteViewport>(() => resolveRemoteViewport({
		innerWidth: typeof window === "undefined" ? 1 : window.innerWidth,
		innerHeight: typeof window === "undefined" ? 1 : window.innerHeight,
		documentWidth: typeof document === "undefined" ? 1 : document.documentElement.clientWidth,
		documentHeight: typeof document === "undefined" ? 1 : document.documentElement.clientHeight,
		visualViewport: typeof window === "undefined" || !window.visualViewport ? null : window.visualViewport,
	}));
	const compactRemoteClient = shouldUseCompactRemoteControls(coarsePointerClient, remoteViewport);
	// Phones start with an unobstructed full-screen canvas and a small floating
	// handle. The complete control surface opens only when the user asks for it.
	// Use both pointer capabilities and the actual visible viewport: a number of
	// Android WebViews incorrectly announce a fine pointer in landscape.
	const [controlsCollapsed, setControlsCollapsed] = useState(() => compactRemoteClient);
	const [mobileDockHidden, setMobileDockHidden] = useState(false);
	const [fullscreenActive, setFullscreenActive] = useState(false);
	const scaleBeforeFullscreenRef = useRef<RemoteScaleMode | null>(null);
	const explicitSessionFinishRef = useRef(false);
	const [camera, setCamera] = useState({ zoom: 1, panX: 0, panY: 0 });
	const cameraRef = useRef(camera);
	const pendingCameraRef = useRef(camera);
	const cameraAnimationFrame = useRef(0);
	const pinchSuppressionAnimationFrame = useRef(0);
	const viewportRef = useRef<HTMLDivElement>(null);
	const workspaceRef = useRef<HTMLElement>(null);
	const mobileKeyboardRef = useRef<HTMLInputElement>(null);
	const mobileTextSyncedRef = useRef("");
	const mobileBoundaryDeleteAt = useRef(0);
	const mobileEnterAt = useRef(0);
	const keyboardOpenRef = useRef(false);
	const wheelRepeat = useRef({ delay: 0, interval: 0, pointerId: -1 });
	const desktopDropDepthRef = useRef(0);
	const desktopDropControllerRef = useRef<AbortController | null>(null);
	const sasFeedbackTimer = useRef(0);
	const sasPendingInputID = useRef(0);
	const clipboardSyncEnabledRef = useRef(targetWindows);
	const clipboardInitialSnapshotRef = useRef<Promise<LocalClipboardSnapshot> | null>(targetWindows ? takePrimedRemoteClipboardSnapshot() : null);
	const clipboardLastLocalRef = useRef("");
	const clipboardLastRemoteAckRef = useRef(0);
	const clipboardLastLocalImageRef = useRef("");
	const clipboardLastRemoteImageSequenceRef = useRef(0);
	const clipboardPendingRemoteImageRef = useRef<Blob | null>(null);
	const clipboardPendingRemoteTextRef = useRef<string | null>(null);
	const clipboardPendingWriteRef = useRef<PendingRemoteClipboardWrite | null>(null);
	const clipboardBackgroundReadBlockedRef = useRef(false);
	const clipboardPollBusyRef = useRef(false);
	const clipboardPermissionErrorRef = useRef(false);
	const macCommandKeysRef = useRef(new Set<string>());
	const macCommandClient = useMemo(() => typeof navigator !== "undefined" && /Macintosh|Mac OS X|MacIntel/i.test(`${navigator.userAgent} ${navigator.platform}`), []);
	const localCursorRef = useRef<HTMLSpanElement>(null);
	const pointerMoveCadence = useRef(new LatestPointerCadence<{ event: Record<string, unknown>; activatesControl: boolean }>(8));
	const pointerMoveTimer = useRef(0);
	const pointerViewportRevision = useRef(0);
	const trackpadGestureViewportRevision = useRef(0);
	const directGestureViewportRevision = useRef(0);
	const pinchGestureViewportRevision = useRef(0);
  const trackpadCursor = useRef({ x: 0, y: 0, frameWidth: 0, frameHeight: 0, ready: false });
	const trackpadGesture = useRef({ pointerId: -1, lastX: 0, lastY: 0, lastTime: 0, distance: 0, longPress: false, dragging: false, secondTap: false, suppressTap: false, timer: 0, pendingX: 0, pendingY: 0, pendingTimer: 0 });
	const lastTrackpadTap = useRef({ at: 0, clientX: 0, clientY: 0 });
	const directGesture = useRef<{ pointerId: number; startX: number; startY: number; startRemoteX: number; startRemoteY: number; x: number; y: number; coordinateWidth: number; coordinateHeight: number; moved: boolean; leftDown: boolean; longPress: boolean; suppressTap: boolean; timer: number }>({ pointerId: -1, startX: 0, startY: 0, startRemoteX: 0, startRemoteY: 0, x: 0, y: 0, coordinateWidth: 1, coordinateHeight: 1, moved: false, leftDown: false, longPress: false, suppressTap: false, timer: 0 });
	const heldMouseButton = useRef<"left" | "middle" | "right" | null>(null);
	const lastMousePosition = useRef<{ x: number; y: number; coordinateWidth: number; coordinateHeight: number } | null>(null);
	const controlEnabledRef = useRef(false);
	const controlActivationRef = useRef<Promise<void> | null>(null);
	const frameArrivalTimes = useRef<number[]>([]);
	const activeTouches = useRef(new Map<number, { x: number; y: number }>());
	const pinchGesture = useRef({ active: false, suppress: false, mode: "pending" as "pending" | "zoom" | "scroll", startDistance: 1, lastDistance: 1, lastMidX: 0, lastMidY: 0, wheelDistance: 0, midpointTravel: 0, startedAt: 0, rightClickSent: false });
	const wheelAccumulator = useRef(0);
	const inputQueue = useRef<Record<string, unknown>[]>([]);
	const inputFlushTimer = useRef(0);
	const inputInFlight = useRef(false);
	const inputAbortController = useRef<AbortController | null>(null);
	const inputClientPrefix = useRef(`viewer-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`);
	const inputClientSequence = useRef(0);
	const inputBatchSequence = useRef(0);
	const inputPendingBatches = useRef(new Map<string, PendingRemoteInputBatch<WebSocket>>());
	const activeInputSessionID = useRef("");
	const flushInputQueueRef = useRef<() => void>(() => undefined);
	// Orientation, browser chrome and visibility changes can cancel native pointer
	// capture without delivering pointerup. Keep one stable escape hatch which
	// always calls the latest input closure and releases any remote drag before
	// local gesture coordinates are rebased.
	const releaseActivePointerRef = useRef<() => void>(() => undefined);
	const textKeyboardKeys = useRef(new Set<string>());
	const frameImageRef = useRef<HTMLImageElement>(null);
	const frameImageLayerRef = useRef<HTMLDivElement>(null);
	const frameSocketRef = useRef<WebSocket | null>(null);
	const mobileTrackpadMode = shouldUseRemoteTrackpad(compactRemoteClient, pointerMode);
	const localCursorVisible = mobileTrackpadMode;
	// Remote cursor pixels are disabled for every viewer. Desktop already has a
	// native cursor and mobile trackpad mode draws an immediate local overlay;
	// encoding a second cursor in JPEG created the duplicate/lagging pointer.
	const encodedRemoteCursorVisible = false;
	// Pointer input has its own WebSocket and must not be tied to the video FPS.
	// The cadence below sends an immediate leading sample and one newest trailing
	// sample, so 120/240 Hz phones stay responsive without leaving the last local
	// cursor position unsent or building a stale input backlog.

	useEffect(() => { cameraRef.current = camera; pendingCameraRef.current = camera; }, [camera]);
	useEffect(() => { keyboardOpenRef.current = keyboardOpen; }, [keyboardOpen]);

	useEffect(() => () => {
		window.cancelAnimationFrame(cameraAnimationFrame.current);
		window.cancelAnimationFrame(pinchSuppressionAnimationFrame.current);
	}, []);
	useEffect(() => () => {
		window.clearTimeout(sasFeedbackTimer.current);
		sasPendingInputID.current = 0;
	}, []);
	useEffect(() => () => {
		window.clearTimeout(wheelRepeat.current.delay);
		window.clearInterval(wheelRepeat.current.interval);
		desktopDropControllerRef.current?.abort();
	}, []);

	function scheduleCamera(next: { zoom: number; panX: number; panY: number }) {
		pendingCameraRef.current = clampCamera(next);
		if (cameraAnimationFrame.current) return;
		cameraAnimationFrame.current = window.requestAnimationFrame(() => {
			cameraAnimationFrame.current = 0;
			cameraRef.current = pendingCameraRef.current;
			setCamera(pendingCameraRef.current);
		});
	}

	useEffect(() => {
		const viewport = viewportRef.current;
		if (!viewport) return;
		const delayedUpdates = new Set<number>();
		const readVisibleViewport = () => resolveRemoteViewport({
			innerWidth: window.innerWidth,
			innerHeight: window.innerHeight,
			documentWidth: document.documentElement.clientWidth,
			documentHeight: document.documentElement.clientHeight,
			visualViewport: window.visualViewport,
		});
		const readLayoutLandscape = () => resolveRemoteLayoutLandscape({
			innerWidth: window.innerWidth,
			innerHeight: window.innerHeight,
			documentWidth: document.documentElement.clientWidth,
			documentHeight: document.documentElement.clientHeight,
		});
		const initialVisible = readVisibleViewport();
		let lastLandscape = initialVisible.landscape;
		let lastPointerViewport = initialVisible;
		let lastPointerSurface = { width: viewport.clientWidth, height: viewport.clientHeight };
		const applyOrientationState = (visible: RemoteViewport, force = false, layoutLandscape: boolean | null = null) => {
			if (!shouldApplyRemoteOrientationTransition(lastLandscape, visible.landscape, keyboardOpenRef.current, force, layoutLandscape)) return;
			lastLandscape = visible.landscape;
			if (!compactRemoteClient) return;
			releaseActivePointerRef.current();
			// Browsers do not agree on whether rotating an already-open page emits
			// orientationchange, resize, both, or only visualViewport.resize. Drive
			// the compact controller from the measured rectangle so every engine
			// reaches the same state without treating an IME resize as a rotation.
			setMobileKeyboardVisibility(false);
			setControlsCollapsed(true);
			setMobileDockHidden(false);
			// A real rotation starts from one predictable camera: the complete remote
			// desktop fitted into the final visual viewport. Retaining a portrait zoom
			// or fixed percentage in landscape forced the administrator to pan up/down.
			setScreenScale("fit");
			const reset = { zoom: 1, panX: 0, panY: 0 };
			cameraRef.current = reset;
			pendingCameraRef.current = reset;
			setCamera(reset);
			activeTouches.current.clear();
			pinchGesture.current.active = false;
			window.cancelAnimationFrame(pinchSuppressionAnimationFrame.current);
			pinchSuppressionAnimationFrame.current = 0;
			pinchGesture.current.suppress = false;
			viewport.scrollLeft = 0;
			viewport.scrollTop = 0;
		};
		const update = () => {
			const measured = readVisibleViewport();
			const layoutLandscape = readLayoutLandscape();
			applyOrientationState(measured, false, layoutLandscape);
			// Keep CSS orientation tied to the physical device while the IME merely
			// shortens the visible rectangle. Otherwise the phone receives landscape
			// controls in the middle of typing even though it is still upright.
			const visible = remoteViewportWithStableOrientation(measured, lastLandscape, keyboardOpenRef.current, layoutLandscape);
			setRemoteViewport((current) => remoteViewportChanged(current, visible) || current.landscape !== visible.landscape ? visible : current);
			const next = { width: viewport.clientWidth, height: viewport.clientHeight };
			if (shouldRebaseRemotePointerViewport(lastPointerViewport, measured, lastPointerSurface, next, keyboardOpenRef.current)) {
				pointerViewportRevision.current += 1;
			}
			lastPointerViewport = measured;
			lastPointerSurface = next;
			setViewportSize((current) => {
				// Android resizes the WebView while its IME is visible. Keep the
				// remote canvas geometry stable during that temporary resize so opening
				// the keyboard never zooms or recentres the remote desktop.
				if (keyboardOpenRef.current && current.width > 0 && Math.abs(next.width-current.width) < 28 && next.height < current.height) return current;
				return current.width === next.width && current.height === next.height ? current : next;
			});
		};
		const updateAfterViewportSettles = () => {
			update();
			// Android WebView changes its visual viewport in two phases while rotating:
			// first the orientation flips, then system bars/insets receive their final
			// size. Measuring both phases prevents a stale portrait-sized fit in
			// landscape (and the inverse when returning to portrait).
			for (const delay of REMOTE_VIEWPORT_SETTLE_DELAYS) {
				const id = window.setTimeout(() => { delayedUpdates.delete(id); update(); }, delay);
				delayedUpdates.add(id);
			}
		};
		const updateAfterOrientationChange = () => {
			// A real orientation change is the only viewport transition that should
			// close the IME and return the compact dock. visualViewport also resizes
			// when the keyboard or browser chrome opens; treating that as rotation
			// would immediately close the keyboard again on Android and Safari.
			applyOrientationState(readVisibleViewport(), true);
			updateAfterViewportSettles();
		};
		if (compactRemoteClient && initialVisible.landscape) applyOrientationState(initialVisible, true);
		update();
		const observer = new ResizeObserver(update);
		observer.observe(viewport);
		window.addEventListener("resize", updateAfterViewportSettles);
		window.addEventListener("orientationchange", updateAfterOrientationChange);
		window.addEventListener("pageshow", updateAfterViewportSettles);
		window.visualViewport?.addEventListener("resize", updateAfterViewportSettles);
		window.visualViewport?.addEventListener("scroll", update);
		return () => {
			observer.disconnect();
			window.removeEventListener("resize", updateAfterViewportSettles);
			window.removeEventListener("orientationchange", updateAfterOrientationChange);
			window.removeEventListener("pageshow", updateAfterViewportSettles);
			window.visualViewport?.removeEventListener("resize", updateAfterViewportSettles);
			window.visualViewport?.removeEventListener("scroll", update);
			for (const id of delayedUpdates) window.clearTimeout(id);
		};
	// Frame URLs can change 15-60 times per second. Re-subscribing the
	// ResizeObserver and every viewport/orientation listener for each decoded
	// JPEG caused Android WebViews to lose gesture state and intermittently
	// re-apply stale viewport measurements. The listener lifecycle depends only
	// on the compact controller mode; decoded frame geometry is tracked by the
	// image callbacks and refs independently.
	}, [compactRemoteClient]);

	useEffect(() => {
		const doc = document as Document & { webkitFullscreenElement?: Element | null };
		const update = () => {
			const active = Boolean(document.fullscreenElement || doc.webkitFullscreenElement);
			setFullscreenActive(active);
			if (!active && scaleBeforeFullscreenRef.current) {
				setScreenScale(scaleBeforeFullscreenRef.current);
				scaleBeforeFullscreenRef.current = null;
			}
		};
		document.addEventListener("fullscreenchange", update);
		document.addEventListener("webkitfullscreenchange", update as EventListener);
		return () => {
			document.removeEventListener("fullscreenchange", update);
			document.removeEventListener("webkitfullscreenchange", update as EventListener);
		};
	}, []);

	useEffect(() => {
		const bridge = (window as unknown as { RemoteItAndroid?: { setRemoteSessionActive: (active: boolean) => void } }).RemoteItAndroid;
		bridge?.setRemoteSessionActive(true);
		// The remote workspace is fixed to the visual viewport. Lock the page
		// behind it too; a live document scrollbar after rotation subtracts from
		// the canvas width and offsets remote pointer coordinates on Android.
		const previousHTMLOverflow = document.documentElement.style.overflow;
		const previousBodyOverflow = document.body.style.overflow;
		const previousBodyOverscroll = document.body.style.overscrollBehavior;
		document.documentElement.style.overflow = "hidden";
		document.body.style.overflow = "hidden";
		document.body.style.overscrollBehavior = "none";
		return () => {
			bridge?.setRemoteSessionActive(false);
			document.documentElement.style.overflow = previousHTMLOverflow;
			document.body.style.overflow = previousBodyOverflow;
			document.body.style.overscrollBehavior = previousBodyOverscroll;
		};
	}, []);

	useEffect(() => {
		cameraRef.current = { zoom: 1, panX: 0, panY: 0 };
		pendingCameraRef.current = cameraRef.current;
		setCamera(cameraRef.current);
		activeTouches.current.clear();
		pinchGesture.current.active = false;
		window.cancelAnimationFrame(pinchSuppressionAnimationFrame.current);
		pinchSuppressionAnimationFrame.current = 0;
		pinchGesture.current.suppress = false;
	}, [screenScale, viewportSize.width > viewportSize.height]);

	useEffect(() => {
		if (!status?.frameWidth || !status?.frameHeight || trackpadCursor.current.ready) return;
		trackpadCursor.current = { x: Math.round(status.frameWidth / 2), y: Math.round(status.frameHeight / 2), frameWidth: status.frameWidth, frameHeight: status.frameHeight, ready: true };
		positionLocalCursor(trackpadCursor.current, { width: status.frameWidth, height: status.frameHeight });
	}, [status?.frameWidth, status?.frameHeight]);

	useLayoutEffect(() => {
		if (!mobileTrackpadMode) return;
		const image = frameImageRef.current;
		if (!image) return;
		const renderedRect = image.getBoundingClientRect();
		// Layout effects run before paint. ensureTrackpadCursor writes the compositor
		// transform synchronously, so scheduling a second requestAnimationFrame here
		// only exposed the old cursor position for one paint after a 1080p/2K/4K
		// profile change or phone rotation.
		ensureTrackpadCursor(image, renderedRect);
	// Do not depend on frameURL: assigning a new image URL briefly clears the
	// intrinsic dimensions in mobile browsers. onLoad updates renderedFrameSize
	// after decode and is the safe point at which to reproject the cursor.
	}, [mobileTrackpadMode, renderedFrameSize.width, renderedFrameSize.height]);

	useEffect(() => {
		if (status?.controlEnabled) controlEnabledRef.current = true;
	}, [status?.controlEnabled]);

  useEffect(() => {
    let disposed = false;
		controlEnabledRef.current = false;
		controlActivationRef.current = null;
		const initialTargetFPS = targetFPS === "auto" ? 0 : Number(targetFPS);
    const request = initialSessionId
			? api<{ id: string }>(`/api/desktop-sessions/${initialSessionId}`, { method: "PATCH", body: JSON.stringify({ targetFps: initialTargetFPS, cursorVisible: encodedRemoteCursorVisible }) }, csrf)
			: api<{ id: string }>(`/api/devices/${device.id}/desktop-sessions`, { method: "POST", body: JSON.stringify({ controlEnabled: false, targetFps: initialTargetFPS, cursorVisible: encodedRemoteCursorVisible }) }, csrf);
    void request
      .then((created) => { if (!disposed) setSessionId(created.id); })
      .catch((reason) => { if (!disposed) setError(reason instanceof Error ? reason.message : "Не удалось создать сеанс"); })
      .finally(() => { if (!disposed) setStarting(false); });
    return () => { disposed = true; };
  }, [device.id, csrf, initialSessionId]);

	useEffect(() => {
		if (!sessionId) return;
		// The encoded remote pointer is useful only for a coarse-pointer mobile
		// client acting as a trackpad. Desktop already has its local low-latency
		// cursor, while direct touch intentionally targets the point under a finger.
		void fetch(`/api/desktop-sessions/${sessionId}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ cursorVisible: encodedRemoteCursorVisible }) });
	}, [csrf, sessionId]);

  useEffect(() => {
    if (!sessionId) return;
		activeInputSessionID.current = sessionId;
    let disposed = false;
    let currentURL = "";
    let lastFrameAt = "";
    let statusTimer = 0;
    let frameTimer = 0;
		const frameSockets: Array<WebSocket | null> = Array(6).fill(null);
		const frameReconnectTimers = Array(6).fill(0) as number[];
		const frameReconnectAttempts = Array(6).fill(0) as number[];
		let inputSocket: WebSocket | null = null;
		let inputReconnectTimer = 0;
		let inputReconnectAttempt = 0;
		let inputAckWatchdogTimer = 0;
		const frameLaneClosed = [true, true, true, true, true, true];
		let fallbackActive = false;
		let fallbackGeneration = 0;
		let consecutiveStatusFailures = 0;
		let lastFrameStatsAt = 0;
		let lastFrameReceivedAt = performance.now();
		let streamWatchdogTimer = 0;
		let lastPresentedSequence = -1;
		let pendingPresentation: { frame: Blob; order: number } | null = null;
		let presentationRunning = false;
		let presentationOrder = 0;
		let lastDecodedPresentationOrder = 0;
		let frameArrivalHead = 0;
		const decoderImage = new Image();
		decoderImage.decoding = "async";
		const retiredFrames = createRemoteFrameRetirementScheduler({
			schedulePaint: (callback) => window.requestAnimationFrame(callback),
			cancelPaint: (id) => window.cancelAnimationFrame(id),
			revoke: (url) => URL.revokeObjectURL(url),
			paintCount: 2,
		});
		frameArrivalTimes.current = [];
		setFrameFPS(0);
    const refreshStatus = async () => {
      if (disposed) return;
      try {
			const requestStarted = performance.now();
        const nextStatus = await api<DesktopSession>(`/api/desktop-sessions/${sessionId}`);
			setLatencyMs(Math.max(1, Math.round(performance.now()-requestStarted)));
        if (disposed) return;
			consecutiveStatusFailures = 0;
			setStatus(nextStatus);
			setError(nextStatus.agentError || "");
			const acknowledgement = nextStatus.inputAck;
			if (acknowledgement?.type === "sas" && acknowledgement.id === sasPendingInputID.current) {
				sasPendingInputID.current = 0;
				window.clearTimeout(sasFeedbackTimer.current);
				if (acknowledgement.error) {
					setSASFeedbackError(true);
					setSASFeedback(`Ctrl+Alt+Delete не выполнен: ${acknowledgement.error}`);
					setError(acknowledgement.error);
				} else {
					setSASFeedbackError(false);
					setSASFeedback("Ctrl+Alt+Delete выполнен системной службой Windows");
					sasFeedbackTimer.current = window.setTimeout(() => setSASFeedback(""), 4_500);
				}
			}
			const clipboardAcknowledgement = nextStatus.clipboardAck;
			if (clipboardSyncEnabledRef.current && clipboardAcknowledgement?.type === "clipboard_read" && clipboardAcknowledgement.id > clipboardLastRemoteAckRef.current) {
				clipboardLastRemoteAckRef.current = clipboardAcknowledgement.id;
				if (clipboardAcknowledgement.error) {
					const pending = clipboardPendingWriteRef.current;
					if (pending) {
						window.clearTimeout(pending.timer);
						pending.reject(new Error(clipboardAcknowledgement.error));
						clipboardPendingWriteRef.current = null;
					}
				} else if (clipboardAcknowledgement.mime !== "image/png") {
					const remoteText = clipboardAcknowledgement.value || "";
					const pending = clipboardPendingWriteRef.current;
					if (pending) {
						window.clearTimeout(pending.timer);
						pending.resolve(new Blob([remoteText], { type: "text/plain" }));
						clipboardPendingWriteRef.current = null;
					}
					if (remoteText === clipboardLastLocalRef.current) return;
					clipboardPendingRemoteTextRef.current = remoteText;
					const writeText = navigator.clipboard?.writeText?.bind(navigator.clipboard);
					if (!writeText) return;
					void writeText(remoteText).then(() => {
						clipboardLastLocalRef.current = remoteText;
						clipboardPendingRemoteTextRef.current = null;
						clipboardPermissionErrorRef.current = false;
					}).catch(() => undefined);
				}
			}
      } catch (reason) {
				if (!disposed) {
					consecutiveStatusFailures += 1;
					setError(isRecoverableRemoteStatusFailure(consecutiveStatusFailures)
						? "Связь нестабильна — RemoteIt переподключается автоматически…"
						: (reason instanceof Error ? reason.message : "Удалённый сеанс временно недоступен"));
				}
      } finally {
        if (!disposed) statusTimer = window.setTimeout(() => void refreshStatus(), 900);
      }
    };
		const drainPresentationQueue = async () => {
			if (presentationRunning || disposed) return;
			presentationRunning = true;
			try {
				while (!disposed && pendingPresentation) {
					const candidate = pendingPresentation;
					pendingPresentation = null;
					const nextURL = URL.createObjectURL(candidate.frame);
					const loaded = new Promise<void>((resolve, reject) => {
						decoderImage.onload = () => resolve();
						decoderImage.onerror = () => reject(new Error("JPEG decode failed"));
					});
					decoderImage.src = nextURL;
					try {
						await decoderImage.decode();
					} catch {
						try { await loaded; } catch { URL.revokeObjectURL(nextURL); continue; }
					}
					if (disposed) { URL.revokeObjectURL(nextURL); break; }
					// JPEG decode is deliberately single-flight. Never compare this decoded
					// candidate with the latest *arrival*: on a sustained 60 FPS stream that
					// can starve presentation indefinitely. Show each completed decode, then
					// immediately consume the newest item from the one-slot pending queue.
					if (!shouldPresentDecodedRemoteFrame(candidate.order, lastDecodedPresentationOrder)) {
						URL.revokeObjectURL(nextURL);
						continue;
					}
					lastDecodedPresentationOrder = candidate.order;
					const previousURL = currentURL;
					currentURL = nextURL;
					if (frameImageRef.current) {
						// Paint the already-decoded JPEG as the layer backdrop before swapping
						// the foreground element. Android Chromium/WebView can otherwise expose
						// one empty GPU tile while a transformed <img> changes its blob source.
						if (frameImageLayerRef.current) frameImageLayerRef.current.style.backgroundImage = `url("${nextURL}")`;
						frameImageRef.current.src = nextURL;
					}
					else setFrameURL(nextURL);
					if (previousURL) retiredFrames.retire(previousURL);
					const now = performance.now();
					const arrivals = frameArrivalTimes.current;
					arrivals.push(now);
					while (frameArrivalHead < arrivals.length && now - arrivals[frameArrivalHead] >= 1_000) frameArrivalHead += 1;
					// Keep the hot path allocation-free while bounding storage for sessions
					// that remain open for days. Splicing only once every few seconds is much
					// cheaper than filter+spread for every decoded 30/60 FPS JPEG.
					if (frameArrivalHead >= 128) {
						arrivals.splice(0, frameArrivalHead);
						frameArrivalHead = 0;
					}
					if (now - lastFrameStatsAt >= 250) {
						lastFrameStatsAt = now;
						setFrameFPS(arrivals.length - frameArrivalHead);
					}
				}
			} finally {
				presentationRunning = false;
				if (!disposed && pendingPresentation) void drainPresentationQueue();
			}
		};
    const presentFrame = (frame: Blob) => {
			if (disposed || frame.size < 100) return;
			lastFrameReceivedAt = performance.now();
			pendingPresentation = { frame, order: ++presentationOrder };
			void drainPresentationQueue();
		};
    const refreshFrame = async (generation: number) => {
			if (disposed || !isCurrentRemoteFallbackGeneration(fallbackActive, generation, fallbackGeneration)) return;
      try {
        const after = lastFrameAt ? `&after=${encodeURIComponent(lastFrameAt)}` : "";
        const response = await fetch(`/api/desktop-sessions/${sessionId}/frame?t=${Date.now()}${after}`, { credentials: "same-origin", cache: "no-store" });
        const receivedAt = response.headers.get("X-RemoteIt-Frame-At") || "";
        const frame = await desktopFrameBlob(response);
				if (frame && isCurrentRemoteFallbackGeneration(fallbackActive, generation, fallbackGeneration)) {
          lastFrameAt = receivedAt || lastFrameAt;
					presentFrame(frame);
        }
      } catch (reason) {
        if (!disposed) setError(reason instanceof Error ? reason.message : "Связь с удалённым экраном потеряна");
      } finally {
				// Poll shortly after the previous response. A 20 ms client-side gap on
				// top of HTTP latency capped a healthy 30 FPS Agent at 24-26 displayed
				// FPS and delayed every newly uploaded frame. Five milliseconds keeps
				// the browser close to the producer cadence without parallel requests.
				if (!disposed && isCurrentRemoteFallbackGeneration(fallbackActive, generation, fallbackGeneration)) {
					frameTimer = window.setTimeout(() => void refreshFrame(generation), 5);
				}
      }
    };
		const startFrameFallback = () => {
			if (disposed || fallbackActive) return;
			fallbackActive = true;
			const generation = ++fallbackGeneration;
			void refreshFrame(generation);
		};
		const stopFrameFallback = () => {
			fallbackActive = false;
			fallbackGeneration += 1;
			window.clearTimeout(frameTimer);
			frameTimer = 0;
		};
		const decodeViewerBuffer = (buffer: ArrayBuffer) => {
			if (disposed) return;
			const bytes = new Uint8Array(buffer);
			const isSequenced = bytes.length >= 12 && bytes[0] === 0x52 && bytes[1] === 0x54 && bytes[2] === 0x56 && bytes[3] === 0x31;
			if (!isSequenced) {
				presentFrame(new Blob([bytes], { type: "image/jpeg" }));
				return;
			}
			const view = new DataView(buffer);
			const sequence = view.getUint32(4, false) * 0x1_0000_0000 + view.getUint32(8, false);
			if (sequence <= lastPresentedSequence) return;
			lastPresentedSequence = sequence;
			// A Uint8Array view avoids ArrayBuffer.slice copying every 200-300 KiB
			// JPEG before the browser decoder sees it. This matters at 60 FPS and
			// also reduces WebView garbage-collection hitches on Android.
			presentFrame(new Blob([bytes.subarray(12)], { type: "image/jpeg" }));
		};
		const presentViewerPayload = (payload: unknown, isCurrent: () => boolean, onAccepted: () => void) => {
			const accept = (buffer: ArrayBuffer) => {
				// Safari can still expose a Blob even after binaryType was switched to
				// arraybuffer. Its asynchronous conversion may finish after Android has
				// resumed and replaced this lane. Never let that stale payload stop the
				// working HTTP fallback or overwrite a frame from the current socket.
				if (disposed || !isCurrent()) return;
				onAccepted();
				decodeViewerBuffer(buffer);
			};
			if (payload instanceof ArrayBuffer) {
				accept(payload);
				return;
			}
			if (payload instanceof Blob) void payload.arrayBuffer().then(accept).catch(() => undefined);
		};
		const streamProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
		const scheduleFrameReconnect = (lane: number) => {
			if (disposed || frameReconnectTimers[lane]) return;
			const delay = remoteReconnectDelay(frameReconnectAttempts[lane]++);
			frameReconnectTimers[lane] = window.setTimeout(() => {
				frameReconnectTimers[lane] = 0;
				openFrameLane(lane);
			}, delay);
		};
		const laneClosed = (lane: number, socket: WebSocket) => {
			if (frameSockets[lane] !== socket) return;
			frameSockets[lane] = null;
			frameLaneClosed[lane] = true;
			if (disposed) return;
			if (shouldUseRemoteFrameFallback(frameLaneClosed)) startFrameFallback();
			scheduleFrameReconnect(lane);
		};
		function openFrameLane(lane: number) {
			if (disposed) return;
			const socket = new WebSocket(`${streamProtocol}//${window.location.host}/api/desktop-sessions/${sessionId}/stream?lane=${lane}`);
			frameSockets[lane] = socket;
			socket.binaryType = "arraybuffer";
			let connectTimeout = window.setTimeout(() => {
				if (!disposed && socket.readyState !== WebSocket.OPEN) socket.close();
			}, 2_500);
			socket.onopen = () => {
				window.clearTimeout(connectTimeout);
				connectTimeout = 0;
				// A browser can deliver an OPEN event from a socket that was replaced
				// after Android resumed or the network changed. Never let that stale
				// lane disable fallback or accept frames for the new generation.
				if (frameSockets[lane] !== socket) {
					socket.close();
					return;
				}
				frameReconnectAttempts[lane] = 0;
				frameLaneClosed[lane] = false;
				setError((value) => value.startsWith("Связь нестабильна") ? "" : value);
			};
			socket.onmessage = (event) => {
				// OPEN only proves that the WebSocket handshake completed. After a
				// Wi-Fi/VPN route change Android can expose a newly-open socket before
				// the Agent has resumed publishing frames. Keep the working HTTP
				// long-poll alive until the current lane actually delivers video, and
				// reject delayed messages from a socket superseded during resume.
				if (frameSockets[lane] !== socket) return;
				presentViewerPayload(
					event.data,
					() => frameSockets[lane] === socket,
					stopFrameFallback,
				);
			};
			socket.onerror = () => socket.close();
			socket.onclose = () => {
				window.clearTimeout(connectTimeout);
				laneClosed(lane, socket);
			};
		}
		[0, 1, 2, 3, 4, 5].forEach(openFrameLane);
		// A dedicated low-bandwidth socket keeps keyboard and pointer packets out
		// of both JPEG TCP flows. If it closes, the existing ordered HTTP fallback
		// takes over without interrupting the video lanes.
		const scheduleInputReconnect = () => {
			if (disposed || inputReconnectTimer) return;
			inputReconnectTimer = window.setTimeout(() => {
				inputReconnectTimer = 0;
				openInputSocket();
			}, remoteReconnectDelay(inputReconnectAttempt++));
		};
		const restorePendingInput = (predicate: (batch: PendingRemoteInputBatch<WebSocket>) => boolean) => {
			const retry = takePendingRemoteInputBatches(inputPendingBatches.current, predicate);
			if (!retry.length || disposed) return;
			inputQueue.current = restoreRemoteInputBatch(retry, inputQueue.current);
			window.clearTimeout(inputFlushTimer.current);
			inputFlushTimer.current = window.setTimeout(() => flushInputQueueRef.current(), 0);
		};
		const watchInputAcknowledgements = () => {
			inputAckWatchdogTimer = 0;
			if (disposed) return;
			const now = performance.now();
			restorePendingInput((batch) => batch.expiresAt <= now);
			inputAckWatchdogTimer = window.setTimeout(watchInputAcknowledgements, 250);
		};
		const ensureInputAcknowledgementWatchdog = () => {
			if (!disposed && !inputAckWatchdogTimer) {
				inputAckWatchdogTimer = window.setTimeout(watchInputAcknowledgements, 250);
			}
		};
		function openInputSocket() {
			if (disposed) return;
			const socket = new WebSocket(`${streamProtocol}//${window.location.host}/api/desktop-sessions/${sessionId}/stream?lane=input`);
			inputSocket = socket;
			let connectTimeout = window.setTimeout(() => {
				if (!disposed && socket.readyState !== WebSocket.OPEN) socket.close();
			}, 2_500);
			socket.onopen = () => {
				window.clearTimeout(connectTimeout);
				connectTimeout = 0;
				if (inputSocket !== socket) {
					socket.close();
					return;
				}
				inputReconnectAttempt = 0;
				frameSocketRef.current = socket;
				if (inputQueue.current.length) flushInputQueueRef.current();
			};
			socket.onmessage = (event) => {
				const batchID = remoteInputAckID(event.data);
				if (!batchID) return;
				const batch = inputPendingBatches.current.get(batchID);
				if (batch?.socket === socket) inputPendingBatches.current.delete(batchID);
			};
			socket.onerror = () => socket.close();
			socket.onclose = () => {
				window.clearTimeout(connectTimeout);
				const wasCurrent = inputSocket === socket;
				if (wasCurrent) {
					inputSocket = null;
					if (frameSocketRef.current === socket) frameSocketRef.current = null;
				}
				restorePendingInput((batch) => batch.socket === socket);
				// Closing a superseded socket must not arm another reconnect timer;
				// otherwise an online/visibility wake-up can create two input lanes and
				// make pointer packets alternate between them.
				if (!wasCurrent) return;
				if (!disposed) scheduleInputReconnect();
			};
		}
		openInputSocket();
		ensureInputAcknowledgementWatchdog();
		const reconnectRemoteSocketsNow = (force = false) => {
			if (disposed) return;
			frameSockets.forEach((socket, lane) => {
				window.clearTimeout(frameReconnectTimers[lane]);
				frameReconnectTimers[lane] = 0;
				frameReconnectAttempts[lane] = 0;
				if (!force && socket?.readyState === WebSocket.OPEN) return;
				// Detach before close so the old onclose handler cannot schedule a
				// competing lane. Reopening immediately avoids waiting up to eight
				// seconds after Wi-Fi/VPN recovery or Android browser resume.
				frameSockets[lane] = null;
				frameLaneClosed[lane] = true;
				socket?.close();
				openFrameLane(lane);
			});
			window.clearTimeout(inputReconnectTimer);
			window.clearTimeout(inputAckWatchdogTimer);
			inputAckWatchdogTimer = 0;
			inputReconnectTimer = 0;
			inputReconnectAttempt = 0;
			if (force || inputSocket?.readyState !== WebSocket.OPEN) {
				const previous = inputSocket;
				inputSocket = null;
				if (frameSocketRef.current === previous) frameSocketRef.current = null;
				previous?.close();
				openInputSocket();
			}
			// An online/pageshow wake-up deliberately replaces every socket. It must
			// also re-arm acknowledgement expiry; otherwise the first network roam
			// permanently disables retry for later pointer and keyboard batches.
			ensureInputAcknowledgementWatchdog();
		};
		const forceRemoteSocketRefresh = () => reconnectRemoteSocketsNow(true);
		const resumeRemoteSockets = () => {
			if (document.visibilityState !== "hidden") reconnectRemoteSocketsNow(false);
		};
		const watchRemoteStreams = () => {
			if (disposed) return;
			if (document.visibilityState !== "hidden" && isRemoteFrameStreamStalled(lastFrameReceivedAt, performance.now())) {
				// Start the already authenticated HTTP path immediately, then replace
				// every possibly-zombie WebSocket. A successful frame through either
				// transport resets the watchdog and prevents reconnect churn.
				startFrameFallback();
				forceRemoteSocketRefresh();
				lastFrameReceivedAt = performance.now();
			}
			streamWatchdogTimer = window.setTimeout(watchRemoteStreams, 1_000);
		};
		streamWatchdogTimer = window.setTimeout(watchRemoteStreams, 1_000);
		window.addEventListener("online", forceRemoteSocketRefresh);
		window.addEventListener("pageshow", forceRemoteSocketRefresh);
		document.addEventListener("visibilitychange", resumeRemoteSockets);
    void refreshStatus();
    return () => {
      disposed = true;
		if (activeInputSessionID.current === sessionId) activeInputSessionID.current = "";
			window.removeEventListener("online", forceRemoteSocketRefresh);
			window.removeEventListener("pageshow", forceRemoteSocketRefresh);
			document.removeEventListener("visibilitychange", resumeRemoteSockets);
			pendingPresentation = null;
			frameSockets.forEach((socket) => socket?.close());
			frameReconnectTimers.forEach((timer) => window.clearTimeout(timer));
			inputSocket?.close();
			window.clearTimeout(inputReconnectTimer);
			window.clearTimeout(inputAckWatchdogTimer);
			inputAckWatchdogTimer = 0;
			window.clearTimeout(streamWatchdogTimer);
			if (frameSocketRef.current === inputSocket) frameSocketRef.current = null;
			window.clearTimeout(statusTimer);
			window.clearTimeout(frameTimer);
			window.clearTimeout(inputFlushTimer.current);
			inputAbortController.current?.abort();
			inputAbortController.current = null;
			inputQueue.current = [];
			inputPendingBatches.current.clear();
			inputInFlight.current = false;
			retiredFrames.dispose();
      if (currentURL) URL.revokeObjectURL(currentURL);
			if (explicitSessionFinishRef.current) {
				void fetch(`/api/desktop-sessions/${sessionId}`, { method: "DELETE", credentials: "same-origin", headers: { "X-CSRF-Token": csrf } });
			} else {
				// A route change, reload, mobile orientation rebuild or transient
				// browser disconnect releases control but keeps the short resumable
				// viewer lease. The server expires it automatically after inactivity.
				void fetch(`/api/desktop-sessions/${sessionId}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ controlEnabled: false }) });
			}
			controlEnabledRef.current = false;
			controlActivationRef.current = null;
    };
  }, [sessionId, csrf, initialSessionId]);

	flushInputQueueRef.current = () => {
		if (!sessionId || inputInFlight.current || !controlEnabledRef.current || inputQueue.current.length === 0) return;
		const deliverySessionID = sessionId;
		const events = inputQueue.current.splice(0, 32);
		const body = events.length === 1 ? events[0] : { events };
		const socket = frameSocketRef.current;
		if (socket?.readyState === WebSocket.OPEN && socket.bufferedAmount < 64 << 10) {
			const batchID = remoteInputBatchID(inputClientPrefix.current, ++inputBatchSequence.current);
			try {
				inputPendingBatches.current.set(batchID, { events, socket, expiresAt: performance.now() + 2_500 });
				socket.send(JSON.stringify({ batchId: batchID, events }));
				if (inputQueue.current.length) inputFlushTimer.current = window.setTimeout(() => flushInputQueueRef.current(), 0);
				return;
			} catch {
				inputPendingBatches.current.delete(batchID);
				// The compatibility HTTP path below preserves ordered input if a proxy
				// closes the duplex channel between the ready-state check and send().
			}
		}
		inputInFlight.current = true;
		const controller = new AbortController();
		inputAbortController.current = controller;
		const timeout = window.setTimeout(() => controller.abort(), 2_500);
		let delivered = false;
		void fetch(`/api/desktop-sessions/${sessionId}/input`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify(body), signal: controller.signal })
			.then((response) => {
				if (!response.ok) throw new Error(`HTTP ${response.status}`);
				delivered = true;
			})
			.catch(() => {
				// Keep the exact client IDs. The server remembers accepted IDs even after
				// Agent delivery, so a response-loss retry cannot type text or click twice.
				if (shouldRetryRemoteInputDelivery(activeInputSessionID.current, deliverySessionID)) {
					inputQueue.current = restoreRemoteInputBatch(events, inputQueue.current);
					setError("Команда управления не подтверждена — автоматически повторяем…");
				}
			})
			.finally(() => {
				window.clearTimeout(timeout);
				if (inputAbortController.current === controller) {
					inputAbortController.current = null;
					inputInFlight.current = false;
					if (inputQueue.current.length) inputFlushTimer.current = window.setTimeout(() => flushInputQueueRef.current(), delivered ? 0 : 120);
				}
			});
	};

	const activateRemoteControl = useCallback((): Promise<void> => {
		if (!sessionId) return Promise.reject(new Error("Удалённый сеанс ещё не готов"));
		if (controlEnabledRef.current) return Promise.resolve();
		if (!controlActivationRef.current) {
			controlActivationRef.current = fetch(`/api/desktop-sessions/${sessionId}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ controlEnabled: true }) })
				.then((response) => {
					if (!response.ok) throw new Error(`Не удалось включить управление: HTTP ${response.status}`);
					controlEnabledRef.current = true;
					setStatus((current) => current ? { ...current, controlEnabled: true } : current);
				})
				.finally(() => { controlActivationRef.current = null; });
		}
		return controlActivationRef.current!;
	}, [sessionId, csrf]);

	const sendInput = useCallback((event: Record<string, unknown>, activatesControl = true) => {
		if (!sessionId) return;
		// Bind every pointer packet to the exact JPEG coordinate space used by the
		// viewer when the event was produced. The Agent may switch capture profiles
		// before this packet arrives; without this basis, 1920px input interpreted
		// against a later 1600px frame moves the Windows pointer to the wrong place.
		let queuedEvent: Record<string, unknown> = {
			...event,
			clientInputId: remoteInputClientID(inputClientPrefix.current, ++inputClientSequence.current),
		};
		if (event.type === "pointer") {
			let coordinateWidth = Number(event.coordinateWidth);
			let coordinateHeight = Number(event.coordinateHeight);
			// Every current pointer producer already binds its sample to the decoded
			// frame. Re-reading getBoundingClientRect() here forced a second layout on
			// every high-frequency movement and could observe the next JPEG's geometry,
			// making a correct coordinate jump. Keep the fallback only for legacy or
			// synthetic packets without an explicit basis.
			if (!(coordinateWidth > 0 && coordinateHeight > 0)) {
				const image = frameImageRef.current;
				const size = image ? frameSize(image) : {
					width: Math.max(1, status?.frameWidth || 1),
					height: Math.max(1, status?.frameHeight || 1),
				};
				coordinateWidth = size.width;
				coordinateHeight = size.height;
			}
			queuedEvent = bindRemoteInputCoordinates(queuedEvent, coordinateWidth, coordinateHeight);
		}
		const enqueueOrdered = () => {
			const last = inputQueue.current.at(-1);
			if (queuedEvent.type === "pointer" && queuedEvent.action === "move" && last?.type === "pointer" && last?.action === "move") inputQueue.current[inputQueue.current.length - 1] = queuedEvent;
			else inputQueue.current.push(queuedEvent);
			window.clearTimeout(inputFlushTimer.current);
			inputFlushTimer.current = 0;
			// The first control packet is latency-sensitive and already runs on its own
			// low-bandwidth WebSocket. Deferring it to a zero-delay timer allowed a JPEG
			// decode/layout task to run first, adding a full frame of input lag on mobile.
			// Flush synchronously; the existing 32-event batching, websocket pressure
			// guard and ordered HTTP fallback still bound and preserve the queue.
			flushInputQueueRef.current();
		};
		if (controlEnabledRef.current) { enqueueOrdered(); return; }
		if (!activatesControl) return;
		void activateRemoteControl().then(enqueueOrdered).catch((reason) => setError(reason instanceof Error ? reason.message : "Не удалось включить управление — проверьте соединение."));
		return;
	}, [sessionId, activateRemoteControl, status?.frameWidth, status?.frameHeight]);

	useEffect(() => {
		if (!targetWindows || !sessionId) return;
		const primed = clipboardInitialSnapshotRef.current;
		clipboardInitialSnapshotRef.current = null;
		if (!primed) return;
		let disposed = false;
		void primed.then(async (snapshot) => {
			if (disposed) return;
			await activateRemoteControl();
			if (!disposed) await pushLocalClipboardSnapshot(snapshot);
		}).catch(() => {
			// Safari may decline the initial read. Cmd+V and the common-buffer button
			// retain trusted user-gesture paths, so this is not a session error.
			clipboardBackgroundReadBlockedRef.current = true;
		});
		return () => { disposed = true; };
	}, [targetWindows, sessionId, activateRemoteControl]);

	useEffect(() => {
		clipboardSyncEnabledRef.current = clipboardSyncEnabled;
		if (!clipboardSyncEnabled || !sessionId) return;
		let disposed = false;
		const poll = async () => {
			if (disposed || document.visibilityState === "hidden" || clipboardPollBusyRef.current) return;
			clipboardPollBusyRef.current = true;
			try {
				// Chromium can read a previously granted clipboard in the background.
				// WebKit deliberately cannot: remember that result and keep polling only
				// the remote side instead of showing a permission prompt every 700 ms.
				if (!clipboardBackgroundReadBlockedRef.current) {
					try {
						await pushLocalClipboardSnapshot(await readLocalClipboardSnapshot());
					} catch {
						clipboardBackgroundReadBlockedRef.current = true;
					}
				}
				const remoteResponse = await fetch(`/api/desktop-sessions/${sessionId}/clipboard-image?after=${clipboardLastRemoteImageSequenceRef.current}`, { credentials: "same-origin", cache: "no-store" });
				if (remoteResponse.status === 200) {
					const sequence = Number(remoteResponse.headers.get("X-RemoteIt-Clipboard-Sequence") || "0");
					const remoteImage = await remoteResponse.blob();
					if (sequence > clipboardLastRemoteImageSequenceRef.current && remoteImage.type === "image/png") {
						clipboardLastRemoteImageSequenceRef.current = sequence;
						try {
							await writeClipboardPNG(remoteImage);
							clipboardLastLocalImageRef.current = await clipboardBlobFingerprint(remoteImage);
							clipboardPendingRemoteImageRef.current = null;
						} catch {
							clipboardPendingRemoteImageRef.current = remoteImage;
						}
					}
				} else if (remoteResponse.status !== 204 && !remoteResponse.ok) {
					throw new Error(`Буфер изображений: HTTP ${remoteResponse.status}`);
				}
			} catch {
				// A brief clipboard/API failure must not interrupt video or control.
			} finally {
				if (!disposed) sendInput({ type: "clipboard_read" });
				clipboardPollBusyRef.current = false;
			}
		};
		void poll();
		const timer = window.setInterval(() => void poll(), 700);
		return () => {
			disposed = true;
			window.clearInterval(timer);
			clipboardPollBusyRef.current = false;
		};
	}, [clipboardSyncEnabled, sessionId, sendInput, csrf]);

	useEffect(() => () => {
		const pending = clipboardPendingWriteRef.current;
		if (pending) {
			window.clearTimeout(pending.timer);
			pending.reject(new Error("Удалённый сеанс закрыт"));
			clipboardPendingWriteRef.current = null;
		}
	}, []);

	function armPointerMoveTimer(delayMs: number) {
		if (pointerMoveTimer.current) return;
		pointerMoveTimer.current = window.setTimeout(() => {
			pointerMoveTimer.current = 0;
			const cadence = pointerMoveCadence.current;
			const pending = cadence.take(performance.now());
			if (pending) {
				sendInput(pending.event, pending.activatesControl);
				return;
			}
			const remaining = cadence.remaining(performance.now());
			if (remaining > 0) armPointerMoveTimer(remaining);
		}, Math.max(1, Math.ceil(delayMs)));
	}

	function schedulePointerMove(event: Record<string, unknown>, activatesControl = true) {
		const cadence = pointerMoveCadence.current;
		cadence.setInterval(remotePointerCadenceMillis(coarsePointerClient, frameSocketRef.current?.bufferedAmount || 0));
		const result = cadence.offer({ event, activatesControl }, performance.now());
		if (result.send) {
			window.clearTimeout(pointerMoveTimer.current);
			pointerMoveTimer.current = 0;
			sendInput(result.send.event, result.send.activatesControl);
			return;
		}
		armPointerMoveTimer(result.delayMs);
	}

	function flushPendingPointerMove() {
		window.clearTimeout(pointerMoveTimer.current);
		pointerMoveTimer.current = 0;
		const pending = pointerMoveCadence.current.take(performance.now(), true);
		if (pending) sendInput(pending.event, pending.activatesControl);
	}

	function discardPendingPointerMove() {
		window.clearTimeout(pointerMoveTimer.current);
		pointerMoveTimer.current = 0;
		pointerMoveCadence.current.clear();
	}

	releaseActivePointerRef.current = () => {
		discardPendingPointerMove();
		clearPendingTrackpadMotion(false);
		stopWheelRepeat();
		if (directGesture.current.timer) window.clearTimeout(directGesture.current.timer);
		if (trackpadGesture.current.timer) window.clearTimeout(trackpadGesture.current.timer);
		if (directGesture.current.leftDown) {
			const gesture = directGesture.current;
			sendInput({
				type: "pointer", action: "up", button: "left",
				x: gesture.x, y: gesture.y,
				coordinateWidth: gesture.coordinateWidth,
				coordinateHeight: gesture.coordinateHeight,
			}, false);
		}
		if (trackpadGesture.current.dragging) {
			sendInput({ type: "pointer", action: "up", button: "left", ...serialisedTrackpadPosition() }, false);
		}
		if (heldMouseButton.current && lastMousePosition.current) {
			sendInput({ type: "pointer", action: "up", button: heldMouseButton.current, ...lastMousePosition.current }, false);
		}
		heldMouseButton.current = null;
		directGesture.current.pointerId = -1;
		directGesture.current.leftDown = false;
		directGesture.current.longPress = false;
		directGesture.current.suppressTap = false;
		directGesture.current.timer = 0;
		trackpadGesture.current.pointerId = -1;
		trackpadGesture.current.dragging = false;
		trackpadGesture.current.longPress = false;
		trackpadGesture.current.suppressTap = false;
		trackpadGesture.current.timer = 0;
		trackpadGesture.current.pendingX = 0;
		trackpadGesture.current.pendingY = 0;
		trackpadGesture.current.pendingTimer = 0;
		activeTouches.current.clear();
		pinchGesture.current.active = false;
		window.cancelAnimationFrame(pinchSuppressionAnimationFrame.current);
		pinchSuppressionAnimationFrame.current = 0;
		pinchGesture.current.suppress = false;
	};

	useEffect(() => () => {
		discardPendingPointerMove();
		clearPendingTrackpadMotion(false);
	}, [sessionId]);

	const releaseRemoteModifiers = useCallback(() => {
		textKeyboardKeys.current.clear();
		for (const keyCode of [16, 17, 18, 91, 92]) sendInput({ type: "key", action: "up", keyCode }, false);
	}, [sendInput]);

	useEffect(() => {
		const release = () => {
			releaseActivePointerRef.current();
			releaseRemoteModifiers();
		};
		const visibility = () => { if (document.hidden) release(); };
		window.addEventListener("blur", release);
		document.addEventListener("visibilitychange", visibility);
		return () => {
			window.removeEventListener("blur", release);
			document.removeEventListener("visibilitychange", visibility);
		};
	}, [releaseRemoteModifiers]);

	function updateTargetFPS(value: "auto" | "15" | "30" | "60") {
		releaseRemoteModifiers();
		setTargetFPS(value);
		if (!sessionId) return;
		const numeric = value === "auto" ? 0 : Number(value);
		void fetch(`/api/desktop-sessions/${sessionId}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf }, body: JSON.stringify({ targetFps: numeric }) })
			.then((response) => { if (!response.ok) throw new Error(`HTTP ${response.status}`); setStatus((current) => current ? { ...current, targetFps: numeric } : current); })
			.catch(() => setError("Не удалось изменить частоту кадров."));
	}

	function touchDistance(points: { x: number; y: number }[]) {
		return Math.hypot(points[1].x - points[0].x, points[1].y - points[0].y);
	}

	function touchMidpoint(points: { x: number; y: number }[]) {
		return { x: (points[0].x + points[1].x) / 2, y: (points[0].y + points[1].y) / 2 };
	}

	function coalescedPointerSamples(event: ReactPointerEvent<HTMLDivElement>): PointerEvent[] {
		let samples: PointerEvent[] = [];
		try { samples = event.nativeEvent.getCoalescedEvents?.() || []; } catch { samples = []; }
		const finalSample = event.nativeEvent;
		const last = samples[samples.length - 1];
		// Chromium normally includes the dispatched sample, while WebKit builds have
		// returned only the preceding coalesced points. Append the final coordinate
		// only when it is absent so neither engine loses nor double-applies motion.
		if (!last || last.clientX !== finalSample.clientX || last.clientY !== finalSample.clientY) samples.push(finalSample);
		return samples;
	}

	function clampCamera(next: { zoom: number; panX: number; panY: number }) {
		return clampRemoteCamera(next, { x: fittedFrame.width, y: fittedFrame.height }, { x: viewportSize.width, y: viewportSize.height });
	}

	function frameSize(element: HTMLImageElement, renderedRect?: { width: number; height: number }) {
		// A pointer sample must use one atomic view of the image geometry. Reading
		// getBoundingClientRect() repeatedly in the same sample forces extra layout
		// work on Android and can observe two different compositor states while the
		// camera is panning. Reuse the caller's snapshot whenever it already has one.
		const rect = renderedRect || element.getBoundingClientRect();
		const size = authoritativeRemoteFrameSize(
			{ x: element.naturalWidth, y: element.naturalHeight },
			{ x: renderedFrameSize.width, y: renderedFrameSize.height },
			{ x: status?.frameWidth || 0, y: status?.frameHeight || 0 },
			{ x: rect.width, y: rect.height },
		);
		return {
			width: size.x,
			height: size.y,
		};
	}

	function ensureTrackpadCursor(element: HTMLImageElement, renderedRect?: { width: number; height: number }) {
		const size = frameSize(element, renderedRect);
		if (!trackpadCursor.current.ready) {
			trackpadCursor.current = { x: Math.round(size.width / 2), y: Math.round(size.height / 2), frameWidth: size.width, frameHeight: size.height, ready: true };
		} else if (trackpadCursor.current.frameWidth !== size.width || trackpadCursor.current.frameHeight !== size.height) {
			const next = reprojectRemotePoint(
				trackpadCursor.current,
				{ x: trackpadCursor.current.frameWidth || size.width, y: trackpadCursor.current.frameHeight || size.height },
				{ x: size.width, y: size.height },
			);
			trackpadCursor.current = { ...next, frameWidth: size.width, frameHeight: size.height, ready: true };
		}
		positionLocalCursor(trackpadCursor.current, size);
		return size;
	}

	function serialisedTrackpadPosition() {
		const image = frameImageRef.current;
		const size = image ? ensureTrackpadCursor(image) : {
			width: Math.max(1, status?.frameWidth || 1),
			height: Math.max(1, status?.frameHeight || 1),
		};
		const position = clampRemotePoint(trackpadCursor.current, { x: size.width, y: size.height });
		return { ...position, coordinateWidth: size.width, coordinateHeight: size.height };
	}

	function positionLocalCursor(position: { x: number; y: number }, size: { width: number; height: number }) {
		const cursor = localCursorRef.current;
		if (!cursor) return;
		// Keep the cursor in the decoded frame's pixel coordinate space. Percentage
		// layout caused a synchronous style/layout pass on every touch sample and
		// rounded differently on 1366px/2256px/ultrawide frames, which looked like
		// blinking or teleporting on Android. A compositor-only transform stays
		// stable while the JPEG element is being replaced underneath it.
		const { x, y } = remoteCursorVisualPointForLayer(
			position,
			{ x: size.width, y: size.height },
			{ x: renderedFrameSize.width, y: renderedFrameSize.height },
		);
		cursor.style.setProperty("--remote-cursor-x", `${x}px`);
		cursor.style.setProperty("--remote-cursor-y", `${y}px`);
	}

	function commitTrackpadDeltas(deltas: Array<{ x: number; y: number }>) {
		if (!deltas.length) return false;
		const image = frameImageRef.current;
		if (!image) return false;
		const rect = image.getBoundingClientRect();
		const size = ensureTrackpadCursor(image, rect);
		let next = { x: trackpadCursor.current.x, y: trackpadCursor.current.y };
		for (const delta of deltas) {
			trackpadGesture.current.distance += Math.abs(delta.x) + Math.abs(delta.y);
			next = advanceRemoteTrackpadCursor(
				next,
				delta,
				{ x: size.width, y: size.height },
				{ x: rect.width, y: rect.height },
			);
		}
		if (trackpadGesture.current.distance > 10 && trackpadGesture.current.timer) {
			window.clearTimeout(trackpadGesture.current.timer);
			trackpadGesture.current.timer = 0;
		}
		trackpadCursor.current = { ...next, frameWidth: size.width, frameHeight: size.height, ready: true };
		positionLocalCursor(next, size);
		const followedCamera = cameraFollowingRemotePoint(
			pendingCameraRef.current,
			{ x: next.x, y: next.y },
			{ x: size.width, y: size.height },
			{ x: fittedFrame.width, y: fittedFrame.height },
			{ x: viewportSize.width, y: viewportSize.height },
		);
		if (followedCamera.panX !== pendingCameraRef.current.panX || followedCamera.panY !== pendingCameraRef.current.panY) scheduleCamera(followedCamera);
		// A trackpad drag is an explicit control action. Send the leading sample
		// immediately and keep one newest trailing coordinate. This preserves the
		// exact final position on high-Hz touchscreens without queueing old moves.
		schedulePointerMove({ type: "pointer", action: "move", ...clampRemotePoint(next, { x: size.width, y: size.height }), coordinateWidth: size.width, coordinateHeight: size.height });
		return true;
	}

	function clearPendingTrackpadMotion(flush: boolean) {
		const gesture = trackpadGesture.current;
		if (gesture.pendingTimer) window.clearTimeout(gesture.pendingTimer);
		gesture.pendingTimer = 0;
		const pending = gesture.pendingX !== 0 || gesture.pendingY !== 0
			? { x: gesture.pendingX, y: gesture.pendingY }
			: null;
		gesture.pendingX = 0;
		gesture.pendingY = 0;
		if (flush && pending) return commitTrackpadDeltas([pending]);
		return false;
	}

	function armPendingTrackpadMotion(pointerId: number) {
		const gesture = trackpadGesture.current;
		if (gesture.pendingTimer) window.clearTimeout(gesture.pendingTimer);
		gesture.pendingTimer = window.setTimeout(() => {
			const current = trackpadGesture.current;
			current.pendingTimer = 0;
			if (current.pointerId !== pointerId || (current.pendingX === 0 && current.pendingY === 0)) return;
			const pending = { x: current.pendingX, y: current.pendingY };
			current.pendingX = 0;
			current.pendingY = 0;
			commitTrackpadDeltas([pending]);
		// One 60 Hz paint is enough to distinguish a browser viewport rebase from
		// a real swipe. The previous 34 ms look-ahead was safe but made the first
		// movement on a phone feel one extra frame behind.
		}, 18);
	}

	function pointerPosition(event: ReactPointerEvent<HTMLDivElement>) {
		const image = frameImageRef.current;
		if (!image) {
			const coordinateWidth = Math.max(1, status?.frameWidth || 1);
			const coordinateHeight = Math.max(1, status?.frameHeight || 1);
			return { x: 0, y: 0, coordinateWidth, coordinateHeight };
		}
		const rect = image.getBoundingClientRect();
		const { width: frameWidth, height: frameHeight } = frameSize(image, rect);
		const position = remotePointFromClient(
			{ x: event.clientX, y: event.clientY },
			rect,
			{ x: frameWidth, y: frameHeight },
		);
		trackpadCursor.current = { ...position, frameWidth, frameHeight, ready: true };
		positionLocalCursor(position, { width: frameWidth, height: frameHeight });
		if (!compactRemoteClient && (screenScale === "fill" || screenScale === "actual" || pendingCameraRef.current.zoom > 1)) {
			const followedCamera = cameraFollowingRemotePoint(
				pendingCameraRef.current,
				position,
				{ x: frameWidth, y: frameHeight },
				{ x: fittedFrame.width, y: fittedFrame.height },
				{ x: viewportSize.width, y: viewportSize.height },
			);
			if (followedCamera.panX !== pendingCameraRef.current.panX || followedCamera.panY !== pendingCameraRef.current.panY) scheduleCamera(followedCamera);
		}
		return { ...position, coordinateWidth: frameWidth, coordinateHeight: frameHeight };
  }

	function beginTouchGesture(event: ReactPointerEvent<HTMLDivElement>) {
		activeTouches.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
		if (activeTouches.current.size < 2) return false;
		window.cancelAnimationFrame(pinchSuppressionAnimationFrame.current);
		pinchSuppressionAnimationFrame.current = 0;
		const points = [...activeTouches.current.values()].slice(0, 2);
		const midpoint = touchMidpoint(points);
		const distance = Math.max(1, touchDistance(points));
		pinchGesture.current = {
			active: true,
			suppress: true,
			mode: mobileTrackpadMode ? "pending" : "zoom",
			startDistance: distance,
			lastDistance: distance,
			lastMidX: midpoint.x,
			lastMidY: midpoint.y,
			wheelDistance: 0,
			midpointTravel: 0,
			startedAt: performance.now(),
			rightClickSent: false,
		};
		pinchGestureViewportRevision.current = pointerViewportRevision.current;
		if (directGesture.current.timer) window.clearTimeout(directGesture.current.timer);
		if (directGesture.current.leftDown) {
			const direct = directGesture.current;
			sendInput({ type: "pointer", action: "up", button: "left", x: direct.x, y: direct.y, coordinateWidth: direct.coordinateWidth, coordinateHeight: direct.coordinateHeight });
		}
		if (trackpadGesture.current.timer) { window.clearTimeout(trackpadGesture.current.timer); trackpadGesture.current.timer = 0; }
		clearPendingTrackpadMotion(false);
		if (trackpadGesture.current.dragging) sendInput({ type: "pointer", action: "up", button: "left", ...serialisedTrackpadPosition() });
		trackpadGesture.current.pointerId = -1;
		trackpadGesture.current.dragging = false;
		directGesture.current.pointerId = -1;
		return true;
	}

	function updateTouchGesture(event: ReactPointerEvent<HTMLDivElement>) {
		if (!activeTouches.current.has(event.pointerId)) return false;
		if (!pinchGesture.current.active || activeTouches.current.size < 2) return false;
		if (pinchGestureViewportRevision.current !== pointerViewportRevision.current) {
			// The visual viewport changed while both fingers were down. Mixing samples
			// from the old and new coordinate spaces produces a large false pinch.
			// Suppress the rest of this native gesture and resume after both fingers
			// are lifted; the already committed camera remains untouched.
			pinchGestureViewportRevision.current = pointerViewportRevision.current;
			pinchGesture.current.active = false;
			pinchGesture.current.suppress = true;
			return true;
		}
		const gesture = pinchGesture.current;
		const viewport = viewportRef.current?.getBoundingClientRect();
		// Work in viewport-local coordinates. Android changes WebView insets while
		// rotating or showing the IME; absolute page coordinates made those inset
		// changes look like a camera jump even when the fingers had not moved.
		const left = viewport?.left || 0;
		const top = viewport?.top || 0;
		const centerX = (viewport?.width || viewportSize.width) / 2;
		const centerY = (viewport?.height || viewportSize.height) / 2;
		for (const sample of coalescedPointerSamples(event)) {
			activeTouches.current.set(event.pointerId, { x: sample.clientX, y: sample.clientY });
			const points = [...activeTouches.current.values()].slice(0, 2);
			if (points.length < 2) continue;
			const midpoint = touchMidpoint(points);
			const distance = Math.max(1, touchDistance(points));
			const scaleChange = distance / Math.max(1, gesture.startDistance);
			const midpointDelta = Math.hypot(midpoint.x - gesture.lastMidX, midpoint.y - gesture.lastMidY);
			gesture.midpointTravel += midpointDelta;
			gesture.mode = classifyRemoteTouchGesture(gesture.mode, mobileTrackpadMode, scaleChange, gesture.midpointTravel);
			if (gesture.mode === "zoom") {
				// Apply every native sample to the latest pending camera, while React
				// commits only once on the next animation frame. This keeps the midpoint
				// under the fingers without re-rendering for every 120/240 Hz touch point.
				scheduleCamera(advanceRemotePinch(
					pendingCameraRef.current,
					{ x: gesture.lastMidX - left, y: gesture.lastMidY - top },
					{ x: midpoint.x - left, y: midpoint.y - top },
					{ x: centerX, y: centerY },
					gesture.lastDistance,
					distance,
				));
			} else if (gesture.mode === "scroll") {
				gesture.wheelDistance += midpoint.y - gesture.lastMidY;
			}
			gesture.lastMidX = midpoint.x;
			gesture.lastMidY = midpoint.y;
			gesture.lastDistance = distance;
		}
		// Bound transport work independently of the number of coalesced native
		// samples. Residual distance remains for the next event, so scrolling is
		// continuous but never emits an unbounded packet burst after a UI stall.
		let steps = 0;
		while (gesture.mode === "scroll" && Math.abs(gesture.wheelDistance) >= 8 && steps < 6) {
			const direction = gesture.wheelDistance > 0 ? 1 : -1;
			sendInput({ type: "wheel", delta: direction > 0 ? -120 : 120 });
			gesture.wheelDistance -= direction * 8;
			steps += 1;
		}
		return true;
	}

	function endTouchGesture(pointerId: number, cancelled = false) {
		const wasPinching = pinchGesture.current.active || pinchGesture.current.suppress;
		const gesture = pinchGesture.current;
		// Microsoft-style mouse mode: a quick two-finger tap is a right click.
		// Pinch and two-finger scrolling are classified first, so releasing those
		// gestures can never accidentally open a context menu.
		if (gesture.active && !gesture.rightClickSent && isRemoteTwoFingerTap(gesture.mode, mobileTrackpadMode, cancelled, performance.now() - gesture.startedAt, gesture.midpointTravel)) {
			gesture.rightClickSent = true;
			sendPointerTap("right");
		}
		activeTouches.current.delete(pointerId);
		if (activeTouches.current.size < 2) pinchGesture.current.active = false;
		if (canReleaseRemoteTouchSuppression(activeTouches.current.size, pinchGesture.current.active)) {
			window.cancelAnimationFrame(pinchSuppressionAnimationFrame.current);
			// React commits the camera state on the first frame and the transformed
			// image rectangle becomes observable after the following paint. Releasing
			// earlier lets a very fast post-pinch tap use the previous rectangle.
			pinchSuppressionAnimationFrame.current = window.requestAnimationFrame(() => {
				pinchSuppressionAnimationFrame.current = window.requestAnimationFrame(() => {
					pinchSuppressionAnimationFrame.current = 0;
					if (canReleaseRemoteTouchSuppression(activeTouches.current.size, pinchGesture.current.active)) pinchGesture.current.suppress = false;
				});
			});
		}
		return wasPinching;
	}

	function movePointer(event: ReactPointerEvent<HTMLDivElement>) {
	event.preventDefault();
		if (event.pointerType !== "mouse") {
			if (updateTouchGesture(event)) return;
			if (pinchGesture.current.suppress) return;
		}
		if (mobileTrackpadMode) {
			if (trackpadGesture.current.pointerId !== event.pointerId) return;
			if (trackpadGestureViewportRevision.current !== pointerViewportRevision.current) {
				const samples = coalescedPointerSamples(event);
				const latest = samples[samples.length - 1] || event;
				discardPendingPointerMove();
				clearPendingTrackpadMotion(false);
				trackpadGesture.current.lastX = latest.clientX;
				trackpadGesture.current.lastY = latest.clientY;
				trackpadGesture.current.lastTime = latest.timeStamp;
				trackpadGesture.current.suppressTap = true;
				trackpadGestureViewportRevision.current = pointerViewportRevision.current;
				return;
			}
			const image = frameImageRef.current;
			if (!image) return;
			const rect = image.getBoundingClientRect();
			const nativeSamples = coalescedPointerSamples(event);
			const previousNative = { x: trackpadGesture.current.lastX, y: trackpadGesture.current.lastY };
			const previousTime = trackpadGesture.current.lastTime;
			const filtered = stableRemoteTrackpadSamples(
				previousNative,
				nativeSamples.map((sample) => ({ x: sample.clientX, y: sample.clientY, time: sample.timeStamp })),
				{ x: rect.width, y: rect.height },
				previousTime,
			);
			trackpadGesture.current.lastX = filtered.last.x;
			trackpadGesture.current.lastY = filtered.last.y;
			trackpadGesture.current.lastTime = filtered.lastTime;
			const elapsed = previousTime > 0 && filtered.lastTime >= previousTime ? filtered.lastTime - previousTime : 0;
			const currentPending = trackpadGesture.current.pendingX !== 0 || trackpadGesture.current.pendingY !== 0
				? { x: trackpadGesture.current.pendingX, y: trackpadGesture.current.pendingY }
				: null;
			const stabilized = stabilizeRemoteTrackpadMotion(currentPending, filtered.deltas, { x: rect.width, y: rect.height }, elapsed);
			clearPendingTrackpadMotion(false);
			if (stabilized.pending) {
				trackpadGesture.current.pendingX = stabilized.pending.x;
				trackpadGesture.current.pendingY = stabilized.pending.y;
				armPendingTrackpadMotion(event.pointerId);
			}
			const nativeTravel = Math.hypot(filtered.last.x - previousNative.x, filtered.last.y - previousNative.y);
			if (nativeTravel > 10 && trackpadGesture.current.timer) {
				window.clearTimeout(trackpadGesture.current.timer);
				trackpadGesture.current.timer = 0;
			}
			commitTrackpadDeltas(stabilized.deltas);
			return;
		}
		if (directGesture.current.pointerId === event.pointerId) {
			if (directGestureViewportRevision.current !== pointerViewportRevision.current) {
				const gesture = directGesture.current;
				discardPendingPointerMove();
				if (gesture.timer) window.clearTimeout(gesture.timer);
				if (gesture.leftDown) {
					sendInput({
						type: "pointer", action: "up", button: "left",
						x: gesture.x, y: gesture.y,
						coordinateWidth: gesture.coordinateWidth,
						coordinateHeight: gesture.coordinateHeight,
					}, false);
				}
				const position = pointerPosition(event);
				gesture.startX = event.clientX;
				gesture.startY = event.clientY;
				gesture.startRemoteX = position.x;
				gesture.startRemoteY = position.y;
				gesture.x = position.x;
				gesture.y = position.y;
				gesture.coordinateWidth = position.coordinateWidth;
				gesture.coordinateHeight = position.coordinateHeight;
				gesture.moved = true;
				gesture.leftDown = false;
				gesture.suppressTap = true;
				gesture.timer = 0;
				directGestureViewportRevision.current = pointerViewportRevision.current;
				return;
			}
			const position = pointerPosition(event);
			const distance = Math.abs(event.clientX-directGesture.current.startX)+Math.abs(event.clientY-directGesture.current.startY);
			directGesture.current.x = position.x; directGesture.current.y = position.y;
			directGesture.current.coordinateWidth = position.coordinateWidth;
			directGesture.current.coordinateHeight = position.coordinateHeight;
			if (distance > 10 && !directGesture.current.longPress) {
				directGesture.current.moved = true;
				if (directGesture.current.timer) { window.clearTimeout(directGesture.current.timer); directGesture.current.timer = 0; }
				if (!directGesture.current.leftDown) {
					const start = {
						x: directGesture.current.startRemoteX,
						y: directGesture.current.startRemoteY,
						coordinateWidth: directGesture.current.coordinateWidth,
						coordinateHeight: directGesture.current.coordinateHeight,
					};
					// Begin a direct-touch drag at the actual touch-down coordinate. Starting
					// at the threshold-crossing sample can miss a narrow title bar or handle.
					sendInput({ type: "pointer", action: "move", ...start });
					sendInput({ type: "pointer", action: "down", button: "left", ...start });
					directGesture.current.leftDown = true;
				}
			}
			schedulePointerMove({ type: "pointer", action: "move", ...position }, directGesture.current.leftDown);
			return;
		}
		const position = pointerPosition(event);
		if (event.pointerType === "mouse") lastMousePosition.current = position;
		schedulePointerMove({ type: "pointer", action: "move", ...position }, directGesture.current.leftDown);
  }

	function pointerButton(event: ReactPointerEvent<HTMLDivElement>, action: "down" | "up") {
    event.preventDefault();
		if (action === "down") void flushPendingRemoteClipboardFromGesture();
		// Stateful input must never overtake the newest throttled move. Flushing here
		// keeps drag boundaries ordered: move -> down/up, including on slow proxies.
		flushPendingPointerMove();
    viewportRef.current?.focus();
		if (event.pointerType !== "mouse") {
			if (action === "down") {
				capturePointerSafely(event.currentTarget, event.pointerId);
				if (beginTouchGesture(event)) return;
			} else if (pinchGesture.current.active || pinchGesture.current.suppress) {
				// Remove the native pointer from our gesture before releasing capture.
				// Chromium may dispatch lostpointercapture synchronously here; if the
				// pointer still looks active, the interruption handler tears down the
				// whole gesture and the next sample appears to jump.
				endTouchGesture(event.pointerId);
				releasePointerSafely(event.currentTarget, event.pointerId);
				return;
			}
		}
		if (mobileTrackpadMode) {
			const image = frameImageRef.current;
			if (!image) return;
			const size = ensureTrackpadCursor(image);
			if (action === "down") {
				clearPendingTrackpadMotion(false);
				capturePointerSafely(event.currentTarget, event.pointerId);
				const previousTap = lastTrackpadTap.current;
				const secondTap = performance.now() - previousTap.at < 330 && Math.hypot(event.clientX - previousTap.clientX, event.clientY - previousTap.clientY) < 48;
				if (secondTap) {
					lastTrackpadTap.current.at = 0;
					const position = serialisedTrackpadPosition();
					sendInput({ type: "pointer", action: "move", ...position });
					sendInput({ type: "pointer", action: "down", button: "left", ...position });
					trackpadGestureViewportRevision.current = pointerViewportRevision.current;
					trackpadGesture.current = { pointerId: event.pointerId, lastX: event.clientX, lastY: event.clientY, lastTime: event.timeStamp, distance: 0, longPress: false, dragging: true, secondTap: true, suppressTap: false, timer: 0, pendingX: 0, pendingY: 0, pendingTimer: 0 };
					positionLocalCursor(trackpadCursor.current, size);
					return;
				}
				const timer = window.setTimeout(() => {
					const gesture = trackpadGesture.current;
					if (gesture.pointerId !== event.pointerId || gesture.distance > 10 || pinchGesture.current.active) return;
					gesture.longPress = true;
					lastTrackpadTap.current.at = 0;
					sendPointerTap("right");
				}, 2000);
				trackpadGestureViewportRevision.current = pointerViewportRevision.current;
				trackpadGesture.current = { pointerId: event.pointerId, lastX: event.clientX, lastY: event.clientY, lastTime: event.timeStamp, distance: 0, longPress: false, dragging: false, secondTap: false, suppressTap: false, timer, pendingX: 0, pendingY: 0, pendingTimer: 0 };
				positionLocalCursor(trackpadCursor.current, size);
				return;
			}
			const ownsTrackpadGesture = trackpadGesture.current.pointerId === event.pointerId;
			if (ownsTrackpadGesture) {
				clearPendingTrackpadMotion(true);
				flushPendingPointerMove();
			}
			const wasDragging = ownsTrackpadGesture && trackpadGesture.current.dragging;
			const wasTap = ownsTrackpadGesture && trackpadGesture.current.distance < 10 && !trackpadGesture.current.longPress && !trackpadGesture.current.suppressTap;
			if (trackpadGesture.current.timer) window.clearTimeout(trackpadGesture.current.timer);
			trackpadGesture.current.pointerId = -1;
			trackpadGesture.current.dragging = false;
			trackpadGesture.current.timer = 0;
			if (event.pointerType !== "mouse") endTouchGesture(event.pointerId);
			releasePointerSafely(event.currentTarget, event.pointerId);
			if (wasDragging) {
				const position = serialisedTrackpadPosition();
				sendInput({ type: "pointer", action: "up", button: "left", ...position });
			} else if (wasTap) {
				const position = serialisedTrackpadPosition();
				sendInput({ type: "pointer", action: "move", ...position });
				for (const input of remotePointerTapActions("left")) sendInput({ type: "pointer", ...input, ...position });
				lastTrackpadTap.current = { at: performance.now(), clientX: event.clientX, clientY: event.clientY };
			}
			return;
		}
		if (event.pointerType !== "mouse") {
			const position = pointerPosition(event);
			if (action === "down") {
				capturePointerSafely(event.currentTarget, event.pointerId);
				directGestureViewportRevision.current = pointerViewportRevision.current;
				directGesture.current = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, startRemoteX: position.x, startRemoteY: position.y, x: position.x, y: position.y, coordinateWidth: position.coordinateWidth, coordinateHeight: position.coordinateHeight, moved: false, leftDown: false, longPress: false, suppressTap: false, timer: window.setTimeout(() => {
					const gesture = directGesture.current; if (gesture.pointerId !== event.pointerId || gesture.moved) return;
					const current = { x: gesture.x, y: gesture.y, coordinateWidth: gesture.coordinateWidth, coordinateHeight: gesture.coordinateHeight };
					gesture.longPress = true; sendInput({ type: "pointer", action: "move", ...current }); sendInput({ type: "pointer", action: "down", button: "right", ...current }); sendInput({ type: "pointer", action: "up", button: "right", ...current });
				}, 2000) };
				return;
			}
			const gesture = directGesture.current;
			const ownsDirectGesture = gesture.pointerId === event.pointerId;
			const wasLongPress = gesture.longPress;
			const wasLeftDown = gesture.leftDown;
			if (gesture.timer) window.clearTimeout(gesture.timer);
			// As above, make a normal release indistinguishable from an active
			// interruption before releasePointerCapture can emit its callback.
			directGesture.current.pointerId = -1;
			directGesture.current.timer = 0;
			endTouchGesture(event.pointerId);
			releasePointerSafely(event.currentTarget, event.pointerId);
			if (ownsDirectGesture && !wasLongPress && !gesture.suppressTap) {
				if (wasLeftDown) sendInput({ type: "pointer", action: "up", button: "left", ...position });
				else { sendInput({ type: "pointer", action: "down", button: "left", ...position }); sendInput({ type: "pointer", action: "up", button: "left", ...position }); }
			}
			return;
		}
		const button = event.button === 2 ? "right" : event.button === 1 ? "middle" : "left";
		const position = pointerPosition(event);
		lastMousePosition.current = position;
		if (action === "down") {
			heldMouseButton.current = button;
			capturePointerSafely(event.currentTarget, event.pointerId);
		} else {
			// Clear this before releasePointerCapture: a conforming browser dispatches
			// lostpointercapture synchronously or immediately afterwards. The dedicated
			// handler must only synthesize pointer-up for an actually interrupted press.
			heldMouseButton.current = null;
			releasePointerSafely(event.currentTarget, event.pointerId);
		}
		sendInput({ type: "pointer", action, button, ...position });
  }

	function lostPointerCapture(event: ReactPointerEvent<HTMLDivElement>) {
		const ownsGesture = directGesture.current.pointerId === event.pointerId
			|| trackpadGesture.current.pointerId === event.pointerId
			|| activeTouches.current.has(event.pointerId)
			|| heldMouseButton.current !== null;
		if (!ownsGesture) return;
		event.preventDefault();
		releaseActivePointerRef.current();
	}

	function cancelPointer(event: ReactPointerEvent<HTMLDivElement>) {
		event.preventDefault();
		flushPendingPointerMove();
		if (event.pointerType !== "mouse") endTouchGesture(event.pointerId, true);
		// pointercancel is an interruption, not a click. Release exactly the buttons
		// that are genuinely held, clear local ownership first, then relinquish native
		// capture. Releasing capture before clearing the gesture lets Chromium fire
		// lostpointercapture synchronously and used to synthesize a second pointer-up
		// (or an unrelated left-up for the second finger of a pinch).
		releaseActivePointerRef.current();
		releasePointerSafely(event.currentTarget, event.pointerId);
	}

	function wheel(event: ReactWheelEvent<HTMLDivElement>) {
    event.preventDefault();
		flushPendingPointerMove();
		const pixelDelta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? Math.max(320, viewportSize.height) : 1);
		wheelAccumulator.current += pixelDelta;
		let steps = 0;
		while (Math.abs(wheelAccumulator.current) >= 36 && steps < 6) {
			const direction = wheelAccumulator.current > 0 ? 1 : -1;
			sendInput({ type: "wheel", delta: direction > 0 ? -120 : 120 });
			wheelAccumulator.current -= direction * 36;
			steps += 1;
		}
  }

	function sendWindowsShortcut(keyCode: number) {
		sendInput({ type: "key", action: "down", keyCode: 17 });
		sendInput({ type: "key", action: "down", keyCode });
		sendInput({ type: "key", action: "up", keyCode });
		sendInput({ type: "key", action: "up", keyCode: 17 });
	}

	function beginDeferredRemoteClipboardWrite(): boolean {
		if (!navigator.clipboard?.write || typeof ClipboardItem === "undefined") return false;
		const previous = clipboardPendingWriteRef.current;
		if (previous) {
			window.clearTimeout(previous.timer);
			previous.reject(new Error("Запрошено новое копирование"));
		}
		let resolveBlob: (blob: Blob) => void = () => undefined;
		let rejectBlob: (reason?: unknown) => void = () => undefined;
		const value = new Promise<Blob>((resolve, reject) => { resolveBlob = resolve; rejectBlob = reject; });
		const pending: PendingRemoteClipboardWrite = {
			resolve: resolveBlob,
			reject: rejectBlob,
			timer: window.setTimeout(() => {
				if (clipboardPendingWriteRef.current !== pending) return;
				clipboardPendingWriteRef.current = null;
				rejectBlob(new Error("Удалённый буфер не ответил"));
			}, 3_000),
		};
		clipboardPendingWriteRef.current = pending;
		// WebKit preserves the trusted Cmd+C activation while this promised payload
		// is resolved asynchronously by the Agent acknowledgement.
		void navigator.clipboard.write([new ClipboardItem({ "text/plain": value })]).catch(() => {
			if (clipboardPendingWriteRef.current === pending) {
				window.clearTimeout(pending.timer);
				clipboardPendingWriteRef.current = null;
			}
		});
		return true;
	}

	function copyRemoteClipboardFromMac(cut: boolean) {
		beginDeferredRemoteClipboardWrite();
		sendWindowsShortcut(cut ? 88 : 67);
		window.setTimeout(() => sendInput({ type: "clipboard_read" }), 100);
	}

	async function pasteLocalClipboardToRemote(snapshot: LocalClipboardSnapshot) {
		await activateRemoteControl();
		await pushLocalClipboardSnapshot(snapshot);
		sendWindowsShortcut(86);
	}

	function flushPendingRemoteClipboardFromGesture() {
		const pendingImage = clipboardPendingRemoteImageRef.current;
		if (pendingImage) {
			void writeClipboardPNG(pendingImage).then(async () => {
				clipboardLastLocalImageRef.current = await clipboardBlobFingerprint(pendingImage);
				if (clipboardPendingRemoteImageRef.current === pendingImage) clipboardPendingRemoteImageRef.current = null;
			}).catch(() => undefined);
		}
		const pendingText = clipboardPendingRemoteTextRef.current;
		if (pendingText !== null && navigator.clipboard?.writeText) {
			void navigator.clipboard.writeText(pendingText).then(() => {
				clipboardLastLocalRef.current = pendingText;
				if (clipboardPendingRemoteTextRef.current === pendingText) clipboardPendingRemoteTextRef.current = null;
			}).catch(() => undefined);
		}
	}

	function pasteFromBrowser(event: ReactClipboardEvent<HTMLDivElement>) {
		if (!targetWindows) return;
		const text = event.clipboardData.getData("text/plain");
		const imageItem = Array.from(event.clipboardData.items).find((item) => item.type === "image/png");
		const image = imageItem?.getAsFile() || null;
		if (!image && text === "") return;
		event.preventDefault();
		void pasteLocalClipboardToRemote({ text, image }).catch((reason) => setError(reason instanceof Error ? reason.message : "Не удалось вставить буфер"));
	}

	function keyboard(event: ReactKeyboardEvent<HTMLDivElement>, action: "down" | "up") {
		if (action === "down") void flushPendingRemoteClipboardFromGesture();
		const physicalKey = event.code || event.key;
		if (targetWindows && macCommandClient) {
			if (physicalKey === "MetaLeft" || physicalKey === "MetaRight") {
				event.preventDefault();
				event.stopPropagation();
				return;
			}
			if (action === "up" && macCommandKeysRef.current.delete(physicalKey)) {
				event.preventDefault();
				event.stopPropagation();
				return;
			}
			if (action === "down" && event.metaKey) {
				const keyCode = browserCodeToVirtualKey(physicalKey);
				if (keyCode) {
					macCommandKeysRef.current.add(physicalKey);
					event.preventDefault();
					event.stopPropagation();
					if (keyCode === 67 || keyCode === 88) copyRemoteClipboardFromMac(keyCode === 88);
					else if (keyCode === 86) void readLocalClipboardSnapshot().then(pasteLocalClipboardToRemote).catch((reason) => setError(reason instanceof Error ? reason.message : "Браузер не разрешил вставку"));
					else sendWindowsShortcut(keyCode);
					return;
				}
			}
		}
		const plan = planRemoteKeyboardInput({
			code: event.code,
			key: event.key,
			ctrlKey: event.ctrlKey,
			altKey: event.altKey,
			metaKey: event.metaKey,
			altGraphKey: event.getModifierState("AltGraph"),
		}, action, textKeyboardKeys.current);
		if (!plan.handled) return;
    event.preventDefault();
    event.stopPropagation();
		if (plan.input) sendInput(plan.input);
  }

	function sendRemoteText(text: string) {
		for (const chunk of chunkRemoteText(text)) sendInput({ type: "text", text: chunk });
	}

	function sendVirtualKeyTap(keyCode: number) {
		sendInput({ type: "key", action: "down", keyCode });
		sendInput({ type: "key", action: "up", keyCode });
	}

	async function sendCtrlAltDelete() {
		// Ctrl+Alt+Delete is a Windows Secure Attention Sequence. It cannot be
		// synthesized with ordinary key packets. Use the acknowledged HTTP path for
		// this privileged event so the UI reports success only after the Windows
		// service, not merely the web server, has executed SendSAS.
		releaseRemoteModifiers();
		setError("");
		window.clearTimeout(sasFeedbackTimer.current);
		sasPendingInputID.current = 0;
		setSASFeedbackError(false);
		setSASFeedback("Передаём Ctrl+Alt+Delete системной службе Windows…");
		try {
			await activateRemoteControl();
			if (!sessionId) throw new Error("Удалённый сеанс ещё не готов");
			const response = await fetch(`/api/desktop-sessions/${sessionId}/input`, {
				method: "POST",
				credentials: "same-origin",
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				body: JSON.stringify({ type: "sas" }),
			});
			if (!response.ok) throw new Error(`Команда Ctrl+Alt+Delete не принята: HTTP ${response.status}`);
			const result = await response.json() as { inputId?: number };
			if (!Number.isSafeInteger(result.inputId) || Number(result.inputId) <= 0) throw new Error("Сервер не присвоил номер команде Ctrl+Alt+Delete");
			sasPendingInputID.current = Number(result.inputId);
			setSASFeedback("Команда доставлена Agent — ожидаем подтверждение Windows…");
			sasFeedbackTimer.current = window.setTimeout(() => {
				if (sasPendingInputID.current !== Number(result.inputId)) return;
				sasPendingInputID.current = 0;
				const message = "Agent не подтвердил Ctrl+Alt+Delete за 8 секунд. Проверьте системную установку и связь.";
				setSASFeedbackError(true);
				setSASFeedback(message);
				setError(message);
			}, 8_000);
		} catch (reason) {
			sasPendingInputID.current = 0;
			const message = reason instanceof Error ? reason.message : "Не удалось передать Ctrl+Alt+Delete";
			setSASFeedbackError(true);
			setSASFeedback(message);
			setError(message);
		}
	}

	function sendPointerTap(button: "left" | "right") {
		discardPendingPointerMove();
		const position = serialisedTrackpadPosition();
		sendInput({ type: "pointer", action: "move", ...position });
		for (const input of remotePointerTapActions(button)) sendInput({ type: "pointer", ...input, ...position });
	}

	function selectPointerMode(next: "direct" | "trackpad") {
		if (next === pointerMode) return;
		discardPendingPointerMove();
		clearPendingTrackpadMotion(false);
		setPointerMode(next);
		if (next !== "trackpad") return;
		window.requestAnimationFrame(() => {
			const image = frameImageRef.current;
			if (!image) return;
			const size = ensureTrackpadCursor(image);
			positionLocalCursor(trackpadCursor.current, size);
		});
	}

	function finishRemoteSession() {
		discardPendingPointerMove();
		clearPendingTrackpadMotion(false);
		explicitSessionFinishRef.current = true;
		onClose();
	}

	function openRemoteFiles() {
		if (keyboardOpen) setMobileKeyboardVisibility(false);
		setControlsCollapsed(true);
		setFilesOpen(true);
	}

	function remoteDesktopDragHasFiles(event: ReactDragEvent<HTMLDivElement>) {
		return Array.from(event.dataTransfer.types).includes("Files");
	}

	function enterRemoteDesktopDrop(event: ReactDragEvent<HTMLDivElement>) {
		if (!targetWindows || !remoteDesktopDragHasFiles(event)) return;
		event.preventDefault();
		desktopDropDepthRef.current += 1;
		setDesktopDropActive(true);
	}

	function leaveRemoteDesktopDrop(event: ReactDragEvent<HTMLDivElement>) {
		if (!targetWindows || !remoteDesktopDragHasFiles(event)) return;
		event.preventDefault();
		desktopDropDepthRef.current = Math.max(0, desktopDropDepthRef.current - 1);
		if (desktopDropDepthRef.current === 0) setDesktopDropActive(false);
	}

	async function dropFilesOnRemoteDesktop(event: ReactDragEvent<HTMLDivElement>) {
		if (!targetWindows || !remoteDesktopDragHasFiles(event)) return;
		event.preventDefault();
		event.stopPropagation();
		desktopDropDepthRef.current = 0;
		setDesktopDropActive(false);
		const files = Array.from(event.dataTransfer.files);
		if (!files.length) return;
		if (files.some((file) => file.size > 10 * 1024 * 1024 * 1024)) {
			setError("Размер каждого файла не должен превышать 10 ГБ");
			return;
		}
		const controller = new AbortController();
		desktopDropControllerRef.current?.abort();
		desktopDropControllerRef.current = controller;
		setError("");
		try {
			await uploadLocalFilesToDevice(device, csrf, files, REMOTE_USER_DESKTOP_TRANSFER_PATH, setDesktopDropProgress, controller.signal);
			setSASFeedbackError(false);
			setSASFeedback(`${files.length === 1 ? `Файл «${files[0].name}»` : `Файлов: ${files.length}`} загружено на рабочий стол ${device.name}`);
			sasFeedbackTimer.current = window.setTimeout(() => setSASFeedback(""), 5_000);
		} catch (reason) {
			if (!isAbortError(reason)) {
				const message = reason instanceof Error ? reason.message : "Не удалось загрузить файл на удалённый рабочий стол";
				setError(message);
				setSASFeedbackError(true);
				setSASFeedback(message);
			}
		} finally {
			if (desktopDropControllerRef.current === controller) desktopDropControllerRef.current = null;
			setDesktopDropProgress(null);
		}
	}

	function setMobileKeyboardVisibility(open: boolean) {
		keyboardOpenRef.current = open;
		setKeyboardOpen(open);
		if (open) {
			// The IME needs only a slim text bridge. Hide the large tool sheet and the
			// compact dock so the remote desktop remains visible above the keyboard.
			if (compactRemoteClient) {
				setMobileDockHidden(true);
				setControlsCollapsed(true);
			}
			window.requestAnimationFrame(() => window.setTimeout(() => mobileKeyboardRef.current?.focus({ preventScroll: true }), 30));
			return;
		}
		mobileKeyboardRef.current?.blur();
		// The text field is only a live input bridge, not a document editor. Clear
		// its local mirror without sending Backspace to the remote computer.
		setMobileText("");
		mobileTextSyncedRef.current = "";
		const bridge = (window as unknown as { RemoteItAndroid?: { hideKeyboard?: () => void } }).RemoteItAndroid;
		bridge?.hideKeyboard?.();
		// Android reports its full viewport a moment after the IME closes. Refresh
		// only then; while it is open the ResizeObserver intentionally freezes the
		// remote camera geometry.
		window.setTimeout(() => {
			const viewport = viewportRef.current;
			if (viewport && !keyboardOpenRef.current) setViewportSize({ width: viewport.clientWidth, height: viewport.clientHeight });
		}, 180);
	}

	function syncMobileText(next: string) {
		const plan = planRemoteTextReconciliation(mobileTextSyncedRef.current, next);
		for (let index = 0; index < plan.backspaces; index += 1) sendVirtualKeyTap(8);
		if (plan.text) sendRemoteText(plan.text);
		mobileTextSyncedRef.current = next;
	}

	function updateMobileText(next: string) {
		setMobileText(next);
		// Android IMEs commonly replace the composing suffix and do not emit a
		// dependable key event for Backspace. Reconcile every native input update;
		// the operation is idempotent, including the final composition event.
		syncMobileText(next);
	}

	function sendMobileEnter() {
		const now = performance.now();
		// A few Android IMEs emit both keydown and beforeinput for the same action.
		// Accept either path, but never turn one tap into two remote Enter presses.
		if (now - mobileEnterAt.current < 36) return;
		mobileEnterAt.current = now;
		sendVirtualKeyTap(13);
		setMobileText("");
		mobileTextSyncedRef.current = "";
		if (keyboardOpen) window.requestAnimationFrame(() => mobileKeyboardRef.current?.focus({ preventScroll: true }));
	}

	function sendMobileBoundaryDelete(keyCode: 8 | 46) {
		const now = performance.now();
		// Some Android keyboards emit both keydown and beforeinput for one press.
		// Keep the two paths as fallbacks while de-duplicating the same deletion.
		// Twelve milliseconds still collapses the duplicate keydown/beforeinput
		// pair emitted for one tap, but no longer drops legitimate fast key-repeat
		// events from Samsung/Gboard (often 25-35 ms apart).
		if (now - mobileBoundaryDeleteAt.current < 12) return;
		mobileBoundaryDeleteAt.current = now;
		sendVirtualKeyTap(keyCode);
	}

	function stopWheelRepeat(pointerId?: number) {
		const repeat = wheelRepeat.current;
		if (pointerId !== undefined && repeat.pointerId !== -1 && repeat.pointerId !== pointerId) return;
		window.clearTimeout(repeat.delay);
		window.clearInterval(repeat.interval);
		repeat.delay = 0;
		repeat.interval = 0;
		repeat.pointerId = -1;
	}

	function startWheelRepeat(event: ReactPointerEvent<HTMLButtonElement>, direction: 1 | -1) {
		if (!event.isPrimary) return;
		event.preventDefault();
		stopWheelRepeat();
		wheelRepeat.current.pointerId = event.pointerId;
		capturePointerSafely(event.currentTarget, event.pointerId);
		sendInput({ type: "wheel", delta: direction * 240 });
		wheelRepeat.current.delay = window.setTimeout(() => {
			wheelRepeat.current.interval = window.setInterval(() => sendInput({ type: "wheel", delta: direction * 120 }), 80);
		}, 240);
	}

	function wheelKeyboard(event: ReactKeyboardEvent<HTMLButtonElement>, direction: 1 | -1) {
		if (event.key !== "Enter" && event.key !== " ") return;
		event.preventDefault();
		sendInput({ type: "wheel", delta: direction * 240 });
	}

	function toggleControls() {
		const next = !controlsCollapsed;
		if (next && keyboardOpen) setMobileKeyboardVisibility(false);
		if (!next) setMobileDockHidden(false);
		setControlsCollapsed(next);
	}

	async function toggleClipboardSync() {
		window.clearTimeout(sasFeedbackTimer.current);
		try {
			if (!navigator.clipboard?.readText || !navigator.clipboard?.writeText) throw new Error("Этот браузер не поддерживает общий буфер");
			flushPendingRemoteClipboardFromGesture();
			// Read while the button click still owns transient browser activation.
			// Awaiting the control API first makes Safari discard that grant.
			const snapshot = await readLocalClipboardSnapshot();
			await activateRemoteControl();
			clipboardLastRemoteAckRef.current = 0;
			clipboardPermissionErrorRef.current = false;
			clipboardBackgroundReadBlockedRef.current = false;
			await pushLocalClipboardSnapshot(snapshot);
			sendInput({ type: "clipboard_read" });
			setSASFeedbackError(false);
			setSASFeedback("Общий буфер синхронизирован в обе стороны");
			sasFeedbackTimer.current = window.setTimeout(() => setSASFeedback(""), 4_500);
		} catch (reason) {
			const message = reason instanceof Error ? reason.message : "Браузер не разрешил общий буфер";
			setSASFeedbackError(true);
			setSASFeedback(`${message}. На Mac используйте Cmd+C / Cmd+V прямо в удалённом экране.`);
		}
	}

	async function pushLocalClipboardSnapshot(snapshot: LocalClipboardSnapshot) {
		if (snapshot.image) {
			if (snapshot.image.size < 1 || snapshot.image.size > 12<<20) throw new Error("PNG в буфере больше 12 МБ — передайте его как файл");
			const hash = await clipboardBlobFingerprint(snapshot.image);
			if (hash === clipboardLastLocalImageRef.current) return;
			await uploadLocalClipboardImage(snapshot.image);
			clipboardLastLocalImageRef.current = hash;
			clipboardPermissionErrorRef.current = false;
			return;
		}
		if (new TextEncoder().encode(snapshot.text).byteLength > 32<<10) throw new Error("Текст в буфере больше 32 КБ — передайте его как файл");
		if (snapshot.text === clipboardLastLocalRef.current) return;
		clipboardLastLocalRef.current = snapshot.text;
		clipboardPermissionErrorRef.current = false;
		sendInput({ type: "clipboard_write", text: snapshot.text });
	}

	async function uploadLocalClipboardImage(image: Blob) {
		if (image.size < 1 || image.size > 12<<20) throw new Error("PNG в буфере больше 12 МБ — передайте его как файл");
		const response = await fetch(`/api/desktop-sessions/${sessionId}/clipboard-image`, {
			method: "POST",
			credentials: "same-origin",
			headers: { "Content-Type": "image/png", "X-CSRF-Token": csrf },
			body: image,
		});
		if (!response.ok) {
			let message = `Изображение буфера: HTTP ${response.status}`;
			try {
				const payload = await response.json() as { error?: string };
				if (payload.error) message = payload.error;
			} catch {
				// Keep the bounded HTTP error when an intermediary returns non-JSON.
			}
			throw new Error(message);
		}
	}

	async function copyRemoteScreenshot() {
		const image = frameImageRef.current;
		if (!image || !image.complete || image.naturalWidth <= 0 || image.naturalHeight <= 0) {
			setError("Кадр удалённого экрана ещё не готов");
			return;
		}
		try {
			const canvas = document.createElement("canvas");
			canvas.width = image.naturalWidth;
			canvas.height = image.naturalHeight;
			const context = canvas.getContext("2d", { alpha: false });
			if (!context) throw new Error("Браузер не создал снимок экрана");
			context.imageSmoothingEnabled = false;
			context.drawImage(image, 0, 0, canvas.width, canvas.height);
			const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((result) => result ? resolve(result) : reject(new Error("Не удалось сформировать PNG")), "image/png"));
			const clipboardWrite = navigator.clipboard?.write?.bind(navigator.clipboard);
			if (clipboardWrite && typeof ClipboardItem !== "undefined") {
				try {
					await clipboardWrite([new ClipboardItem({ "image/png": blob })]);
					clipboardLastLocalImageRef.current = await clipboardBlobFingerprint(blob);
					window.clearTimeout(sasFeedbackTimer.current);
					setSASFeedbackError(false);
					setSASFeedback(`Снимок ${canvas.width}×${canvas.height} скопирован; общий буфер продолжает работать`);
					sasFeedbackTimer.current = window.setTimeout(() => setSASFeedback(""), 4_500);
					return;
				} catch {
					// Clipboard image writes require a transient browser permission and are
					// commonly rejected by Firefox/Safari/managed Chromium. Download the
					// already-created PNG instead of turning a usable screenshot into an error.
				}
			}
			const safeDevice = device.name.replace(/[^\p{L}\p{N}_.-]+/gu, "-").replace(/^-+|-+$/g, "") || "device";
			const link = document.createElement("a");
			const url = URL.createObjectURL(blob);
			link.href = url;
			link.download = `RemoteIt-${safeDevice}-${new Date().toISOString().replace(/[:.]/g, "-")}.png`;
			link.click();
			window.setTimeout(() => URL.revokeObjectURL(url), 1_000);
			setSASFeedbackError(false);
			setSASFeedback("Браузер не разрешил изображение в буфере — снимок скачан как PNG");
		} catch (reason) {
			const message = reason instanceof Error ? reason.message : "Не удалось скопировать снимок";
			setSASFeedbackError(true);
			setSASFeedback(message);
			setError(message);
		}
	}

	// The decoded image wins over the asynchronously refreshed status. This is
	// essential when a VDI session changes monitor/resolution or a phone rotates:
	// stale status dimensions previously produced a visibly cropped frame.
	const sourceWidth = renderedFrameSize.width || status?.frameWidth || 0;
	const sourceHeight = renderedFrameSize.height || status?.frameHeight || 0;
	const autoFit = fitRemoteFrame({ x: sourceWidth, y: sourceHeight }, { x: viewportSize.width, y: viewportSize.height });
	const autoFill = fillRemoteFrame({ x: sourceWidth, y: sourceHeight }, { x: viewportSize.width, y: viewportSize.height });
	const viewFrame = screenScale === "fill" ? autoFill : screenScale === "actual" ? { x: Math.max(1, sourceWidth), y: Math.max(1, sourceHeight) } : autoFit;
	const fittedFrame = { width: viewFrame.x, height: viewFrame.y };
	const baseFrameScale = sourceWidth > 0 ? fittedFrame.width / sourceWidth : 1;
	const totalFrameScale = Math.max(0.0001, baseFrameScale * camera.zoom);
	const remoteImageLayerStyle = sourceWidth > 0 && sourceHeight > 0 ? {
		// Keep the DOM image at its natural remote resolution and scale the layer
		// once. Scaling an image that had first been laid out at phone width caused
		// the browser to magnify a low-resolution compositor surface during pinch.
		width: `${sourceWidth}px`,
		height: `${sourceHeight}px`,
		transform: `translate3d(${camera.panX}px,${camera.panY}px,0) scale(${totalFrameScale})`,
		transformOrigin: "center center",
		"--remote-camera-zoom": totalFrameScale,
	} as CSSProperties : undefined;

	const resetCamera = () => {
		// "По размеру" is the Remote Desktop-style home view: the complete remote
		// frame is visible in both orientations, independent of the previous fixed
		// scale or pinch zoom.
		if (screenScale !== "fit") setScreenScale("fit");
		scheduleCamera({ zoom: 1, panX: 0, panY: 0 });
	};

	async function toggleRemoteFullscreen() {
		const target = workspaceRef.current as (HTMLElement & { webkitRequestFullscreen?: () => Promise<void> | void }) | null;
		const doc = document as Document & { webkitFullscreenElement?: Element | null; webkitExitFullscreen?: () => Promise<void> | void };
		try {
			const bridge = (window as unknown as { RemoteItAndroid?: { requestImmersiveFullscreen?: () => void } }).RemoteItAndroid;
			bridge?.requestImmersiveFullscreen?.();
			if (document.fullscreenElement || doc.webkitFullscreenElement) {
				if (document.exitFullscreen) await document.exitFullscreen();
				else await doc.webkitExitFullscreen?.();
				return;
			}
			if (!target) return;
			scaleBeforeFullscreenRef.current = screenScale;
			// A desktop browser must keep the whole remote monitor visible in
			// fullscreen. "fill" crops the long axis whenever the local and remote
			// aspect ratios differ, which made the edges and Windows taskbar appear
			// to be missing. Compact phone clients intentionally retain their
			// edge-to-edge fill presentation; desktop returns to Remote Desktop-style
			// fit-to-screen geometry.
			setScreenScale(compactRemoteClient ? "fill" : "fit");
			scheduleCamera({ zoom: 1, panX: 0, panY: 0 });
			if (target.requestFullscreen) await target.requestFullscreen({ navigationUI: "hide" });
			else if (target.webkitRequestFullscreen) await target.webkitRequestFullscreen();
			else {
				// iOS Safari does not expose element fullscreen for normal DOM nodes.
				// The visual-viewport layout still fills every CSS pixel it provides.
				setError("Браузер не поддерживает полноэкранный режим страницы — экран заполнен до границ браузера");
			}
		} catch {
			if (scaleBeforeFullscreenRef.current) setScreenScale(scaleBeforeFullscreenRef.current);
			scaleBeforeFullscreenRef.current = null;
			setError("Полноэкранный режим заблокирован браузером — нажмите кнопку ещё раз");
		}
	}

	const remoteViewportStyle = compactRemoteClient ? {
		"--remote-vv-left": `${remoteViewport.left}px`,
		"--remote-vv-top": `${remoteViewport.top}px`,
		"--remote-vv-width": `${remoteViewport.width}px`,
		"--remote-vv-height": `${remoteViewport.height}px`,
	} as CSSProperties : undefined;
	const desktopDropFileProgress = desktopDropProgress ? fileTransferProgress(desktopDropProgress.received, desktopDropProgress.size, desktopDropProgress.startedAt) : null;

	const workspace = <section ref={workspaceRef} style={remoteViewportStyle} className={`remote-desktop-modal ${embedded ? "remote-desktop-embedded" : ""} ${compactRemoteClient ? "remote-compact-client" : ""} ${controlsCollapsed ? "remote-controls-collapsed" : ""} ${mobileDockHidden ? "remote-mobile-dock-hidden" : ""} ${keyboardOpen ? "remote-keyboard-open" : ""} ${remoteViewport.landscape ? "remote-runtime-landscape" : "remote-runtime-portrait"} ${fullscreenActive ? "remote-is-fullscreen" : ""}`}>
		<header>
			<div><span className="eyebrow">ЗАЩИЩЁННЫЙ СЕАНС</span><h2>{device.name}</h2><small><span className={`desktop-live-dot ${status?.agentConnected ? "active" : ""}`} />{status?.agentConnected ? `Экран ${status.frameWidth || "—"}×${status.frameHeight || "—"} · ${status.controlEnabled ? "управление активно" : "предпросмотр — нажмите на экран для управления"}` : "Ожидание настольного Agent…"}</small></div>
			<div className="desktop-actions">
				<span className="remote-mode-badge">{pointerMode === "direct" ? "Касание" : "Курсор"}</span>
				<label className="remote-scale-control" title="Частота кадров"><span>FPS</span><select value={targetFPS} onChange={(event) => updateTargetFPS(event.target.value as typeof targetFPS)}><option value="auto">Авто · 30</option><option value="15">15</option><option value="30">30</option><option value="60">60 · Плавность</option></select></label>
				<label className="remote-scale-control" title="Как разместить удалённый рабочий стол"><span>Вид</span><select value={screenScale} onChange={(event) => setScreenScale(event.target.value as RemoteScaleMode)}><option value="fit">Весь экран</option><option value="fill">Заполнить</option><option value="actual">1:1</option></select></label>
				<button className="remote-header-tool" onClick={() => void toggleRemoteFullscreen()} title={fullscreenActive ? "Выйти из полноэкранного режима" : "На весь экран"}><Maximize2 size={17} /><span>{fullscreenActive ? "Свернуть" : "Полный экран"}</span></button>
				<button className="remote-header-tool" onClick={() => void copyRemoteScreenshot()} title="Скопировать текущий кадр как PNG"><Camera size={17} /><span>Снимок</span></button>
				{targetWindows && <button className="remote-header-tool" onClick={() => void sendCtrlAltDelete()} title="Отправить Ctrl+Alt+Del"><Keyboard size={17} /><span>Ctrl+Alt+Del</span></button>}
				{targetWindows && <button className="remote-header-tool active" onClick={() => void toggleClipboardSync()} title="Общий буфер включён; нажмите для немедленной синхронизации"><Clipboard size={17} /><span>Буфер · общий</span></button>}
				<button className={`remote-header-tool ${keyboardOpen ? "active" : ""}`} onClick={() => setMobileKeyboardVisibility(!keyboardOpen)} title="Экранная клавиатура"><Keyboard size={17} /><span>Клавиатура</span></button>
				{targetWindows && <button className="remote-header-tool" onClick={openRemoteFiles} title="Файлы устройства"><Folder size={17} /><span>Файлы</span></button>}
				<button className="remote-header-tool danger" onClick={finishRemoteSession} title="Завершить сеанс"><Power size={18} /><span>Завершить</span></button>
			</div>
		</header>
		<div className={`remote-screen pointer-${pointerMode} screen-scale-${screenScale} ${desktopDropActive ? "remote-desktop-drop-active" : ""}`} ref={viewportRef} tabIndex={0} onKeyDown={(event) => keyboard(event, "down")} onKeyUp={(event) => keyboard(event, "up")} onPaste={pasteFromBrowser} onPointerMove={movePointer} onPointerDown={(event) => pointerButton(event, "down")} onPointerUp={(event) => pointerButton(event, "up")} onPointerCancel={cancelPointer} onLostPointerCapture={lostPointerCapture} onWheel={wheel} onContextMenu={(event) => event.preventDefault()} onDragEnter={enterRemoteDesktopDrop} onDragOver={(event) => { if (targetWindows && remoteDesktopDragHasFiles(event)) { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; } }} onDragLeave={leaveRemoteDesktopDrop} onDrop={(event) => void dropFilesOnRemoteDesktop(event)}>
			{desktopDropActive && <div className="remote-desktop-drop-overlay"><span><Upload size={34} /><strong>Отпустите файл</strong><small>Он появится на рабочем столе {device.name}</small></span></div>}
			{desktopDropProgress && desktopDropFileProgress && <div className="remote-desktop-drop-progress" role="status"><Upload size={18} /><span><strong>{desktopDropProgress.stage}</strong><small>{desktopDropProgress.label} · {formatBytes(desktopDropFileProgress.received)} из {formatBytes(desktopDropFileProgress.total)}</small><i><b style={{ width: `${desktopDropFileProgress.percent}%` }} /></i></span><em>{Math.round(desktopDropFileProgress.percent)}%</em></div>}
			{frameURL ? <>
				<div className="remote-screen-canvas">
					<div ref={frameImageLayerRef} className="remote-screen-image-layer" style={remoteImageLayerStyle}>
				<img ref={frameImageRef} className="remote-screen-image" src={frameURL} draggable={false} onLoad={(event) => { const width = event.currentTarget.naturalWidth; const height = event.currentTarget.naturalHeight; if (frameImageLayerRef.current && !frameImageLayerRef.current.style.backgroundImage) frameImageLayerRef.current.style.backgroundImage = `url("${event.currentTarget.src}")`; setRenderedFrameSize((current) => current.width === width && current.height === height ? current : { width, height }); }} />
						{localCursorVisible && <span ref={localCursorRef} className="remote-local-cursor" aria-hidden="true"><MousePointer2 size={22} strokeWidth={2.4} /></span>}
					</div>
				</div>
				<span className={`remote-stream-stats ${streamStatsCollapsed ? "collapsed" : ""}`} title={status?.captureDiagnostics ? `${status.captureDiagnostics.captureBackend}: захват ${status.captureDiagnostics.captureMillis} мс, копирование ${status.captureDiagnostics.copyMillis} мс, масштаб ${status.captureDiagnostics.scaleMillis} мс, JPEG ${status.captureDiagnostics.encodeMillis} мс` : ""}>
					<b>HD</b>
					<span className="remote-stream-stats-details"><span>{latencyMs || "—"} мс</span><em>{frameFPS || "—"} FPS</em><em>{camera.zoom > 1 ? `${Math.round(camera.zoom * 100)}%` : screenScale === "fit" ? "ВЕСЬ ЭКРАН" : screenScale === "fill" ? "ЗАПОЛНЕНО" : "1:1"}</em></span>
					<button type="button" className="remote-stream-stats-toggle" onPointerDown={(event) => event.stopPropagation()} onPointerUp={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); setStreamStatsCollapsed((current) => !current); }} aria-expanded={!streamStatsCollapsed} aria-label={streamStatsCollapsed ? "Развернуть показатели соединения" : "Свернуть показатели соединения"} title={streamStatsCollapsed ? "Развернуть показатели" : "Свернуть показатели"}><ChevronLeft size={12} /></button>
				</span>
			</> : <div className="remote-screen-wait"><ScreenShare size={42} /><strong>{starting ? "Создаём сеанс…" : "Ожидаем первый кадр"}</strong><span>На удалённом компьютере должен быть запущен RemoteIt Agent 0.9.26 или новее.</span></div>}
		</div>
		{!compactRemoteClient && fullscreenActive && <nav className="remote-desktop-fullscreen-tools" aria-label="Управление полноэкранным сеансом">
			<button type="button" onClick={() => void copyRemoteScreenshot()} title="Скопировать снимок"><Camera size={18} /></button>
			{targetWindows && <button type="button" className="active" onClick={() => void toggleClipboardSync()} title="Синхронизировать общий буфер"><Clipboard size={18} /></button>}
			<button type="button" onClick={() => void toggleRemoteFullscreen()} title="Выйти из полноэкранного режима"><Maximize2 size={18} /></button>
			<button type="button" className="danger" onClick={finishRemoteSession} title="Завершить сеанс"><Power size={18} /></button>
		</nav>}
		{compactRemoteClient && controlsCollapsed && !mobileDockHidden && <nav className="remote-mobile-dock" aria-label="Быстрое управление удалённым компьютером">
			<button type="button" className={pointerMode === "trackpad" ? "active" : ""} aria-pressed={pointerMode === "trackpad"} onClick={() => selectPointerMode("trackpad")}><MousePointer2 size={18} /><span>Курсор</span></button>
			<button type="button" className={pointerMode === "direct" ? "active" : ""} aria-pressed={pointerMode === "direct"} onClick={() => selectPointerMode("direct")}><Hand size={18} /><span>Касание</span></button>
			<button type="button" onClick={() => setMobileKeyboardVisibility(true)}><Keyboard size={18} /><span>Клавиатура</span></button>
			<button type="button" onClick={() => { setMobileDockHidden(false); setControlsCollapsed(false); }}><SlidersHorizontal size={18} /><span>Ещё</span></button>
			<button type="button" className="dock-hide" onClick={() => setMobileDockHidden(true)} aria-label="Скрыть все элементы управления"><X size={17} /></button>
		</nav>}
		{compactRemoteClient && controlsCollapsed && mobileDockHidden && <button type="button" className="remote-mobile-dock-reveal" onClick={() => setMobileDockHidden(false)} aria-label="Показать управление"><SlidersHorizontal size={18} /></button>}
		{compactRemoteClient && controlsCollapsed && <button type="button" className="remote-mobile-fullscreen-fab" onClick={() => void toggleRemoteFullscreen()} aria-label={fullscreenActive ? "Выйти из полноэкранного режима" : "Открыть на весь экран"} title={fullscreenActive ? "Свернуть" : "На весь экран"}><Maximize2 size={19} /></button>}
		{compactRemoteClient && (controlsCollapsed || remoteViewport.landscape) && frameURL && <nav className="remote-mobile-scroll-rail" aria-label="Прокрутка удалённого экрана">
			<button type="button" onPointerDown={(event) => startWheelRepeat(event, 1)} onPointerUp={(event) => stopWheelRepeat(event.pointerId)} onPointerCancel={(event) => stopWheelRepeat(event.pointerId)} onLostPointerCapture={(event) => stopWheelRepeat(event.pointerId)} onKeyDown={(event) => wheelKeyboard(event, 1)} aria-label="Прокрутить вверх; удерживайте для непрерывной прокрутки" title="Вверх · можно удерживать"><ChevronUp size={18} /></button>
			<button type="button" onPointerDown={(event) => startWheelRepeat(event, -1)} onPointerUp={(event) => stopWheelRepeat(event.pointerId)} onPointerCancel={(event) => stopWheelRepeat(event.pointerId)} onLostPointerCapture={(event) => stopWheelRepeat(event.pointerId)} onKeyDown={(event) => wheelKeyboard(event, -1)} aria-label="Прокрутить вниз; удерживайте для непрерывной прокрутки" title="Вниз · можно удерживать"><ChevronDown size={18} /></button>
		</nav>}
		{compactRemoteClient && <button type="button" className={`remote-mobile-keyboard-fab ${keyboardOpen ? "active" : ""} ${controlsCollapsed && !mobileDockHidden ? "dock-visible" : ""}`} onClick={() => setMobileKeyboardVisibility(!keyboardOpen)} aria-label={keyboardOpen ? "Закрыть клавиатуру" : "Открыть клавиатуру"} title={keyboardOpen ? "Закрыть клавиатуру" : "Открыть клавиатуру"}><Keyboard size={20} /></button>}
		{compactRemoteClient && !controlsCollapsed && <button type="button" className="remote-controls-scrim" aria-label="Скрыть панель управления" onClick={toggleControls} />}
		<footer className="remote-session-footer">
			<div className="remote-session-tools">
				<span><MousePointer2 size={15} /> {pointerMode === "direct" ? "Касание — левый клик · удержание 2 секунды — правый" : "Ведите пальцем — перемещайте курсор; удержание 2 секунды — правый клик"}</span>
				<div className="remote-pointer-modes">
					<button type="button" className={`remote-tool-button ${pointerMode === "trackpad" ? "active" : ""}`} aria-pressed={pointerMode === "trackpad"} onClick={() => selectPointerMode("trackpad")}><MousePointer2 size={16} /> Курсор</button>
					<button type="button" className={`remote-tool-button ${pointerMode === "direct" ? "active" : ""}`} aria-pressed={pointerMode === "direct"} onClick={() => selectPointerMode("direct")}><Hand size={16} /> Касание</button>
				</div>
				<button type="button" className={`remote-tool-button ${keyboardOpen ? "active" : ""}`} onClick={() => setMobileKeyboardVisibility(!keyboardOpen)}><Keyboard size={15} /> Клавиатура</button>
				{targetWindows && <button type="button" className="remote-tool-button" onClick={() => void sendCtrlAltDelete()}><Keyboard size={15} /> Ctrl+Alt+Del</button>}
				<button type="button" className="remote-tool-button" onClick={() => void toggleRemoteFullscreen()}><Maximize2 size={15} /> {fullscreenActive ? "Свернуть" : "Полный экран"}</button>
				<button type="button" className="remote-tool-button" onClick={resetCamera}><Maximize2 size={15} /> По размеру</button>
				<button type="button" className="remote-tool-button" onClick={() => void copyRemoteScreenshot()}><Camera size={15} /> Снимок</button>
				{targetWindows && <button type="button" className="remote-tool-button" onClick={openRemoteFiles}><Folder size={15} /> Файлы</button>}
				{targetWindows && <button type="button" className="remote-tool-button active" onClick={() => void toggleClipboardSync()}><Clipboard size={15} /> Буфер общий</button>}
				<button type="button" className="remote-tool-button remote-collapse-tool" onClick={toggleControls} title={controlsCollapsed ? "Открыть управление" : "Скрыть управление"}><SlidersHorizontal size={16} />{controlsCollapsed ? "Управление" : "Скрыть"}</button>
			</div>
			<button className="danger-button remote-footer-end" onClick={finishRemoteSession}><Power size={15} /> Завершить сеанс</button>
		</footer>
		{keyboardOpen && <div className="remote-mobile-keyboard" role="group" aria-label="Ввод на удалённом компьютере">
			<input ref={mobileKeyboardRef} name="remoteit-live-input" autoComplete="off" value={mobileText} onBeforeInput={(event) => { const input = event.currentTarget; const plan = planRemoteMobileBeforeInput((event.nativeEvent as InputEvent).inputType || "", input.value, input.selectionStart, input.selectionEnd); if (plan.handled && plan.keyCode) { event.preventDefault(); if (plan.keyCode === 13) sendMobileEnter(); else sendMobileBoundaryDelete(plan.keyCode); } }} onInput={(event) => updateMobileText(event.currentTarget.value)} onCompositionEnd={(event) => updateMobileText(event.currentTarget.value)} onSelect={(event) => { const input = event.currentTarget; if (input.selectionStart !== input.value.length || input.selectionEnd !== input.value.length) window.requestAnimationFrame(() => input.setSelectionRange(input.value.length, input.value.length)); }} onKeyDown={(event) => { if ((event.key === "Backspace" || event.key === "Delete") && !event.nativeEvent.isComposing) { const inputType = event.key === "Backspace" ? "deleteContentBackward" : "deleteContentForward"; const plan = planRemoteBoundaryDeletion(inputType, event.currentTarget.value, event.currentTarget.selectionStart, event.currentTarget.selectionEnd); if (plan.handled && plan.keyCode) { event.preventDefault(); sendMobileBoundaryDelete(plan.keyCode); return; } } if (event.key === "Enter" && !event.nativeEvent.isComposing) { event.preventDefault(); sendMobileEnter(); } }} placeholder="Ввод в реальном времени" enterKeyHint="send" inputMode="text" autoCapitalize="none" autoCorrect="off" spellCheck={false} />
			<button type="button" className="remote-keyboard-enter" onClick={sendMobileEnter}>Enter</button>
			<button type="button" className="remote-keyboard-close" onClick={() => setMobileKeyboardVisibility(false)} aria-label="Закрыть клавиатуру"><X size={17} /></button>
		</div>}
		{filesOpen && <section className="remote-files-overlay" aria-label="Файлы удалённого компьютера">
			<header><div><span className="eyebrow">ФАЙЛЫ УСТРОЙСТВА</span><h2>{device.name}</h2></div><button type="button" className="icon-button" onClick={() => setFilesOpen(false)} aria-label="Закрыть файлы"><X size={20} /></button></header>
			<div className="remote-files-overlay-body"><RemoteFiles device={device} csrf={csrf} /></div>
		</section>}
		{sasFeedback && <div className={`desktop-command-feedback ${sasFeedbackError ? "error" : ""}`} role="status">{sasFeedbackError ? <AlertTriangle size={16} /> : <CheckCircle2 size={16} />}{sasFeedback}</div>}
		{error && <div className="desktop-error">{error}</div>}
	</section>;
	return embedded ? workspace : <div className="remote-desktop-backdrop" onMouseDown={(event) => event.target === event.currentTarget && finishRemoteSession()}>{workspace}</div>;
}

type RemoteFileEntry = {
  name: string;
  path: string;
  directory: boolean;
  size: number;
  modifiedAt: string;
};

type RemoteFileList = {
  path: string;
  parent: string;
  entries: RemoteFileEntry[];
};

type LargeFileTransfer = { id: string; status: "uploading" | "queued" | "transferring" | "ready" | "completed" | "failed" | "cancelled" | "expired"; size: number; received: number; error: string };

type ActiveFileTransfer = { id: string; direction: "to_device" | "from_device"; label: string; stage: string; received: number; size: number; startedAt: number };

const REMOTE_USER_DESKTOP_TRANSFER_PATH = "::remoteit-user-desktop::";

async function waitForLargeTransfer(id: string, onProgress: (transfer: LargeFileTransfer) => void, readyStatus: "ready" | "completed", signal?: AbortSignal) {
  for (;;) {
    const transfer = await api<LargeFileTransfer>(`/api/file-transfers/${id}`, { signal }); onProgress(transfer);
    if (transfer.status === readyStatus || (readyStatus === "ready" && transfer.status === "completed")) return transfer;
    if (["failed", "cancelled", "expired"].includes(transfer.status)) throw new Error(transfer.error || "Передача файла прервана");
		await abortableDelay(300, signal);
  }
}

async function uploadLocalFilesToDevice(
	device: Device,
	csrf: string,
	files: File[],
	destinationPath: string,
	onProgress: (progress: ActiveFileTransfer | null) => void,
	signal: AbortSignal,
) {
	for (const file of files) {
		const startedAt = Date.now();
		let transferId = "";
		try {
			const created = await api<{ id: string }>(`/api/devices/${device.id}/file-transfers`, {
				method: "POST",
				body: JSON.stringify({ direction: "to_device", name: file.name, remotePath: destinationPath, size: file.size }),
			}, csrf);
			transferId = created.id;
			onProgress({ id: created.id, direction: "to_device", label: file.name, stage: "Отправка на сервер RemoteIt", received: 0, size: file.size, startedAt });
			let offset = 0;
			while (offset < file.size) {
				const chunkOffset = offset;
				const chunkSize = browserTransferChunkLength(chunkOffset, file.size);
				let uploaded = false;
				let lastError = "";
				for (let attempt = 0; attempt < 5 && !uploaded; attempt += 1) {
					try {
						const chunk = file.slice(chunkOffset, Math.min(file.size, chunkOffset + chunkSize));
						const checkpoint = await uploadTransferChunk(
							`/api/file-transfers/${created.id}/data?offset=${chunkOffset}`,
							chunk,
							csrf,
							signal,
							(sent) => onProgress({ id: created.id, direction: "to_device", label: file.name, stage: "Отправка на сервер RemoteIt", received: chunkOffset + sent, size: file.size, startedAt }),
						);
						offset = validateTransferCheckpoint(checkpoint, chunkOffset, chunk.size, file.size).received;
						uploaded = true;
					} catch (reason) {
						if (isAbortError(reason)) throw reason;
						lastError = reason instanceof Error ? reason.message : "ошибка сети";
						const checkpoint = await api<LargeFileTransfer>(`/api/file-transfers/${created.id}`, { signal }).catch(() => null);
						if (checkpoint) {
							try {
								offset = validateTransferCheckpoint(checkpoint, chunkOffset, Math.min(chunkSize, file.size - chunkOffset), file.size).received;
								uploaded = true;
								break;
							} catch {
								// Retry from the last committed checkpoint.
							}
						}
						await abortableDelay(750 * (attempt + 1), signal);
					}
				}
				if (!uploaded) throw new Error(`Не удалось передать часть файла: ${lastError}`);
			}
			await api(`/api/file-transfers/${created.id}/ready`, { method: "POST", body: "{}", signal }, csrf);
			onProgress({ id: created.id, direction: "to_device", label: file.name, stage: "Сохранение на удалённом компьютере", received: file.size, size: file.size, startedAt });
			await waitForLargeTransfer(created.id, () => undefined, "completed", signal);
			transferId = "";
		} catch (reason) {
			if (transferId) await api(`/api/file-transfers/${transferId}`, { method: "DELETE" }, csrf).catch(() => undefined);
			throw reason;
		}
	}
	onProgress(null);
}

async function waitForDeviceJob(deviceId: string, jobId: string): Promise<AgentJob> {
	const deadline = Date.now() + 100_000;
	let delay = 250;
	while (Date.now() < deadline) {
    const result = await api<{ jobs: AgentJob[] }>(`/api/devices/${deviceId}/jobs`);
    const job = result.jobs.find((item) => item.id === jobId);
    if (job && ["succeeded", "failed", "cancelled", "expired"].includes(job.status)) return job;
		await new Promise((resolve) => window.setTimeout(resolve, delay));
		delay = Math.min(1_000, delay + 125);
  }
  throw new Error("Агент не ответил за 100 секунд");
}

function RemoteFiles({ device, csrf }: { device: Device; csrf: string }) {
	const supportsLargeTransfers = versionAtLeast(device.agentVersion, "0.6.0");
  const [started, setStarted] = useState(false);
  const [path, setPath] = useState("");
  const [parent, setParent] = useState("");
  const [entries, setEntries] = useState<RemoteFileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
	const [dragging, setDragging] = useState(false);
	const [localDragging, setLocalDragging] = useState(false);
	const [remoteDropTarget, setRemoteDropTarget] = useState("");
	const [localFiles, setLocalFiles] = useState<File[]>([]);
  const [activeTransfer, setActiveTransfer] = useState<ActiveFileTransfer | null>(null);
  const uploadInput = useRef<HTMLInputElement>(null);
	const transferController = useRef<AbortController | null>(null);
	const activeTransferRef = useRef<ActiveFileTransfer | null>(null);

	useEffect(() => { activeTransferRef.current = activeTransfer; }, [activeTransfer]);
	useEffect(() => () => transferController.current?.abort(), []);

	useEffect(() => {
		if (device.online) void openPath("");
	}, [device.id, device.online]);

  async function runFileJob(type: "files_list", targetPath: string, extra: Record<string, unknown> = {}) {
    const created = await api<{ id: string }>(`/api/devices/${device.id}/jobs`, {
      method: "POST",
      body: JSON.stringify({ type, path: targetPath, timeoutSeconds: 60, ...extra })
    }, csrf);
    const job = await waitForDeviceJob(device.id, created.id);
    if (job.status !== "succeeded") throw new Error(job.error || "Операция с файлами не выполнена");
    return job.output;
  }

  async function openPath(nextPath: string) {
    setLoading(true); setError(""); setMessage(""); setStarted(true);
    try {
      const output = await runFileJob("files_list", nextPath);
      const result = JSON.parse(output) as RemoteFileList;
      setPath(result.path || ""); setParent(result.parent || ""); setEntries(result.entries || []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось открыть папку");
    } finally { setLoading(false); }
  }

  async function downloadRemoteFile(entry: RemoteFileEntry) {
		if (!supportsLargeTransfers) return setError("Обновите RemoteIt Agent до версии 0.6.0 для передачи файлов до 10 ГБ.");
    if (entry.size > 10 * 1024 * 1024 * 1024) return setError("Размер файла превышает 10 ГБ.");
    setLoading(true); setError("");
    const controller = new AbortController(); transferController.current = controller;
    const startedAt = Date.now(); let transferId = "";
    try {
      const created = await api<{ id: string }>(`/api/devices/${device.id}/file-transfers`, { method: "POST", body: JSON.stringify({ direction: "from_device", name: entry.name, remotePath: entry.path, size: entry.size }) }, csrf);
      transferId = created.id;
      setActiveTransfer({ id: created.id, direction: "from_device", label: entry.name, stage: "Подготовка файла на удалённом компьютере", received: 0, size: entry.size, startedAt });
      await waitForLargeTransfer(created.id, (transfer) => setActiveTransfer({ id: created.id, direction: "from_device", label: entry.name, stage: "Передача с удалённого компьютера", received: transfer.received, size: transfer.size, startedAt }), "ready", controller.signal);
      setActiveTransfer({ id: created.id, direction: "from_device", label: entry.name, stage: "Передача в загрузки этого устройства", received: entry.size, size: entry.size, startedAt });
      const anchor = document.createElement("a");
			anchor.href = `/api/file-transfers/${created.id}/download`;
			anchor.download = entry.name;
			anchor.rel = "noopener";
			anchor.style.display = "none";
			document.body.appendChild(anchor);
			anchor.click();
			window.setTimeout(() => anchor.remove(), 1_000);
      setMessage(`Скачивание «${entry.name}» запущено`);
    } catch (reason) {
      if (transferId) await api(`/api/file-transfers/${transferId}`, { method: "DELETE" }, csrf).catch(() => undefined);
      if (isAbortError(reason)) setMessage("Передача отменена");
      else setError(reason instanceof Error ? reason.message : "Не удалось скачать файл");
    } finally { if (transferController.current === controller) transferController.current = null; setLoading(false); setActiveTransfer(null); }
  }

	async function uploadFiles(files: File[], destinationPath = path) {
		if (!supportsLargeTransfers) return setError("Обновите RemoteIt Agent до версии 0.6.0 для передачи файлов до 10 ГБ.");
		if (!destinationPath) return setError("Сначала откройте папку назначения на удалённом компьютере.");
    const selected = files.filter((file) => file.size <= 10 * 1024 * 1024 * 1024);
    if (selected.length !== files.length) return setError("Размер каждого загружаемого файла не должен превышать 10 ГБ.");
    if (selected.length === 0) return;
    setLoading(true); setError(""); setMessage("");
    const controller = new AbortController(); transferController.current = controller;
    let transferId = "";
    try {
      for (const file of selected) {
        const startedAt = Date.now();
				const created = await api<{ id: string }>(`/api/devices/${device.id}/file-transfers`, { method: "POST", body: JSON.stringify({ direction: "to_device", name: file.name, remotePath: destinationPath, size: file.size }) }, csrf);
        transferId = created.id;
        setActiveTransfer({ id: created.id, direction: "to_device", label: file.name, stage: "Отправка на сервер RemoteIt", received: 0, size: file.size, startedAt });
        // Commit 8 MiB first so the agent starts immediately, then use 64 MiB
        // steady-state chunks to keep fast LAN/Wi-Fi links full with fewer
        // request and fsync round trips.
        let offset = 0;
        while (offset < file.size) {
          const chunkOffset = offset;
          const chunkSize = browserTransferChunkLength(chunkOffset, file.size);
          let uploaded = false; let lastError = "";
          for (let attempt = 0; attempt < 5 && !uploaded; attempt += 1) {
            try {
              const chunk = file.slice(chunkOffset, Math.min(file.size, chunkOffset + chunkSize));
              const progress = await uploadTransferChunk(
                `/api/file-transfers/${created.id}/data?offset=${chunkOffset}`,
                chunk,
                csrf,
                controller.signal,
                (sent) => setActiveTransfer({ id: created.id, direction: "to_device", label: file.name, stage: "Отправка на сервер RemoteIt", received: chunkOffset + sent, size: file.size, startedAt })
              );
              offset = validateTransferCheckpoint(progress, chunkOffset, chunk.size, file.size).received;
              uploaded = true;
            } catch (reason) {
              if (isAbortError(reason)) throw reason;
              lastError = reason instanceof Error ? reason.message : "ошибка сети";
              const checkpoint = await api<LargeFileTransfer>(`/api/file-transfers/${created.id}`, { signal: controller.signal }).catch(() => null);
              if (checkpoint) {
								const expectedSize = Math.min(chunkSize, file.size - chunkOffset);
								try {
									offset = validateTransferCheckpoint(checkpoint, chunkOffset, expectedSize, file.size).received;
									uploaded = true;
									break;
								} catch {
									// The server still reports the previous committed checkpoint.
								}
							}
              await abortableDelay(750 * (attempt + 1), controller.signal);
            }
          }
          if (!uploaded) throw new Error(`Не удалось передать часть файла: ${lastError}`);
          setActiveTransfer({ id: created.id, direction: "to_device", label: file.name, stage: "Отправка на сервер RemoteIt", received: offset, size: file.size, startedAt });
        }
        await api(`/api/file-transfers/${created.id}/ready`, { method: "POST", body: "{}" }, csrf);
        setActiveTransfer({ id: created.id, direction: "to_device", label: file.name, stage: "Сохранение на удалённом компьютере", received: file.size, size: file.size, startedAt });
        await waitForLargeTransfer(created.id, () => undefined, "completed", controller.signal);
        transferId = "";
      }
			setMessage(`Загружено файлов: ${selected.length}${destinationPath !== path ? ` · в папку ${destinationPath}` : ""}`);
		setLocalFiles((current) => current.filter((item) => !selected.some((sent) => sent.name === item.name && sent.size === item.size && sent.lastModified === item.lastModified)));
			const output = await runFileJob("files_list", path);
      const result = JSON.parse(output) as RemoteFileList;
      setParent(result.parent || ""); setEntries(result.entries || []);
    } catch (reason) {
      if (transferId) await api(`/api/file-transfers/${transferId}`, { method: "DELETE" }, csrf).catch(() => undefined);
      if (isAbortError(reason)) setMessage("Передача отменена");
      else setError(reason instanceof Error ? reason.message : "Не удалось загрузить файл");
    } finally { if (transferController.current === controller) transferController.current = null; setLoading(false); setActiveTransfer(null); }
  }

	async function cancelActiveTransfer() {
		const transfer = activeTransferRef.current;
		transferController.current?.abort();
		if (transfer?.id) await api(`/api/file-transfers/${transfer.id}`, { method: "DELETE" }, csrf).catch(() => undefined);
		setActiveTransfer(null); setLoading(false); setMessage("Передача отменена");
	}

	function addLocalFiles(files: File[]) {
		setLocalFiles((current) => {
			const next = [...current];
			for (const file of files) {
				if (!next.some((item) => item.name === file.name && item.size === file.size && item.lastModified === file.lastModified)) next.push(file);
			}
			return next;
		});
	}

	function filesFromDrag(event: ReactDragEvent<HTMLElement>) {
		const droppedFiles = Array.from(event.dataTransfer.files);
		if (droppedFiles.length > 0) return droppedFiles;
		const localKey = event.dataTransfer.getData("application/x-remoteit-local-file");
		const selected = localFiles.find((file) => `${file.name}\u0000${file.size}\u0000${file.lastModified}` === localKey);
		return selected ? [selected] : [];
	}

	function dragCanUpload(event: ReactDragEvent<HTMLElement>) {
		return Array.from(event.dataTransfer.types).some((type) => type === "Files" || type === "application/x-remoteit-local-file");
	}

	function dropOnRemote(event: ReactDragEvent<HTMLDivElement>) {
		event.preventDefault(); setDragging(false);
		const droppedFiles = filesFromDrag(event);
		if (droppedFiles.length > 0) {
			addLocalFiles(droppedFiles);
			void uploadFiles(droppedFiles);
		}
	}

	function dropOnRemoteFolder(event: ReactDragEvent<HTMLDivElement>, destinationPath: string) {
		event.preventDefault(); event.stopPropagation(); setDragging(false); setRemoteDropTarget("");
		const droppedFiles = filesFromDrag(event);
		if (droppedFiles.length === 0) return;
		addLocalFiles(droppedFiles);
		void uploadFiles(droppedFiles, destinationPath);
	}

	function dropOnLocal(event: ReactDragEvent<HTMLDivElement>) {
		event.preventDefault(); setLocalDragging(false);
		const remotePath = event.dataTransfer.getData("application/x-remoteit-remote-file");
		if (remotePath) {
			const entry = entries.find((item) => !item.directory && item.path === remotePath);
			if (entry) void downloadRemoteFile(entry);
			return;
		}
		addLocalFiles(Array.from(event.dataTransfer.files));
	}

  const progress = activeTransfer ? fileTransferProgress(activeTransfer.received, activeTransfer.size, activeTransfer.startedAt) : null;

	return <section className="remote-files-card" aria-busy={loading}>
		<div className="remote-files-head"><div><strong><FolderOpen size={19} /> Передача файлов</strong><small>Два устройства рядом: выберите или перетащите файл между панелями</small></div><span className="remote-files-cap"><ShieldCheck size={14} /> до 10 ГБ</span></div>
		<input ref={uploadInput} className="remote-upload-input" type="file" multiple onChange={(event) => { addLocalFiles(Array.from(event.target.files || [])); event.target.value = ""; }} />
		{!started ? <button className="secondary-button remote-files-open" disabled={!device.online || loading} onClick={() => void openPath("")}><FolderOpen size={17} /> {device.online ? "Открыть проводник" : "Агент не в сети"}</button> : <div className="remote-explorer">
			<section className={`remote-explorer-pane remote-local-pane ${localDragging ? "dragging" : ""}`} onDragEnter={(event) => { event.preventDefault(); setLocalDragging(true); }} onDragOver={(event) => event.preventDefault()} onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setLocalDragging(false); }} onDrop={dropOnLocal}>
				<header><span className="remote-pane-icon"><HardDrive size={17} /></span><span><strong>Моё устройство</strong><small>Файлы, выбранные в браузере</small></span><button type="button" className="remote-pane-action" disabled={loading} onClick={() => uploadInput.current?.click()}><Plus size={15} /> Добавить</button></header>
				<div className="remote-local-list">
					{localFiles.map((file) => { const key = `${file.name}\u0000${file.size}\u0000${file.lastModified}`; return <div className="remote-local-row" key={key} draggable={!loading} onDragStart={(event) => { event.dataTransfer.effectAllowed = "copy"; event.dataTransfer.setData("application/x-remoteit-local-file", key); }}><span className="remote-file-icon"><FileIcon size={17} /></span><span className="remote-file-name"><strong>{file.name}</strong><small>{formatBytes(file.size)} · готов к отправке</small></span><button type="button" className="remote-row-remove" disabled={loading} title="Убрать из списка" onClick={() => setLocalFiles((current) => current.filter((item) => item !== file))}><X size={15} /></button></div>; })}
					{localFiles.length === 0 && <button type="button" className="remote-local-empty" disabled={loading} onClick={() => uploadInput.current?.click()}><Upload size={24} /><strong>Перетащите файлы сюда</strong><small>или выберите их на телефоне/компьютере</small></button>}
				</div>
				<footer><span>{localFiles.length ? `${localFiles.length} выбрано · ${formatBytes(localFiles.reduce((sum, file) => sum + file.size, 0))}` : "Локальные файлы не загружаются без вашего выбора"}</span><button type="button" className="primary-button" disabled={loading || !path || localFiles.length === 0} onClick={() => void uploadFiles(localFiles)}>Отправить на {device.name} <Upload size={15} /></button></footer>
			</section>

			<div className="remote-explorer-divider" aria-hidden="true"><span>⇄</span></div>

			<section className={`remote-explorer-pane remote-device-pane ${dragging ? "dragging" : ""}`} onDragEnter={(event) => { event.preventDefault(); setDragging(true); }} onDragOver={(event) => event.preventDefault()} onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }} onDrop={dropOnRemote}>
				<header><span className="remote-pane-icon"><Monitor size={17} /></span><span><strong>{device.name}</strong><small>Удалённый компьютер</small></span><button type="button" className="remote-pane-action" disabled={loading} onClick={() => void openPath(path)}><RefreshCw size={15} className={loading ? "spin" : ""} /> Обновить</button></header>
				<div className="remote-files-toolbar">
					<button type="button" className="secondary-button remote-parent" disabled={loading || !parent} onClick={() => void openPath(parent)} title="На уровень выше"><ChevronUp size={16} /><span>Выше</span></button>
					<form className="remote-path" onSubmit={(event) => { event.preventDefault(); void openPath(path); }}><input value={path} onChange={(event) => setPath(event.target.value)} placeholder="C:\\ или /home" aria-label="Путь на удалённом компьютере" /><button className="secondary-button" disabled={loading}>Перейти</button></form>
				</div>
				<div className="remote-file-list-head"><span>Имя</span><small>{entries.length} объектов</small></div>
				<div className="remote-file-list">{entries.map((entry) => <div className={`remote-file-row ${entry.directory && remoteDropTarget === entry.path ? "drop-target" : ""}`} key={entry.path} draggable={!loading && !entry.directory && supportsLargeTransfers && entry.size <= 10 * 1024 * 1024 * 1024} onDragStart={(event) => { if (entry.directory) return; event.dataTransfer.effectAllowed = "copy"; event.dataTransfer.setData("application/x-remoteit-remote-file", entry.path); }} onDragEnter={entry.directory ? (event) => { if (dragCanUpload(event)) { event.preventDefault(); event.stopPropagation(); setRemoteDropTarget(entry.path); } } : undefined} onDragOver={entry.directory ? (event) => { if (dragCanUpload(event)) { event.preventDefault(); event.stopPropagation(); event.dataTransfer.dropEffect = "copy"; setRemoteDropTarget(entry.path); } } : undefined} onDragLeave={entry.directory ? (event) => { event.stopPropagation(); if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setRemoteDropTarget((current) => current === entry.path ? "" : current); } : undefined} onDrop={entry.directory ? (event) => dropOnRemoteFolder(event, entry.path) : undefined}><span className="remote-file-icon">{entry.directory ? <Folder size={18} /> : <FileIcon size={18} />}</span><button className="remote-file-name" disabled={loading} onClick={() => entry.directory ? void openPath(entry.path) : void downloadRemoteFile(entry)}><strong>{entry.name}</strong><small>{entry.directory ? remoteDropTarget === entry.path ? "Отпустите, чтобы загрузить в эту папку" : "Папка · можно перетащить файл внутрь" : formatBytes(entry.size)} · {new Date(entry.modifiedAt).toLocaleString("ru-RU")}</small></button>{entry.directory ? <button className="remote-download" disabled={loading} title="Открыть папку" onClick={() => void openPath(entry.path)}><ChevronDown size={16} className="remote-open-folder-icon" /></button> : <button className="remote-download" disabled={loading || !supportsLargeTransfers || entry.size > 10 * 1024 * 1024 * 1024} title={!supportsLargeTransfers ? "Обновите Agent" : entry.size > 10 * 1024 * 1024 * 1024 ? "Файл превышает 10 ГБ" : "Скачать на моё устройство"} onClick={() => void downloadRemoteFile(entry)}><Download size={16} /><span>Скачать</span></button>}</div>)}</div>
				{!loading && entries.length === 0 && !error && <div className="remote-files-empty">Папка пуста</div>}
				<div className="remote-device-drop-hint"><Upload size={15} /> Перетащите сюда, чтобы отправить в текущую папку</div>
			</section>
		</div>}
    {activeTransfer && progress ? <div className="remote-transfer-progress" role="status">
		<div className="remote-transfer-summary"><span className="remote-transfer-direction">{activeTransfer.direction === "to_device" ? <Upload size={16} /> : <Download size={16} />}</span><span><strong>{activeTransfer.stage}</strong><small>{activeTransfer.label} · {formatBytes(progress.received)} из {formatBytes(progress.total)}</small></span><b>{Math.round(progress.percent)}%</b></div>
		<div className="remote-transfer-bar" aria-label={`Передано ${Math.round(progress.percent)}%`}><span style={{ width: `${progress.percent}%` }} /></div>
		<div className="remote-transfer-meta"><span>{progress.bytesPerSecond > 0 ? `${formatBytes(progress.bytesPerSecond)}/с` : "Расчёт скорости…"}</span><span>{progress.remainingSeconds != null ? `Осталось ≈ ${formatTransferDuration(progress.remainingSeconds)}` : "Завершение…"}</span><button type="button" onClick={() => void cancelActiveTransfer()}><X size={14} /> Отменить</button></div>
	</div> : loading && <div className="remote-files-loading"><RefreshCw className="spin" size={16} /> Ожидание ответа Agent…</div>}
    {message && <div className="form-success">{message}</div>}
    {error && <div className="form-error">{error}</div>}
		<small className="remote-files-limit"><ShieldCheck size={14} /> Передача идёт потоковыми частями, поддерживает отмену и автоматически продолжается после краткого обрыва. Файл до 10 ГБ не хранится целиком в памяти.</small>
  </section>;
}

function jobStatusLabel(status: AgentJob["status"]) {
  return ({ queued: "В очереди", running: "Выполняется", succeeded: "Готово", failed: "Ошибка", cancelled: "Отменено", expired: "Истекло" })[status];
}

function formatBytes(value: number) {
  if (!value) return "0 Б";
  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / Math.pow(1024, index)).toFixed(index > 2 ? 1 : 0)} ${units[index]}`;
}

function formatUptime(value: number) {
	const seconds = Math.max(0, Math.floor(value));
	const days = Math.floor(seconds / 86400);
	const hours = Math.floor(seconds % 86400 / 3600);
	const minutes = Math.floor(seconds % 3600 / 60);
	if (days) return `${days} дн. · ${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`;
	return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`;
}

function versionAtLeast(actual: string, required: string) {
  const left = actual.split(".").map((part) => Number(part) || 0); const right = required.split(".").map((part) => Number(part) || 0);
  for (let index = 0; index < Math.max(left.length, right.length); index += 1) { if ((left[index] || 0) !== (right[index] || 0)) return (left[index] || 0) > (right[index] || 0); }
  return true;
}

function downloadText(filename: string, content: string) {
	const url = URL.createObjectURL(new Blob([content], { type: "text/plain;charset=utf-8" }));
	const anchor = document.createElement("a");
	anchor.href = url; anchor.download = filename.replace(/[^\p{L}\p{N}._-]+/gu, "-"); anchor.click();
	window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function downloadBoundWindowsInstaller(tokenId: string) {
  if (!/^[0-9a-f-]{36}$/i.test(tokenId)) {
    window.alert("Некорректный идентификатор токена");
    return;
  }
  const anchor = document.createElement("a");
  anchor.href = `/api/enrollment-tokens/${encodeURIComponent(tokenId)}/windows-agent`;
  anchor.download = "RemoteIt-Agent.exe";
  anchor.click();
}

function downloadBoundUnixInstaller(tokenId: string) {
  if (!/^[0-9a-f-]{36}$/i.test(tokenId)) {
    window.alert("Некорректный идентификатор токена");
    return;
  }
  const anchor = document.createElement("a");
  anchor.href = `/api/enrollment-tokens/${encodeURIComponent(tokenId)}/unix-agent`;
  anchor.download = "RemoteIt-Agent-Setup.sh";
  anchor.click();
}

function unixInstallCommand(token: string) {
	const url = "https://supportgenesis.ru/downloads/install-remoteit.sh";
	return `if command -v curl >/dev/null 2>&1; then curl -fsSL "${url}" -o /tmp/install-remoteit.sh; elif command -v wget >/dev/null 2>&1; then wget -qO /tmp/install-remoteit.sh "${url}"; elif command -v apt-get >/dev/null 2>&1; then if [ "$(id -u)" -eq 0 ]; then apt-get update && apt-get install -y ca-certificates curl; elif command -v sudo >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y ca-certificates curl; else echo "Нужны root-права для установки curl" >&2; exit 1; fi && curl -fsSL "${url}" -o /tmp/install-remoteit.sh; else echo "Для установки нужен curl или wget" >&2; exit 1; fi && sh /tmp/install-remoteit.sh --token "${token}"`;
}

const auditLabels: Record<string, string> = {
  "auth.login": "Вход в систему",
  "auth.login_failed": "Неудачная попытка входа",
  "auth.logout": "Выход из системы",
  "auth.password_changed": "Пароль изменён",
  "auth.session_revoked": "Сессия завершена",
  "user.created": "Пользователь создан",
  "user.updated": "Права пользователя изменены",
	"user.password_reset": "Пароль пользователя сброшен",
  "enrollment.created": "Токен установки создан",
	"enrollment.updated": "Токен установки изменён",
	"enrollment.deleted": "Токен установки удалён",
  "device.enrolled": "Устройство зарегистрировано",
	"enrollment.agent_downloaded": "Скачан готовый Windows Agent",
	"enrollment.public_agent_downloaded": "Agent скачан по публичному коду",
	"device.renamed": "Устройство переименовано",
	"device.updated": "Устройство изменено",
	"device.deleted": "Устройство удалено",
	"device.forgotten": "Устройство удалено только из панели",
	"device.uninstall_requested": "Запрошено удаление агента",
	"device.uninstall_scheduled": "Локальное удаление агента запущено",
	"device.uninstall_failed": "Не удалось полностью удалить агент",
	"device.uninstalled": "Агент удалён с компьютера",
  "agent_job.created": "Задание создано",
  "agent_job.completed": "Задание завершено",
	"agent_job.cancelled": "Задание отменено",
	"ai.analysis_requested": "Запрошен AI-анализ устройства",
  "file_transfer.created": "Передача файла создана"
};

function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
	const [query, setQuery] = useState("");
	const [typeFilter, setTypeFilter] = useState("all");
	const [actorFilter, setActorFilter] = useState("all");
	const [targetFilter, setTargetFilter] = useState("all");
  const load = useCallback(async () => {
    try {
      const result = await api<{ events: AuditEvent[] }>("/api/audit");
      setEvents(result.events); setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось загрузить журнал");
    } finally { setLoading(false); }
  }, []);
  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [load]);
	const actors = Array.from(new Set(events.map((event) => event.actor || "Агент"))).sort((left, right) => left.localeCompare(right, "ru"));
	const targets = Array.from(new Set(events.map((event) => event.targetType).filter(Boolean))).sort((left, right) => left.localeCompare(right, "ru"));
	const categories = Array.from(new Set(events.map((event) => event.eventType.split(".")[0]))).sort((left, right) => left.localeCompare(right, "ru"));
	const visible = events.filter((event) => {
		const text = query.trim().toLowerCase();
		const details = JSON.stringify(event.details || {});
		return (!text || [event.eventType, auditLabels[event.eventType], event.actor, event.targetType, event.targetId, event.ip, details].some((value) => value?.toLowerCase().includes(text))) &&
			(typeFilter === "all" || event.eventType.startsWith(`${typeFilter}.`)) &&
			(actorFilter === "all" || (event.actor || "Агент") === actorFilter) &&
			(targetFilter === "all" || event.targetType === targetFilter);
	});
	const eventState = (event: AuditEvent) => {
		const value = event.eventType.toLowerCase();
		if (["failed", "error", "denied", "invalid"].some((part) => value.includes(part))) return { label: "Ошибка", tone: "error" };
		if (["created", "updated", "enrolled", "issued"].some((part) => value.includes(part))) return { label: "Информация", tone: "info" };
		return { label: "Успешно", tone: "success" };
	};
  return <>
    <section className="page-heading"><div><span className="eyebrow">БЕЗОПАСНОСТЬ И КОНТРОЛЬ</span><h1>Журнал действий</h1><p>Входы, изменения прав, регистрация устройств и административные задания.</p></div><button className="secondary-button" onClick={() => void load()}><RefreshCw size={17} className={loading ? "spin" : ""} /> Обновить</button></section>
    <section className="device-panel audit-panel"><div className="audit-toolbar"><label><span>Тип события</span><select value={typeFilter} onChange={(event) => setTypeFilter(event.target.value)}><option value="all">Все типы</option>{categories.map((item) => <option value={item} key={item}>{item}</option>)}</select></label><label><span>Пользователь</span><select value={actorFilter} onChange={(event) => setActorFilter(event.target.value)}><option value="all">Все пользователи</option>{actors.map((item) => <option value={item} key={item}>{item}</option>)}</select></label><label><span>Объект</span><select value={targetFilter} onChange={(event) => setTargetFilter(event.target.value)}><option value="all">Все объекты</option>{targets.map((item) => <option value={item} key={item}>{item}</option>)}</select></label><label className="panel-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск по событию, ID, IP или объекту" /></label><button className="secondary-button" onClick={() => { setQuery(""); setTypeFilter("all"); setActorFilter("all"); setTargetFilter("all"); }}><RefreshCw size={15} /> Сбросить</button></div><div className="audit-result-line"><span>Найдено {visible.length} событий</span><span className="audit-retention"><ShieldCheck size={15} /> Команды не записываются в детали аудита</span></div>{error && <div className="panel-error">{error}</div>}<div className="table-wrap"><table className="audit-table audit-table-premium"><thead><tr><th>Время</th><th>Событие</th><th>Пользователь</th><th>Объект</th><th>Подробности</th><th>IP-адрес</th><th>Статус</th></tr></thead><tbody>{loading && events.length === 0 ? <LoadingRows /> : visible.map((event) => { const state = eventState(event); const details = Object.keys(event.details || {}).length ? JSON.stringify(event.details) : "без дополнительных данных"; return <tr key={event.id}><td><div className="stacked"><strong>{new Date(event.createdAt).toLocaleDateString("ru-RU")}</strong><small>{new Date(event.createdAt).toLocaleTimeString("ru-RU")}</small></div></td><td><div className="audit-event"><span className={`audit-dot ${state.tone}`} /><div><strong>{auditLabels[event.eventType] || event.eventType}</strong><small>{event.eventType}</small></div></div></td><td><div className="stacked"><strong>{event.actor || "Агент"}</strong><small>{event.actor ? "пользователь панели" : "системный агент"}</small></div></td><td><div className="stacked"><strong>{event.targetType || "—"}</strong><small>{event.targetId ? event.targetId.slice(0, 14) : "—"}</small></div></td><td><span className="audit-details" title={details}>{details}</span></td><td><code>{event.ip || "—"}</code></td><td><span className={`audit-status ${state.tone}`}><span />{state.label}</span></td></tr>; })}</tbody></table>{!loading && visible.length === 0 && <div className="empty-state"><Clock3 size={28} /><h3>{events.length ? "События не найдены" : "Журнал пока пуст"}</h3><p>{events.length ? "Измените фильтры или строку поиска." : "Значимые действия появятся здесь автоматически."}</p></div>}</div><div className="compact-list-footer"><span>Показано {visible.length} из {events.length}</span><small>Автоматическое обновление каждые 15 секунд</small></div></section>
  </>;
}

const actionNames: Record<string, string> = {
  "diagnostic.system": "Диагностика системы",
  "diagnostic.network": "Диагностика сети",
  "diagnostic.services": "Диагностика служб",
  "service.restart": "Перезапуск службы",
  "process.terminate": "Завершение процесса",
  "file.download": "Безопасное скачивание файла",
  "package.install": "Установка пакета",
  "local.group.add_member": "Добавление в локальную группу",
  "windows.vpn.upsert": "Настройка VPN Windows",
  "system.reboot": "Перезагрузка компьютера",
  "script.execute": "Проверенный сценарий"
};

const actionStates: Record<ActionJob["status"], { label: string; tone: string }> = {
  awaiting_approval: { label: "Ждёт подтверждения", tone: "warning" },
  queued: { label: "В очереди", tone: "info" },
  running: { label: "Выполняется", tone: "info" },
  succeeded: { label: "Выполнено", tone: "success" },
  failed: { label: "Ошибка", tone: "error" },
  cancelled: { label: "Отменено", tone: "muted" },
  expired: { label: "Истекло", tone: "muted" }
};

function CodexIntegrationPanel({ currentUser, csrf }: { currentUser: User; csrf: string }) {
  const [tokens, setTokens] = useState<IntegrationToken[]>([]);
  const [actions, setActions] = useState<ActionJob[]>([]);
  const [tokenName, setTokenName] = useState("Codex на основном компьютере");
  const [expiresDays, setExpiresDays] = useState(90);
  const [newToken, setNewToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const canManageIntegrations = currentUser.role === "owner" || currentUser.role === "admin";
  const canReviewActions = currentUser.role === "owner" || currentUser.role === "admin";

  const load = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const requests: Promise<unknown>[] = [];
      if (canManageIntegrations) requests.push(api<{ tokens: IntegrationToken[] }>("/api/integration-tokens").then((result) => setTokens(result.tokens)));
      if (canReviewActions) requests.push(api<{ actions: ActionJob[] }>("/api/action-jobs").then((result) => setActions(result.actions)));
      await Promise.all(requests);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось загрузить интеграцию Codex");
    } finally {
      if (!quiet) setLoading(false);
    }
  }, [canManageIntegrations, canReviewActions]);

  useEffect(() => {
    void load();
    if (!canReviewActions) return;
    const timer = window.setInterval(() => void load(true), 5000);
    return () => window.clearInterval(timer);
  }, [canReviewActions, load]);

  async function requestIntegrationToken() {
    return api<{ id: string; token: string; expiresAt: string }>("/api/integration-tokens", { method: "POST", body: JSON.stringify({ name: tokenName, expiresDays }) }, csrf);
  }

  async function createToken(event: FormEvent) {
    event.preventDefault();
    setWorking("create-token"); setError(""); setMessage(""); setNewToken("");
    try {
      const result = await requestIntegrationToken();
      setNewToken(result.token);
      setMessage("Токен создан. Для ручной настройки он показывается только сейчас.");
      await load(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось создать токен интеграции");
    } finally { setWorking(""); }
  }

  async function downloadReadyWindowsSetup() {
    setWorking("ready-windows"); setError(""); setMessage(""); setNewToken("");
    try {
      const result = await requestIntegrationToken();
      const installer = buildWindowsMCPInstaller(window.location.origin, result.token);
      downloadText("RemoteIt-AI-Administrator-Setup.cmd", installer);
      setNewToken(result.token);
      setMessage("Готовое персональное подключение скачано. Запустите файл двойным кликом на компьютере с Codex — команды и токен вставлять не нужно.");
      await load(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось подготовить готовое подключение AI-администратора");
    } finally { setWorking(""); }
  }

  function downloadCodexInstruction() {
    try {
      downloadText("RemoteIt-инструкция-для-Codex.txt", buildCodexOperatorInstruction(window.location.origin));
      setError("");
      setMessage("Инструкция для другого Codex скачана. В ней нет токена или пароля.");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось подготовить инструкцию");
    }
  }

  async function revokeToken(id: string) {
    if (!window.confirm("Отозвать эту интеграцию? Codex сразу потеряет доступ к RemoteIt.")) return;
    setWorking(id); setError(""); setMessage("");
    try {
      await api(`/api/integration-tokens/${id}`, { method: "DELETE" }, csrf);
      setMessage("Интеграция отозвана.");
      await load(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось отозвать интеграцию");
    } finally { setWorking(""); }
  }

  async function updateAction(id: string, operation: "approve" | "cancel") {
    setWorking(id); setError(""); setMessage("");
    try {
      await api(`/api/action-jobs/${id}/${operation}`, { method: "POST" }, csrf);
      setMessage(operation === "approve" ? "Действие подтверждено и поставлено в защищённую очередь." : "Действие отменено.");
      await load(true);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось изменить действие");
    } finally { setWorking(""); }
  }

  async function copyToken() {
    if (!newToken) return;
    try {
      await navigator.clipboard.writeText(newToken);
      setMessage("Токен скопирован. Не пересылайте его в чаты и не сохраняйте в репозитории.");
    } catch { setError("Не удалось скопировать токен — выделите его вручную."); }
  }

  const pendingCount = actions.filter((item) => item.status === "awaiting_approval").length;
  const setupCommand = newToken ? `codex mcp add remoteit --env REMOTEIT_URL="${window.location.origin}" --env REMOTEIT_INTEGRATION_TOKEN="${newToken}" -- "%LOCALAPPDATA%\\RemoteIt\\MCP\\RemoteIt-MCP.exe"` : "";

  if (!canManageIntegrations && !canReviewActions) return null;
  return <section className="codex-integration-card">
    <div className="codex-integration-head">
      <span className="codex-mark"><Sparkles size={21} /></span>
      <div><span className="eyebrow">AI И АВТОМАТИЗАЦИЯ</span><h2>AI-администратор</h2><p>Подключите Codex одним готовым файлом. Он сможет находить устройства по Remote ID, проводить диагностику и создавать подписанные административные задания.</p></div>
      <div className="codex-security-badge"><ShieldCheck size={17} /><span>Только владелец и администраторы<strong>критические действия подтверждает владелец</strong></span></div>
    </div>
    {(error || message) && <div className={error ? "panel-error codex-feedback" : "settings-success codex-feedback"}>{error || message}</div>}
    <div className="codex-integration-grid">
      {canManageIntegrations && <article className="codex-token-panel">
        <div className="codex-section-head"><div><h3>Подключить Codex</h3><p>Персональный установщик сам скачает MCP, проверит SHA-256 и зарегистрирует RemoteIt. Копировать команды или токен не потребуется.</p></div><span>{tokens.filter((item) => !item.revokedAt && new Date(item.expiresAt).getTime() > Date.now()).length} активных</span></div>
        <div className="codex-ready-setup"><span className="codex-ready-icon"><Sparkles size={22} /></span><div><strong>Готовое подключение для Windows</strong><small>Один файл · без ручной вставки · удаляет себя после успешной установки</small></div><button className="primary-button" type="button" disabled={Boolean(working)} onClick={() => void downloadReadyWindowsSetup()}>{working === "ready-windows" ? <RefreshCw size={16} className="spin" /> : <Download size={16} />} Скачать и подключить</button></div>
        <div className="codex-setup-options"><label><span>Название подключения</span><input value={tokenName} onChange={(event) => setTokenName(event.target.value)} maxLength={100} required /></label><label><span>Срок доступа, дней</span><input type="number" min={1} max={365} value={expiresDays} onChange={(event) => setExpiresDays(Number(event.target.value))} required /></label></div>
		<div className="codex-guide-actions"><button className="secondary-button" type="button" onClick={downloadCodexInstruction}><FileCode2 size={15} /> Инструкция для другого Codex</button><a className="secondary-button" href="/downloads/SHA256SUMS.txt" download><ShieldCheck size={15} /> SHA-256</a></div>
        <details className="codex-manual-setup"><summary>Ручная настройка и Linux</summary><form className="codex-token-form" onSubmit={createToken}><label><span>Название</span><input value={tokenName} onChange={(event) => setTokenName(event.target.value)} maxLength={100} required /></label><label><span>Срок, дней</span><input type="number" min={1} max={365} value={expiresDays} onChange={(event) => setExpiresDays(Number(event.target.value))} required /></label><button className="primary-button" disabled={working === "create-token"}>{working === "create-token" ? <RefreshCw size={16} className="spin" /> : <Plus size={16} />} Создать токен</button></form><div className="codex-downloads"><a href="/downloads/remoteit-mcp-linux-amd64" download><Download size={16} /> MCP для Linux</a></div></details>
        {newToken && <div className="codex-new-token"><div><span>РЕЗЕРВНАЯ РУЧНАЯ НАСТРОЙКА · ПОКАЗЫВАЕТСЯ ОДИН РАЗ</span><code>{newToken}</code></div><button className="secondary-button" onClick={() => void copyToken()}><Copy size={15} /> Копировать</button><details><summary>Показать команду</summary><code>{setupCommand}</code><p>Используйте только если готовый установщик не смог зарегистрировать MCP автоматически.</p></details></div>}
        <div className="integration-token-list">{loading && tokens.length === 0 ? <PanelLoader /> : tokens.map((item) => { const active = !item.revokedAt && new Date(item.expiresAt).getTime() > Date.now(); return <div className="integration-token-row" key={item.id}><span className={`integration-state ${active ? "active" : "revoked"}`}><Link2 size={15} /></span><div><strong>{item.name}</strong><small>до {new Date(item.expiresAt).toLocaleString("ru-RU")} · {item.lastUsedAt ? `использован ${formatRelative(item.lastUsedAt)}` : "ещё не использован"}</small></div><span className={`action-status ${active ? "success" : "muted"}`}>{active ? "Активен" : "Отозван"}</span>{active && <button className="danger-button compact-action" disabled={working === item.id} onClick={() => void revokeToken(item.id)}><Ban size={13} /> Отозвать</button>}</div>; })}</div>
      </article>}
      {canReviewActions && <article className="codex-actions-panel">
        <div className="codex-section-head"><div><h3>Задания Codex</h3><p>Последние запросы, подтверждения и результаты на удалённых устройствах.</p></div><span className={pendingCount ? "needs-action" : ""}>{pendingCount ? `${pendingCount} ждут решения` : "нет ожидающих"}</span><button className="icon-button" onClick={() => void load()} aria-label="Обновить задания"><RefreshCw size={16} className={loading ? "spin" : ""} /></button></div>
        <div className="action-job-list">{loading && actions.length === 0 ? <PanelLoader /> : actions.slice(0, 30).map((item) => { const state = actionStates[item.status]; const canCancel = item.status === "awaiting_approval" || item.status === "queued"; const canApprove = item.status === "awaiting_approval" && (item.risk !== "critical" || currentUser.role === "owner"); return <details className={`action-job action-${state.tone}`} key={item.id} open={item.status === "awaiting_approval"}><summary><span className={`action-risk risk-${item.risk}`}>{item.risk === "read" ? <Eye size={16} /> : <AlertTriangle size={16} />}</span><div><strong>{actionNames[item.action] || item.plan?.title || item.action}</strong><small>{item.deviceName} · ID {item.remoteId} · {item.requestedVia === "mcp" ? "запрос Codex" : "из панели"}</small></div><span className={`action-status ${state.tone}`}>{state.label}</span><ChevronDown size={16} /></summary><div className="action-job-body"><p>{item.plan?.description || "Описание задания недоступно."}</p>{item.plan?.steps?.length ? <ol>{item.plan.steps.map((step) => <li key={step}>{step}</li>)}</ol> : null}<div className="action-parameters"><strong>Точные параметры перед подтверждением</strong><pre>{JSON.stringify(item.parameters || {}, null, 2)}</pre></div>{item.plan?.rollback && <div className="action-rollback"><ShieldCheck size={14} /><span><strong>План отката</strong>{item.plan.rollback}</span></div>}<div className="action-job-meta"><span>Создано: {new Date(item.createdAt).toLocaleString("ru-RU")}</span><span>Хеш: <code>{item.requestHash.slice(0, 16)}…</code></span>{item.approvalRequired && <span>Требуется ручное подтверждение</span>}{item.risk === "critical" && currentUser.role !== "owner" && <span>Критическое действие подтверждает только владелец</span>}</div>{(item.output || item.error) && <pre className={item.error ? "action-output error" : "action-output"}>{item.error || item.output}</pre>}<div className="action-job-buttons">{canApprove && <button className="primary-button" disabled={working === item.id} onClick={() => void updateAction(item.id, "approve")}><ShieldCheck size={15} /> Проверил параметры — выполнить</button>}{canCancel && <button className="danger-button" disabled={working === item.id} onClick={() => void updateAction(item.id, "cancel")}><Ban size={15} /> Отменить</button>}</div></div></details>; })}{!loading && actions.length === 0 && <div className="codex-empty"><CheckCircle2 size={25} /><strong>Заданий пока нет</strong><span>Они появятся после запроса Codex через RemoteIt MCP.</span></div>}</div>
      </article>}
    </div>
  </section>;
}

function downloadAndroidAgent(token?: string) {
	if (token) void navigator.clipboard?.writeText(token).catch(() => undefined);
	const anchor = document.createElement("a");
	anchor.href = "/downloads/RemoteIt-Agent-Android.apk";
	anchor.download = "RemoteIt-Agent-Android.apk";
	anchor.click();
}

function formatTransferDuration(value: number) {
	const seconds = Math.max(0, Math.round(value));
	if (seconds < 60) return `${seconds} сек.`;
	const minutes = Math.floor(seconds / 60);
	const rest = seconds % 60;
	if (minutes < 60) return `${minutes} мин. ${rest ? `${rest} сек.` : ""}`.trim();
	return `${Math.floor(minutes / 60)} ч. ${minutes % 60} мин.`;
}

function SettingsPage({ currentUser, csrf, theme, onTheme }: { currentUser: User; csrf: string; theme: Theme; onTheme: (theme: Theme) => void }) {
	const [settingsTab, setSettingsTab] = useState<"profile" | "security" | "appearance" | "sessions">("profile");
  const [sessions, setSessions] = useState<AuthSession[]>([]);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [sessionsLoading, setSessionsLoading] = useState(true);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const loadSessions = useCallback(async () => {
    setSessionsLoading(true);
    try {
      const result = await api<{ sessions: AuthSession[] }>("/api/auth/sessions");
      setSessions(result.sessions);
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось загрузить сессии");
    } finally {
      setSessionsLoading(false);
    }
  }, []);

  useEffect(() => { void loadSessions(); }, [loadSessions]);

  async function changePassword(event: FormEvent) {
    event.preventDefault();
    if (newPassword !== confirmPassword) return setError("Новые пароли не совпадают");
    setLoading(true); setError(""); setMessage("");
    try {
      await api("/api/auth/change-password", { method: "POST", body: JSON.stringify({ currentPassword, newPassword }) }, csrf);
      setCurrentPassword(""); setNewPassword(""); setConfirmPassword("");
      setMessage("Пароль изменён. Остальные сессии аккаунта завершены.");
      await loadSessions();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось изменить пароль");
    } finally {
      setLoading(false);
    }
  }

  async function revokeSession(id: string) {
    setError(""); setMessage("");
    try {
      await api(`/api/auth/sessions/${id}`, { method: "DELETE" }, csrf);
      setMessage("Сессия завершена.");
      await loadSessions();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось завершить сессию");
    }
  }

  const roleLabel = currentUser.role === "owner" ? "Владелец" : currentUser.role === "admin" ? "Администратор" : currentUser.role === "technician" ? "Техник" : "Наблюдатель";
  return <>
    <section className="page-heading settings-page-heading"><div><span className="eyebrow">АККАУНТ И БЕЗОПАСНОСТЬ</span><h1>Настройки</h1><p>Управляйте профилем, защитой, оформлением и активными входами аккаунта {currentUser.username}.</p></div><span className="settings-heading-badge"><ShieldCheck size={17} /><span><strong>{roleLabel}</strong><small>защищённый аккаунт</small></span></span></section>
	<section className="settings-tabs" role="tablist" aria-label="Разделы настроек">
		<button id="settings-tab-profile" type="button" role="tab" aria-controls="settings-panel-profile" aria-selected={settingsTab === "profile"} className={settingsTab === "profile" ? "active" : ""} onClick={() => setSettingsTab("profile")}><span className="settings-tab-icon"><CircleUserRound size={18} /></span><span className="settings-tab-copy"><strong>Профиль</strong><small>Аккаунт и приложения</small></span></button>
		<button id="settings-tab-security" type="button" role="tab" aria-controls="settings-panel-security" aria-selected={settingsTab === "security"} className={settingsTab === "security" ? "active" : ""} onClick={() => setSettingsTab("security")}><span className="settings-tab-icon"><LockKeyhole size={18} /></span><span className="settings-tab-copy"><strong>Безопасность</strong><small>Пароль и защита</small></span></button>
		<button id="settings-tab-appearance" type="button" role="tab" aria-controls="settings-panel-appearance" aria-selected={settingsTab === "appearance"} className={settingsTab === "appearance" ? "active" : ""} onClick={() => setSettingsTab("appearance")}><span className="settings-tab-icon"><Palette size={18} /></span><span className="settings-tab-copy"><strong>Внешний вид</strong><small>Тема интерфейса</small></span></button>
		<button id="settings-tab-sessions" type="button" role="tab" aria-controls="settings-panel-sessions" aria-selected={settingsTab === "sessions"} className={settingsTab === "sessions" ? "active" : ""} onClick={() => setSettingsTab("sessions")}><span className="settings-tab-icon"><Monitor size={18} /></span><span className="settings-tab-copy"><strong>Сессии</strong><small>Активные входы</small></span></button>
	</section>
    {(error || message) && <div className={error ? "panel-error settings-feedback" : "settings-success"}>{error || message}</div>}
    <section id={`settings-panel-${settingsTab}`} className="settings-grid" role="tabpanel" aria-labelledby={`settings-tab-${settingsTab}`}>
		{settingsTab === "profile" && <>
			<article className="settings-card account-card"><div className="settings-card-head"><span className="stat-icon blue"><CircleUserRound size={20} /></span><div><h2>Профиль аккаунта</h2><p>Ваши текущие права в RemoteIt</p></div></div><div className="account-summary"><span className="avatar">{currentUser.username.slice(0, 1).toUpperCase()}</span><div><strong>{currentUser.username}</strong><small>{roleLabel}</small></div></div><div className="account-facts"><span><small>Права доступа</small><strong>{roleLabel}</strong></span><span><small>Имя входа</small><strong>@{currentUser.username}</strong></span><span><small>Защита</small><strong><ShieldCheck size={13} /> Включена</strong></span></div></article>
			<article className="settings-card downloads-card"><div className="settings-card-head"><span className="stat-icon amber"><Download size={20} /></span><div><h2>Приложения</h2><p>Проверенные сборки с сервера RemoteIt</p></div></div><div className="settings-downloads"><a href="/downloads/RemoteIt-Console.exe" download><Monitor size={16} /> RemoteIt Console</a><a href="/downloads/RemoteIt.apk" download><Download size={16} /> Android администратора</a><a href="/downloads/RemoteIt-Agent-Android.apk" download><DeviceOSIcon os="Android" size={16} /> Android Agent пользователя</a><a href="/downloads/RemoteIt-Agent-Setup.exe" download><Download size={16} /> Windows Agent</a><a href="/downloads/install-remoteit.sh" download><Download size={16} /> Ubuntu / macOS</a>{(currentUser.role === "owner" || currentUser.role === "admin") && <a href="/downloads/SHA256SUMS.txt" download><ShieldCheck size={16} /> SHA-256 суммы</a>}</div></article>
			<article className="settings-card creator-card"><div className="settings-card-head"><span className="stat-icon green"><Send size={20} /></span><div><h2>О RemoteIt</h2><p>Частная платформа удалённого администрирования</p></div></div><a className="creator-link" href="https://t.me/Sanchcz" target="_blank" rel="noreferrer"><span><small>Создатель</small><strong>@Sanchcz</strong></span><span>Telegram</span></a></article>
			<div className="settings-codex-slot"><CodexIntegrationPanel currentUser={currentUser} csrf={csrf} /></div>
		</>}
		{settingsTab === "security" && <article className="settings-card settings-card-wide"><div className="settings-card-head"><span className="stat-icon green"><KeyRound size={20} /></span><div><h2>Изменить пароль</h2><p>От 4 символов, без обязательных спецсимволов</p></div></div><form className="settings-form" onSubmit={changePassword}><label><span>Текущий пароль</span><input type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required /></label><label><span>Новый пароль</span><input type="password" autoComplete="new-password" minLength={4} maxLength={256} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required /></label><label><span>Повторите новый пароль</span><input type="password" autoComplete="new-password" minLength={4} maxLength={256} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required /></label><button className="primary-button" disabled={loading}>{loading ? <RefreshCw size={17} className="spin" /> : <ShieldCheck size={17} />} Сохранить пароль</button></form></article>}
		{settingsTab === "appearance" && <article className="settings-card appearance-card settings-card-wide"><div className="settings-card-head"><span className="stat-icon violet"><Palette size={20} /></span><div><h2>Внешний вид</h2><p>Белая тема используется по умолчанию</p></div></div><div className="theme-setting"><span>Оформление панели</span><ThemeSwitcher theme={theme} onChange={onTheme} /></div><small className="appearance-note">Выбор сохраняется только для этого браузера и не меняет тему у других администраторов.</small></article>}
		{settingsTab === "sessions" && <article className="settings-card sessions-card settings-card-wide"><div className="settings-card-head"><span className="stat-icon violet"><Activity size={20} /></span><div><h2>Активные сессии</h2><p>{sessions.length} входов, срок каждой — 12 часов</p></div><button className="icon-button" onClick={() => void loadSessions()} aria-label="Обновить сессии"><RefreshCw size={17} className={sessionsLoading ? "spin" : ""} /></button></div><div className="sessions-list">{sessionsLoading && sessions.length === 0 ? <div className="session-placeholder">Загрузка…</div> : sessions.map((session) => <div className="session-row" key={session.id}><span className={`session-state ${session.current ? "current" : ""}`}><Monitor size={17} /></span><div><strong>{session.current ? "Текущая сессия" : session.userAgent || "Неизвестное устройство"}</strong><small>{session.ip || "IP неизвестен"} · активность {formatRelative(session.lastUsedAt)}</small></div>{session.current ? <span className="current-badge">эта сессия</span> : <button className="danger-button compact-action" onClick={() => void revokeSession(session.id)}><Ban size={14} /> Завершить</button>}</div>)}</div></article>}
    </section>
  </>;
}

function UsersPage({ currentUser, csrf }: { currentUser: User; csrf: string }) {
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [createOpen, setCreateOpen] = useState(false);
	const [resetTarget, setResetTarget] = useState<ManagedUser | null>(null);
	const [query, setQuery] = useState("");
	const [roleFilter, setRoleFilter] = useState("all");

  const loadUsers = useCallback(async () => {
    setLoading(true);
    try {
      const result = await api<{ users: ManagedUser[] }>("/api/users");
      setUsers(result.users);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось загрузить пользователей");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadUsers(); }, [loadUsers]);

  async function updateUser(target: ManagedUser, role: string, disabled: boolean) {
    try {
      await api(`/api/users/${target.id}`, { method: "PATCH", body: JSON.stringify({ role, disabled }) }, csrf);
      await loadUsers();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось обновить пользователя");
    }
  }

  const active = users.filter((item) => !item.disabled).length;
  const admins = users.filter((item) => !item.disabled && ["owner", "admin"].includes(item.role)).length;
	const owners = users.filter((item) => item.role === "owner").length;
	const viewers = users.filter((item) => item.role === "viewer").length;
	const visibleUsers = users.filter((item) => {
		const text = query.trim().toLowerCase();
		return (!text || [item.username, item.displayName, item.role].some((value) => value?.toLowerCase().includes(text))) && (roleFilter === "all" || item.role === roleFilter);
	});

  return <>
    <section className="page-heading">
      <div><span className="eyebrow">ДОСТУП И РОЛИ</span><h1>Пользователи</h1><p>Отдельные учётные записи друзей и администраторов RemoteIt.</p></div>
      <button className="primary-button" onClick={() => setCreateOpen(true)}><UserPlus size={18} /> Создать пользователя</button>
    </section>
    <section className="stats-grid users-stats users-stats-wide">
      <Stat icon={Users} label="Всего пользователей" value={String(users.length)} note="в организации" tone="blue" />
      <Stat icon={CheckCircle2} label="Активные" value={String(active)} note="могут войти" tone="green" />
		<Stat icon={Crown} label="Владельцы" value={String(owners)} note="полный контроль" tone="blue" />
      <Stat icon={ShieldCheck} label="Администраторы" value={String(Math.max(0, admins - owners))} note="повышенные права" tone="violet" />
		<Stat icon={Eye} label="Только просмотр" value={String(viewers)} note="наблюдатели" tone="amber" />
    </section>
    <section className="device-panel users-panel">
		<div className="users-toolbar"><div><h2>Пользователи</h2><span>{visibleUsers.length}</span></div><label className="panel-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск пользователей…" /></label><select value={roleFilter} onChange={(event) => setRoleFilter(event.target.value)}><option value="all">Все роли</option><option value="owner">Владелец</option><option value="admin">Администратор</option><option value="technician">Техник</option><option value="viewer">Наблюдатель</option></select><button className="icon-button" onClick={() => void loadUsers()} aria-label="Обновить"><RefreshCw size={17} className={loading ? "spin" : ""} /></button></div>
      {error && <div className="panel-error">{error}</div>}
      <div className="table-wrap">
        <table className="users-table users-table-premium">
			<thead><tr><th>Пользователь</th><th>Роль</th><th>Статус</th><th>Последний вход</th><th>Первый вход</th><th>Безопасность</th><th>Действия</th></tr></thead>
          <tbody>{loading && users.length === 0 ? <LoadingRows /> : visibleUsers.map((item) => {
            const locked = item.id === currentUser.id || item.role === "owner" || (currentUser.role === "admin" && item.role === "admin");
            return <tr key={item.id}>
              <td><div className="device-name"><span className={`avatar user-avatar ${item.disabled ? "disabled" : ""}`}>{item.username.slice(0, 1).toUpperCase()}</span><div><strong>{item.displayName || item.username}</strong><small>@{item.username}</small></div></div></td>
              <td><select className="role-select" value={item.role} disabled={locked} onChange={(e) => void updateUser(item, e.target.value, item.disabled)}><option value="owner" disabled>Владелец</option><option value="admin" disabled={currentUser.role !== "owner"}>Администратор</option><option value="technician">Техник</option><option value="viewer">Наблюдатель</option></select></td>
				<td><span className={`status-pill ${item.disabled ? "is-offline" : "is-online"}`}><span />{item.disabled ? "Неактивен" : "Активен"}</span></td>
              <td>{item.lastLoginAt ? formatRelative(item.lastLoginAt) : "Никогда"}</td>
				<td>{item.mustChangePassword ? <span className="status-pill waiting"><span />Ожидается</span> : <span className="stacked"><strong>{new Date(item.createdAt).toLocaleDateString("ru-RU")}</strong><small>вход выполнен</small></span>}</td>
				<td>{item.disabled || item.mustChangePassword ? <span className="security-state attention"><AlertTriangle size={15} /> Требует внимания</span> : <span className="security-state"><ShieldCheck size={15} /> Защищено</span>}</td>
              <td><div className="user-actions"><button className="secondary-button compact-action" disabled={locked} onClick={() => setResetTarget(item)} title="Сбросить пароль"><KeyRound size={15} /><span>Пароль</span></button><button className={`secondary-button compact-action ${item.disabled ? "enable" : "danger"}`} disabled={locked} onClick={() => void updateUser(item, item.role, !item.disabled)} title={item.disabled ? "Включить" : "Отключить"}>{item.disabled ? <CheckCircle2 size={15} /> : <Ban size={15} />}<span>{item.disabled ? "Включить" : "Отключить"}</span></button></div></td>
            </tr>;
          })}</tbody>
        </table>
		{!loading && visibleUsers.length === 0 && <div className="empty-state"><h3>{users.length ? "Пользователи не найдены" : "Пользователей пока нет"}</h3><p>{users.length ? "Измените поиск или фильтр роли." : "Создайте первую дополнительную учётную запись."}</p></div>}
      </div>
		<div className="compact-list-footer"><span>Показано {visibleUsers.length} из {users.length}</span><small>Роли и статус меняются сразу</small></div>
    </section>
    {createOpen && <CreateUserModal csrf={csrf} isOwner={currentUser.role === "owner"} onClose={() => setCreateOpen(false)} onCreated={() => { setCreateOpen(false); void loadUsers(); }} />}
	{resetTarget && <ResetPasswordModal csrf={csrf} user={resetTarget} onClose={() => setResetTarget(null)} onDone={() => { setResetTarget(null); void loadUsers(); }} />}
  </>;
}

function ResetPasswordModal({ csrf, user, onClose, onDone }: { csrf: string; user: ManagedUser; onClose: () => void; onDone: () => void }) {
	const [password, setPassword] = useState("");
	const [confirm, setConfirm] = useState("");
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState("");
	async function submit(event: FormEvent) {
		event.preventDefault();
		if (password !== confirm) return setError("Пароли не совпадают");
		setLoading(true); setError("");
		try {
			await api(`/api/users/${user.id}/reset-password`, { method: "POST", body: JSON.stringify({ temporaryPassword: password }) }, csrf);
			onDone();
		} catch (reason) {
			setError(reason instanceof Error ? reason.message : "Не удалось сбросить пароль");
		} finally { setLoading(false); }
	}
	return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><section className="modal small-modal"><div className="modal-head"><div><span className="eyebrow">СБРОС ДОСТУПА</span><h2>Новый пароль для {user.username}</h2></div><button className="icon-button" onClick={onClose}><X size={19} /></button></div><form className="modal-form" onSubmit={submit}><label><span>Временный пароль</span><input type="password" minLength={4} maxLength={256} value={password} onChange={(event) => setPassword(event.target.value)} required autoFocus /></label><label><span>Повторите пароль</span><input type="password" minLength={4} maxLength={256} value={confirm} onChange={(event) => setConfirm(event.target.value)} required /></label><div className="notice"><ShieldCheck size={18} /><span>Все текущие сессии пользователя завершатся. При следующем входе он обязан задать собственный пароль.</span></div>{error && <div className="form-error">{error}</div>}<div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Отмена</button><button className="primary-button" disabled={loading}>{loading ? <RefreshCw size={17} className="spin" /> : <KeyRound size={17} />} Сбросить</button></div></form></section></div>;
}

function CreateUserModal({ csrf, isOwner, onClose, onCreated }: { csrf: string; isOwner: boolean; onClose: () => void; onCreated: () => void }) {
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState("technician");
  const [temporaryPassword, setTemporaryPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setLoading(true); setError("");
    try {
      await api("/api/users", { method: "POST", body: JSON.stringify({ username, displayName, role, temporaryPassword }) }, csrf);
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось создать пользователя");
    } finally {
      setLoading(false);
    }
  }

  return <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}><section className="modal"><div className="modal-head"><div><span className="eyebrow">НОВАЯ УЧЁТНАЯ ЗАПИСЬ</span><h2>Создать пользователя</h2></div><button className="icon-button" onClick={onClose}><X size={19} /></button></div><form onSubmit={submit} className="modal-form"><label><span>Отображаемое имя</span><input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Например, Алексей" maxLength={100} /></label><label><span>Логин</span><input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="alex" pattern="[A-Za-zА-Яа-яЁё0-9._-]{3,64}" required /></label><label><span>Роль</span><select value={role} onChange={(e) => setRole(e.target.value)}><option value="technician">Техник — инвентаризация без shell-команд</option><option value="viewer">Наблюдатель — только просмотр</option>{isOwner && <option value="admin">Администратор — пользователи и устройства</option>}</select></label><label><span>Временный пароль</span><input type="password" value={temporaryPassword} onChange={(e) => setTemporaryPassword(e.target.value)} minLength={4} maxLength={256} placeholder="Минимум 4 символа" required /></label><div className="notice"><ShieldCheck size={18} /><span>При первом входе пользователь обязательно задаст собственный пароль.</span></div>{error && <div className="form-error">{error}</div>}<div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Отмена</button><button className="primary-button" disabled={loading}>{loading ? <RefreshCw className="spin" size={17} /> : <UserPlus size={17} />} Создать</button></div></form></section></div>;
}

function TokensPage({ csrf, refreshKey, onCreate }: { csrf: string; refreshKey: number; onCreate: () => void }) {
	const [tokens, setTokens] = useState<EnrollmentTokenInfo[]>([]);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [expanded, setExpanded] = useState<string | null>(null);
	const [copiedToken, setCopiedToken] = useState<string | null>(null);
	const [selectedToken, setSelectedToken] = useState<EnrollmentTokenInfo | null>(null);
	const [query, setQuery] = useState("");
	const [groupFilter, setGroupFilter] = useState("all");
	const [stateFilter, setStateFilter] = useState("all");
	const load = useCallback(async (silent = false) => {
		if (!silent) setLoading(true);
		try {
			const result = await api<{ tokens: EnrollmentTokenInfo[] }>("/api/enrollment-tokens");
			setTokens(result.tokens); setError("");
		} catch (reason) {
			setError(reason instanceof Error ? reason.message : "Не удалось загрузить токены");
		} finally { if (!silent) setLoading(false); }
	}, []);
	useEffect(() => { void load(); }, [load, refreshKey]);
	useEffect(() => {
		const timer = window.setInterval(() => void load(true), 30_000);
		return () => window.clearInterval(timer);
	}, [load]);
	async function toggle(item: EnrollmentTokenInfo) {
		try {
			await api(`/api/enrollment-tokens/${item.id}`, { method: "PATCH", body: JSON.stringify({ disabled: !item.disabled }) }, csrf);
			await load();
		} catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось изменить токен"); }
	}
	async function deleteToken(item: EnrollmentTokenInfo) {
		if (!window.confirm(`Удалить токен «${item.name}»? Новые установки с ним сразу перестанут работать. Уже подключённые компьютеры останутся в RemoteIt.`)) return;
		try {
			await api(`/api/enrollment-tokens/${item.id}`, { method: "DELETE" }, csrf);
			setSelectedToken(null);
			await load();
		} catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось удалить токен"); }
	}
	async function copyToken(item: EnrollmentTokenInfo) {
		if (!item.token) return;
		try {
			await navigator.clipboard.writeText(item.token);
			setCopiedToken(item.id);
			window.setTimeout(() => setCopiedToken((current) => current === item.id ? null : current), 1500);
		} catch {
			setError("Не удалось скопировать токен. Выделите его вручную.");
		}
	}
	const now = Date.now();
	const stateOf = (item: EnrollmentTokenInfo) => {
		if (item.disabled) return { label: "Отозван", tone: "revoked" };
		if (new Date(item.expiresAt).getTime() <= now) return { label: "Истёк", tone: "expired" };
		if (item.uses >= item.maxUses) return { label: "Лимит исчерпан", tone: "exhausted" };
		return { label: "Активен", tone: "active" };
	};
	const active = tokens.filter((item) => stateOf(item).tone === "active").length;
	const used = tokens.reduce((sum, item) => sum + item.uses, 0);
	const linkedDevices = tokens.reduce((sum, item) => sum + (item.devices?.length || 0), 0);
	const groups = Array.from(new Set(tokens.map((item) => item.group))).sort((left, right) => left.localeCompare(right, "ru"));
	const visibleTokens = tokens.filter((item) => {
		const text = query.trim().toLowerCase();
		const matchesText = !text || [item.name, item.group, item.token, item.id].some((value) => value?.toLowerCase().includes(text));
		return matchesText && (groupFilter === "all" || item.group === groupFilter) && (stateFilter === "all" || stateOf(item).tone === stateFilter);
	});
	return <>
		<section className="page-heading"><div><span className="eyebrow">ПОДКЛЮЧЕНИЕ УСТРОЙСТВ</span><h1>Токены установки</h1><p>Все приглашения для агентов, остаток установок и зарегистрированные через них компьютеры.</p></div><button className="primary-button" onClick={onCreate}><Plus size={18} /> Создать токен</button></section>
		<section className="stats-grid token-stats">
			<Stat icon={KeyRound} label="Активные токены" value={String(active)} note={`из ${tokens.length} созданных`} tone="green" />
			<Stat icon={CheckCircle2} label="Использовано" value={String(used)} note="регистраций агентов" tone="blue" />
			<Stat icon={Boxes} label="Связанные устройства" value={String(linkedDevices)} note="компьютеров в истории" tone="violet" />
			<Stat icon={ShieldCheck} label="Доступ" value="Admin" note="значения видят только администраторы" tone="amber" />
		</section>
		<section className="device-panel token-panel-full">
			<div className="token-toolbar"><label className="panel-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск по имени, группе или токену" /></label><select value={groupFilter} onChange={(event) => setGroupFilter(event.target.value)}><option value="all">Все группы</option>{groups.map((item) => <option key={item} value={item}>{item}</option>)}</select><select value={stateFilter} onChange={(event) => setStateFilter(event.target.value)}><option value="all">Все статусы</option><option value="active">Активные</option><option value="revoked">Отозванные</option><option value="expired">Истёкшие</option><option value="exhausted">Лимит исчерпан</option></select><button className="secondary-button" onClick={() => void load()}><RefreshCw size={17} className={loading ? "spin" : ""} /> Обновить</button></div>
			{error && <div className="panel-error">{error}</div>}
			<div className="token-row-header"><span>Токен</span><span>Использовано</span><span>Осталось</span><span>Лимит</span><span>Действует до</span><span>Статус</span><span>Действия</span></div>
			<div className="token-list-full token-list-compact">{loading && tokens.length === 0 ? <PanelLoader /> : visibleTokens.map((item) => {
				const state = stateOf(item);
				const left = Math.max(0, item.maxUses - item.uses);
				const progress = Math.min(100, Math.round(item.uses / item.maxUses * 100));
				const open = expanded === item.id;
				const canToggle = new Date(item.expiresAt).getTime() > now && item.uses < item.maxUses;
				return <article className={`token-row token-${state.tone}`} key={item.id}>
					<div className="token-row-summary"><span className="token-row-chevron"><ChevronDown size={15} className={open ? "rotate" : ""} /></span><span className="token-card-icon"><KeyRound size={18} /></span><div className="token-card-title"><strong>{item.name}</strong><small>Группа: {item.group}</small><code title={item.token || "Значение старого токена не сохранено"}>{item.token || `ID: ${item.id.slice(0, 12)}`}</code></div></div>
					<div className="token-row-metric"><span>Использовано</span><strong>{item.uses}</strong><i><b style={{ width: `${progress}%` }} /></i></div>
					<div className="token-row-metric"><span>Осталось</span><strong>{left}</strong></div>
					<div className="token-row-metric"><span>Лимит</span><strong>{item.maxUses}</strong></div>
					<div className="token-row-metric"><span>Действует до</span><strong>{new Date(item.expiresAt).toLocaleString("ru-RU", { dateStyle: "short", timeStyle: "short" })}</strong></div>
					<div className="token-row-status"><span className={`token-state ${state.tone}`}>{state.label}</span><small>Связано: {item.devices?.length || 0}</small></div>
					<div className="token-row-actions"><button type="button" disabled={!item.token} onClick={() => void copyToken(item)} title="Копировать токен"><Copy size={16} /><span>{copiedToken === item.id ? "Готово" : "Копировать"}</span></button><button type="button" disabled={!item.token || state.tone !== "active"} onClick={() => downloadBoundWindowsInstaller(item.id)} title="Скачать готовый Agent"><Download size={16} /><span>Agent</span></button><button type="button" onClick={() => setSelectedToken(item)} title="Открыть токен"><KeyRound size={16} /><span>Открыть</span></button><button type="button" onClick={() => setExpanded(open ? null : item.id)} title="Связанные компьютеры"><Monitor size={16} /><span>ПК</span></button><button type="button" className={state.tone === "active" ? "danger" : "enable"} disabled={!canToggle} onClick={() => void toggle(item)} title={state.tone === "active" ? "Отозвать" : "Включить"}>{state.tone === "active" ? <Ban size={16} /> : <CheckCircle2 size={16} />}</button></div>
					{open && <div className="token-devices"><div className="token-devices-head"><strong>Зарегистрированные компьютеры</strong><small>{item.devices?.length ? "Связь сохраняется для новых регистраций" : item.uses ? `${item.uses} ранних регистраций были выполнены до включения истории связей` : "Токен ещё не использовался"}</small></div>{item.devices?.length ? item.devices.map((device) => <div className="token-device-row" key={device.id}><span className="status-dot" /><div><strong>{device.name}</strong><small>{device.connectionCode} · {new Date(device.enrolledAt).toLocaleString("ru-RU")}</small></div></div>) : <div className="token-device-empty">Подключённых устройств пока нет.</div>}</div>}
				</article>;
			})}{!loading && visibleTokens.length === 0 && <div className="empty-state"><KeyRound size={30} /><h3>{tokens.length ? "Ничего не найдено" : "Токенов пока нет"}</h3><p>{tokens.length ? "Измените строку поиска или фильтры." : "Создайте установщик, чтобы подключить первый компьютер."}</p>{!tokens.length && <button className="secondary-button" onClick={onCreate}><Plus size={17} /> Создать токен</button>}</div>}</div>
			<div className="compact-list-footer"><span>Показано {visibleTokens.length} из {tokens.length}</span><small>Полные значения доступны только владельцу и администраторам</small></div>
		</section>
		{selectedToken && <TokenDetailsModal item={selectedToken} onClose={() => setSelectedToken(null)} onDelete={() => void deleteToken(selectedToken)} />}
	</>;
}

function TokenDetailsModal({ item, onClose, onDelete }: { item: EnrollmentTokenInfo; onClose: () => void; onDelete: () => void }) {
	const [copied, setCopied] = useState(false);
	const [linkCopied, setLinkCopied] = useState(false);
	const [commandCopied, setCommandCopied] = useState(false);
	async function copy() {
		if (!item.token) return;
		await navigator.clipboard.writeText(item.token);
		setCopied(true);
		window.setTimeout(() => setCopied(false), 1500);
	}
	async function copyUnixCommand() {
		if (!item.token) return;
		await navigator.clipboard.writeText(unixInstallCommand(item.token));
		setCommandCopied(true);
		window.setTimeout(() => setCommandCopied(false), 1800);
	}
	async function copyInstallLink() {
		if (!item.token) return;
		await navigator.clipboard.writeText(`${window.location.origin}/#install=${encodeURIComponent(item.token)}`);
		setLinkCopied(true);
		window.setTimeout(() => setLinkCopied(false), 1800);
	}
	const expires = new Date(item.expiresAt);
	const active = !item.disabled && expires.getTime() > Date.now() && item.uses < item.maxUses;
	return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
		<section className="modal token-details-modal enrollment-result-modal">
			<div className="modal-head enrollment-result-head token-details-head"><div><span className="eyebrow">УПРАВЛЕНИЕ ТОКЕНОМ</span><h2>{item.name}</h2><p>{item.token ? "Токен готов к установке Agent на компьютеры." : "Архивная запись без сохранённого значения токена."}</p><small>Группа «{item.group}» · использовано {item.uses} из {item.maxUses}</small></div><button className="icon-button" onClick={onClose}><X size={21} /></button></div>
			<div className="token-result enrollment-token-result token-details-body">
				{item.token ? <>
					<div className="ready-token-box"><div className="ready-token-content"><span>ВАШ ТОКЕН</span><code>{item.token}</code></div><div className="ready-token-actions"><button type="button" onClick={() => void copy()}><Copy size={18} /> {copied ? "Скопировано" : "Код"}</button><button type="button" onClick={() => void copyInstallLink()}><Link2 size={18} /> {linkCopied ? "Ссылка скопирована" : "Ссылка"}</button></div></div>
					<div className="ready-token-validity"><ShieldCheck size={17} /><span>{active ? `Активен до ${expires.toLocaleString("ru-RU", { dateStyle: "medium", timeStyle: "short" })} · осталось ${Math.max(0, item.maxUses - item.uses)} установок.` : "Токен сейчас недоступен для новых установок: он отозван, истёк или исчерпал лимит."}</span></div>

					<section className="ready-section ready-platform-section">
						<div className="ready-section-title"><div><h3>Быстрая установка Agent</h3><p>Выберите операционную систему и передайте готовый установщик.</p></div></div>
						<div className="ready-platform-grid">
							<article className="ready-platform-card platform-windows"><span className="ready-platform-logo"><DeviceOSIcon os="Windows" size={38} /></span><h4>Windows</h4><p>Готовый Agent</p><small>Токен уже внутри · тихая установка</small><button type="button" disabled={!active} onClick={() => downloadBoundWindowsInstaller(item.id)}><Download size={17} /><span>Скачать Agent<strong>{active ? "Windows 10 / 11 / Server" : "токен недоступен"}</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Запустите файл двойным кликом, укажите имя компьютера и подтвердите UAC. Устройство привяжется автоматически.</p></details></article>
							<article className="ready-platform-card platform-macos"><span className="ready-platform-logo"><DeviceOSIcon os="macOS" size={38} /></span><h4>macOS</h4><p>Готовый Agent</p><small>Apple Silicon и Intel · токен внутри</small><button type="button" disabled={!active} onClick={() => downloadBoundUnixInstaller(item.id)}><Download size={17} /><span>Скачать Agent<strong>{active ? "macOS shell-установщик" : "токен недоступен"}</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Скачайте файл, откройте Terminal и запустите его. Токен уже встроен; установщик запросит только имя компьютера.</p></details></article>
							<article className="ready-platform-card platform-linux"><span className="ready-platform-logo"><DeviceOSIcon os="Linux" size={38} /></span><h4>Linux</h4><p>Готовый Agent</p><small>Ubuntu / Debian / x64 · токен внутри</small><button type="button" disabled={!active} onClick={() => downloadBoundUnixInstaller(item.id)}><Download size={17} /><span>Скачать Agent<strong>{active ? "Linux shell-установщик" : "токен недоступен"}</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Запустите файл через Terminal. Установщик при необходимости поставит curl и запросит только имя компьютера. Команда одной строкой доступна ниже.</p></details></article>
							<article className="ready-platform-card platform-android"><span className="ready-platform-logo"><DeviceOSIcon os="Android" size={38} /></span><h4>Android</h4><p>Agent для телефона</p><small>Экран и управление · токен копируется</small><button type="button" disabled={!active} onClick={() => downloadAndroidAgent(item.token)}><Download size={17} /><span>Скачать Agent<strong>{active ? "Android 8 или новее" : "токен недоступен"}</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Установите APK, вставьте скопированный токен, включите службу специальных возможностей и подтвердите системное разрешение на трансляцию экрана.</p></details></article>
						</div>
					</section>

					<details className="ready-other-platforms"><summary><span />ДРУГИЕ ПЛАТФОРМЫ<ChevronDown size={16} /><span /></summary><div><a href="/downloads/remoteit-agent-macos-arm64" download><Apple size={17} /> macOS Apple Silicon</a><a href="/downloads/remoteit-agent-macos-amd64" download><Apple size={17} /> macOS Intel</a><a href="/downloads/remoteit-agent-linux-amd64" download><TerminalSquare size={17} /> Linux amd64</a><button type="button" disabled={!active} onClick={() => void copyUnixCommand()}><Copy size={17} /> {commandCopied ? "Команда скопирована" : "Команда установки"}</button></div><code>{unixInstallCommand(item.token)}</code></details>

					<section className="ready-section ready-extra-section"><div className="ready-section-title"><div><h3>Дополнительные файлы</h3><p>Проверка сборок и приложения RemoteIt.</p></div></div><div className="ready-extra-grid"><a href="/downloads/SHA256SUMS.txt" download><ShieldCheck size={19} /><span>Контрольные суммы<strong>SHA-256</strong></span><ChevronDown size={15} /></a><a href="/downloads/APK-SIGNER.txt" download><ShieldCheck size={19} /><span>Подпись APK<strong>release certificate</strong></span><ChevronDown size={15} /></a><a href="/downloads/RemoteIt-Console.exe" download><Monitor size={19} /><span>RemoteIt Console<strong>настольная админка</strong></span><ChevronDown size={15} /></a><a href="/downloads/RemoteIt.apk" download><Download size={19} /><span>Android администратора<strong>мобильная панель</strong></span><ChevronDown size={15} /></a></div></section>
				</> : <div className="notice"><AlertTriangle size={18} /><span>Этот токен был создан до включения хранения полного значения. Его хеш остался рабочим, но показать код или собрать из него готовый Agent невозможно. Создайте новый токен.</span></div>}
				<details className="token-devices token-details-devices" open={Boolean(item.devices?.length)}>
					<summary className="token-devices-head"><span><strong>Подключённые компьютеры ({item.devices?.length || 0})</strong><small>Устройства, зарегистрированные с помощью этого токена.</small></span><ChevronDown size={17} /></summary>
					{item.devices?.length ? item.devices.map((device) => <div className="token-device-row" key={device.id}><span className="status-dot" /><div><strong>{device.name}</strong><small>{device.connectionCode} · {new Date(device.enrolledAt).toLocaleString("ru-RU")}</small></div></div>) : <div className="token-device-empty">Подключённых устройств пока нет.</div>}
				</details>
				<div className="ready-result-notice"><ShieldCheck size={18} /><span>Токен и установщики доступны только владельцу и администраторам. Windows Agent уже содержит этот токен; Linux/macOS запрашивает имя компьютера при запуске.</span></div>
				<div className="token-detail-footer"><button type="button" className="danger-button" onClick={onDelete}><Ban size={16} /> Удалить токен</button><button className="primary-button ready-done-button" onClick={onClose}>Готово</button></div>
			</div>
		</section>
	</div>;
}

function TokenManager({ csrf }: { csrf: string }) {
	const [tokens, setTokens] = useState<EnrollmentTokenInfo[]>([]);
	const [error, setError] = useState("");
	const load = useCallback(async () => {
		try {
			const result = await api<{ tokens: EnrollmentTokenInfo[] }>("/api/enrollment-tokens");
			setTokens(result.tokens.slice(0, 6)); setError("");
		} catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось загрузить токены"); }
	}, []);
	useEffect(() => { void load(); }, [load]);
	async function toggle(item: EnrollmentTokenInfo) {
		try {
			await api(`/api/enrollment-tokens/${item.id}`, { method: "PATCH", body: JSON.stringify({ disabled: !item.disabled }) }, csrf);
			await load();
		} catch (reason) { setError(reason instanceof Error ? reason.message : "Не удалось изменить токен"); }
	}
	if (!tokens.length && !error) return null;
	return <section className="token-manager"><div className="token-manager-head"><strong>Последние токены</strong><small>Полные значения и готовые агенты доступны в разделе «Токены»</small></div>{tokens.map((item) => {
		const expired = new Date(item.expiresAt).getTime() <= Date.now();
		const exhausted = item.uses >= item.maxUses;
		const active = !item.disabled && !expired && !exhausted;
		return <div className="managed-token" key={item.id}><div><strong>{item.name}</strong><small>{item.group} · {item.uses}/{item.maxUses} · {expired ? "истёк" : `до ${new Date(item.expiresAt).toLocaleDateString("ru-RU")}`}</small></div><button type="button" className={active ? "token-revoke" : "token-enable"} disabled={expired || exhausted} onClick={() => void toggle(item)}>{active ? "Отозвать" : item.disabled ? "Включить" : "Завершён"}</button></div>;
	})}{error && <div className="form-error">{error}</div>}</section>;
}

function EnrollmentModal({ csrf, onClose }: { csrf: string; onClose: () => void }) {
  const [name, setName] = useState("Основной установщик");
  const [group, setGroup] = useState("Основная группа");
  const [maxUses, setMaxUses] = useState(100);
	const [validDays, setValidDays] = useState(7);
  const [token, setToken] = useState("");
  const [tokenId, setTokenId] = useState("");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
	const [linkCopied, setLinkCopied] = useState(false);
	const [commandCopied, setCommandCopied] = useState(false);
  const [loading, setLoading] = useState(false);

  async function create(event: FormEvent) {
    event.preventDefault();
    setLoading(true); setError("");
    try {
      const result = await api<{ id: string; token: string }>("/api/enrollment-tokens", { method: "POST", body: JSON.stringify({ name, group, maxUses, validHours: validDays * 24 }) }, csrf);
      setTokenId(result.id);
      setToken(result.token);
    } catch (e) { setError(e instanceof Error ? e.message : "Не удалось создать токен"); }
    finally { setLoading(false); }
  }

  async function copy() {
    await navigator.clipboard.writeText(token);
    setCopied(true); window.setTimeout(() => setCopied(false), 1500);
  }
	async function copyInstallLink() {
		await navigator.clipboard.writeText(`${window.location.origin}/#install=${encodeURIComponent(token)}`);
		setLinkCopied(true); window.setTimeout(() => setLinkCopied(false), 1800);
	}

	const unixCommand = unixInstallCommand(token);
	async function copyUnixCommand() {
		await navigator.clipboard.writeText(unixCommand);
		setCommandCopied(true); window.setTimeout(() => setCommandCopied(false), 1800);
	}

  return <div className="modal-backdrop enrollment-modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
    <section className={`modal enrollment-modal ${token ? "enrollment-result-modal" : ""}`} role="dialog" aria-modal="true" aria-labelledby="enrollment-modal-title">
      <div className={`modal-head ${token ? "enrollment-result-head" : ""}`}><div><span className="eyebrow">АВТОМАТИЧЕСКАЯ ПРИВЯЗКА</span><h2 id="enrollment-modal-title">{token ? "Токен готов" : "Добавить устройства"}</h2>{token && <><p>Используйте токен при установке Agent на компьютере.</p><small>После установки устройство автоматически появится в группе «{group}».</small></>}</div><button className="icon-button" onClick={onClose} aria-label="Закрыть окно добавления устройства"><X size={21} /></button></div>
      {token ? <div className="token-result enrollment-token-result">
        <div className="ready-token-box"><div className="ready-token-content"><span>ВАШ ТОКЕН</span><code>{token}</code></div><div className="ready-token-actions"><button type="button" onClick={copy}><Copy size={18} /> {copied ? "Скопировано" : "Код"}</button><button type="button" onClick={copyInstallLink}><Link2 size={18} /> {linkCopied ? "Ссылка скопирована" : "Ссылка"}</button></div></div>
        <div className="ready-token-validity"><ShieldCheck size={17} /><span>До {maxUses} установок · действует {validDays} дн. · доступ можно отозвать в панели.</span></div>

        <section className="ready-section ready-platform-section">
          <div className="ready-section-title"><div><h3>Быстрая установка Agent</h3><p>Выберите систему и скачайте подходящий установщик.</p></div></div>
          <div className="ready-platform-grid">
            <article className="ready-platform-card platform-windows"><span className="ready-platform-logo"><DeviceOSIcon os="Windows" size={38} /></span><h4>Windows</h4><p>Готовый Agent</p><small>Токен уже внутри · тихая установка</small><button type="button" onClick={() => downloadBoundWindowsInstaller(tokenId)}><Download size={17} /><span>Скачать Agent<strong>Windows 10 / 11 / Server</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Запустите файл двойным кликом, укажите имя компьютера и подтвердите UAC. Устройство появится в панели автоматически.</p></details></article>
            <article className="ready-platform-card platform-macos"><span className="ready-platform-logo"><DeviceOSIcon os="macOS" size={38} /></span><h4>macOS</h4><p>Готовый Agent</p><small>Apple Silicon и Intel · токен внутри</small><button type="button" onClick={() => downloadBoundUnixInstaller(tokenId)}><Download size={17} /><span>Скачать Agent<strong>macOS shell-установщик</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Скачайте файл, откройте Terminal и запустите его. Токен уже встроен; потребуется только имя компьютера.</p></details></article>
			<article className="ready-platform-card platform-linux"><span className="ready-platform-logo"><DeviceOSIcon os="Linux" size={38} /></span><h4>Linux</h4><p>Готовый Agent</p><small>Ubuntu / Debian / x64 · токен внутри</small><button type="button" onClick={() => downloadBoundUnixInstaller(tokenId)}><Download size={17} /><span>Скачать Agent<strong>Linux shell-установщик</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Запустите файл через Terminal. Он автоматически использует токен и запросит только имя компьютера; команда одной строкой доступна ниже.</p></details></article>
			<article className="ready-platform-card platform-android"><span className="ready-platform-logo"><DeviceOSIcon os="Android" size={38} /></span><h4>Android</h4><p>Agent для телефона</p><small>Экран и управление · токен копируется</small><button type="button" onClick={() => downloadAndroidAgent(token)}><Download size={17} /><span>Скачать Agent<strong>Android 8 или новее</strong></span></button><details><summary><FileCode2 size={13} /> Инструкция</summary><p>Установите APK, вставьте скопированный токен, включите службу специальных возможностей и подтвердите системное разрешение на трансляцию экрана.</p></details></article>
          </div>
        </section>

        <details className="ready-other-platforms"><summary><span />ДРУГИЕ ПЛАТФОРМЫ<ChevronDown size={16} /><span /></summary><div><a href="/downloads/remoteit-agent-macos-arm64" download><Apple size={17} /> macOS Apple Silicon</a><a href="/downloads/remoteit-agent-macos-amd64" download><Apple size={17} /> macOS Intel</a><a href="/downloads/remoteit-agent-linux-amd64" download><TerminalSquare size={17} /> Linux amd64</a><button type="button" onClick={() => void copyUnixCommand()}><Copy size={17} /> {commandCopied ? "Команда скопирована" : "Команда установки"}</button></div><code>{unixCommand}</code></details>

        <section className="ready-section ready-extra-section"><div className="ready-section-title"><div><h3>Дополнительные файлы</h3><p>Проверка сборок и приложения RemoteIt.</p></div></div><div className="ready-extra-grid"><a href="/downloads/SHA256SUMS.txt" download><ShieldCheck size={19} /><span>Контрольные суммы<strong>SHA-256</strong></span><ChevronDown size={15} /></a><a href="/downloads/APK-SIGNER.txt" download><ShieldCheck size={19} /><span>Подпись APK<strong>release certificate</strong></span><ChevronDown size={15} /></a><a href="/downloads/RemoteIt-Console.exe" download><Monitor size={19} /><span>RemoteIt Console<strong>настольная админка</strong></span><ChevronDown size={15} /></a><a href="/downloads/RemoteIt.apk" download><Download size={19} /><span>Android APK<strong>мобильное приложение</strong></span><ChevronDown size={15} /></a></div></section>

        <div className="ready-result-notice"><CircleHelp size={18} /><span>После успешной установки Agent свяжется с сервером и появится в списке устройств. Убедитесь, что сервер RemoteIt доступен из сети машины.</span></div>
        <button className="primary-button full ready-done-button" onClick={onClose}>Готово</button>
      </div> : <form onSubmit={create} className="modal-form">
        <label><span>Название установщика</span><input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} required /></label>
        <label><span>Группа устройств</span><input value={group} onChange={(e) => setGroup(e.target.value)} maxLength={100} required /></label>
        <label><span>Количество установок</span><input type="number" min={1} max={300} value={maxUses} onChange={(e) => setMaxUses(Number(e.target.value))} required /></label>
		<label><span>Срок действия, дней</span><input type="number" min={1} max={30} value={validDays} onChange={(e) => setValidDays(Number(e.target.value))} required /></label>
        <TokenManager csrf={csrf} />{error && <div className="form-error">{error}</div>}
        <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Отмена</button><button className="primary-button" disabled={loading}>{loading ? <RefreshCw className="spin" size={17} /> : <KeyRound size={17} />} Создать токен</button></div>
      </form>}
    </section>
  </div>;
}

function formatRelative(value: string) {
  if (!value) return "никогда";
  const seconds = Math.floor((Date.now() - new Date(value).getTime()) / 1000);
  if (seconds < 60) return `${seconds} сек. назад`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} мин. назад`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} ч. назад`;
  return `${Math.floor(seconds / 86400)} дн. назад`;
}

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [csrf, setCsrf] = useState("");
  const [ready, setReady] = useState(false);
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = window.localStorage.getItem("genesis-theme");
    return saved === "light" || saved === "blue" || saved === "dark" ? saved : "light";
  });

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem("genesis-theme", theme);
  }, [theme]);

  useEffect(() => {
    api<{ user: User; csrfToken: string }>("/api/auth/me").then(({ user, csrfToken }) => { setUser(user); setCsrf(csrfToken); }).catch(() => setUser(null)).finally(() => setReady(true));
  }, []);

  if (!ready) return <div className="boot-screen"><Brand /><RefreshCw className="spin" /></div>;
  if (!user) return <Login theme={theme} onTheme={setTheme} onLogin={(nextUser, nextCsrf) => { setUser(nextUser); setCsrf(nextCsrf); }} />;
  if (!csrf) return <Login theme={theme} onTheme={setTheme} onLogin={(nextUser, nextCsrf) => { setUser(nextUser); setCsrf(nextCsrf); }} />;
  if (user.mustChangePassword) return <ChangePassword user={user} csrf={csrf} theme={theme} onTheme={setTheme} onDone={() => setUser({ ...user, mustChangePassword: false })} />;
  return <Dashboard user={user} csrf={csrf} theme={theme} onTheme={setTheme} onLogout={() => { setUser(null); setCsrf(""); }} />;
}
