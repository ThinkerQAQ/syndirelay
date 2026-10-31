package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	blogcompiler "github.com/ThinkerQAQ/syndirelay/compiler"
)

func (a app) runCompileFile(args []string) error {
	options := blogcompiler.FileCompileOptions{}
	flags := flag.NewFlagSet("compile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&options.Path, "file", "", "Markdown file to compile")
	flags.StringVar(&options.Platform, "platform", "", "target platform ID")
	flags.StringVar(&options.CanonicalURL, "canonical-url", "", "optional HTTPS canonical URL")
	flags.StringVar(&options.AssetBaseURL, "asset-base-url", "", "optional HTTPS asset prefix for diagrams and generated images")
	flags.StringVar(&options.Language, "language", "", "en or zh-CN (optional)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if options.Path == "" || options.Platform == "" {
		return errors.New("usage: syndirelay compile --file <markdown> --platform <platform> [--canonical-url <https-url>] [--asset-base-url <https-url>]")
	}
	compiled, err := blogcompiler.CompileFile(context.Background(), options)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(compiled, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compiled content: %w", err)
	}
	_, err = fmt.Fprintln(a.out, string(encoded))
	return err
}
