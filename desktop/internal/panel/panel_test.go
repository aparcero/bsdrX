package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bsdrx/desktop/internal/agent"
)

type fakeEngine struct {
	state   agent.State
	target  *url.URL
	retries int
}

func (e *fakeEngine) State() agent.State          { return e.state }
func (e *fakeEngine) Target() (*url.URL, bool)    { return e.target, e.state.Ready }
func (e *fakeEngine) Start(context.Context) error { e.retries++; return nil }

func request(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Origin", "wails://localhost")
	return r
}

func TestForwardsAPIAndPreservesPanel(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "http://"+r.Host {
			t.Errorf("C origin guard would reject: %s", r.Header.Get("Origin"))
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html><body>All existing controls</body></html>")
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.ContentLength != int64(len(body)) || len(r.TransferEncoding) != 0 {
			t.Error("C server cannot decode chunked request bodies")
		}
		if string(body) != `{"bitrate":5000}` || r.URL.RawQuery != "source=desktop" || r.Method != "POST" {
			t.Errorf("request changed: %s %s %s", r.Method, r.URL, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer backend.Close()
	target, _ := url.Parse(backend.URL)
	handler := New(context.Background(), &fakeEngine{state: agent.State{Ready: true}, target: target})
	for _, path := range []string{"/", "/api/bitrate?source=desktop"} {
		w := httptest.NewRecorder()
		r := request("POST", path, `{"bitrate":5000}`)
		r.ContentLength = -1 // native WebView adapters can provide streaming bodies
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if path == "/" && (!strings.Contains(w.Body.String(), "All existing controls") || !strings.Contains(w.Body.String(), "/_desktop/status")) {
			t.Fatal("panel or crash detection missing")
		}
		if path == "/" && !strings.Contains(w.Body.String(), `src="/wails/runtime.js"`) {
			t.Fatal("native Wails runtime must load for events and WebView inspection")
		}
	}
}

func TestRejectsForeignOriginsBeforeForwarding(t *testing.T) {
	engine := &fakeEngine{}
	handler := New(context.Background(), engine)
	for _, origin := range []string{"https://evil.example", "http://localhost.evil.example", "null"} {
		r := request("POST", "/_desktop/retry", "")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("allowed %q", origin)
		}
	}
	if engine.retries != 0 {
		t.Fatal("untrusted request restarted agent")
	}
	r := request("GET", "/", "")
	r.Host = "localhost.evil.example"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("untrusted Host accepted")
	}
}

func TestStartupAndErrorPages(t *testing.T) {
	engine := &fakeEngine{state: agent.State{Starting: true}}
	handler := New(context.Background(), engine)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "/", ""))
	if !strings.Contains(w.Body.String(), "Starting your desktop connection") {
		t.Fatal(w.Body.String())
	}
	engine.state = agent.State{Message: "missing <agent>", LogPath: "/repo/logs/agent.log"}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "/", ""))
	if !strings.Contains(w.Body.String(), "missing &lt;agent&gt;") || !strings.Contains(w.Body.String(), "Try again") {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("POST", "/_desktop/retry", ""))
	var retryState agent.State
	if engine.retries != 1 || w.Code != http.StatusOK || w.Header().Get("Location") != "" || json.Unmarshal(w.Body.Bytes(), &retryState) != nil {
		t.Fatal("retry failed")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request("GET", "/api/status", ""))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("dead API pretended to be ready")
	}
}
