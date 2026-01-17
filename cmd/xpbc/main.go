// Command xpbc is the XPB compiler, which generates code from XPB schema files.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	xpbast "github.com/anthropic/xpb/pkg/ast"
	"github.com/anthropic/xpb/pkg/codegen/golang"
	"github.com/anthropic/xpb/pkg/codegen/typescript"
	"github.com/anthropic/xpb/pkg/parser"
)

func main() {
	var (
		lang   = flag.String("lang", "go", "Output language(s): go, ts, or comma-separated list")
		outDir = flag.String("out", ".", "Output directory")
		stdout = flag.Bool("stdout", false, "Output generated code to stdout instead of files")
		help   = flag.Bool("help", false, "Show help")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: xpbc [options] <schema.xpb>\n\n")
		fmt.Fprintf(os.Stderr, "XPB Compiler - generates code from XPB schema files.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go user.xpb          Generate Go code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=ts user.xpb          Generate TypeScript code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go,ts user.xpb       Generate both\n")
		fmt.Fprintf(os.Stderr, "  xpbc --out=./gen user.xpb        Output to ./gen directory\n")
		fmt.Fprintf(os.Stderr, "  xpbc --stdout user.xpb           Output to stdout\n")
	}

	flag.Parse()

	if *help || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(0)
	}

	schemaPath := flag.Arg(0)

	// Read schema file
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse schema
	file, err := parser.ParseFile(string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	// Generate code for each requested language
	baseName := strings.TrimSuffix(filepath.Base(schemaPath), ".xpb")
	langs := strings.Split(*lang, ",")

	for _, l := range langs {
		l = strings.TrimSpace(l)
		switch l {
		case "go", "golang":
			if err := generateGo(file, *outDir, baseName, *stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Go generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.xpb.go\n", *outDir, baseName)
			}

		case "ts", "typescript":
			if err := generateTypeScript(file, *outDir, baseName, *stdout); err != nil {
				fmt.Fprintf(os.Stderr, "TypeScript generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.xpb.ts\n", *outDir, baseName)
			}

		default:
			fmt.Fprintf(os.Stderr, "Unknown language: %s\n", l)
			os.Exit(1)
		}
	}
}

func generateGo(file *xpbast.File, outDir, baseName string, stdout bool) error {
	code, err := golang.Generate(file)
	if err != nil {
		return err
	}
	if stdout {
		os.Stdout.Write(code)
		return nil
	}
	outPath := filepath.Join(outDir, baseName+".xpb.go")
	return os.WriteFile(outPath, code, 0644)
}

func generateTypeScript(file *xpbast.File, outDir, baseName string, stdout bool) error {
	code, err := typescript.Generate(file)
	if err != nil {
		return err
	}
	if stdout {
		os.Stdout.Write(code)
		return nil
	}
	outPath := filepath.Join(outDir, baseName+".xpb.ts")
	return os.WriteFile(outPath, code, 0644)
}
