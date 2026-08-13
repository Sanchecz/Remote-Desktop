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
	recentActivity   *walk.Label
	recentActivity2  *walk.Label
	recentActivity3  *walk.Label
	readyState       *walk.Label
	readyDescription *walk.Label
}

func makeAgentIconLabel(parent walk.Container, icon *walk.Icon, size int) *walk.ImageView {
	view, _ := walk.NewImageView(parent)
	view.SetImage(icon)
	view.SetMode(walk.ImageViewModeShrink)
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
}

func newAgentUITheme() (*agentUITheme, error) {
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
	if theme.navFont, err = walk.NewFont("Segoe UI", 11, walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.cardTitleFont, err = walk.NewFont("Segoe UI", 12, walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.cardTextFont, err = walk.NewFont("Segoe UI", 10, 0); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.iconFont, err = walk.NewFont("Segoe UI Symbol", 13, walk.FontBold); err != nil {
		theme.Dispose()
		return nil, err
	}
	if theme.arrowFont, err = walk.NewFont("Segoe UI", 15, 0); err != nil {
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
		theme.cardTextFont, theme.iconFont, theme.arrowFont,
	} {
		if disposable != nil {
			disposable.Dispose()
		}
	}
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
		iconSize := 40
		iconX := 12
		textX := 66
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

func trayCommand() error {
	setDesktopProcessDPIAwareness()
	theme, err := newAgentUITheme()
	if err != nil {
		return err
	}
	defer theme.Dispose()
	window, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer window.Dispose()
	window.SetTitle("RemoteIt Agent")
	// Match the spacious dashboard proportions without filling a 1080p work
	// area. PerMonitorV2 keeps these logical sizes stable at 125/150% scaling.
	window.SetSize(walk.Size{Width: 1450, Height: 1040})
	window.SetMinMaxSize(walk.Size{Width: 1180, Height: 820}, walk.Size{Width: 2200, Height: 1500})
	window.SetBackground(theme.pageBrush)
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

	view := trayView{}

	sidebar, _ := walk.NewComposite(window)
	sidebar.SetMinMaxSize(walk.Size{Width: 304}, walk.Size{Width: 304})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(252, 253, 252)); brushErr == nil {
		defer brush.Dispose()
		sidebar.SetBackground(brush)
	}
	sidebarLayout := walk.NewVBoxLayout()
	sidebarLayout.SetMargins(walk.Margins{HNear: 34, VNear: 34, HFar: 18, VFar: 22})
	sidebarLayout.SetSpacing(9)
	_ = sidebar.SetLayout(sidebarLayout)

	agentBadge, _ := walk.NewLabel(sidebar)
	agentBadge.SetText("  АГЕНТ")
	agentBadge.SetTextColor(walk.RGB(13, 158, 105))
	agentBadge.SetMinMaxSize(walk.Size{Height: 20}, walk.Size{Height: 20})
	if badgeFont, fontErr := walk.NewFont("Segoe UI", 8, walk.FontBold); fontErr == nil {
		defer badgeFont.Dispose()
		agentBadge.SetFont(badgeFont)
	}
	brandRow, _ := walk.NewComposite(sidebar)
	brandRowLayout := walk.NewHBoxLayout()
	brandRowLayout.SetMargins(walk.Margins{})
	brandRowLayout.SetSpacing(10)
	_ = brandRow.SetLayout(brandRowLayout)
	brandRow.SetMinMaxSize(walk.Size{Height: 48}, walk.Size{Height: 48})
	makeAgentIconLabel(brandRow, onlineIcon, 38)
	brand, _ := walk.NewLabel(brandRow)
	brand.SetText("RemoteIt")
	brand.SetMinMaxSize(walk.Size{Height: 48}, walk.Size{Height: 48})
	if font, fontErr := walk.NewFont("Segoe UI", 22, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		brand.SetFont(font)
	}
	brandCaption, _ := walk.NewLabel(sidebar)
	brandCaption.SetText("Агент безопасного\nудалённого доступа")
	brandCaption.SetTextColor(walk.RGB(91, 99, 96))
	brandCaption.SetMinMaxSize(walk.Size{Height: 40}, walk.Size{Height: 40})

	var refresh func()
	var openAgentLogs func()
	var openAgentFolder func()
	var checkConnection func()
	var copyRemoteID func()
	var openAgentSettings func()
	if _, err := newAgentActionCard(sidebar, theme, "●", "Обзор", "", true, 54, func() { refresh() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "↗", "Панель управления", "", false, 54, func() { _ = openURL(defaultServer) }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "↻", "Проверить соединение", "", false, 54, func() { checkConnection() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "ID", "Remote ID", "", false, 54, func() { copyRemoteID() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "≡", "Журнал Agent", "", false, 54, func() { openAgentLogs() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "▣", "Папка Agent", "", false, 54, func() { openAgentFolder() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(sidebar, theme, "⚙", "Настройки", "", false, 54, func() { openAgentSettings() }); err != nil {
		return err
	}
	walk.NewVSpacer(sidebar)

	sideStatus, _ := walk.NewComposite(sidebar)
	sideStatus.SetMinMaxSize(walk.Size{Height: 102}, walk.Size{Height: 102})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(241, 250, 246)); brushErr == nil {
		defer brush.Dispose()
		sideStatus.SetBackground(brush)
	}
	sideStatusLayout := walk.NewVBoxLayout()
	sideStatusLayout.SetMargins(walk.Margins{HNear: 14, VNear: 12, HFar: 14, VFar: 10})
	sideStatusLayout.SetSpacing(4)
	_ = sideStatus.SetLayout(sideStatusLayout)
	sideStatusTitle, _ := walk.NewLabel(sideStatus)
	sideStatusTitle.SetText("●  Агент работает")
	sideStatusTitle.SetTextColor(walk.RGB(13, 148, 99))
	if font, fontErr := walk.NewFont("Segoe UI", 9, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		sideStatusTitle.SetFont(font)
	}
	sideStatusText, _ := walk.NewLabel(sideStatus)
	sideStatusText.SetText("Служба запущена и готова\nк безопасному подключению.")
	sideStatusText.SetTextColor(walk.RGB(93, 105, 99))
	sideStatusText.SetMinMaxSize(walk.Size{Height: 40}, walk.Size{Height: 40})
	sideVersion, _ := walk.NewLabel(sidebar)
	sideVersion.SetText("Версия Agent " + version)
	sideVersion.SetTextColor(walk.RGB(105, 114, 110))

	content, _ := walk.NewComposite(window)
	contentLayout := walk.NewVBoxLayout()
	contentLayout.SetMargins(walk.Margins{HNear: 40, VNear: 28, HFar: 34, VFar: 18})
	contentLayout.SetSpacing(13)
	_ = content.SetLayout(contentLayout)

	header, _ := walk.NewComposite(content)
	header.SetMinMaxSize(walk.Size{Height: 108}, walk.Size{Height: 108})
	headerLayout := walk.NewHBoxLayout()
	headerLayout.SetMargins(walk.Margins{})
	headerLayout.SetSpacing(18)
	_ = header.SetLayout(headerLayout)
	heading, _ := walk.NewComposite(header)
	headingLayout := walk.NewVBoxLayout()
	headingLayout.SetMargins(walk.Margins{})
	headingLayout.SetSpacing(2)
	_ = heading.SetLayout(headingLayout)
	title, _ := walk.NewLabel(heading)
	title.SetText("RemoteIt")
	title.SetMinMaxSize(walk.Size{Height: 50}, walk.Size{Height: 50})
	if font, fontErr := walk.NewFont("Segoe UI", 34, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		title.SetFont(font)
	}
	description, _ := walk.NewLabel(heading)
	description.SetText("Агент безопасного удалённого доступа")
	description.SetTextColor(walk.RGB(93, 102, 99))
	walk.NewHSpacer(header)
	statusCard, _ := walk.NewComposite(header)
	statusCard.SetMinMaxSize(walk.Size{Width: 300, Height: 86}, walk.Size{Width: 350, Height: 86})
	statusLayout := walk.NewVBoxLayout()
	statusLayout.SetMargins(walk.Margins{HNear: 20, VNear: 15, HFar: 20, VFar: 12})
	statusLayout.SetSpacing(5)
	_ = statusCard.SetLayout(statusLayout)
	if statusBrush, brushErr := walk.NewSolidColorBrush(walk.RGB(240, 249, 245)); brushErr == nil {
		defer statusBrush.Dispose()
		statusCard.SetBackground(statusBrush)
	}
	view.connection, _ = walk.NewLabel(statusCard)
	if statusFont, fontErr := walk.NewFont("Segoe UI", 16, walk.FontBold); fontErr == nil {
		defer statusFont.Dispose()
		view.connection.SetFont(statusFont)
	}
	view.lastHeartbeat, _ = walk.NewLabel(statusCard)
	view.lastHeartbeat.SetTextColor(walk.RGB(91, 106, 99))

	body, _ := walk.NewComposite(content)
	bodyLayout := walk.NewHBoxLayout()
	bodyLayout.SetMargins(walk.Margins{})
	bodyLayout.SetSpacing(14)
	_ = body.SetLayout(bodyLayout)

	details, _ := walk.NewComposite(body)
	details.SetMinMaxSize(walk.Size{Width: 490}, walk.Size{Width: 2000})
	if detailsBrush, brushErr := walk.NewSolidColorBrush(walk.RGB(255, 255, 255)); brushErr == nil {
		defer detailsBrush.Dispose()
		details.SetBackground(detailsBrush)
	}
	detailsLayout := walk.NewVBoxLayout()
	detailsLayout.SetMargins(walk.Margins{HNear: 22, VNear: 18, HFar: 22, VFar: 18})
	detailsLayout.SetSpacing(6)
	_ = details.SetLayout(detailsLayout)
	detailsTitle, _ := walk.NewLabel(details)
	detailsTitle.SetText("▣   Устройство")
	detailsTitle.SetTextColor(walk.RGB(14, 148, 99))
	if headingFont, fontErr := walk.NewFont("Segoe UI", 12, walk.FontBold); fontErr == nil {
		defer headingFont.Dispose()
		detailsTitle.SetFont(headingFont)
	}
	view.name = addTrayInfoRow(details, "Название")
	view.connectionID = addTrayInfoRow(details, "Remote ID")
	view.localIP = addTrayInfoRow(details, "Локальный IP")
	view.version = addTrayInfoRow(details, "Версия")
	view.installMode = addTrayInfoRow(details, "Установка")
	view.service = addTrayInfoRow(details, "Фоновый агент")
	view.remoteSession = addTrayInfoRow(details, "Удалённый доступ")
	view.server = addTrayInfoRow(details, "Сервер")

	readiness, _ := walk.NewComposite(body)
	readiness.SetMinMaxSize(walk.Size{Width: 590}, walk.Size{Width: 2000})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(232, 249, 241)); brushErr == nil {
		defer brush.Dispose()
		readiness.SetBackground(brush)
	}
	readinessLayout := walk.NewVBoxLayout()
	readinessLayout.SetMargins(walk.Margins{HNear: 30, VNear: 34, HFar: 30, VFar: 24})
	readinessLayout.SetSpacing(11)
	_ = readiness.SetLayout(readinessLayout)
	view.readyState, _ = walk.NewLabel(readiness)
	view.readyState.SetText("✓   Онлайн и готов к подключению")
	view.readyState.SetTextColor(walk.RGB(10, 142, 91))
	if font, fontErr := walk.NewFont("Segoe UI", 17, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		view.readyState.SetFont(font)
	}
	view.readyDescription, _ = walk.NewLabel(readiness)
	view.readyDescription.SetText("Служба Agent работает корректно\nи подключена к серверу RemoteIt.")
	view.readyDescription.SetTextColor(walk.RGB(66, 86, 77))
	view.readyDescription.SetMinMaxSize(walk.Size{Height: 48}, walk.Size{Height: 48})
	readyDivider, _ := walk.NewLabel(readiness)
	readyDivider.SetText("────────────────────────────────────────────")
	readyDivider.SetTextColor(walk.RGB(177, 222, 202))
	readySummary, _ := walk.NewLabel(readiness)
	readySummary.SetText("✓  Подключение       ◇  Сервер       TLS 1.2       ⬡  Шифрование включено")
	readySummary.SetTextColor(walk.RGB(12, 142, 92))

	actions, _ := walk.NewComposite(content)
	actionsLayout := walk.NewHBoxLayout()
	actionsLayout.SetMargins(walk.Margins{})
	actionsLayout.SetSpacing(12)
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
	if _, err := newAgentActionCard(actions, theme, "▣", "Панель управления", "Устройства, доступ и настройки", false, 142, func() { _ = openURL(defaultServer) }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(actions, theme, "⌁", "Проверить соединение", "Диагностика связи с сервером", false, 142, func() { checkConnection() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(actions, theme, "ID", "Remote ID", "Уникальный ID этого устройства", false, 142, func() { copyRemoteID() }); err != nil {
		return err
	}
	if _, err := newAgentActionCard(actions, theme, "≡", "Журнал Agent", "События службы и диагностика", false, 142, func() { openAgentLogs() }); err != nil {
		return err
	}

	health, _ := walk.NewComposite(content)
	health.SetMinMaxSize(walk.Size{Height: 84}, walk.Size{Height: 84})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(255, 255, 255)); brushErr == nil {
		defer brush.Dispose()
		health.SetBackground(brush)
	}
	healthLayout := walk.NewHBoxLayout()
	healthLayout.SetMargins(walk.Margins{HNear: 18, VNear: 12, HFar: 18, VFar: 12})
	healthLayout.SetSpacing(12)
	_ = health.SetLayout(healthLayout)
	healthTitle, _ := walk.NewLabel(health)
	healthTitle.SetText("Состояние системы")
	healthTitle.SetTextColor(walk.RGB(13, 148, 99))
	healthTitle.SetMinMaxSize(walk.Size{Width: 150}, walk.Size{Width: 150})
	view.serviceHealth = addTrayHealthItem(health, "Служба агента")
	view.serverHealth = addTrayHealthItem(health, "Связь с сервером")
	view.configHealth = addTrayHealthItem(health, "Конфигурация")
	view.securityHealth = addTrayHealthItem(health, "Безопасность")

	activity, _ := walk.NewComposite(content)
	activity.SetMinMaxSize(walk.Size{Height: 122}, walk.Size{Height: 122})
	if brush, brushErr := walk.NewSolidColorBrush(walk.RGB(245, 251, 248)); brushErr == nil {
		defer brush.Dispose()
		activity.SetBackground(brush)
	}
	activityLayout := walk.NewVBoxLayout()
	activityLayout.SetMargins(walk.Margins{HNear: 18, VNear: 10, HFar: 18, VFar: 10})
	activityLayout.SetSpacing(12)
	_ = activity.SetLayout(activityLayout)
	activityTitle, _ := walk.NewLabel(activity)
	activityTitle.SetText("◷  Недавняя активность")
	activityTitle.SetMinMaxSize(walk.Size{Height: 24}, walk.Size{Height: 24})
	if font, fontErr := walk.NewFont("Segoe UI", 10, walk.FontBold); fontErr == nil {
		defer font.Dispose()
		activityTitle.SetFont(font)
	}
	view.recentActivity, _ = walk.NewLabel(activity)
	view.recentActivity.SetText("●  Ожидаем первую синхронизацию")
	view.recentActivity.SetTextColor(walk.RGB(86, 98, 92))
	view.recentActivity2, _ = walk.NewLabel(activity)
	view.recentActivity2.SetText("●  Служба Agent запущена")
	view.recentActivity2.SetTextColor(walk.RGB(86, 98, 92))
	view.recentActivity3, _ = walk.NewLabel(activity)
	view.recentActivity3.SetText("●  Конфигурация проверена")
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

	info, _ := walk.NewLabel(content)
	info.SetText("🛡  Название устройства меняется только администратором или техником в авторизованной панели и автоматически синхронизируется с Agent.")
	info.SetTextColor(walk.RGB(22, 132, 92))
	info.SetMinMaxSize(walk.Size{Height: 34}, walk.Size{Height: 40})

	footer, _ := walk.NewLabel(content)
	footer.SetText("Фоновая служба запускается вместе с Windows. Закрытие окна сворачивает Agent обратно в трей.")
	footer.SetTextColor(walk.RGB(111, 123, 117))
	footer.SetMinMaxSize(walk.Size{Height: 22}, walk.Size{Height: 28})

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

func addTrayInfoRow(parent walk.Container, caption string) *walk.Label {
	row, _ := walk.NewComposite(parent)
	row.SetMinMaxSize(walk.Size{Height: 25}, walk.Size{Height: 25})
	layout := walk.NewHBoxLayout()
	layout.SetMargins(walk.Margins{})
	layout.SetSpacing(12)
	_ = row.SetLayout(layout)
	label, _ := walk.NewLabel(row)
	label.SetText(caption)
	label.SetMinMaxSize(walk.Size{Width: 150}, walk.Size{Width: 150})
	label.SetTextColor(walk.RGB(99, 112, 104))
	value, _ := walk.NewLabel(row)
	value.SetText("—")
	return value
}

func addTrayHealthItem(parent walk.Container, caption string) *walk.Label {
	item, _ := walk.NewComposite(parent)
	item.SetMinMaxSize(walk.Size{Width: 105}, walk.Size{Width: 1000})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 8, VNear: 4, HFar: 8, VFar: 4})
	layout.SetSpacing(2)
	_ = item.SetLayout(layout)
	title, _ := walk.NewLabel(item)
	title.SetText("✓  " + caption)
	title.SetTextColor(walk.RGB(12, 150, 98))
	value, _ := walk.NewLabel(item)
	value.SetText("Проверяется")
	value.SetTextColor(walk.RGB(91, 104, 98))
	return value
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
		view.recentActivity.SetText("Синхронизация выполнена · устройство успешно связано с сервером")
		if !previous {
			_ = tray.SetIcon(onlineIcon)
			_ = window.SetIcon(onlineIcon)
		}
		_ = tray.SetToolTip("RemoteIt Agent — в сети")
	} else {
		view.connection.SetText("●  Нет связи — переподключение…")
		view.connection.SetTextColor(walk.RGB(211, 67, 67))
		view.serverHealth.SetText("Переподключение")
		view.recentActivity.SetText("Восстанавливаем защищённое соединение с сервером")
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
		view.recentActivity.SetText("Активно удалённое управление мышью и клавиатурой")
	} else {
		view.remoteSession.SetText("Нет активного сеанса")
		view.remoteSession.SetTextColor(walk.RGB(91, 106, 99))
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
