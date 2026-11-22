package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	dashboardRate = 20
	packetLimit   = 4096
)

type dashboard struct {
	meterAccessState
	ctx              context.Context
	source           session.Source
	language         string
	mobile           bool
	settings         guiSettings
	settingsKey      string
	dark             bool
	lastBarStyle     mygo.BarStyle
	androidHidden    atomic.Bool
	notifyLanguage   atomic.Value
	islandDisplay    atomic.Pointer[islandDisplay]
	window           *mygo.Window
	service          *backend.Service
	sessionCtx       context.Context
	cancel           context.CancelFunc
	done             chan struct{}
	err              error
	logs             *diagnostics.Buffer
	logLevel         slog.Level
	logView          connectionLogView
	page             int
	activePage       atomic.Int32
	pageDirection    int
	scroll           ui.ScrollState
	ovLeftScroll     ui.ScrollState
	channel          int
	chartSeries      uint8
	chartHistory     chartHistoryCache
	chartStats       chartStatsCache
	chartGeometry    chartGeometry
	chartCanvas      *ui.Canvas
	chartCanvasKey   chartPlotCanvasKey
	chartPixelPts    [][2]float32
	chartTickMetrics [2]chartTickMetricsCache
	headerPlanCache  desktopHeaderPlanCache
	windowSec        float64
	fullSession      bool
	held             *backend.Snapshot
	chartPaused      atomic.Bool
	hasViewEnd       bool
	viewEnd          float64
	hasCursor        bool
	cursorAt         float64
	hasHover         bool
	hoverAt          float64
	chartHandle      ui.Handle
	dragMode         uint8
	dragMoved        bool
	dragClicks       int
	dragStartX       float32
	dragLastX        float32
	lastPressDragged bool
	displayAt        time.Time
	displaySample    *device.Sample
	displayStats     backend.Statistics
	displayBounds    [3]channelStat
	packets          packetView
	picker           filePicker
	meters           []device.Info
	deviceMenuOpen   bool
	menus            map[string]*bool
	dir              string
	toasts           []ui.Toast
	themeReveal      kit.ThemeReveal
	themeChanges     []themeChange
	snapshotOnly     bool
	busy             bool
	work             sync.WaitGroup
	uiMu             sync.Mutex
	uiQueue          []func()
}

const (
	minWindowWidth  = 640
	minWindowHeight = 560
)

func Run(parent context.Context, source session.Source, language, directory string) error {
	logs := &diagnostics.Buffer{}
	handler := logs.Handler()
	if os.Getenv("WATTSCOPE_DEBUG") == "1" {
		handler = slog.NewMultiHandler(handler, slog.NewTextHandler(os.Stderr, locales.DiagnosticTextOptions()))
	}
	logger := slog.New(handler)
	logger.Info(diagnostics.AppStarted, "os", runtime.GOOS, "arch", runtime.GOARCH)
	parent = diagnostics.WithLogger(parent, logger)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	model := &dashboard{
		ctx: ctx, source: source, dir: directory, logs: logs, windowSec: defaultWindowSec, fullSession: true,
		mobile: runtime.GOOS == "android" || runtime.GOOS == "ios",
	}
	model.source.Demo = false
	mygo.App.SetName("WattScope")
	offQuit := mygo.App.OnQuit(cancel)
	defer offQuit()
	if runtime.GOOS == "android" {
		offSuspend := mygo.Power.OnSuspend(func() {
			model.androidHidden.Store(true)
		})
		defer offSuspend()
		offResume := mygo.Power.OnResume(func() {
			model.androidHidden.Store(false)
			if ctx.Err() == nil {
				model.work.Go(ensureMeterForeground)
			}
			if model.window != nil {
				model.dispatchTo(model.window)(func() {
					if model.page == pageSettings {
						model.refreshMeterAccess()
					}
				})
			}
		})
		defer offResume()
	}
	var stopParentWatch func() bool
	mygo.App.WhenReady(func() {
		if ctx.Err() != nil {
			mygo.App.Quit()
			return
		}
		model.loadSettings(language)
		mygo.Theme.SetSource(model.settings.Theme)
		windowOptions := mygo.WindowOptions{
			Title: "WattScope", Width: 1120, Height: 720,
			Content: ui.View(model.view),
		}
		if runtime.GOOS == "android" {
			_, light := kit.ResolveTheme(false, model.settings.Accent)
			_, dark := kit.ResolveTheme(true, model.settings.Accent)
			windowOptions.BackgroundColor = fmt.Sprintf("light-dark(%s, %s)", kit.HexColor(light.Canvas), kit.HexColor(dark.Canvas))
		}
		if !model.mobile {
			windowOptions.StateKey = "main"
			windowOptions.TitleBarStyle = mygo.TitleBarHidden
			windowOptions.HideWindowButtons = true
			windowOptions.MinWidth = minWindowWidth
			windowOptions.MinHeight = minWindowHeight
		}
		model.window = mygo.NewWindow(windowOptions)
		model.window.OnClosed(cancel)
		stopParentWatch = context.AfterFunc(parent, mygo.App.Quit)
		model.connectWith(model.automaticMeterContext(), source)
	})
	err := mygo.App.Run()
	if stopParentWatch != nil {
		stopParentWatch()
	}
	cancel()
	model.work.Wait()
	return err
}

func (d *dashboard) notify(title, kind string) {
	if title == "" {
		return
	}
	timeout := 4 * time.Second
	if kind == "error" {
		timeout = 6 * time.Second
	}
	t := ui.Toast{
		ID:      title,
		Title:   title,
		Type:    kind,
		Timeout: timeout,
	}
	for i := range d.toasts {
		if d.toasts[i].ID == t.ID {
			d.toasts[i] = t
			return
		}
	}
	if len(d.toasts) >= 3 {
		copy(d.toasts, d.toasts[1:])
		d.toasts = d.toasts[:2]
	}
	d.toasts = append(d.toasts, t)
}

func (d *dashboard) menuOpen(key string) *bool {
	if d.menus == nil {
		d.menus = map[string]*bool{}
	}
	if flag, ok := d.menus[key]; ok {
		return flag
	}
	var open bool
	d.menus[key] = &open
	return &open
}

func (d *dashboard) hasOpenMenu() bool {
	if d.deviceMenuOpen {
		return true
	}
	for _, open := range d.menus {
		if *open {
			return true
		}
	}
	return false
}

func (d *dashboard) closeAllMenus() {
	d.deviceMenuOpen = false
	for _, open := range d.menus {
		*open = false
	}
}

var (
	mobilePageOrder  = [7]int{pageOverview: 0, pagePD: 1, pageRecords: 2, pageMore: 3, pageDevice: 4, pageConnectionLog: 5, pageSettings: 6}
	desktopPageOrder = [7]int{pageOverview: 0, pagePD: 1, pageRecords: 2, pageDevice: 3, pageConnectionLog: 4, pageSettings: 5, pageMore: 6}
)

func (d *dashboard) pageOrder(page int) int {
	if page < 0 || page >= len(desktopPageOrder) {
		return 0
	}
	if d.mobile {
		return mobilePageOrder[page]
	}
	return desktopPageOrder[page]
}

func (d *dashboard) goPage(page int) {
	if page < pageOverview || page > pageConnectionLog {
		return
	}
	if d.page == page {
		return
	}
	d.pageDirection = 1
	if d.pageOrder(page) < d.pageOrder(d.page) {
		d.pageDirection = -1
	}
	d.page = page
	d.activePage.Store(int32(page))
	d.scroll = ui.ScrollState{}
	d.ovLeftScroll = ui.ScrollState{}
	d.logView.list = ui.ListState{}
	d.closeAllMenus()
	if page == pageSettings {
		d.refreshMeterAccess()
	}
}

func (d *dashboard) connect() {
	d.connectWith(d.ctx, d.source)
}

func (d *dashboard) connectWith(parent context.Context, source session.Source) {
	if d.ctx.Err() != nil {
		return
	}
	logger := diagnostics.Logger(d.ctx)
	logger.InfoContext(d.ctx, diagnostics.Connecting, "driver_filter", source.Driver, "device", source.Device, "demo", source.Demo)
	service, err := session.New(session.Options{
		Source: source, Rate: dashboardRate, HistorySize: -1,
		PacketHistorySize: packetLimit, Reconnect: true,
	})
	if err != nil {
		logger.WarnContext(d.ctx, diagnostics.SessionFailed, "error", err)
		if d.service == nil {
			d.err = err
		}
		d.notify(err.Error(), "error")
		return
	}
	if d.cancel != nil {
		d.cancel()
	}
	d.err, d.service = nil, service
	d.packets = packetView{}
	d.resetChartView()
	d.displaySample, d.displayStats, d.displayBounds = nil, backend.Statistics{}, [3]channelStat{}
	ctx, cancel := context.WithCancel(parent)
	previous, done := d.done, make(chan struct{})
	d.sessionCtx, d.cancel, d.done = ctx, cancel, done
	d.work.Go(func() {
		defer close(done)
		if previous != nil {
			<-previous
		}
		stopMonitor := d.watchMeterNotification(ctx, service)
		defer stopMonitor()
		// Run reports terminal errors through the session snapshot and diagnostics.
		_ = service.Run(ctx)
	})
	if window := d.window; window != nil {
		invalidate := func() {
			if runtime.GOOS != "android" || !d.androidHidden.Load() {
				window.Invalidate()
			}
		}
		d.work.Go(func() {
			updates, unsubscribe := service.Subscribe()
			defer unsubscribe()
			var last dashboardUpdate
			var pausedRefresh <-chan time.Time
			for ctx.Err() == nil {
				select {
				case <-updates:
					page := int(d.activePage.Load())
					update, live := readDashboardUpdate(service, page)
					paused := runtime.GOOS == "android" && page == pageOverview && d.chartPaused.Load()
					if update != last || live && !paused {
						invalidate()
					} else if live && pausedRefresh == nil && !d.androidHidden.Load() {
						// While paused, only the surrounding status and counters need updating.
						pausedRefresh = time.After(readoutInterval)
					}
					last = update
				case <-pausedRefresh:
					pausedRefresh = nil
					invalidate()
				case <-done:
					if ctx.Err() == nil {
						invalidate()
					}
					return
				case <-ctx.Done():
					return
				}
			}
		})
	}
}

type dashboardUpdate struct {
	page          int
	connection    backend.ConnectionStatus
	oldest, total uint64
	recording     backend.CaptureRecording
}

func readDashboardUpdate(service *backend.Service, page int) (dashboardUpdate, bool) {
	update := dashboardUpdate{page: page}
	switch page {
	case pageOverview:
		update.connection = service.ConnectionStatus()
		return update, true
	case pageRecords, pageDevice:
		return update, true
	case pagePD:
		packets := service.PacketSnapshot(0, 0)
		update.oldest, update.total, update.recording = packets.Oldest, packets.Total, packets.Recording
	}
	update.connection = service.ConnectionStatus()
	return update, false
}
