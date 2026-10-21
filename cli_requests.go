package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"midwing/internal/midwing"
)

func runRequestCLI(args []string, store *midwing.Store, out io.Writer) error {
	usage := "midwing " + args[0] + " SERVICE PATH"
	help := "Call an enabled GET endpoint; quote paths containing queries. Returns one JSON page."
	wantArgs := 3
	if args[0] == "post" {
		usage += " JSON"
		help = "Call an enabled POST endpoint with one JSON body argument (max 1 MiB). Quote paths containing queries and JSON bodies containing spaces."
		wantArgs = 4
	}
	if len(args) == 2 && args[1] == "--help" {
		_, err := fmt.Fprintln(out, usage+"\n"+help)
		return err
	}
	if len(args) != wantArgs {
		return fmt.Errorf("usage: %s; run midwing %s --help", usage, args[0])
	}
	var body json.RawMessage
	if args[0] == "post" {
		body = json.RawMessage(args[3])
	}
	v, err := (&midwing.Client{Store: store}).CallWithBody(context.Background(), args[1], strings.ToUpper(args[0]), args[2], body)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(v))
	return err
}
