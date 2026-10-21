package main

import "midwing/internal/midwing"

func saveTestService(store *midwing.Store, id, token string, permissions []string) error {
	service, err := store.Service(id)
	if err != nil {
		return err
	}
	return store.SaveService(id, service.DefinitionHash, token, permissions)
}

type cliVault map[string]string

func (v cliVault) Set(ref, token string) error { v[ref] = token; return nil }

func (v cliVault) Get(ref string) (string, error) { return v[ref], nil }

func (v cliVault) Delete(ref string) error { delete(v, ref); return nil }
