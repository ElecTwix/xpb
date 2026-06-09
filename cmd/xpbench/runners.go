package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func runGoBenchmarks() ([]Result, error) {
	fmt.Println("🔵 Running Go benchmarks...")
	var results []Result

	// 1. Get Performance Data (go test -bench=.)
	cmd := exec.Command("go", "test", "-bench=.", "-count=1", "./benchmarks/go")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go bench failed: %w", err)
	}
	perfOutput := string(out)

	// 2. Get Size Data (go test -v -run=Test.*EncodedSizes)
	cmdSize := exec.Command("go", "test", "-v", "-run=Test.*EncodedSizes", "./benchmarks/go")
	outSize, err := cmdSize.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go size test failed: %w", err)
	}
	sizeOutput := string(outSize)

	// Parse Sizes
	sizes := parseGoSizes(sizeOutput)

	// Parse Performance and map sizes
	// Regex for standard go bench line: Benchmark<Name>-<Procs> <Iter> <Ns/op>
	// Example: BenchmarkXPB_Encode_Small-20
	re := regexp.MustCompile(`Benchmark([A-Za-z0-9_]+)-(\d+)\s+(\d+)\s+([0-9\.]+)\s+ns/op`)

	lines := strings.Split(perfOutput, "\n")
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 0 {
			nameFull := matches[1]
			nsPerOp, _ := strconv.ParseFloat(matches[4], 64)

			// Parse name: Format_Operation_Category (e.g., XPB_Encode_Small)
			parts := strings.Split(nameFull, "_")
			if len(parts) < 3 {
				continue
			}

			format := parts[0]   // XPB, JSON, Msgpack, Protobuf
			op := parts[1]       // Encode, Decode
			category := parts[2] // Small, Large, StringArray100...

			// Clean up Category
			cleanCat := category
			if strings.HasPrefix(category, "StringArray") {
				cleanCat = "StringArray"
			} else if strings.HasPrefix(category, "Int32Array") {
				cleanCat = "Int32Array"
			} else if strings.HasPrefix(category, "StringMap") {
				cleanCat = "StringMap"
			} else if category == "Simple" {
				cleanCat = "Small" // Match Node naming
			}

			// Clean up Format
			cleanFmt := format
			if format == "XPB" {
				cleanFmt = "XPB V2"
			}

			// Find size
			// Size key needs to match how we stored it from TestEncodedSizes
			// We construct a key: Format + "_" + CleanCat
			sizeKey := cleanFmt + "_" + cleanCat
			// Special handling for legacy names in Go tests
			if cleanCat == "Simple" {
				sizeKey = cleanFmt + "_Small"
			}

			// Try to match specific sizes
			// sizes map has keys like "XPB V2_Small", "JSON_Large", "Msgpack_StringArray"

			sizeVal := sizes[sizeKey]

			results = append(results, Result{
				Platform:  "Go",
				Category:  cleanCat,
				Format:    cleanFmt,
				Operation: op,
				NsPerOp:   nsPerOp,
				Size:      sizeVal,
			})
		}
	}
	return results, nil
}

func parseGoSizes(output string) map[string]int64 {
	sizes := make(map[string]int64)
	lines := strings.Split(output, "\n")

	var currentCat string

	// Helper to map test context to category
	// TestEncodedSizes -> Small
	// TestLargeEncodedSizes -> Large
	// TestCollectionEncodedSizes -> (reads header lines)

	reHeader := regexp.MustCompile(`=== (.*) ===`)
	reSize := regexp.MustCompile(`(XPB V2|JSON|Msgpack|Protobuf)[^:]*:\s+(\d+)\s+bytes`)

	for _, line := range lines {
		// Detect context switch
		if strings.Contains(line, "RUN   TestEncodedSizes") {
			currentCat = "Small"
		} else if strings.Contains(line, "RUN   TestLargeEncodedSizes") {
			currentCat = "Large"
		} else if strings.Contains(line, "RUN   ") {
			currentCat = ""
		} else if sub := reHeader.FindStringSubmatch(line); len(sub) > 0 {
			// Sub-headers in Collection tests: "String Array (100 elements)"
			header := sub[1]
			if strings.Contains(header, "String Array") {
				currentCat = "StringArray"
			} else if strings.Contains(header, "Int32 Array") {
				currentCat = "Int32Array"
			} else if strings.Contains(header, "String Map") {
				currentCat = "StringMap"
			} else if strings.Contains(header, "Simple Message") {
				currentCat = "Small"
			} else if strings.Contains(header, "Large Message") && !strings.Contains(header, "~") && !strings.Contains(header, "XLarge") {
				currentCat = "Large"
			}
		}

		if match := reSize.FindStringSubmatch(line); len(match) > 0 {
			format := match[1]
			size, _ := strconv.ParseInt(match[2], 10, 64)
			key := format + "_" + currentCat
			sizes[key] = size
		}
	}
	return sizes
}

func runNodeBenchmarks() ([]Result, error) {
	fmt.Println("🟢 Running Node.js benchmarks...")
	cmd := exec.Command("npx", "tsx", "src/benchmark.ts", "--json")
	cmd.Dir = "benchmarks/ts"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("node bench failed: %w", err)
	}

	// Parse JSON output
	// The output might contain some logs before JSON, find the first '{'
	outputStr := string(out)
	idx := strings.Index(outputStr, "{")
	if idx == -1 {
		return nil, fmt.Errorf("no json found in node output")
	}
	jsonStr := outputStr[idx:]

	var data NodeOutput
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("failed to parse node json: %w", err)
	}

	return flattenNodeResults(data, "Node"), nil
}

func runBrowserBenchmarks() ([]Result, error) {
	fmt.Println("🔴 Running Browser benchmarks...")

	// Must build first? npm run bench does it.
	// We call `npm run bench -- --json`
	cmd := exec.Command("npm", "run", "bench", "--", "--json")
	cmd.Dir = "benchmarks/browser"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("browser bench failed: %w", err)
	}

	outputStr := string(out)
	idx := strings.Index(outputStr, "{")
	if idx == -1 {
		return nil, fmt.Errorf("no json found in browser output")
	}
	jsonStr := outputStr[idx:]

	var data NodeOutput
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("failed to parse browser json: %w", err)
	}

	return flattenNodeResults(data, "Browser"), nil
}

func flattenNodeResults(data NodeOutput, platform string) []Result {
	var results []Result

	helper := func(list []NodeResult, cat string) {
		for _, r := range list {
			results = append(results, Result{
				Platform:  platform,
				Category:  cat,
				Format:    r.Name, // Keep original name (e.g. "XPB V2 (JIT)")
				Operation: "Encode",
				NsPerOp:   r.EncodeNs,
				Size:      r.SizeBytes,
			})
			results = append(results, Result{
				Platform:  platform,
				Category:  cat,
				Format:    r.Name,
				Operation: "Decode",
				NsPerOp:   r.DecodeNs,
				Size:      r.SizeBytes,
			})
		}
	}

	helper(data.Small, "Small")
	helper(data.Large, "Large")
	helper(data.Collections.StringArray, "StringArray")
	helper(data.Collections.IntArray, "Int32Array")
	helper(data.Collections.StringMap, "StringMap")

	return results
}
