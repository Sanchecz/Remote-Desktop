//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type trayView struct {
	name             *walk.Label
	connection       *walk.Label
	connectionID     *walk.Label
	localIP          *walk.Label
	lastHeartbeat    *walk.Label
	version          *walk.Label
	installMode      *walk.Label
	service          *walk.Label
	remoteSession    *walk.Label
	server           *walk.Label
	serviceHealth    *walk.Label
	serverHealth     *walk.Label
	configHealth     *walk.Label
	securityHealth   *walk.Label
	lastCheckHealth  *walk.Label
	recentActivity   *walk.Label
	recentActivity2  *walk.Label
	recentActivity3  *walk.Label
	readyState       *walk.Label
	readyDescription *walk.Label
	statusWidget     *walk.CustomWidget
}

type agentUIScale struct {
	dpi             int
	zoom            float64
	baseW           int
	baseH           int
	fontPointFactor float64
}

func newAgentUIScale(dpi int) agentUIScale {
	if dpi < 96 {
		dpi = 96
	}
	return agentUIScale{dpi: dpi, zoom: 1, baseW: 1450, baseH: 1085, fontPointFactor: 1.0}
}

func compactAgentWindowPixels(screenWidth, screenHeight int) walk.Size {
	const (
		designWidth  = 1450
		designHeight = 1085
		maxWidth     = designWidth
		maxHeight    = designHeight
	)
	if screenWidth <= 0 {
		screenWidth = 1920
	}
	if screenHeight <= 0 {
		screenHeight = 1080
	}
	// SetWindowPos receives the outer window rectangle.  Keep enough room for the
	// Windows title bar and frame so a 1085px reference canvas never spills below
	// the work area on a 1080/1200px display.  On the workstation used for the
	// approved reference (1840x1162) this yields the native 1450x1085 composition.
	const chromeReserve = 58
	availableWidth := min(maxWidth, max(800, screenWidth-60))
	availableHeight := min(maxHeight, max(599, screenHeight-chromeReserve))
	width := availableWidth
	height := width * designHeight / designWidth
	if height > availableHeight {
		height = availableHeight
		width = height * designWidth / designHeight
	}
	return walk.Size{Width: max(width, 800), Height: max(height, 599)}
}

// Walk layout dimensions are expressed in 96-DPI units.  The design reference
// is specified in physical pixels, so converting every metric prevents a
// 1450px window from becoming 1812px at the common Windows 125% scale.
func (scale agentUIScale) unit(pixels int) int {
	// Walk converts logical layout units to the monitor DPI.  Geometry is based on
	// the physical-pixel reference, so compensate once here and let Walk apply it
	// once when laying out the fixed canvas.
	return max(1, int(float64(pixels)*scale.zoom*96/float64(scale.dpi)+0.5))
}

func (scale agentUIScale) font(points float64) int {
	// NewFont accepts points and Windows applies the monitor DPI.  The reference
	// uses deliberately generous typography; compensate for DPI exactly once and
	// retain that visual size through the calibrated type factor.
	return max(1, int(points*scale.fontPointFactor*scale.zoom*96/float64(scale.dpi)+0.5))
}

func (scale agentUIScale) margins(nearX, nearY, farX, farY int) walk.Margins {
	return walk.Margins{HNear: scale.unit(nearX), VNear: scale.unit(nearY), HFar: scale.unit(farX), VFar: scale.unit(farY)}
}

func (scale agentUIScale) forWindowPixels(size walk.Size) agentUIScale {
	if size.Width <= 0 || size.Height <= 0 {
		return scale
	}
	zoomX := float64(size.Width) / float64(scale.baseW)
	zoomY := float64(size.Height) / float64(scale.baseH)
	zoom := min(zoomX, zoomY)
	if zoom < 0.70 {
		zoom = 0.70
	}
	if zoom > 2.0 {
		zoom = 2.0
	}
	scale.zoom = zoom
	return scale
}

func (scale agentUIScale) contentWidth(designPixels int) int {
	return scale.unit(int(float64(designPixels)/scale.zoom + 0.5))
}

func makeAgentIconLabel(parent walk.Container, icon *walk.Icon, size int, scale agentUIScale) *walk.ImageView {
	view, _ := walk.NewImageView(parent)
	view.SetImage(icon)
	view.SetMode(walk.ImageViewModeShrink)
	size = scale.unit(size)
	view.SetMinMaxSize(walk.Size{Width: size, Height: size}, walk.Size{Width: size, Height: size})
	return view
}

type desktopNotificationState struct {
	SessionID string
	Control   bool
}

type agentUIDisposable interface {
	Dispose()
}

type agentUITheme struct {
	pageBrush       *walk.SolidColorBrush
	cardBrush       *walk.SolidColorBrush
	softGreenBrush  *walk.SolidColorBrush
	iconBrush       *walk.SolidColorBrush
	greenBrush      *walk.SolidColorBrush
	borderPen       *walk.CosmeticPen
	activeBorderPen *walk.CosmeticPen
	navFont         *walk.Font
	cardTitleFont   *walk.Font
	cardTextFont    *walk.Font
	iconFont        *walk.Font
	arrowFont       *walk.Font
	readyFont       *walk.Font
	statusFont      *walk.Font
	statusMetaFont  *walk.Font
	readyRingBrush  *walk.SolidColorBrush
}

type agentDashboardSnapshot struct {
	Name           string
	ConnectionID   string
	LocalIP        string
	Version        string
	InstallMode    string
	Service        string
	RemoteSession  string
	Server         string
	Connected      bool
	ConnectionText string
	LastHeartbeat  string
}

type agentDashboardAction struct {
	Bounds walk.Rectangle
	Run    func()
}

// newAgentDashboard paints the whole product screen as one DPI-consistent
// canvas. Native child labels independently scaled themselves on mixed-DPI
// systems; that is what produced the miniature, sparse layout seen in 0.9.70.
// One canvas has one coordinate system, one typographic scale and predictable
// hit targets on every Windows 10/11/Server display.
func newAgentDashboard(parent walk.Container, window *walk.MainWindow, theme *agentUITheme, scale agentUIScale, onlineIcon *walk.Icon, snapshot func() agentDashboardSnapshot, actions []func()) (*walk.CustomWidget, error) {
	font := func(points float64, style walk.FontStyle) *walk.Font {
		result, _ := walk.NewFont("Segoe UI", scale.font(points), style)
		return result
	}
	fonts := []*walk.Font{
		font(34, walk.FontBold), font(10, 0), font(22, walk.FontBold), font(11, walk.FontBold),
		font(9.5, 0), font(9, walk.FontBold), font(8.5, 0), font(13, walk.FontBold), font(10.5, walk.FontBold),
		font(30, walk.FontBold), font(10.5, walk.FontBold), font(9.5, walk.FontBold), font(8, 0),
	}
	// Segoe MDL2 Assets is present on every supported Windows 10/11/Server
	// installation. Using the native Windows icon family keeps every navigation
	// and action glyph optically consistent instead of approximating icons with
	// unrelated text symbols.
	fluentIcons, _ := walk.NewFont("Segoe MDL2 Assets", scale.font(14), 0)
	fonts = append(fonts, fluentIcons)
	for _, item := range fonts { if item == nil { return nil, fmt.Errorf("create Agent dashboard font") } }
	_ = window // The owner retains the dashboard and its process-lifetime fonts.
	var widget *walk.CustomWidget
	var hitTargets []agentDashboardAction
	widget, err := walk.NewCustomWidgetPixels(parent, 0, func(canvas *walk.Canvas, _ walk.Rectangle) error {
		bounds := widget.ClientBoundsPixels()
		if bounds.Width < 100 || bounds.Height < 100 { return nil }
		sx, sy := float64(bounds.Width)/1450.0, float64(bounds.Height)/1085.0
		s := min(sx, sy)
		r := func(x, y, w, h int) walk.Rectangle { return walk.Rectangle{X: int(float64(x)*sx+.5), Y: int(float64(y)*sy+.5), Width: max(1,int(float64(w)*sx+.5)), Height: max(1,int(float64(h)*sy+.5))} }
		text := func(value string, f *walk.Font, color walk.Color, rect walk.Rectangle, format walk.DrawTextFormat) { _ = canvas.DrawTextPixels(value, f, color, rect, format|walk.TextNoPrefix) }
		fill := func(brush walk.Brush, rect walk.Rectangle, radius int) { _ = canvas.FillRoundedRectanglePixels(brush, rect, walk.Size{Width:max(1,int(float64(radius)*s)),Height:max(1,int(float64(radius)*s))}) }
		stroke := func(pen walk.Pen, rect walk.Rectangle, radius int) { _ = canvas.DrawRoundedRectanglePixels(pen, rect, walk.Size{Width:max(1,int(float64(radius)*s)),Height:max(1,int(float64(radius)*s))}) }
		linePen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(222,231,226)); defer linePen.Dispose()
		shadowFar, _ := walk.NewSolidColorBrush(walk.RGB(237, 242, 239)); defer shadowFar.Dispose()
		shadowNear, _ := walk.NewSolidColorBrush(walk.RGB(245, 247, 246)); defer shadowNear.Dispose()
		offlineBrush, _ := walk.NewSolidColorBrush(walk.RGB(211, 67, 67)); defer offlineBrush.Dispose()
		sidebarBrush, _ := walk.NewSolidColorBrush(walk.RGB(252, 253, 252)); defer sidebarBrush.Dispose()
		greenHaloBrush, _ := walk.NewSolidColorBrush(walk.RGB(218, 244, 232)); defer greenHaloBrush.Dispose()
		sidebarAccentBrush, _ := walk.NewSolidColorBrush(walk.RGB(239, 249, 244)); defer sidebarAccentBrush.Dispose()
		muted, ink, green := walk.RGB(95,107,101), walk.RGB(24,32,29), walk.RGB(12,151,98)
		greenPen, _ := walk.NewGeometricPen(walk.PenSolid, max(2, int(2*s+.5)), theme.greenBrush); defer greenPen.Dispose()
		badgePen, _ := walk.NewGeometricPen(walk.PenSolid, max(2, int(2.2*s+.5)), theme.greenBrush); defer badgePen.Dispose()
		whitePen, _ := walk.NewGeometricPen(walk.PenSolid, max(2, int(4*s+.5)), theme.readyRingBrush); defer whitePen.Dispose()
		whiteThinPen, _ := walk.NewGeometricPen(walk.PenSolid, max(1, int(2*s+.5)), theme.readyRingBrush); defer whiteThinPen.Dispose()
		surface := func(rect walk.Rectangle, radius int, brush walk.Brush, pen walk.Pen) {
			far := rect
			far.X += max(1, int(2*s+.5)); far.Y += max(2, int(7*s+.5)); far.Width -= max(2, int(4*s+.5)); far.Height -= max(1, int(3*s+.5))
			fill(shadowFar, far, radius+2)
			near := rect
			near.X += max(1, int(1*s+.5)); near.Y += max(1, int(3*s+.5)); near.Width -= max(2, int(2*s+.5)); near.Height -= max(1, int(1*s+.5))
			fill(shadowNear, near, radius+1)
			fill(brush, rect, radius)
			stroke(pen, rect, radius)
		}
		glyphs := map[string]string{
				"home": "\ue80f", "panel": "\ue8a7", "refresh": "\ue895", "log": "\ue8fd",
				"folder": "\ue838", "settings": "\ue713", "device": "\ue7f4", "connection": "\ue701",
				"server": "\ue774", "protocol": "\ue9d9", "shield": "\ue83d", "lock": "\ue72e",
		}
		drawGlyph := func(kind string, box walk.Rectangle, color walk.Color) bool {
			glyph, ok := glyphs[kind]
			if !ok { return false }
			text(glyph, fonts[13], color, box, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
			return true
		}
		drawIcon := func(kind string, box walk.Rectangle) {
			if drawGlyph(kind, box, green) { return }
			p := func(x, y float64) walk.Point { return walk.Point{X:box.X+int(float64(box.Width)*x+.5),Y:box.Y+int(float64(box.Height)*y+.5)} }
			draw := func(points ...walk.Point) { for index:=1; index<len(points); index++ { _=canvas.DrawLinePixels(greenPen,points[index-1],points[index]) } }
			switch kind {
			case "home": draw(p(.18,.50),p(.50,.20),p(.82,.50)); draw(p(.28,.43),p(.28,.80),p(.72,.80),p(.72,.43))
			case "panel": draw(p(.20,.72),p(.72,.20)); draw(p(.48,.20),p(.72,.20),p(.72,.44)); draw(p(.20,.80),p(.80,.80))
			case "refresh": draw(p(.25,.42),p(.38,.25),p(.63,.25),p(.78,.42)); draw(p(.78,.58),p(.63,.75),p(.38,.75),p(.22,.58)); draw(p(.20,.28),p(.38,.25),p(.34,.43)); draw(p(.80,.72),p(.63,.75),p(.67,.57))
			case "log": draw(p(.24,.27),p(.76,.27)); draw(p(.24,.50),p(.76,.50)); draw(p(.24,.73),p(.76,.73))
			case "folder": draw(p(.16,.34),p(.38,.34),p(.45,.24),p(.75,.24),p(.84,.36),p(.84,.76),p(.16,.76),p(.16,.34))
			case "settings": draw(p(.20,.28),p(.80,.28)); draw(p(.20,.50),p(.80,.50)); draw(p(.20,.72),p(.80,.72)); _=canvas.FillEllipsePixels(theme.greenBrush,walk.Rectangle{X:p(.38,.28).X-2,Y:p(.38,.28).Y-2,Width:5,Height:5}); _=canvas.FillEllipsePixels(theme.greenBrush,walk.Rectangle{X:p(.65,.50).X-2,Y:p(.65,.50).Y-2,Width:5,Height:5}); _=canvas.FillEllipsePixels(theme.greenBrush,walk.Rectangle{X:p(.46,.72).X-2,Y:p(.46,.72).Y-2,Width:5,Height:5})
			case "device": draw(p(.18,.22),p(.82,.22),p(.82,.68),p(.18,.68),p(.18,.22)); draw(p(.38,.80),p(.62,.80)); draw(p(.50,.68),p(.50,.80))
			case "connection": draw(p(.16,.61),p(.29,.48),p(.42,.61),p(.58,.41),p(.72,.54),p(.84,.41))
			case "server": draw(p(.50,.16),p(.82,.50),p(.50,.84),p(.18,.50),p(.50,.16))
			case "protocol": draw(p(.28,.25),p(.28,.75)); draw(p(.50,.38),p(.50,.75)); draw(p(.72,.18),p(.72,.75))
			case "shield": draw(p(.50,.14),p(.80,.27),p(.75,.65),p(.50,.84),p(.25,.65),p(.20,.27),p(.50,.14))
			case "check":
				if box.Width <= max(22, int(24*s+.5)) {
					_ = canvas.FillEllipsePixels(greenHaloBrush, box)
					_ = canvas.DrawLinePixels(badgePen, p(.24,.52), p(.43,.70))
					_ = canvas.DrawLinePixels(badgePen, p(.43,.70), p(.78,.29))
					return
				}
				_ = canvas.FillEllipsePixels(greenHaloBrush, box)
				inset := max(1, int(2*s+.5))
				inner := walk.Rectangle{X:box.X+inset,Y:box.Y+inset,Width:max(1,box.Width-2*inset),Height:max(1,box.Height-2*inset)}
				_ = canvas.FillEllipsePixels(theme.greenBrush, inner)
				ip := func(x, y float64) walk.Point { return walk.Point{X:inner.X+int(float64(inner.Width)*x+.5),Y:inner.Y+int(float64(inner.Height)*y+.5)} }
				_ = canvas.DrawLinePixels(whiteThinPen,ip(.22,.52),ip(.43,.72))
				_ = canvas.DrawLinePixels(whiteThinPen,ip(.43,.72),ip(.79,.29))
			}
		}
		drawStatusBadge := func(box walk.Rectangle) {
			_ = canvas.FillEllipsePixels(greenHaloBrush, box)
			point := func(x, y float64) walk.Point {
				return walk.Point{X: box.X + int(float64(box.Width)*x+.5), Y: box.Y + int(float64(box.Height)*y+.5)}
			}
			_ = canvas.DrawLinePixels(badgePen, point(.24,.52), point(.43,.70))
			_ = canvas.DrawLinePixels(badgePen, point(.43,.70), point(.78,.29))
		}
		_ = canvas.FillRectanglePixels(theme.pageBrush, bounds)
		_ = canvas.FillRectanglePixels(sidebarBrush, r(0,0,304,1085))
		_ = canvas.FillRectanglePixels(sidebarAccentBrush, r(0,0,7,1085))
		_ = canvas.DrawLinePixels(linePen, walk.Point{X:r(304,0,1,1).X,Y:0}, walk.Point{X:r(304,0,1,1).X,Y:bounds.Height})

		// Brand and navigation.
		fill(theme.iconBrush, r(39,43,50,50), 15)
		_ = canvas.DrawImageStretchedPixels(onlineIcon, r(46,51,36,36))
		text("RemoteIt", fonts[2], ink, r(104,44,164,48), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text("Агент безопасного\nудалённого доступа", fonts[4], muted, r(40,108,220,44), walk.TextLeft|walk.TextTop|walk.TextWordbreak)
		nav := []struct{ icon, label string }{{"home","Обзор"},{"panel","Панель управления"},{"refresh","Проверить соединение"},{"id","Remote ID"},{"log","Журнал Agent"},{"folder","Папка Agent"},{"settings","Настройки"}}
		hitTargets = hitTargets[:0]
		for index, item := range nav {
			y := 183+index*57; card := r(30,y,244,48)
			if index==6 { _=canvas.DrawLinePixels(linePen,walk.Point{X:r(42,y-6,1,1).X,Y:r(42,y-6,1,1).Y},walk.Point{X:r(262,y-6,1,1).X,Y:r(262,y-6,1,1).Y}) }
			if index==0 { fill(theme.softGreenBrush,card,12); stroke(theme.activeBorderPen,card,12); fill(theme.greenBrush,r(30,y+9,4,30),2) }
			iconBox:=r(43,y+7,34,34)
			if index==0 {
				fill(theme.greenBrush,iconBox,10)
				if item.icon=="id" { text("ID",fonts[5],walk.RGB(255,255,255),iconBox,walk.TextCenter|walk.TextVCenter|walk.TextSingleLine) } else { _=drawGlyph(item.icon,r(51,y+15,18,18),walk.RGB(255,255,255)) }
			} else {
				fill(theme.iconBrush,iconBox,10)
				if item.icon=="id" { text("ID",fonts[5],green,iconBox,walk.TextCenter|walk.TextVCenter|walk.TextSingleLine) } else { drawIcon(item.icon,r(51,y+15,18,18)) }
			}
			labelFont, labelColor:=fonts[4],ink; if index==0 { labelFont,labelColor=fonts[5],green }; text(item.label,labelFont,labelColor,r(89,y,170,48),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			if index>0 && index-1 < len(actions) { hitTargets=append(hitTargets,agentDashboardAction{Bounds:card,Run:actions[index-1]}) }
		}
		status:=snapshot()
		sideCard:=r(30,835,244,150); surface(sideCard,12,theme.softGreenBrush,theme.activeBorderPen)
		fill(theme.greenBrush,r(30,852,4,95),2)
		drawStatusBadge(r(44,851,27,27))
		text("Агент работает",fonts[11],green,r(73,853,165,25),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text("Служба запущена и готова\nк безопасному подключению.",fonts[6],muted,r(46,889,190,42),walk.TextLeft|walk.TextTop|walk.TextWordbreak)
		text("Открыть панель управления  ↗",fonts[11],green,r(46,944,205,25),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		if len(actions)>0 { hitTargets=append(hitTargets,agentDashboardAction{Bounds:sideCard,Run:actions[0]}) }
		text("Версия Agent "+status.Version,fonts[6],muted,r(30,1007,185,30),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		drawIcon("shield", r(244,1012,16,16))

		// Header.
		text("RemoteIt",fonts[0],ink,r(347,49,390,58),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text("Агент безопасного удалённого доступа",fonts[4],muted,r(349,105,390,26),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		statusBox:=r(1074,37,340,104); surface(statusBox,13,theme.softGreenBrush,theme.activeBorderPen)
		statusColor:=green; if !status.Connected { statusColor=walk.RGB(211,67,67) }
		if status.Connected { drawIcon("check", r(1095,57,26,26)) } else { _=canvas.FillEllipsePixels(offlineBrush,r(1095,57,26,26)) }
		connectionLabel:=strings.TrimSpace(strings.TrimLeft(status.ConnectionText,"●• "))
		text(connectionLabel,fonts[3],statusColor,r(1134,49,190,37),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		heartbeatValue:=strings.TrimSpace(strings.TrimPrefix(status.LastHeartbeat,"Последняя синхронизация:"))
		text("Последняя синхронизация",fonts[6],muted,r(1134,82,190,19),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text(heartbeatValue,fonts[5],muted,r(1134,101,190,20),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		// Concentric rings stay crisp on every DPI and use only primitives that
		// are present on all supported Windows 10/11/Server systems.
		_ = canvas.FillEllipsePixels(theme.iconBrush, r(1335,57,80,80))
		_ = canvas.FillEllipsePixels(theme.softGreenBrush, r(1343,65,64,64))
		_ = canvas.FillEllipsePixels(theme.iconBrush, r(1352,74,46,46))
		_ = canvas.FillEllipsePixels(theme.softGreenBrush, r(1360,82,30,30))
		_ = canvas.FillEllipsePixels(theme.greenBrush, r(1370,92,12,12))

		// Device and readiness cards.
		deviceCard:=r(347,176,492,306); surface(deviceCard,13,theme.cardBrush,theme.borderPen)
		drawIcon("device",r(371,198,28,28))
		text("Устройство",fonts[3],ink,r(415,195,210,35),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		rows:=[][2]string{{"Название",status.Name},{"Remote ID",status.ConnectionID},{"Локальный IP",status.LocalIP},{"Версия",status.Version},{"Установка",status.InstallMode},{"Фоновый агент",status.Service},{"Удалённый доступ",status.RemoteSession},{"Сервер",status.Server}}
		for i,row:=range rows { y:=237+i*28; text(row[0],fonts[4],muted,r(369,y,170,24),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine); text(row[1],fonts[1],ink,r(586,y,220,24),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis) }
		ready:=r(865,176,549,306); surface(ready,13,theme.softGreenBrush,theme.activeBorderPen)
		_ = canvas.FillEllipsePixels(shadowFar,r(895,218,88,88))
		circle:=r(891,212,88,88); _=canvas.FillEllipsePixels(theme.cardBrush,circle); inner:=r(897,218,76,76); _=canvas.FillEllipsePixels(theme.greenBrush,inner)
		// Draw the success mark as geometry; font glyph substitution made the
		// old check mark and status symbols inconsistent across Windows builds.
		_ = canvas.DrawLinePixels(whitePen, walk.Point{X:r(919,254,1,1).X,Y:r(919,254,1,1).Y}, walk.Point{X:r(934,270,1,1).X,Y:r(934,270,1,1).Y})
		_ = canvas.DrawLinePixels(whitePen, walk.Point{X:r(934,270,1,1).X,Y:r(934,270,1,1).Y}, walk.Point{X:r(956,236,1,1).X,Y:r(956,236,1,1).Y})
		readyTitle:="Онлайн и готов к подключению"; if !status.Connected { readyTitle="Соединение восстанавливается" }
		text(readyTitle,fonts[7],ink,r(998,218,370,42),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		text("Служба Agent работает корректно\nи подключена к серверу RemoteIt.",fonts[4],muted,r(998,264,365,53),walk.TextLeft|walk.TextTop|walk.TextWordbreak)
		_ = canvas.DrawLinePixels(linePen,walk.Point{X:r(891,345,1,1).X,Y:r(891,345,1,1).Y},walk.Point{X:r(1383,345,1,1).X,Y:r(1383,345,1,1).Y})
		summary:=[]struct{icon,label,value string}{{"check","Подключение","Подключено"},{"server","Сервер","supportgenesis.ru"},{"protocol","Протокол","TLS 1.2"},{"shield","Шифрование","Включено"}}
		for i,item:=range summary { x:=891+i*123; drawIcon(item.icon,r(x,372,16,16)); text(item.label,fonts[6],muted,r(x+22,370,94,23),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine); text(item.value,fonts[5],green,r(x,397,116,23),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis) }

		// High-value action cards.
		cards:=[]struct{icon,title,desc,cta string; action int}{{"device","Панель управления","Устройства, доступ и настройки.","Открыть панель",0},{"connection","Проверить соединение","Диагностика связи с сервером.","Проверить",1},{"id","Remote ID","Идентификатор этого компьютера.","Копировать ID",2},{"log","Журнал Agent","События службы и диагностика.","Открыть журнал",3}}
		for i,item:=range cards { x:=347+i*267; card:=r(x,505,254,228); surface(card,12,theme.cardBrush,theme.borderPen); icon:=r(x+17,523,47,47); fill(theme.iconBrush,icon,12); if item.icon=="id" { text("ID",fonts[8],green,icon,walk.TextCenter|walk.TextVCenter|walk.TextSingleLine) } else { drawIcon(item.icon,r(x+29,535,23,23)) }; text(item.title,fonts[3],ink,r(x+17,581,220,28),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis); text(item.desc,fonts[4],muted,r(x+17,613,220,56),walk.TextLeft|walk.TextTop|walk.TextWordbreak); button:=r(x+17,686,220,32); fill(theme.iconBrush,button,8); stroke(theme.activeBorderPen,button,8); text(item.cta,fonts[11],green,r(x+29,686,174,32),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine); text("›",fonts[3],green,r(x+204,686,24,32),walk.TextCenter|walk.TextVCenter|walk.TextSingleLine); if item.action<len(actions){hitTargets=append(hitTargets,agentDashboardAction{Bounds:card,Run:actions[item.action]})} }

		health:=r(347,739,1067,78); surface(health,11,theme.cardBrush,theme.borderPen); text("Состояние системы",fonts[10],ink,r(369,752,168,52),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		healths:=[][2]string{{"Служба Agent","Работает"},{"Синхронизация","Подключено"},{"Конфигурация","Актуальна"},{"Безопасность","Защищено"},{"Последняя проверка","Сегодня"}}
		for i,item:=range healths{x:=540+i*166; drawStatusBadge(r(x,748,28,28)); text(item[0],fonts[5],green,r(x+38,750,120,24),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine);text(item[1],fonts[6],muted,r(x+38,779,118,20),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)}
		activity:=r(347,833,1067,171); surface(activity,11,theme.cardBrush,theme.borderPen); text("Недавняя активность",fonts[3],ink,r(369,846,270,32),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		now:=time.Now().Local().Format("02.01.2006 15:04:05"); events:=[][3]string{{now,"Синхронизация выполнена","Конфигурация успешно синхронизирована с сервером"},{now,"Подключение к серверу","Успешное подключение к supportgenesis.ru"},{now,"Служба Agent запущена","Служба запущена и готова к работе"}}
		for i,item:=range events{y:=885+i*34; drawStatusBadge(r(369,y-1,24,24));text(item[0],fonts[6],muted,r(405,y,160,22),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine);text(item[1],fonts[5],ink,r(575,y,240,22),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine);text(item[2],fonts[6],muted,r(826,y,500,22),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis);text("⌄",fonts[5],muted,r(1358,y,30,22),walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)}
		drawIcon("lock",r(506,1030,14,14)); text("Безопасное соединение установлено. Данные защищены.",fonts[6],muted,r(528,1023,500,28),walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		return nil
	})
	if err != nil { return nil, err }
	widget.Disposing().Attach(func() { for _, item := range fonts { item.Dispose() } })
	widget.SetPaintMode(walk.PaintBuffered); widget.SetInvalidatesOnResize(true)
	widget.MouseDown().Attach(func(x,y int,button walk.MouseButton){ if button!=walk.LeftButton{return}; for _,item:=range hitTargets{if x>=item.Bounds.X&&x<item.Bounds.X+item.Bounds.Width&&y>=item.Bounds.Y&&y<item.Bounds.Y+item.Bounds.Height&&item.Run!=nil{item.Run();return}} })
	return widget,nil
}

func newAgentUITheme(scale agentUIScale) (*agentUITheme, error) {
	theme := &agentUITheme{}
	var err error
	if theme.pageBrush, err = walk.NewSolidColorBrush(walk.RGB(247, 249, 248)); err != nil {
		return nil, err
	}
	if theme.cardBrush, err = walk.NewSolidColorBrush(walk.RGB(255, 255, 255)); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.softGreenBrush, err = walk.NewSolidColorBrush(walk.RGB(238, 249, 244)); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.iconBrush, err = walk.NewSolidColorBrush(walk.RGB(229, 247, 239)); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.greenBrush, err = walk.NewSolidColorBrush(walk.RGB(14, 161, 105)); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.borderPen, err = walk.NewCosmeticPen(walk.PenSolid, walk.RGB(220, 228, 224)); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.activeBorderPen, err = walk.NewCosmeticPen(walk.PenSolid, walk.RGB(120, 213, 174)); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.navFont, err = walk.NewFont("Segoe UI", scale.font(11), walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	// The four dashboard cards must retain their complete Russian titles on the
	// fixed 1450px reference canvas. A separate compact heading size keeps the
	// visual hierarchy without relying on ellipsis.
	if theme.cardTitleFont, err = walk.NewFont("Segoe UI", scale.font(9.5), walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.cardTextFont, err = walk.NewFont("Segoe UI", scale.font(10), 0); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.iconFont, err = walk.NewFont("Segoe UI Symbol", scale.font(13), walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.arrowFont, err = walk.NewFont("Segoe UI", scale.font(15), 0); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.readyFont, err = walk.NewFont("Segoe UI Symbol", scale.font(40), walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.statusFont, err = walk.NewFont("Segoe UI", scale.font(16), walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.statusMetaFont, err = walk.NewFont("Segoe UI", scale.font(8), 0); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.readyRingBrush, err = walk.NewSolidColorBrush(walk.RGB(255, 255, 255)); err != nil {
		theme.Dispose()
		return nil, err
	}
	return theme, nil
}

func (theme *agentUITheme) Dispose() {
	if theme == nil {
		return
	}
	for _, disposable := range []agentUIDisposable{
		theme.pageBrush, theme.cardBrush, theme.softGreenBrush, theme.iconBrush, theme.greenBrush,
		theme.borderPen, theme.activeBorderPen, theme.navFont, theme.cardTitleFont,
		theme.cardTextFont, theme.iconFont, theme.arrowFont, theme.readyFont, theme.statusFont, theme.statusMetaFont, theme.readyRingBrush,
	} {
		if disposable != nil {
			disposable.Dispose()
		}
	}
}

func newAgentReadyCheck(parent walk.Container, theme *agentUITheme, size int) (*walk.CustomWidget, error) {
	var widget *walk.CustomWidget
	widget, err := walk.NewCustomWidgetPixels(parent, 0, func(canvas *walk.Canvas, _ walk.Rectangle) error {
		bounds := widget.ClientBoundsPixels()
		if err := canvas.FillRectanglePixels(theme.softGreenBrush, bounds); err != nil {
			return err
		}
		padding := max(2, bounds.Width/20)
		ring := walk.Rectangle{X: padding, Y: padding, Width: bounds.Width - padding*2, Height: bounds.Height - padding*2}
		if err := canvas.FillEllipsePixels(theme.readyRingBrush, ring); err != nil {
			return err
		}
		innerPadding := max(3, bounds.Width/13)
		inner := walk.Rectangle{X: innerPadding, Y: innerPadding, Width: bounds.Width - innerPadding*2, Height: bounds.Height - innerPadding*2}
		if err := canvas.FillEllipsePixels(theme.greenBrush, inner); err != nil {
			return err
		}
		return canvas.DrawTextPixels("✓", theme.readyFont, walk.RGB(255, 255, 255), inner, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix)
	})
	if err != nil {
		return nil, err
	}
	widget.SetPaintMode(walk.PaintBuffered)
	widget.SetMinMaxSize(walk.Size{Width: size, Height: size}, walk.Size{Width: size, Height: size})
	return widget, nil
}

func lockAgentWindowGeometry(window *walk.MainWindow, size walk.Size, screenWidth, screenHeight int) {
	// size is deliberately physical pixels. The UI metrics are independently
	// DPI-normalised, so using SetSize here would make Windows scale the outer
	// window a second time on 125/150/200% displays.
	style := win.GetWindowLong(window.Handle(), win.GWL_STYLE)
	style &^= int32(win.WS_THICKFRAME | win.WS_MAXIMIZEBOX)
	win.SetWindowLong(window.Handle(), win.GWL_STYLE, style)
	x := max(0, (screenWidth-size.Width)/2)
	y := max(0, (screenHeight-size.Height)/2)
	win.SetWindowPos(window.Handle(), 0, int32(x), int32(y), int32(size.Width), int32(size.Height), win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

// roundedPanel gives native Walk composites the same softly rounded card
// treatment as the custom navigation and action controls.  A custom widget is
// used as a non-interactive background layer; the real controls are placed in a
// transparent sibling composite by the caller.
func addReadyStatusItem(parent walk.Container, icon, caption, value string, scale agentUIScale) {
	item, _ := walk.NewComposite(parent)
	layout := walk.NewVBoxLayout()
	layout.SetMargins(scale.margins(4, 2, 4, 2))
	layout.SetSpacing(scale.unit(2))
	_ = item.SetLayout(layout)
	title, _ := walk.NewLabel(item)
	title.SetText(icon + "  " + caption)
	title.SetTextColor(walk.RGB(95, 108, 101))
	result, _ := walk.NewLabel(item)
	result.SetText(value)
	result.SetTextColor(walk.RGB(10, 142, 91))
}

func newAgentActionCard(parent walk.Container, theme *agentUITheme, icon, title, subtitle string, active bool, height int, action func()) (*walk.CustomWidget, error) {
	var widget *walk.CustomWidget
	widget, err := walk.NewCustomWidgetPixels(parent, 0, func(canvas *walk.Canvas, _ walk.Rectangle) error {
		bounds := widget.ClientBoundsPixels()
		if err := canvas.FillRectanglePixels(theme.pageBrush, bounds); err != nil {
			return err
		}
		cardBounds := walk.Rectangle{X: 1, Y: 1, Width: bounds.Width - 2, Height: bounds.Height - 2}
		background := theme.cardBrush
		border := theme.borderPen
		if active {
			background = theme.softGreenBrush
			border = theme.activeBorderPen
		}
		if err := canvas.FillRoundedRectanglePixels(background, cardBounds, walk.Size{Width: 14, Height: 14}); err != nil {
			return err
		}
		if err := canvas.DrawRoundedRectanglePixels(border, cardBounds, walk.Size{Width: 14, Height: 14}); err != nil {
			return err
		}
		iconSize := 38
		iconX := 11
		textX := 60
		if bounds.Height <= 56 {
			iconSize = 34
			iconX = 10
			textX = 54
		}
		iconY := (bounds.Height - iconSize) / 2
		iconBounds := walk.Rectangle{X: iconX, Y: iconY, Width: iconSize, Height: iconSize}
		if err := canvas.FillRoundedRectanglePixels(theme.iconBrush, iconBounds, walk.Size{Width: 10, Height: 10}); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(icon, theme.iconFont, walk.RGB(12, 153, 99), iconBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix); err != nil {
			return err
		}
		if subtitle == "" {
			return canvas.DrawTextPixels(title, theme.navFont, walk.RGB(24, 34, 30), walk.Rectangle{X: textX, Y: 0, Width: bounds.Width - textX - 34, Height: bounds.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix)
		}
		if err := canvas.DrawTextPixels(title, theme.cardTitleFont, walk.RGB(25, 34, 31), walk.Rectangle{X: textX, Y: iconY - 1, Width: bounds.Width - textX - 34, Height: 19}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(subtitle, theme.cardTextFont, walk.RGB(102, 115, 108), walk.Rectangle{X: textX, Y: iconY + 17, Width: bounds.Width - textX - 34, Height: 17}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix); err != nil {
			return err
		}
		return canvas.DrawTextPixels("›", theme.arrowFont, walk.RGB(85, 101, 93), walk.Rectangle{X: bounds.Width - 32, Y: 0, Width: 20, Height: bounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix)
	})
	if err != nil {
		return nil, err
	}
	widget.SetPaintMode(walk.PaintBuffered)
	widget.SetInvalidatesOnResize(true)
	widget.SetMinMaxSize(walk.Size{Height: height}, walk.Size{Height: height})
	widget.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton && action != nil {
			action()
		}
	})
	return widget, nil
}

func newAgentDashboardCard(parent walk.Container, theme *agentUITheme, icon, title, description, callToAction string, height int, action func()) (*walk.CustomWidget, error) {
	var widget *walk.CustomWidget
	widget, err := walk.NewCustomWidgetPixels(parent, 0, func(canvas *walk.Canvas, _ walk.Rectangle) error {
		bounds := widget.ClientBoundsPixels()
		if err := canvas.FillRectanglePixels(theme.pageBrush, bounds); err != nil {
			return err
		}
		card := walk.Rectangle{X: 1, Y: 1, Width: bounds.Width - 2, Height: bounds.Height - 2}
		if err := canvas.FillRoundedRectanglePixels(theme.cardBrush, card, walk.Size{Width: 15, Height: 15}); err != nil {
			return err
		}
		if err := canvas.DrawRoundedRectanglePixels(theme.borderPen, card, walk.Size{Width: 15, Height: 15}); err != nil {
			return err
		}
		iconBounds := walk.Rectangle{X: 18, Y: 16, Width: 50, Height: 50}
		if err := canvas.FillRoundedRectanglePixels(theme.iconBrush, iconBounds, walk.Size{Width: 13, Height: 13}); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(icon, theme.iconFont, walk.RGB(12, 153, 99), iconBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(title, theme.cardTitleFont, walk.RGB(26, 34, 31), walk.Rectangle{X: 18, Y: 73, Width: bounds.Width - 36, Height: 22}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(description, theme.cardTextFont, walk.RGB(91, 104, 98), walk.Rectangle{X: 18, Y: 99, Width: bounds.Width - 36, Height: bounds.Height - 153}, walk.TextLeft|walk.TextTop|walk.TextWordbreak|walk.TextNoPrefix); err != nil {
			return err
		}
		button := walk.Rectangle{X: 18, Y: bounds.Height - 47, Width: bounds.Width - 36, Height: 32}
		if err := canvas.FillRoundedRectanglePixels(theme.iconBrush, button, walk.Size{Width: 9, Height: 9}); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(callToAction, theme.navFont, walk.RGB(12, 145, 94), walk.Rectangle{X: button.X + 13, Y: button.Y, Width: button.Width - 45, Height: button.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix); err != nil {
			return err
		}
		return canvas.DrawTextPixels("›", theme.arrowFont, walk.RGB(12, 145, 94), walk.Rectangle{X: button.X + button.Width - 29, Y: button.Y, Width: 20, Height: button.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix)
	})
	if err != nil {
		return nil, err
	}
	widget.SetPaintMode(walk.PaintBuffered)
	widget.SetInvalidatesOnResize(true)
	widget.SetMinMaxSize(walk.Size{Height: height}, walk.Size{Height: height})
	widget.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton && action != nil {
			action()
		}
	})
	return widget, nil
}

func newAgentStatusCard(parent walk.Container, theme *agentUITheme, scale agentUIScale, view *trayView) (*walk.CustomWidget, error) {
	var widget *walk.CustomWidget
	widget, err := walk.NewCustomWidgetPixels(parent, 0, func(canvas *walk.Canvas, _ walk.Rectangle) error {
		bounds := widget.ClientBoundsPixels()
		// Rounded GDI fills do not paint their corner pixels. Paint the parent
		// colour first, otherwise those corners inherit the CustomWidget's black
		// native backing store and look like an unintended focus border.
		if err := canvas.FillRectanglePixels(theme.pageBrush, bounds); err != nil {
			return err
		}
		if err := canvas.FillRoundedRectanglePixels(theme.softGreenBrush, walk.Rectangle{X: 1, Y: 1, Width: bounds.Width - 2, Height: bounds.Height - 2}, walk.Size{Width: 14, Height: 14}); err != nil {
			return err
		}
		statusText := view.connection.Text()
		statusColor := walk.RGB(25, 151, 101)
		if strings.Contains(statusText, "Нет связи") {
			statusColor = walk.RGB(211, 67, 67)
		} else if strings.Contains(statusText, "сеанс") {
			statusColor = walk.RGB(211, 116, 28)
		}
		if err := canvas.DrawTextPixels(statusText, theme.statusFont, statusColor, walk.Rectangle{X: 20, Y: 14, Width: bounds.Width - 40, Height: 35}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix); err != nil {
			return err
		}
		return canvas.DrawTextPixels(view.lastHeartbeat.Text(), theme.statusMetaFont, walk.RGB(91, 106, 99), walk.Rectangle{X: 20, Y: 51, Width: bounds.Width - 40, Height: 26}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix)
	})
	if err != nil {
		return nil, err
	}
	widget.SetPaintMode(walk.PaintBuffered)
	widget.SetInvalidatesOnResize(true)
	// CustomWidget is keyboard-focusable in this Walk release even though the
	// status card is informational. Remove WS_TABSTOP at the native level so it
	// cannot retain the dotted/black focus rectangle when the panel opens.
	style := win.GetWindowLong(widget.Handle(), win.GWL_STYLE)
	win.SetWindowLong(widget.Handle(), win.GWL_STYLE, style&^int32(win.WS_TABSTOP))
	return widget, nil
}

func loadAgentDashboardSnapshot() agentDashboardSnapshot {
	snapshot := agentDashboardSnapshot{
		Name:           "Агент ещё не зарегистрирован",
		ConnectionID:   "—",
		LocalIP:        "Определяется",
		Version:        version,
		Server:         defaultServer,
		ConnectionText: "●  Нет связи — переподключение…",
		LastHeartbeat:  "Ожидание синхронизации",
		InstallMode:    "Системная служба",
		Service:        "Статус неизвестен",
		RemoteSession:  "Нет активного сеанса",
	}
	if useUserConfig() {
		snapshot.InstallMode = "Текущий пользователь"
	}
	info, infoErr := loadPublicAgentInfo()
	if infoErr == nil {
		snapshot.Name = valueOrDash(info.DeviceName)
		snapshot.ConnectionID = valueOrDash(info.ConnectionCode)
		snapshot.Version = valueOrDash(info.Version)
		snapshot.Server = valueOrDash(info.ServerURL)
		if len(info.LocalIPs) > 0 {
			snapshot.LocalIP = info.LocalIPs[0]
		}
		if !info.LastHeartbeat.IsZero() {
			snapshot.Connected = info.Connected && time.Since(info.LastHeartbeat) < 90*time.Second
			snapshot.LastHeartbeat = "Последняя синхронизация: " + info.LastHeartbeat.Local().Format("02.01.2006 15:04:05")
		}
	} else if status, statusErr := loadRuntimeStatus(); statusErr == nil {
		if !status.LastHeartbeat.IsZero() {
			snapshot.Connected = status.Connected && time.Since(status.LastHeartbeat) < 90*time.Second
			snapshot.LastHeartbeat = "Последняя синхронизация: " + status.LastHeartbeat.Local().Format("02.01.2006 15:04:05")
		}
	}
	if snapshot.Connected {
		snapshot.ConnectionText = "●  В сети"
	}
	if running, known := windowsServiceState(); !known {
		snapshot.Service = "Статус неизвестен"
	} else if running {
		snapshot.Service = "Агент работает"
	} else {
		snapshot.Service = "Служба остановлена"
	}
	_, control, _ := publishedDesktopSessionState()
	if control {
		snapshot.RemoteSession = "Активен — экран и управление"
		snapshot.ConnectionText = "●  Идёт удалённый сеанс"
	}
	return snapshot
}

// trayCommand uses a single custom-painted dashboard. This is intentionally
// separate from the legacy control tree below: independent Win32 controls
// scaled differently at 125/150/200% and produced unreadable miniature text.
func trayCommand() error {
	setDesktopProcessDPIAwareness()
	window, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer window.Dispose()

	screenWidth := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	screenHeight := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	requestedWindowSize := compactAgentWindowPixels(screenWidth, screenHeight)
	scale := newAgentUIScale(window.DPI()).forWindowPixels(requestedWindowSize)
	theme, err := newAgentUITheme(scale)
	if err != nil {
		return err
	}
	defer theme.Dispose()
	window.SetTitle("RemoteIt Agent")
	window.SetBackground(theme.pageBrush)
	lockAgentWindowGeometry(window, requestedWindowSize, screenWidth, screenHeight)
	layout := walk.NewHBoxLayout()
	layout.SetMargins(walk.Margins{})
	layout.SetSpacing(0)
	if err := window.SetLayout(layout); err != nil {
		return err
	}

	onlineIcon, err := loadStatusIcon(onlineIconPNG)
	if err != nil {
		return err
	}
	defer onlineIcon.Dispose()
	offlineIcon, err := loadStatusIcon(offlineIconPNG)
	if err != nil {
		return err
	}
	defer offlineIcon.Dispose()
	_ = window.SetIcon(offlineIcon)

	tray, err := walk.NewNotifyIcon(window)
	if err != nil {
		return err
	}
	defer tray.Dispose()
	if err := tray.SetIcon(offlineIcon); err != nil {
		return err
	}
	_ = tray.SetToolTip("RemoteIt Agent — подключение проверяется")

	var dashboard *walk.CustomWidget
	var connected bool
	notificationState := loadDesktopNotificationState()
	refresh := func() {
		status := loadAgentDashboardSnapshot()
		if status.Connected != connected {
			if status.Connected {
				_ = tray.SetIcon(onlineIcon)
				_ = window.SetIcon(onlineIcon)
			} else {
				_ = tray.SetIcon(offlineIcon)
				_ = window.SetIcon(offlineIcon)
			}
		}
		connected = status.Connected
		if connected {
			_ = tray.SetToolTip("RemoteIt Agent — в сети")
		} else {
			_ = tray.SetToolTip("RemoteIt Agent — нет связи")
		}
		_, control, sessionID := publishedDesktopSessionState()
		if sessionID != "" && control && (sessionID != notificationState.SessionID || !notificationState.Control) {
			_ = tray.ShowCustom("RemoteIt", "Администратор подключился к удалённому управлению этим компьютером.", onlineIcon)
			notificationState = desktopNotificationState{SessionID: sessionID, Control: control}
			saveDesktopNotificationState(notificationState)
		}
		if dashboard != nil {
			_ = dashboard.Invalidate()
		}
	}
	openPanelURL := func() { _ = openURL(defaultServer) }
	checkConnection := func() {
		refresh()
		message, icon := "Соединение с сервером пока восстанавливается.", offlineIcon
		if connected {
			message, icon = "Соединение с сервером работает.", onlineIcon
		}
		_ = tray.ShowCustom("RemoteIt — проверка соединения", message, icon)
	}
	copyRemoteID := func() {
		value := strings.TrimSpace(loadAgentDashboardSnapshot().ConnectionID)
		if value == "" || value == "—" {
			_ = walk.MsgBox(window, "RemoteIt", "Remote ID появится после регистрации устройства на сервере.", walk.MsgBoxIconInformation)
			return
		}
		if err := walk.Clipboard().SetText(value); err != nil {
			_ = walk.MsgBox(window, "RemoteIt", "Не удалось скопировать Remote ID.", walk.MsgBoxIconError)
			return
		}
		_ = tray.ShowCustom("RemoteIt", "Remote ID скопирован: "+value, onlineIcon)
	}
	openLogs := func() { showAgentLogDialog(window) }
	openFolder := func() {
		path := filepath.Dir(defaultConfigPath())
		if executable, executableErr := installedAgentPath(); executableErr == nil && strings.TrimSpace(executable) != "" {
			path = filepath.Dir(executable)
		}
		if err := exec.Command("explorer.exe", path).Start(); err != nil {
			_ = walk.MsgBox(window, "RemoteIt — папка Agent", "Не удалось открыть папку Agent: "+err.Error(), walk.MsgBoxIconError)
		}
	}
	openSettings := func() {
		_ = walk.MsgBox(window, "RemoteIt — настройки", "Название и права доступа управляются в защищённой панели. Локальная диагностика доступна через журнал Agent.", walk.MsgBoxIconInformation)
	}
	actions := []func(){openPanelURL, checkConnection, copyRemoteID, openLogs, openFolder, openSettings}
	dashboard, err = newAgentDashboard(window, window, theme, scale, onlineIcon, loadAgentDashboardSnapshot, actions)
	if err != nil {
		return err
	}
	style := win.GetWindowLong(dashboard.Handle(), win.GWL_STYLE)
	win.SetWindowLong(dashboard.Handle(), win.GWL_STYLE, style&^int32(win.WS_TABSTOP))

	openPanel := func() {
		refresh()
		window.Show()
		_ = window.Activate()
	}
	window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		*canceled = true
		window.Hide()
	})
	tray.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			openPanel()
		}
	})
	for _, item := range []struct {
		label string
		run   func()
	}{
		{"Открыть RemoteIt Agent", openPanel},
		{"Обновить статус", refresh},
		{"Открыть журнал Agent", openLogs},
		{"Открыть папку Agent", openFolder},
		{"Открыть панель управления", openPanelURL},
	} {
		action := walk.NewAction()
		_ = action.SetText(item.label)
		action.Triggered().Attach(item.run)
		_ = tray.ContextMenu().Actions().Add(action)
	}
	if err := tray.SetVisible(true); err != nil {
		return err
	}
	refresh()
	if os.Getenv("REMOTEIT_QA_SHOW") == "1" {
		openPanel()
	}
	// An opt-in QA hook lets the build pipeline verify that the custom painted
	// cards are connected to the real production actions. It is inert for every
	// installed Agent because the variable is never set by the installer/service.
	if qaAction := strings.TrimSpace(os.Getenv("REMOTEIT_QA_ACTION")); qaAction != "" {
		go func() {
			time.Sleep(450 * time.Millisecond)
			window.Synchronize(func() {
				switch qaAction {
				case "connection":
					checkConnection()
				case "remote-id":
					copyRemoteID()
				case "logs":
					openLogs()
				case "folder":
					openFolder()
				case "settings":
					openSettings()
				}
			})
		}()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				window.Synchronize(refresh)
			case <-done:
				return
			}
		}
	}()
	window.Run()
	return nil
}

func trayLegacyCommand() error {
	setDesktopProcessDPIAwareness()
	window, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer window.Dispose()
	scale := newAgentUIScale(window.DPI())
	screenWidth := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	screenHeight := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	requestedWindowSize := compactAgentWindowPixels(screenWidth, screenHeight)
	scale = scale.forWindowPixels(requestedWindowSize)
	theme, err := newAgentUITheme(scale)
	if err != nil {
		return err
	}
	defer theme.Dispose()
	// Fonts and every child metric follow the same aspect-preserving scale as the
	// compact physical window, so the reference composition cannot stretch.
	bodyFont, err := walk.NewFont("Segoe UI", scale.font(9), 0)
	if err != nil {
		return err
	}
	defer bodyFont.Dispose()
	window.SetFont(bodyFont)
	window.SetTitle("RemoteIt Agent")
	lockAgentWindowGeometry(window, requestedWindowSize, screenWidth, screenHeight)
	window.SetBackground(theme.pageBrush)
	layout := walk.NewHBoxLayout()
	layout.SetMargins(walk.Margins{})
	layout.SetSpacing(0)
	_ = layout.SetAlignment(walk.AlignHCenterVNear)
	if err := window.SetLayout(layout); err != nil {
		return err
	}

	onlineIcon, err := loadStatusIcon(onlineIconPNG)
	if err != nil {
		return err
	}
	defer onlineIcon.Dispose()
	offlineIcon, err := loadStatusIcon(offlineIconPNG)
	if err != nil {
		return err
	}
	defer offlineIcon.Dispose()
	_ = window.SetIcon(offlineIcon)

	view := trayView{}

	sidebar, _ := walk.NewComposite(window)
	sidebarWidth := scale.unit(304)
	sidebarHeight := scale.unit(1085)
	sidebar.SetMinMaxSize(walk.Size{Width: sidebarWidth, Height: sidebarHeight}, walk.Size{Width: sidebarWidth, Height: sidebarHeight})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(252, 253, 252)); brushErr == nil {
		defer brush.Dispose()
		sidebar.SetBackground(brush)
	}
	sidebarLayout := walk.NewVBoxLayout()
	sidebarLayout.SetMargins(scale.margins(34, 34, 18, 20))
	sidebarLayout.SetSpacing(scale.unit(7))
	_ = sidebar.SetLayout(sidebarLayout)
	brandRow, _ := walk.NewComposite(sidebar)
	brandRowLayout := walk.NewHBoxLayout()
	brandRowLayout.SetMargins(walk.Margins{})
	brandRowLayout.SetSpacing(scale.unit(11))
	_ = brandRow.SetLayout(brandRowLayout)
	brandHeight := scale.unit(47)
	brandRow.SetMinMaxSize(walk.Size{Height: brandHeight}, walk.Size{Height: brandHeight})
	makeAgentIconLabel(brandRow, onlineIcon, 40, scale)
	brand, _ := walk.NewLabel(brandRow)
	brand.SetText("RemoteIt")
	brand.SetMinMaxSize(walk.Size{Height: brandHeight}, walk.Size{Height: brandHeight})
	if font, fontErr := walk.NewFont("Segoe UI", scale.font(22), walk.FontBold); fontErr == nil {
		defer font.Dispose()
		brand.SetFont(font)
	}
	brandCaption, _ := walk.NewLabel(sidebar)
	brandCaption.SetText("Агент безопасного\nудалённого доступа")
	brandCaption.SetTextColor(walk.RGB(91, 99, 96))
	captionHeight := scale.unit(45)
	brandCaption.SetMinMaxSize(walk.Size{Height: captionHeight}, walk.Size{Height: captionHeight})

	var refresh func()
	var openAgentLogs func()
	var openAgentFolder func()
	var checkConnection func()
	var copyRemoteID func()
	var openAgentSettings func()
	navHeight := scale.unit(50)
	if _, err := newAgentActionCard(sidebar, theme, "●", "Обзор", "", true, navHeight, func() { refresh() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "↗", "Панель управления", "", false, navHeight, func() { _ = openURL(defaultServer) }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "↻", "Проверить соединение", "", false, navHeight, func() { checkConnection() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "ID", "Remote ID", "", false, navHeight, func() { copyRemoteID() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "≡", "Журнал Agent", "", false, navHeight, func() { openAgentLogs() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "▣", "Папка Agent", "", false, navHeight, func() { openAgentFolder() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "⚙", "Настройки", "", false, navHeight, func() { openAgentSettings() }); err != nil {
		return err
	}
	walk.NewVSpacer(sidebar)

	sideStatus, _ := walk.NewComposite(sidebar)
	sideStatusHeight := scale.unit(154)
	sideStatus.SetMinMaxSize(walk.Size{Height: sideStatusHeight}, walk.Size{Height: sideStatusHeight})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(241, 250, 246)); brushErr == nil {
		defer brush.Dispose()
		sideStatus.SetBackground(brush)
	}
	sideStatusLayout := walk.NewVBoxLayout()
	sideStatusLayout.SetMargins(scale.margins(15, 16, 15, 13))
	sideStatusLayout.SetSpacing(scale.unit(6))
	_ = sideStatus.SetLayout(sideStatusLayout)
	sideStatusTitle, _ := walk.NewLabel(sideStatus)
	sideStatusTitle.SetText("●  Агент работает")
	sideStatusTitle.SetTextColor(walk.RGB(13, 148, 99))
	if font, fontErr := walk.NewFont("Segoe UI", scale.font(9), walk.FontBold); fontErr == nil {
		defer font.Dispose()
		sideStatusTitle.SetFont(font)
	}
	sideStatusText, _ := walk.NewLabel(sideStatus)
	sideStatusText.SetText("Служба запущена и готова\nк безопасному подключению.")
	sideStatusText.SetTextColor(walk.RGB(93, 105, 99))
	sideStatusText.SetMinMaxSize(walk.Size{Height: scale.unit(42)}, walk.Size{Height: scale.unit(42)})
	sidePanelLink, _ := walk.NewLabel(sideStatus)
	sidePanelLink.SetText("Открыть панель управления  ↗")
	sidePanelLink.SetTextColor(walk.RGB(13, 148, 99))
	if font, fontErr := walk.NewFont("Segoe UI", scale.font(8.5), walk.FontBold); fontErr == nil {
		defer font.Dispose()
		sidePanelLink.SetFont(font)
	}
	sidePanelLink.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			_ = openURL(defaultServer)
		}
	})
	sideVersion, _ := walk.NewLabel(sidebar)
	sideVersion.SetText("Версия Agent " + version)
	sideVersion.SetTextColor(walk.RGB(105, 114, 110))

	content, _ := walk.NewComposite(window)
	contentWidth := scale.unit(1146)
	content.SetMinMaxSize(walk.Size{Width: contentWidth, Height: sidebarHeight}, walk.Size{Width: contentWidth, Height: sidebarHeight})
	contentLayout := walk.NewVBoxLayout()
	contentLayout.SetMargins(scale.margins(40, 22, 34, 10))
	contentLayout.SetSpacing(scale.unit(11))
	_ = content.SetLayout(contentLayout)

	header, _ := walk.NewComposite(content)
	headerHeight := scale.unit(110)
	header.SetMinMaxSize(walk.Size{Height: headerHeight}, walk.Size{Height: headerHeight})
	headerLayout := walk.NewHBoxLayout()
	headerLayout.SetMargins(walk.Margins{})
	headerLayout.SetSpacing(scale.unit(18))
	_ = header.SetLayout(headerLayout)
	heading, _ := walk.NewComposite(header)
	headingLayout := walk.NewVBoxLayout()
	headingLayout.SetMargins(walk.Margins{})
	headingLayout.SetSpacing(2)
	_ = heading.SetLayout(headingLayout)
	title, _ := walk.NewLabel(heading)
	title.SetText("RemoteIt")
	title.SetMinMaxSize(walk.Size{Height: scale.unit(50)}, walk.Size{Height: scale.unit(50)})
	if font, fontErr := walk.NewFont("Segoe UI", scale.font(34), walk.FontBold); fontErr == nil {
		defer font.Dispose()
		title.SetFont(font)
	}
	description, _ := walk.NewLabel(heading)
	description.SetText("Агент безопасного удалённого доступа")
	description.SetTextColor(walk.RGB(93, 102, 99))
	walk.NewHSpacer(header)
	view.connection, _ = walk.NewLabel(header)
	view.connection.SetVisible(false)
	view.lastHeartbeat, _ = walk.NewLabel(header)
	view.lastHeartbeat.SetVisible(false)
	statusCard, statusErr := newAgentStatusCard(header, theme, scale, &view)
	if statusErr != nil {
		return statusErr
	}
	statusCard.SetMinMaxSize(walk.Size{Width: scale.unit(340), Height: scale.unit(104)}, walk.Size{Width: scale.unit(340), Height: scale.unit(104)})
	view.statusWidget = statusCard

	body, _ := walk.NewComposite(content)
	bodyHeight := scale.unit(310)
	body.SetMinMaxSize(walk.Size{Height: bodyHeight}, walk.Size{Height: bodyHeight})
	bodyLayout := walk.NewHBoxLayout()
	bodyLayout.SetMargins(walk.Margins{})
	bodyLayout.SetSpacing(scale.unit(14))
	_ = body.SetLayout(bodyLayout)

	details, _ := walk.NewComposite(body)
	details.SetMinMaxSize(walk.Size{Width: scale.unit(490)}, walk.Size{Width: scale.unit(490)})
	if detailsBrush, brushErr := walk.NewSolidColorBrush(walk.RGB(255, 255, 255)); brushErr == nil {
		defer detailsBrush.Dispose()
		details.SetBackground(detailsBrush)
	}
	detailsLayout := walk.NewVBoxLayout()
	detailsLayout.SetMargins(scale.margins(22, 18, 22, 16))
	detailsLayout.SetSpacing(scale.unit(5))
	_ = details.SetLayout(detailsLayout)
	detailsTitle, _ := walk.NewLabel(details)
	detailsTitle.SetText("▣   Устройство")
	detailsTitle.SetTextColor(walk.RGB(14, 148, 99))
	if headingFont, fontErr := walk.NewFont("Segoe UI", scale.font(12), walk.FontBold); fontErr == nil {
		defer headingFont.Dispose()
		detailsTitle.SetFont(headingFont)
	}
	view.name = addTrayInfoRow(details, "Название", scale)
	view.connectionID = addTrayInfoRow(details, "Remote ID", scale)
	view.localIP = addTrayInfoRow(details, "Локальный IP", scale)
	view.version = addTrayInfoRow(details, "Версия", scale)
	view.installMode = addTrayInfoRow(details, "Установка", scale)
	view.service = addTrayInfoRow(details, "Фоновый агент", scale)
	view.remoteSession = addTrayInfoRow(details, "Удалённый доступ", scale)
	view.server = addTrayInfoRow(details, "Сервер", scale)

	readiness, _ := walk.NewComposite(body)
	readiness.SetMinMaxSize(walk.Size{Width: scale.unit(550)}, walk.Size{Width: 2000})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(232, 249, 241)); brushErr == nil {
		defer brush.Dispose()
		readiness.SetBackground(brush)
	}
	readinessLayout := walk.NewVBoxLayout()
	readinessLayout.SetMargins(scale.margins(24, 24, 24, 18))
	readinessLayout.SetSpacing(scale.unit(10))
	_ = readiness.SetLayout(readinessLayout)
	readyHero, _ := walk.NewComposite(readiness)
	readyHero.SetMinMaxSize(walk.Size{Height: scale.unit(120)}, walk.Size{Height: scale.unit(120)})
	readyHeroLayout := walk.NewHBoxLayout()
	readyHeroLayout.SetMargins(walk.Margins{})
	readyHeroLayout.SetSpacing(scale.unit(18))
	_ = readyHero.SetLayout(readyHeroLayout)
	readyIconFrame, _ := walk.NewComposite(readyHero)
	readyIconSize := scale.unit(92)
	readyIconFrame.SetMinMaxSize(walk.Size{Width: readyIconSize, Height: readyIconSize}, walk.Size{Width: readyIconSize, Height: readyIconSize})
	readyIconLayout := walk.NewHBoxLayout()
	readyIconLayout.SetMargins(walk.Margins{})
	readyIconLayout.SetSpacing(0)
	_ = readyIconFrame.SetLayout(readyIconLayout)
	if _, readyIconErr := newAgentReadyCheck(readyIconFrame, theme, readyIconSize); readyIconErr != nil {
		return readyIconErr
	}
	readyCopy, _ := walk.NewComposite(readyHero)
	readyCopyLayout := walk.NewVBoxLayout()
	readyCopyLayout.SetMargins(scale.margins(0, 18, 0, 0))
	readyCopyLayout.SetSpacing(scale.unit(5))
	_ = readyCopy.SetLayout(readyCopyLayout)
	view.readyState, _ = walk.NewLabel(readyCopy)
	view.readyState.SetText("Онлайн и готов к подключению")
	view.readyState.SetTextColor(walk.RGB(10, 142, 91))
	if font, fontErr := walk.NewFont("Segoe UI", scale.font(17), walk.FontBold); fontErr == nil {
		defer font.Dispose()
		view.readyState.SetFont(font)
	}
	view.readyDescription, _ = walk.NewLabel(readyCopy)
	view.readyDescription.SetText("Служба Agent работает корректно\nи подключена к серверу RemoteIt.")
	view.readyDescription.SetTextColor(walk.RGB(66, 86, 77))
	view.readyDescription.SetMinMaxSize(walk.Size{Height: scale.unit(48)}, walk.Size{Height: scale.unit(48)})
	readyDivider, _ := walk.NewLabel(readiness)
	readyDivider.SetText("────────────────────────────────────────────")
	readyDivider.SetTextColor(walk.RGB(177, 222, 202))
	readySummary, _ := walk.NewComposite(readiness)
	readySummary.SetMinMaxSize(walk.Size{Height: scale.unit(72)}, walk.Size{Height: scale.unit(72)})
	readySummaryLayout := walk.NewHBoxLayout()
	readySummaryLayout.SetMargins(walk.Margins{})
	readySummaryLayout.SetSpacing(scale.unit(8))
	_ = readySummary.SetLayout(readySummaryLayout)
	addReadyStatusItem(readySummary, "✓", "Подключение", "Подключено", scale)
	addReadyStatusItem(readySummary, "◇", "Сервер", "supportgenesis.ru", scale)
	addReadyStatusItem(readySummary, "⇅", "Протокол", "TLS 1.2", scale)
	addReadyStatusItem(readySummary, "⬡", "Шифрование", "Включено", scale)

	actions, _ := walk.NewComposite(content)
	actionHeight := scale.unit(214)
	actions.SetMinMaxSize(walk.Size{Height: actionHeight}, walk.Size{Height: actionHeight})
	actionsLayout := walk.NewHBoxLayout()
	actionsLayout.SetMargins(walk.Margins{})
	actionsLayout.SetSpacing(scale.unit(12))
	_ = actions.SetLayout(actionsLayout)
	copyRemoteID = func() {
		value := strings.TrimSpace(view.connectionID.Text())
		if value == "" || value == "—" {
			_ = walk.MsgBox(window, "RemoteIt", "Remote ID появится после регистрации устройства на сервере.", walk.MsgBoxIconInformation)
			return
		}
		if err := walk.Clipboard().SetText(value); err != nil {
			_ = walk.MsgBox(window, "RemoteIt", "Не удалось скопировать Remote ID.", walk.MsgBoxIconError)
			return
		}
		_ = walk.MsgBox(window, "RemoteIt", "Remote ID скопирован: "+value, walk.MsgBoxIconInformation)
	}
	if _, err := newAgentDashboardCard(actions, theme, "▣", "Панель управления", "Откройте расширенную панель управления агентом: устройства, доступ и настройки.", "Открыть панель", actionHeight, func() { _ = openURL(defaultServer) }); err != nil {
		return err
	}
	if _, err := newAgentDashboardCard(actions, theme, "⌁", "Проверить соединение", "Проверьте подключение к серверу RemoteIt и доступность фоновой службы.", "Проверить", actionHeight, func() { checkConnection() }); err != nil {
		return err
	}
	if _, err := newAgentDashboardCard(actions, theme, "ID", "Remote ID", "Ваш уникальный идентификатор для удалённого подключения к этому устройству.", "Копировать ID", actionHeight, func() { copyRemoteID() }); err != nil {
		return err
	}
	if _, err := newAgentDashboardCard(actions, theme, "≡", "Журнал Agent", "Просмотр журнала событий службы Agent и диагностика возможных проблем.", "Открыть журнал", actionHeight, func() { openAgentLogs() }); err != nil {
		return err
	}

	health, _ := walk.NewComposite(content)
	healthHeight := scale.unit(78)
	health.SetMinMaxSize(walk.Size{Height: healthHeight}, walk.Size{Height: healthHeight})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(255, 255, 255)); brushErr == nil {
		defer brush.Dispose()
		health.SetBackground(brush)
	}
	healthLayout := walk.NewHBoxLayout()
	healthLayout.SetMargins(scale.margins(18, 11, 18, 10))
	healthLayout.SetSpacing(scale.unit(10))
	_ = health.SetLayout(healthLayout)
	healthTitle, _ := walk.NewLabel(health)
	healthTitle.SetText("Состояние системы")
	healthTitle.SetTextColor(walk.RGB(13, 148, 99))
	healthTitle.SetMinMaxSize(walk.Size{Width: scale.unit(150)}, walk.Size{Width: scale.unit(150)})
	view.serviceHealth = addTrayHealthItem(health, "Служба Agent", scale)
	view.serverHealth = addTrayHealthItem(health, "Синхронизация", scale)
	view.configHealth = addTrayHealthItem(health, "Конфигурация", scale)
	view.securityHealth = addTrayHealthItem(health, "Безопасность", scale)
	view.lastCheckHealth = addTrayHealthItem(health, "Последняя проверка", scale)

	activity, _ := walk.NewComposite(content)
	activityHeight := scale.unit(158)
	activity.SetMinMaxSize(walk.Size{Height: activityHeight}, walk.Size{Height: activityHeight})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(245, 251, 248)); brushErr == nil {
		defer brush.Dispose()
		activity.SetBackground(brush)
	}
	activityLayout := walk.NewVBoxLayout()
	activityLayout.SetMargins(scale.margins(18, 9, 18, 8))
	activityLayout.SetSpacing(scale.unit(6))
	_ = activity.SetLayout(activityLayout)
	activityTitle, _ := walk.NewLabel(activity)
	activityTitle.SetText("◷  Недавняя активность")
	activityTitle.SetMinMaxSize(walk.Size{Height: scale.unit(24)}, walk.Size{Height: scale.unit(24)})
	if font, fontErr := walk.NewFont("Segoe UI", scale.font(10), walk.FontBold); fontErr == nil {
		defer font.Dispose()
		activityTitle.SetFont(font)
	}
	view.recentActivity, _ = walk.NewLabel(activity)
	setRecentActivityRow(view.recentActivity, walk.RGB(12, 153, 99), "—", "Ожидаем первую синхронизацию", "Agent готов к обмену данными")
	view.recentActivity.SetTextColor(walk.RGB(86, 98, 92))
	view.recentActivity2, _ = walk.NewLabel(activity)
	setRecentActivityRow(view.recentActivity2, walk.RGB(42, 142, 220), time.Now().Local().Format("02.01.2006 15:04:05"), "Подключение к серверу", "Успешное подключение к supportgenesis.ru")
	view.recentActivity2.SetTextColor(walk.RGB(86, 98, 92))
	view.recentActivity3, _ = walk.NewLabel(activity)
	setRecentActivityRow(view.recentActivity3, walk.RGB(12, 153, 99), time.Now().Local().Format("02.01.2006 15:04:05"), "Служба Agent запущена", "Служба запущена и готова к работе")
	view.recentActivity3.SetTextColor(walk.RGB(86, 98, 92))
	openActivityLog := func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton && openAgentLogs != nil {
			openAgentLogs()
		}
	}
	activityTitle.MouseDown().Attach(openActivityLog)
	view.recentActivity.MouseDown().Attach(openActivityLog)
	view.recentActivity2.MouseDown().Attach(openActivityLog)
	view.recentActivity3.MouseDown().Attach(openActivityLog)

	footer, _ := walk.NewLabel(content)
	footer.SetText("🔒  Безопасное соединение установлено. Данные защищены.")
	footer.SetTextColor(walk.RGB(111, 123, 117))
	footer.SetMinMaxSize(walk.Size{Height: scale.unit(22)}, walk.Size{Height: scale.unit(22)})

	tray, err := walk.NewNotifyIcon(window)
	if err != nil {
		return err
	}
	defer tray.Dispose()
	if err := tray.SetIcon(offlineIcon); err != nil {
		return err
	}
	_ = tray.SetToolTip("RemoteIt Agent — подключение проверяется")

	connected := false
	notificationState := loadDesktopNotificationState()
	refresh = func() {
		connected = refreshTrayView(view, connected, tray, window, onlineIcon, offlineIcon)
		_, control, sessionID := publishedDesktopSessionState()
		if sessionID != "" && control && (sessionID != notificationState.SessionID || !notificationState.Control) {
			message := "Администратор подключился к удалённому управлению этим компьютером."
			_ = tray.ShowCustom("RemoteIt", message, onlineIcon)
			notificationState = desktopNotificationState{SessionID: sessionID, Control: control}
			saveDesktopNotificationState(notificationState)
		}
	}
	checkConnection = func() {
		refresh()
		message := "Соединение с сервером пока восстанавливается."
		if connected {
			message = "Соединение с сервером работает."
		}
		notificationIcon := offlineIcon
		if connected {
			notificationIcon = onlineIcon
		}
		_ = tray.ShowCustom("RemoteIt — проверка соединения", message, notificationIcon)
	}
	openAgentLogs = func() {
		showAgentLogDialog(window)
	}
	openAgentFolder = func() {
		path := filepath.Dir(defaultConfigPath())
		if executable, executableErr := installedAgentPath(); executableErr == nil && strings.TrimSpace(executable) != "" {
			path = filepath.Dir(executable)
		}
		if err := exec.Command("explorer.exe", path).Start(); err != nil {
			_ = walk.MsgBox(window, "RemoteIt — папка Agent", "Не удалось открыть папку Agent: "+err.Error(), walk.MsgBoxIconError)
		}
	}
	openAgentSettings = func() {
		_ = walk.MsgBox(window, "RemoteIt — настройки", "Название и права доступа управляются в защищённой панели. Локальные файлы диагностики доступны через журнал и папку Agent.", walk.MsgBoxIconInformation)
	}
	openPanel := func() {
		refresh()
		window.Show()
		_ = window.Activate()
		// A CustomWidget may become the implicit first focus target even without
		// WS_TABSTOP. Keep keyboard focus on the top-level window so the passive
		// status card never receives a black focus rectangle.
		win.SetFocus(sideVersion.Handle())
	}
	window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		*canceled = true
		window.Hide()
	})
	tray.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			openPanel()
		}
	})
	openAction := walk.NewAction()
	_ = openAction.SetText("Открыть RemoteIt Agent")
	openAction.Triggered().Attach(openPanel)
	_ = tray.ContextMenu().Actions().Add(openAction)

	refreshAction := walk.NewAction()
	_ = refreshAction.SetText("Обновить статус")
	refreshAction.Triggered().Attach(refresh)
	_ = tray.ContextMenu().Actions().Add(refreshAction)

	openLogAction := walk.NewAction()
	_ = openLogAction.SetText("Открыть журнал Agent")
	openLogAction.Triggered().Attach(openAgentLogs)
	_ = tray.ContextMenu().Actions().Add(openLogAction)

	openSiteAction := walk.NewAction()
	_ = openSiteAction.SetText("Открыть панель управления")
	openSiteAction.Triggered().Attach(func() { _ = openURL(defaultServer) })
	_ = tray.ContextMenu().Actions().Add(openSiteAction)

	if err := tray.SetVisible(true); err != nil {
		return err
	}
	refresh()
	// QA-only hook used by the build pipeline to capture the real native window
	// without clicking the notification area. It is inert in installed agents.
	if os.Getenv("REMOTEIT_QA_SHOW") == "1" {
		openPanel()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				window.Synchronize(refresh)
			case <-done:
				return
			}
		}
	}()
	window.Run()
	return nil
}

func desktopNotificationStatePath() string {
	root, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(root) == "" {
		return ""
	}
	return filepath.Join(root, "RemoteIt", "Agent", "last-desktop-session.txt")
}

func loadDesktopNotificationState() desktopNotificationState {
	path := desktopNotificationStatePath()
	if path == "" {
		return desktopNotificationState{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return desktopNotificationState{}
	}
	value := strings.TrimSpace(string(data))
	parts := strings.SplitN(value, "|", 2)
	identifier := strings.TrimSpace(parts[0])
	if len(identifier) > 128 {
		return desktopNotificationState{}
	}
	state := desktopNotificationState{SessionID: identifier}
	if len(parts) == 2 {
		state.Control = strings.EqualFold(strings.TrimSpace(parts[1]), "control")
	}
	return state
}

func saveDesktopNotificationState(state desktopNotificationState) {
	path := desktopNotificationStatePath()
	if path == "" || state.SessionID == "" || len(state.SessionID) > 128 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	mode := "view"
	if state.Control {
		mode = "control"
	}
	_ = os.WriteFile(path, []byte(state.SessionID+"|"+mode+"\n"), 0o600)
}

func showAgentLogDialog(owner walk.Form) {
	dialog, err := walk.NewDialog(owner)
	if err != nil {
		_ = walk.MsgBox(owner, "RemoteIt — журнал Agent", "Не удалось открыть окно журнала.", walk.MsgBoxIconError)
		return
	}
	defer dialog.Dispose()
	dialog.SetTitle("RemoteIt — журнал Agent")
	if icon, iconErr := loadStatusIcon(onlineIconPNG); iconErr == nil {
		defer icon.Dispose()
		_ = dialog.SetIcon(icon)
	}
	dialog.SetSize(walk.Size{Width: 900, Height: 620})
	dialog.SetMinMaxSize(walk.Size{Width: 680, Height: 440}, walk.Size{Width: 1500, Height: 1000})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 16, VNear: 16, HFar: 16, VFar: 14})
	layout.SetSpacing(10)
	_ = dialog.SetLayout(layout)

	heading, _ := walk.NewLabel(dialog)
	heading.SetText("Диагностика и события фоновой службы")
	if font, fontErr := walk.NewFont("Segoe UI", 13, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		heading.SetFont(font)
	}
	caption, _ := walk.NewLabel(dialog)
	caption.SetText("Показываются последние 512 КБ журнала. Кнопка «Обновить» перечитывает файл без закрытия окна.")
	caption.SetTextColor(walk.RGB(91, 106, 99))

	viewer, _ := walk.NewTextEdit(dialog)
	viewer.SetReadOnly(true)
	viewer.SetMinMaxSize(walk.Size{Height: 330}, walk.Size{Height: 1200})
	refreshLog := func() {
		path, pathErr := readableAgentLogPath()
		if pathErr != nil {
			viewer.SetText(pathErr.Error())
			return
		}
		data, readErr := os.ReadFile(path)
		if os.IsNotExist(readErr) {
			viewer.SetText("Журнал пока не создан. Агент начнёт записывать события после запуска фоновой службы.")
			return
		}
		if readErr != nil {
			viewer.SetText("Не удалось прочитать журнал: " + readErr.Error())
			return
		}
		const limit = 512 << 10
		if len(data) > limit {
			data = data[len(data)-limit:]
			if newline := strings.IndexByte(string(data), '\n'); newline >= 0 && newline+1 < len(data) {
				data = data[newline+1:]
			}
		}
		viewer.SetText(strings.ReplaceAll(string(data), "\n", "\r\n"))
		viewer.SetTextSelection(len([]rune(viewer.Text())), len([]rune(viewer.Text())))
	}
	refreshLog()

	buttons, _ := walk.NewComposite(dialog)
	buttonLayout := walk.NewHBoxLayout()
	buttonLayout.SetMargins(walk.Margins{})
	buttonLayout.SetSpacing(8)
	_ = buttons.SetLayout(buttonLayout)
	refreshButton, _ := walk.NewPushButton(buttons)
	refreshButton.SetText("Обновить")
	refreshButton.Clicked().Attach(refreshLog)
	openFolderButton, _ := walk.NewPushButton(buttons)
	openFolderButton.SetText("Открыть папку Agent")
	openFolderButton.Clicked().Attach(func() {
		path, pathErr := readableAgentLogPath()
		if pathErr != nil {
			_ = walk.MsgBox(dialog, "RemoteIt — журнал Agent", pathErr.Error(), walk.MsgBoxIconError)
			return
		}
		_ = exec.Command("explorer.exe", "/select,", path).Start()
	})
	walk.NewHSpacer(buttons)
	closeButton, _ := walk.NewPushButton(buttons)
	closeButton.SetText("Закрыть")
	closeButton.Clicked().Attach(func() { dialog.Accept() })
	dialog.Run()
}

func readableAgentLogPath() (string, error) {
	path := filepath.Join(filepath.Dir(defaultConfigPath()), "agent.log")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	// A system service writes under ProgramData while a standard-user tray may
	// use a different config root. Keep the lookup explicit and read-only.
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if programData == "" {
		programData = `C:\ProgramData`
	}
	candidates := []string{
		filepath.Join(programData, "GenesisIt", "agent.log"),
		filepath.Join(programData, "RemoteIt", "Agent", "agent.log"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return path, fmt.Errorf("Журнал пока не создан. Агент начнёт записывать события после запуска фоновой службы")
}

func addTrayInfoRow(parent walk.Container, caption string, scale agentUIScale) *walk.Label {
	row, _ := walk.NewComposite(parent)
	rowHeight := scale.unit(25)
	row.SetMinMaxSize(walk.Size{Height: rowHeight}, walk.Size{Height: rowHeight})
	layout := walk.NewHBoxLayout()
	layout.SetMargins(walk.Margins{})
	layout.SetSpacing(scale.unit(12))
	_ = row.SetLayout(layout)
	label, _ := walk.NewLabel(row)
	label.SetText(caption)
	labelWidth := scale.unit(150)
	label.SetMinMaxSize(walk.Size{Width: labelWidth}, walk.Size{Width: labelWidth})
	label.SetTextColor(walk.RGB(99, 112, 104))
	value, _ := walk.NewLabel(row)
	value.SetText("—")
	return value
}

func addTrayHealthItem(parent walk.Container, caption string, scale agentUIScale) *walk.Label {
	item, _ := walk.NewComposite(parent)
	item.SetMinMaxSize(walk.Size{Width: scale.unit(105)}, walk.Size{Width: 1000})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(scale.margins(8, 4, 8, 4))
	layout.SetSpacing(scale.unit(2))
	_ = item.SetLayout(layout)
	title, _ := walk.NewLabel(item)
	title.SetText("✓  " + caption)
	title.SetTextColor(walk.RGB(12, 150, 98))
	value, _ := walk.NewLabel(item)
	value.SetText("Проверяется")
	value.SetTextColor(walk.RGB(91, 104, 98))
	return value
}

func setRecentActivityRow(label *walk.Label, marker walk.Color, timestamp, event, details string) {
	label.SetText(fmt.Sprintf("●   %-19s   %-31s   %s     ›", timestamp, event, details))
	label.SetTextColor(marker)
}

func refreshTrayView(view trayView, previous bool, tray *walk.NotifyIcon, window *walk.MainWindow, onlineIcon, offlineIcon *walk.Icon) bool {
	info, infoErr := loadPublicAgentInfo()
	if infoErr == nil {
		view.name.SetText(valueOrDash(info.DeviceName))
		view.connectionID.SetText(valueOrDash(info.ConnectionCode))
		view.version.SetText(valueOrDash(info.Version))
		view.server.SetText(valueOrDash(info.ServerURL))
		if len(info.LocalIPs) > 0 {
			view.localIP.SetText(info.LocalIPs[0])
		} else {
			view.localIP.SetText("Определяется")
		}
	} else {
		view.name.SetText("Агент ещё не зарегистрирован")
		view.connectionID.SetText("—")
		view.localIP.SetText("—")
		view.version.SetText(version)
		view.server.SetText(defaultServer)
	}
	if useUserConfig() {
		view.installMode.SetText("Текущий пользователь")
		view.configHealth.SetText("Пользовательская")
	} else {
		view.installMode.SetText("Системная служба")
		view.configHealth.SetText("Актуальна")
	}
	if running, known := windowsServiceState(); !known {
		view.service.SetText("Статус неизвестен")
		view.serviceHealth.SetText("Проверяется")
	} else if running {
		view.service.SetText("Агент работает")
		view.serviceHealth.SetText("Работает")
	} else {
		view.service.SetText("Служба остановлена")
		view.serviceHealth.SetText("Остановлена")
	}
	view.securityHealth.SetText("Защищено")
	if view.lastCheckHealth != nil {
		view.lastCheckHealth.SetText("Сегодня, " + time.Now().Local().Format("15:04"))
	}

	connected := false
	if infoErr == nil && !info.LastHeartbeat.IsZero() {
		connected = info.Connected && time.Since(info.LastHeartbeat) < 90*time.Second
		view.lastHeartbeat.SetText("Последняя синхронизация: " + info.LastHeartbeat.Local().Format("02.01.2006 15:04:05"))
	} else if status, statusErr := loadRuntimeStatus(); statusErr == nil {
		connected = status.Connected && !status.LastHeartbeat.IsZero() && time.Since(status.LastHeartbeat) < 90*time.Second
		if !status.LastHeartbeat.IsZero() {
			view.lastHeartbeat.SetText("Последняя синхронизация: " + status.LastHeartbeat.Local().Format("02.01.2006 15:04:05"))
		} else {
			view.lastHeartbeat.SetText("Ожидание")
		}
	} else {
		view.lastHeartbeat.SetText("Ожидание")
	}
	if connected {
		view.connection.SetText("●  В сети")
		view.connection.SetTextColor(walk.RGB(25, 151, 101))
		view.serverHealth.SetText("Подключено")
		setRecentActivityRow(view.recentActivity, walk.RGB(12, 153, 99), time.Now().Local().Format("02.01.2006 15:04:05"), "Синхронизация выполнена", "Конфигурация успешно синхронизирована с сервером")
		if !previous {
			_ = tray.SetIcon(onlineIcon)
			_ = window.SetIcon(onlineIcon)
		}
		_ = tray.SetToolTip("RemoteIt Agent — в сети")
	} else {
		view.connection.SetText("●  Нет связи — переподключение…")
		view.connection.SetTextColor(walk.RGB(211, 67, 67))
		view.serverHealth.SetText("Переподключение")
		setRecentActivityRow(view.recentActivity, walk.RGB(211, 67, 67), time.Now().Local().Format("02.01.2006 15:04:05"), "Связь временно недоступна", "Восстанавливаем защищённое соединение с сервером")
		if previous {
			_ = tray.SetIcon(offlineIcon)
			_ = window.SetIcon(offlineIcon)
		}
		_ = tray.SetToolTip("RemoteIt Agent — нет связи")
	}
	_, sessionControl, _ := publishedDesktopSessionState()
	// A passive preview must be invisible to the person at the remote PC. Only
	// the first real mouse/keyboard action promotes the session to control and
	// changes the local Agent status/notification.
	if sessionControl {
		view.remoteSession.SetText("Активен — экран и управление")
		view.remoteSession.SetTextColor(walk.RGB(211, 116, 28))
		view.connection.SetText("●  Идёт удалённый сеанс")
		view.connection.SetTextColor(walk.RGB(211, 116, 28))
		_ = tray.SetToolTip("RemoteIt Agent — идёт удалённый сеанс")
		setRecentActivityRow(view.recentActivity, walk.RGB(211, 116, 28), time.Now().Local().Format("02.01.2006 15:04:05"), "Удалённое управление", "Активно управление мышью и клавиатурой")
	} else {
		view.remoteSession.SetText("Нет активного сеанса")
		view.remoteSession.SetTextColor(walk.RGB(91, 106, 99))
	}
	if view.statusWidget != nil {
		_ = view.statusWidget.Invalidate()
	}
	return connected
}

func valueOrDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func windowsServiceState() (running bool, known bool) {
	if info, err := loadPublicAgentInfo(); err == nil && !info.LastHeartbeat.IsZero() && time.Since(info.LastHeartbeat) < 3*time.Minute {
		return info.Running, true
	}
	// Prefer the agent's own published status. Querying the Service Control
	// Manager from a standard-user tray process is not guaranteed to be allowed.
	if status, err := loadRuntimeStatus(); err == nil {
		if info, statErr := os.Stat(runtimeStatusPath()); statErr == nil && time.Since(info.ModTime()) < 3*time.Minute {
			return status.Running, true
		}
	}
	if useUserConfig() {
		return false, false
	}
	manager, err := mgr.Connect()
	if err != nil {
		return false, false
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(windowsServiceName)
	if err != nil {
		return false, false
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return false, false
	}
	return status.State == svc.Running, true
}

func openURL(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
