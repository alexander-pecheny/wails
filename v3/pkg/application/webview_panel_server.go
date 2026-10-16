//go:build server

package application

// Native embedded webviews are unavailable in server deployments.
func newPanelImpl(_ *WebviewPanel) webviewPanelImpl { return nil }

func panelAreaSize(*WebviewWindow) (int, int, bool) { return 0, 0, false }
