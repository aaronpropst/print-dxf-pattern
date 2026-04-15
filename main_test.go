package main

import (
	"strings"
	"testing"
)

func TestParseArgs_defaults(t *testing.T) {
	cfg, err := parseArgs([]string{"drawing.dxf"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.inputDXF != "drawing.dxf" {
		t.Errorf("inputDXF = %q, want %q", cfg.inputDXF, "drawing.dxf")
	}
	if cfg.outputPDF != "drawing.pdf" {
		t.Errorf("outputPDF = %q, want %q", cfg.outputPDF, "drawing.pdf")
	}
	if cfg.page != "letter" {
		t.Errorf("page = %q, want %q", cfg.page, "letter")
	}
	if cfg.marginMM != 10.0 {
		t.Errorf("marginMM = %v, want 10.0", cfg.marginMM)
	}
	if cfg.overlapMM != 10.0 {
		t.Errorf("overlapMM = %v, want 10.0", cfg.overlapMM)
	}
	if cfg.dxfUnits != "inch" {
		t.Errorf("dxfUnits = %q, want %q", cfg.dxfUnits, "inch")
	}
	if cfg.noDashed {
		t.Errorf("noDashed = true, want false")
	}
	if len(cfg.layers) != 0 {
		t.Errorf("layers = %v, want empty", cfg.layers)
	}
}

func TestParseArgs_explicitOutput(t *testing.T) {
	cfg, err := parseArgs([]string{"in.dxf", "out.pdf"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.outputPDF != "out.pdf" {
		t.Errorf("outputPDF = %q, want %q", cfg.outputPDF, "out.pdf")
	}
}

func TestParseArgs_outputDerivation(t *testing.T) {
	cases := []struct{ input, want string }{
		{"foo.dxf", "foo.pdf"},
		{"path/to/bar.dxf", "path/to/bar.pdf"},
		{"no-ext", "no-ext.pdf"},
	}
	for _, tc := range cases {
		cfg, err := parseArgs([]string{tc.input})
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", tc.input, err)
		}
		if cfg.outputPDF != tc.want {
			t.Errorf("input %q: outputPDF = %q, want %q", tc.input, cfg.outputPDF, tc.want)
		}
	}
}

func TestParseArgs_flags(t *testing.T) {
	cfg, err := parseArgs([]string{
		"--page", "a4",
		"--margin-mm", "15",
		"--overlap-mm", "5",
		"--dxf-units", "mm",
		"--layers", "cut, score, engrave",
		"--no-dashed",
		"drawing.dxf",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.page != "a4" {
		t.Errorf("page = %q, want a4", cfg.page)
	}
	if cfg.marginMM != 15.0 {
		t.Errorf("marginMM = %v, want 15.0", cfg.marginMM)
	}
	if cfg.overlapMM != 5.0 {
		t.Errorf("overlapMM = %v, want 5.0", cfg.overlapMM)
	}
	if cfg.dxfUnits != "mm" {
		t.Errorf("dxfUnits = %q, want mm", cfg.dxfUnits)
	}
	if !cfg.noDashed {
		t.Errorf("noDashed = false, want true")
	}
	want := []string{"cut", "score", "engrave"}
	if len(cfg.layers) != len(want) {
		t.Fatalf("layers = %v, want %v", cfg.layers, want)
	}
	for i, w := range want {
		if cfg.layers[i] != w {
			t.Errorf("layers[%d] = %q, want %q", i, cfg.layers[i], w)
		}
	}
}

func TestParseArgs_missingInput(t *testing.T) {
	_, err := parseArgs([]string{})
	if err == nil {
		t.Fatal("expected error for missing input, got nil")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("error %q should mention usage", err.Error())
	}
}

func TestParseArgs_badPage(t *testing.T) {
	_, err := parseArgs([]string{"--page", "tabloid", "drawing.dxf"})
	if err == nil {
		t.Fatal("expected error for bad page size, got nil")
	}
}

func TestParseArgs_badUnits(t *testing.T) {
	_, err := parseArgs([]string{"--dxf-units", "furlongs", "drawing.dxf"})
	if err == nil {
		t.Fatal("expected error for bad units, got nil")
	}
}
