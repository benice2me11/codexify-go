// mcptrace analyzes one local metadata capture without contacting a connector.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/benice2me11/codexify-go/internal/mcpdiag"
)

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("mcptrace", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("log", "", "one metadata JSONL capture")
	groups := flags.Int("max-groups", 20, "maximum operation groups in the report (1..100)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: mcptrace -log <capture.jsonl> [-max-groups 20]")
	}
	file, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer file.Close()
	report, err := mcpdiag.Analyze(file, *groups)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "mcptrace:", err)
		os.Exit(1)
	}
}
