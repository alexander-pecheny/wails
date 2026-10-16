//go:build darwin && !ios && !server

package application

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include <stdlib.h>
#include "Cocoa/Cocoa.h"
#import "webview_window_darwin.h"
#import "mac_private_api_darwin.h"
#import <WebKit/WebKit.h>

// Implemented in webview_panel_darwin_delegate.m.
extern void* panelSchemeHandlerNew(unsigned int windowId);
extern void* panelDelegateNew(unsigned int windowId, unsigned int panelId);
extern void panelObjectRelease(void* object);

static NSUInteger panelAutoresizingMask(bool top, bool bottom, bool left, bool right) {
	NSUInteger mask = 0;
	if (left && right) {
		mask |= NSViewWidthSizable;
	} else if (right) {
		mask |= NSViewMinXMargin;
	}
	if (top && bottom) {
		mask |= NSViewHeightSizable;
	} else if (bottom) {
		mask |= NSViewMaxYMargin;
	} else {
		mask |= NSViewMinYMargin;
	}
	return mask;
}

static void panelCopyPreferences(WKPreferences* to, WKPreferences* from) {
	to.tabFocusesLinks = from.tabFocusesLinks;
	to.minimumFontSize = from.minimumFontSize;
#if MAC_OS_X_VERSION_MAX_ALLOWED >= 101500
	if (@available(macOS 10.15, *)) {
		to.fraudulentWebsiteWarningEnabled = from.fraudulentWebsiteWarningEnabled;
	}
#endif
#if MAC_OS_X_VERSION_MAX_ALLOWED >= 110300
	if (@available(macOS 11.3, *)) {
		to.textInteractionEnabled = from.textInteractionEnabled;
	}
#endif
#if MAC_OS_X_VERSION_MAX_ALLOWED >= 120300
	if (@available(macOS 12.3, *)) {
		to.elementFullscreenEnabled = from.elementFullscreenEnabled;
	}
#endif
}

// Create a new WebviewPanel
void* panelNew(void* parentWindow, void* schemeHandler, void* delegate, int x, int y, int width, int height,
		bool transparent, bool top, bool bottom, bool left, bool right) {
	NSWindow<WailsWebviewWindow>* window = (NSWindow<WailsWebviewWindow>*)parentWindow;
	NSView* contentView = [window contentView];

	// Calculate frame (macOS uses bottom-left origin)
	NSRect contentBounds = [contentView bounds];
	NSRect frame = NSMakeRect(x, contentBounds.size.height - y - height, width, height);

	WKWebViewConfiguration* config = [[WKWebViewConfiguration alloc] init];
	[config autorelease];

	// Sharing the main webview's data store keeps one cookie jar for the window.
	WKWebView* mainWebView = window.webView;
	if (mainWebView != nil) {
		WKWebViewConfiguration* mainConfig = mainWebView.configuration;
		config.processPool = mainConfig.processPool;
		config.websiteDataStore = mainConfig.websiteDataStore;
		config.applicationNameForUserAgent = mainConfig.applicationNameForUserAgent;
		config.allowsAirPlayForMediaPlayback = mainConfig.allowsAirPlayForMediaPlayback;
		config.mediaTypesRequiringUserActionForPlayback = mainConfig.mediaTypesRequiringUserActionForPlayback;
		panelCopyPreferences(config.preferences, mainConfig.preferences);
	} else {
		config.applicationNameForUserAgent = @"wails.io";
	}
	config.suppressesIncrementalRendering = true;
	[config setURLSchemeHandler:(id<WKURLSchemeHandler>)schemeHandler forURLScheme:@"wails"];

	WKWebView* webView = [[WKWebView alloc] initWithFrame:frame configuration:config];
	if (mainWebView != nil) {
		webView.allowsMagnification = mainWebView.allowsMagnification;
		webView.allowsBackForwardNavigationGestures = mainWebView.allowsBackForwardNavigationGestures;
	}
	[webView setNavigationDelegate:(id<WKNavigationDelegate>)delegate];
	[webView setUIDelegate:(id<WKUIDelegate>)delegate];
	[webView setAutoresizingMask:panelAutoresizingMask(top, bottom, left, right)];

	if (transparent) {
		wailsPrivateSetWebviewTransparent((void*)webView);
	}

	[contentView addSubview:webView];

	return webView;
}

static bool panelHasFocus(WKWebView* webView, NSWindow* window) {
	NSResponder* responder = [window firstResponder];
	return responder == webView || ([responder isKindOfClass:[NSView class]] && [(NSView*)responder isDescendantOf:webView]);
}

// AppKit would otherwise leave the key focus in a view the user cannot see.
static void panelYieldFocus(WKWebView* webView, void* parentWindow) {
	NSWindow<WailsWebviewWindow>* window = (NSWindow<WailsWebviewWindow>*)parentWindow;
	if (panelHasFocus(webView, window)) {
		[window makeFirstResponder:window.webView];
	}
}

// Destroy a WebviewPanel
void panelDestroy(void* panel, void* parentWindow, void* schemeHandler, void* delegate) {
	WKWebView* webView = (WKWebView*)panel;
	panelYieldFocus(webView, parentWindow);
	[webView stopLoading];
	[webView setNavigationDelegate:nil];
	[webView setUIDelegate:nil];
	[webView removeFromSuperview];
	[webView release];
	panelObjectRelease(delegate);
	panelObjectRelease(schemeHandler);
}

// Set panel bounds
void panelSetBounds(void* panel, void* parentWindow, int x, int y, int width, int height) {
	WKWebView* webView = (WKWebView*)panel;
	NSWindow<WailsWebviewWindow>* window = (NSWindow<WailsWebviewWindow>*)parentWindow;
	NSView* contentView = [window contentView];

	// Calculate frame (macOS uses bottom-left origin)
	NSRect contentBounds = [contentView bounds];
	NSRect frame = NSMakeRect(x, contentBounds.size.height - y - height, width, height);

	[webView setFrame:frame];
}

// Get panel bounds
void panelGetBounds(void* panel, void* parentWindow, int* x, int* y, int* width, int* height) {
	WKWebView* webView = (WKWebView*)panel;
	NSWindow<WailsWebviewWindow>* window = (NSWindow<WailsWebviewWindow>*)parentWindow;
	NSView* contentView = [window contentView];

	NSRect frame = [webView frame];
	NSRect contentBounds = [contentView bounds];

	*x = (int)frame.origin.x;
	*y = (int)(contentBounds.size.height - frame.origin.y - frame.size.height);
	*width = (int)frame.size.width;
	*height = (int)frame.size.height;
}

static void panelAreaGetSize(void* parentWindow, int* width, int* height) {
	NSRect bounds = [[(NSWindow*)parentWindow contentView] bounds];
	*width = (int)bounds.size.width;
	*height = (int)bounds.size.height;
}

static NSComparisonResult panelCompareRank(__kindof NSView* a, __kindof NSView* b, void* context) {
	NSDictionary* ranks = (NSDictionary*)context;
	NSInteger rankA = [ranks[@((uintptr_t)a)] integerValue];
	NSInteger rankB = [ranks[@((uintptr_t)b)] integerValue];
	if (rankA < rankB) return NSOrderedAscending;
	if (rankA > rankB) return NSOrderedDescending;
	return NSOrderedSame;
}

// Place panels in ascending z-index order above the other subviews. Sorting
// reorders in place: removing and re-adding a view would take its focus and
// fire visibilitychange in its page.
static void panelOrder(void* parentWindow, void** panels, int count) {
	NSView* contentView = [(NSWindow*)parentWindow contentView];
	NSArray* subviews = [contentView subviews];
	NSUInteger total = [subviews count];
	NSMutableDictionary* ranks = [NSMutableDictionary dictionaryWithCapacity:total];
	for (NSUInteger i = 0; i < total; i++) {
		ranks[@((uintptr_t)subviews[i])] = @(i);
	}
	for (int i = 0; i < count; i++) {
		ranks[@((uintptr_t)panels[i])] = @(total + i);
	}
	BOOL ordered = YES;
	NSInteger previous = -1;
	for (NSView* view in subviews) {
		NSInteger rank = [ranks[@((uintptr_t)view)] integerValue];
		if (rank < previous) {
			ordered = NO;
			break;
		}
		previous = rank;
	}
	if (!ordered) {
		[contentView sortSubviewsUsingFunction:panelCompareRank context:ranks];
	}
}

// NSURL rejects some strings Go accepts, such as non-ASCII or spaces in the query.
static NSURL* panelURL(const char* url) {
	NSString* string = [NSString stringWithUTF8String:url];
	if (string == nil) return nil;
	NSURL* result = [NSURL URLWithString:string];
	if (result != nil) return result;
	NSMutableCharacterSet* allowed = [[NSCharacterSet URLQueryAllowedCharacterSet] mutableCopy];
	[allowed addCharactersInString:@"#%[]"];
	NSString* encoded = [string stringByAddingPercentEncodingWithAllowedCharacters:allowed];
	[allowed release];
	if (encoded == nil) return nil;
	return [NSURL URLWithString:encoded];
}

// Navigate to URL, with headers given as a JSON object or NULL.
static bool panelLoadURL(void* panel, const char* url, const char* headersJSON) {
	WKWebView* webView = (WKWebView*)panel;
	NSURL* target = panelURL(url);
	if (target == nil) return false;
	NSMutableURLRequest* request = [NSMutableURLRequest requestWithURL:target];
	if (headersJSON != NULL) {
		NSData* data = [[NSString stringWithUTF8String:headersJSON] dataUsingEncoding:NSUTF8StringEncoding];
		id parsed = data == nil ? nil : [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
		if ([parsed isKindOfClass:[NSDictionary class]]) {
			NSDictionary* headers = (NSDictionary*)parsed;
			for (NSString* key in headers) {
				[request setValue:headers[key] forHTTPHeaderField:key];
			}
		}
	}
	[webView loadRequest:request];
	return true;
}

// Reload
void panelReload(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	[webView reload];
}

// Force reload (bypass cache)
void panelForceReload(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	[webView reloadFromOrigin];
}

// Show panel
void panelShow(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	[webView setHidden:NO];
}

// Hide panel
void panelHide(void* panel, void* parentWindow) {
	WKWebView* webView = (WKWebView*)panel;
	panelYieldFocus(webView, parentWindow);
	[webView setHidden:YES];
}

// Check if visible
bool panelIsVisible(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	return ![webView isHidden];
}

// Set zoom
void panelSetZoom(void* panel, double zoom) {
	WKWebView* webView = (WKWebView*)panel;
	[webView setMagnification:zoom];
}

// Get zoom
double panelGetZoom(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	return [webView magnification];
}

// Focus panel
void panelFocus(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	[[webView window] makeFirstResponder:webView];
}

// Check if focused
bool panelIsFocused(void* panel) {
	WKWebView* webView = (WKWebView*)panel;
	return panelHasFocus(webView, [webView window]);
}

// Set background color
void panelSetBackgroundColour(void* panel, int r, int g, int b, int a) {
	if (a == 0) {
		wailsPrivateSetWebviewTransparent(panel);
	} else {
		wailsPrivateSetWebviewBackgroundColour(panel, r, g, b, a);
	}
}

static void panelConfigure(void* panel, const char* userAgent) {
	WKWebView* view = (WKWebView*)panel;
	if (userAgent && userAgent[0]) view.customUserAgent = [NSString stringWithUTF8String:userAgent];
}
*/
import "C"
import (
	"encoding/json"
	"net/url"
	"unsafe"
)

type darwinPanelImpl struct {
	panel          *WebviewPanel
	webview        unsafe.Pointer
	parentNSWindow unsafe.Pointer
	schemeHandler  unsafe.Pointer
	delegate       unsafe.Pointer
}

func newPanelImpl(panel *WebviewPanel) webviewPanelImpl {
	parentWindow := panel.parent
	if parentWindow == nil || parentWindow.impl == nil {
		return nil
	}

	darwinParent, ok := parentWindow.impl.(*macosWebviewWindow)
	if !ok {
		return nil
	}

	return &darwinPanelImpl{
		panel:          panel,
		parentNSWindow: darwinParent.nsWindow,
	}
}

func panelAreaSize(w *WebviewWindow) (int, int, bool) {
	native, ok := w.impl.(*macosWebviewWindow)
	if !ok || native.nsWindow == nil {
		return 0, 0, false
	}
	var width, height C.int
	C.panelAreaGetSize(native.nsWindow, &width, &height)
	return int(width), int(height), true
}

func (p *darwinPanelImpl) create() {
	options := p.panel.snapshotOptions()

	transparent := options.Transparent
	windowID := C.uint(p.panel.parent.id)
	p.schemeHandler = C.panelSchemeHandlerNew(windowID)
	p.delegate = C.panelDelegateNew(windowID, C.uint(p.panel.id))

	p.webview = C.panelNew(
		p.parentNSWindow,
		p.schemeHandler,
		p.delegate,
		C.int(options.X),
		C.int(options.Y),
		C.int(options.Width),
		C.int(options.Height),
		C.bool(transparent),
		C.bool(options.Anchor.HasAnchor(AnchorTop)),
		C.bool(options.Anchor.HasAnchor(AnchorBottom)),
		C.bool(options.Anchor.HasAnchor(AnchorLeft)),
		C.bool(options.Anchor.HasAnchor(AnchorRight)),
	)

	// Set background colour if not transparent
	if !transparent {
		C.panelSetBackgroundColour(
			p.webview,
			C.int(options.BackgroundColour.Red),
			C.int(options.BackgroundColour.Green),
			C.int(options.BackgroundColour.Blue),
			C.int(options.BackgroundColour.Alpha),
		)
	}

	// Set initial visibility
	if options.Visible != nil && !*options.Visible {
		C.panelHide(p.webview, p.parentNSWindow)
	}

	// Set zoom if specified
	if options.Zoom > 0 && options.Zoom != 1.0 {
		C.panelSetZoom(p.webview, C.double(options.Zoom))
	}

	agent := C.CString(options.UserAgent)
	C.panelConfigure(p.webview, agent)
	C.free(unsafe.Pointer(agent))
	enabled := globalApplication.isDebugMode
	if options.DevToolsEnabled != nil {
		enabled = *options.DevToolsEnabled
	}
	if enabled {
		enableEmbeddedPanelDevTools(p.webview)
	}
	if options.URL != "" {
		p.load(options.URL, initialHeaders(options))
	}
	if enabled && options.OpenInspectorOnStartup {
		p.openDevTools()
	}
}

// initialHeaders returns the headers for the first request as JSON, or nil
// when the URL may not receive them: over plain http they would travel in clear.
func initialHeaders(options WebviewPanelOptions) []byte {
	if len(options.Headers) == 0 {
		return nil
	}
	target, err := url.Parse(options.URL)
	if err != nil || (target.Scheme != "https" && target.Scheme != "wails") {
		return nil
	}
	headers, err := json.Marshal(options.Headers)
	if err != nil {
		return nil
	}
	return headers
}

func (p *darwinPanelImpl) load(target string, headers []byte) {
	uri := C.CString(target)
	defer C.free(unsafe.Pointer(uri))
	var rawHeaders *C.char
	if headers != nil {
		rawHeaders = C.CString(string(headers))
		defer C.free(unsafe.Pointer(rawHeaders))
	}
	if !C.panelLoadURL(p.webview, uri, rawHeaders) {
		globalApplication.error("panel URL is not valid", "panel", p.panel.name, "url", target)
	}
}

func (p *darwinPanelImpl) destroy() {
	if p.webview != nil {
		C.panelDestroy(p.webview, p.parentNSWindow, p.schemeHandler, p.delegate)
		p.webview = nil
		p.schemeHandler = nil
		p.delegate = nil
	}
}

func (p *darwinPanelImpl) setBounds(bounds Rect) {
	if p.webview == nil {
		return
	}
	C.panelSetBounds(
		p.webview,
		p.parentNSWindow,
		C.int(bounds.X),
		C.int(bounds.Y),
		C.int(bounds.Width),
		C.int(bounds.Height),
	)
}

func (p *darwinPanelImpl) bounds() Rect {
	if p.webview == nil {
		return Rect{}
	}
	var x, y, width, height C.int
	C.panelGetBounds(p.webview, p.parentNSWindow, &x, &y, &width, &height)
	return Rect{
		X:      int(x),
		Y:      int(y),
		Width:  int(width),
		Height: int(height),
	}
}

func (p *darwinPanelImpl) setZIndex(_ int) {
	var views []unsafe.Pointer
	for _, panel := range p.panel.sortedSiblings() {
		panel.destroyedLock.RLock()
		native, ok := panel.impl.(*darwinPanelImpl)
		panel.destroyedLock.RUnlock()
		if ok && native.webview != nil {
			views = append(views, native.webview)
		}
	}
	if len(views) == 0 {
		return
	}
	buffer := C.malloc(C.size_t(len(views)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	defer C.free(buffer)
	copy(unsafe.Slice((*unsafe.Pointer)(buffer), len(views)), views)
	C.panelOrder(p.parentNSWindow, (*unsafe.Pointer)(buffer), C.int(len(views)))
}

func (p *darwinPanelImpl) setURL(target string) {
	if p.webview == nil {
		return
	}
	p.load(target, nil)
}

func (p *darwinPanelImpl) reload() {
	if p.webview == nil {
		return
	}
	C.panelReload(p.webview)
}

func (p *darwinPanelImpl) forceReload() {
	if p.webview == nil {
		return
	}
	C.panelForceReload(p.webview)
}

func (p *darwinPanelImpl) show() {
	if p.webview == nil {
		return
	}
	C.panelShow(p.webview)
}

func (p *darwinPanelImpl) hide() {
	if p.webview == nil {
		return
	}
	C.panelHide(p.webview, p.parentNSWindow)
}

func (p *darwinPanelImpl) isVisible() bool {
	if p.webview == nil {
		return false
	}
	return bool(C.panelIsVisible(p.webview))
}

func (p *darwinPanelImpl) setZoom(zoom float64) {
	if p.webview == nil {
		return
	}
	C.panelSetZoom(p.webview, C.double(zoom))
}

func (p *darwinPanelImpl) getZoom() float64 {
	if p.webview == nil {
		return 1.0
	}
	return float64(C.panelGetZoom(p.webview))
}

func (p *darwinPanelImpl) openDevTools() {
	if p.webview == nil {
		return
	}
	openEmbeddedPanelDevTools(p.webview)
}

func (p *darwinPanelImpl) focus() {
	if p.webview == nil {
		return
	}
	C.panelFocus(p.webview)
}

func (p *darwinPanelImpl) isFocused() bool {
	if p.webview == nil {
		return false
	}
	return bool(C.panelIsFocused(p.webview))
}
