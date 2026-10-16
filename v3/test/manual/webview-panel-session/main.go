// Checks that panels share one persistent profile and never reach the runtime.
// Run twice: `go run ./test/manual/webview-panel-session set`, then `... check`.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// A fixed port keeps the cookie's origin the same across both runs.
const site = "127.0.0.1:18765"

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "set" && os.Args[1] != "check") {
		log.Fatal("usage: webview-panel-session set|check")
	}
	phase := os.Args[1]
	seen := make(chan string, 8)
	listener, err := net.Listen("tcp", site)
	if err != nil {
		log.Fatal(err)
	}
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("session")
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "kept", MaxAge: 3600, Path: "/"})
			fmt.Fprint(w, `<script>fetch('/report?from=setter')</script>`)
		case "/read":
			fmt.Fprint(w, `<script>fetch('/report?from=reader')</script>`)
		case "/report":
			seen <- fmt.Sprintf("%s:%v", r.URL.Query().Get("from"), cookie != nil && cookie.Value == "kept")
		}
	}))
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/local.html":
			fmt.Fprint(w, `<script>fetch('/wails/runtime',{method:'POST',body:'{}'}).then(r=>fetch('/runtime-status?'+r.status))</script>`)
		case "/runtime-status":
			seen <- "runtime:" + r.URL.RawQuery
		default:
			fmt.Fprint(w, `<p>session test</p>`)
		}
	})
	app := application.New(application.Options{Name: "Panel session test", Assets: application.AssetOptions{Handler: assets}})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{Width: 800, Height: 600, URL: "/"})
	want := map[string]string{"runtime": "403", "reader": "true"}
	window.NewPanel(application.WebviewPanelOptions{Name: "local", URL: "/local.html", Width: 200, Height: 200})
	if phase == "set" {
		want["setter"] = "true"
		window.NewPanel(application.WebviewPanelOptions{Name: "setter", URL: "http://" + site + "/set", X: 200, Width: 200, Height: 200})
	} else {
		window.NewPanel(application.WebviewPanelOptions{Name: "reader", URL: "http://" + site + "/read", X: 200, Width: 200, Height: 200})
	}
	var result error
	go func() {
		defer app.Quit()
		got := map[string]string{}
		deadline := time.After(45 * time.Second)
		for len(got) < len(want) {
			select {
			case s := <-seen:
				key, value, _ := strings.Cut(s, ":")
				if key == "setter" {
					// A panel created after the cookie was set must see it
					// through the shared profile.
					window.NewPanel(application.WebviewPanelOptions{Name: "reader", URL: "http://" + site + "/read", X: 400, Width: 200, Height: 200})
				}
				got[key] = value
			case <-deadline:
				result = fmt.Errorf("timeout: got %v, want %v", got, want)
				return
			}
		}
		for key, value := range want {
			if got[key] != value {
				result = fmt.Errorf("%s: got %q, want %q", key, got[key], value)
				return
			}
		}
	}()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	if result != nil {
		log.Fatal(result)
	}
	fmt.Println("PASS", phase)
}
