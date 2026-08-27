//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
)

// newAgentDashboardV3 implements the approved 1536x1024 RemoteIt Agent
// composition. UI chrome and text are native, while artwork and icons come
// from high-resolution, consistently stroked assets. The canvas receives each
// target rectangle only once, avoiding repeated DPI scaling at 100/125/150/200%.
func newAgentDashboardV3(parent walk.Container, scale agentUIScale, brandIcon, offlineBrandIcon *walk.Icon, snapshot func() agentDashboardSnapshot, actions []func()) (*walk.CustomWidget, error) {
	newFont := func(points float64, style walk.FontStyle) (*walk.Font, error) {
		return walk.NewFont("Segoe UI", scale.font(points), style)
	}
	titleFont, err := newFont(24, walk.FontBold)
	if err != nil {
		return nil, err
	}
	brandFont, err := newFont(16, walk.FontBold)
	if err != nil {
		titleFont.Dispose()
		return nil, err
	}
	statusFont, err := newFont(14, walk.FontBold)
	if err != nil {
		titleFont.Dispose()
		brandFont.Dispose()
		return nil, err
	}
	cardTitleFont, err := newFont(12, walk.FontBold)
	if err != nil {
		titleFont.Dispose()
		brandFont.Dispose()
		statusFont.Dispose()
		return nil, err
	}
	bodyFont, err := newFont(10, 0)
	if err != nil {
		titleFont.Dispose()
		brandFont.Dispose()
		statusFont.Dispose()
		cardTitleFont.Dispose()
		return nil, err
	}
	bodyBoldFont, err := newFont(10, walk.FontBold)
	if err != nil {
		titleFont.Dispose()
		brandFont.Dispose()
		statusFont.Dispose()
		cardTitleFont.Dispose()
		bodyFont.Dispose()
		return nil, err
	}
	metaFont, err := newFont(8.5, 0)
	if err != nil {
		titleFont.Dispose()
		brandFont.Dispose()
		statusFont.Dispose()
		cardTitleFont.Dispose()
		bodyFont.Dispose()
		bodyBoldFont.Dispose()
		return nil, err
	}
	metaBoldFont, err := newFont(8.5, walk.FontBold)
	if err != nil {
		titleFont.Dispose()
		brandFont.Dispose()
		statusFont.Dispose()
		cardTitleFont.Dispose()
		bodyFont.Dispose()
		bodyBoldFont.Dispose()
		metaFont.Dispose()
		return nil, err
	}
	monoFont, err := walk.NewFont("Consolas", scale.font(9), 0)
	if err != nil {
		for _, font := range []*walk.Font{titleFont, brandFont, statusFont, cardTitleFont, bodyFont, bodyBoldFont, metaFont, metaBoldFont} {
			font.Dispose()
		}
		return nil, err
	}
	fonts := []*walk.Font{titleFont, brandFont, statusFont, cardTitleFont, bodyFont, bodyBoldFont, metaFont, metaBoldFont, monoFont}
	deviceMonitor, err := loadEmbeddedBitmap(deviceMonitorPNG)
	if err != nil {
		for _, font := range fonts {
			font.Dispose()
		}
		return nil, err
	}
	iconSet, err := loadAgentIconSet()
	if err != nil {
		deviceMonitor.Dispose()
		for _, font := range fonts {
			font.Dispose()
		}
		return nil, err
	}

	var widget *walk.CustomWidget
	var hitTargets []agentDashboardAction
	activeScreen := 0
	hoveredTarget := -1
	logCache := []publicAgentEvent{}
	var logCacheAt time.Time
	widget, err = walk.NewCustomWidgetPixels(parent, 0, func(canvas *walk.Canvas, _ walk.Rectangle) error {
		bounds := widget.ClientBoundsPixels()
		if bounds.Width < 100 || bounds.Height < 100 {
			return nil
		}
		const designWidth = 1536.0
		const designHeight = 974.0
		renderScale := min(float64(bounds.Width)/designWidth, float64(bounds.Height)/designHeight)
		offsetX := (float64(bounds.Width) - designWidth*renderScale) / 2
		offsetY := (float64(bounds.Height) - designHeight*renderScale) / 2
		r := func(x, y, width, height int) walk.Rectangle {
			return walk.Rectangle{
				X:      int(offsetX + float64(x)*renderScale + 0.5),
				Y:      int(offsetY + float64(y)*renderScale + 0.5),
				Width:  max(1, int(float64(width)*renderScale+0.5)),
				Height: max(1, int(float64(height)*renderScale+0.5)),
			}
		}
		text := func(value string, font *walk.Font, color walk.Color, rect walk.Rectangle, format walk.DrawTextFormat) {
			_ = canvas.DrawTextPixels(value, font, color, rect, format|walk.TextNoPrefix)
		}
		fill := func(brush walk.Brush, rect walk.Rectangle, radius int) {
			radiusPixels := max(1, int(float64(radius)*renderScale+0.5))
			_ = canvas.FillRoundedRectanglePixels(brush, rect, walk.Size{Width: radiusPixels, Height: radiusPixels})
		}
		stroke := func(pen walk.Pen, rect walk.Rectangle, radius int) {
			radiusPixels := max(1, int(float64(radius)*renderScale+0.5))
			_ = canvas.DrawRoundedRectanglePixels(pen, rect, walk.Size{Width: radiusPixels, Height: radiusPixels})
		}

		ink := walk.RGB(18, 31, 55)
		muted := walk.RGB(94, 112, 139)
		green := walk.RGB(5, 163, 104)
		greenDark := walk.RGB(1, 132, 82)
		orange := walk.RGB(217, 119, 6)
		red := walk.RGB(204, 65, 65)
		white := walk.RGB(255, 255, 255)
		pageBrush, _ := walk.NewSolidColorBrush(walk.RGB(252, 253, 253))
		defer pageBrush.Dispose()
		sidebarBrush, _ := walk.NewSolidColorBrush(walk.RGB(254, 255, 255))
		defer sidebarBrush.Dispose()
		cardBrush, _ := walk.NewSolidColorBrush(white)
		defer cardBrush.Dispose()
		softBrush, _ := walk.NewSolidColorBrush(walk.RGB(232, 248, 240))
		defer softBrush.Dispose()
		softBrush2, _ := walk.NewSolidColorBrush(walk.RGB(242, 251, 247))
		defer softBrush2.Dispose()
		greenBrush, _ := walk.NewSolidColorBrush(green)
		defer greenBrush.Dispose()
		orangeBrush, _ := walk.NewSolidColorBrush(orange)
		defer orangeBrush.Dispose()
		redBrush, _ := walk.NewSolidColorBrush(red)
		defer redBrush.Dispose()
		redSoftBrush, _ := walk.NewSolidColorBrush(walk.RGB(255, 242, 242))
		defer redSoftBrush.Dispose()
		orangeSoftBrush, _ := walk.NewSolidColorBrush(walk.RGB(255, 247, 237))
		defer orangeSoftBrush.Dispose()
		shadowBrush, _ := walk.NewSolidColorBrush(walk.RGB(238, 242, 244))
		defer shadowBrush.Dispose()
		linePen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(225, 231, 235))
		defer linePen.Dispose()
		greenPen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(182, 227, 207))
		defer greenPen.Dispose()
		orangePen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(245, 199, 143))
		defer orangePen.Dispose()
		redPen, _ := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(237, 194, 194))
		defer redPen.Dispose()
		greenHoverBrush, _ := walk.NewSolidColorBrush(walk.RGB(218, 244, 232))
		defer greenHoverBrush.Dispose()
		greenHoverStrongBrush, _ := walk.NewSolidColorBrush(walk.RGB(1, 145, 89))
		defer greenHoverStrongBrush.Dispose()
		surface := func(rect walk.Rectangle, brush walk.Brush, pen walk.Pen, radius int) {
			shadow := rect
			shadow.X += max(1, int(2*renderScale+0.5))
			shadow.Y += max(2, int(5*renderScale+0.5))
			shadow.Width -= max(2, int(4*renderScale+0.5))
			shadow.Height -= max(1, int(2*renderScale+0.5))
			fill(shadowBrush, shadow, radius+2)
			fill(brush, rect, radius)
			stroke(pen, rect, radius)
		}
		drawIcon := func(kind string, box walk.Rectangle, color walk.Color, _ bool) {
			if kind == "id" {
				text("ID", bodyBoldFont, color, box, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
				return
			}
			colorName := "green"
			switch color {
			case ink:
				colorName = "ink"
			case muted:
				colorName = "muted"
			case white:
				colorName = "white"
			case orange:
				colorName = "orange"
			case red:
				colorName = "red"
			}
			if kind == "config" {
				kind = "settings"
			}
			if bitmap := iconSet.BitmapSized(kind, colorName, box.Width, box.Height); bitmap != nil {
				_ = canvas.DrawImageStretchedPixels(bitmap, box)
			}
		}
		drawStatusDot := func(box walk.Rectangle, color walk.Color) {
			// Status dots use the same pre-rasterized, area-filtered atlas as the
			// remaining UI. A tiny GDI ellipse visibly stair-steps at 125/150%
			// Windows scaling, which made healthy/reconnecting/error states look
			// like rough squares in the native Agent window.
			drawIcon("dot", box, color, false)
		}
		hitTargets = hitTargets[:0]
		status := snapshot()
		statusColor := greenDark
		statusSurfaceBrush := walk.Brush(cardBrush)
		statusSoftBrush := walk.Brush(softBrush)
		statusBorderPen := walk.Pen(linePen)
		switch status.Severity {
		case agentStatusReconnecting:
			statusColor = orange
			statusSurfaceBrush = orangeSoftBrush
			statusSoftBrush = orangeSoftBrush
			statusBorderPen = orangePen
		case agentStatusCritical:
			statusColor = red
			statusSurfaceBrush = redSoftBrush
			statusSoftBrush = redSoftBrush
			statusBorderPen = redPen
		}
		addTarget := func(key int, bounds walk.Rectangle, run func()) {
			hitTargets = append(hitTargets, agentDashboardAction{Bounds: bounds, Key: key, Run: run})
		}
		goToScreen := func(screen int) func() {
			return func() {
				activeScreen = screen
				hoveredTarget = -1
				_ = widget.Invalidate()
			}
		}

		_ = canvas.FillRectanglePixels(pageBrush, bounds)
		_ = canvas.FillRectanglePixels(sidebarBrush, r(0, 0, 302, 974))
		_ = canvas.DrawLinePixels(linePen, walk.Point{X: r(302, 0, 1, 1).X, Y: r(0, 0, 1, 1).Y}, walk.Point{X: r(302, 974, 1, 1).X, Y: r(302, 974, 1, 1).Y})

		// Brand and sidebar navigation.
		fill(softBrush2, r(40, 38, 66, 66), 20)
		stroke(greenPen, r(40, 38, 66, 66), 20)
		_ = canvas.DrawImageStretchedPixels(brandIcon, r(50, 48, 46, 46))
		text("RemoteIt", brandFont, ink, r(122, 40, 145, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text("Агент безопасного\nудалённого доступа", metaFont, muted, r(122, 76, 150, 42), walk.TextLeft|walk.TextTop|walk.TextWordbreak)
		nav := []struct {
			icon   string
			label  string
			screen int
		}{
			{icon: "monitor", label: "Обзор", screen: 0},
			{icon: "panel", label: "Панель управления", screen: 1},
			{icon: "log", label: "Журнал Agent", screen: 2},
			{icon: "folder", label: "Папка Agent", screen: 3},
			{icon: "settings", label: "Настройки", screen: 4},
			{icon: "info", label: "О программе", screen: 5},
		}
		for index, item := range nav {
			y := 157 + index*74
			box := r(24, y, 256, 62)
			key := 100 + index
			active := item.screen == activeScreen
			iconColor := muted
			labelColor := muted
			labelFont := bodyFont
			if active {
				fill(softBrush, box, 13)
				fill(greenBrush, r(39, y+11, 40, 40), 10)
				iconColor = white
				labelColor = ink
				labelFont = bodyBoldFont
			} else if hoveredTarget == key {
				fill(softBrush2, box, 13)
			}
			if !active {
				drawIcon(item.icon, r(47, y+19, 24, 24), iconColor, false)
			} else {
				drawIcon(item.icon, r(48, y+20, 22, 22), iconColor, false)
			}
			text(item.label, labelFont, labelColor, r(90, y, 174, 62), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			addTarget(key, box, goToScreen(item.screen))
		}

		sideStatus := r(38, 766, 228, 123)
		surface(sideStatus, statusSurfaceBrush, statusBorderPen, 13)
		statusTitle := "Агент работает"
		statusBody := "Служба запущена и готова\nк безопасному подключению."
		if status.Severity == agentStatusReconnecting {
			statusTitle = "Связь восстанавливается"
			statusBody = "Agent работает локально\nи повторяет подключение."
		} else if status.Severity == agentStatusCritical {
			statusTitle = "Агент недоступен"
			statusBody = "Проверьте службу Agent\nи сетевое подключение."
		}
		drawStatusDot(r(55, 793, 13, 13), statusColor)
		text(statusTitle, bodyBoldFont, statusColor, r(79, 779, 170, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		text(statusBody, metaFont, muted, r(57, 823, 176, 43), walk.TextLeft|walk.TextTop|walk.TextWordbreak)
		text("Версия Agent "+status.Version, metaFont, muted, r(38, 916, 180, 24), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)

		// Header and live status card.
		pageTitles := []string{
			"Ваш доступ. Под вашим контролем.",
			"Панель управления",
			"Журнал Agent",
			"Папка Agent",
			"Настройки Agent",
			"О программе",
		}
		pageSubtitles := []string{
			"RemoteIt Agent обеспечивает безопасное и надёжное удалённое подключение.",
			"Локальное управление устройством, подключением и доступом в одном окне.",
			"События фоновой службы и диагностика соединения без выхода из Agent.",
			"Пути установки, конфигурации и журнала этого компьютера.",
			"Практичные параметры подключения, безопасности и обновлений.",
			"Версия, сервер и сведения о защищённом Agent RemoteIt.",
		}
		if activeScreen < 0 || activeScreen >= len(pageTitles) {
			activeScreen = 0
		}
		text(pageTitles[activeScreen], titleFont, ink, r(343, 37, 655, 52), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		connectionMode := r(1013, 51, 170, 46)
		fill(statusSoftBrush, connectionMode, 13)
		stroke(statusBorderPen, connectionMode, 13)
		drawIcon("update", r(1028, 63, 22, 22), statusColor, false)
		text("Автосвязь", metaBoldFont, statusColor, r(1060, 51, 103, 46), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text(pageSubtitles[activeScreen], bodyFont, muted, r(343, 92, 700, 30), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		liveCard := r(1208, 35, 274, 126)
		surface(liveCard, statusSurfaceBrush, statusBorderPen, 13)
		drawStatusDot(r(1233, 63, 15, 15), statusColor)
		connectionLabel := strings.TrimSpace(strings.TrimLeft(status.ConnectionText, "●• "))
		if connectionLabel == "" {
			connectionLabel = "Проверка связи"
		}
		text(connectionLabel, statusFont, statusColor, r(1262, 51, 188, 42), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		text("Последняя синхронизация", bodyFont, muted, r(1234, 98, 206, 22), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		heartbeat := strings.TrimSpace(strings.TrimPrefix(status.LastHeartbeat, "Последняя синхронизация:"))
		text(heartbeat, bodyFont, muted, r(1234, 121, 206, 22), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

		if activeScreen != 0 {
			runAction := func(index int) func() {
				if index < 0 || index >= len(actions) {
					return func() {}
				}
				return actions[index]
			}
			drawButton := func(key int, box walk.Rectangle, icon, label string, primary bool, run func()) {
				if primary {
					brush := greenBrush
					if hoveredTarget == key {
						brush = greenHoverStrongBrush
					}
					fill(brush, box, 11)
					// The icon and text use the already-scaled target rectangle directly.
					iconBox := walk.Rectangle{X: box.X + max(12, int(18*renderScale)), Y: box.Y + (box.Height-max(18, int(24*renderScale)))/2, Width: max(18, int(24*renderScale)), Height: max(18, int(24*renderScale))}
					drawIcon(icon, iconBox, white, false)
					text(label, bodyBoldFont, white, walk.Rectangle{X: iconBox.X + iconBox.Width + max(8, int(12*renderScale)), Y: box.Y, Width: box.Width - iconBox.Width - max(54, int(70*renderScale)), Height: box.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
					drawIcon("arrow", walk.Rectangle{X: box.X + box.Width - max(32, int(42*renderScale)), Y: box.Y + (box.Height-max(16, int(22*renderScale)))/2, Width: max(16, int(22*renderScale)), Height: max(16, int(22*renderScale))}, white, false)
				} else {
					brush := cardBrush
					if hoveredTarget == key {
						brush = softBrush2
					}
					fill(brush, box, 11)
					stroke(func() walk.Pen {
						if hoveredTarget == key {
							return greenPen
						}
						return linePen
					}(), box, 11)
					iconBox := walk.Rectangle{X: box.X + max(12, int(16*renderScale)), Y: box.Y + (box.Height-max(18, int(22*renderScale)))/2, Width: max(18, int(22*renderScale)), Height: max(18, int(22*renderScale))}
					drawIcon(icon, iconBox, green, false)
					text(label, bodyBoldFont, ink, walk.Rectangle{X: iconBox.X + iconBox.Width + max(8, int(11*renderScale)), Y: box.Y, Width: box.Width - iconBox.Width - max(45, int(61*renderScale)), Height: box.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				}
				addTarget(key, box, run)
			}
			drawStatusBadge := func(box walk.Rectangle, label string, severity agentConnectionSeverity) {
				brush := walk.Brush(softBrush)
				color := greenDark
				switch severity {
				case agentStatusReconnecting:
					brush, color = orangeSoftBrush, orange
				case agentStatusCritical:
					brush, color = redSoftBrush, red
				}
				fill(brush, box, 10)
				drawStatusDot(walk.Rectangle{X: box.X + max(7, int(9*renderScale)), Y: box.Y + (box.Height-max(12, int(15*renderScale)))/2, Width: max(12, int(15*renderScale)), Height: max(12, int(15*renderScale))}, color)
				text(label, metaBoldFont, color, walk.Rectangle{X: box.X + max(26, int(33*renderScale)), Y: box.Y, Width: box.Width - max(34, int(43*renderScale)), Height: box.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			}
			drawField := func(icon, label, value string, box walk.Rectangle) {
				surface(box, cardBrush, linePen, 13)
				fill(softBrush, walk.Rectangle{X: box.X + max(12, int(16*renderScale)), Y: box.Y + max(12, int(16*renderScale)), Width: max(32, int(42*renderScale)), Height: max(32, int(42*renderScale))}, 10)
				drawIcon(icon, walk.Rectangle{X: box.X + max(20, int(25*renderScale)), Y: box.Y + max(20, int(25*renderScale)), Width: max(17, int(23*renderScale)), Height: max(17, int(23*renderScale))}, green, false)
				text(label, metaFont, muted, walk.Rectangle{X: box.X + max(60, int(74*renderScale)), Y: box.Y + max(9, int(13*renderScale)), Width: box.Width - max(74, int(92*renderScale)), Height: max(23, int(29*renderScale))}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				text(value, bodyBoldFont, ink, walk.Rectangle{X: box.X + max(60, int(74*renderScale)), Y: box.Y + max(38, int(48*renderScale)), Width: box.Width - max(74, int(92*renderScale)), Height: max(28, int(35*renderScale))}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			}
			drawFeature := func(icon, title, description, state string, box walk.Rectangle) {
				fill(softBrush2, box, 12)
				stroke(linePen, box, 12)
				iconFrame := walk.Rectangle{X: box.X + max(14, int(18*renderScale)), Y: box.Y + (box.Height-max(42, int(54*renderScale)))/2, Width: max(42, int(54*renderScale)), Height: max(42, int(54*renderScale))}
				fill(softBrush, iconFrame, 12)
				drawIcon(icon, walk.Rectangle{X: iconFrame.X + max(9, int(12*renderScale)), Y: iconFrame.Y + max(9, int(12*renderScale)), Width: iconFrame.Width - max(18, int(24*renderScale)), Height: iconFrame.Height - max(18, int(24*renderScale))}, green, false)
				contentX := iconFrame.X + iconFrame.Width + max(12, int(16*renderScale))
				// At laptop DPI the old fixed 165px status reservation consumed almost
				// half of the card and forced useful titles into an ellipsis. Keep the
				// status compact and let the title use the remaining measured width.
				stateWidth := min(max(76, int(112*renderScale)), max(76, box.Width/3))
				stateGap := max(10, int(14*renderScale))
				titleWidth := max(96, box.Width-(contentX-box.X)-stateWidth-stateGap-max(12, int(18*renderScale)))
				text(title, bodyBoldFont, ink, walk.Rectangle{X: contentX, Y: box.Y + max(10, int(14*renderScale)), Width: titleWidth, Height: max(25, int(32*renderScale))}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				text(description, metaFont, muted, walk.Rectangle{X: contentX, Y: box.Y + max(37, int(47*renderScale)), Width: box.Width - (contentX - box.X) - max(18, int(24*renderScale)), Height: max(38, int(48*renderScale))}, walk.TextLeft|walk.TextTop|walk.TextWordbreak|walk.TextEndEllipsis)
				if state != "" {
					text(state, metaBoldFont, greenDark, walk.Rectangle{X: box.X + box.Width - stateWidth - max(12, int(18*renderScale)), Y: box.Y + max(10, int(14*renderScale)), Width: stateWidth, Height: max(25, int(32*renderScale))}, walk.TextRight|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				}
			}

			switch activeScreen {
			case 1:
				panel := r(343, 181, 1139, 651)
				surface(panel, cardBrush, linePen, 15)
				drawIcon("panel", r(371, 210, 34, 34), green, false)
				text("Локальная панель этого устройства", cardTitleFont, ink, r(420, 202, 480, 48), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				drawStatusBadge(r(1245, 208, 194, 42), func() string {
					if status.Connected {
						return "В сети и готов"
					}
					if status.Severity == agentStatusReconnecting {
						return "Переподключение"
					}
					return "Требует внимания"
				}(), status.Severity)
				text("Состояние, идентификатор и безопасные локальные действия собраны здесь. Веб-панель открывается только отдельной кнопкой.", bodyFont, muted, r(371, 256, 970, 36), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				ready := r(371, 306, 1035, 116)
				fill(softBrush2, ready, 13)
				stroke(greenPen, ready, 13)
				_ = canvas.FillEllipsePixels(cardBrush, r(393, 326, 76, 76))
				drawIcon("circle-check", r(411, 344, 40, 40), statusColor, false)
				readyTitle := "Agent подключён и готов к работе"
				readyState := "Соединение защищено"
				if status.Severity == agentStatusReconnecting {
					readyTitle = "Agent работает и восстанавливает соединение"
					readyState = "Повторная попытка"
				} else if status.Severity == agentStatusCritical {
					readyTitle = "Agent требует проверки службы или сети"
					readyState = "Нет соединения"
				}
				text(readyTitle, cardTitleFont, ink, r(492, 320, 620, 38), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				text("Фоновая служба продолжает работу независимо от открытого окна.", bodyFont, muted, r(492, 360, 660, 28), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				drawStatusBadge(r(1190, 340, 190, 42), readyState, status.Severity)
				drawField("monitor", "Устройство", status.Name, r(371, 446, 333, 106))
				drawField("id", "Remote ID", status.ConnectionID, r(722, 446, 333, 106))
				drawField("link", "Сервер", strings.TrimPrefix(status.Server, "https://"), r(1073, 446, 333, 106))
				drawButton(210, r(371, 711, 238, 66), "link", "Проверить связь", false, runAction(1))
				drawButton(211, r(627, 711, 238, 66), "copy", "Копировать ID", false, runAction(2))
				drawButton(212, r(883, 711, 523, 66), "panel", "Открыть полную веб-панель", true, runAction(0))
			case 2:
				panel := r(343, 181, 1139, 651)
				surface(panel, cardBrush, linePen, 15)
				drawIcon("log", r(371, 208, 34, 34), green, false)
				text("События фоновой службы", cardTitleFont, ink, r(420, 201, 430, 48), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				logPath := publicEventsPath()
				pathLabel := logPath
				text(pathLabel, metaFont, muted, r(371, 250, 1000, 28), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				if time.Since(logCacheAt) > 2*time.Second {
					logCacheAt = time.Now()
					logCache = []publicAgentEvent{}
					if events, readErr := loadPublicAgentEvents(); readErr == nil {
						logCache = events
					}
				}
				type journalEntry struct {
					icon, title, detail, at string
					severity                agentConnectionSeverity
				}
				journal := make([]journalEntry, 0, 5)
				for index := len(logCache) - 1; index >= 0 && len(journal) < 5; index-- {
					event := logCache[index]
					entry := journalEntry{icon: "log", title: event.Title, detail: event.Detail, at: event.At.Local().Format("02.01 15:04"), severity: agentStatusHealthy}
					switch event.Kind {
					case "link", "network":
						entry.icon = "link"
					case "update":
						entry.icon = "update"
					case "service":
						entry.icon = "service"
					case "settings", "identity":
						entry.icon = "settings"
					}
					switch event.Level {
					case "warning":
						entry.severity = agentStatusReconnecting
					case "error":
						entry.severity = agentStatusCritical
					}
					journal = append(journal, entry)
				}
				if len(journal) == 0 {
					journal = []journalEntry{
						{icon: "link", title: "Связь с сервером", detail: func() string {
							if status.Connected {
								return "Защищённое соединение с supportgenesis.ru установлено"
							}
							return "Agent автоматически повторяет подключение"
						}(), at: heartbeat, severity: func() agentConnectionSeverity {
							if status.Connected {
								return agentStatusHealthy
							}
							return agentStatusReconnecting
						}()},
						{icon: "service", title: "Фоновая служба", detail: status.Service, at: "Работает", severity: func() agentConnectionSeverity {
							if status.Service == "Агент работает" {
								return agentStatusHealthy
							}
							return agentStatusCritical
						}()},
						{icon: "update", title: "Автоматическое переподключение", detail: "Смена сети, IP или VPN отслеживается каждую секунду", at: "Включено", severity: agentStatusHealthy},
						{icon: "shield", title: "Проверка подписанных обновлений", detail: "Новая версия применяется только после проверки подписи и SHA-256", at: "Включено", severity: agentStatusHealthy},
					}
				}
				for index, entry := range journal {
					y := 292 + index*77
					box := r(371, y, 1035, 65)
					rowBrush, rowPen, iconColor := softBrush2, linePen, green
					if entry.severity == agentStatusReconnecting {
						rowBrush, rowPen, iconColor = orangeSoftBrush, orangePen, orange
					} else if entry.severity == agentStatusCritical {
						rowBrush, rowPen, iconColor = redSoftBrush, redPen, red
					}
					fill(rowBrush, box, 11)
					stroke(rowPen, box, 11)
					fill(cardBrush, r(389, y+10, 45, 45), 11)
					drawIcon(entry.icon, r(400, y+21, 23, 23), iconColor, false)
					text(entry.title, bodyBoldFont, ink, r(456, y+5, 340, 29), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
					text(entry.detail, metaFont, muted, r(456, y+32, 770, 24), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
					text(entry.at, metaBoldFont, func() walk.Color {
						switch entry.severity {
						case agentStatusReconnecting:
							return orange
						case agentStatusCritical:
							return red
						default:
							return greenDark
						}
					}(), r(1237, y+5, 145, 50), walk.TextRight|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				}
				drawButton(220, r(371, 711, 258, 66), "log", "Обновить журнал", false, func() { logCacheAt = time.Time{}; _ = widget.Invalidate() })
				drawButton(221, r(647, 711, 759, 66), "folder", "Открыть расположение журнала в проводнике", true, runAction(4))
			case 3:
				installPath := filepath.Dir(defaultConfigPath())
				if executable, executableErr := installedAgentPath(); executableErr == nil && strings.TrimSpace(executable) != "" {
					installPath = filepath.Dir(executable)
				}
				logPath, logErr := readableAgentLogPath()
				if logErr != nil {
					logPath = filepath.Join(filepath.Dir(defaultConfigPath()), "agent.log")
				}
				panel := r(343, 181, 1139, 651)
				surface(panel, cardBrush, linePen, 15)
				drawIcon("folder", r(371, 208, 34, 34), green, false)
				text("Файлы и расположения Agent", cardTitleFont, ink, r(420, 201, 460, 48), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				text("Все пути показаны только для чтения. Открытие папки не изменяет конфигурацию Agent.", bodyFont, muted, r(371, 250, 920, 32), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				paths := []struct{ icon, title, path string }{
					{icon: "folder", title: "Каталог установки", path: installPath},
					{icon: "settings", title: "Файл конфигурации", path: defaultConfigPath()},
					{icon: "log", title: "Журнал службы", path: logPath},
				}
				for index, item := range paths {
					y := 310 + index*126
					box := r(371, y, 1035, 102)
					key := 231 + index
					if hoveredTarget == key {
						fill(greenHoverBrush, box, 12)
						stroke(greenPen, box, 12)
					} else {
						fill(softBrush2, box, 12)
						stroke(linePen, box, 12)
					}
					fill(softBrush, r(391, y+21, 58, 58), 13)
					drawIcon(item.icon, r(407, y+37, 27, 27), green, false)
					text(item.title, bodyBoldFont, ink, r(475, y+14, 420, 32), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
					text(item.path, monoFont, muted, r(475, y+48, 845, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
					drawIcon("arrow", r(1362, y+39, 22, 24), ink, false)
					addTarget(key, box, runAction(4))
				}
				drawButton(230, r(371, 711, 1035, 66), "folder", "Открыть папку Agent в проводнике", true, runAction(4))
			case 4:
				panel := r(343, 181, 1139, 651)
				surface(panel, cardBrush, linePen, 15)
				drawIcon("settings", r(371, 208, 34, 34), green, false)
				text("Рабочие параметры", cardTitleFont, ink, r(420, 201, 430, 48), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				text("Критические права и название устройства меняются только авторизованным администратором в панели.", bodyFont, muted, r(371, 250, 1000, 32), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				drawFeature("link", "Автоматическое переподключение", "Agent сам восстанавливает связь после смены сети или VPN.", "Включено", r(371, 310, 498, 112))
				drawFeature("shield", "Безопасный системный режим", "Служба запускается с Windows и использует защищённый канал.", "Защищено", r(888, 310, 518, 112))
				drawFeature("clock", "Автоматические обновления", "Новая проверенная версия устанавливается через служебный механизм.", "Включено", r(371, 446, 498, 112))
				drawFeature("info", "Уведомления о доступе", "Предпросмотр бесшумен; уведомление появляется при управлении.", "По политике", r(888, 446, 518, 112))
				text("Эти параметры защищены политикой администратора и не могут быть отключены на удалённом компьютере.", metaFont, muted, r(371, 584, 1035, 30), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				drawButton(240, r(371, 711, 238, 66), "link", "Проверить связь", false, runAction(1))
				drawButton(241, r(627, 711, 260, 66), "update", "Проверить версию", false, runAction(7))
				drawButton(242, r(905, 711, 501, 66), "panel", "Открыть защищённые настройки", true, runAction(0))
			case 5:
				panel := r(343, 181, 1139, 651)
				surface(panel, cardBrush, linePen, 15)
				fill(softBrush2, r(391, 233, 180, 180), 42)
				stroke(greenPen, r(391, 233, 180, 180), 42)
				_ = canvas.DrawImageStretchedPixels(brandIcon, r(432, 274, 98, 98))
				text("RemoteIt Agent", titleFont, ink, r(616, 238, 520, 52), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				text("Защищённый агент удалённого доступа для Windows.", bodyFont, muted, r(616, 294, 620, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
				drawStatusBadge(r(616, 347, 220, 42), "Версия "+status.Version, agentStatusHealthy)
				aboutRows := [][2]string{{"Сервер", status.Server}, {"Установка", status.InstallMode}, {"Фоновая служба", status.Service}, {"Шифрование", "TLS · защищённый канал"}, {"Создатель", "@Sanchcz"}}
				for index, item := range aboutRows {
					y := 446 + index*48
					text(item[0], bodyFont, muted, r(391, y, 210, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
					valueColor := ink
					if item[0] == "Создатель" {
						valueColor = greenDark
						creatorLink := r(603, y-2, 190, 38)
						if hoveredTarget == 251 {
							fill(softBrush, creatorLink, 9)
						}
						addTarget(251, creatorLink, runAction(8))
					}
					text(item[1], bodyBoldFont, valueColor, r(616, y, 720, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				}
				drawButton(250, r(371, 722, 1035, 66), "panel", "Открыть панель RemoteIt", true, runAction(0))
			}
			return nil
		}

		// Device card and illustration.
		deviceCard := r(343, 181, 645, 395)
		surface(deviceCard, cardBrush, linePen, 15)
		drawIcon("monitor", r(367, 204, 28, 28), green, false)
		text("Устройство", cardTitleFont, ink, r(405, 198, 240, 38), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		// The monitor is a dedicated high-resolution transparent illustration.
		// It is scaled once into its target box; text and all UI chrome remain native.
		_ = canvas.DrawImageStretchedPixels(deviceMonitor, r(365, 270, 185, 238))
		if status.Severity == agentStatusCritical && offlineBrandIcon != nil {
			fill(cardBrush, r(438, 359, 54, 54), 14)
			_ = canvas.DrawImageStretchedPixels(offlineBrandIcon, r(443, 364, 44, 44))
		}
		rows := [][2]string{{"Название", status.Name}, {"Remote ID", status.ConnectionID}, {"Локальный IP", status.LocalIP}, {"Версия", status.Version}, {"Установка", status.InstallMode}, {"Фоновый агент", status.Service}, {"Удалённый доступ", status.RemoteSession}, {"Сервер", status.Server}}
		for index, row := range rows {
			y := 252 + index*35
			text(row[0], bodyFont, muted, r(598, y, 160, 27), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
			// Values deliberately use the stronger role from the approved render:
			// labels recede, while the actual device data remains immediately scannable.
			text(row[1], bodyBoldFont, ink, r(786, y, 177, 27), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		}
		addTarget(270, r(770, 276, 200, 54), func() {
			if len(actions) > 2 {
				actions[2]()
			}
		})

		// Quick actions.
		quickCard := r(1012, 181, 470, 395)
		surface(quickCard, cardBrush, linePen, 15)
		drawIcon("bolt", r(1034, 202, 29, 29), ink, false)
		text("Быстрые действия", cardTitleFont, ink, r(1074, 198, 260, 38), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		quick := []struct {
			icon, title, caption string
			action               int
		}{
			{icon: "pencil", title: "Открыть панель управления", caption: "Устройства, доступ и настройки", action: 0},
			{icon: "link", title: "Проверить соединение", caption: "Связь с сервером RemoteIt", action: 1},
			{icon: "id", title: "Скопировать Remote ID", caption: "Идентификатор этого компьютера", action: 2},
			{icon: "list", title: "Открыть журнал Agent", caption: "Диагностика и события службы", action: 3},
		}
		for index, item := range quick {
			y := 246 + index*80
			box := r(1036, y, 422, 68)
			key := 300 + index
			if hoveredTarget == key {
				fill(softBrush2, box, 12)
				stroke(greenPen, box, 12)
			} else {
				fill(cardBrush, box, 12)
				stroke(linePen, box, 12)
			}
			fill(softBrush, r(1050, y+12, 46, 44), 11)
			drawIcon(item.icon, r(1061, y+22, 24, 24), green, false)
			text(item.title, bodyBoldFont, ink, r(1122, y+7, 292, 29), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			text(item.caption, bodyFont, muted, r(1122, y+35, 292, 24), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			drawIcon("arrow", r(1423, y+22, 22, 24), ink, false)
			if item.action < len(actions) {
				run := actions[item.action]
				if index == 0 {
					run = goToScreen(1)
				} else if index == 3 {
					run = goToScreen(2)
				}
				addTarget(key, box, run)
			}
		}

		// Diagnostics and system state.
		diagnosticsCard := r(343, 598, 645, 234)
		surface(diagnosticsCard, cardBrush, linePen, 15)
		drawIcon("pulse", r(365, 616, 29, 29), green, false)
		text("Диагностика и состояние", cardTitleFont, ink, r(400, 613, 300, 38), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		metrics := []struct {
			icon, label, value string
			color              walk.Color
		}{
			{icon: "service", label: "Служба агента", value: status.Service, color: func() walk.Color {
				if status.Service == "Агент работает" {
					return green
				}
				if status.Severity == agentStatusReconnecting {
					return orange
				}
				return red
			}()},
			{icon: "link", label: "Связь с сервером", value: func() string {
				if status.Connected {
					return "Подключено"
				}
				if status.Severity == agentStatusReconnecting {
					return "Переподключение"
				}
				return "Нет связи"
			}(), color: statusColor},
			{icon: "config", label: "Конфигурация", value: "Актуальна", color: green},
			{icon: "shield", label: "Безопасность", value: "Защищено", color: green},
			{icon: "cpu", label: "Система", value: "В норме", color: green},
		}
		for index, item := range metrics {
			x := 365 + index*120
			drawIcon(item.icon, r(x+43, 663, 34, 34), item.color, false)
			text(item.label, metaFont, ink, r(x, 706, 120, 22), walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			text(item.value, metaFont, muted, r(x, 731, 120, 21), walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		}
		notice := r(364, 765, 603, 46)
		fill(statusSoftBrush, notice, 9)
		drawIcon("shield", r(376, 776, 22, 22), statusColor, false)
		noticeText := "Все системы работают нормально. Устройство готово к удалённым подключениям."
		if status.Severity == agentStatusReconnecting {
			noticeText = "Agent работает. Подключение будет восстановлено автоматически после смены сети."
		} else if status.Severity == agentStatusCritical {
			noticeText = "Agent требует проверки: служба остановлена либо соединение недоступно."
		}
		text(noticeText, metaFont, statusColor, r(406, 765, 548, 46), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

		// Recent activity.
		activityCard := r(1012, 598, 470, 234)
		surface(activityCard, cardBrush, linePen, 15)
		drawIcon("clock", r(1034, 616, 29, 29), ink, false)
		text("Недавняя активность", cardTitleFont, ink, r(1074, 613, 300, 38), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		events := []struct {
			label string
			at    string
		}{
			{label: "Синхронизация выполнена", at: heartbeat},
			{label: "Связь с сервером подтверждена", at: heartbeat},
			{label: "Автопереподключение активно", at: "Постоянно"},
		}
		for index, item := range events {
			y := 657 + index*40
			eventColor := green
			if index < 2 {
				eventColor = statusColor
			}
			drawIcon("circle-check", r(1038, y+6, 21, 21), eventColor, false)
			text(item.label, metaFont, muted, r(1074, y, 250, 34), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			text(item.at, metaFont, muted, r(1321, y, 139, 34), walk.TextRight|walk.TextVCenter|walk.TextSingleLine)
		}
		_ = canvas.DrawLinePixels(linePen, walk.Point{X: r(1034, 786, 1, 1).X, Y: r(1034, 786, 1, 1).Y}, walk.Point{X: r(1459, 786, 1, 1).X, Y: r(1459, 786, 1, 1).Y})
		text("Открыть журнал Agent", bodyFont, greenDark, r(1038, 790, 250, 32), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		drawIcon("arrow", r(1424, 794, 24, 24), ink, false)
		activityLink := r(1034, 786, 425, 40)
		if hoveredTarget == 310 {
			fill(softBrush2, activityLink, 8)
		}
		if len(actions) > 3 {
			addTarget(310, activityLink, goToScreen(2))
		}

		// Bottom readiness and primary call to action.
		bottom := r(343, 856, 1139, 94)
		fill(softBrush2, bottom, 14)
		stroke(greenPen, bottom, 14)
		_ = canvas.FillEllipsePixels(cardBrush, r(359, 866, 74, 74))
		drawIcon("shield", r(374, 882, 43, 43), statusColor, true)
		bottomTitle := "Готово к работе"
		bottomBody := "Устройство в сети и защищено. Управляйте доступом\nи подключениями в панели управления."
		if status.Severity == agentStatusReconnecting {
			bottomTitle = "Восстанавливаем соединение"
			bottomBody = "Служба работает; повторное подключение выполняется\nавтоматически после смены сети или VPN."
		} else if status.Severity == agentStatusCritical {
			bottomTitle = "Требуется проверка"
			bottomBody = "Проверьте состояние службы Agent и сетевого доступа\nк серверу RemoteIt."
		}
		text(bottomTitle, cardTitleFont, ink, r(452, 871, 280, 31), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		text(bottomBody, metaFont, muted, r(452, 902, 485, 37), walk.TextLeft|walk.TextTop|walk.TextWordbreak)
		cta := r(1046, 871, 419, 62)
		if hoveredTarget == 311 {
			fill(greenHoverStrongBrush, cta, 11)
		} else {
			fill(greenBrush, cta, 11)
		}
		drawIcon("monitor", r(1093, 888, 28, 28), white, false)
		text("Открыть панель управления", cardTitleFont, white, r(1135, 871, 270, 62), walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		drawIcon("arrow", r(1413, 890, 24, 24), white, false)
		if len(actions) > 0 {
			addTarget(311, cta, goToScreen(1))
		}
		return nil
	})
	if err != nil {
		iconSet.Dispose()
		deviceMonitor.Dispose()
		for _, font := range fonts {
			font.Dispose()
		}
		return nil, err
	}
	widget.Disposing().Attach(func() {
		iconSet.Dispose()
		deviceMonitor.Dispose()
		for _, font := range fonts {
			font.Dispose()
		}
	})
	widget.SetPaintMode(walk.PaintBuffered)
	widget.SetInvalidatesOnResize(false)
	widget.MouseMove().Attach(func(x, y int, _ walk.MouseButton) {
		next := -1
		for _, item := range hitTargets {
			if x >= item.Bounds.X && x < item.Bounds.X+item.Bounds.Width && y >= item.Bounds.Y && y < item.Bounds.Y+item.Bounds.Height {
				next = item.Key
				break
			}
		}
		if next != hoveredTarget {
			hoveredTarget = next
			_ = widget.Invalidate()
		}
	})
	widget.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		for _, item := range hitTargets {
			if x >= item.Bounds.X && x < item.Bounds.X+item.Bounds.Width && y >= item.Bounds.Y && y < item.Bounds.Y+item.Bounds.Height && item.Run != nil {
				item.Run()
				return
			}
		}
	})
	return widget, nil
}
