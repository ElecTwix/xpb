// Command xpbc is the XPB compiler, which generates code from XPB schema files.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	xpbast "github.com/ElecTwix/xpb/pkg/ast"
	"github.com/ElecTwix/xpb/pkg/codegen/c"
	"github.com/ElecTwix/xpb/pkg/codegen/golang"
	"github.com/ElecTwix/xpb/pkg/codegen/java"
	"github.com/ElecTwix/xpb/pkg/codegen/lua"
	"github.com/ElecTwix/xpb/pkg/codegen/rust"
	"github.com/ElecTwix/xpb/pkg/codegen/typescript"
	"github.com/ElecTwix/xpb/pkg/parser"
)

func main() {
	var (
		lang            = flag.String("lang", "go", "Output language(s): go, ts, c, lua, java, rust, or comma-separated list")
		outDir          = flag.String("out", ".", "Output directory")
		stdout          = flag.Bool("stdout", false, "Output generated code to stdout instead of files")
		tsRuntimeImport = flag.String("ts-runtime-import", "", "Module specifier for the xpb runtime import in generated TypeScript (default \""+typescript.DefaultRuntimeImport+"\")")
		goOptionalStyle = flag.String("go-optional-style", golang.OptionalValue, "Go optional-field representation: value (value + Has<Field> bool, default, fewer decode allocations) or pointer (*T, opt-out)")
		goSafeBytes     = flag.Bool("go-safe-bytes", false, "Go: opt OUT of the zero-copy bytes default; decode bytes fields by COPYING (ReadBytes) so the decoded []byte owns its memory. Default (false) decodes bytes by aliasing the input buffer (zero-copy), valid only while the source buffer is alive and unmodified")
		help            = flag.Bool("help", false, "Show help")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: xpbc [options] <schema.xpb>\n\n")
		fmt.Fprintf(os.Stderr, "XPB Compiler - generates code from XPB schema files.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go user.xpb          Generate Go code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=ts user.xpb          Generate TypeScript code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=c user.xpb          Generate C code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=lua user.xpb         Generate Lua code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=java user.xpb        Generate Java code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=rust user.xpb        Generate Rust code\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go,ts user.xpb       Generate Go and TypeScript\n")
		fmt.Fprintf(os.Stderr, "  xpbc --out=./gen user.xpb        Output to ./gen directory\n")
		fmt.Fprintf(os.Stderr, "  xpbc --stdout user.xpb           Output to stdout\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=ts --ts-runtime-import=../runtime user.xpb   Vendored TS runtime import\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go user.xpb                                 Default Go: value optionals + zero-copy bytes (fast path)\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go --go-optional-style=pointer user.xpb     Opt out to pointer (*T) optionals\n")
		fmt.Fprintf(os.Stderr, "  xpbc --lang=go --go-safe-bytes user.xpb                 Opt out to copying bytes decode (decoded []byte owns its memory)\n")
	}

	flag.Parse()

	if *help || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(0)
	}

	switch *goOptionalStyle {
	case golang.OptionalPointer, golang.OptionalValue:
	default:
		fmt.Fprintf(os.Stderr, "Invalid --go-optional-style %q: want %q or %q\n", *goOptionalStyle, golang.OptionalPointer, golang.OptionalValue)
		os.Exit(1)
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

	// Create output directory
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate code for each requested language
	baseName := strings.TrimSuffix(filepath.Base(schemaPath), ".xpb")
	langs := strings.Split(*lang, ",")

	for _, l := range langs {
		l = strings.TrimSpace(l)
		switch l {
		case "go", "golang":
			goOpts := golang.Options{OptionalStyle: *goOptionalStyle, SafeBytes: *goSafeBytes}
			if err := generateGo(file, *outDir, baseName, *stdout, goOpts); err != nil {
				fmt.Fprintf(os.Stderr, "Go generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.xpb.go\n", *outDir, baseName)
			}

		case "ts", "typescript":
			if err := generateTypeScript(file, *outDir, baseName, *stdout, *tsRuntimeImport); err != nil {
				fmt.Fprintf(os.Stderr, "TypeScript generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.xpb.ts\n", *outDir, baseName)
			}

		case "c":
			if err := generateC(file, *outDir, baseName, *stdout); err != nil {
				fmt.Fprintf(os.Stderr, "C generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.h\n", *outDir, baseName)
			}

		case "lua":
			if err := generateLua(file, *outDir, baseName, *stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Lua generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.lua\n", *outDir, baseName)
			}

		case "java":
			if err := generateJava(file, *outDir, baseName, *stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Java generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.java\n", *outDir, baseName)
			}

		case "rust":
			if err := generateRust(file, *outDir, baseName, *stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Rust generation error: %v\n", err)
				os.Exit(1)
			}
			if !*stdout {
				fmt.Printf("Generated: %s/%s.xpb.rs\n", *outDir, baseName)
			}

		default:
			fmt.Fprintf(os.Stderr, "Unknown language: %s\n", l)
			os.Exit(1)
		}
	}
}

func generateGo(file *xpbast.File, outDir, baseName string, stdout bool, opts golang.Options) error {
	code, err := golang.GenerateWithOptions(file, opts)
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

func generateTypeScript(file *xpbast.File, outDir, baseName string, stdout bool, runtimeImport string) error {
	code, err := typescript.GenerateWithOptions(file, typescript.Options{RuntimeImport: runtimeImport})
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

func generateC(file *xpbast.File, outDir, baseName string, stdout bool) error {
	code, err := c.Generate(file)
	if err != nil {
		return err
	}
	if stdout {
		os.Stdout.Write(code)
		return nil
	}
	outPath := filepath.Join(outDir, baseName+".h")
	return os.WriteFile(outPath, code, 0644)
}

func generateLua(file *xpbast.File, outDir, baseName string, stdout bool) error {
	code, err := lua.Generate(file)
	if err != nil {
		return err
	}
	if stdout {
		os.Stdout.Write(code)
		return nil
	}
	outPath := filepath.Join(outDir, baseName+".lua")
	return os.WriteFile(outPath, code, 0644)
}

func generateJava(file *xpbast.File, outDir, baseName string, stdout bool) error {
	code, err := java.Generate(file)
	if err != nil {
		return err
	}
	if stdout {
		os.Stdout.Write(code)
		return nil
	}
	outPath := filepath.Join(outDir, baseName+".java")
	return os.WriteFile(outPath, code, 0644)
}

func generateRust(file *xpbast.File, outDir, baseName string, stdout bool) error {
	code, err := rust.Generate(file)
	if err != nil {
		return err
	}
	if stdout {
		os.Stdout.Write(code)
		return nil
	}
	outPath := filepath.Join(outDir, baseName+".xpb.rs")
	return os.WriteFile(outPath, code, 0644)
}
