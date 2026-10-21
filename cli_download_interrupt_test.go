package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"midwing/internal/midwing"
)

func TestInterruptedDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix process signals")
	}
	if path := os.Getenv("MIDWING_TEST_DOWNLOAD_PATH"); path != "" {
		store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
		d := midwing.CustomService{ID: "files", Name: "Files", BaseURL: "https://files.example.test", AuthKind: "token", Header: "X-API-Key", Endpoints: []midwing.CustomEndpoint{{Path: "/file", Methods: []string{"GET", "POST"}}}}
		if err := store.CreateCustomService(d); err != nil {
			t.Fatal(err)
		}
		if err := saveTestService(store, d.ID, "secret-token", []string{"endpoint-1", "endpoint-1-post"}); err != nil {
			t.Fatal(err)
		}
		http.DefaultTransport = requestTransport(func(r *http.Request) (*http.Response, error) {
			fmt.Fprintln(os.Stdout, "ready")
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		method := os.Getenv("MIDWING_TEST_DOWNLOAD_METHOD")
		args := []string{method, d.ID, "/file"}
		if method == "post" {
			args = append(args, `{}`)
		}
		var out bytes.Buffer
		if err := runRequestCLI(append(args, "--output", path), store, &out); err == nil {
			t.Fatal("interrupted request succeeded")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("interrupted request left an output file")
		}
		if out.Len() != 0 {
			t.Fatal("interrupted request printed success")
		}
		history, err := store.History()
		if err != nil || len(history) != 1 || history[0].Outcome != "failed" {
			t.Fatalf("unexpected history: %+v (%v)", history, err)
		}
		return
	}
	for _, method := range []string{"get", "post"} {
		for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
			t.Run(method+"/"+signal.String(), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				path := filepath.Join(t.TempDir(), "download")
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInterruptedDownload$")
				cmd.Env = append(os.Environ(), "MIDWING_TEST_DOWNLOAD_PATH="+path, "MIDWING_TEST_DOWNLOAD_METHOD="+method)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				ready, readErr := bufio.NewReader(stdout).ReadString('\n')
				if readErr != nil || ready != "ready\n" {
					cancel()
					_ = cmd.Wait()
					t.Fatalf("child not ready: %q (%v): %s", ready, readErr, stderr.String())
				}
				if err := cmd.Process.Signal(signal); err != nil {
					cancel()
					_ = cmd.Wait()
					t.Fatal(err)
				}
				remaining, readErr := io.ReadAll(stdout)
				if err := cmd.Wait(); err != nil || readErr != nil {
					t.Fatalf("interrupted download failed: %v (%v)\n%s%s", err, readErr, remaining, stderr.String())
				}
			})
		}
	}
}
