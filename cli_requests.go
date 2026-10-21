package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"midwing/internal/midwing"
)

func runRequestCLI(args []string, store *midwing.Store, out io.Writer) error {
	usage := "midwing " + args[0] + " SERVICE PATH"
	help := "Call an enabled GET endpoint; quote paths containing queries. Returns one JSON page."
	wantArgs := 3
	if args[0] == "post" {
		usage += " JSON [--output FILE]"
		help = "Call an enabled POST endpoint with one JSON body argument (max 1 MiB). Quote paths containing queries and JSON bodies containing spaces."
		wantArgs = 4
	}
	if len(args) == 2 && args[1] == "--help" {
		if args[0] == "post" {
			help += " Use --output FILE to save a non-JSON response such as audio bytes."
		}
		_, err := fmt.Fprintln(out, usage+"\n"+help)
		return err
	}
	outputPath := ""
	if args[0] == "post" && len(args) == 6 && args[4] == "--output" {
		outputPath = args[5]
	}
	if len(args) != wantArgs && outputPath == "" {
		return fmt.Errorf("usage: %s; run midwing %s --help", usage, args[0])
	}
	var body json.RawMessage
	if args[0] == "post" {
		body = json.RawMessage(args[3])
	}
	client := &midwing.Client{Store: store}
	if outputPath != "" {
		file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("could not create output file: %w", err)
		}
		if err = client.CallWithBodyTo(context.Background(), args[1], args[2], body, file); err != nil {
			_ = file.Close()
			_ = os.Remove(outputPath)
			return err
		}
		if err = file.Close(); err != nil {
			_ = os.Remove(outputPath)
			return fmt.Errorf("could not finish output file: %w", err)
		}
		_, err = fmt.Fprintf(out, "Saved response to %s\n", outputPath)
		return err
	}
	v, err := client.CallWithBody(context.Background(), args[1], strings.ToUpper(args[0]), args[2], body)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(v))
	return err
}
