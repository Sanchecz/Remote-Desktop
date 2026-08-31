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
	"image/png"
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
	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
)

var desktopSessionActive atomic.Bool
var desktopSessionControl atomic.Bool
var desktopLastFrameUnix atomic.Int64
var desktopSessionIdentifier atomic.Value

const (
	// A detailed or high-entropy native 4K desktop can legitimately exceed the
	// former 8 MiB ceiling at q92 4:4:4. Keep a bounded 16 MiB wire limit and
	// re-encode only those exceptional frames using compact fallbacks.
	desktopMaximumFrameBytes      = 16 << 20
	desktopOversizeProfileBackoff = 5 * time.Second
)

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
	sasDesktop                 = windows.NewLazySystemDLL("sas.dll")
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
	procOpenClipboard          = user32Desktop.NewProc("OpenClipboard")
	procCloseClipboard         = user32Desktop.NewProc("CloseClipboard")
	procEmptyClipboard         = user32Desktop.NewProc("EmptyClipboard")
	procIsClipboardFormat      = user32Desktop.NewProc("IsClipboardFormatAvailable")
	procGetClipboardData       = user32Desktop.NewProc("GetClipboardData")
	procSetClipboardData       = user32Desktop.NewProc("SetClipboardData")
	procCreateDCW              = gdi32Desktop.NewProc("CreateDCW")
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
	procGlobalAlloc            = kernel32Desktop.NewProc("GlobalAlloc")
	procGlobalLock             = kernel32Desktop.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32Desktop.NewProc("GlobalUnlock")
	procGlobalSize             = kernel32Desktop.NewProc("GlobalSize")
	procGlobalFree             = kernel32Desktop.NewProc("GlobalFree")
	procMoveMemory             = kernel32Desktop.NewProc("RtlMoveMemory")
	procSendSAS                = sasDesktop.NewProc("SendSAS")
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
	pooled    bool
}

var desktopJPEGUploadPool sync.Pool

func cloneDesktopJPEGForUpload(source []byte) ([]byte, bool) {
	if len(source) == 0 {
		return nil, false
	}
	var destination []byte
	if pooled, ok := desktopJPEGUploadPool.Get().(*[]byte); ok && pooled != nil {
		destination = *pooled
	}
	if cap(destination) < len(source) {
		destination = make([]byte, len(source))
	} else {
		destination = destination[:len(source)]
	}
	copy(destination, source)
	return destination, true
}

func releaseDesktopFrameUpload(upload *desktopFrameUpload) {
	if upload == nil || !upload.pooled || upload.capture.JPEG == nil {
		return
	}
	buffer := upload.capture.JPEG[:0]
	// Remote frames are bounded well below this in normal profiles. Do not keep
	// an abnormal multi-megabyte allocation alive forever after one bad frame.
	if cap(buffer) <= 8<<20 {
		desktopJPEGUploadPool.Put(&buffer)
	}
	upload.capture.JPEG = nil
	upload.pooled = false
}

type desktopFrameUploadResult struct {
	sessionID   string
	lane        int
	capture     desktopCapture
	duration    time.Duration
	completedAt time.Time
	dropped     bool
	err         error
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
	ID               int64  `json:"-"`
	Type             string `json:"type"`
	Action           string `json:"action,omitempty"`
	Button           string `json:"button,omitempty"`
	Text             string `json:"text,omitempty"`
	X                int    `json:"x,omitempty"`
	Y                int    `json:"y,omitempty"`
	CoordinateWidth  int    `json:"coordinateWidth,omitempty"`
	CoordinateHeight int    `json:"coordinateHeight,omitempty"`
	Delta            int    `json:"delta,omitempty"`
	KeyCode          int    `json:"keyCode,omitempty"`
	ImagePNG         []byte `json:"-"`
	TransportErr     error  `json:"-"`
}

type desktopInputBatch struct {
	SessionID string
	Access    desktopAgentAccess
	Events    []desktopInput
}

type desktopInputStreamTarget struct {
	access    desktopAgentAccess
	sessionID string
}

type desktopInputTask struct {
	sessionID string
	events    []desktopInput
	capture   desktopCapture
}

type desktopInputTaskResult struct {
	err                 error
	acknowledgedResults []desktopAcknowledgedInputResult
}

type desktopAcknowledgedInputResult struct {
	inputID   int64
	inputType string
	value     string
	mime      string
	imagePNG  []byte
	err       error
}

func runDesktopInputWorker(ctx context.Context, tasks <-chan desktopInputTask, results chan<- desktopInputTaskResult, activeSession *atomic.Value) {
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
			currentSessionID := desktopAtomicString(activeSession)
			if task.sessionID == "" || task.sessionID != currentSessionID {
				continue
			}
			// A secure-desktop transition can make SyncIfStale take noticeably
			// longer than a normal SendInput call. Collapse everything that arrived
			// while this worker was busy before touching Windows again. This keeps
			// the newest free cursor position and preserves every click, key, wheel,
			// text and SAS barrier in order instead of replaying several old cursor
			// positions one task at a time after the desktop becomes available.
			for len(task.events) < 256 {
				select {
				case more := <-tasks:
					if more.sessionID != currentSessionID {
						continue
					}
					task.events = append(task.events, more.events...)
					task.capture = more.capture
				default:
					goto executeTask
				}
			}
		executeTask:
			task.events = coalesceDesktopInput(task.events)
			result := desktopInputTaskResult{}
			var surfaceErr error
			surfaceReady := false
			for _, event := range task.events {
				if task.sessionID != desktopAtomicString(activeSession) {
					break
				}
				// The Secure Attention Sequence is handled by the SCM service and
				// must not depend on attaching this worker thread to the interactive
				// desktop. In particular, OpenInputDesktop may be unavailable while
				// Windows is already showing Winlogon — exactly where SAS is needed.
				if event.Type == "sas" {
					sasErr := executeDesktopInput(event, task.capture)
					result.acknowledgedResults = append(result.acknowledgedResults, desktopAcknowledgedInputResult{inputID: event.ID, inputType: "sas", err: sasErr})
					if result.err == nil && sasErr != nil {
						result.err = sasErr
					}
					continue
				}
				if !surfaceReady && surfaceErr == nil {
					_, surfaceErr = surface.SyncIfStale(100 * time.Millisecond)
					surfaceReady = surfaceErr == nil
				}
				if surfaceErr != nil {
					if result.err == nil {
						result.err = surfaceErr
					}
					continue
				}
				if event.Type == "clipboard_read" {
					imagePNG, imageAvailable, imageErr := readWindowsClipboardPNG()
					if imageAvailable || imageErr != nil {
						if imageErr == nil && !windowsClipboardPNGChanged(imagePNG) {
							imagePNG = nil
						}
						result.acknowledgedResults = append(result.acknowledgedResults, desktopAcknowledgedInputResult{inputID: event.ID, inputType: event.Type, mime: "image/png", imagePNG: imagePNG, err: imageErr})
						if result.err == nil && imageErr != nil {
							result.err = imageErr
						}
						continue
					}
					value, clipboardErr := walk.Clipboard().Text()
					result.acknowledgedResults = append(result.acknowledgedResults, desktopAcknowledgedInputResult{inputID: event.ID, inputType: event.Type, value: value, err: clipboardErr})
					if result.err == nil && clipboardErr != nil {
						result.err = clipboardErr
					}
					continue
				}
				if event.Type == "clipboard_write" {
					clipboardErr := walk.Clipboard().SetText(event.Text)
					result.acknowledgedResults = append(result.acknowledgedResults, desktopAcknowledgedInputResult{inputID: event.ID, inputType: event.Type, err: clipboardErr})
					if result.err == nil && clipboardErr != nil {
						result.err = clipboardErr
					}
					continue
				}
				if event.Type == "clipboard_image_write" {
					clipboardErr := event.TransportErr
					if clipboardErr == nil {
						clipboardErr = writeWindowsClipboardPNG(event.ImagePNG)
					}
					result.acknowledgedResults = append(result.acknowledgedResults, desktopAcknowledgedInputResult{inputID: event.ID, inputType: event.Type, err: clipboardErr})
					if result.err == nil && clipboardErr != nil {
						result.err = clipboardErr
					}
					continue
				}
				if inputErr := executeDesktopInput(event, task.capture); inputErr != nil && result.err == nil {
					result.err = inputErr
				}
			}
			select {
			case results <- result:
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
	return surface.SyncBeforeSwitch(nil)
}

// SyncBeforeSwitch releases resources owned by the current desktop before
// SetThreadDesktop attaches this thread to the newly visible one. DXGI output
// duplication, screen DCs and compatible bitmaps pin their creating thread to
// the old desktop; attempting SetThreadDesktop while they are still alive can
// fail with ERROR_BUSY or leave capture returning the final old frame forever.
func (surface *desktopInputSurface) SyncBeforeSwitch(beforeSwitch func()) (bool, error) {
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
	if beforeSwitch != nil {
		beforeSwitch()
	}
	if switched, _, switchErr := procSetThreadDesktop.Call(handle); switched == 0 {
		procCloseDesktop.Call(handle)
		// Retry on the next capture iteration. Keeping lastChecked at the normal
		// cadence made a failed secure-desktop transition look frozen for another
		// half second even after all old capture resources had been released.
		surface.lastChecked = time.Time{}
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
	return surface.SyncIfStaleBeforeSwitch(interval, nil)
}

func (surface *desktopInputSurface) SyncIfStaleBeforeSwitch(interval time.Duration, beforeSwitch func()) (bool, error) {
	if surface.handle != 0 && time.Since(surface.lastChecked) < interval {
		return false, nil
	}
	return surface.SyncBeforeSwitch(beforeSwitch)
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
	// Six independent WebSocket lanes carry video. The generic client is
	// intentionally limited to two connections, which is appropriate for
	// control/input HTTP but can serialize or stall four video handshakes on
	// transports that keep upgraded connections in the per-host accounting.
	// Reserve one connection for every lane plus two short-lived HTTP fallback
	// requests so 30/60 FPS is not silently reduced to the capacity of two TCP
	// flows.
	frameClient := newDesktopHTTPClientWithLimit(8*time.Second, desktopAutoVideoLaneCount+2)
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
	lastFrameEnqueuedAt := time.Time{}
	nextFrameAt := time.Time{}
	lastCaptureInterval := time.Duration(0)
	currentOffer := desktopSessionOffer{}
	offerActive := false
	lastOfferAt := time.Time{}
	var offerFetchInFlight atomic.Bool
	offerResults := make(chan desktopOfferRefresh, 1)
	var inputFetchInFlight atomic.Bool
	latestCapture := atomic.Value{}
	latestCapture.Store(desktopCapture{})
	activeInputSession := atomic.Value{}
	activeInputSession.Store("")
	defer activeInputSession.Store("")
	inputTasks := make(chan desktopInputTask, 8)
	inputResults := make(chan desktopInputTaskResult, 1)
	go runDesktopInputWorker(ctx, inputTasks, inputResults, &activeInputSession)
	inputErrors := make(chan error, 1)
	streamInputBatches := make(chan desktopInputBatch, 16)
	inputStreamTargets := make(chan desktopInputStreamTarget, 1)
	var dedicatedInputStreamActive atomic.Bool
	var legacyInputStreamActive atomic.Bool
	var latestInputNanos atomic.Int64
	frameUploads := make(chan desktopFrameUpload, 1)
	frameUploadResults := make(chan desktopFrameUploadResult, 8)
	go runDesktopFrameUploader(ctx, frameClient, frameUploads, frameUploadResults, streamInputBatches, &legacyInputStreamActive)
	go runDesktopInputStream(ctx, inputClient, inputStreamTargets, streamInputBatches, &dedicatedInputStreamActive)
	go runDesktopStreamInputDispatcher(ctx, controlClient, streamInputBatches, inputTasks, &latestCapture, &latestInputNanos, &activeInputSession)
	autoCadence := newDesktopAutoCadence()
	access := desktopAgentAccess{}
	accessLoadedAt := time.Time{}
	var frameSequence uint64
	lastInputStreamTargetKey := ""
	publishInputStreamTarget := func(target desktopInputStreamTarget) {
		key := target.sessionID + "\x00" + target.access.ServerURL + "\x00" + target.access.DeviceID + "\x00" + target.access.DesktopSecret
		if target.sessionID == "" {
			key = ""
		}
		if key == lastInputStreamTargetKey {
			return
		}
		lastInputStreamTargetKey = key
		select {
		case inputStreamTargets <- target:
		default:
			select {
			case <-inputStreamTargets:
			default:
			}
			select {
			case inputStreamTargets <- target:
			default:
			}
		}
	}
	for ctx.Err() == nil {
		var err error
		if access.ServerURL == "" || time.Since(accessLoadedAt) >= 5*time.Second {
			access, err = loadDesktopAgentAccess()
			if err == nil {
				accessLoadedAt = time.Now()
			}
		}
		if err != nil {
			publishInputStreamTarget(desktopInputStreamTarget{})
			offerActive = false
			activeInputSession.Store("")
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
					publishInputStreamTarget(desktopInputStreamTarget{})
					offerActive = false
					activeInputSession.Store("")
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
				publishInputStreamTarget(desktopInputStreamTarget{})
				offerActive = false
				activeInputSession.Store("")
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
		publishInputStreamTarget(desktopInputStreamTarget{access: access, sessionID: offer.ID})
		activeInputSession.Store(offer.ID)
		setDesktopSessionState(true, offer.ControlEnabled, offer.ID)
		if offer.ID != lastSessionID {
			lastSessionID = offer.ID
			frameSequence = 0
			drainDesktopFrameUploads(frameUploads)
			lastCapture = desktopCapture{}
			lastFrameEnqueuedAt = time.Time{}
			latestCapture.Store(desktopCapture{})
			nextFrameAt = time.Time{}
			lastCaptureInterval = 0
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
			for _, acknowledged := range completed.acknowledgedResults {
				if acknowledged.inputID <= 0 {
					continue
				}
				if len(acknowledged.imagePNG) > 0 && acknowledged.err == nil {
					acknowledged.err = uploadDesktopClipboardImage(ctx, controlClient, access, offer.ID, acknowledged.imagePNG)
					if acknowledged.err == nil {
						rememberWindowsClipboardPNG(acknowledged.imagePNG)
					}
				}
				if err := reportDesktopInputResult(ctx, controlClient, access, offer.ID, acknowledged); err != nil {
					select {
					case inputErrors <- err:
					default:
					}
				}
			}
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
		if offer.ControlEnabled && !dedicatedInputStreamActive.Load() && !legacyInputStreamActive.Load() && len(lastCapture.JPEG) > 0 && inputFetchInFlight.CompareAndSwap(false, true) {
			accessCopy, sessionID := access, offer.ID
			go func() {
				defer inputFetchInFlight.Store(false)
				events, inputErr := fetchDesktopInputs(ctx, inputClient, accessCopy, sessionID)
				if currentDesktopSessionIdentifier() != sessionID {
					return
				}
				if inputErr == nil && len(events) > 0 {
					select {
					case streamInputBatches <- desktopInputBatch{SessionID: sessionID, Access: accessCopy, Events: events}:
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
				if uploaded.dropped {
					if offer.TargetFPS == 0 {
						processing := time.Duration(uploaded.capture.CaptureMillis+uploaded.capture.EncodeMillis) * time.Millisecond
						autoCadence.ObserveDropped(processing, uploaded.completedAt)
					}
				} else if uploaded.err != nil {
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
						autoCadence.Observe(uploaded.duration, processing, uploaded.lane, uploaded.completedAt)
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
			autoCadence.SetMaximumFPS(desktopAutoMaximumFPS(capturer.screenWidth))
			effectiveFPS = autoCadence.FPS
		}
		if effectiveFPS != 15 && effectiveFPS != 30 && effectiveFPS != 60 {
			effectiveFPS = 30
		}
		captureInterval := desktopCaptureInterval(effectiveFPS)
		if captureInterval != lastCaptureInterval {
			// A user FPS change (or an Auto promotion/demotion) starts a fresh
			// cadence immediately. Carrying a deadline from the previous mode can
			// otherwise leave one conspicuously long or short transition frame.
			nextFrameAt = time.Time{}
			lastCaptureInterval = captureInterval
		}
		captureErr := error(nil)
		if nextFrameAt.IsZero() {
			nextFrameAt = time.Now()
		}
		if len(lastCapture.JPEG) == 0 || !time.Now().Before(nextFrameAt) {
			frameStartedAt := time.Now()
			// Pace from this frame's actual start. If a VDI capture is late, do not
			// follow the long gap with a catch-up burst: that alternating rhythm is
			// perceived as a much stronger jerk than an honest dropped frame.
			nextFrameAt = desktopNextFrameDeadline(frameStartedAt, captureInterval)
			// The dedicated input worker checks winlogon/UAC transitions every 100 ms.
			// Capture only needs a slower safety check: OpenInputDesktop and querying
			// its object name can consume most of one 16.7 ms frame on virtual display
			// drivers. A 500 ms cadence removes that periodic 60 FPS hitch while input
			// remains responsive and the capture surface follows shortly afterwards.
			desktopChanged, desktopErr := inputSurface.SyncIfStaleBeforeSwitch(500*time.Millisecond, func() {
				// Close every DXGI/GDI object before SetThreadDesktop. Windows does
				// not permit a thread with desktop-owned objects to switch reliably.
				capturer.Close()
				lastCapture = desktopCapture{}
				lastFrameEnqueuedAt = time.Time{}
				latestCapture.Store(desktopCapture{})
				nextFrameAt = time.Time{}
			})
			if desktopErr != nil {
				captureErr = desktopErr
			} else if desktopChanged {
				// The transition callback already released the old surface. Capture
				// the newly visible Windows desktop immediately in this iteration.
				nextFrameAt = time.Now()
			}
			// Auto begins with the same 30 FPS profile as explicit 30. It changes
			// geometry only after the cadence controller has accumulated sustained
			// evidence (20 slow or 60 fast samples), so an occasional network spike
			// cannot trigger the old 15/30/60 resource-rebuild loop.
			captureProfileFPS := effectiveFPS
			capture := desktopCapture{}
			if captureErr == nil {
				interactive := desktopInteractionIsActive(offer.ControlEnabled, lastInputAt, time.Now())
				constrained := offer.TargetFPS == 0 && autoCadence.Constrained
				// Auto preserves its 30 FPS control cadence under sustained CPU/link
				// pressure by reusing the bounded high-quality motion surface. The old
				// 15 FPS fallback selected an even heavier 4K profile and could trap a
				// slow VDI host at only a few real frames per second.
				// An explicit 60 FPS selection is quality-first. Auto-promoted 60 FPS
				// remains bounded so it can demote cleanly when the host or link cannot
				// sustain the sharper profile.
				preserveDetail := offer.TargetFPS == 60
				capture, captureErr = capturer.CaptureJPEG(captureProfileFPS, interactive, constrained, preserveDetail, offer.CursorVisible, desktopRequiresSecureCapture(inputSurface.name))
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
					// The server intentionally expires video frames older than 15 seconds.
					// Credential and lock screens can remain pixel-identical much longer,
					// so periodically republish the immutable JPEG as a keyframe. This is
					// also an end-to-end liveness proof for the viewer without spending
					// 15/30/60 duplicate uploads per second.
					if desktopShouldPublishHeartbeat(lastFrameEnqueuedAt, time.Now()) {
						heartbeat := capture
						heartbeat.JPEG, _ = cloneDesktopJPEGForUpload(capture.JPEG)
						frameSequence++
						enqueueLatestDesktopFrame(frameUploads, desktopFrameUpload{access: access, sessionID: offer.ID, sequence: frameSequence, capture: heartbeat, pooled: true})
						lastFrameEnqueuedAt = time.Now()
					}
				} else {
					uploadCapture := capture
					// desktopCapturer reuses its TurboJPEG buffer. The uploader runs in
					// parallel, so give it an immutable copy and keep at most the newest
					// waiting frame. This overlaps capture/encoding with HTTPS without
					// ever building a latency-producing frame backlog.
					uploadCapture.JPEG, _ = cloneDesktopJPEGForUpload(capture.JPEG)
					frameSequence++
					enqueueLatestDesktopFrame(frameUploads, desktopFrameUpload{access: access, sessionID: offer.ID, sequence: frameSequence, capture: uploadCapture, pooled: true})
					lastFrameEnqueuedAt = time.Now()
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
	return newDesktopHTTPClientWithLimit(timeout, 2)
}

func newDesktopHTTPClientWithLimit(timeout time.Duration, maxConnectionsPerHost int) *http.Client {
	if maxConnectionsPerHost < 1 {
		maxConnectionsPerHost = 1
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = max(8, maxConnectionsPerHost)
	transport.MaxIdleConnsPerHost = maxConnectionsPerHost
	transport.MaxConnsPerHost = maxConnectionsPerHost
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

func reportDesktopInputResult(ctx context.Context, client *http.Client, access desktopAgentAccess, sessionID string, result desktopAcknowledgedInputResult) error {
	inputError := ""
	if result.err != nil {
		inputError = result.err.Error()
	}
	payload, err := json.Marshal(map[string]any{"inputId": result.inputID, "inputType": result.inputType, "inputError": inputError, "inputValue": result.value, "inputMime": result.mime})
	if err != nil {
		return err
	}
	response, err := desktopRequest(ctx, client, access, http.MethodPost, "/api/desktop/agent/sessions/"+sessionID+"/status", bytes.NewReader(payload), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("desktop input acknowledgement: HTTP %d", response.StatusCode)
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

func uploadDesktopClipboardImage(ctx context.Context, client *http.Client, access desktopAgentAccess, sessionID string, imagePNG []byte) error {
	if len(imagePNG) == 0 || len(imagePNG) > desktopMaximumClipboardImageBytes {
		return errors.New("изображение удалённого буфера пустое или слишком большое")
	}
	response, err := desktopRequest(ctx, client, access, http.MethodPost, "/api/desktop/agent/sessions/"+url.PathEscape(sessionID)+"/clipboard-image", bytes.NewReader(imagePNG), map[string]string{"Content-Type": "image/png"})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("desktop clipboard image upload: HTTP %d", response.StatusCode)
	}
	return nil
}

func downloadDesktopClipboardImage(ctx context.Context, client *http.Client, access desktopAgentAccess, sessionID, sequence string) ([]byte, error) {
	if _, err := strconv.ParseUint(sequence, 10, 64); err != nil {
		return nil, errors.New("некорректная версия изображения буфера")
	}
	path := "/api/desktop/agent/sessions/" + url.PathEscape(sessionID) + "/clipboard-image/" + url.PathEscape(sequence)
	response, err := desktopRequest(ctx, client, access, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("desktop clipboard image download: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, desktopMaximumClipboardImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > desktopMaximumClipboardImageBytes {
		return nil, errors.New("изображение буфера пустое или слишком большое")
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, errors.New("сервер вернул некорректное изображение буфера")
	}
	return data, nil
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
	connection     *websocket.Conn
	sessionID      string
	accessKey      string
	lane           int
	retryAfter     time.Time
	retryDelay     time.Duration
	retrySessionID string
	retryAccessKey string
	readCancel     context.CancelFunc
	inputs         chan<- desktopInputBatch
	active         *atomic.Bool
	readGeneration atomic.Uint64
}

const (
	desktopStreamRetryInitial = 250 * time.Millisecond
	desktopStreamRetryMaximum = 4 * time.Second
)

func nextDesktopStreamRetry(current time.Duration, hadConnection bool) time.Duration {
	if hadConnection || current <= 0 {
		return desktopStreamRetryInitial
	}
	next := current * 2
	if next > desktopStreamRetryMaximum {
		return desktopStreamRetryMaximum
	}
	return next
}

func desktopStreamRetryWait(retryAfter, now time.Time) time.Duration {
	if retryAfter.IsZero() || !now.Before(retryAfter) {
		return 0
	}
	return retryAfter.Sub(now)
}

func (stream *desktopFrameStreamClient) resetRetry() {
	stream.retryAfter = time.Time{}
	stream.retryDelay = 0
	stream.retrySessionID = ""
	stream.retryAccessKey = ""
}

func (stream *desktopFrameStreamClient) recordRetryFailure(sessionID, accessKey string, hadConnection bool) {
	if stream.retrySessionID != sessionID || stream.retryAccessKey != accessKey {
		stream.retryDelay = 0
	}
	stream.retrySessionID = sessionID
	stream.retryAccessKey = accessKey
	stream.retryDelay = nextDesktopStreamRetry(stream.retryDelay, hadConnection)
	stream.retryAfter = time.Now().Add(stream.retryDelay)
}

func (stream *desktopFrameStreamClient) Close() {
	stream.readGeneration.Add(1)
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

func desktopFrameStreamHeader(upload desktopFrameUpload) []byte {
	backend := []byte(upload.capture.CaptureBackend)
	if len(backend) > 48 {
		backend = backend[:48]
	}
	header := make([]byte, 26+len(backend))
	copy(header[:4], "RIT3")
	binary.BigEndian.PutUint32(header[4:8], uint32(upload.capture.FrameWidth))
	binary.BigEndian.PutUint32(header[8:12], uint32(upload.capture.FrameHeight))
	header[12] = byte(min(255, max(0, upload.capture.CaptureMillis)))
	header[13] = byte(min(255, max(0, upload.capture.CopyMillis)))
	header[14] = byte(min(255, max(0, upload.capture.ScaleMillis)))
	header[15] = byte(min(255, max(0, upload.capture.EncodeMillis)))
	header[16] = 0 // reserved for future transport diagnostics
	header[17] = byte(len(backend))
	binary.BigEndian.PutUint64(header[18:26], upload.sequence)
	copy(header[26:], backend)
	return header
}

func (stream *desktopFrameStreamClient) send(ctx context.Context, client *http.Client, upload desktopFrameUpload) error {
	key := upload.access.ServerURL + "\x00" + upload.access.DeviceID + "\x00" + upload.access.DesktopSecret
	if stream.retrySessionID != "" && (stream.retrySessionID != upload.sessionID || stream.retryAccessKey != key) {
		// A failed old session or pre-VPN endpoint must not hold a newly selected
		// session behind its backoff window. New connection identities always get
		// an immediate first attempt.
		stream.resetRetry()
	}
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
		endpoint, err := desktopWebSocketURL(upload.access.ServerURL, "/api/desktop/agent/sessions/"+upload.sessionID+"/stream?lane="+strconv.Itoa(stream.lane)+"&videoOnly=1")
		if err != nil {
			stream.recordRetryFailure(upload.sessionID, key, false)
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
			stream.recordRetryFailure(upload.sessionID, key, false)
			return err
		}
		stream.connection = connection
		stream.sessionID = upload.sessionID
		stream.accessKey = key
		stream.resetRetry()
		if stream.inputs != nil {
			readCtx, cancel := context.WithCancel(ctx)
			stream.readCancel = cancel
			generation := stream.readGeneration.Add(1)
			go stream.readInputs(readCtx, connection, upload.sessionID, upload.access, generation)
		}
	}
	header := desktopFrameStreamHeader(upload)
	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	writer, err := stream.connection.Writer(writeCtx, websocket.MessageBinary)
	if err == nil {
		_, err = writer.Write(header)
	}
	if err == nil {
		_, err = writer.Write(upload.capture.JPEG)
	}
	if writer != nil {
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
	}
	cancel()
	if err != nil {
		stream.Close()
		// A previously established stream should reconnect quickly; repeated dial
		// failures then back off exponentially to protect the server during outages.
		stream.recordRetryFailure(upload.sessionID, key, true)
	}
	return err
}

func (stream *desktopFrameStreamClient) readInputs(ctx context.Context, connection *websocket.Conn, sessionID string, access desktopAgentAccess, generation uint64) {
	defer func() {
		if stream.active != nil && stream.readGeneration.Load() == generation {
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
		if stream.active != nil && stream.readGeneration.Load() == generation {
			// Old servers ignore videoOnly=1 and still deliver control through lane
			// zero. Mark the compatibility channel usable only after a real batch;
			// otherwise it could suppress HTTP fallback while a new server correctly
			// keeps this video connection input-free.
			stream.active.Store(true)
		}
		select {
		case stream.inputs <- desktopInputBatch{SessionID: sessionID, Access: access, Events: events}:
		case <-ctx.Done():
			return
		}
	}
}

func dialDesktopInputStream(ctx context.Context, client *http.Client, target desktopInputStreamTarget) (*websocket.Conn, error) {
	endpoint, err := desktopWebSocketURL(target.access.ServerURL, "/api/desktop/agent/sessions/"+target.sessionID+"/stream?lane=input")
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	headers.Set("X-Genesis-Device-Id", target.access.DeviceID)
	headers.Set("Authorization", "Desktop "+target.access.DesktopSecret)
	headers.Set("User-Agent", "RemoteIt-Desktop/"+version)
	connection, response, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPClient: client, HTTPHeader: headers, CompressionMode: websocket.CompressionDisabled})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	return connection, err
}

func receiveDesktopInputStream(ctx context.Context, connection *websocket.Conn, target desktopInputStreamTarget, inputs chan<- desktopInputBatch) error {
	for {
		messageType, payload, err := connection.Read(ctx)
		if err != nil {
			return err
		}
		if messageType != websocket.MessageText || len(payload) == 0 || len(payload) > 64<<10 {
			continue
		}
		events, decodeErr := decodeDesktopInputStreamMessage(payload)
		if decodeErr != nil {
			continue
		}
		select {
		case inputs <- desktopInputBatch{SessionID: target.sessionID, Access: target.access, Events: events}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

const (
	desktopInputStreamRetryInitial = 150 * time.Millisecond
	desktopInputStreamRetryMaximum = 2 * time.Second
)

func nextDesktopInputStreamRetry(current time.Duration, connected bool) time.Duration {
	if connected || current <= 0 {
		return desktopInputStreamRetryInitial
	}
	next := current * 2
	if next > desktopInputStreamRetryMaximum {
		return desktopInputStreamRetryMaximum
	}
	return next
}

func runDesktopInputStream(ctx context.Context, client *http.Client, targets <-chan desktopInputStreamTarget, inputs chan<- desktopInputBatch, active *atomic.Bool) {
	var cancelCurrent context.CancelFunc
	var generation atomic.Uint64
	for {
		select {
		case <-ctx.Done():
			generation.Add(1)
			if cancelCurrent != nil {
				cancelCurrent()
			}
			active.Store(false)
			return
		case target, ok := <-targets:
			if !ok {
				generation.Add(1)
				if cancelCurrent != nil {
					cancelCurrent()
				}
				active.Store(false)
				return
			}
			currentGeneration := generation.Add(1)
			if cancelCurrent != nil {
				cancelCurrent()
			}
			active.Store(false)
			if target.sessionID == "" {
				cancelCurrent = nil
				continue
			}
			streamCtx, cancel := context.WithCancel(ctx)
			cancelCurrent = cancel
			go func(target desktopInputStreamTarget, currentGeneration uint64) {
				retry := desktopInputStreamRetryInitial
				for streamCtx.Err() == nil {
					connection, err := dialDesktopInputStream(streamCtx, client, target)
					if err == nil {
						// A connection that completed its handshake proves the route is back.
						// If it later drops (VPN switch, Wi-Fi roam, proxy restart), retry from
						// 150 ms instead of retaining the outage's multi-second backoff.
						retry = nextDesktopInputStreamRetry(retry, true)
						if generation.Load() == currentGeneration {
							active.Store(true)
						}
						err = receiveDesktopInputStream(streamCtx, connection, target, inputs)
						_ = connection.Close(websocket.StatusNormalClosure, "")
					}
					if generation.Load() == currentGeneration {
						active.Store(false)
					}
					if streamCtx.Err() != nil {
						return
					}
					timer := time.NewTimer(retry)
					select {
					case <-streamCtx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
					retry = nextDesktopInputStreamRetry(retry, false)
				}
			}(target, currentGeneration)
		}
	}
}

func decodeDesktopInputStreamMessage(payload []byte) ([]desktopInput, error) {
	var message struct {
		Events []struct {
			ID    int64        `json:"id"`
			Event desktopInput `json:"event"`
		} `json:"events"`
	}
	if len(payload) == 0 || len(payload) > 64<<10 || json.Unmarshal(payload, &message) != nil || len(message.Events) == 0 || len(message.Events) > 64 {
		return nil, errors.New("invalid desktop input stream message")
	}
	events := make([]desktopInput, 0, len(message.Events))
	for _, queued := range message.Events {
		queued.Event.ID = queued.ID
		events = append(events, queued.Event)
	}
	return events, nil
}

// coalesceDesktopInput replaces only consecutive pointer positions. Pointer
// button and wheel actions are barriers: preserving the newest move on each
// side of down/up is required for drag-and-drop and keeps the wheel target
// deterministic. This still removes high-refresh cursor noise without moving a
// drag sample past the matching button release.
func coalesceDesktopInput(events []desktopInput) []desktopInput {
	result := make([]desktopInput, 0, len(events))
	for _, event := range events {
		if event.Type == "pointer" && event.Action == "move" && len(result) > 0 {
			last := len(result) - 1
			if result[last].Type == "pointer" && result[last].Action == "move" {
				result[last] = event
				continue
			}
		}
		result = append(result, event)
	}
	return result
}

func desktopAtomicString(value *atomic.Value) string {
	if value == nil {
		return ""
	}
	loaded := value.Load()
	result, _ := loaded.(string)
	return result
}

// runDesktopStreamInputDispatcher keeps network input entirely off the capture
// cadence thread. The previous main-loop drain made a continuous wheel/trackpad
// gesture contend with DXGI and reduced a fast 30/60 FPS producer to roughly
// 15-20 FPS even though scale+encode itself took only 5-10ms. This dispatcher
// coalesces only obsolete free pointer positions; clicks, keys and wheel events
// retain their order and are handed to the dedicated locked Windows input
// thread immediately.
func runDesktopStreamInputDispatcher(ctx context.Context, client *http.Client, batches <-chan desktopInputBatch, tasks chan desktopInputTask, latestCapture *atomic.Value, latestInputNanos *atomic.Int64, activeSession *atomic.Value) {
	var carry *desktopInputBatch
	lastSessionID := ""
	var lastDispatchedID int64
	for {
		var batch desktopInputBatch
		if carry != nil {
			batch = *carry
			carry = nil
		} else {
			select {
			case <-ctx.Done():
				return
			case batch = <-batches:
			}
		}
		activeSessionID := desktopAtomicString(activeSession)
		if batch.SessionID == "" || batch.SessionID != activeSessionID || len(batch.Events) == 0 {
			continue
		}
		if batch.SessionID != lastSessionID {
			lastSessionID = batch.SessionID
			lastDispatchedID = 0
		}
		pending := append([]desktopInput(nil), batch.Events...)
		for len(pending) < 64 {
			select {
			case more := <-batches:
				currentSessionID := desktopAtomicString(activeSession)
				if more.SessionID == "" || more.SessionID != currentSessionID || len(more.Events) == 0 {
					continue
				}
				if more.SessionID != batch.SessionID {
					copy := more
					carry = &copy
					goto dispatch
				}
				pending = append(pending, more.Events...)
			default:
				goto dispatch
			}
		}
	dispatch:
		if batch.SessionID != desktopAtomicString(activeSession) {
			continue
		}
		pending = coalesceDesktopInput(pending)
		for index := range pending {
			if pending[index].Type != "clipboard_image_write" {
				continue
			}
			pending[index].ImagePNG, pending[index].TransportErr = downloadDesktopClipboardImage(ctx, client, batch.Access, batch.SessionID, pending[index].Text)
		}
		// The server restores an input batch if an Agent WebSocket write fails.
		// A complete message can nevertheless have reached this process before the
		// close became visible to the sender, so IDs make reconnect delivery
		// idempotent without adding an acknowledgement round trip to every click.
		filtered := pending[:0]
		for _, event := range pending {
			if event.ID > 0 && event.ID <= lastDispatchedID {
				continue
			}
			filtered = append(filtered, event)
		}
		pending = filtered
		if len(pending) == 0 {
			continue
		}
		for _, event := range pending {
			if event.ID > lastDispatchedID {
				lastDispatchedID = event.ID
			}
		}
		latestInputNanos.Store(time.Now().UnixNano())
		capture, _ := latestCapture.Load().(desktopCapture)
		task := desktopInputTask{sessionID: batch.SessionID, events: pending, capture: capture}
		select {
		case tasks <- task:
		case <-ctx.Done():
			return
		}
	}
}

func runDesktopFrameUploadLane(ctx context.Context, client *http.Client, lane int, uploads <-chan desktopFrameUpload, results chan<- desktopFrameUploadResult, inputs chan<- desktopInputBatch, active *atomic.Bool, httpFallback bool) {
	stream := &desktopFrameStreamClient{lane: lane, inputs: inputs, active: active}
	defer stream.Close()
	for {
		// An auxiliary lane in exponential reconnect backoff must stop advertising
		// itself as an immediately available writer. Previously its unbuffered
		// channel still had a receiver, so the dispatcher handed it every sixth
		// frame; send() rejected that frame before dial and the lane silently
		// discarded a stable share of otherwise healthy 30/60 FPS output. The
		// primary lane remains available because it can use the HTTP compatibility
		// path while its websocket reconnects.
		if !httpFallback {
			if wait := desktopStreamRetryWait(stream.retryAfter, time.Now()); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return
				case <-timer.C:
				}
				continue
			}
		}
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
				// Still report a dropped sample to Auto: otherwise a failed auxiliary
				// lane looks healthy for the freshness window and repeated first-attempt
				// failures can lower delivered FPS without any adaptation signal.
				resultCapture := upload.capture
				resultCapture.JPEG = nil
				result := desktopFrameUploadResult{
					sessionID:   upload.sessionID,
					lane:        lane,
					capture:     resultCapture,
					completedAt: time.Now(),
					dropped:     true,
				}
				releaseDesktopFrameUpload(&upload)
				select {
				case results <- result:
				default:
				}
				continue
			}
			resultCapture := upload.capture
			resultCapture.JPEG = nil
			result := desktopFrameUploadResult{sessionID: upload.sessionID, lane: lane, capture: resultCapture, duration: time.Since(startedAt), completedAt: time.Now(), err: err}
			releaseDesktopFrameUpload(&upload)
			select {
			case results <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func runDesktopFrameUploader(ctx context.Context, client *http.Client, uploads <-chan desktopFrameUpload, results chan<- desktopFrameUploadResult, inputs chan<- desktopInputBatch, legacyActive *atomic.Bool) {
	// A single websocket becomes the limiting factor for high-quality JPEG at
	// 30/60 FPS even when capture and encoding take only a few milliseconds.
	// Six independent lanes preserve frame quality and expand the available
	// transport window without coupling keyboard/mouse input to a slow image
	// write. The lanes are intentionally unbuffered: a frame is handed only to
	// a writer that can start it immediately. Keeping one waiting JPEG per lane
	// used to retain up to six already-obsolete frames during congestion. The
	// producer sequence numbers still let the server discard a late in-flight
	// frame when lanes complete out of order.
	lanes := [6]chan desktopFrameUpload{
		make(chan desktopFrameUpload), make(chan desktopFrameUpload),
		make(chan desktopFrameUpload), make(chan desktopFrameUpload),
		make(chan desktopFrameUpload), make(chan desktopFrameUpload),
	}
	go runDesktopFrameUploadLane(ctx, client, 0, lanes[0], results, inputs, legacyActive, true)
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
			captureMetadata := upload.capture
			captureMetadata.JPEG = nil
			var sent bool
			lane, sent = dispatchDesktopFrameToAvailableLane(lanes[:], lane, upload)
			if !sent {
				// The frame was released immediately, but Auto still needs a bounded
				// congestion signal. Never block the uploader to report telemetry: a
				// later drop or completed upload will provide another sample.
				select {
				case results <- desktopFrameUploadResult{
					sessionID:   upload.sessionID,
					lane:        -1,
					capture:     captureMetadata,
					completedAt: time.Now(),
					dropped:     true,
				}:
				default:
				}
			}
		}
	}
}

func dispatchDesktopFrameToAvailableLane(lanes []chan desktopFrameUpload, start int, upload desktopFrameUpload) (int, bool) {
	if len(lanes) == 0 {
		releaseDesktopFrameUpload(&upload)
		return 0, false
	}
	if start < 0 || start >= len(lanes) {
		start = 0
	}
	for offset := 0; offset < len(lanes); offset++ {
		lane := (start + offset) % len(lanes)
		select {
		case lanes[lane] <- upload:
			return (lane + 1) % len(lanes), true
		default:
		}
	}
	// All transport writes are busy. Starting this frame later would only add
	// latency because a newer capture will arrive first; release it immediately.
	releaseDesktopFrameUpload(&upload)
	return start, false
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
	case stale := <-uploads:
		releaseDesktopFrameUpload(&stale)
	default:
	}
	select {
	case uploads <- upload:
	default:
		releaseDesktopFrameUpload(&upload)
	}
}

func drainDesktopFrameUploads(uploads chan desktopFrameUpload) {
	for {
		select {
		case stale := <-uploads:
			releaseDesktopFrameUpload(&stale)
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
			ID    int64        `json:"id"`
			Event desktopInput `json:"event"`
		} `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&payload); err != nil {
		return nil, err
	}
	events := make([]desktopInput, 0, len(payload.Events))
	for _, item := range payload.Events {
		item.Event.ID = item.ID
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
	cursorRestore           []byte
	dxgiScaleX              []int32
	dxgiScaleWeight         []int32
	motionScaleX            []int32
	motionScaleWeight       []int32
	nativeMotionScaleX      []int32
	nativeMotionScaleWeight []int32
	cursorBaseSurface       desktopCursorSurface
	cursorMotionSurface     desktopCursorSurface
	frame                   *image.RGBA
	info                    bitmapInfo
	encoder                 desktopJPEGEncoder
	fast                    desktopFastCapturer
	lastJPEG                []byte
	lastFrameWidth          int
	lastFrameHeight         int
	lastQuality             int
	lastChroma              desktopJPEGChroma
	lastEncodedQuality      int
	lastEncodedChroma       desktopJPEGChroma
	lastCursor              desktopCursorState
	lastCursorVisible       bool
	oversizeQuality         int
	oversizeChroma          desktopJPEGChroma
	oversizeUntil           time.Time
}

// desktopCursorSurface is an independent GDI surface used only for composing
// the remote pointer. The capture DIB has the selected base-profile stride
// (for example 3840 pixels), while an interactive/constrained frame can be
// 2560 pixels wide. Drawing a 2560-wide frame through that 3840-wide DIB mixes
// scan-line strides and produces the blinking/teleporting cursor seen on 4K
// and other non-standard displays. Keeping one surface for the base geometry
// and one for the motion geometry also avoids recreating GDI objects every
// time the 220 ms interaction window changes profile.
type desktopCursorSurface struct {
	memoryDC uintptr
	bitmap   uintptr
	previous uintptr
	pixels   []byte
	width    int
	height   int
}

func (surface *desktopCursorSurface) Close() {
	if surface.memoryDC != 0 && surface.previous != 0 {
		procSelectObject.Call(surface.memoryDC, surface.previous)
	}
	if surface.bitmap != 0 {
		procDeleteObject.Call(surface.bitmap)
	}
	if surface.memoryDC != 0 {
		procDeleteDC.Call(surface.memoryDC)
	}
	*surface = desktopCursorSurface{}
}

func (surface *desktopCursorSurface) ensure(screenDC uintptr, width, height int) bool {
	if screenDC == 0 || width <= 0 || height <= 0 {
		return false
	}
	if surface.memoryDC != 0 && surface.width == width && surface.height == height && len(surface.pixels) == width*height*4 {
		return true
	}
	surface.Close()
	surface.memoryDC, _, _ = procCreateCompatibleDC.Call(screenDC)
	if surface.memoryDC == 0 {
		return false
	}
	info := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}}
	var pixelMemory unsafe.Pointer
	surface.bitmap, _, _ = procCreateDIBSection.Call(
		screenDC,
		uintptr(unsafe.Pointer(&info)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&pixelMemory)),
		0,
		0,
	)
	if surface.bitmap == 0 || pixelMemory == nil {
		surface.Close()
		return false
	}
	surface.previous, _, _ = procSelectObject.Call(surface.memoryDC, surface.bitmap)
	surface.pixels = unsafe.Slice((*byte)(pixelMemory), width*height*4)
	surface.width, surface.height = width, height
	return true
}

func (capturer *desktopCapturer) Close() {
	capturer.encoder.Close()
	capturer.fast.Close()
	capturer.cursorBaseSurface.Close()
	capturer.cursorMotionSurface.Close()
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

func (capturer *desktopCapturer) ensure(targetFPS int, preserveDetail bool) error {
	screenX, _, _ := procGetSystemMetrics.Call(76)
	screenY, _, _ := procGetSystemMetrics.Call(77)
	screenWidth, _, _ := procGetSystemMetrics.Call(78)
	screenHeight, _, _ := procGetSystemMetrics.Call(79)
	x, y := int(int32(screenX)), int(int32(screenY))
	fullWidth, fullHeight := int(int32(screenWidth)), int(int32(screenHeight))
	if fullWidth <= 0 || fullHeight <= 0 || fullWidth > 12000 || fullHeight > 12000 {
		return errors.New("некорректный размер рабочего стола")
	}
	profile := desktopProfileForCapture(targetFPS, preserveDetail)
	width := min(fullWidth, profile.maxWidth)
	height := max(1, fullHeight*width/fullWidth)
	interactionWidth := desktopInteractionWidthMode(targetFPS, width, preserveDetail)
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

func (capturer *desktopCapturer) copyGDISurface() (string, bool) {
	if capturer.screenDC == 0 || capturer.memoryDC == 0 || capturer.width <= 0 || capturer.height <= 0 {
		return "-unavailable", false
	}
	copyFrom := func(sourceDC, rasterOperation uintptr) bool {
		if sourceDC == 0 {
			return false
		}
		if capturer.screenWidth == capturer.width && capturer.screenHeight == capturer.height {
			copied, _, _ := procBitBlt.Call(capturer.memoryDC, 0, 0, uintptr(capturer.width), uintptr(capturer.height), sourceDC, uintptr(capturer.screenX), uintptr(capturer.screenY), rasterOperation)
			return copied != 0
		}
		// COLORONCOLOR is intentionally used only on the secure/compatibility GDI
		// path. HALFTONE makes VMware recompose the native surface and can add
		// hundreds of milliseconds before JPEG encoding starts.
		procSetStretchBltMode.Call(capturer.memoryDC, 3) // COLORONCOLOR
		copied, _, _ := procStretchBlt.Call(capturer.memoryDC, 0, 0, uintptr(capturer.width), uintptr(capturer.height), sourceDC, uintptr(capturer.screenX), uintptr(capturer.screenY), uintptr(capturer.screenWidth), uintptr(capturer.screenHeight), rasterOperation)
		return copied != 0
	}
	const (
		sourceCopy        = uintptr(0x00CC0020) // SRCCOPY
		sourceCopyLayered = uintptr(0x40CC0020) // SRCCOPY | CAPTUREBLT
	)
	if copyFrom(capturer.screenDC, sourceCopy) {
		if capturer.screenWidth == capturer.width && capturer.screenHeight == capturer.height {
			return "-bitblt", true
		}
		return "-stretch-color", true
	}
	// RDP/VDI display drivers can keep an HDC handle alive while invalidating the
	// surface behind it during reconnect, user-session switching or CredUI. The
	// composited CAPTUREBLT retry recovers layered/protected windows without
	// affecting the normal fast path.
	if copyFrom(capturer.screenDC, sourceCopyLayered) {
		return "-layered", true
	}
	// Re-open DISPLAY in the currently attached input desktop. This is deliberately
	// a last-resort, per-failure DC: keeping it would recreate the stale-VDI-handle
	// problem after the next RDP reconnect.
	displayName, displayNameErr := windows.UTF16PtrFromString("DISPLAY")
	if displayNameErr != nil {
		return "-display-name", false
	}
	displayDC, _, _ := procCreateDCW.Call(uintptr(unsafe.Pointer(displayName)), 0, 0, 0)
	if displayDC == 0 {
		return "-display-unavailable", false
	}
	defer procDeleteDC.Call(displayDC)
	if copyFrom(displayDC, sourceCopy) {
		return "-display", true
	}
	if copyFrom(displayDC, sourceCopyLayered) {
		return "-display-layered", true
	}
	return "-failed", false
}

func (capturer *desktopCapturer) CaptureJPEG(targetFPS int, interactive, constrained, preserveDetail bool, cursorVisible bool, secureDesktop bool) (desktopCapture, error) {
	if err := capturer.ensure(targetFPS, preserveDetail); err != nil {
		return desktopCapture{}, err
	}
	// During active mouse/keyboard input prefer immediate motion over spending
	// bandwidth on visually lossless chroma in every intermediate frame. The
	// geometry remains unchanged, so pointer mapping cannot jump. A sharp 4:4:4
	// frame is emitted automatically after the short interaction window.
	captureStartedAt := time.Now()
	copyStartedAt := captureStartedAt
	copyMillis := 0
	scaleMillis := 0
	captureBackend := "dxgi"
	if secureDesktop {
		// Desktop Duplication is designed for the composited user desktop. On
		// winlogon/CredUI/UAC it can successfully open yet keep returning the last
		// user-desktop texture. GDI is slower but is the supported fresh source for
		// the protected input desktop and avoids a frozen credential prompt.
		captureBackend = "gdi-secure"
	}
	if interactive {
		captureBackend += "-motion"
	}
	framePixels := capturer.dxgiPixels
	frameWidth, frameHeight := capturer.screenWidth, capturer.screenHeight
	desiredWidth, desiredHeight := desktopOutputGeometryMode(capturer.width, capturer.height, targetFPS, interactive, constrained, preserveDetail)
	profile := desktopProfileForInteractionMode(targetFPS, interactive, constrained, desiredWidth, preserveDetail)
	fastResult := -1
	cursor := currentDesktopCursorState()
	// Desktop Duplication operates at the native output resolution. The former
	// <=1920 guard accidentally forced every high-DPI/4K display through GDI,
	// even when the low-latency DXGI path was available.
	if !secureDesktop {
		fastResult = capturer.fast.CaptureBGRA(framePixels, frameWidth, frameHeight)
	}
	copyMillis = int(time.Since(copyStartedAt).Milliseconds())
	if fastResult == 0 && len(capturer.lastJPEG) > 0 &&
		(!cursorVisible || cursor == capturer.lastCursor) &&
		cursorVisible == capturer.lastCursorVisible &&
		capturer.lastFrameWidth == desiredWidth && capturer.lastFrameHeight == desiredHeight &&
		capturer.lastQuality == profile.quality && capturer.lastChroma == profile.chroma &&
		desktopCanReuseBoundedJPEG(
			profile,
			capturer.lastEncodedQuality,
			capturer.lastEncodedChroma,
			capturer.oversizeUntil,
			time.Now(),
		) {
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
	} else if secureDesktop || fastResult < 0 || (fastResult == 0 && len(capturer.lastJPEG) == 0) {
		if !secureDesktop {
			captureBackend = capturer.fast.BackendDetail()
		}
		framePixels = capturer.pixels
		frameWidth, frameHeight = capturer.width, capturer.height
		copyBackend, copied := capturer.copyGDISurface()
		captureBackend += copyBackend
		if !copied {
			// VDI display drivers invalidate screen DCs when an RDP user reconnects,
			// the active session changes, or CredUI switches desktops. A persistent
			// GDI surface which was valid one frame ago then fails forever unless all
			// dependent objects are released. Recreate the exact target surface and
			// retry once in the same capture request, instead of showing a permanent
			// "не удалось скопировать экран" state for an otherwise healthy Agent.
			capturer.Close()
			if err := capturer.ensure(targetFPS, preserveDetail); err != nil {
				return desktopCapture{}, fmt.Errorf("не удалось переподключить захват VDI: %w", err)
			}
			desiredWidth, desiredHeight = desktopOutputGeometryMode(capturer.width, capturer.height, targetFPS, interactive, constrained, preserveDetail)
			profile = desktopProfileForInteractionMode(targetFPS, interactive, constrained, desiredWidth, preserveDetail)
			framePixels = capturer.pixels
			frameWidth, frameHeight = capturer.width, capturer.height
			copyBackend, copied = capturer.copyGDISurface()
			captureBackend += "-reinit" + copyBackend
			if !copied {
				capturer.Close()
				return desktopCapture{}, errors.New("не удалось скопировать экран после переподключения к VDI-сеансу")
			}
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
		if desktopUseRealtimeScalerMode(targetFPS, interactive, preserveDetail) {
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
	cursorPatch := desktopCursorPatch{}
	if cursorVisible {
		// Draw directly into the selected capture surface and restore only the
		// affected cursor rectangle after synchronous JPEG encoding. The previous
		// safety copy duplicated the entire 13.5 MiB notebook frame (or 31.6 MiB at
		// 4K) on every pointer update, which reduced FPS precisely in mobile
		// trackpad mode. The compact patch remains private to this capture call.
		cursorPatch = capturer.drawCursor(framePixels, frameWidth, frameHeight, cursor)
		if cursorPatch.width > 0 {
			defer restoreDesktopCursorPatch(framePixels, frameWidth, cursorPatch, capturer.cursorRestore)
		}
	}
	// TurboJPEG consumes packed BGRA directly at every supported resolution,
	// including native 2560 px and 4K profiles. The previous >1920 branch first
	// converted the entire surface to RGBA, encoded it in pure Go and silently
	// reduced it back to 1920 px. That caused both visible pixelation and the
	// 4-7 FPS ceiling measured on a 2560 px desktop.
	captureMillis := int(time.Since(captureStartedAt).Milliseconds())
	encodeStartedAt := time.Now()
	encodeNow := time.Now()
	preferFallback := profile.quality == capturer.oversizeQuality && profile.chroma == capturer.oversizeChroma && encodeNow.Before(capturer.oversizeUntil)
	var encoded []byte
	encodedQuality, encodedChroma := profile.quality, profile.chroma
	for attempt := 0; ; attempt++ {
		quality, chroma, ok := desktopBoundedEncodingProfile(profile, attempt, preferFallback)
		if !ok {
			return desktopCapture{}, errors.New("desktop frame exceeds the bounded transport limit")
		}
		candidate, encodeErr := capturer.encoder.EncodeBGRA(framePixels, frameWidth, frameHeight, quality, chroma)
		if encodeErr != nil {
			return desktopCapture{}, encodeErr
		}
		if len(candidate) <= desktopMaximumFrameBytes {
			encoded, encodedQuality, encodedChroma = candidate, quality, chroma
			break
		}
		if quality == profile.quality && chroma == profile.chroma {
			capturer.oversizeQuality = profile.quality
			capturer.oversizeChroma = profile.chroma
			capturer.oversizeUntil = encodeNow.Add(desktopOversizeProfileBackoff)
		}
	}
	if encodedQuality != profile.quality || encodedChroma != profile.chroma {
		captureBackend += "-bounded-jpeg"
	} else if !preferFallback {
		capturer.oversizeUntil = time.Time{}
	}
	capturer.lastJPEG = encoded
	capturer.lastFrameWidth, capturer.lastFrameHeight = frameWidth, frameHeight
	// Remember both the requested and the actually encoded profile. A frame that
	// temporarily exceeded the wire limit is safe to reuse only for the short
	// oversize backoff. Once it expires, an unchanged desktop must retry the
	// requested quality instead of remaining on the fallback JPEG forever.
	capturer.lastQuality, capturer.lastChroma = profile.quality, profile.chroma
	capturer.lastEncodedQuality, capturer.lastEncodedChroma = encodedQuality, encodedChroma
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

type desktopCursorPatch struct {
	left   int
	top    int
	width  int
	height int
}

func restoreDesktopCursorPatch(frame []byte, frameWidth int, patch desktopCursorPatch, saved []byte) {
	if patch.width <= 0 || patch.height <= 0 || frameWidth <= 0 || len(saved) < patch.width*patch.height*4 {
		return
	}
	stride := frameWidth * 4
	rowBytes := patch.width * 4
	for row := 0; row < patch.height; row++ {
		targetStart := (patch.top+row)*stride + patch.left*4
		savedStart := row * rowBytes
		if targetStart < 0 || targetStart+rowBytes > len(frame) {
			return
		}
		copy(frame[targetStart:targetStart+rowBytes], saved[savedStart:savedStart+rowBytes])
	}
}

func (capturer *desktopCapturer) drawCursor(frame []byte, frameWidth, frameHeight int, cursor desktopCursorState) desktopCursorPatch {
	if !cursor.Visible || cursor.Handle == 0 || capturer.screenDC == 0 || len(frame) < frameWidth*frameHeight*4 {
		return desktopCursorPatch{}
	}
	surface := &capturer.cursorMotionSurface
	if frameWidth == capturer.width && frameHeight == capturer.height {
		surface = &capturer.cursorBaseSurface
	}
	if !surface.ensure(capturer.screenDC, frameWidth, frameHeight) {
		return desktopCursorPatch{}
	}
	icon := desktopIconInfo{}
	if result, _, _ := procGetIconInfo.Call(cursor.Handle, uintptr(unsafe.Pointer(&icon))); result == 0 {
		return desktopCursorPatch{}
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
		return desktopCursorPatch{}
	}
	stride := frameWidth * 4
	patch := desktopCursorPatch{left: clipLeft, top: clipTop, width: clipRight - clipLeft, height: clipBottom - clipTop}
	patchBytes := patch.width * patch.height * 4
	if cap(capturer.cursorRestore) < patchBytes {
		capturer.cursorRestore = make([]byte, patchBytes)
	} else {
		capturer.cursorRestore = capturer.cursorRestore[:patchBytes]
	}
	rowBytes := patch.width * 4
	// Only seed the cursor rectangle. Copying an entire 4K frame into the GDI
	// scratch DIB for a 32x32 pointer adds tens of megabytes of memory traffic to
	// every mouse frame and directly reduces the delivered FPS.
	for row, y := 0, clipTop; y < clipBottom; row, y = row+1, y+1 {
		start, end := y*stride+clipLeft*4, y*stride+clipRight*4
		copy(capturer.cursorRestore[row*rowBytes:(row+1)*rowBytes], frame[start:end])
		copy(surface.pixels[start:end], frame[start:end])
	}
	const drawNormal = 0x0003 // DI_MASK | DI_IMAGE
	if result, _, _ := procDrawIconEx.Call(surface.memoryDC, uintptr(left), uintptr(top), cursor.Handle, uintptr(cursorWidth), uintptr(cursorHeight), 0, 0, drawNormal); result == 0 {
		capturer.cursorRestore = capturer.cursorRestore[:0]
		return desktopCursorPatch{}
	}
	for y := clipTop; y < clipBottom; y++ {
		start, end := y*stride+clipLeft*4, y*stride+clipRight*4
		copy(frame[start:end], surface.pixels[start:end])
	}
	return patch
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
		x, y := desktopPointerScreenPoint(event, capture)
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
	case "sas":
		return sendDesktopSecureAttentionSequence()
	}
	return nil
}

func desktopPointerScreenPoint(event desktopInput, capture desktopCapture) (int, int) {
	// New clients identify the coordinate space of the JPEG they actually saw.
	// Keep the current capture dimensions as a compatibility fallback for older
	// consoles, but never reinterpret a new packet after the capture profile has
	// changed between idle and interactive streaming.
	coordinateWidth := event.CoordinateWidth
	coordinateHeight := event.CoordinateHeight
	if coordinateWidth <= 0 {
		coordinateWidth = capture.FrameWidth
	}
	if coordinateHeight <= 0 {
		coordinateHeight = capture.FrameHeight
	}
	// Both the browser and the Windows absolute-pointer API use inclusive pixel
	// endpoints.  Mapping width-to-width used to leave the last physical pixel
	// unreachable and made an out-of-date mobile packet jump outside ultrawide or
	// portrait desktops.  Clamp in the packet's own space and preserve both
	// endpoints exactly.
	packetX := min(max(event.X, 0), max(0, coordinateWidth-1))
	packetY := min(max(event.Y, 0), max(0, coordinateHeight-1))
	return capture.ScreenX + packetX*max(0, capture.ScreenWidth-1)/max(1, coordinateWidth-1),
		capture.ScreenY + packetY*max(0, capture.ScreenHeight-1)/max(1, coordinateHeight-1)
}

func sendDesktopSecureAttentionSequence() error {
	// A LocalSystem child in the interactive session is still not the NT service
	// authorized by Windows software-SAS policy. Ask the real SCM service to call
	// SendSAS while impersonating this session and wait for its acknowledgement.
	return requestWindowsServiceSAS()
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
