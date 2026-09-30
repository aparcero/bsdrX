// Package panel serves the existing C control panel inside the native WebView.
package panel

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bsdrx/desktop/internal/agent"
)

//go:embed offline.html
var offlineHTML string

var offline = template.Must(template.New("offline").Parse(offlineHTML))

type Engine interface {
	State() agent.State
	Target() (*url.URL, bool)
	Start(context.Context) error
}

type Handler struct {
	engine    Engine
	ctx       context.Context
	transport *http.Transport
}

func New(ctx context.Context, engine Engine) *Handler {
	return &Handler{engine: engine, ctx: ctx, transport: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 30 * time.Second}}
}

// Only Wails' local asset origins may be rewritten for the loopback C API.
// This also rejects foreign origins in development HTTP tests/server builds.
func trusted(r *http.Request) bool {
	if r.Host != "localhost" && r.Host != "wails.localhost" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == "wails://localhost" || origin == "http://wails.localhost"
	}
	return true
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !trusted(r) {
		http.Error(w, "Forbidden origin", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/_desktop/status":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.engine.State())
		return
	case "/_desktop/retry":
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		_ = h.engine.Start(h.ctx)
		// Native custom-scheme adapters do not consistently follow HTTP
		// redirects. The recovery page navigates after this request completes.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.engine.State())
		return
	}
	target, ready := h.engine.Target()
	if !ready {
		h.unavailable(w, r)
		return
	}
	// Wails' native request adapters may not supply a content length. The C
	// server consumes Content-Length bodies and does not decode chunked input.
	if r.Body != nil {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		r.Body.Close()
		if err != nil {
			http.Error(w, "Request body too large or unreadable", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.TransferEncoding = nil
	}
	proxy := &httputil.ReverseProxy{
		Transport: h.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			if pr.Out.Header.Get("Origin") != "" {
				pr.Out.Header.Set("Origin", target.String())
			}
			pr.Out.Header.Del("Accept-Encoding")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) { h.unavailable(w, r) },
		ModifyResponse: func(response *http.Response) error {
			if r.URL.Path != "/" || !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
				return nil
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				return err
			}
			// If the engine exits after loading, replace the stale controls with
			// the actionable error page on the next heartbeat.
			watch := `<script type="module" src="/wails/runtime.js"></script><script>setInterval(()=>fetch('/_desktop/status').then(r=>r.json()).then(s=>{if(!s.ready)location.reload()}).catch(()=>{}),1500)</script>`
			body = bytes.Replace(body, []byte("</body>"), []byte(watch+"</body>"), 1)
			response.Body = io.NopCloser(bytes.NewReader(body))
			response.ContentLength = int64(len(body))
			response.Header.Set("Content-Length", strconv.Itoa(len(body)))
			return nil
		},
	}
	proxy.ServeHTTP(w, r)
}

func (h *Handler) unavailable(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Streaming agent unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = offline.Execute(w, h.engine.State())
}
