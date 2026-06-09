package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

func main() {
	runGo := flag.Bool("go", false, "Run Go benchmarks")
	runNode := flag.Bool("node", false, "Run Node.js benchmarks")
	runBrowser := flag.Bool("browser", false, "Run Browser benchmarks")
	flag.Parse()

	// Default to all if none specified
	if !*runGo && !*runNode && !*runBrowser {
		*runGo = true
		*runNode = true
		*runBrowser = true
	}

	var allResults []Result

	if *runGo {
		res, err := runGoBenchmarks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running Go benchmarks: %v\n", err)
		} else {
			allResults = append(allResults, res...)
		}
	}

	if *runNode {
		res, err := runNodeBenchmarks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running Node benchmarks: %v\n", err)
		} else {
			allResults = append(allResults, res...)
		}
	}

	if *runBrowser {
		res, err := runBrowserBenchmarks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running Browser benchmarks: %v\n", err)
		} else {
			allResults = append(allResults, res...)
		}
	}

	printTable(allResults)
}

func printTable(results []Result) {
	fmt.Println("\n╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║               XPB V2 Unified Benchmark Results                ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")

	// Filter/Aggregate best XPB results if multiple (e.g. JIT vs Manual)
	// For this report, we'll just show them all sorted.

	// Group by Category -> Operation -> Format -> Platform

	categories := []string{"Small", "Large", "StringArray", "Int32Array", "StringMap"}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	for _, cat := range categories {
		fmt.Printf("\n📦 %s Benchmarks\n", cat)
		fmt.Fprintln(w, "Format\tOperation\tGo\tNode.js\tBrowser\tSize")
		fmt.Fprintln(w, "------\t---------\t--\t-------\t-------\t----")

		// Find unique formats in this category
		formats := make(map[string]bool)
		for _, r := range results {
			if r.Category == cat {
				// Simplify format name for grouping
				fmtName := simplifyFormatName(r.Format)
				formats[fmtName] = true
			}
		}

		sortedFormats := []string{}
		for f := range formats {
			sortedFormats = append(sortedFormats, f)
		}
		sort.Strings(sortedFormats)

		// Prioritize XPB
		sort.Slice(sortedFormats, func(i, j int) bool {
			if strings.Contains(sortedFormats[i], "XPB") && !strings.Contains(sortedFormats[j], "XPB") {
				return true
			}
			if !strings.Contains(sortedFormats[i], "XPB") && strings.Contains(sortedFormats[j], "XPB") {
				return false
			}
			return sortedFormats[i] < sortedFormats[j]
		})

		for _, fmtName := range sortedFormats {
			for _, op := range []string{"Encode", "Decode"} {
				goRes := findResult(results, "Go", cat, fmtName, op)
				nodeRes := findResult(results, "Node", cat, fmtName, op)
				browserRes := findResult(results, "Browser", cat, fmtName, op)

				// Format times
				goStr := formatNs(goRes)
				nodeStr := formatNs(nodeRes)
				browserStr := formatNs(browserRes)

				// Size (take from any)
				size := int64(0)
				if goRes != nil {
					size = goRes.Size
				} else if nodeRes != nil {
					size = nodeRes.Size
				} else if browserRes != nil {
					size = browserRes.Size
				}

				sizeStr := "-"
				if size > 0 {
					sizeStr = fmt.Sprintf("%d B", size)
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", fmtName, op, goStr, nodeStr, browserStr, sizeStr)
			}
			// Add separator between formats
			// fmt.Fprintln(w, "\t\t\t\t\t")
		}
		w.Flush()
	}
}

func simplifyFormatName(name string) string {
	if strings.Contains(name, "XPB") {
		return "XPB V2"
	} // Group JIT/Manual together?
	// Actually, we want to see JIT vs Manual.
	// But keeping it simple for the consolidated table:
	if strings.Contains(name, "JIT") {
		return "XPB V2 (JIT)"
	}
	if strings.Contains(name, "Manual") {
		return "XPB V2 (Man)"
	}
	return name
}

func findResult(results []Result, platform, cat, fmtName, op string) *Result {
	// Try exact match first
	for i := range results {
		r := &results[i]
		if r.Platform == platform && r.Category == cat && r.Operation == op {
			// fuzzy match format
			if simplifyFormatName(r.Format) == fmtName {
				// If multiple matches (e.g. Manual vs JIT both mapping to XPB V2), pick best?
				// But we distinguished them above.
				return r
			}
		}
	}
	return nil
}

func formatNs(r *Result) string {
	if r == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f ns", r.NsPerOp)
}
