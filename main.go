package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	inputDXF  string
	outputPDF string
	page      string
	marginMM  float64
	overlapMM float64
	dxfUnits  string
	layers    []string
	noDashed  bool
}

func parseArgs(args []string) (*config, error) {
	fs := flag.NewFlagSet("dxf2pdf", flag.ContinueOnError)

	page := fs.String("page", "letter", "Page size: letter or a4")
	marginMM := fs.Float64("margin-mm", 10.0, "Margin in mm")
	overlapMM := fs.Float64("overlap-mm", 10.0, "Tile overlap in mm")
	dxfUnits := fs.String("dxf-units", "inch", "DXF drawing units: mm or inch")
	layersStr := fs.String("layers", "", "Comma-separated layer names to include (empty = all)")
	noDashed := fs.Bool("no-dashed", false, "Skip entities with non-continuous linetypes")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if fs.NArg() < 1 {
		return nil, fmt.Errorf("usage: dxf2pdf [options] <input.dxf> [output.pdf]")
	}

	*page = strings.ToLower(*page)
	if *page != "letter" && *page != "a4" {
		return nil, fmt.Errorf("--page must be 'letter' or 'a4', got %q", *page)
	}

	*dxfUnits = strings.ToLower(*dxfUnits)
	if *dxfUnits != "mm" && *dxfUnits != "inch" {
		return nil, fmt.Errorf("--dxf-units must be 'mm' or 'inch', got %q", *dxfUnits)
	}

	inputDXF := fs.Arg(0)

	outputPDF := ""
	if fs.NArg() >= 2 {
		outputPDF = fs.Arg(1)
	} else {
		ext := filepath.Ext(inputDXF)
		outputPDF = strings.TrimSuffix(inputDXF, ext) + ".pdf"
	}

	var layers []string
	if *layersStr != "" {
		for _, l := range strings.Split(*layersStr, ",") {
			if t := strings.TrimSpace(l); t != "" {
				layers = append(layers, t)
			}
		}
	}

	return &config{
		inputDXF:  inputDXF,
		outputPDF: outputPDF,
		page:      *page,
		marginMM:  *marginMM,
		overlapMM: *overlapMM,
		dxfUnits:  *dxfUnits,
		layers:    layers,
		noDashed:  *noDashed,
	}, nil
}

func run(cfg *config) error {
	if _, err := os.Stat(cfg.inputDXF); err != nil {
		return fmt.Errorf("input file not found: %s", cfg.inputDXF)
	}
	if _, err := os.Stat(cfg.outputPDF); err == nil {
		return fmt.Errorf("output file already exists: %s", cfg.outputPDF)
	}

	d, err := ParseDXF(cfg.inputDXF)
	if err != nil {
		return fmt.Errorf("parsing DXF: %w", err)
	}

	return RenderTiledPDF(cfg, d)
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
