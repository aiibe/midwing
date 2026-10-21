package main

import "midwing/internal/clisetup"

func (a *App) CLISetupStatus() (clisetup.Status, error) {
	manager, err := clisetup.Current()
	if err != nil {
		return clisetup.Status{}, err
	}
	return manager.Status()
}

func (a *App) ChangeCLISetup(action string) (clisetup.Status, error) {
	manager, err := clisetup.Current()
	if err != nil {
		return clisetup.Status{}, err
	}
	return manager.Change(action)
}
