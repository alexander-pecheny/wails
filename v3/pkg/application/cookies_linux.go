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

// WebKitGTK keeps cookies in memory unless told where to store them, so a
// login would not survive a restart. They go next to the rest of the website
// data, which is already persistent.
static void persist_cookies(void) {
#if GTK_MAJOR_VERSION >= 4
 WebKitNetworkSession *session = webkit_network_session_get_default();
 WebKitWebsiteDataManager *data = webkit_network_session_get_website_data_manager(session);
 WebKitCookieManager *cookies = webkit_network_session_get_cookie_manager(session);
#else
 WebKitWebContext *context = webkit_web_context_get_default();
 WebKitWebsiteDataManager *data = webkit_web_context_get_website_data_manager(context);
 WebKitCookieManager *cookies = webkit_website_data_manager_get_cookie_manager(data);
#endif
 const char *dir = webkit_website_data_manager_get_base_data_directory(data);
 if (!dir) return;
 char *file = g_build_filename(dir, "cookies.sqlite", NULL);
 webkit_cookie_manager_set_persistent_storage(cookies, file, WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
 g_free(file);
}
*/
import "C"
import "sync"

var persistCookiesOnce sync.Once

func persistCookies() {
	persistCookiesOnce.Do(func() { C.persist_cookies() })
}
