package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"bsdrx/desktop/internal/agent"
	"bsdrx/desktop/internal/panel"
	"bsdrx/desktop/internal/windowstate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed assets/icon.png
var icon []byte

type engineLifecycle struct{ engine *agent.Agent }

func (l *engineLifecycle) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	// Return immediately so startup errors remain visible in a native window.
	go func() {
		if err := l.engine.Start(ctx); err != nil {
			log.Printf("agent: %v", err)
		}
	}()
	return nil
}

func (l *engineLifecycle) ServiceShutdown() error { l.engine.Close(); return nil }

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--help" || arg == "-h" {
			fmt.Println("bsdrX Desktop — native Wails control panel\nUsage: bsdrx-desktop [agent media/pairing options]\nUse just agent-run --help for agent options.\nThe desktop manages the web listener and closes its agent when the window closes.")
			return
		}
	}
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary := os.Getenv("BSDRX_AGENT")
	if binary == "" {
		binary = filepath.Join(filepath.Dir(executable), "bsdr_agent")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	stateDir := os.Getenv("BSDRX_DATA_DIR")
	if stateDir == "" {
		stateDir = filepath.Join(filepath.Dir(filepath.Dir(executable)), "build-local")
	}
	stateDir, err = filepath.Abs(stateDir)
	if err != nil {
		return err
	}
	if err := agent.ValidateArgs(os.Args[1:]); err != nil {
		return err
	}
	// WebKitGTK's default network session uses GLib's XDG directories. Set
	// these before Wails initialises GTK so its own browser profile is local,
	// alongside the C agent's settings and caches.
	if runtime.GOOS == "linux" {
		for name, dir := range map[string]string{
			"XDG_CACHE_HOME": filepath.Join(stateDir, "cache"),
			"XDG_DATA_HOME":  filepath.Join(stateDir, "data"),
		} {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return err
			}
			if err := os.Setenv(name, dir); err != nil {
				return err
			}
		}
	}
	engine := agent.New(agent.Config{Binary: binary, StateDir: stateDir, Args: os.Args[1:]})
	defer engine.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name: "bsdrX", Description: "Screen and audio casting for Bigscreen VR", Icon: icon,
		Assets:     application.AssetOptions{Handler: panel.New(ctx, engine), DisableLogging: true},
		Services:   []application.Service{application.NewService(&engineLifecycle{engine})},
		OnShutdown: cancel,
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Linux:      application.LinuxOptions{ProgramName: "bsdrx-desktop"},
		Windows:    application.WindowsOptions{WebviewUserDataPath: filepath.Join(stateDir, "data", "webview2")},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "net.nexlab.bsdrx.desktop",
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
	})
	statePath := windowstate.Path(filepath.Join(stateDir, "desktop"))
	saved, found, err := windowstate.Load(statePath)
	if err != nil {
		log.Printf("window state: %v", err)
	}
	minFrame := windowstate.Frame{Width: 780, Height: 560}
	plan := windowstate.PlanRestore(saved, found, windowstate.Frame{Width: 1180, Height: 850}, minFrame)
	options := application.WebviewWindowOptions{
		Title: "bsdrX", Width: plan.Frame.Width, Height: plan.Frame.Height,
		MinWidth: minFrame.Width, MinHeight: minFrame.Height,
		BackgroundColour: application.NewRGB(12, 16, 24), URL: "/",
	}
	if plan.UsePosition {
		options.X, options.Y, options.InitialPosition = plan.Frame.X, plan.Frame.Y, application.WindowXY
	}
	if plan.Maximised {
		options.StartState = application.WindowStateMaximised
	}
	window = app.Window.NewWithOptions(options)
	if plan.UsePosition {
		windowstate.ReconcileWhenShown(window, minFrame)
	}
	windowstate.Attach(window, statePath, minFrame, 500*time.Millisecond)
	return app.Run()
}
