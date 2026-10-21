package main

type cliVault map[string]string

func (v cliVault) Set(ref, token string) error { v[ref] = token; return nil }

func (v cliVault) Get(ref string) (string, error) { return v[ref], nil }

func (v cliVault) Delete(ref string) error { delete(v, ref); return nil }
