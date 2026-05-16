package main

import (
	"strings"
	"testing"
)

func makeMinimalPDF(text string) string {
	return "%PDF-1.4\n" +
		"1 0 obj\n<< /Type /Page /Parent 3 0 R /Contents 2 0 R >>\nendobj\n" +
		"2 0 obj\n<< /Length 44 >>\nstream\nBT /F1 12 Tf 72 720 Td (" + text + ") Tj ET\nendstream\nendobj\n" +
		"3 0 obj\n<< /Type /Pages /Kids [1 0 R] /Count 1 >>\nendobj\n" +
		"xref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000085 00000 n \n0000000161 00000 n \n" +
		"trailer\n<< /Size 4 /Root 3 0 R >>\nstartxref\n226\n%%EOF"
}

func TestExtractText_Simple(t *testing.T) {
	pdf := makeMinimalPDF("Hello")
	text, err := ExtractText(strings.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello") {
		t.Errorf("expected text to contain 'Hello', got %q", text)
	}
}

func TestExtractText_MultiPage(t *testing.T) {
	pdf := "%PDF-1.4\n" +
		"1 0 obj\n<< /Length 24 >>\nstream\nBT (Page1) Tj ET\nendstream\nendobj\n" +
		"2 0 obj\n<< /Length 24 >>\nstream\nBT (Page2) Tj ET\nendstream\nendobj\n" +
		"xref\n0 3\n0000000000 65535 f \n0000000009 00000 n \n0000000085 00000 n \n" +
		"trailer\n<< /Size 3 >>\nstartxref\n161\n%%EOF"

	text, err := ExtractText(strings.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Page1") || !strings.Contains(text, "Page2") {
		t.Errorf("expected both pages, got %q", text)
	}
}

func TestExtractText_InvalidInput(t *testing.T) {
	_, err := ExtractText(strings.NewReader("not a pdf"))
	if err == nil {
		t.Fatal("expected error for invalid PDF")
	}
}

func TestExtractText_NoText(t *testing.T) {
	pdf := "%PDF-1.4\n1 0 obj\n<< >>\nendobj\nxref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 1 >>\nstartxref\n9\n%%EOF"
	_, err := ExtractText(strings.NewReader(pdf))
	if err == nil {
		t.Fatal("expected error for PDF with no text")
	}
}

func TestUnescapePDFString(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"hello", "hello"},
		{`\(parens\)`, "(parens)"},
		{`line\nbreak`, "line\nbreak"},
		{`tab\ttest`, "tab\ttest"},
		{`back\\\\slash`, `back\\slash`},
	}
	for _, tc := range tests {
		got := unescapePDFString(tc.input)
		if got != tc.expected {
			t.Errorf("unescapePDFString(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestDecodeHexString(t *testing.T) {
	result := decodeHexString("48656C6C6F")
	if result != "Hello" {
		t.Errorf("decodeHexString = %q, want 'Hello'", result)
	}
}
