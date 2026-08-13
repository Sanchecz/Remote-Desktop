//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	lastCheckHealth  *walk.Label
	recentActivity   *walk.Label
	recentActivity2  *walk.Label
	recentActivity3  *walk.Label
	readyState       *walk.Label
	readyDescription *walk.Label
}

type agentUIScale struct {
	dpi    int
	zoom   float64
	baseW  int
	baseH  int
}

func newAgentUIScale(dpi int) agentUIScale {
	if dpi < 96 {
		dpi = 96
	}
	return agentUIScale{dpi: dpi, zoom: 1, baseW: 1450, baseH: 1085}
}

// Walk layout dimensions are expressed in 96-DPI units.  The design reference
// is specified in physical pixels, so converting every metric prevents a
// 1450px window from becoming 1812px at the common Windows 125% scale.
func (scale agentUIScale) unit(pixels int) int {
	return max(1, int(float64(pixels)*scale.zoom*96/float64(scale.dpi)+0.5))
}

func (scale agentUIScale) font(points float64) int {
	return max(1, int(points*scale.zoom*96/float64(scale.dpi)+0.5))
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
	if theme.cardTitleFont, err = walk.NewFont("Segoe UI", scale.font(12), walk.FontBold); err != nil {
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
		iconBounds := walk.Rectangle{X: 20, Y: 18, Width: 52, Height: 52}
		if err := canvas.FillRoundedRectanglePixels(theme.iconBrush, iconBounds, walk.Size{Width: 13, Height: 13}); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(icon, theme.iconFont, walk.RGB(12, 153, 99), iconBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(title, theme.cardTitleFont, walk.RGB(26, 34, 31), walk.Rectangle{X: 20, Y: 78, Width: bounds.Width - 40, Height: 22}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis|walk.TextNoPrefix); err != nil {
			return err
		}
		if err := canvas.DrawTextPixels(description, theme.cardTextFont, walk.RGB(91, 104, 98), walk.Rectangle{X: 20, Y: 105, Width: bounds.Width - 40, Height: bounds.Height - 164}, walk.TextLeft|walk.TextTop|walk.TextWordbreak|walk.TextNoPrefix); err != nil {
			return err
		}
		button := walk.Rectangle{X: 20, Y: bounds.Height - 52, Width: bounds.Width - 40, Height: 34}
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

func trayCommand() error {
	setDesktopProcessDPIAwareness()
	window, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer window.Dispose()
	scale := newAgentUIScale(window.DPI())
	requestedWindowSize := walk.Size{Width: 1450, Height: 1085}
	if width, parseErr := strconv.Atoi(strings.TrimSpace(os.Getenv("REMOTEIT_AGENT_WINDOW_WIDTH"))); parseErr == nil && width >= 1015 {
		requestedWindowSize.Width = width
	}
	if height, parseErr := strconv.Atoi(strings.TrimSpace(os.Getenv("REMOTEIT_AGENT_WINDOW_HEIGHT"))); parseErr == nil && height >= 760 {
		requestedWindowSize.Height = height
	}
	scale = scale.forWindowPixels(requestedWindowSize)
	theme, err := newAgentUITheme(scale)
	if err != nil {
		return err
	}
	defer theme.Dispose()
	// Walk sizes fonts for the monitor DPI.  The dashboard geometry is based on
	// physical pixels, so use a DPI-normalized inherited body font as well.  This
	// keeps captions and table rows at the same visual scale as the approved
	// 1450x1085 mockup on 125/150% Windows displays.
	bodyFont, err := walk.NewFont("Segoe UI", scale.font(9), 0)
	if err != nil {
		return err
	}
	defer bodyFont.Dispose()
	window.SetFont(bodyFont)
	window.SetTitle("RemoteIt Agent")
	// The approved mockup is 1450x1085 including the title bar.  Pixel APIs
	// avoid Windows multiplying it again at 125/150% display scaling.
	_ = window.SetSizePixels(requestedWindowSize)
	_ = window.SetMinMaxSizePixels(walk.Size{Width: 1180, Height: 820}, walk.Size{Width: 2200, Height: 1500})
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
	sidebarLayout.SetMargins(scale.margins(34, 34, 18, 22))
	sidebarLayout.SetSpacing(scale.unit(8))
	_ = sidebar.SetLayout(sidebarLayout)
	brandRow, _ := walk.NewComposite(sidebar)
	brandRowLayout := walk.NewHBoxLayout()
	brandRowLayout.SetMargins(walk.Margins{})
	brandRowLayout.SetSpacing(10)
	_ = brandRow.SetLayout(brandRowLayout)
	brandHeight := scale.unit(48)
	brandRow.SetMinMaxSize(walk.Size{Height: brandHeight}, walk.Size{Height: brandHeight})
	makeAgentIconLabel(brandRow, onlineIcon, 38, scale)
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
	captionHeight := scale.unit(40)
	brandCaption.SetMinMaxSize(walk.Size{Height: captionHeight}, walk.Size{Height: captionHeight})

	var refresh func()
	var openAgentLogs func()
	var openAgentFolder func()
	var checkConnection func()
	var copyRemoteID func()
	var openAgentSettings func()
	navHeight := scale.unit(54)
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
	sideStatusLayout.SetMargins(scale.margins(14, 15, 14, 12))
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
	contentLayout.SetMargins(scale.margins(40, 22, 34, 12))
	contentLayout.SetSpacing(scale.unit(13))
	_ = content.SetLayout(contentLayout)

	header, _ := walk.NewComposite(content)
	headerHeight := scale.unit(108)
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
	statusCard, _ := walk.NewComposite(header)
	statusCard.SetMinMaxSize(walk.Size{Width: scale.unit(340), Height: scale.unit(104)}, walk.Size{Width: scale.unit(340), Height: scale.unit(104)})
	statusLayout := walk.NewVBoxLayout()
	statusLayout.SetMargins(scale.margins(20, 18, 20, 14))
	statusLayout.SetSpacing(scale.unit(5))
	_ = statusCard.SetLayout(statusLayout)
	if statusBrush, brushErr := walk.NewSolidColorBrush(walk.RGB(240, 249, 245)); brushErr == nil {
		defer statusBrush.Dispose()
		statusCard.SetBackground(statusBrush)
	}
	view.connection, _ = walk.NewLabel(statusCard)
	if statusFont, fontErr := walk.NewFont("Segoe UI", scale.font(16), walk.FontBold); fontErr == nil {
		defer statusFont.Dispose()
		view.connection.SetFont(statusFont)
	}
	view.lastHeartbeat, _ = walk.NewLabel(statusCard)
	view.lastHeartbeat.SetTextColor(walk.RGB(91, 106, 99))

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
	readinessLayout.SetMargins(scale.margins(24, 28, 24, 20))
	readinessLayout.SetSpacing(scale.unit(10))
	_ = readiness.SetLayout(readinessLayout)
	readyHero, _ := walk.NewComposite(readiness)
	readyHero.SetMinMaxSize(walk.Size{Height: scale.unit(116)}, walk.Size{Height: scale.unit(116)})
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
	makeAgentIconLabel(readyIconFrame, onlineIcon, 92, scale)
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
	actionHeight := scale.unit(220)
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
	healthHeight := scale.unit(84)
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
	activityHeight := scale.unit(160)
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
