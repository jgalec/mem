package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	inputPath := flag.String("input", "", "Path to input PDF file")
	outputPath := flag.String("output", "", "Path to output text file (default: stdout)")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: pdf-text-extractor --input <pdf-file> [--output <txt-file>]")
		os.Exit(1)
	}

	f, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening input: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	text, err := ExtractText(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting text: %v\n", err)
		os.Exit(1)
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, []byte(text), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(text)
	}
}
