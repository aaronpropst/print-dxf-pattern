#!/usr/bin/env python3
from __future__ import annotations

import math
import argparse
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


def entity_bbox_wu(
    e,
    *,
    scale_wu_to_pt: Optional[float] = None,
    spline_flatten_mm: float = 0.5,
) -> Optional[Tuple[float, float, float, float]]:
    """Return (minx, miny, maxx, maxy) in DXF world units for supported entities.

    Notes:
    - SPLINE entities are approximated by flattening into short line segments.
    - For SPLINE we need `scale_wu_to_pt` so the flattening tolerance can be
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

        if t == "SPLINE":
            if not scale_wu_to_pt:
                return None
            flatten_dist_wu = (spline_flatten_mm * mm) / float(scale_wu_to_pt)
            if flatten_dist_wu <= 0:
                return None
            p = make_path(e)
            pts = list(p.flattening(flatten_dist_wu))
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


def draw_registration_cross(c: canvas.Canvas, x: float, y: float, size_pt: float = 10*mm):
    """Simple crosshair registration mark centered at (x,y)."""
    half = size_pt / 2.0
    c.line(x - half, y, x + half, y)
    c.line(x, y - half, x, y + half)


def draw_edge_alignment_marks(
    c: canvas.Canvas,
    *,
    seam_x_pt: Optional[float] = None,
    seam_y_pt: Optional[float] = None,
    page_w_pt: float,
    page_h_pt: float,
    inset_pt: float,
    size_pt: float = 8 * mm,
):
    """Draw alignment marks that indicate where the *next page's paper edge* should land.

    - If seam_x_pt is provided, draw two marks on a vertical seam line.
    - If seam_y_pt is provided, draw two marks on a horizontal seam line.

    The marks are inset from the paper edges by inset_pt so they remain visible
    after overlapping pages.
    """
    if seam_x_pt is not None:
        y0 = inset_pt
        y1 = page_h_pt - inset_pt
        draw_registration_cross(c, seam_x_pt, y0, size_pt=size_pt)
        draw_registration_cross(c, seam_x_pt, y1, size_pt=size_pt)

    if seam_y_pt is not None:
        x0 = inset_pt
        x1 = page_w_pt - inset_pt
        draw_registration_cross(c, x0, seam_y_pt, size_pt=size_pt)
        draw_registration_cross(c, x1, seam_y_pt, size_pt=size_pt)


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


def draw_entity(c: canvas.Canvas, e, xform: WorldToPage):
    t = e.dxftype()
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

    if t == "SPLINE":
        # Approximate curve with line segments.
        # Choose a flattening distance that corresponds to ~0.5mm on paper.
        flatten_dist_wu = (0.5 * mm) / float(xform.scale_wu_to_pt)
        if flatten_dist_wu <= 0:
            return
        try:
            p = make_path(e)
            pts = list(p.flattening(flatten_dist_wu))
        except Exception:
            return
        if len(pts) < 2:
            return
        x0, y0 = xform.w2p(pts[0].x, pts[0].y)
        path = c.beginPath()
        path.moveTo(x0, y0)
        for v in pts[1:]:
            xp, yp = xform.w2p(v.x, v.y)
            path.lineTo(xp, yp)
        c.drawPath(path)
        return

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
    ap.add_argument("output_pdf")
    ap.add_argument("--page", default="letter", choices=["letter", "a4"])
    ap.add_argument("--margin-mm", type=float, default=10.0)
    ap.add_argument("--overlap-mm", type=float, default=10.0)
    ap.add_argument("--dxf-units", default="mm", choices=["mm", "inch"])
    ap.add_argument("--layers", default=None,
                    help="Comma-separated layer names to include (optional).")
    args = ap.parse_args()

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

    c = canvas.Canvas(args.output_pdf, pagesize=(
        spec.width_pt, spec.height_pt))
    c.setLineWidth(0.6)

    minx, miny, maxx, maxy = bbox

    for (i, j, tile_x0_wu, tile_y0_wu) in tiles:
        # page header
        c.setFont("Helvetica", 9)
        c.drawString(spec.margin_pt, spec.height_pt -
                     spec.margin_pt + 2, f"Tile {i+1}/{nx} x {j+1}/{ny}")
        c.drawRightString(spec.width_pt - spec.margin_pt, spec.height_pt -
                          spec.margin_pt + 2, "DXF tiled pattern (1:1)")

        # printable rect (optional visual aid)
        x0p = spec.margin_pt
        y0p = spec.margin_pt
        x1p = spec.width_pt - spec.margin_pt
        y1p = spec.height_pt - spec.margin_pt

        # Edge-alignment marks: align the next page's PAPER EDGE to these marks.
        # Only draw where a neighbor exists.
        inset_pt = 10 * mm
        if i < nx - 1:
            draw_edge_alignment_marks(
                c,
                seam_x_pt=step_w_pt,
                seam_y_pt=None,
                page_w_pt=spec.width_pt,
                page_h_pt=spec.height_pt,
                inset_pt=inset_pt,
            )
        if j < ny - 1:
            draw_edge_alignment_marks(
                c,
                seam_x_pt=None,
                seam_y_pt=step_h_pt,
                page_w_pt=spec.width_pt,
                page_h_pt=spec.height_pt,
                inset_pt=inset_pt,
            )

        # scale bar near bottom margin
        draw_scale_bar(c, x0p + 5*mm, y0p + 5*mm, length_mm=100.0)

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
            draw_entity(c, e, xform)

        # Show where this tile sits in overall pattern (optional text)
        c.setFont("Helvetica", 7)
        c.drawString(spec.margin_pt, spec.margin_pt - 8,
                     f"World origin for tile: ({tile_x0_wu:.2f}, {tile_y0_wu:.2f}) {args.dxf_units}")

        c.showPage()

    c.save()


if __name__ == "__main__":
    main()
