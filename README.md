# DXF → Tiled PDF Pattern Printer

This repo contains a single script, [dxf_tiled_pdf.py](dxf_tiled_pdf.py), that reads a DXF drawing and produces a multi-page PDF where the drawing is **tiled** across pages for printing and taping together as a full-size pattern.

Each page includes:
- A header showing the tile index (column/row)
- Crop marks around the printable rectangle
- Registration crosses at the printable corners
- A 100 mm scale bar

## Install

You need Python 3 plus these libraries:
- `ezdxf` (DXF parsing)
- `reportlab` (PDF generation)

Install with:

```bash
pip3 install ezdxf reportlab
```

## Quick start

DXF units are commonly **mm** (e.g. exported from CAD). Generate letter-sized tiles with 10 mm margins and 10 mm overlap:

```bash
python3 dxf_tiled_pdf.py input.dxf output.pdf \
  --page letter \
  --margin-mm 10 \
  --overlap-mm 10 \
  --dxf-units mm
```

If your DXF coordinates are in **inches**:

```bash
python3 dxf_tiled_pdf.py input.dxf output.pdf \
  --page letter \
  --margin-mm 10 \
  --overlap-mm 10 \
  --dxf-units inch
```

## Command-line arguments

The script interface is:

```text
dxf_tiled_pdf.py INPUT_DXF OUTPUT_PDF [--page {letter,a4}] [--margin-mm MM] [--overlap-mm MM]
                                 [--dxf-units {mm,inch}] [--layers L1,L2,...]
```

### Positional arguments

- `INPUT_DXF`
  - Path to the DXF file to read.
- `OUTPUT_PDF`
  - Path to write the resulting tiled PDF.

### Options

- `--page {letter,a4}` (default: `letter`)
  - Paper size.
  - `letter` is US Letter (8.5" × 11").
  - `a4` is ISO A4 (210 mm × 297 mm).

- `--margin-mm <float>` (default: `10.0`)
  - **Physical** page margin in millimeters.
  - This margin defines the **printable rectangle** the script uses when laying out tiles.

- `--overlap-mm <float>` (default: `10.0`)
  - **Physical** overlap between neighboring tiles, in millimeters.
  - Overlap gives you shared geometry between pages, so alignment/taping is easier.

- `--dxf-units {mm,inch}` (default: `mm`)
  - Declares what one DXF “world unit” means.
  - This controls the 1:1 mapping from DXF coordinates to printed size.

- `--layers L1,L2,...` (default: all layers)
  - Optional comma-separated list of layer names to include.
  - Example: `--layers CUT,MARKS,NOTCHES`
  - Names must match the DXF layer names (including spacing/case as stored in the DXF).

## How distances and scaling work

The script produces a PDF at **true size** (1:1) assuming you pick the correct `--dxf-units`.

### Unit systems involved

- **PDF points (pt)**: ReportLab’s internal unit.
  - $1\ \text{in} = 72\ \text{pt}$
- **Millimeters**: used for margins/overlap UI and for the scale bar.
  - $1\ \text{mm} \approx 2.83464567\ \text{pt}$
- **DXF world units (wu)**: the coordinate system stored in the DXF.
  - If `--dxf-units mm`, then $1\ \text{wu} = 1\ \text{mm}$.
  - If `--dxf-units inch`, then $1\ \text{wu} = 1\ \text{inch}$.

### The key mapping

For each tile/page, DXF coordinates $(x_{wu}, y_{wu})$ are mapped onto the page as:

$$
\begin{aligned}
 x_{pt} &= x_{page0,pt} + (x_{wu} - x_{world0,wu}) \cdot s\\
 y_{pt} &= y_{page0,pt} + (y_{wu} - y_{world0,wu}) \cdot s
\end{aligned}
$$

Where:
- $s$ is `scale_wu_to_pt`
  - If `--dxf-units mm`, then $s = 1\ \text{mm in pt}$
  - If `--dxf-units inch`, then $s = 1\ \text{inch in pt}$
- $(x_{world0,wu}, y_{world0,wu})$ is the tile’s world-space origin (bottom-left of that tile)
- $(x_{page0,pt}, y_{page0,pt})$ is the printable-rectangle origin on the page (equal to the margin)

### Important nuance: `--overlap-mm` is always specified in millimeters

Even when `--dxf-units inch`, the script still treats `--overlap-mm` as millimeters of **physical** overlap.
Internally it converts that overlap into world units (inches) by dividing by 25.4.

This is intentional: margins/overlap describe how you want the *paper layout* to behave.

Similarly, `--margin-mm` is always interpreted as a physical millimeter margin on paper (it does not change if your DXF units are inches).

## Printable area, margins, and registration marks

The script defines a “printable rectangle” by subtracting margins from the page size:

- Printable width (pt): `page_width_pt - 2*margin_pt`
- Printable height (pt): `page_height_pt - 2*margin_pt`

ASCII view of one page:

```text
+----------------------------------------------------+  page edge
|                                                    |
|   margin                                            |
|   +--------------------------------------------+    |
|   |  x  registration cross             registration x|   <- printable corners
|   |                                            |    |
|   |                                            |    |
|   |   (DXF geometry for this tile)             |    |
|   |                                            |    |
|   |  scale bar (100 mm)                        |    |
|   |  x  registration cross             registration x| 
|   +--------------------------------------------+    |
|                                                    |
+----------------------------------------------------+
```

Crop marks are drawn around the printable rectangle corners, and registration crosses are drawn centered on the printable rectangle corners.

## Tiling: how many pages you get

### Step size and overlap

Let:
- `printable_w_wu` / `printable_h_wu` be the printable rectangle size expressed in DXF world units
- `overlap_wu` be the overlap expressed in DXF world units

The script advances tiles by:

- `step_w = printable_w_wu - overlap_wu`
- `step_h = printable_h_wu - overlap_wu`

So neighboring tiles overlap by `overlap_wu`.

ASCII view of two horizontal neighbors:

```text
world X ->

Tile i covers:        [---------------- printable_w_wu ----------------]
Tile i+1 covers:                     [---------------- printable_w_wu ----------------]
                                      ^^^^^^^^^^^^^ overlap_wu ^^^^^^^^^^^^^

step_w = printable_w_wu - overlap_wu
```

If `step_w <= 0` or `step_h <= 0`, the script errors with:

> Overlap too large; no printable area remains.

### Tile count

Given drawing bounds (bbox) width/height:

- `total_w = maxx - minx`
- `total_h = maxy - miny`

The tile grid dimensions are:

- `nx = max(1, ceil((total_w - overlap_wu) / step_w))`
- `ny = max(1, ceil((total_h - overlap_wu) / step_h))`

Tiles are emitted row-by-row (y changes slowest):

```text
(j increases upward)

(0,2) (1,2) (2,2)
(0,1) (1,1) (2,1)
(0,0) (1,0) (2,0)
 i->
```

Each PDF page is one `(i, j)` tile.

### Mermaid: pipeline overview

```mermaid
flowchart TD
  A[Read DXF] --> B[Select layers (optional)]
  B --> C[Compute drawing bbox in DXF world units]
  C --> D[Compute printable area from page + margin]
  D --> E[Convert printable area to world units using dxf-units]
  E --> F[Compute tile grid using overlap]
  F --> G[For each tile: world->page transform]
  G --> H[Draw crop marks + registration crosses + scale bar]
  H --> I[Draw all supported entities onto page]
  I --> J[Next page]
```

## Examples

### 1) Letter paper, no overlap (but harder to align)

```bash
python3 dxf_tiled_pdf.py PouchV1.dxf pattern_tiles-no-overlap.pdf \
  --page letter \
  --margin-mm 10 \
  --overlap-mm 0 \
  --dxf-units inch
```

### 2) A4 paper, 10 mm overlap, only specific layers

```bash
python3 dxf_tiled_pdf.py input.dxf pattern_a4.pdf \
  --page a4 \
  --margin-mm 10 \
  --overlap-mm 10 \
  --dxf-units mm \
  --layers CUT,MARKS
```

### 3) Sanity-check print scaling

- Print one page at **100% / Actual Size** (do not “fit to page”).
- Measure the scale bar on the output: it should be exactly 100 mm.

If the scale bar does not measure 100 mm, your PDF viewer/printer settings are scaling the print.

## Supported DXF entities (current)

The renderer currently draws:
- `LINE`
- `LWPOLYLINE`
- `POLYLINE`
- `SPLINE`
- `CIRCLE`
- `ARC`

Bounding boxes are computed for the same set.

Notes:
- `ARC` bounding boxes are conservative (treated like a full circle), which can slightly increase the number of tiles.
- `SPLINE` geometry is flattened into short line segments for both drawing and bounding boxes.

## Limitations / notes

- No clipping is applied per-tile: the script draws all entities on every page, relying on the world→page transform to place most geometry off-page. This is simple and robust, but for very large DXFs it can be slower.
- DXF text (`TEXT`, `MTEXT`) and splines/ellipses are not rendered yet.
- `--margin-mm` and `--overlap-mm` are physical paper distances in millimeters, independent of DXF units.

## Troubleshooting

- **Output is the wrong physical size**: most often `--dxf-units` is wrong (mm vs inch) or your printer dialog is scaling the print.
- **Too many pages**: reduce `--margin-mm`, reduce `--overlap-mm`, or verify the DXF content doesn’t contain a far-away stray entity.
- **No geometry found**: the DXF may only contain unsupported entity types; export polylines/lines, or extend the script to support additional entities.
