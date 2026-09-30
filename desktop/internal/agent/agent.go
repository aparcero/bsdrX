// Package agent manages the existing C streaming agent for the desktop window.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Binary         string
	StateDir       string
	Args           []string
	StartupTimeout time.Duration
	StopTimeout    time.Duration
}

type State struct {
	Ready    bool   `json:"ready"`
	Starting bool   `json:"starting"`
	Message  string `json:"message"`
	LogPath  string `json:"logPath"`
}

type Agent struct {
	cfg    Config
	op     sync.Mutex // serialises start/retry/close
	mu     sync.RWMutex
	state  State
	target *url.URL
	cmd    *exec.Cmd
	done   chan struct{}
	closed bool
}

func New(cfg Config) *Agent {
	if cfg.StartupTimeout == 0 {
		cfg.StartupTimeout = 15 * time.Second
	}
	if cfg.StopTimeout == 0 {
		cfg.StopTimeout = 8 * time.Second
	}
	return &Agent{cfg: cfg, state: State{Starting: true, LogPath: filepath.Join(cfg.StateDir, "logs", "agent.log")}}
}

func (a *Agent) State() State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *Agent) Target() (*url.URL, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.state.Ready || a.target == nil {
		return nil, false
	}
	target := *a.target
	return &target, true
}

func (a *Agent) fail(err error) error {
	a.mu.Lock()
	a.state.Ready, a.state.Starting, a.state.Message = false, false, err.Error()
	a.mu.Unlock()
	return err
}

// Start returns only when the child's API is ready or startup failed. It never
// attaches to or terminates an independently running bsdr_agent.
func (a *Agent) Start(ctx context.Context) error {
	a.op.Lock()
	defer a.op.Unlock()
	if a.closed {
		return errors.New("desktop is closing")
	}
	if a.State().Ready {
		return nil
	}
	if err := ValidateArgs(a.cfg.Args); err != nil {
		return a.fail(err)
	}
	a.mu.Lock()
	a.state.Starting, a.state.Message = true, ""
	a.mu.Unlock()
	for _, dir := range []string{"config", "cache/bsdrX/models", "logs"} {
		if err := os.MkdirAll(filepath.Join(a.cfg.StateDir, dir), 0700); err != nil {
			return a.fail(err)
		}
	}
	logFile, err := os.OpenFile(a.State().LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return a.fail(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		logFile.Close()
		return a.fail(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	args := append([]string{}, a.cfg.Args...)
	args = append(args, "--no-browser", "--web-bind", "127.0.0.1", "--web-port", strconv.Itoa(port))
	cmd := exec.Command(a.cfg.Binary, args...)
	// Inherit the caller's working directory so relative media paths keep the
	// same meaning as in the standalone CLI. Plugin/state paths are absolute.
	cmd.Env = environment(os.Environ(), map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(a.cfg.StateDir, "config"),
		"XDG_CACHE_HOME":  filepath.Join(a.cfg.StateDir, "cache"),
		"BSDR_MODEL_DIR":  filepath.Join(a.cfg.StateDir, "cache", "bsdrX", "models"),
		"BSDR_PLUGIN_DIR": filepath.Join(filepath.Dir(a.cfg.Binary), "plugins"),
	})
	cmd.Stdout, cmd.Stderr = logFile, logFile
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return a.fail(fmt.Errorf("start streaming agent: %w", err))
	}
	done := make(chan struct{})
	a.mu.Lock()
	a.cmd, a.done, a.target = cmd, done, target
	a.mu.Unlock()
	go func() {
		err := cmd.Wait()
		logFile.Close()
		a.mu.Lock()
		a.state.Ready, a.state.Starting = false, false
		if err != nil {
			a.state.Message = "Streaming agent exited: " + err.Error()
		} else {
			a.state.Message = "Streaming agent stopped."
		}
		a.mu.Unlock()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(ctx, a.cfg.StartupTimeout)
	defer cancel()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.stop(cmd, done)
			return a.fail(fmt.Errorf("streaming agent did not become ready: %w", ctx.Err()))
		case <-done:
			return errors.New(a.State().Message)
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.String()+"/api/status", nil)
			response, err := client.Do(req)
			if err != nil {
				continue
			}
			var status struct {
				Quest *json.RawMessage `json:"quest"`
			}
			err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status)
			response.Body.Close()
			if response.StatusCode != http.StatusOK || err != nil || status.Quest == nil {
				continue
			}
			a.mu.Lock()
			if a.state.Starting {
				a.state.Ready, a.state.Starting = true, false
			}
			ready := a.state.Ready
			a.mu.Unlock()
			if ready {
				return nil
			}
		}
	}
}

func (a *Agent) stop(cmd *exec.Cmd, done <-chan struct{}) {
	if cmd == nil {
		return
	}
	select {
	case <-done:
		return
	default:
	}
	if err := interrupt(cmd); err != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(a.cfg.StopTimeout):
		_ = cmd.Process.Kill()
		<-done
	}
}

func (a *Agent) Close() {
	a.op.Lock()
	defer a.op.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	a.mu.RLock()
	cmd, done := a.cmd, a.done
	a.mu.RUnlock()
	a.stop(cmd, done)
}

// The desktop owns the control listener and window, so contradictory agent
// flags fail explicitly. All media and pairing flags pass through unchanged.
func ValidateArgs(args []string) error {
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		switch name {
		case "--no-ui", "--browser", "--web-port", "--web-bind", "--web-allow", "--sniff-helper":
			return fmt.Errorf("%s is managed by the desktop app; use just agent-run for the standalone agent", name)
		}
	}
	return nil
}

func environment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[key]; !replaced {
			result = append(result, item)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}
