//go:build windows && !server

package application

import (
	"errors"
	"math"
	"net/url"

	"github.com/wailsapp/wails/v3/internal/assetserver/webview"
	"github.com/wailsapp/wails/v3/internal/webview2/pkg/edge"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// assetFilters match the asset origin, as the request cancellation Fetch patterns do.
var assetFilters = []string{"http://wails.localhost/*", "http://wails.localhost:*/*"}

type windowsPanelImpl struct {
	panel               *WebviewPanel
	parent              *windowsWebviewWindow
	chromium            *edge.Chromium
	requestCancellation *windowsRequestCancellation
	hwnd                w32.HWND
	// navigable is false until request cancellation is set up; the initial
	// navigation then goes to the latest URL.
	navigable bool
	// headerFilter is the exact initial URL while its headers are still owed.
	headerFilter     string
	recoveryAttempts int
	recoveryPending  bool
}

func newPanelImpl(panel *WebviewPanel) webviewPanelImpl {
	parent, ok := panel.parent.impl.(*windowsWebviewWindow)
	if !ok || parent.hwnd == 0 {
		return nil
	}
	if panel.parent.options.Windows.WebView2CompositionHosting {
		globalApplication.error("panel %s: panels are not supported in windows with WebView2CompositionHosting", panel.name)
		return nil
	}
	return &windowsPanelImpl{panel: panel, parent: parent}
}

func panelImplOf(panel *WebviewPanel) *windowsPanelImpl {
	panel.destroyedLock.RLock()
	defer panel.destroyedLock.RUnlock()
	native, _ := panel.impl.(*windowsPanelImpl)
	return native
}

func windowScale(hwnd w32.HWND) float64 {
	if w32.HasGetDpiForWindowFunc() {
		if dpi := w32.GetDpiForWindow(hwnd); dpi != 0 {
			return float64(dpi) / 96
		}
	}
	return 1
}

func (p *windowsPanelImpl) scale() float64 { return windowScale(p.parent.hwnd) }

func panelAreaSize(w *WebviewWindow) (int, int, bool) {
	native, ok := w.impl.(*windowsWebviewWindow)
	if !ok || native.hwnd == 0 {
		return 0, 0, false
	}
	rect := w32.GetClientRect(native.hwnd)
	if rect == nil {
		return 0, 0, false
	}
	scale := windowScale(native.hwnd)
	dip := func(v int32) int { return int(math.Round(float64(v) / scale)) }
	return dip(rect.Right - rect.Left), dip(rect.Bottom - rect.Top), true
}

func (p *windowsPanelImpl) create() {
	options := p.panel.snapshotOptions()
	if p.parent.chromium == nil || p.parent.chromium.Environment() == nil {
		globalApplication.error("panel %s: the window has no WebView2 environment", p.panel.name)
		return
	}
	style := uint(w32.WS_CHILD | w32.WS_CLIPCHILDREN | w32.WS_CLIPSIBLINGS)
	if *options.Visible {
		style |= w32.WS_VISIBLE
	}
	p.hwnd = w32.CreateWindowEx(0, w32.MustStringToUTF16Ptr("STATIC"), nil, style,
		0, 0, 1, 1, p.parent.hwnd, 0, w32.GetModuleHandle(""), nil)
	if p.hwnd == 0 {
		globalApplication.error("panel %s: create child window: %v", p.panel.name, w32.GetLastError())
		return
	}
	chromium := edge.NewChromium()
	if globalApplication.options.ErrorHandler != nil {
		chromium.SetErrorCallback(globalApplication.options.ErrorHandler)
	}
	chromium.BrowserPath = globalApplication.options.Windows.WebviewBrowserPath
	chromium.WebResourceRequestedCallback = p.processRequest
	chromium.AcceleratorKeyCallback = p.parent.processKeyBinding
	chromium.NewWindowRequestedCallback = p.newWindowRequested
	chromium.ProcessFailedCallback = p.processFailed
	chromium.NavigationCompletedCallback = p.navigationCompleted
	p.parent.applyPermissions(chromium)
	// Sharing the window's environment shares its profile, so cookies persist
	// where the window's do, and its browser arguments and features.
	cancelled := func() bool { return p.panel.isDestroyed() || p.panel.parent.isDestroyed() }
	if err := chromium.EmbedWithEnvironment(p.hwnd, p.parent.chromium.Environment(), cancelled); err != nil {
		if !errors.Is(err, edge.ErrEmbedCancelled) {
			globalApplication.error("panel %s: create WebView2: %v", p.panel.name, err)
		}
		p.destroy()
		return
	}
	p.chromium = chromium
	if err := p.configure(options); err != nil {
		globalApplication.error("panel %s: configure WebView2: %v", p.panel.name, err)
		p.destroy()
		return
	}
	p.setBounds(Rect{X: options.X, Y: options.Y, Width: options.Width, Height: options.Height})
	if *options.Visible {
		p.show()
	} else {
		p.hide()
	}
	if globalApplication.assets == nil {
		p.navigateInitial()
	} else {
		p.requestCancellation = newRequestCancellation(chromium)
		p.requestCancellation.start(func() bool { return p.chromium == chromium && !p.panel.isDestroyed() }, p.navigateInitial)
	}
	if devToolsEnabled(options) && options.OpenInspectorOnStartup {
		p.openDevTools()
	}
}

func devToolsEnabled(options WebviewPanelOptions) bool {
	if options.DevToolsEnabled != nil {
		return *options.DevToolsEnabled
	}
	return globalApplication.isDebugMode
}

func (p *windowsPanelImpl) configure(options WebviewPanelOptions) error {
	settings, err := p.chromium.GetSettings()
	if err != nil {
		return err
	}
	defer settings.Release()
	enabled := devToolsEnabled(options)
	if err := settings.PutAreDevToolsEnabled(enabled); err != nil {
		return err
	}
	if err := settings.PutAreDefaultContextMenusEnabled(enabled); err != nil {
		return err
	}
	if err := settings.PutAreBrowserAcceleratorKeysEnabled(false); err != nil {
		return err
	}
	if options.UserAgent != "" {
		if err := settings.PutUserAgent(options.UserAgent); err != nil {
			return err
		}
	}
	p.setZoom(options.Zoom)
	colour := options.BackgroundColour
	if options.Transparent {
		colour = RGBA{}
	}
	if err := p.chromium.SetBackgroundColourWithError(colour.Red, colour.Green, colour.Blue, colour.Alpha); err != nil {
		return err
	}
	for _, filter := range assetFilters {
		if err := p.chromium.AddWebResourceRequestedFilterWithError(filter, edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL); err != nil {
			return err
		}
	}
	if len(options.Headers) > 0 && options.URL != "" {
		filter := panelRequestURL(options.URL)
		if err := p.chromium.AddWebResourceRequestedFilterWithError(filter, edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_DOCUMENT); err != nil {
			return err
		}
		p.headerFilter = filter
	}
	return nil
}

func (p *windowsPanelImpl) navigateInitial() {
	p.navigable = true
	if url := p.panel.snapshotOptions().URL; url != "" {
		p.chromium.Navigate(url)
	}
}

func (p *windowsPanelImpl) dropHeaderFilter() {
	if p.headerFilter == "" {
		return
	}
	if err := p.chromium.RemoveWebResourceRequestedFilterWithError(p.headerFilter, edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_DOCUMENT); err != nil {
		globalApplication.error("panel %s: remove request filter: %v", p.panel.name, err)
	}
	p.headerFilter = ""
}

// processRequest supplies initial navigation headers and local assets without
// installing the parent window's message bridge in panel content.
func (p *windowsPanelImpl) processRequest(req *edge.ICoreWebView2WebResourceRequest, args *edge.ICoreWebView2WebResourceRequestedEventArgs) {
	uri, err := req.GetUri()
	if err != nil {
		return
	}
	if p.headerFilter != "" && panelRequestURL(uri) == p.headerFilter {
		if headers, err := req.GetHeaders(); err == nil {
			for key, value := range p.panel.snapshotOptions().Headers {
				if err := headers.SetHeader(key, value); err != nil {
					globalApplication.error("panel %s: request header: %v", p.panel.name, err)
				}
			}
			headers.Release()
		}
		p.dropHeaderFilter()
	}
	if globalApplication.assets == nil || !isAssetURL(uri) {
		return
	}
	context, err := args.GetResourceContext()
	document := err == nil && context == edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_DOCUMENT
	source, _ := p.chromium.Source()
	request, err := webview.NewRequest(p.chromium.Environment(), args, InvokeSync)
	if err != nil {
		globalApplication.error("panel %s: asset request: %v", p.panel.name, err)
		return
	}
	if p.requestCancellation != nil && !p.requestCancellation.closed {
		request = p.requestCancellation.tracker.Wrap(request)
	}
	webviewRequests <- &webViewAssetRequest{
		Request:         request,
		windowId:        p.parent.parent.id,
		windowName:      p.parent.parent.Name(),
		panel:           true,
		foreignDocument: panelForeignDocument(source, document),
	}
}

func isAssetURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "http" && u.Hostname() == "wails.localhost"
}

// panelForeignDocument reports whether an asset request comes from a page the
// asset server did not serve. A document request is a navigation, which an
// external page may start but whose response it cannot read.
func panelForeignDocument(source string, document bool) bool {
	return !document && !isAssetURL(source)
}

// A browser adds '/' to an empty HTTP path and strips fragments from requests.
func panelRequestURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment, parsed.RawFragment = "", ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}

func (p *windowsPanelImpl) newWindowRequested(args *edge.ICoreWebView2NewWindowRequestedEventArgs) {
	if err := args.PutHandled(true); err != nil {
		globalApplication.error("panel %s: refuse new window: %v", p.panel.name, err)
	}
	uri, err := args.GetUri()
	if err != nil || uri == "" || uri == "about:blank" {
		return
	}
	go func() {
		if err := globalApplication.Browser.OpenURL(uri); err != nil {
			globalApplication.error("panel %s: open %s: %v", p.panel.name, uri, err)
		}
	}()
}

// processFailed reloads a panel whose renderer died. A browser process exit
// takes the parent window's WebView down as well, and rebuildPanels recreates
// the panels in the parent's new environment.
func (p *windowsPanelImpl) processFailed(_ *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
	kind, err := args.GetProcessFailedKind()
	if err != nil || p.recoveryPending || webviewRecoveryActionFor(kind) != webviewRecoveryRenavigate {
		return
	}
	if p.recoveryAttempts >= maxWebviewRecoveryAttempts {
		globalApplication.error("panel %s: renderer failed %d times in a row; giving up", p.panel.name, p.recoveryAttempts)
		return
	}
	p.recoveryAttempts++
	p.recoveryPending = true
	chromium := p.chromium
	// Reload after the event handler returns, as the window's recovery does.
	go InvokeAsync(func() {
		p.recoveryPending = false
		if p.chromium == chromium && !p.panel.isDestroyed() {
			if err := chromium.Reload(); err != nil {
				globalApplication.error("panel %s: reload after renderer failure: %v", p.panel.name, err)
			}
		}
	})
}

func (p *windowsPanelImpl) navigationCompleted(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
	if ok, err := args.GetIsSuccess(); shouldResetWebviewRecoveryBudget(ok, err) {
		p.recoveryAttempts = 0
	}
}

func (p *windowsPanelImpl) destroy() {
	if p.parent.lastFocusedPanel == p {
		p.parent.lastFocusedPanel = nil
	}
	// Cancellation sends CDP commands, so it must close before the WebView does.
	p.requestCancellation.close()
	p.requestCancellation = nil
	if p.chromium != nil {
		p.chromium.Close()
		p.chromium = nil
	}
	if p.hwnd != 0 {
		w32.DestroyWindow(p.hwnd)
		p.hwnd = 0
	}
	p.navigable, p.headerFilter = false, ""
	p.recoveryAttempts, p.recoveryPending = 0, false
}

func (p *windowsPanelImpl) setBounds(bounds Rect) {
	if p.hwnd == 0 {
		return
	}
	scale := p.scale()
	physical := func(v int) int { return int(math.Round(float64(v) * scale)) }
	w32.SetWindowPos(p.hwnd, 0, physical(bounds.X), physical(bounds.Y), physical(bounds.Width), physical(bounds.Height), w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
	if p.chromium != nil {
		p.chromium.Resize()
	}
}

func (p *windowsPanelImpl) bounds() Rect {
	if p.hwnd == 0 {
		return Rect{}
	}
	rect := w32.GetWindowRect(p.hwnd)
	if rect == nil {
		return Rect{}
	}
	x, y := w32.ClientToScreen(p.parent.hwnd, 0, 0)
	scale := p.scale()
	dip := func(v int) int { return int(math.Round(float64(v) / scale)) }
	return Rect{X: dip(int(rect.Left) - x), Y: dip(int(rect.Top) - y), Width: dip(int(rect.Right - rect.Left)), Height: dip(int(rect.Bottom - rect.Top))}
}

func (p *windowsPanelImpl) setZIndex(_ int) {
	// Reorder every sibling so values, rather than the last setter, determine order.
	for _, panel := range p.panel.sortedSiblings() {
		if native := panelImplOf(panel); native != nil && native.hwnd != 0 {
			w32.SetWindowPos(native.hwnd, w32.HWND_TOP, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOACTIVATE)
		}
	}
}

func (p *windowsPanelImpl) setURL(url string) {
	if p.chromium == nil {
		return
	}
	p.dropHeaderFilter()
	if p.navigable {
		p.chromium.Navigate(url)
	}
}

func (p *windowsPanelImpl) reload() {
	if p.chromium != nil {
		if err := p.chromium.Reload(); err != nil {
			globalApplication.error("panel %s: reload: %v", p.panel.name, err)
		}
	}
}

func (p *windowsPanelImpl) forceReload() {
	if p.chromium == nil {
		return
	}
	chromium := p.chromium
	err := chromium.CallDevTools("Page.reload", `{"ignoreCache":true}`, func(_ string, err error) {
		if err != nil && p.chromium == chromium {
			p.reload()
		}
	})
	if err != nil {
		p.reload()
	}
}

func (p *windowsPanelImpl) show() {
	if p.hwnd == 0 {
		return
	}
	w32.ShowWindow(p.hwnd, w32.SW_SHOWNA)
	if p.chromium != nil {
		if err := p.chromium.Show(); err != nil {
			globalApplication.error("panel %s: show: %v", p.panel.name, err)
		}
	}
}

func (p *windowsPanelImpl) hide() {
	if p.hwnd == 0 {
		return
	}
	focused := p.isFocused()
	w32.ShowWindow(p.hwnd, w32.SW_HIDE)
	if p.chromium != nil {
		if err := p.chromium.Hide(); err != nil {
			globalApplication.error("panel %s: hide: %v", p.panel.name, err)
		}
	}
	if p.parent.lastFocusedPanel == p {
		p.parent.lastFocusedPanel = nil
	}
	if focused && !p.parent.isMinimised() {
		p.parent.focus()
	}
}

func (p *windowsPanelImpl) isVisible() bool {
	return p.hwnd != 0 && uint32(w32.GetWindowLong(p.hwnd, w32.GWL_STYLE))&w32.WS_VISIBLE != 0
}

func (p *windowsPanelImpl) setZoom(zoom float64) {
	if p.chromium != nil {
		p.chromium.PutZoomFactor(zoom)
	}
}

func (p *windowsPanelImpl) getZoom() float64 {
	if p.chromium != nil {
		if controller := p.chromium.GetController(); controller != nil {
			if zoom, err := controller.GetZoomFactor(); err == nil {
				return zoom
			}
		}
	}
	return p.panel.snapshotOptions().Zoom
}

func (p *windowsPanelImpl) openDevTools() {
	if p.chromium != nil {
		p.chromium.OpenDevToolsWindow()
	}
}

func (p *windowsPanelImpl) focus() {
	if p.chromium != nil {
		p.parent.focusingChromium = true
		p.chromium.Focus()
		p.parent.focusingChromium = false
	}
}

func (p *windowsPanelImpl) isFocused() bool {
	if p.hwnd == 0 {
		return false
	}
	for hwnd := w32.GetFocus(); hwnd != 0; hwnd = w32.GetParent(hwnd) {
		if hwnd == p.hwnd {
			return true
		}
	}
	return false
}

// rememberPanelFocus records which panel has focus before the window loses it.
func (w *windowsWebviewWindow) rememberPanelFocus() {
	w.lastFocusedPanel = nil
	for _, panel := range w.parent.GetPanels() {
		if native := panelImplOf(panel); native != nil && native.isFocused() {
			w.lastFocusedPanel = native
			return
		}
	}
}

// focusLastPanel gives focus back to the panel that had it, if it is still shown.
func (w *windowsWebviewWindow) focusLastPanel() bool {
	p := w.lastFocusedPanel
	if p == nil || p.chromium == nil || !p.isVisible() || p.panel.isDestroyed() {
		return false
	}
	p.focus()
	return true
}

func (w *windowsWebviewWindow) notifyPanelsMoved() {
	for _, panel := range w.parent.GetPanels() {
		if native := panelImplOf(panel); native != nil && native.chromium != nil {
			_ = native.chromium.NotifyParentWindowPositionChanged()
		}
	}
}

// rebuildPanels recreates the panels after the window's WebView was rebuilt:
// they shared its dead browser process, and the new WebView covers them.
func (w *windowsWebviewWindow) rebuildPanels() {
	var top *windowsPanelImpl
	for _, panel := range w.parent.GetPanels() {
		native := panelImplOf(panel)
		if native == nil || panel.creating || panel.nativeDestroyed || panel.isDestroyed() {
			continue
		}
		panel.creating = true
		native.destroy()
		native.create()
		panel.creating = false
		if panel.isDestroyed() {
			panel.destroyNative(native)
			continue
		}
		top = native
	}
	if top != nil {
		top.setZIndex(0)
	}
}
