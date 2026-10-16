//go:build darwin && !ios && !server && (!production || devtools)

package application

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c -DWAILS_MAC_DEVTOOLS
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <WebKit/WebKit.h>
#include "mac_private_api_darwin.h"

static void embeddedPanelEnableDevTools(void* panel) {
#if MAC_OS_X_VERSION_MAX_ALLOWED >= 130300
	if (@available(macOS 13.3, *)) {
		((WKWebView*)panel).inspectable = YES;
		return;
	}
#endif
	wailsPrivateEnableWebviewInspector(panel);
}
*/
import "C"
import "unsafe"

func enableEmbeddedPanelDevTools(view unsafe.Pointer) { C.embeddedPanelEnableDevTools(view) }
func openEmbeddedPanelDevTools(view unsafe.Pointer)   { C.wailsPrivateOpenWebviewInspector(view) }
