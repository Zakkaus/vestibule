package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/Zakkaus/vestibule/migrations"
)

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("schema-manifest", flag.ContinueOnError)
	output := flags.String("output", "", "write the manifest to this file instead of standard output")
	check := flags.String("check", "", "verify this manifest matches migrations.Table")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("schema-manifest accepts no positional arguments")
	}
	if *output != "" && *check != "" {
		return fmt.Errorf("schema-manifest accepts only one of -output or -check")
	}

	manifest, err := migrations.CurrentSchemaManifest()
	if err != nil {
		return err
	}
	text := []byte(manifest.String())

	if *check != "" {
		current, err := os.ReadFile(*check)
		if err != nil {
			return fmt.Errorf("read schema manifest %s: %w", *check, err)
		}
		if !bytes.Equal(current, text) {
			return fmt.Errorf(
				"schema manifest %s does not match migrations.Table; regenerate with: go run ./cmd/schema-manifest -output %s",
				*check,
				*check,
			)
		}
		return nil
	}

	if *output != "" {
		if err := os.WriteFile(*output, text, 0o644); err != nil { // #nosec G306 -- schema manifests are public release metadata.
			return fmt.Errorf("write schema manifest %s: %w", *output, err)
		}
		return nil
	}
	_, err = stdout.Write(text)
	return err
}
