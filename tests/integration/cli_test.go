// Package integration contains end-to-end tests for the CLI (xpbc).
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_GenerateGo(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	outputDir := filepath.Join(tmpDir, "generated")

	// Write schema
	schema := `package test

message User {
    1: string name
    2: int32 age
}
`
	if err := os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
		t.Fatalf("Write schema file failed: %v", err)
	}

	// Build CLI using absolute path
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Run CLI (flags must come before positional arguments)
	genCmd := exec.Command(cliPath, "--out="+outputDir, "--lang=go", schemaFile)
	if err := genCmd.Run(); err != nil {
		t.Fatalf("CLI run failed: %v", err)
	}

	// Verify output file exists and has correct content
	outputFile := filepath.Join(outputDir, "test.xpb.go")
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Read output failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "package test") {
		t.Error("Output should contain 'package test'")
	}
	if !strings.Contains(outputStr, "type User struct") {
		t.Error("Output should contain 'type User struct'")
	}
}

func TestCLI_GenerateTypeScript(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	outputDir := filepath.Join(tmpDir, "generated")

	// Write schema
	schema := `package test

message User {
    1: string name
    2: int32 age
}
`
	if err := os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
		t.Fatalf("Write schema file failed: %v", err)
	}

	// Build CLI using absolute path
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Run CLI (flags must come before positional arguments)
	genCmd := exec.Command(cliPath, "--out="+outputDir, "--lang=ts", schemaFile)
	if err := genCmd.Run(); err != nil {
		t.Fatalf("CLI run failed: %v", err)
	}

	// Verify output file exists and has correct content
	outputFile := filepath.Join(outputDir, "test.xpb.ts")
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Read output failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "export interface UserData") {
		t.Error("Output should contain 'export interface UserData'")
	}
	if !strings.Contains(outputStr, "export class User") {
		t.Error("Output should contain 'export class User'")
	}
}

func TestCLI_GenerateBoth(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	outputDir := filepath.Join(tmpDir, "generated")

	// Write schema
	schema := `package test

message Point {
    1: int32 x
    2: int32 y
}
`
	if err := os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
		t.Fatalf("Write schema file failed: %v", err)
	}

	// Build CLI
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Create output dir failed: %v", err)
	}

	// Run CLI with both languages to output directory (flags must come before positional arguments)
	genCmd := exec.Command(cliPath, "--out="+outputDir, "--lang=go,ts", schemaFile)
	if err := genCmd.Run(); err != nil {
		t.Fatalf("CLI run failed: %v", err)
	}

	// Verify both outputs exist (CLI generates {basename}.xpb.{go,ts})
	goOutput := filepath.Join(outputDir, "test.xpb.go")
	tsOutput := filepath.Join(outputDir, "test.xpb.ts")
	if _, err := os.Stat(goOutput); os.IsNotExist(err) {
		t.Error("Go output file should exist at " + goOutput)
	}
	if _, err := os.Stat(tsOutput); os.IsNotExist(err) {
		t.Error("TypeScript output file should exist at " + tsOutput)
	}
}

func TestCLI_Help(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	cliPath := filepath.Join(tmpDir, "xpbc")

	// Build CLI
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Run CLI with help flag
	helpCmd := exec.Command(cliPath, "--help")
	output, err := helpCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Usage:") {
		t.Error("Help output should contain 'Usage:'")
	}
	if !strings.Contains(outputStr, "-lang") {
		t.Error("Help output should contain '-lang'")
	}
}

func TestCLI_InvalidSchema(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "invalid.xpb")

	// Write invalid schema
	schema := `package test

message User {
    invalid_field string name
}
`
	if err := os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
		t.Fatalf("Write schema file failed: %v", err)
	}

	// Build CLI
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Run CLI - should fail
	genCmd := exec.Command(cliPath, "--lang=go", schemaFile)
	output, err := genCmd.CombinedOutput()
	if err == nil {
		t.Error("CLI should fail on invalid schema")
	}
	if !strings.Contains(string(output), "error") {
		t.Error("Error output should contain 'error'")
	}
}

func TestCLI_Stdout(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	outputDir := filepath.Join(tmpDir, "generated")

	// Write schema
	schema := `package test

message Empty {}
`
	if err := os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
		t.Fatalf("Write schema file failed: %v", err)
	}

	// Build CLI
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Run CLI to stdout (CLI prints "Generated:" message)
	genCmd := exec.Command(cliPath, "--out="+outputDir, "--lang=go", schemaFile)
	output, err := genCmd.Output()
	if err != nil {
		t.Fatalf("CLI run failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Generated:") {
		t.Error("Output should contain 'Generated:' message")
	}
	if !strings.Contains(outputStr, "test.xpb.go") {
		t.Error("Output should mention the generated file")
	}
}

func TestCLI_OutputDir(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	outputDir := filepath.Join(tmpDir, "generated")

	// Write schema
	schema := `package test

message User {
    1: string name
}
`
	if err := os.WriteFile(schemaFile, []byte(schema), 0644); err != nil {
		t.Fatalf("Write schema file failed: %v", err)
	}

	// Build CLI
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")
	cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		t.Fatalf("Build CLI failed: %v", err)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Create output dir failed: %v", err)
	}

	// Run CLI with output dir (flags must come before positional arguments)
	genCmd := exec.Command(cliPath, "--out="+outputDir, "--lang=go", schemaFile)
	if err := genCmd.Run(); err != nil {
		t.Fatalf("CLI run failed: %v", err)
	}

	// Verify output file exists in directory
	expectedFile := filepath.Join(outputDir, "test.xpb.go")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Error("Output file should exist in output directory at " + expectedFile)
	}
}

// CLI benchmarks
func BenchmarkCLI_Build(b *testing.B) {
	b.StopTimer()
	tmpDir := b.TempDir()
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc")
		cmd.Dir = repoRoot
		cmd.Run()
	}
}

func BenchmarkCLI_GenerateGo(b *testing.B) {
	// Setup
	tmpDir := b.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")

	schema := `package test

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	os.WriteFile(schemaFile, []byte(schema), 0644)
	exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc").Dir = repoRoot
	exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc").Run()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(cliPath, "--lang=go", schemaFile)
		cmd.Run()
	}
}

func BenchmarkCLI_GenerateTypeScript(b *testing.B) {
	// Setup
	tmpDir := b.TempDir()
	schemaFile := filepath.Join(tmpDir, "test.xpb")
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	cliPath := filepath.Join(tmpDir, "xpbc")

	schema := `package test

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	os.WriteFile(schemaFile, []byte(schema), 0644)
	exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc").Dir = repoRoot
	exec.Command("go", "build", "-o", cliPath, "./cmd/xpbc").Run()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(cliPath, "--lang=ts", schemaFile)
		cmd.Run()
	}
}
