# PDF Text Extractor - PRD

## Overview
A Go CLI tool that extracts plain text from PDF files. Supports multi-page documents, handles common encodings, and outputs clean UTF-8 text.

## Features
- Extract text from single or multi-page PDFs
- Handle common PDF text encodings (ASCII, UTF-8, Latin-1)
- Stream-based extraction for memory efficiency
- CLI with `--input` and `--output` flags

## Architecture
- `extractor.go` — Core extraction engine: parses PDF cross-reference table, locates page objects, decodes content streams
- `main.go` — CLI entry point using stdlib `flag` package
- `extractor_test.go` — Unit tests with minimal valid PDF fixtures

## Non-Goals
- No image/OCR extraction
- No PDF writing/editing
- No encryption or password-protected PDFs
