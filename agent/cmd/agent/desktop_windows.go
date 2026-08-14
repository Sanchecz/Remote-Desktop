//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/coder/websocket"
	"golang.org/x/sys/windows"
)

var desktopSessionActive atomic.Bool
var desktopSessionControl atomic.Bool
var desktopLastFrameUnix atomic.Int64
var desktopSessionIdentifier atomic.Value

const desktopMaximumFrameBytes = 8 << 20

type desktopPublishedState struct {
	Active    bool      `json:"active"`
	Control   bool      `json:"control"`
	SessionID string    `json:"sessionId,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

var (
	desktopPublishedMu      sync.Mutex
	desktopPublishedKey     string
	desktopPublishedWritten time.Time
)

var (
	user32Desktop              = windows.NewLazySystemDLL("user32.dll")
	gdi32Desktop               = windows.NewLazySystemDLL("gdi32.dll")
	winmmDesktop               = windows.NewLazySystemDLL("winmm.dll")
	kernel32Desktop            = windows.NewLazySystemDLL("kernel32.dll")
	procGetDC                  = user32Desktop.NewProc("GetDC")
	procReleaseDC              = user32Desktop.NewProc("ReleaseDC")
	procGetSystemMetrics       = user32Desktop.NewProc("GetSystemMetrics")
	procGetCursorInfo          = user32Desktop.NewProc("GetCursorInfo")
	procGetIconInfo            = user32Desktop.NewProc("GetIconInfo")
	procDrawIconEx             = user32Desktop.NewProc("DrawIconEx")
	procSetProcessDPIAware     = user32Desktop.NewProc("SetProcessDPIAware")
	procSetProcessDPIContext   = user32Desktop.NewProc("SetProcessDpiAwarenessContext")
	procSendInput              = user32Desktop.NewProc("SendInput")
	procOpenInputDesktop       = user32Desktop.NewProc("OpenInputDesktop")
	procSetThreadDesktop       = user32Desktop.NewProc("SetThreadDesktop")
	procCloseDesktop           = user32Desktop.NewProc("CloseDesktop")
	procGetUserObjectInfo      = user32Desktop.NewProc("GetUserObjectInformationW")
	procCreateCompatibleDC     = gdi32Desktop.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32Desktop.NewProc("DeleteDC")
	procCreateDIBSection       = gdi32Desktop.NewProc("CreateDIBSection")
	procCreateCompatibleBitmap = gdi32Desktop.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32Desktop.NewProc("SelectObject")
	procDeleteObject           = gdi32Desktop.NewProc("DeleteObject")
	procBitBlt                 = gdi32Desktop.NewProc("BitBlt")
	procStretchBlt             = gdi32Desktop.NewProc("StretchBlt")
	procSetStretchBltMode      = gdi32Desktop.NewProc("SetStretchBltMode")
	procGetDIBits              = gdi32Desktop.NewProc("GetDIBits")
	procTimeBeginPeriod        = winmmDesktop.NewProc("timeBeginPeriod")
	procTimeEndPeriod          = winmmDesktop.NewProc("timeEndPeriod")
	procCreateWaitableTimerEx  = kernel32Desktop.NewProc("CreateWaitableTimerExW")
	procSetWaitableTimerEx     = kernel32Desktop.NewProc("SetWaitableTimerEx")
	procWaitForSingleObject    = kernel32Desktop.NewProc("WaitForSingleObject")
	procCloseHandle            = kernel32Desktop.NewProc("CloseHandle")
)

type desktopSessionOffer struct {
	ID             string `json:"id"`
	ControlEnabled bool   `json:"controlEnabled"`
	TargetFPS      int    `json:"targetFps"`
	CursorVisible  bool   `json:"cursorVisible"`
}

type desktopOfferRefresh struct {
	offer  desktopSessionOffer
	active bool
	err    error
}

type desktopFrameUpload struct {
	access    desktopAgentAccess
	sessionID string
	sequence  uint64
	capture   desktopCapture
}

type desktopFrameUploadResult struct {
	sessionID string
	capture   desktopCapture
	duration  time.Duration
	err       error
}

type desktopWaitTimer struct {
	handle uintptr
}

func newDesktopWaitTimer() desktopWaitTimer {
	const createWaitableTimerHighResolution = 0x00000002
	const timerAllAccess = 0x001F0003
	handle, _, _ := procCreateWaitableTimerEx.Call(0, 0, createWaitableTimerHighResolution, timerAllAccess)
	return desktopWaitTimer{handle: handle}
}

func (timer *desktopWaitTimer) Close() {
	if timer.handle != 0 {
		procCloseHandle.Call(timer.handle)
		timer.handle = 0
	}
}

type desktopInput struct {
	Type    string `json:"type"`
	Action  string `json:"action,omitempty"`
	Button  string `json:"button,omitempty"`
	Text    string `json:"text,omitempty"`
	X       int    `json:"x,omitempty"`
	Y       int    `json:"y,omitempty"`
	Delta   int    `json:"delta,omitempty"`
	KeyCode int    `json:"keyCode,omitempty"`
}

type desktopInputTask struct {
	events  []desktopInput
	capture desktopCapture
}

type desktopInputTaskResult struct {
	err error
}

func runDesktopInputWorker(ctx context.Context, tasks <-chan desktopInputTask, results chan<- desktopInputTaskResult) {
	// SendInput must follow the currently visible Windows desktop, but it does
	// not need to occupy the capture/encode cadence thread. A dedicated locked
	// OS thread prevents dense 60 Hz pointer batches from stealing the next
	// frame deadline while preserving support for winlogon/UAC desktops.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	surface := &desktopInputSurface{}
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-tasks:
			_, taskErr := surface.SyncIfStale(100 * time.Millisecond)
			if taskErr == nil {
				for _, event := range task.events {
					if taskErr = executeDesktopInput(event, task.capture); taskErr != nil {
						break
					}
				}
			}
			select {
			case results <- desktopInputTaskResult{err: taskErr}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// desktopInputSurface keeps the capture thread attached to the desktop that is
// currently visible on the physical console. Windows uses winsta0\\default for
// the signed-in user and winsta0\\winlogon for the lock/sign-in screen. A
// LocalSystem process may open both, but GetDC/BitBlt and Desktop Duplication
// only see the desktop assigned to the calling thread.
type desktopInputSurface struct {
	handle      uintptr
	name        string
	lastChecked time.Time
}

func (surface *desktopInputSurface) Sync() (bool, error) {
	surface.lastChecked = time.Now()
	const maximumAllowed = 0x02000000
	handle, _, callErr := procOpenInputDesktop.Call(0, 0, maximumAllowed)
	if handle == 0 {
		return false, fmt.Errorf("open the visible Windows desktop: %w", callErr)
	}
	name, err := windowsDesktopObjectName(handle)
	if err != nil {
		procCloseDesktop.Call(handle)
		return false, err
	}
	if surface.handle != 0 && strings.EqualFold(surface.name, name) {
		procCloseDesktop.Call(handle)
		return false, nil
	}
	if switched, _, switchErr := procSetThreadDesktop.Call(handle); switched == 0 {
		procCloseDesktop.Call(handle)
		return false, fmt.Errorf("switch capture to Windows desktop %q: %w", name, switchErr)
	}
	previous := surface.handle
	surface.handle, surface.name = handle, name
	if previous != 0 {
		procCloseDesktop.Call(previous)
	}
	return true, nil
}

// SyncIfStale avoids reopening and querying winsta0 on every video frame and
// every pointer packet. OpenInputDesktop/GetUserObjectInformation are kernel
// round trips and, on virtual display drivers, two redundant calls per frame
// consumed roughly 8-12 ms outside the capture diagnostics. A 100 ms check is
// still fast enough to follow lock/UAC desktop transitions without a visible
// delay, while leaving the full 16.7 ms budget available to explicit 60 FPS.
func (surface *desktopInputSurface) SyncIfStale(interval time.Duration) (bool, error) {
	if surface.handle != 0 && time.Since(surface.lastChecked) < interval {
		return false, nil
	}
	return surface.Sync()
}

func windowsDesktopObjectName(handle uintptr) (string, error) {
	const userObjectName = 2
	var required uint32
	procGetUserObjectInfo.Call(handle, userObjectName, 0, 0, uintptr(unsafe.Pointer(&required)))
	if required < 2 || required > 64*1024 {
		return "", errors.New("Windows returned an invalid desktop name length")
	}
	buffer := make([]uint16, (required+1)/2)
	result, _, callErr := procGetUserObjectInfo.Call(
		handle,
		userObjectName,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)*2),
		uintptr(unsafe.Pointer(&required)),
	)
	if result == 0 {
		return "", fmt.Errorf("read the visible Windows desktop name: %w", callErr)
	}
	return windows.UTF16ToString(buffer), nil
}

type desktopCapture struct {
	JPEG           []byte
	FrameWidth     int
	FrameHeight    int
	ScreenX        int
	ScreenY        int
	ScreenWidth    int
	ScreenHeight   int
	CaptureMillis  int
	CopyMillis     int
	ScaleMillis    int
	EncodeMillis   int
	CaptureBackend string
}

type desktopWindowsInput struct {
	Kind uint32
	_    uint32
	Data [32]byte
}

type desktopWindowsKeyboardInput struct {
	VirtualKey uint16
	ScanCode   uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

type desktopWindowsMouseInput struct {
	DX        int32
	DY        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type rgbQuad struct {
	Blue, Green, Red, Reserved byte
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]rgbQuad
}

type desktopPoint struct {
	X int32
	Y int32
}

type desktopCursorInfo struct {
	Size     uint32
	Flags    uint32
	Cursor   uintptr
	Position desktopPoint
}

type desktopIconInfo struct {
	Icon     int32
	HotspotX uint32
	HotspotY uint32
	Mask     uintptr
	Color    uintptr
}

type desktopCursorState struct {
	Visible bool
	X       int
	Y       int
	Handle  uintptr
}

func runDesktopAgentLoop(done <-chan struct{}) {
	// SetThreadDesktop is thread-scoped. Keep the control/capture loop on one OS
	// thread so it can follow default <-> winlogon transitions reliably.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Windows otherwise rounds short waits to the default ~15.6 ms timer tick.
	// That quantises a requested 60 FPS loop into 31/47/62 ms steps even when
	// DXGI and TurboJPEG finish in a few milliseconds. Request a 1 ms timer for
	// the lifetime of the desktop worker and always release it on shutdown.
	if result, _, _ := procTimeBeginPeriod.Call(1); result == 0 {
		defer procTimeEndPeriod.Call(1)
	}
	waitTimer := newDesktopWaitTimer()
	defer waitTimer.Close()
	setDesktopSessionState(false, false, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	setDesktopProcessDPIAwareness()
	controlClient := newDesktopHTTPClient(15 * time.Second)
	frameClient := newDesktopHTTPClient(8 * time.Second)
	inputClient := newDesktopHTTPClient(3 * time.Second)
	defer controlClient.CloseIdleConnections()
	defer frameClient.CloseIdleConnections()
	defer inputClient.CloseIdleConnections()
	capturer := &desktopCapturer{}
	defer capturer.Close()
	inputSurface := &desktopInputSurface{}
	lastReportedError := ""
	lastReportedAt := time.Time{}
	lastSessionID := ""
	lastInputAt := time.Time{}
	lastCapture := desktopCapture{}
	nextFrameAt := time.Time{}
	currentOffer := desktopSessionOffer{}
	offerActive := false
	lastOfferAt := time.Time{}
	var offerFetchInFlight atomic.Bool
	offerResults := make(chan desktopOfferRefresh, 1)
	var inputFetchInFlight atomic.Bool
	latestCapture := atomic.Value{}
	latestCapture.Store(desktopCapture{})
	inputTasks := make(chan desktopInputTask, 8)
	inputResults := make(chan desktopInputTaskResult, 1)
	go runDesktopInputWorker(ctx, inputTasks, inputResults)
	inputErrors := make(chan error, 1)
	streamInputBatches := make(chan []desktopInput, 16)
	var inputStreamActive atomic.Bool
	var latestInputNanos atomic.Int64
	frameUploads := make(chan desktopFrameUpload, 1)
	frameUploadResults := make(chan desktopFrameUploadResult, 8)
	go runDesktopFrameUploader(ctx, frameClient, frameUploads, frameUploadResults, streamInputBatches, &inputStreamActive)
	go runDesktopStreamInputDispatcher(ctx, streamInputBatches, inputTasks, &latestCapture, &latestInputNanos)
	autoCadence := newDesktopAutoCadence()
	access := desktopAgentAccess{}
	accessLoadedAt := time.Time{}
	var frameSequence uint64
	for ctx.Err() == nil {
		var err error
		if access.ServerURL == "" || time.Since(accessLoadedAt) >= 5*time.Second {
			access, err = loadDesktopAgentAccess()
			if err == nil {
				accessLoadedAt = time.Now()
			}
		}
		if err != nil {
			offerActive = false
			setDesktopSessionState(false, false, "")
			if !desktopWait(ctx, 2*time.Second, &waitTimer) {
				return
			}
			continue
		}
		// Refresh session parameters independently from the video cadence. A
		// synchronous offer request every few frames used to steal 2-4 FPS from
		// otherwise fast captures. The first offer remains synchronous; changes
		// to control/FPS are then picked up by the background refresh channel.
		if offerActive {
			select {
			case refreshed := <-offerResults:
				offerFetchInFlight.Store(false)
				if refreshed.err != nil || !refreshed.active {
					offerActive = false
					setDesktopSessionState(false, false, "")
					lastReportedError = ""
					lastReportedAt = time.Time{}
					if !desktopWait(ctx, 1500*time.Millisecond, &waitTimer) {
						return
					}
					continue
				}
				if currentOffer.ID != refreshed.offer.ID || currentOffer.ControlEnabled != refreshed.offer.ControlEnabled || currentOffer.TargetFPS != refreshed.offer.TargetFPS || currentOffer.CursorVisible != refreshed.offer.CursorVisible {
					resetDesktopInputState()
				}
				currentOffer = refreshed.offer
			default:
			}
		}
		if !offerActive {
			offer, active, offerErr := fetchDesktopOffer(ctx, controlClient, access)
			lastOfferAt = time.Now()
			if offerErr != nil || !active {
				offerActive = false
				setDesktopSessionState(false, false, "")
				lastReportedError = ""
				lastReportedAt = time.Time{}
				if !desktopWait(ctx, 1500*time.Millisecond, &waitTimer) {
					return
				}
				continue
			}
			currentOffer = offer
			offerActive = true
		} else if time.Since(lastOfferAt) >= 250*time.Millisecond && offerFetchInFlight.CompareAndSwap(false, true) {
			lastOfferAt = time.Now()
			accessCopy := access
			go func() {
				offer, active, offerErr := fetchDesktopOffer(ctx, controlClient, accessCopy)
				select {
				case offerResults <- desktopOfferRefresh{offer: offer, active: active, err: offerErr}:
				case <-ctx.Done():
				}
			}()
		}
		offer := currentOffer
		setDesktopSessionState(true, offer.ControlEnabled, offer.ID)
		if offer.ID != lastSessionID {
			lastSessionID = offer.ID
			frameSequence = 0
			drainDesktopFrameUploads(frameUploads)
			lastCapture = desktopCapture{}
			latestCapture.Store(desktopCapture{})
			nextFrameAt = time.Time{}
			autoCadence.Reset()
		}
		// Execute pushed input on this locked OS thread. Besides removing the old
		// 150 ms long-poll delay, this keeps SendInput attached to the same visible
		// default/winlogon desktop as capture and avoids intermittent access errors.
		//
		// A 60 FPS viewer can deliver several pointer packets between two capture
		// iterations. Synchronising OpenInputDesktop for every packet made input
		// compete with DXGI and produced an 8-10 FPS feedback loop. Drain a bounded
		// group, retain the newest stale pointer move and synchronise the Windows
		// desktop once. Button/key/wheel actions remain ordered and are never lost.
		select {
		case completed := <-inputResults:
			if completed.err != nil {
				select {
				case inputErrors <- completed.err:
				default:
				}
			}
		default:
		}
		if inputAt := latestInputNanos.Load(); inputAt > 0 {
			lastInputAt = time.Unix(0, inputAt)
		}
		// Restrictive proxies and older servers still use the HTTP long-poll
		// fallback. Its goroutine only performs I/O; input is executed above.
		if offer.ControlEnabled && !inputStreamActive.Load() && len(lastCapture.JPEG) > 0 && inputFetchInFlight.CompareAndSwap(false, true) {
			accessCopy, sessionID := access, offer.ID
			go func() {
				defer inputFetchInFlight.Store(false)
				events, inputErr := fetchDesktopInputs(ctx, inputClient, accessCopy, sessionID)
				if currentDesktopSessionIdentifier() != sessionID {
					return
				}
				if inputErr == nil && len(events) > 0 {
					select {
					case streamInputBatches <- events:
					case <-ctx.Done():
					}
				}
				if inputErr != nil {
					select {
					case inputErrors <- inputErr:
					default:
					}
				}
			}()
		}
		select {
		case inputErr := <-inputErrors:
			message := inputErr.Error()
			if message != lastReportedError || time.Since(lastReportedAt) >= 10*time.Second {
				if reportDesktopStatus(ctx, controlClient, access, offer.ID, message) == nil {
					lastReportedError = message
					lastReportedAt = time.Now()
				}
			}
		default:
		}
		select {
		case uploaded := <-frameUploadResults:
			if uploaded.sessionID == offer.ID {
				if uploaded.err != nil {
					message := uploaded.err.Error()
					if message != lastReportedError || time.Since(lastReportedAt) >= 10*time.Second {
						if reportDesktopStatus(ctx, controlClient, access, offer.ID, message) == nil {
							lastReportedError = message
							lastReportedAt = time.Now()
						}
					}
				} else {
					if offer.TargetFPS == 0 {
						// Capture/scale/encode and HTTPS upload run as a two-stage
						// pipeline, so its sustainable cadence is determined by the
						// slower stage, not by upload time alone.  This keeps Auto at
						// its 30 FPS default on a healthy machine, lowers it when either
						// CPU or the link is persistently congested, and only promotes
						// it to 60 after both stages prove fast.
						processing := time.Duration(uploaded.capture.CaptureMillis+uploaded.capture.EncodeMillis) * time.Millisecond
						autoCadence.Observe(max(uploaded.duration, processing))
					}
					desktopLastFrameUnix.Store(time.Now().Unix())
					if lastReportedError != "" {
						_ = reportDesktopStatus(ctx, controlClient, access, offer.ID, "")
						lastReportedError = ""
						lastReportedAt = time.Time{}
					}
				}
			}
		default:
		}
		// Manual modes cap the stream at 15, 30 or 60 FPS. Auto starts at 30,
		// lowers the cap only on sustained congestion and raises it to 60 only
		// after the channel stays comfortably fast.
		effectiveFPS := offer.TargetFPS
		if effectiveFPS == 0 {
			effectiveFPS = autoCadence.FPS
		}
		if effectiveFPS != 15 && effectiveFPS != 30 && effectiveFPS != 60 {
			effectiveFPS = 30
		}
		captureInterval := desktopCaptureInterval(effectiveFPS)
		captureErr := error(nil)
		if nextFrameAt.IsZero() {
			nextFrameAt = time.Now()
		}
		if len(lastCapture.JPEG) == 0 || !time.Now().Before(nextFrameAt) {
			frameStartedAt := time.Now()
			// Pace from an absolute deadline rather than capture start. Starting a
			// fresh interval after each frame adds capture+encode overhead to every
			// period (30 FPS became ~24, 60 became ~40). Advance through missed
			// deadlines without queuing stale work, preserving low latency.
			for !nextFrameAt.After(frameStartedAt) {
				nextFrameAt = nextFrameAt.Add(captureInterval)
			}
			// The dedicated input worker checks winlogon/UAC transitions every 100 ms.
			// Capture only needs a slower safety check: OpenInputDesktop and querying
			// its object name can consume most of one 16.7 ms frame on virtual display
			// drivers. A 500 ms cadence removes that periodic 60 FPS hitch while input
			// remains responsive and the capture surface follows shortly afterwards.
			desktopChanged, desktopErr := inputSurface.SyncIfStale(500 * time.Millisecond)
			if desktopErr != nil {
				captureErr = desktopErr
			} else if desktopChanged {
				// DXGI duplication and GDI DCs belong to the previous desktop. Rebuild
				// them exactly once when Windows locks, unlocks or shows UAC.
				capturer.Close()
				lastCapture = desktopCapture{}
				latestCapture.Store(desktopCapture{})
			}
			// Auto begins with the same 30 FPS profile as explicit 30. It changes
			// geometry only after the cadence controller has accumulated sustained
			// evidence (20 slow or 300 fast samples), so an occasional network spike
			// cannot trigger the old 15/30/60 resource-rebuild loop.
			captureProfileFPS := effectiveFPS
			capture := desktopCapture{}
			if captureErr == nil {
				interactive := offer.ControlEnabled && time.Since(lastInputAt) < 350*time.Millisecond
				capture, captureErr = capturer.CaptureJPEG(captureProfileFPS, interactive, offer.CursorVisible)
			}
			if captureErr == nil {
				// Desktop Duplication reports when the surface did not change. Do not
				// upload the previous JPEG again: duplicate 30/60 FPS traffic used to
				// compete with mouse and keyboard requests without changing a pixel.
				if capture.CaptureBackend == "dxgi-wait-reuse" && len(lastCapture.JPEG) > 0 {
					lastCapture = capture
					inputCapture := capture
					inputCapture.JPEG = nil
					latestCapture.Store(inputCapture)
				} else {
					uploadCapture := capture
					// desktopCapturer reuses its TurboJPEG buffer. The uploader runs in
					// parallel, so give it an immutable copy and keep at most the newest
					// waiting frame. This overlaps capture/encoding with HTTPS without
					// ever building a latency-producing frame backlog.
					uploadCapture.JPEG = append([]byte(nil), capture.JPEG...)
					frameSequence++
					enqueueLatestDesktopFrame(frameUploads, desktopFrameUpload{access: access, sessionID: offer.ID, sequence: frameSequence, capture: uploadCapture})
					lastCapture = capture
					inputCapture := capture
					inputCapture.JPEG = nil
					latestCapture.Store(inputCapture)
				}
			}
			if captureErr == nil {
				// Upload success/error and Auto adaptation are handled asynchronously
				// through frameUploadResults above.
			}
		}
		if captureErr != nil {
			message := captureErr.Error()
			if message != lastReportedError || time.Since(lastReportedAt) >= 10*time.Second {
				if reportDesktopStatus(ctx, controlClient, access, offer.ID, message) == nil {
					lastReportedError = message
					lastReportedAt = time.Now()
				}
			}
		}
		delay := 2 * time.Millisecond
		if len(lastCapture.JPEG) > 0 {
			untilNextFrame := time.Until(nextFrameAt)
			if untilNextFrame > 0 {
				delay = min(untilNextFrame, 20*time.Millisecond)
			}
		}
		if !desktopWait(ctx, delay, &waitTimer) {
			return
		}
	}
	setDesktopSessionState(false, false, "")
}

func desktopPublishedStatePath() string {
	return filepath.Join(filepath.Dir(defaultConfigPath()), "desktop-session-status.json")
}

func setDesktopSessionState(active, control bool, sessionID string) {
	desktopSessionActive.Store(active)
	desktopSessionControl.Store(control)
	desktopSessionIdentifier.Store(sessionID)

	desktopPublishedMu.Lock()
	defer desktopPublishedMu.Unlock()
	key := strconv.FormatBool(active) + "|" + strconv.FormatBool(control) + "|" + sessionID
	now := time.Now().UTC()
	if key == desktopPublishedKey && now.Sub(desktopPublishedWritten) < 2*time.Second {
		return
	}
	state := desktopPublishedState{Active: active, Control: control, SessionID: sessionID, UpdatedAt: now}
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	path := desktopPublishedStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return
	}
	// os.Rename does not replace an existing destination on Windows.
	_ = os.Remove(path)
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return
	}
	desktopPublishedKey = key
	desktopPublishedWritten = now
}

func publishedDesktopSessionState() (active, control bool, sessionID string) {
	data, err := os.ReadFile(desktopPublishedStatePath())
	if err != nil {
		return false, false, ""
	}
	var state desktopPublishedState
	if json.Unmarshal(data, &state) != nil || state.UpdatedAt.IsZero() || time.Since(state.UpdatedAt) > 8*time.Second {
		return false, false, ""
	}
	return state.Active, state.Control, state.SessionID
}

func newDesktopHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 8
	transport.MaxIdleConnsPerHost = 2
	transport.MaxConnsPerHost = 2
	transport.IdleConnTimeout = 30 * time.Second
	return &http.Client{Timeout: timeout, Transport: transport}
}

func setDesktopProcessDPIAwareness() {
	// PER_MONITOR_AWARE_V2 must be selected before Walk creates the tray
	// window. Otherwise Windows virtualizes GetSystemMetrics/BitBlt coordinates
	// and the captured frame is cropped on scaled or multi-monitor desktops.
	if procSetProcessDPIContext.Find() == nil {
		const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		if result, _, _ := procSetProcessDPIContext.Call(perMonitorAwareV2); result != 0 {
			return
		}
	}
	_, _, _ = procSetProcessDPIAware.Call()
}

func currentDesktopSessionIdentifier() string {
	value := desktopSessionIdentifier.Load()
	if value == nil {
		return ""
	}
	identifier, _ := value.(string)
	return identifier
}

func reportDesktopStatus(ctx context.Context, client *http.Client, access desktopAgentAccess, sessionID, statusError string) error {
	payload, err := json.Marshal(map[string]string{"error": statusError})
	if err != nil {
		return err
	}
	response, err := desktopRequest(ctx, client, access, http.MethodPost, "/api/desktop/agent/sessions/"+sessionID+"/status", bytes.NewReader(payload), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("desktop status: HTTP %d", response.StatusCode)
	}
	return nil
}

func desktopWait(ctx context.Context, delay time.Duration, timer *desktopWaitTimer) bool {
	if delay > 0 && delay <= 25*time.Millisecond && timer != nil && timer.handle != 0 {
		// Go's Windows timer ultimately shares the scheduler/timer-queue cadence
		// and can still wake on 15.6 ms boundaries under a service account even
		// after timeBeginPeriod(1). A high-resolution waitable timer gives the
		// capture loop a real sub-millisecond deadline. Use bounded 25 ms waits so
		// cancellation remains responsive without an auxiliary waiting thread.
		dueTime := -max(int64(1), delay.Nanoseconds()/100)
		if result, _, _ := procSetWaitableTimerEx.Call(timer.handle, uintptr(unsafe.Pointer(&dueTime)), 0, 0, 0, 0, 0); result != 0 {
			const waitObject0 = 0
			if result, _, _ := procWaitForSingleObject.Call(timer.handle, uintptr(max(1, delay.Milliseconds()+5))); result == waitObject0 {
				return ctx.Err() == nil
			}
		}
	}
	fallbackTimer := time.NewTimer(delay)
	defer fallbackTimer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-fallbackTimer.C:
		return true
	}
}

func desktopRequest(ctx context.Context, client *http.Client, access desktopAgentAccess, method, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, access.ServerURL+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Genesis-Device-Id", access.DeviceID)
	request.Header.Set("Authorization", "Desktop "+access.DesktopSecret)
	request.Header.Set("User-Agent", "RemoteIt-Desktop/"+version)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return client.Do(request)
}

func fetchDesktopOffer(ctx context.Context, client *http.Client, access desktopAgentAccess) (desktopSessionOffer, bool, error) {
	response, err := desktopRequest(ctx, client, access, http.MethodGet, "/api/desktop/agent/session", nil, nil)
	if err != nil {
		return desktopSessionOffer{}, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return desktopSessionOffer{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return desktopSessionOffer{}, false, fmt.Errorf("desktop session: HTTP %d", response.StatusCode)
	}
	var offer desktopSessionOffer
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<10)).Decode(&offer); err != nil {
		return desktopSessionOffer{}, false, err
	}
	return offer, offer.ID != "", nil
}

func uploadDesktopFrame(ctx context.Context, client *http.Client, access desktopAgentAccess, sessionID string, capture desktopCapture) error {
	headers := map[string]string{
		"Content-Type":               "image/jpeg",
		"X-RemoteIt-Width":           strconv.Itoa(capture.FrameWidth),
		"X-RemoteIt-Height":          strconv.Itoa(capture.FrameHeight),
		"X-RemoteIt-Capture-Ms":      strconv.Itoa(capture.CaptureMillis),
		"X-RemoteIt-Copy-Ms":         strconv.Itoa(capture.CopyMillis),
		"X-RemoteIt-Scale-Ms":        strconv.Itoa(capture.ScaleMillis),
		"X-RemoteIt-Encode-Ms":       strconv.Itoa(capture.EncodeMillis),
		"X-RemoteIt-Capture-Backend": capture.CaptureBackend,
	}
	response, err := desktopRequest(ctx, client, access, http.MethodPost, "/api/desktop/agent/sessions/"+sessionID+"/frame", bytes.NewReader(capture.JPEG), headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("desktop frame: HTTP %d", response.StatusCode)
	}
	return nil
}

type desktopFrameStreamClient struct {
	connection *websocket.Conn
	sessionID  string
	accessKey  string
	lane       int
	retryAfter time.Time
	readCancel context.CancelFunc
	inputs     chan<- []desktopInput
	active     *atomic.Bool
}

func (stream *desktopFrameStreamClient) Close() {
	if stream.readCancel != nil {
		stream.readCancel()
		stream.readCancel = nil
	}
	if stream.active != nil {
		stream.active.Store(false)
	}
	if stream.connection != nil {
		_ = stream.connection.Close(websocket.StatusNormalClosure, "")
	}
	stream.connection = nil
	stream.sessionID = ""
	stream.accessKey = ""
}

func desktopWebSocketURL(serverURL, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(serverURL, "/") + path)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", errors.New("desktop stream server URL must use http or https")
	}
	return parsed.String(), nil
}

func (stream *desktopFrameStreamClient) send(ctx context.Context, client *http.Client, upload desktopFrameUpload) error {
	key := upload.access.ServerURL + "\x00" + upload.access.DeviceID + "\x00" + upload.access.DesktopSecret
	if stream.connection != nil && stream.active != nil && !stream.active.Load() {
		stream.Close()
	}
	if stream.connection != nil && (stream.sessionID != upload.sessionID || stream.accessKey != key) {
		stream.Close()
	}
	if stream.connection == nil {
		if time.Now().Before(stream.retryAfter) {
			return errors.New("desktop websocket temporarily unavailable")
		}
		endpoint, err := desktopWebSocketURL(upload.access.ServerURL, "/api/desktop/agent/sessions/"+upload.sessionID+"/stream?lane="+strconv.Itoa(stream.lane))
		if err != nil {
			stream.retryAfter = time.Now().Add(15 * time.Second)
			return err
		}
		headers := http.Header{}
		headers.Set("X-Genesis-Device-Id", upload.access.DeviceID)
		headers.Set("Authorization", "Desktop "+upload.access.DesktopSecret)
		headers.Set("User-Agent", "RemoteIt-Desktop/"+version)
		connection, response, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPClient: client, HTTPHeader: headers, CompressionMode: websocket.CompressionDisabled})
		if err != nil && response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err != nil {
			stream.retryAfter = time.Now().Add(15 * time.Second)
			return err
		}
		stream.connection = connection
		stream.sessionID = upload.sessionID
		stream.accessKey = key
		stream.retryAfter = time.Time{}
		readCtx, cancel := context.WithCancel(ctx)
		stream.readCancel = cancel
		if stream.active != nil {
			stream.active.Store(true)
		}
		go stream.readInputs(readCtx, connection)
	}
	backend := []byte(upload.capture.CaptureBackend)
	if len(backend) > 48 {
		backend = backend[:48]
	}
	payload := make([]byte, 26+len(backend)+len(upload.capture.JPEG))
	copy(payload[:4], "RIT3")
	binary.BigEndian.PutUint32(payload[4:8], uint32(upload.capture.FrameWidth))
	binary.BigEndian.PutUint32(payload[8:12], uint32(upload.capture.FrameHeight))
	payload[12] = byte(min(255, max(0, upload.capture.CaptureMillis)))
	payload[13] = byte(min(255, max(0, upload.capture.CopyMillis)))
	payload[14] = byte(min(255, max(0, upload.capture.ScaleMillis)))
	payload[15] = byte(min(255, max(0, upload.capture.EncodeMillis)))
	payload[16] = 0 // reserved for future transport diagnostics
	payload[17] = byte(len(backend))
	binary.BigEndian.PutUint64(payload[18:26], upload.sequence)
	copy(payload[26:], backend)
	copy(payload[26+len(backend):], upload.capture.JPEG)
	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := stream.connection.Write(writeCtx, websocket.MessageBinary, payload)
	cancel()
	if err != nil {
		stream.Close()
		stream.retryAfter = time.Now().Add(2 * time.Second)
	}
	return err
}

func (stream *desktopFrameStreamClient) readInputs(ctx context.Context, connection *websocket.Conn) {
	defer func() {
		if stream.active != nil {
			stream.active.Store(false)
		}
	}()
	for {
		messageType, payload, err := connection.Read(ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageText || len(payload) == 0 || len(payload) > 64<<10 {
			continue
		}
		events, decodeErr := decodeDesktopInputStreamMessage(payload)
		if decodeErr != nil {
			continue
		}
		select {
		case stream.inputs <- events:
		case <-ctx.Done():
			return
		}
	}
}

func decodeDesktopInputStreamMessage(payload []byte) ([]desktopInput, error) {
	var message struct {
		Events []struct {
			Event desktopInput `json:"event"`
		} `json:"events"`
	}
	if len(payload) == 0 || len(payload) > 64<<10 || json.Unmarshal(payload, &message) != nil || len(message.Events) == 0 || len(message.Events) > 64 {
		return nil, errors.New("invalid desktop input stream message")
	}
	events := make([]desktopInput, 0, len(message.Events))
	for _, queued := range message.Events {
		events = append(events, queued.Event)
	}
	return events, nil
}

// coalesceDesktopInput drops obsolete cursor positions without ever removing
// click, wheel, keyboard or text actions. Button events contain their own
// coordinates, so retaining only the newest free pointer move cannot change a
// click target and prevents a high-refresh viewer from building input latency.
func coalesceDesktopInput(events []desktopInput) []desktopInput {
	result := make([]desktopInput, 0, len(events))
	for _, event := range events {
		if event.Type == "pointer" && event.Action == "move" {
			filtered := result[:0]
			for _, pending := range result {
				if pending.Type != "pointer" || pending.Action != "move" {
					filtered = append(filtered, pending)
				}
			}
			result = filtered
		}
		result = append(result, event)
	}
	return result
}

// runDesktopStreamInputDispatcher keeps network input entirely off the capture
// cadence thread. The previous main-loop drain made a continuous wheel/trackpad
// gesture contend with DXGI and reduced a fast 30/60 FPS producer to roughly
// 15-20 FPS even though scale+encode itself took only 5-10ms. This dispatcher
// coalesces only obsolete free pointer positions; clicks, keys and wheel events
// retain their order and are handed to the dedicated locked Windows input
// thread immediately.
func runDesktopStreamInputDispatcher(ctx context.Context, batches <-chan []desktopInput, tasks chan desktopInputTask, latestCapture *atomic.Value, latestInputNanos *atomic.Int64) {
	for {
		select {
		case <-ctx.Done():
			return
		case events := <-batches:
			if len(events) == 0 {
				continue
			}
			pending := append([]desktopInput(nil), events...)
			for len(pending) < 64 {
				select {
				case more := <-batches:
					pending = append(pending, more...)
				default:
					goto dispatch
				}
			}
		dispatch:
			pending = coalesceDesktopInput(pending)
			if len(pending) == 0 {
				continue
			}
			latestInputNanos.Store(time.Now().UnixNano())
			capture, _ := latestCapture.Load().(desktopCapture)
			task := desktopInputTask{events: pending, capture: capture}
			select {
			case tasks <- task:
			case <-ctx.Done():
				return
			}
		}
	}
}

func runDesktopFrameUploadLane(ctx context.Context, client *http.Client, lane int, uploads <-chan desktopFrameUpload, results chan<- desktopFrameUploadResult, inputs chan<- []desktopInput, active *atomic.Bool, httpFallback bool) {
	stream := &desktopFrameStreamClient{lane: lane, inputs: inputs, active: active}
	defer stream.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case upload := <-uploads:
			startedAt := time.Now()
			err := stream.send(ctx, client, upload)
			if err != nil && httpFallback {
				// HTTP remains a tested compatibility path for old reverse proxies and
				// during rolling server upgrades. It never queues more than the newest
				// complete frame, so falling back cannot accumulate video latency.
				err = uploadDesktopFrame(ctx, client, upload.access, upload.sessionID, upload.capture)
			}
			if err != nil && !httpFallback {
				// The second lane is an acceleration path. The primary lane continues
				// to provide both video and the HTTP compatibility fallback while this
				// lane reconnects, so do not surface a false session error to the user.
				continue
			}
			result := desktopFrameUploadResult{sessionID: upload.sessionID, capture: upload.capture, duration: time.Since(startedAt), err: err}
			select {
			case results <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func runDesktopFrameUploader(ctx context.Context, client *http.Client, uploads <-chan desktopFrameUpload, results chan<- desktopFrameUploadResult, inputs chan<- []desktopInput, active *atomic.Bool) {
	// A single websocket becomes the limiting factor for high-quality JPEG at
	// 30/60 FPS even when capture and encoding take only a few milliseconds.
	// Six independent, latest-only lanes preserve frame quality and expand the
	// available transport window without coupling keyboard/mouse input to a
	// slow image write. Producer sequence numbers let the server discard a late
	// frame when lanes complete out of order.
	lanes := [6]chan desktopFrameUpload{
		make(chan desktopFrameUpload, 1), make(chan desktopFrameUpload, 1),
		make(chan desktopFrameUpload, 1), make(chan desktopFrameUpload, 1),
		make(chan desktopFrameUpload, 1), make(chan desktopFrameUpload, 1),
	}
	go runDesktopFrameUploadLane(ctx, client, 0, lanes[0], results, inputs, active, true)
	go runDesktopFrameUploadLane(ctx, client, 1, lanes[1], results, nil, nil, false)
	go runDesktopFrameUploadLane(ctx, client, 2, lanes[2], results, nil, nil, false)
	go runDesktopFrameUploadLane(ctx, client, 3, lanes[3], results, nil, nil, false)
	go runDesktopFrameUploadLane(ctx, client, 4, lanes[4], results, nil, nil, false)
	go runDesktopFrameUploadLane(ctx, client, 5, lanes[5], results, nil, nil, false)
	lane := 0
	for {
		select {
		case <-ctx.Done():
			return
		case upload := <-uploads:
			enqueueLatestDesktopFrame(lanes[lane], upload)
			lane = (lane + 1) % len(lanes)
		}
	}
}

func enqueueLatestDesktopFrame(uploads chan desktopFrameUpload, upload desktopFrameUpload) {
	select {
	case uploads <- upload:
		return
	default:
	}
	// One frame may be in flight and one may be waiting. If the waiting frame
	// is already stale, replace it with the newest complete image. Remote input
	// therefore never waits behind seconds of obsolete screen history.
	select {
	case <-uploads:
	default:
	}
	select {
	case uploads <- upload:
	default:
	}
}

func drainDesktopFrameUploads(uploads chan desktopFrameUpload) {
	for {
		select {
		case <-uploads:
		default:
			return
		}
	}
}

func fetchDesktopInputs(ctx context.Context, client *http.Client, access desktopAgentAccess, sessionID string) ([]desktopInput, error) {
	response, err := desktopRequest(ctx, client, access, http.MethodGet, "/api/desktop/agent/sessions/"+sessionID+"/inputs", nil, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("desktop input: HTTP %d", response.StatusCode)
	}
	var payload struct {
		Events []struct {
			Event desktopInput `json:"event"`
		} `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&payload); err != nil {
		return nil, err
	}
	events := make([]desktopInput, 0, len(payload.Events))
	for _, item := range payload.Events {
		events = append(events, item.Event)
	}
	return events, nil
}

// desktopCapturer owns its GDI objects for the lifetime of the agent loop.
// Recreating a DC and a full-screen bitmap for every frame adds visible input
// latency at 30/60 FPS and puts avoidable pressure on the Windows GDI heap.
type desktopCapturer struct {
	screenDC                uintptr
	memoryDC                uintptr
	bitmap                  uintptr
	previous                uintptr
	screenX                 int
	screenY                 int
	width                   int
	height                  int
	interactionWidth        int
	screenWidth             int
	screenHeight            int
	pixels                  []byte
	dxgiPixels              []byte
	dxgiScaled              []byte
	motionScaled            []byte
	cursorFrame             []byte
	dxgiScaleX              []int32
	dxgiScaleWeight         []int32
	motionScaleX            []int32
	motionScaleWeight       []int32
	nativeMotionScaleX      []int32
	nativeMotionScaleWeight []int32
	frame                   *image.RGBA
	info                    bitmapInfo
	encoder                 desktopJPEGEncoder
	fast                    desktopFastCapturer
	lastJPEG                []byte
	lastFrameWidth          int
	lastFrameHeight         int
	lastQuality             int
	lastChroma              desktopJPEGChroma
	lastCursor              desktopCursorState
	lastCursorVisible       bool
}

func (capturer *desktopCapturer) Close() {
	capturer.encoder.Close()
	capturer.fast.Close()
	if capturer.memoryDC != 0 && capturer.previous != 0 {
		procSelectObject.Call(capturer.memoryDC, capturer.previous)
	}
	if capturer.bitmap != 0 {
		procDeleteObject.Call(capturer.bitmap)
	}
	if capturer.memoryDC != 0 {
		procDeleteDC.Call(capturer.memoryDC)
	}
	if capturer.screenDC != 0 {
		procReleaseDC.Call(0, capturer.screenDC)
	}
	*capturer = desktopCapturer{}
}

func (capturer *desktopCapturer) ensure(targetFPS int) error {
	screenX, _, _ := procGetSystemMetrics.Call(76)
	screenY, _, _ := procGetSystemMetrics.Call(77)
	screenWidth, _, _ := procGetSystemMetrics.Call(78)
	screenHeight, _, _ := procGetSystemMetrics.Call(79)
	x, y := int(int32(screenX)), int(int32(screenY))
	fullWidth, fullHeight := int(int32(screenWidth)), int(int32(screenHeight))
	if fullWidth <= 0 || fullHeight <= 0 || fullWidth > 12000 || fullHeight > 12000 {
		return errors.New("некорректный размер рабочего стола")
	}
	profile := desktopProfileForFPS(targetFPS)
	width := min(fullWidth, profile.maxWidth)
	height := max(1, fullHeight*width/fullWidth)
	interactionWidth := desktopInteractionWidth(targetFPS, width)
	if capturer.screenDC != 0 && capturer.screenX == x && capturer.screenY == y && capturer.screenWidth == fullWidth && capturer.screenHeight == fullHeight && capturer.width == width && capturer.height == height && capturer.interactionWidth == interactionWidth {
		return nil
	}
	capturer.Close()
	capturer.screenX, capturer.screenY = x, y
	capturer.screenWidth, capturer.screenHeight = fullWidth, fullHeight
	capturer.width, capturer.height = width, height
	capturer.interactionWidth = interactionWidth
	capturer.dxgiPixels = make([]byte, fullWidth*fullHeight*4)
	capturer.dxgiScaled = make([]byte, width*height*4)
	motionWidth := interactionWidth
	motionHeight := max(1, height*motionWidth/width)
	capturer.motionScaled = make([]byte, motionWidth*motionHeight*4)
	capturer.cursorFrame = make([]byte, width*height*4)
	capturer.dxgiScaleX = make([]int32, width)
	capturer.dxgiScaleWeight = make([]int32, width)
	capturer.motionScaleX = make([]int32, motionWidth)
	capturer.motionScaleWeight = make([]int32, motionWidth)
	capturer.nativeMotionScaleX = make([]int32, motionWidth)
	capturer.nativeMotionScaleWeight = make([]int32, motionWidth)
	for targetX := range capturer.dxgiScaleX {
		sourceX256 := ((targetX*2+1)*fullWidth*128)/width - 128
		sourceX256 = max(0, min(sourceX256, (fullWidth-1)*256))
		baseX := sourceX256 >> 8
		weightX := sourceX256 & 255
		if baseX >= fullWidth-1 {
			baseX = fullWidth - 1
			weightX = 0
		}
		capturer.dxgiScaleX[targetX] = int32(baseX)
		capturer.dxgiScaleWeight[targetX] = int32(weightX)
	}
	for targetX := range capturer.motionScaleX {
		sourceX256 := ((targetX*2+1)*width*128)/motionWidth - 128
		sourceX256 = max(0, min(sourceX256, (width-1)*256))
		baseX := sourceX256 >> 8
		weightX := sourceX256 & 255
		if baseX >= width-1 {
			baseX = width - 1
			weightX = 0
		}
		capturer.motionScaleX[targetX] = int32(baseX)
		capturer.motionScaleWeight[targetX] = int32(weightX)
		nativeX256 := ((targetX*2+1)*fullWidth*128)/motionWidth - 128
		nativeX256 = max(0, min(nativeX256, (fullWidth-1)*256))
		nativeBaseX := nativeX256 >> 8
		nativeWeightX := nativeX256 & 255
		if nativeBaseX >= fullWidth-1 {
			nativeBaseX = fullWidth - 1
			nativeWeightX = 0
		}
		capturer.nativeMotionScaleX[targetX] = int32(nativeBaseX)
		capturer.nativeMotionScaleWeight[targetX] = int32(nativeWeightX)
	}
	capturer.screenDC, _, _ = procGetDC.Call(0)
	if capturer.screenDC == 0 {
		capturer.Close()
		return errors.New("не удалось открыть рабочий стол")
	}
	capturer.memoryDC, _, _ = procCreateCompatibleDC.Call(capturer.screenDC)
	if capturer.memoryDC == 0 {
		capturer.Close()
		return errors.New("не удалось создать буфер экрана")
	}
	// The GDI fallback writes directly into the selected output profile. On the
	// production VMware display driver, copying the full 2560x1920 surface into
	// system memory took ~285 ms before any scaling. A profile-sized DIB lets
	// the driver discard source pixels during the transfer and avoids moving a
	// second full native frame through RAM. DXGI still uses its independent
	// native buffer and the high-quality native scaler above.
	capturer.info = bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}}
	var pixelMemory unsafe.Pointer
	capturer.bitmap, _, _ = procCreateDIBSection.Call(
		capturer.screenDC,
		uintptr(unsafe.Pointer(&capturer.info)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&pixelMemory)),
		0,
		0,
	)
	if capturer.bitmap == 0 {
		capturer.Close()
		return errors.New("не удалось создать кадр")
	}
	capturer.previous, _, _ = procSelectObject.Call(capturer.memoryDC, capturer.bitmap)
	if pixelMemory == nil {
		capturer.Close()
		return errors.New("desktop frame buffer is unavailable")
	}
	capturer.pixels = unsafe.Slice((*byte)(pixelMemory), width*height*4)
	capturer.frame = image.NewRGBA(image.Rect(0, 0, width, height))
	return nil
}

func (capturer *desktopCapturer) CaptureJPEG(targetFPS int, interactive bool, cursorVisible bool) (desktopCapture, error) {
	if err := capturer.ensure(targetFPS); err != nil {
		return desktopCapture{}, err
	}
	// During active mouse/keyboard input prefer immediate motion over spending
	// bandwidth on visually lossless chroma in every intermediate frame. The
	// geometry remains unchanged, so pointer mapping cannot jump. A sharp 4:4:4
	// frame is emitted automatically after 350 ms of inactivity.
	profile := desktopProfileForInteraction(targetFPS, interactive)
	captureStartedAt := time.Now()
	copyStartedAt := captureStartedAt
	copyMillis := 0
	scaleMillis := 0
	captureBackend := "dxgi"
	if interactive {
		captureBackend += "-motion"
	}
	framePixels := capturer.dxgiPixels
	frameWidth, frameHeight := capturer.screenWidth, capturer.screenHeight
	desiredWidth, desiredHeight := capturer.width, capturer.height
	interactionWidth := desktopInteractionWidth(targetFPS, desiredWidth)
	if interactive && desiredWidth > interactionWidth {
		desiredWidth = interactionWidth
		desiredHeight = max(1, capturer.height*desiredWidth/capturer.width)
	}
	fastResult := -1
	cursor := currentDesktopCursorState()
	// Desktop Duplication operates at the native output resolution. The former
	// <=1920 guard accidentally forced every high-DPI/4K display through GDI,
	// even when the low-latency DXGI path was available.
	fastResult = capturer.fast.CaptureBGRA(framePixels, frameWidth, frameHeight)
	copyMillis = int(time.Since(copyStartedAt).Milliseconds())
	if fastResult == 0 && len(capturer.lastJPEG) > 0 &&
		(!cursorVisible || cursor == capturer.lastCursor) &&
		cursorVisible == capturer.lastCursorVisible &&
		capturer.lastFrameWidth == desiredWidth && capturer.lastFrameHeight == desiredHeight &&
		capturer.lastQuality == profile.quality && capturer.lastChroma == profile.chroma {
		return desktopCapture{
			JPEG:           capturer.lastJPEG,
			FrameWidth:     capturer.lastFrameWidth,
			FrameHeight:    capturer.lastFrameHeight,
			ScreenX:        capturer.screenX,
			ScreenY:        capturer.screenY,
			ScreenWidth:    capturer.screenWidth,
			ScreenHeight:   capturer.screenHeight,
			CaptureMillis:  int(time.Since(captureStartedAt).Milliseconds()),
			CopyMillis:     copyMillis,
			CaptureBackend: "dxgi-wait-reuse",
		}, nil
	}
	if fastResult == 0 && len(capturer.lastJPEG) > 0 {
		// The desktop texture is unchanged, but the independently reported cursor
		// moved or changed shape. Reuse the last DXGI base pixels and recompose only
		// the pointer; falling through to GDI here would reintroduce 100-300 ms
		// captures whenever the user moved the mouse over a static screen.
		captureBackend = "dxgi-cursor"
		if interactive {
			captureBackend += "-motion"
		}
	} else if fastResult < 0 || (fastResult == 0 && len(capturer.lastJPEG) == 0) {
		captureBackend = capturer.fast.BackendDetail()
		framePixels = capturer.pixels
		frameWidth, frameHeight = capturer.width, capturer.height
		const sourceCopy = 0x00CC0020 // SRCCOPY
		copied := uintptr(0)
		if capturer.screenWidth == capturer.width && capturer.screenHeight == capturer.height {
			copied, _, _ = procBitBlt.Call(capturer.memoryDC, 0, 0, uintptr(frameWidth), uintptr(frameHeight), capturer.screenDC, uintptr(capturer.screenX), uintptr(capturer.screenY), sourceCopy)
			captureBackend += "-bitblt"
		} else {
			// COLORONCOLOR is intentionally used only on the secure GDI fallback.
			// HALFTONE forced VMware to recompose the native surface and had the
			// same ~300 ms cost as a full BitBlt. At the normal 75% downscale this
			// mode remains readable, while TurboJPEG keeps chroma 4:4:4.
			procSetStretchBltMode.Call(capturer.memoryDC, 3) // COLORONCOLOR
			copied, _, _ = procStretchBlt.Call(capturer.memoryDC, 0, 0, uintptr(frameWidth), uintptr(frameHeight), capturer.screenDC, uintptr(capturer.screenX), uintptr(capturer.screenY), uintptr(capturer.screenWidth), uintptr(capturer.screenHeight), sourceCopy)
			captureBackend += "-stretch-color"
		}
		if copied == 0 {
			return desktopCapture{}, errors.New("не удалось скопировать экран")
		}
		copyMillis = int(time.Since(copyStartedAt).Milliseconds())
	}
	if frameWidth != desiredWidth || frameHeight != desiredHeight {
		// Desktop Duplication exposes the native desktop surface. Encoding that
		// full surface can be substantially heavier than the selected profile.
		// Scale native DXGI pixels directly to the final interaction size: the old
		// native -> 2048 -> 1600 chain consumed ~36 ms and made 60 FPS slower than
		// 30 FPS. Persistent lookup tables keep the single pass allocation-free.
		scaleStartedAt := time.Now()
		targetPixels := capturer.dxgiScaled
		scaleX, scaleWeight := capturer.dxgiScaleX, capturer.dxgiScaleWeight
		if desiredWidth < capturer.width {
			targetPixels = capturer.motionScaled
			if frameWidth == capturer.screenWidth {
				scaleX, scaleWeight = capturer.nativeMotionScaleX, capturer.nativeMotionScaleWeight
			} else {
				scaleX, scaleWeight = capturer.motionScaleX, capturer.motionScaleWeight
			}
		}
		scaleOK := false
		if interactive {
			scaleOK = scaleDesktopBGRARealtime(framePixels, frameWidth, frameHeight, targetPixels, desiredWidth, desiredHeight, scaleX, scaleWeight)
			captureBackend += "-realtime-scale"
		} else {
			scaleOK = scaleDesktopBGRAFast(framePixels, frameWidth, frameHeight, targetPixels, desiredWidth, desiredHeight, scaleX, scaleWeight)
		}
		if !scaleOK {
			return desktopCapture{}, errors.New("desktop frame scaling failed")
		}
		scaleMillis = int(time.Since(scaleStartedAt).Milliseconds())
		framePixels = targetPixels
		frameWidth, frameHeight = desiredWidth, desiredHeight
	}
	// Desktop Duplication exposes the pointer separately from the texture. The
	// viewer explicitly requests composition only for mobile trackpad mode.
	// Desktop and direct-touch clients rely on their local/finger position, so
	// skipping this copy removes the duplicate pointer and also avoids needless
	// JPEG work whenever the remote user's cursor moves over a static desktop.
	if cursorVisible {
		if len(capturer.cursorFrame) != len(framePixels) {
			capturer.cursorFrame = make([]byte, len(framePixels))
		}
		copy(capturer.cursorFrame, framePixels)
		capturer.drawCursor(capturer.cursorFrame, frameWidth, frameHeight, cursor)
		framePixels = capturer.cursorFrame
	}
	// TurboJPEG consumes packed BGRA directly at every supported resolution,
	// including native 2560 px and 4K profiles. The previous >1920 branch first
	// converted the entire surface to RGBA, encoded it in pure Go and silently
	// reduced it back to 1920 px. That caused both visible pixelation and the
	// 4-7 FPS ceiling measured on a 2560 px desktop.
	captureMillis := int(time.Since(captureStartedAt).Milliseconds())
	encodeStartedAt := time.Now()
	encoded, encodeErr := capturer.encoder.EncodeBGRA(framePixels, frameWidth, frameHeight, profile.quality, profile.chroma)
	if encodeErr != nil {
		return desktopCapture{}, encodeErr
	}
	if len(encoded) > desktopMaximumFrameBytes {
		return desktopCapture{}, errors.New("desktop frame exceeds the size limit")
	}
	capturer.lastJPEG = encoded
	capturer.lastFrameWidth, capturer.lastFrameHeight = frameWidth, frameHeight
	capturer.lastQuality, capturer.lastChroma = profile.quality, profile.chroma
	capturer.lastCursor = cursor
	capturer.lastCursorVisible = cursorVisible
	return desktopCapture{JPEG: capturer.lastJPEG, FrameWidth: frameWidth, FrameHeight: frameHeight, ScreenX: capturer.screenX, ScreenY: capturer.screenY, ScreenWidth: capturer.screenWidth, ScreenHeight: capturer.screenHeight, CaptureMillis: captureMillis, CopyMillis: copyMillis, ScaleMillis: scaleMillis, EncodeMillis: int(time.Since(encodeStartedAt).Milliseconds()), CaptureBackend: captureBackend}, nil
}

func currentDesktopCursorState() desktopCursorState {
	info := desktopCursorInfo{Size: uint32(unsafe.Sizeof(desktopCursorInfo{}))}
	result, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 || info.Flags&1 == 0 || info.Cursor == 0 { // CURSOR_SHOWING
		return desktopCursorState{}
	}
	return desktopCursorState{Visible: true, X: int(info.Position.X), Y: int(info.Position.Y), Handle: info.Cursor}
}

func (capturer *desktopCapturer) drawCursor(frame []byte, frameWidth, frameHeight int, cursor desktopCursorState) {
	if !cursor.Visible || cursor.Handle == 0 || capturer.memoryDC == 0 || len(capturer.pixels) < frameWidth*frameHeight*4 || len(frame) < frameWidth*frameHeight*4 {
		return
	}
	icon := desktopIconInfo{}
	if result, _, _ := procGetIconInfo.Call(cursor.Handle, uintptr(unsafe.Pointer(&icon))); result == 0 {
		return
	}
	if icon.Mask != 0 {
		defer procDeleteObject.Call(icon.Mask)
	}
	if icon.Color != 0 {
		defer procDeleteObject.Call(icon.Color)
	}
	cursorWidthRaw, _, _ := procGetSystemMetrics.Call(13)  // SM_CXCURSOR
	cursorHeightRaw, _, _ := procGetSystemMetrics.Call(14) // SM_CYCURSOR
	cursorWidth := max(1, int(int32(cursorWidthRaw))*frameWidth/max(1, capturer.screenWidth))
	cursorHeight := max(1, int(int32(cursorHeightRaw))*frameHeight/max(1, capturer.screenHeight))
	hotspotX := int(icon.HotspotX) * frameWidth / max(1, capturer.screenWidth)
	hotspotY := int(icon.HotspotY) * frameHeight / max(1, capturer.screenHeight)
	left := (cursor.X-capturer.screenX)*frameWidth/max(1, capturer.screenWidth) - hotspotX
	top := (cursor.Y-capturer.screenY)*frameHeight/max(1, capturer.screenHeight) - hotspotY
	clipLeft, clipTop := max(0, left), max(0, top)
	clipRight, clipBottom := min(frameWidth, left+cursorWidth), min(frameHeight, top+cursorHeight)
	if clipLeft >= clipRight || clipTop >= clipBottom {
		return
	}
	stride := frameWidth * 4
	for y := clipTop; y < clipBottom; y++ {
		start, end := y*stride+clipLeft*4, y*stride+clipRight*4
		copy(capturer.pixels[start:end], frame[start:end])
	}
	const drawNormal = 0x0003 // DI_MASK | DI_IMAGE
	if result, _, _ := procDrawIconEx.Call(capturer.memoryDC, uintptr(left), uintptr(top), cursor.Handle, uintptr(cursorWidth), uintptr(cursorHeight), 0, 0, drawNormal); result == 0 {
		return
	}
	for y := clipTop; y < clipBottom; y++ {
		start, end := y*stride+clipLeft*4, y*stride+clipRight*4
		copy(frame[start:end], capturer.pixels[start:end])
	}
}

func scaleDesktopBGRA(source []byte, sourceWidth, sourceHeight int, target []byte, targetWidth, targetHeight int, scaleX, scaleWeight []int32) {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 || targetHeight <= 0 || len(scaleX) < targetWidth || len(scaleWeight) < targetWidth {
		return
	}
	// Bilinear fixed-point scaling keeps small fonts and one-pixel window borders
	// readable. The previous nearest-neighbour sampler was fast, but it produced
	// the blocky/pixelated text visible whenever a 1600/1920 px desktop was fitted
	// into the remote-control viewport.
	for targetY := 0; targetY < targetHeight; targetY++ {
		sourceY256 := ((targetY*2+1)*sourceHeight*128)/targetHeight - 128
		sourceY256 = max(0, min(sourceY256, (sourceHeight-1)*256))
		sourceY := sourceY256 >> 8
		weightY := sourceY256 & 255
		if sourceY >= sourceHeight-1 {
			sourceY = sourceHeight - 1
			weightY = 0
		}
		nextY := min(sourceY+1, sourceHeight-1)
		sourceRow := sourceY * sourceWidth * 4
		nextRow := nextY * sourceWidth * 4
		targetRow := targetY * targetWidth * 4
		for targetX := 0; targetX < targetWidth; targetX++ {
			sourceX := int(scaleX[targetX])
			nextX := min(sourceX+1, sourceWidth-1)
			weightX := int(scaleWeight[targetX])
			sourceOffset := sourceRow + sourceX*4
			sourceRight := sourceRow + nextX*4
			sourceBottom := nextRow + sourceX*4
			sourceBottomRight := nextRow + nextX*4
			targetOffset := targetRow + targetX*4
			for channel := 0; channel < 4; channel++ {
				top := int(source[sourceOffset+channel])*(256-weightX) + int(source[sourceRight+channel])*weightX
				bottom := int(source[sourceBottom+channel])*(256-weightX) + int(source[sourceBottomRight+channel])*weightX
				target[targetOffset+channel] = byte((top*(256-weightY) + bottom*weightY + 32768) >> 16)
			}
		}
	}
}

func captureDesktopJPEG() (desktopCapture, error) {
	screenX, _, _ := procGetSystemMetrics.Call(76)
	screenY, _, _ := procGetSystemMetrics.Call(77)
	screenWidth, _, _ := procGetSystemMetrics.Call(78)
	screenHeight, _, _ := procGetSystemMetrics.Call(79)
	width, height := int(int32(screenWidth)), int(int32(screenHeight))
	if width <= 0 || height <= 0 || width > 12000 || height > 12000 {
		return desktopCapture{}, errors.New("некорректный размер рабочего стола")
	}
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return desktopCapture{}, errors.New("не удалось открыть рабочий стол")
	}
	defer procReleaseDC.Call(0, screenDC)
	memoryDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memoryDC == 0 {
		return desktopCapture{}, errors.New("не удалось создать буфер экрана")
	}
	defer procDeleteDC.Call(memoryDC)
	bitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return desktopCapture{}, errors.New("не удалось создать кадр")
	}
	defer procDeleteObject.Call(bitmap)
	previous, _, _ := procSelectObject.Call(memoryDC, bitmap)
	defer procSelectObject.Call(memoryDC, previous)
	const sourceCopyWithLayers = 0x40CC0020
	if copied, _, _ := procBitBlt.Call(memoryDC, 0, 0, uintptr(width), uintptr(height), screenDC, screenX, screenY, sourceCopyWithLayers); copied == 0 {
		return desktopCapture{}, errors.New("не удалось скопировать экран")
	}
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}}
	pixels := make([]byte, width*height*4)
	if rows, _, _ := procGetDIBits.Call(memoryDC, bitmap, 0, uintptr(height), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), 0); rows == 0 {
		return desktopCapture{}, errors.New("не удалось прочитать кадр")
	}
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for index := 0; index < len(pixels); index += 4 {
		frame.Pix[index] = pixels[index+2]
		frame.Pix[index+1] = pixels[index+1]
		frame.Pix[index+2] = pixels[index]
		frame.Pix[index+3] = 255
	}
	target := frame
	if width > 1920 {
		targetWidth := 1920
		targetHeight := max(1, height*targetWidth/width)
		target = scaleDesktopFrame(frame, targetWidth, targetHeight)
	}
	var encoded bytes.Buffer
	// Keep the legacy one-shot fallback aligned with the normal balanced profile.
	if err := jpeg.Encode(&encoded, target, &jpeg.Options{Quality: 90}); err != nil {
		return desktopCapture{}, err
	}
	if encoded.Len() > desktopMaximumFrameBytes {
		return desktopCapture{}, errors.New("кадр превышает допустимый размер")
	}
	return desktopCapture{JPEG: encoded.Bytes(), FrameWidth: target.Bounds().Dx(), FrameHeight: target.Bounds().Dy(), ScreenX: int(int32(screenX)), ScreenY: int(int32(screenY)), ScreenWidth: width, ScreenHeight: height}, nil
}

func scaleDesktopFrame(source *image.RGBA, width, height int) *image.RGBA {
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	sourceWidth, sourceHeight := source.Bounds().Dx(), source.Bounds().Dy()
	for y := 0; y < height; y++ {
		sourceY := y * sourceHeight / height
		for x := 0; x < width; x++ {
			sourceX := x * sourceWidth / width
			sourceIndex := source.PixOffset(sourceX, sourceY)
			targetIndex := target.PixOffset(x, y)
			copy(target.Pix[targetIndex:targetIndex+4], source.Pix[sourceIndex:sourceIndex+4])
		}
	}
	return target
}

func executeDesktopInput(event desktopInput, capture desktopCapture) error {
	switch event.Type {
	case "pointer":
		x := capture.ScreenX + event.X*capture.ScreenWidth/max(1, capture.FrameWidth)
		y := capture.ScreenY + event.Y*capture.ScreenHeight/max(1, capture.FrameHeight)
		if err := sendDesktopAbsolutePointer(x, y, capture, 0, 0); err != nil {
			return err
		}
		if event.Action == "down" || event.Action == "up" {
			flags := map[string]map[string]uint32{
				"left":   {"down": 0x0002, "up": 0x0004},
				"right":  {"down": 0x0008, "up": 0x0010},
				"middle": {"down": 0x0020, "up": 0x0040},
			}
			if buttonFlags, ok := flags[event.Button]; ok {
				if flag, ok := buttonFlags[event.Action]; ok {
					return sendDesktopMouseInput(desktopWindowsMouseInput{Flags: flag})
				}
			}
		}
	case "wheel":
		return sendDesktopMouseInput(desktopWindowsMouseInput{MouseData: uint32(int32(event.Delta)), Flags: 0x0800})
	case "key":
		flags := uint32(0)
		if desktopExtendedVirtualKey(event.KeyCode) {
			flags |= 0x0001
		}
		if strings.EqualFold(event.Action, "up") {
			flags |= 0x0002
		}
		return sendDesktopVirtualKey(uint16(event.KeyCode), flags)
	case "text":
		for _, codeUnit := range utf16.Encode([]rune(event.Text)) {
			if err := sendDesktopUnicodeKey(codeUnit, false); err != nil {
				return err
			}
			if err := sendDesktopUnicodeKey(codeUnit, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func resetDesktopInputState() {
	// Browser focus changes and interrupted requests can otherwise leave a
	// modifier or mouse button pressed remotely. Reset stateful inputs whenever
	// the stream mode/session changes so control cannot remain apparently frozen.
	for _, flag := range []uint32{0x0004, 0x0010, 0x0040} { // left/right/middle up
		_ = sendDesktopMouseInput(desktopWindowsMouseInput{Flags: flag})
	}
	for _, virtualKey := range []uint16{0x10, 0x11, 0x12, 0x5B, 0x5C} { // Shift/Ctrl/Alt/Win
		_ = sendDesktopVirtualKey(virtualKey, 0x0002)
	}
}

func sendDesktopAbsolutePointer(x, y int, capture desktopCapture, mouseData, extraFlags uint32) error {
	virtualWidth := max(1, capture.ScreenWidth-1)
	virtualHeight := max(1, capture.ScreenHeight-1)
	normalizedX := int32((x - capture.ScreenX) * 65535 / virtualWidth)
	normalizedY := int32((y - capture.ScreenY) * 65535 / virtualHeight)
	return sendDesktopMouseInput(desktopWindowsMouseInput{
		DX: normalizedX, DY: normalizedY, MouseData: mouseData,
		Flags: 0x0001 | 0x8000 | 0x4000 | extraFlags,
	})
}

func sendDesktopMouseInput(mouse desktopWindowsMouseInput) error {
	record := desktopWindowsInput{Kind: 0}
	*(*desktopWindowsMouseInput)(unsafe.Pointer(&record.Data[0])) = mouse
	return submitDesktopInput(&record)
}

func sendDesktopVirtualKey(virtualKey uint16, flags uint32) error {
	record := desktopWindowsInput{Kind: 1}
	keyboard := (*desktopWindowsKeyboardInput)(unsafe.Pointer(&record.Data[0]))
	*keyboard = desktopWindowsKeyboardInput{VirtualKey: virtualKey, Flags: flags}
	return submitDesktopInput(&record)
}

func desktopExtendedVirtualKey(virtualKey int) bool {
	switch virtualKey {
	case 33, 34, 35, 36, 37, 38, 39, 40, 45, 46, 91, 92:
		return true
	default:
		return false
	}
}

func sendDesktopUnicodeKey(codeUnit uint16, released bool) error {
	const (
		keyboardInputKind = 1
		unicodeFlag       = 0x0004
		keyUpFlag         = 0x0002
	)
	record := desktopWindowsInput{Kind: keyboardInputKind}
	flags := uint32(unicodeFlag)
	if released {
		flags |= keyUpFlag
	}
	keyboard := (*desktopWindowsKeyboardInput)(unsafe.Pointer(&record.Data[0]))
	*keyboard = desktopWindowsKeyboardInput{ScanCode: codeUnit, Flags: flags}
	return submitDesktopInput(&record)
}

func submitDesktopInput(record *desktopWindowsInput) error {
	inserted, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(record)), unsafe.Sizeof(*record))
	if inserted != 1 {
		if callErr == nil || errors.Is(callErr, windows.ERROR_SUCCESS) {
			callErr = errors.New("Windows отклонила событие ввода")
		}
		return fmt.Errorf("удалённый ввод не выполнен: %w", callErr)
	}
	return nil
}
