package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"midwing/internal/midwing"
)

func runRequestCLI(args []string, store *midwing.Store, out io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runRequestCLIContext(ctx, args, store, out)
}

func runRequestCLIContext(ctx context.Context, args []string, store *midwing.Store, out io.Writer) error {
	usage := "midwing " + args[0] + " SERVICE PATH"
	help := "Call an enabled GET endpoint; quote paths containing queries."
	wantArgs := 3
	if args[0] == "post" {
		usage += " JSON"
		help = "Call an enabled POST endpoint with one JSON body argument (max 1 MiB). Quote paths containing queries and JSON bodies containing spaces."
		wantArgs = 4
	}
	usage += " [--output FILE] [--accept TYPE] [--max-bytes N]"
	if len(args) == 2 && args[1] == "--help" {
		_, err := fmt.Fprintln(out, usage+"\n"+help+"\nReturns JSON; empty successful responses print nothing. --output saves checked raw bytes to a new private file. Downloads default to any media type and 8 MiB; --accept and --max-bytes require --output. Responses are buffered in memory.")
		return err
	}
	usageError := func() error { return fmt.Errorf("usage: %s; run midwing %s --help", usage, args[0]) }
	if len(args) < wantArgs {
		return usageError()
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outputPath := flags.String("output", "", "destination file")
	accept := flags.String("accept", "", "response media type")
	maxBytes := flags.Int64("max-bytes", midwing.DefaultMaxResponseBytes, "response limit in bytes")
	if err := flags.Parse(args[wantArgs:]); err != nil || flags.NArg() != 0 {
		return usageError()
	}
	if (flags.NFlag() != 0 && *outputPath == "") || *maxBytes < 1 || *maxBytes == 1<<63-1 {
		return usageError()
	}
	var body json.RawMessage
	if args[0] == "post" {
		body = json.RawMessage(args[3])
	}
	client := &midwing.Client{Store: store}
	method := strings.ToUpper(args[0])
	if *outputPath != "" {
		err := writeResponseFile(*outputPath, func(file *os.File) error {
			return client.CallWithResponse(ctx, args[1], method, args[2], body, midwing.ResponseOptions{Accept: *accept, MaxBytes: *maxBytes}, func(response []byte) error {
				_, err := io.Copy(file, bytes.NewReader(response))
				return errors.Join(err, file.Close())
			})
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Saved response to %s\n", *outputPath)
		return err
	}
	return client.CallJSONTo(ctx, args[1], method, args[2], body, out)
}

func writeResponseFile(path string, write func(*os.File) error) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("could not create output file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, fmt.Errorf("could not finish output file: %w", closeErr))
		}
		if err != nil {
			if removeErr := os.Remove(path); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("could not remove failed output file %q: %w", path, removeErr))
			}
		}
	}()
	return write(file)
}
