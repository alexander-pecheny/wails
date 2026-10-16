//go:build windows && !server

package application

import "testing"

func TestPanelInitialRequestURLCanonicalization(t *testing.T) {
	for _, url := range []string{"https://example.com", "https://example.com/", "https://example.com/#section"} {
		if got := panelRequestURL(url); got != "https://example.com/" {
			t.Errorf("initial headers would miss %q: %q", url, got)
		}
	}
	if panelRequestURL("https://example.com/?page=2") == panelRequestURL("https://example.com/?page=1") {
		t.Fatal("headers could be applied to a different request")
	}
}

func TestPanelForeignDocument(t *testing.T) {
	for _, tc := range []struct {
		source   string
		document bool
		foreign  bool
	}{
		{"http://wails.localhost/index.html", false, false},
		{"http://wails.localhost:34115/", false, false},
		{"https://example.com/", false, true},
		{"http://wails.localhost.example.com/", false, true},
		{"about:blank", false, true},
		{"", false, true},
		{"https://example.com/", true, false},
		{"about:blank", true, false},
	} {
		if got := panelForeignDocument(tc.source, tc.document); got != tc.foreign {
			t.Errorf("panelForeignDocument(%q, %v) = %v, want %v", tc.source, tc.document, got, tc.foreign)
		}
	}
}
