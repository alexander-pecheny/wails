//go:build darwin && !ios && !server

package application

// The callbacks live apart from webview_panel_darwin.go because a cgo file
// with //export may only declare, not define, C functions in its preamble.

/*
#include <stdbool.h>
*/
import "C"
import (
	"unsafe"

	"github.com/wailsapp/wails/v3/internal/assetserver/webview"
)

//export processPanelURLRequest
func processPanelURLRequest(windowID C.uint, wkUrlSchemeTask unsafe.Pointer, foreignDocument C.bool) {
	window, ok := globalApplication.Window.GetByID(uint(windowID))
	if !ok || window == nil {
		globalApplication.debug("could not find window with id", "windowID", windowID)
		return
	}

	webviewRequests <- &webViewAssetRequest{
		Request:         webview.NewRequest(wkUrlSchemeTask),
		windowId:        uint(windowID),
		windowName:      window.Name(),
		panel:           true,
		foreignDocument: bool(foreignDocument),
	}
}

// panelDidCommitURL keeps URL() on the page actually shown, after redirects
// and navigation inside the panel.
//
//export panelDidCommitURL
func panelDidCommitURL(windowID C.uint, panelID C.uint, url *C.char) {
	window, ok := globalApplication.Window.GetByID(uint(windowID))
	if !ok {
		return
	}
	parent, ok := window.(*WebviewWindow)
	if !ok {
		return
	}
	panel := parent.GetPanelByID(uint(panelID))
	if panel == nil {
		return
	}
	committed := C.GoString(url)
	panel.destroyedLock.Lock()
	panel.options.URL = committed
	panel.destroyedLock.Unlock()
}
