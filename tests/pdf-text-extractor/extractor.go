package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

func ExtractText(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}

	content := string(data)

	if !strings.HasPrefix(content, "%PDF-") {
		return "", fmt.Errorf("not a valid PDF file")
	}

	var result strings.Builder

	streams := extractStreams(content)
	for _, stream := range streams {
		decoded := decodeStream(stream)
		result.WriteString(extractTextFromContent(decoded))
	}

	if result.Len() == 0 {
		return "", fmt.Errorf("no text found in PDF")
	}

	return result.String(), nil
}

var streamRe = regexp.MustCompile(`stream\r?\n([\s\S]*?)\r?\nendstream`)

func extractStreams(pdfContent string) []string {
	matches := streamRe.FindAllStringSubmatch(pdfContent, -1)
	var result []string
	for _, m := range matches {
		if len(m) > 1 {
			result = append(result, m[1])
		}
	}
	return result
}

func decodeStream(stream string) string {
	result := stream
	result = strings.ReplaceAll(result, "\r\n", "\n")
	result = strings.ReplaceAll(result, "\r", "\n")
	return result
}

var textOpRe = regexp.MustCompile(`\((?:[^()\\]|\\.)*\)`)
var hexTextRe = regexp.MustCompile(`<([0-9A-Fa-f]+)>`)

func extractTextFromContent(stream string) string {
	var buf bytes.Buffer

	for _, m := range textOpRe.FindAllString(stream, -1) {
		inner := m[1 : len(m)-1]
		inner = unescapePDFString(inner)
		buf.WriteString(inner)
	}

	for _, m := range hexTextRe.FindAllStringSubmatch(stream, -1) {
		if len(m) > 1 {
			decoded := decodeHexString(m[1])
			buf.WriteString(decoded)
		}
	}

	return buf.String()
}

func unescapePDFString(s string) string {
	s = strings.ReplaceAll(s, "\\(", "(")
	s = strings.ReplaceAll(s, "\\)", ")")
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\r", "\r")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

func decodeHexString(hex string) string {
	var buf bytes.Buffer
	hex = strings.TrimSpace(hex)
	for i := 0; i+1 < len(hex); i += 2 {
		var b byte
		n, _ := fmt.Sscanf(hex[i:i+2], "%02x", &b)
		if n == 1 {
			buf.WriteByte(b)
		}
	}
	return buf.String()
}
