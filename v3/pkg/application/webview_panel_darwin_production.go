//go:build darwin && !ios && !server && production && !devtools

package application

import "unsafe"

func enableEmbeddedPanelDevTools(_ unsafe.Pointer) {}
func openEmbeddedPanelDevTools(_ unsafe.Pointer)   {}
