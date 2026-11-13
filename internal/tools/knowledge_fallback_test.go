package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWikipediaFallbackSearch(t *testing.T) {
	var summaryCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/summary/"):
			if atomic.AddInt32(&summaryCalls, 1) == 1 {
				w.WriteHeader(http.StatusNotFound) // exact title missing
				return
			}
			fmt.Fprint(w, `{"title":"Tea","extract":"Tea is brewed leaves.","content_urls":{"desktop":{"page":"https://wiki/Tea"}}}`)
		case r.URL.Path == "/w/api.php":
			fmt.Fprint(w, `["tea",["Tea (drink)"],["a beverage"],["https://wiki/Tea_(drink)"]]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	summary, err := wikipediaSummary(context.Background(), clientFor(srv), "en", "Tea")
	if err != nil || summary.Title != "Tea" || summary.URL == "" {
		t.Fatalf("fallback summary: %v %+v", err, summary)
	}
	if atomic.LoadInt32(&summaryCalls) < 2 {
		t.Fatal("fallback path not exercised")
	}
	// No results at all => the original error is returned.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv2.Close()
	if _, err := wikipediaSummary(context.Background(), clientFor(srv2), "en", "Nothing"); err == nil {
		t.Fatal("missing topic should error")
	}
}
