package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAgentProcess is a real subprocess with the agent's HTTP/signal contract.
// This exercises process ownership and teardown without requiring a headset.
func TestAgentProcess(t *testing.T) {
	mode := os.Getenv("BSDRX_AGENT_TEST")
	if mode == "" {
		return
	}
	if mode == "exit" {
		os.Exit(23)
	}
	port := ""
	for i, arg := range os.Args {
		if arg == "--web-port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	if port == "" {
		os.Exit(24)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if mode == "unready" {
			_, _ = w.Write([]byte(`{"wrong":"service"}`))
			return
		}
		cwd, _ := os.Getwd()
		_ = json.NewEncoder(w).Encode(map[string]any{"quest": map[string]any{}, "config": os.Getenv("XDG_CONFIG_HOME"), "models": os.Getenv("BSDR_MODEL_DIR"), "cwd": cwd})
	})
	server := &http.Server{Addr: "127.0.0.1:" + port, Handler: mux}
	go func() { _ = server.ListenAndServe() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	if mode == "unready" {
		select {}
	} // force the supervisor's bounded kill fallback
	_ = server.Close()
	os.Exit(0)
}

func testConfig(t *testing.T, mode string) Config {
	t.Helper()
	t.Setenv("BSDRX_AGENT_TEST", mode)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Config{Binary: executable, StateDir: filepath.Join(t.TempDir(), "local state"), Args: []string{"-test.run=^TestAgentProcess$", "--"}, StartupTimeout: 3 * time.Second, StopTimeout: 100 * time.Millisecond}
}

func TestLifecycleAndLocalState(t *testing.T) {
	cfg := testConfig(t, "ready")
	a := New(cfg)
	t.Cleanup(a.Close)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	target, ready := a.Target()
	if !ready {
		t.Fatal("agent not ready")
	}
	response, err := http.Get(target.String() + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["config"] != filepath.Join(cfg.StateDir, "config") {
		t.Fatalf("wrong config directory: %v", body)
	}
	if body["models"] != filepath.Join(cfg.StateDir, "cache", "bsdrX", "models") {
		t.Fatalf("wrong model directory: %v", body)
	}
	cwd, err := os.Getwd()
	if err != nil || body["cwd"] != cwd {
		t.Fatalf("agent changed the working directory: got %v, want %q (%v)", body["cwd"], cwd, err)
	}
	firstPID := a.cmd.Process.Pid
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.cmd.Process.Pid != firstPID {
		t.Fatal("duplicate child spawned")
	}
	a.Close()
	a.Close()
	if a.cmd.ProcessState == nil {
		t.Fatal("child not reaped")
	}
	if _, ready := a.Target(); ready {
		t.Fatal("closed agent still ready")
	}
	if err := a.Start(context.Background()); err == nil {
		t.Fatal("closed agent restarted")
	}
}

func TestCrashCanRetry(t *testing.T) {
	a := New(testConfig(t, "ready"))
	t.Cleanup(a.Close)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	pid := a.cmd.Process.Pid
	if err := a.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.done:
	case <-time.After(3 * time.Second):
		t.Fatal("child was not reaped")
	}
	if a.State().Ready || a.State().Message == "" {
		t.Fatal("missing crash state")
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pid == a.cmd.Process.Pid {
		t.Fatal("child not replaced")
	}
}

func TestStartupFailureAndTimeout(t *testing.T) {
	for _, mode := range []string{"exit", "unready", "missing"} {
		t.Run(mode, func(t *testing.T) {
			cfg := testConfig(t, mode)
			cfg.StartupTimeout = 500 * time.Millisecond
			if mode == "missing" {
				cfg.Binary = filepath.Join(t.TempDir(), "missing-agent")
			}
			a := New(cfg)
			t.Cleanup(a.Close)
			if err := a.Start(context.Background()); err == nil {
				t.Fatal("expected startup error")
			}
			if a.State().Ready || a.State().Starting || a.State().Message == "" {
				t.Fatalf("wrong failure state: %+v", a.State())
			}
			if a.cmd != nil {
				select {
				case <-a.done:
				default:
					t.Fatal("failed child left running")
				}
			}
		})
	}
}

func TestManagedFlags(t *testing.T) {
	for _, arg := range []string{"--web-port", "--web-bind=0.0.0.0", "--web-allow=*", "--browser", "--no-ui", "--sniff-helper"} {
		if err := ValidateArgs([]string{arg}); err == nil {
			t.Errorf("accepted %s", arg)
		}
	}
	if err := ValidateArgs([]string{"--control-only", "--file", "/tmp/video with spaces.mp4", "--no-browser"}); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentReplacesInheritedPaths(t *testing.T) {
	result := environment([]string{"XDG_CONFIG_HOME=/user", "PATH=/bin", "XDG_CONFIG_HOME=/other"}, map[string]string{"XDG_CONFIG_HOME": "/repo/config"})
	if strings.Join(result, "\n") != "PATH=/bin\nXDG_CONFIG_HOME=/repo/config" {
		t.Fatalf("unexpected environment: %v", result)
	}
}
