//go:build windows

package edge

import (
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/internal/webview2/pkg/combridge"
	"golang.org/x/sys/windows"
)

type ICoreWebView2NewWindowRequestedEventArgs struct {
	vtbl *struct {
		_IUnknownVtbl
		GetUri       ComProc
		PutNewWindow ComProc
		GetNewWindow ComProc
		PutHandled   ComProc
		GetHandled   ComProc
	}
}

func (a *ICoreWebView2NewWindowRequestedEventArgs) GetUri() (string, error) {
	var value *uint16
	hr, _, _ := a.vtbl.GetUri.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&value)))
	if int32(hr) < 0 {
		return "", syscall.Errno(hr)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(value))
	return windows.UTF16PtrToString(value), nil
}

func (a *ICoreWebView2NewWindowRequestedEventArgs) PutHandled(handled bool) error {
	hr, _, _ := a.vtbl.PutHandled.Call(uintptr(unsafe.Pointer(a)), uintptr(boolToInt(handled)))
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}

type newWindowRequested interface {
	Receive(*ICoreWebView2NewWindowRequestedEventArgs)
}

type newWindowRequestedFunc func(*ICoreWebView2NewWindowRequestedEventArgs)

func (f newWindowRequestedFunc) Receive(args *ICoreWebView2NewWindowRequestedEventArgs) { f(args) }

func init() {
	combridge.RegisterVTable[combridge.IUnknown, newWindowRequested]("{d4c185fe-c81c-4989-97af-2d3fa7ab5651}",
		func(this uintptr, _ *ICoreWebView2, args *ICoreWebView2NewWindowRequestedEventArgs) uintptr {
			combridge.Resolve[newWindowRequested](this).Receive(args)
			return 0
		})
}

func (e *Chromium) addNewWindowRequested() error {
	handler := combridge.New[newWindowRequested](newWindowRequestedFunc(func(args *ICoreWebView2NewWindowRequestedEventArgs) {
		if callback := e.NewWindowRequestedCallback; callback != nil && !e.shuttingDown {
			callback(args)
		}
	}))
	defer handler.Close()
	var token _EventRegistrationToken
	hr, _, _ := e.webview.vtbl.AddNewWindowRequested.Call(uintptr(unsafe.Pointer(e.webview)), handler.Ref(), uintptr(unsafe.Pointer(&token)))
	if int32(hr) < 0 {
		return syscall.Errno(hr)
	}
	return nil
}
