package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"io"
	"midwing/internal/midwing"
	"os"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed wails.json
var buildConfig []byte

// Release builds override this for the repository running the workflow.
var releaseRepository = "aiibe/midwing"

func buildVersion() string {
	var config struct {
		Info struct {
			Version string `json:"productVersion"`
		} `json:"info"`
	}
	_ = json.Unmarshal(buildConfig, &config)
	return config.Info.Version
}

func newUpdateChecker() *midwing.UpdateChecker {
	return &midwing.UpdateChecker{Repository: releaseRepository, Version: buildVersion()}
}

type App struct {
	store        *midwing.Store
	ctx          context.Context
	stopWatching func()
	updates      *midwing.UpdateChecker
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	stop, err := a.store.WatchSettings(ctx, func() { runtime.EventsEmit(ctx, "settings-changed") })
	if err != nil {
		runtime.LogWarning(ctx, "Settings watcher unavailable; refresh on focus remains enabled.")
		return
	}
	a.stopWatching = stop
}
func (a *App) shutdown(context.Context) {
	if a.stopWatching != nil {
		a.stopWatching()
	}
}
func (a *App) Version() string { return buildVersion() }

func (a *App) CheckForUpdate() (midwing.UpdateInfo, error) {
	if a.ctx == nil || a.updates == nil {
		return midwing.UpdateInfo{}, nil
	}
	return a.updates.Check(a.ctx)
}

func (a *App) State() (midwing.DesktopState, error) { return a.store.Snapshot() }
func (a *App) AgentPrompt() (string, error)         { return a.store.AgentPrompt() }
func (a *App) CopyAgentPrompt() error {
	prompt, e := a.store.AgentPrompt()
	if e != nil {
		return e
	}
	if a.ctx == nil {
		return fmt.Errorf("desktop app is not ready")
	}
	return runtime.ClipboardSetText(a.ctx, prompt)
}

func (a *App) CreateCustomService(definition midwing.CustomService) error {
	return a.store.CreateCustomService(definition)
}
func (a *App) DeleteCustomService(id string) (midwing.ChangeResult, error) {
	return changeResult(a.store.DeleteCustomService(id))
}

func (a *App) UpdatePermissionDescriptions(id string, descriptions map[string]string) (midwing.ChangeResult, error) {
	return changeResult(a.store.UpdatePermissionDescriptions(id, descriptions))
}
func (a *App) SaveConnection(service, token string, permissions []string) (midwing.ChangeResult, error) {
	return changeResult(a.store.SaveService(service, token, permissions))
}
func (a *App) ConnectPassword(service, email, password string, permissions []string) (midwing.ChangeResult, error) {
	if a.ctx == nil {
		return midwing.ChangeResult{}, fmt.Errorf("desktop app is not ready")
	}
	if err := a.ctx.Err(); err != nil {
		return midwing.ChangeResult{}, err
	}
	return changeResult((&midwing.Client{Store: a.store}).ConnectPassword(a.ctx, service, email, password, permissions))
}
func (a *App) RemoveConnection(service string) (midwing.ChangeResult, error) {
	return changeResult(a.store.RemoveService(service))
}
func (a *App) Activity() ([]midwing.Activity, error) { return a.store.History() }
func changeResult(err error) (midwing.ChangeResult, error) {
	var committed *midwing.CommittedChangeError
	if errors.As(err, &committed) {
		return midwing.ChangeResult{Warning: committed.Error()}, nil
	}
	return midwing.ChangeResult{}, err
}

func runCLI(args []string, store *midwing.Store, out io.Writer) error {
	return runCLIWithInput(args, store, os.Stdin, out)
}

func runCLIWithInput(args []string, store *midwing.Store, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		_, err := fmt.Fprintln(out, cliHelp)
		return err
	}
	if len(args) > 0 && args[0] == "services" {
		return runServicesCLI(args[1:], store, in, out)
	}
	if len(args) == 2 && args[0] == "get" && args[1] == "--help" {
		_, err := fmt.Fprintln(out, "midwing get SERVICE PATH\nCall an enabled GET endpoint; quote paths containing queries. Returns one JSON page.")
		return err
	}
	if len(args) == 3 && args[0] == "get" {
		v, err := (&midwing.Client{Store: store}).Call(context.Background(), args[1], "GET", args[2])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(v))
		return err
	}
	return fmt.Errorf("unknown command; run midwing --help")
}
func main() {
	store, e := midwing.NewStore()
	if e == nil {
		if len(os.Args) > 1 {
			e = runCLI(os.Args[1:], store, os.Stdout)
		} else {
			app := &App{store: store, updates: newUpdateChecker()}
			e = wails.Run(&options.App{OnStartup: app.startup, OnShutdown: app.shutdown, Title: "Midwing", Width: 1080, Height: 760, MinWidth: 780, MinHeight: 600, BackgroundColour: &options.RGBA{R: 14, G: 17, B: 23, A: 255}, AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{app}})
		}
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "midwing:", e)
		os.Exit(1)
	}
}
