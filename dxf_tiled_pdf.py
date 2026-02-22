#!/usr/bin/env python3
from __future__ import annotations

import math
import argparse
import os
from dataclasses import dataclass
from typing import Iterable, Tuple, List, Optional

import ezdxf
from ezdxf.math import Vec2
from ezdxf.path import make_path
from reportlab.pdfgen import canvas
from reportlab.lib.units import inch, mm
from reportlab.lib.pagesizes import letter, A4

# ----------------------------
# Helpers / data structures
# ----------------------------


@dataclass
class PageSpec:
    width_pt: float
    height_pt: float
    margin_pt: float
    overlap_pt: float


@dataclass
class WorldToPage:
    # World units are whatever your DXF is in (often mm from Fusion).
    # scale_wu_to_pt converts world-unit lengths to PDF points.
    scale_wu_to_pt: float
    # Origin of world space on the current page in world units (bottom-left of printable area)
    world_x0: float
    world_y0: float
    # Page offsets in points for printable area origin
    page_x0_pt: float
    page_y0_pt: float

    def w2p(self, x_wu: float, y_wu: float) -> Tuple[float, float]:
        x_pt = self.page_x0_pt + (x_wu - self.world_x0) * self.scale_wu_to_pt
        y_pt = self.page_y0_pt + (y_wu - self.world_y0) * self.scale_wu_to_pt
        return x_pt, y_pt


def page_size(name: str) -> Tuple[float, float]:
    name = name.lower()
    if name in ("letter", "us-letter", "usletter"):
        return letter
    if name in ("a4",):
        return A4
    raise ValueError(f"Unsupported page size: {name}")

# ----------------------------
# DXF geometry extraction
# ----------------------------


def iter_entities(doc: ezdxf.EzdxfDocument, layers: Optional[List[str]] = None):
    msp = doc.modelspace()
    for e in msp:
        if layers and e.dxf.layer not in layers:
            continue
        yield e


def _linetype_name_is_continuous(name: Optional[str]) -> bool:
    if not name:
        return True
    return name.strip().upper() in ("CONTINUOUS", "CONT")


def _get_linetype_case_insensitive(doc: ezdxf.EzdxfDocument, name: str):
    try:
        return doc.linetypes.get(name)
    except Exception:
        pass
    upper = name.strip().upper()
    for lt in doc.linetypes:
        if lt.dxf.name.strip().upper() == upper:
            return lt
    raise KeyError(name)


def _effective_linetype_name(doc: ezdxf.EzdxfDocument, e) -> str:
    """Return the effective linetype name for an entity.

    DXF entities can specify:
    - an explicit linetype name (e.g. "DASHED")
    - "BYLAYER" (inherit from the entity's layer)
    - "BYBLOCK" (inherit from a block insert; treat as continuous here)
    """
    lt = getattr(e.dxf, "linetype", None) or "BYLAYER"
    lt_u = lt.strip().upper()
    if lt_u == "BYLAYER":
        layer_name = getattr(e.dxf, "layer", "0")
        try:
            layer = doc.layers.get(layer_name)
            return getattr(layer.dxf, "linetype", "CONTINUOUS") or "CONTINUOUS"
        except Exception:
            return "CONTINUOUS"
    if lt_u == "BYBLOCK":
        return "CONTINUOUS"
    return lt


def _reportlab_dash_array_for_linetype(
    doc: ezdxf.EzdxfDocument,
    linetype_name: str,
    *,
    scale_wu_to_pt: float,
) -> Optional[List[float]]:
    """Convert a DXF linetype pattern into a ReportLab dash array in points.

    Uses ezdxf's `simplified_line_pattern()` which returns positive segment
    lengths in drawing units, alternating ON/OFF, starting with ON.
    """
    if _linetype_name_is_continuous(linetype_name):
        return None
    try:
        lt = _get_linetype_case_insensitive(doc, linetype_name)
        pattern_wu = lt.simplified_line_pattern()
    except Exception:
        return None

    if not pattern_wu:
        return None

    # DXF linetype patterns are often visually "chunky" when mapped 1:1 into PDF.
    # Scale the dash/gap lengths down to get a finer-looking pattern.
    dash_scale = 0.3

    pattern_pt: List[float] = []
    for seg in pattern_wu:
        seg_pt = float(seg) * float(scale_wu_to_pt) * dash_scale
        # Avoid zeros which ReportLab dash can't represent.
        if seg_pt <= 0:
            seg_pt = 0.2 * mm
        pattern_pt.append(seg_pt)

    # ReportLab expects an even-length array; if odd, repeat it.
    if len(pattern_pt) % 2 == 1:
        pattern_pt = pattern_pt * 2
    return pattern_pt


def _flatten_path_entity_points(
    e,
    *,
    scale_wu_to_pt: Optional[float],
    flatten_mm: float,
):
    """Flatten a path-like DXF entity (e.g. SPLINE/ELLIPSE) into Vec3 points.

    Returns a list of points in DXF world units.
    """
    if not scale_wu_to_pt:
        return []
    flatten_dist_wu = (float(flatten_mm) * mm) / float(scale_wu_to_pt)
    if flatten_dist_wu <= 0:
        return []
    try:
        p = make_path(e)
        return list(p.flattening(flatten_dist_wu))
    except Exception:
        return []


def _draw_polyline_from_vec3(c: canvas.Canvas, xform: WorldToPage, pts) -> None:
    if len(pts) < 2:
        return
    x0, y0 = xform.w2p(pts[0].x, pts[0].y)
    path = c.beginPath()
    path.moveTo(x0, y0)
    for v in pts[1:]:
        xp, yp = xform.w2p(v.x, v.y)
        path.lineTo(xp, yp)
    c.drawPath(path)


def entity_bbox_wu(
    e,
    *,
    scale_wu_to_pt: Optional[float] = None,
    spline_flatten_mm: float = 0.5,
) -> Optional[Tuple[float, float, float, float]]:
    """Return (minx, miny, maxx, maxy) in DXF world units for supported entities.

    Notes:
        - SPLINE and ELLIPSE entities are approximated by flattening into short line segments.
        - For flattened curves we need `scale_wu_to_pt` so the flattening tolerance can be
            expressed as a physical distance (mm) on paper.
    """
    t = e.dxftype()
    try:
        if t == "LINE":
            p1 = e.dxf.start
            p2 = e.dxf.end
            xs = [p1.x, p2.x]
            ys = [p1.y, p2.y]
            return min(xs), min(ys), max(xs), max(ys)

        if t in ("LWPOLYLINE",):
            pts = [Vec2(p[0], p[1]) for p in e.get_points("xy")]
            xs = [p.x for p in pts]
            ys = [p.y for p in pts]
            return min(xs), min(ys), max(xs), max(ys)

        if t in ("POLYLINE",):
            pts = [Vec2(v.dxf.location.x, v.dxf.location.y)
                   for v in e.vertices()]
            if not pts:
                return None
            xs = [p.x for p in pts]
            ys = [p.y for p in pts]
            return min(xs), min(ys), max(xs), max(ys)

        if t == "CIRCLE":
            c = e.dxf.center
            r = float(e.dxf.radius)
            return c.x - r, c.y - r, c.x + r, c.y + r

        if t == "ARC":
            c = e.dxf.center
            r = float(e.dxf.radius)
            # Conservative bbox: full circle bounds (fast + safe)
            return c.x - r, c.y - r, c.x + r, c.y + r

        if t == "POINT":
            p = e.dxf.location
            return p.x, p.y, p.x, p.y

        if t == "SPLINE":
            pts = _flatten_path_entity_points(
                e, scale_wu_to_pt=scale_wu_to_pt, flatten_mm=spline_flatten_mm
            )
            if not pts:
                return None
            xs = [v.x for v in pts]
            ys = [v.y for v in pts]
            return min(xs), min(ys), max(xs), max(ys)

        if t == "ELLIPSE":
            pts = _flatten_path_entity_points(
                e, scale_wu_to_pt=scale_wu_to_pt, flatten_mm=spline_flatten_mm
            )
            if not pts:
                return None
            xs = [v.x for v in pts]
            ys = [v.y for v in pts]
            return min(xs), min(ys), max(xs), max(ys)

        print(
            f"Warning: Unsupported entity type '{t}' for bounding box; skipping.")

        # Add more types as needed (ELLIPSE, SPLINE, etc.)
        return None
    except Exception:
        return None


def drawing_bbox_wu(
    ents: Iterable,
    *,
    scale_wu_to_pt: Optional[float] = None,
    spline_flatten_mm: float = 0.5,
) -> Tuple[float, float, float, float]:
    bbs = [
        entity_bbox_wu(e, scale_wu_to_pt=scale_wu_to_pt,
                       spline_flatten_mm=spline_flatten_mm)
        for e in ents
    ]
    bbs = [bb for bb in bbs if bb is not None]
    if not bbs:
        raise ValueError(
            "No supported geometry found in DXF (LINE/LWPOLYLINE/SPLINE/etc.).")
    minx = min(bb[0] for bb in bbs)
    miny = min(bb[1] for bb in bbs)
    maxx = max(bb[2] for bb in bbs)
    maxy = max(bb[3] for bb in bbs)
    return minx, miny, maxx, maxy

# ----------------------------
# PDF drawing primitives
# ----------------------------


def draw_edge_alignment_dashed_line(
    c: canvas.Canvas,
    *,
    seam_x_pt: Optional[float] = None,
    seam_y_pt: Optional[float] = None,
    page_w_pt: float,
    page_h_pt: float,
    inset_pt: float,
    dash_on_pt: float = 0.5 * mm,
    dash_off_pt: float = 9.5 * mm,
):
    """Draw a dashed seam line indicating where the *next page's paper edge* should land.

    - seam_x_pt: draws a vertical dashed line at x=seam_x_pt
    - seam_y_pt: draws a horizontal dashed line at y=seam_y_pt

    Lines are inset from paper edges by inset_pt so they remain visible after
    overlapping pages.
    """
    c.saveState()
    try:
        c.setLineWidth(0.8)
        c.setLineCap(1)  # round cap => short dashes look like dots
        c.setDash(dash_on_pt, dash_off_pt)

        if seam_x_pt is not None:
            c.line(seam_x_pt, inset_pt, seam_x_pt, page_h_pt - inset_pt)

        if seam_y_pt is not None:
            c.line(inset_pt, seam_y_pt, page_w_pt - inset_pt, seam_y_pt)
    finally:
        c.restoreState()


def draw_crop_marks(c: canvas.Canvas, x0: float, y0: float, x1: float, y1: float, len_pt: float = 6*mm):
    """Crop marks at the corners of a rectangle."""
    # bottom-left
    c.line(x0, y0, x0 + len_pt, y0)
    c.line(x0, y0, x0, y0 + len_pt)
    # bottom-right
    c.line(x1, y0, x1 - len_pt, y0)
    c.line(x1, y0, x1, y0 + len_pt)
    # top-left
    c.line(x0, y1, x0 + len_pt, y1)
    c.line(x0, y1, x0, y1 - len_pt)
    # top-right
    c.line(x1, y1, x1 - len_pt, y1)
    c.line(x1, y1, x1, y1 - len_pt)


def draw_scale_bar(c: canvas.Canvas, x: float, y: float, length_mm: float = 100.0):
    """Draw a 100mm scale bar by default."""
    length_pt = length_mm * mm
    c.setLineWidth(1)
    c.line(x, y, x + length_pt, y)
    c.line(x, y - 2*mm, x, y + 2*mm)
    c.line(x + length_pt, y - 2*mm, x + length_pt, y + 2*mm)
    c.setFont("Helvetica", 8)
    c.drawString(x, y + 3*mm, f"{int(length_mm)} mm scale bar")


def draw_entity(
    c: canvas.Canvas,
    e,
    xform: WorldToPage,
    *,
    doc: Optional[ezdxf.EzdxfDocument] = None,
    exclude_noncontinuous_linetypes: bool = False,
):
    t = e.dxftype()

    lt_name: Optional[str] = None
    dash: Optional[List[float]] = None
    if doc is not None:
        lt_name = _effective_linetype_name(doc, e)
        if exclude_noncontinuous_linetypes and not _linetype_name_is_continuous(lt_name):
            return
        dash = _reportlab_dash_array_for_linetype(
            doc, lt_name, scale_wu_to_pt=xform.scale_wu_to_pt
        )

    c.saveState()
    try:
        if dash:
            c.setDash(dash, 0)

        if t == "LINE":
            p1 = e.dxf.start
            p2 = e.dxf.end
            x1, y1 = xform.w2p(p1.x, p1.y)
            x2, y2 = xform.w2p(p2.x, p2.y)
            c.line(x1, y1, x2, y2)
            return

        if t == "LWPOLYLINE":
            pts = [(p[0], p[1]) for p in e.get_points("xy")]
            if not pts:
                return
            p0 = pts[0]
            x0, y0 = xform.w2p(p0[0], p0[1])
            path = c.beginPath()
            path.moveTo(x0, y0)
            for (xw, yw) in pts[1:]:
                xp, yp = xform.w2p(xw, yw)
                path.lineTo(xp, yp)
            if e.closed:
                path.close()
            c.drawPath(path)
            return

        if t == "POLYLINE":
            pts = [(v.dxf.location.x, v.dxf.location.y) for v in e.vertices()]
            if not pts:
                return
            x0, y0 = xform.w2p(pts[0][0], pts[0][1])
            path = c.beginPath()
            path.moveTo(x0, y0)
            for (xw, yw) in pts[1:]:
                xp, yp = xform.w2p(xw, yw)
                path.lineTo(xp, yp)
            if e.is_closed:
                path.close()
            c.drawPath(path)
            return

        if t == "CIRCLE":
            cc = e.dxf.center
            r = float(e.dxf.radius)
            x, y = xform.w2p(cc.x, cc.y)
            r_pt = r * xform.scale_wu_to_pt
            c.circle(x, y, r_pt)
            return

        if t == "ARC":
            cc = e.dxf.center
            r = float(e.dxf.radius)
            start = float(e.dxf.start_angle)
            end = float(e.dxf.end_angle)
            # reportlab uses degrees CCW from +x, same convention
            x, y = xform.w2p(cc.x, cc.y)
            r_pt = r * xform.scale_wu_to_pt
            c.arc(x - r_pt, y - r_pt, x + r_pt, y + r_pt,
                  startAng=start, extent=(end - start))
            return

        if t == "POINT":
            p = e.dxf.location
            x, y = xform.w2p(p.x, p.y)
            # Render as a small filled dot in physical units.
            r_pt = 0.4 * mm
            c.circle(x, y, r_pt, stroke=0, fill=1)
            return

        if t == "SPLINE":
            pts = _flatten_path_entity_points(
                e, scale_wu_to_pt=xform.scale_wu_to_pt, flatten_mm=0.5
            )
            _draw_polyline_from_vec3(c, xform, pts)
            return

        if t == "ELLIPSE":
            pts = _flatten_path_entity_points(
                e, scale_wu_to_pt=xform.scale_wu_to_pt, flatten_mm=0.5
            )
            _draw_polyline_from_vec3(c, xform, pts)
            return
    finally:
        c.restoreState()

# ----------------------------
# Tiling logic
# ----------------------------


def compute_tiles(bbox: Tuple[float, float, float, float], printable_w_wu: float, printable_h_wu: float, overlap_wu: float):
    minx, miny, maxx, maxy = bbox
    total_w = maxx - minx
    total_h = maxy - miny

    step_w = printable_w_wu - overlap_wu
    step_h = printable_h_wu - overlap_wu
    if step_w <= 0 or step_h <= 0:
        raise ValueError("Overlap too large; no printable area remains.")

    nx = max(1, math.ceil((total_w - overlap_wu) / step_w))
    ny = max(1, math.ceil((total_h - overlap_wu) / step_h))

    tiles = []
    for j in range(ny):
        for i in range(nx):
            x0 = minx + i * step_w
            y0 = miny + j * step_h
            tiles.append((i, j, x0, y0))
    return tiles, nx, ny

# ----------------------------
# Main
# ----------------------------


def main():
    ap = argparse.ArgumentParser(
        description="Convert DXF to tiled PDF with registration marks.")
    ap.add_argument("input_dxf")
    ap.add_argument(
        "output_pdf",
        nargs="?",
        default=None,
        help="Output PDF path (optional). Defaults to <input_dxf_basename>.pdf",
    )
    ap.add_argument("--page", default="letter", choices=["letter", "a4"])
    ap.add_argument("--margin-mm", type=float, default=10.0)
    ap.add_argument("--overlap-mm", type=float, default=10.0)
    ap.add_argument("--dxf-units", default="inch", choices=["mm", "inch"])
    ap.add_argument("--layers", default=None,
                    help="Comma-separated layer names to include (optional).")
    ap.add_argument(
        "--exclude-noncontinuous-linetypes",
        action="store_true",
        help="Skip entities whose effective linetype is not CONTINUOUS (e.g. DASHED).",
    )
    args = ap.parse_args()

    output_pdf = args.output_pdf
    if not output_pdf:
        root, _ext = os.path.splitext(args.input_dxf)
        output_pdf = root + ".pdf"
    if os.path.exists(output_pdf):
        raise SystemExit(f"Error: output PDF already exists: {output_pdf}")

    doc = ezdxf.readfile(args.input_dxf)
    layer_list = [s.strip()
                  for s in args.layers.split(",")] if args.layers else None

    ents_list = list(iter_entities(doc, layers=layer_list))

    # Map DXF units -> PDF points (1 pt = 1/72 inch)
    if args.dxf_units == "mm":
        scale = mm  # reportlab unit: 1mm in points
        overlap_wu = args.overlap_mm  # overlap in world units (mm)
        margin_pt = args.margin_mm * mm
        overlap_pt = args.overlap_mm * mm
    else:
        scale = inch  # 1 inch in points
        # if user still provides overlap in mm; keep consistent
        overlap_wu = args.overlap_mm / 25.4
        margin_pt = args.margin_mm * mm
        overlap_pt = args.overlap_mm * mm

    bbox = drawing_bbox_wu(ents_list, scale_wu_to_pt=scale)

    page_w_pt, page_h_pt = page_size(args.page)
    spec = PageSpec(width_pt=page_w_pt, height_pt=page_h_pt,
                    margin_pt=margin_pt, overlap_pt=overlap_pt)

    printable_w_pt = spec.width_pt - 2 * spec.margin_pt
    printable_h_pt = spec.height_pt - 2 * spec.margin_pt
    printable_w_wu = printable_w_pt / scale
    printable_h_wu = printable_h_pt / scale

    step_w_pt = printable_w_pt - overlap_pt
    step_h_pt = printable_h_pt - overlap_pt

    tiles, nx, ny = compute_tiles(
        bbox, printable_w_wu, printable_h_wu, overlap_wu)

    c = canvas.Canvas(output_pdf, pagesize=(
        spec.width_pt, spec.height_pt))
    c.setLineWidth(0.6)

    for (i, j, tile_x0_wu, tile_y0_wu) in tiles:
        # page header
        c.setFont("Helvetica", 9)
        c.drawString(spec.margin_pt, spec.height_pt -
                     spec.margin_pt + 2, f"Tile {i+1}/{nx} x {j+1}/{ny}")

        # printable rect (optional visual aid)
        x0p = spec.margin_pt

        # scale bar: print only on the first page, near top-left
        if i == 0 and j == 0:
            y_top_printable = spec.height_pt - spec.margin_pt
            draw_scale_bar(c, x0p + 5 * mm, y_top_printable -
                           8 * mm, length_mm=100.0)

        # Edge-alignment marks: align the next page's PAPER EDGE to these marks.
        # Only draw where a neighbor exists.
        inset_pt = 10 * mm
        if i < nx - 1:
            draw_edge_alignment_dashed_line(
                c,
                seam_x_pt=step_w_pt,
                seam_y_pt=None,
                page_w_pt=spec.width_pt,
                page_h_pt=spec.height_pt,
                inset_pt=inset_pt,
            )
        if j < ny - 1:
            draw_edge_alignment_dashed_line(
                c,
                seam_x_pt=None,
                seam_y_pt=step_h_pt,
                page_w_pt=spec.width_pt,
                page_h_pt=spec.height_pt,
                inset_pt=inset_pt,
            )

        # world->page transform for this tile
        xform = WorldToPage(
            scale_wu_to_pt=scale,
            world_x0=tile_x0_wu,
            world_y0=tile_y0_wu,
            page_x0_pt=spec.margin_pt,
            page_y0_pt=spec.margin_pt,
        )

        # Draw entities (no clipping here; simple + robust. optional clip could be added.)
        c.setLineWidth(1)
        for e in ents_list:
            draw_entity(
                c,
                e,
                xform,
                doc=doc,
                exclude_noncontinuous_linetypes=args.exclude_noncontinuous_linetypes,
            )

        # Show where this tile sits in overall pattern (optional text)
        c.setFont("Helvetica", 7)
        c.drawString(spec.margin_pt, spec.margin_pt - 8,
                     f"World origin for tile: ({tile_x0_wu:.2f}, {tile_y0_wu:.2f}) {args.dxf_units}")

        c.showPage()

    c.save()


if __name__ == "__main__":
    main()
