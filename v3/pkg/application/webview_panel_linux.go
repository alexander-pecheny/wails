//go:build linux && cgo && !android && !server

package application

/*
#cgo gtk3 pkg-config: gtk+-3.0 webkit2gtk-4.1
#cgo !gtk3 pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#if GTK_MAJOR_VERSION >= 4
#include <webkit/webkit.h>
#else
#include <webkit2/webkit2.h>
#endif
#include <stdlib.h>
#include <string.h>

#if GTK_MAJOR_VERSION >= 4
extern gboolean handleKeyPressed(GtkEventControllerKey*, guint, guint, GdkModifierType, uintptr_t);
#else
extern gboolean onKeyPressEvent(GtkWidget*, GdkEventKey*, uintptr_t);
#endif
extern gboolean handlePermissionRequest(WebKitWebView*, WebKitPermissionRequest*, uintptr_t);

static gboolean embedded_panel_position(GtkOverlay *overlay, GtkWidget *widget, GdkRectangle *allocation, gpointer data) {
 GdkRectangle *bounds = g_object_get_data(G_OBJECT(widget), "wails-panel-bounds");
 if (!bounds) return FALSE;
 // Returning TRUE skips the overlay's own handler, which is what measures.
 gtk_widget_get_preferred_size(widget, NULL, NULL);
 *allocation = *bounds;
 return TRUE;
}

static gboolean embedded_panel_is_file(WebKitURIRequest *request) {
 const char *uri = webkit_uri_request_get_uri(request);
 return uri && g_str_has_prefix(uri, "file:");
}

// A link that wants a new window opens in the system browser. A file dropped
// on the panel must not replace the page.
static gboolean embedded_panel_policy(WebKitWebView *view, WebKitPolicyDecision *decision, WebKitPolicyDecisionType type, gpointer data) {
 if (type != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION && type != WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) return FALSE;
 WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(decision));
 WebKitURIRequest *request = webkit_navigation_action_get_request(action);
 if (type == WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) {
  const char *uri = webkit_uri_request_get_uri(request);
  if (uri && !embedded_panel_is_file(request)) g_app_info_launch_default_for_uri(uri, NULL, NULL);
  webkit_policy_decision_ignore(decision);
  return TRUE;
 }
 if (embedded_panel_is_file(request)) {
  webkit_policy_decision_ignore(decision);
  return TRUE;
 }
 return FALSE;
}

static GtkWidget* embedded_panel_create(WebKitWebView *view, WebKitNavigationAction *action, gpointer data) {
 WebKitURIRequest *request = webkit_navigation_action_get_request(action);
 const char *uri = webkit_uri_request_get_uri(request);
 if (uri && !embedded_panel_is_file(request)) g_app_info_launch_default_for_uri(uri, NULL, NULL);
 return NULL;
}

// The size of the area panels are positioned in: the main webview's.
static void embedded_panel_area(GtkWidget *main, int *width, int *height) {
#if GTK_MAJOR_VERSION >= 4
 *width = gtk_widget_get_width(main);
 *height = gtk_widget_get_height(main);
#else
 *width = gtk_widget_get_allocated_width(main);
 *height = gtk_widget_get_allocated_height(main);
#endif
}

static gboolean embedded_panel_foreign(WebKitWebView *view) {
 const char *uri = webkit_web_view_get_uri(view);
 return !uri || !g_str_has_prefix(uri, "wails:");
}

static gboolean embedded_panel_is_panel(WebKitWebView *view) {
 return g_object_get_data(G_OBJECT(view), "wails-panel") != NULL;
}

// Wrap only the main webview, preserving the menu and content-area coordinates.
static GtkWidget* embedded_panel_overlay(GtkWidget *main, GtkWidget *box) {
 GtkWidget *parent = gtk_widget_get_parent(main);
 if (GTK_IS_OVERLAY(parent)) return parent;
 // Moving the webview drops its keyboard focus.
 gboolean focused = gtk_widget_has_focus(main);
 GtkWidget *overlay = gtk_overlay_new();
 g_object_ref(main);
#if GTK_MAJOR_VERSION >= 4
 gtk_box_remove(GTK_BOX(box), main);
 gtk_overlay_set_child(GTK_OVERLAY(overlay), main);
 gtk_box_append(GTK_BOX(box), overlay);
#else
 gtk_container_remove(GTK_CONTAINER(box), main);
 gtk_container_add(GTK_CONTAINER(overlay), main);
 gtk_box_pack_start(GTK_BOX(box), overlay, TRUE, TRUE, 0);
#endif
 g_object_unref(main);
 gtk_widget_set_hexpand(overlay, TRUE);
 gtk_widget_set_vexpand(overlay, TRUE);
 g_signal_connect(overlay, "get-child-position", G_CALLBACK(embedded_panel_position), NULL);
 gtk_widget_set_visible(overlay, TRUE);
 gtk_widget_set_visible(main, TRUE);
 if (focused) gtk_widget_grab_focus(main);
 return overlay;
}

static GtkWidget* embedded_panel_new(GtkWidget *main, unsigned int windowID) {
 WebKitWebContext *context = webkit_web_view_get_context(WEBKIT_WEB_VIEW(main));
 GtkWidget *view = g_object_new(WEBKIT_TYPE_WEB_VIEW, "web-context", context, NULL);
 // The shared context already has Wails' local asset scheme registered.
 g_object_set_data(G_OBJECT(view), "windowid", GUINT_TO_POINTER(windowID));
 g_object_set_data(G_OBJECT(view), "wails-panel", GINT_TO_POINTER(1));
 WebKitSettings *mainSettings = webkit_web_view_get_settings(WEBKIT_WEB_VIEW(main));
 webkit_settings_set_hardware_acceleration_policy(webkit_web_view_get_settings(WEBKIT_WEB_VIEW(view)),
  webkit_settings_get_hardware_acceleration_policy(mainSettings));
 gpointer winID = (gpointer)(uintptr_t)windowID;
#if GTK_MAJOR_VERSION >= 4
 GtkEventController *keys = gtk_event_controller_key_new();
 gtk_widget_add_controller(view, keys);
 g_signal_connect(keys, "key-pressed", G_CALLBACK(handleKeyPressed), winID);
#else
 g_signal_connect(view, "key-press-event", G_CALLBACK(onKeyPressEvent), winID);
 // Parent show_all must not undo an explicitly hidden panel.
 gtk_widget_set_no_show_all(view, TRUE);
#endif
 g_signal_connect(view, "permission-request", G_CALLBACK(handlePermissionRequest), winID);
 g_signal_connect(view, "decide-policy", G_CALLBACK(embedded_panel_policy), NULL);
 g_signal_connect(view, "create", G_CALLBACK(embedded_panel_create), NULL);
 return view;
}

static void embedded_panel_hide(GtkWidget *view, GtkWidget *main) {
 gboolean focused = gtk_widget_has_focus(view);
 gtk_widget_set_visible(view, FALSE);
 if (focused) gtk_widget_grab_focus(main);
}
static void embedded_panel_bounds(GtkWidget *view, int x, int y, int width, int height) {
 GdkRectangle *bounds = g_new(GdkRectangle, 1);
 *bounds = (GdkRectangle){x,y,width,height};
 g_object_set_data_full(G_OBJECT(view), "wails-panel-bounds", bounds, g_free);
 GtkWidget *parent = gtk_widget_get_parent(view);
 if (parent) gtk_widget_queue_resize(parent);
}
static void embedded_panel_raise(GtkWidget *overlay, GtkWidget *view) {
#if GTK_MAJOR_VERSION >= 4
 gtk_widget_insert_before(view, overlay, NULL);
#else
 gtk_overlay_reorder_overlay(GTK_OVERLAY(overlay), view, -1);
#endif
}
static void embedded_panel_remove(GtkWidget *overlay, GtkWidget *view) {
 webkit_web_view_stop_loading(WEBKIT_WEB_VIEW(view));
#if GTK_MAJOR_VERSION >= 4
 gtk_overlay_remove_overlay(GTK_OVERLAY(overlay), view);
#else
 gtk_widget_destroy(view);
#endif
}
static void embedded_panel_header(WebKitURIRequest *request, const char *key, const char *value) {
 SoupMessageHeaders *headers = webkit_uri_request_get_http_headers(request);
 if (headers) soup_message_headers_replace(headers, key, value);
}
static void embedded_panel_colour(WebKitWebView *view, int r, int g, int b, int a) {
 GdkRGBA colour = {r/255.0,g/255.0,b/255.0,a/255.0};
 webkit_web_view_set_background_color(view,&colour);
}
*/
import "C"
import (
	"unsafe"
)

type linuxPanelImpl struct {
	panel   *WebviewPanel
	webview *C.GtkWidget
	overlay *C.GtkWidget
	parent  *linuxWebviewWindow
}

func newPanelImpl(panel *WebviewPanel) webviewPanelImpl {
	parent, ok := panel.parent.impl.(*linuxWebviewWindow)
	if !ok || parent.webview == nil {
		return nil
	}
	return &linuxPanelImpl{panel: panel, parent: parent}
}
func (p *linuxPanelImpl) view() *C.WebKitWebView {
	return (*C.WebKitWebView)(unsafe.Pointer(p.webview))
}
func (p *linuxPanelImpl) create() {
	options := p.panel.snapshotOptions()
	p.overlay = C.embedded_panel_overlay((*C.GtkWidget)(p.parent.webview), (*C.GtkWidget)(p.parent.vbox))
	p.webview = C.embedded_panel_new((*C.GtkWidget)(p.parent.webview), C.uint(p.panel.parent.id))
	C.gtk_overlay_add_overlay((*C.GtkOverlay)(unsafe.Pointer(p.overlay)), p.webview)
	C.gtk_overlay_set_clip_overlay((*C.GtkOverlay)(unsafe.Pointer(p.overlay)), p.webview, 1)
	p.setBounds(Rect{X: options.X, Y: options.Y, Width: options.Width, Height: options.Height})
	settings := C.webkit_web_view_get_settings(p.view())
	enabled := globalApplication.isDebugMode
	if options.DevToolsEnabled != nil {
		enabled = *options.DevToolsEnabled
	}
	C.webkit_settings_set_enable_developer_extras(settings, gtkBool(enabled))
	if options.UserAgent != "" {
		agent := C.CString(options.UserAgent)
		C.webkit_settings_set_user_agent(settings, agent)
		C.free(unsafe.Pointer(agent))
	}
	colour := options.BackgroundColour
	if options.Transparent {
		colour = RGBA{}
	}
	C.embedded_panel_colour(p.view(), C.int(colour.Red), C.int(colour.Green), C.int(colour.Blue), C.int(colour.Alpha))
	p.setZoom(options.Zoom)
	if options.URL != "" {
		uri := C.CString(options.URL)
		request := C.webkit_uri_request_new(uri)
		C.free(unsafe.Pointer(uri))
		for key, value := range options.Headers {
			k, v := C.CString(key), C.CString(value)
			C.embedded_panel_header(request, k, v)
			C.free(unsafe.Pointer(k))
			C.free(unsafe.Pointer(v))
		}
		C.webkit_web_view_load_request(p.view(), request)
		C.g_object_unref(C.gpointer(request))
	}
	if *options.Visible {
		p.show()
	} else {
		p.hide()
	}
	if enabled && options.OpenInspectorOnStartup {
		p.openDevTools()
	}
}
func (p *linuxPanelImpl) destroy() {
	if p.webview != nil {
		C.embedded_panel_remove(p.overlay, p.webview)
		p.webview = nil
		p.overlay = nil
	}
}
func (p *linuxPanelImpl) setBounds(bounds Rect) {
	if p.webview != nil {
		C.embedded_panel_bounds(p.webview, C.int(bounds.X), C.int(bounds.Y), C.int(bounds.Width), C.int(bounds.Height))
	}
}
func (p *linuxPanelImpl) bounds() Rect {
	options := p.panel.snapshotOptions()
	return Rect{X: options.X, Y: options.Y, Width: options.Width, Height: options.Height}
}
func (p *linuxPanelImpl) setZIndex(_ int) {
	panels := p.panel.sortedSiblings()
	for _, panel := range panels {
		panel.destroyedLock.RLock()
		native, ok := panel.impl.(*linuxPanelImpl)
		panel.destroyedLock.RUnlock()
		if ok && native.webview != nil {
			C.embedded_panel_raise(native.overlay, native.webview)
		}
	}
}
func (p *linuxPanelImpl) setURL(url string) {
	if p.webview == nil {
		return
	}
	uri := C.CString(url)
	defer C.free(unsafe.Pointer(uri))
	C.webkit_web_view_load_uri(p.view(), uri)
}
func (p *linuxPanelImpl) reload() {
	if p.webview != nil {
		C.webkit_web_view_reload(p.view())
	}
}
func (p *linuxPanelImpl) forceReload() {
	if p.webview != nil {
		C.webkit_web_view_reload_bypass_cache(p.view())
	}
}
func (p *linuxPanelImpl) show() {
	if p.webview != nil {
		C.gtk_widget_set_visible(p.webview, 1)
	}
}
func (p *linuxPanelImpl) hide() {
	if p.webview != nil {
		C.embedded_panel_hide(p.webview, (*C.GtkWidget)(p.parent.webview))
	}
}
func (p *linuxPanelImpl) isVisible() bool {
	return p.webview != nil && C.gtk_widget_get_visible(p.webview) != 0
}
func (p *linuxPanelImpl) setZoom(zoom float64) {
	if p.webview != nil {
		C.webkit_web_view_set_zoom_level(p.view(), C.double(zoom))
	}
}
func (p *linuxPanelImpl) getZoom() float64 {
	if p.webview != nil {
		return float64(C.webkit_web_view_get_zoom_level(p.view()))
	}
	return 1
}
func (p *linuxPanelImpl) openDevTools() {
	if p.webview != nil {
		C.webkit_web_inspector_show(C.webkit_web_view_get_inspector(p.view()))
	}
}
func (p *linuxPanelImpl) focus() {
	if p.webview != nil {
		C.gtk_widget_grab_focus(p.webview)
	}
}
func (p *linuxPanelImpl) isFocused() bool {
	return p.webview != nil && C.gtk_widget_has_focus(p.webview) != 0
}

func panelAreaSize(w *WebviewWindow) (int, int, bool) {
	parent, ok := w.impl.(*linuxWebviewWindow)
	if !ok || parent.webview == nil {
		return 0, 0, false
	}
	var width, height C.int
	C.embedded_panel_area((*C.GtkWidget)(parent.webview), &width, &height)
	return int(width), int(height), width > 0 && height > 0
}

// panelRequestOrigin tells a panel's requests apart, and among them those made
// by a page that the asset server did not serve.
func panelRequestOrigin(webView unsafe.Pointer) (panel, foreign bool) {
	view := (*C.WebKitWebView)(webView)
	if C.embedded_panel_is_panel(view) == 0 {
		return false, false
	}
	return true, C.embedded_panel_foreign(view) != 0
}
