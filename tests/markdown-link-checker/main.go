package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: markdown-link-checker <file-or-dir> [<file-or-dir> ...]")
		os.Exit(1)
	}

	checker := NewChecker()
	var allResults []LinkResult

	for _, arg := range os.Args[1:] {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error accessing %s: %v\n", arg, err)
			continue
		}

		var files []string
		if info.IsDir() {
			files, err = FindMarkdownFiles(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", arg, err)
				continue
			}
		} else {
			files = []string{arg}
		}

		for _, f := range files {
			results, err := checker.CheckFile(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error checking %s: %v\n", f, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	}

	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].File != allResults[j].File {
			return allResults[i].File < allResults[j].File
		}
		return allResults[i].Line < allResults[j].Line
	})

	ok, broken, redirect, error_, skipped := Summary(allResults)

	output := map[string]interface{}{
		"summary": map[string]interface{}{
			"total":    len(allResults),
			"ok":       ok,
			"broken":   broken,
			"redirect": redirect,
			"error":    error_,
			"skipped":  skipped,
		},
		"results": allResults,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(output)

	if broken > 0 || error_ > 0 {
		os.Exit(1)
	}
}
