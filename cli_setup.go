package main

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"midwing/internal/clisetup"
)

// Used by the source installer after copying the production app into Applications.
func setupCLICommand(out io.Writer) error {
	manager, err := clisetup.Current()
	if err != nil {
		return err
	}
	status, err := manager.Change("ensure")
	if err != nil {
		return err
	}
	if status.Installed {
		_, err = fmt.Fprintf(out, "%s\nCLI: %s\nIf needed, add to your shell profile: %s\n", status.Message, status.Destination, status.PathCommand)
		return err
	}
	_, err = fmt.Fprintln(out, status.Message)
	return err
}

type cliSetupState struct {
	mu      sync.Mutex
	failure string
}

func (s *cliSetupState) status(manager clisetup.Manager) (clisetup.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := manager.Status()
	if s.failure != "" {
		status.SetupError = s.failure
	}
	return status, err
}

func (s *cliSetupState) change(manager clisetup.Manager, action string) (clisetup.Status, error) {
	return s.run(func() (clisetup.Status, error) { return manager.Change(action) })
}

func (s *cliSetupState) run(change func() (clisetup.Status, error)) (clisetup.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := change()
	if errors.Is(err, clisetup.ErrBusy) {
		return status, err
	}
	if err != nil {
		s.failure = err.Error()
	} else {
		s.failure = ""
	}
	return status, err
}

func (a *App) CLISetupStatus() (clisetup.Status, error) {
	manager, err := clisetup.Current()
	if err != nil {
		return clisetup.Status{}, err
	}
	return a.cliSetup.status(manager)
}

func (a *App) ChangeCLISetup(action string) (clisetup.Status, error) {
	manager, err := clisetup.Current()
	if err != nil {
		return clisetup.Status{}, err
	}
	return a.cliSetup.change(manager, action)
}

// ensureCLI links to the running executable, so CLI and app always share a version.
func (a *App) ensureCLI() error {
	manager, err := clisetup.Current()
	if err != nil {
		return err
	}
	status, _ := manager.Status()
	if !status.Available {
		return nil
	}
	_, err = a.cliSetup.run(manager.TryEnsure)
	if errors.Is(err, clisetup.ErrBusy) {
		return nil
	}
	return err
}
