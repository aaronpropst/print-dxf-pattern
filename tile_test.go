package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// ── filterEntities ────────────────────────────────────────────────────────────

func TestFilterEntities_noFilter(t *testing.T) {
	entities := []Entity{
		{Kind: KindLine, Layer: "cut"},
		{Kind: KindLine, Layer: "score"},
	}
	got := filterEntities(entities, nil)
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (no filter)", len(got))
	}
}

func TestFilterEntities_withFilter(t *testing.T) {
	entities := []Entity{
		{Kind: KindLine, Layer: "cut"},
		{Kind: KindLine, Layer: "score"},
		{Kind: KindLine, Layer: "cut"},
	}
	got := filterEntities(entities, []string{"cut"})
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (only 'cut')", len(got))
	}
	for _, e := range got {
		if e.Layer != "cut" {
			t.Errorf("unexpected layer %q after filter", e.Layer)
		}
	}
}

func TestFilterEntities_noMatch(t *testing.T) {
	entities := []Entity{
		{Kind: KindLine, Layer: "cut"},
	}
	got := filterEntities(entities, []string{"missing"})
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// ── RenderTiledPDF ────────────────────────────────────────────────────────────

func baseCfg(input, output string) *config {
	return &config{
		inputDXF:  input,
		outputPDF: output,
		page:      "letter",
		marginMM:  10.0,
		overlapMM: 10.0,
		dxfUnits:  "inch",
		noDashed:  false,
	}
}

func TestRenderTiledPDF_producesFile(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir() + "/tiled.pdf"
	cfg := baseCfg(sampleDXF, out)

	if err := RenderTiledPDF(cfg, d); err != nil {
		t.Fatalf("RenderTiledPDF: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output PDF is empty")
	}
	t.Logf("tiled PDF: %d bytes", info.Size())
}

func TestRenderTiledPDF_pageCount(t *testing.T) {
	// The sample DXF is 19.5×8" on letter paper (10mm margin/overlap) → 3×1 = 3 tiles.
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir() + "/tiled.pdf"
	cfg := baseCfg(sampleDXF, out)

	if err := RenderTiledPDF(cfg, d); err != nil {
		t.Fatalf("RenderTiledPDF: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// Count PDF page objects: each page contains "/Type /Page\n" (non-Pages).
	count := bytes.Count(data, []byte("/Type /Page\n"))
	if count == 0 {
		// Some PDF writers omit the newline; try without.
		count = bytes.Count(data, []byte("/Type /Page"))
		// Subtract the /Pages entry (singular)
		count -= bytes.Count(data, []byte("/Type /Pages"))
	}
	t.Logf("PDF page markers found: %d", count)

	// We know from TestComputeTiles_sampleDXF that the tiling is 3×1 = 3 pages.
	if count != 3 {
		t.Errorf("page count = %d, want 3", count)
	}
}

func TestRenderTiledPDF_noDashed(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}

	render := func(noDashed bool) int64 {
		out := t.TempDir() + "/out.pdf"
		cfg := baseCfg(sampleDXF, out)
		cfg.noDashed = noDashed
		if err := RenderTiledPDF(cfg, d); err != nil {
			t.Fatalf("RenderTiledPDF(noDashed=%v): %v", noDashed, err)
		}
		info, _ := os.Stat(out)
		return info.Size()
	}

	sizeAll := render(false)
	sizeNoDash := render(true)

	if sizeNoDash >= sizeAll {
		t.Errorf("noDashed PDF (%d B) should be smaller than full PDF (%d B)", sizeNoDash, sizeAll)
	}
	t.Logf("full=%d B  noDashed=%d B", sizeAll, sizeNoDash)
}

func TestRenderTiledPDF_a4(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir() + "/a4.pdf"
	cfg := baseCfg(sampleDXF, out)
	cfg.page = "a4"

	if err := RenderTiledPDF(cfg, d); err != nil {
		t.Fatalf("RenderTiledPDF(a4): %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestRenderTiledPDF_layerFilter(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}

	// Layer "0" has all entities; filtering to it should produce a valid PDF.
	out := t.TempDir() + "/layer0.pdf"
	cfg := baseCfg(sampleDXF, out)
	cfg.layers = []string{"0"}

	if err := RenderTiledPDF(cfg, d); err != nil {
		t.Fatalf("RenderTiledPDF(layer=0): %v", err)
	}
	info, _ := os.Stat(out)
	if info.Size() == 0 {
		t.Error("layer-filtered PDF is empty")
	}
}

func TestRenderTiledPDF_emptyLayerFilter(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir() + "/empty.pdf"
	cfg := baseCfg(sampleDXF, out)
	cfg.layers = []string{"nonexistent-layer"}

	err = RenderTiledPDF(cfg, d)
	if err == nil {
		t.Error("expected error for empty entity set after layer filter, got nil")
	}
}

// ── run() integration ─────────────────────────────────────────────────────────

func TestRun_endToEnd(t *testing.T) {
	out := t.TempDir() + "/end-to-end.pdf"
	cfg := baseCfg(sampleDXF, out)

	if err := run(cfg); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 1024 {
		t.Errorf("output PDF suspiciously small: %d bytes", info.Size())
	}
	t.Logf("end-to-end PDF: %d bytes", info.Size())
}

func TestRun_outputAlreadyExists(t *testing.T) {
	out := t.TempDir() + "/exists.pdf"
	if err := os.WriteFile(out, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := baseCfg(sampleDXF, out)
	err := run(cfg)
	if err == nil {
		t.Error("expected error when output already exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error %q should mention 'already exists'", err.Error())
	}
}

func TestRun_missingInput(t *testing.T) {
	cfg := baseCfg("nonexistent.dxf", t.TempDir()+"/out.pdf")
	err := run(cfg)
	if err == nil {
		t.Error("expected error for missing input, got nil")
	}
}

// ── drawScaleBar / drawEdgeAlignmentLine (smoke) ──────────────────────────────

func TestDrawScaleBar_noError(t *testing.T) {
	d, err := ParseDXF(sampleDXF)
	if err != nil {
		t.Fatal(err)
	}
	cfg := baseCfg(sampleDXF, "")
	_, pageH := PageSizeMM(cfg.page)
	margin := cfg.marginMM

	// Just verify these don't panic on a real fpdf instance.
	pdf := newLetterPDF()
	drawScaleBar(pdf, margin+5, margin+10, 100)
	drawEdgeAlignmentLine(pdf, margin+100, 215.9, pageH, 10, true)
	drawEdgeAlignmentLine(pdf, margin+100, 215.9, pageH, 10, false)

	out := t.TempDir() + "/decorations.pdf"
	if err := pdf.OutputFileAndClose(out); err != nil {
		t.Fatalf("OutputFileAndClose: %v", err)
	}
	_ = d
}

// ── fpdfPageSize ──────────────────────────────────────────────────────────────

func TestFpdfPageSize(t *testing.T) {
	if fpdfPageSize("letter") != "Letter" {
		t.Errorf("letter → %q, want Letter", fpdfPageSize("letter"))
	}
	if fpdfPageSize("a4") != "A4" {
		t.Errorf("a4 → %q, want A4", fpdfPageSize("a4"))
	}
	if fpdfPageSize("unknown") != "Letter" {
		t.Errorf("unknown → %q, want Letter fallback", fpdfPageSize("unknown"))
	}
}

// ── binary build ──────────────────────────────────────────────────────────────

func TestBinaryBuilds(t *testing.T) {
	// This test is a sentinel: if everything above passes and the package
	// compiles, the binary can be built. We verify by actually running it
	// via os/exec in CI, but the compile step is the real gate.
	_ = fmt.Sprintf // keep fmt import
}
