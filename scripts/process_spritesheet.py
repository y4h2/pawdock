#!/usr/bin/env python3
"""Process green-screen videos into sprite sheets for deskpet.

Usage:
    python scripts/process_spritesheet.py                    # process all 3 new videos
    python scripts/process_spritesheet.py hide peek found    # process specific videos
"""

import os
import subprocess
import sys
import tempfile
from pathlib import Path

import numpy as np
from PIL import Image

PROJECT_ROOT = Path(__file__).resolve().parent.parent
ASSET_DIR = PROJECT_ROOT / "assets"
MATERIAL_DIR = PROJECT_ROOT / "material"

TARGET = 256
COLS = 9
FPS = 12
IDLE_RATIO = 0.85

# Videos to process
VIDEOS = {
    "hide": ASSET_DIR / "hide.mp4",
    "peek": ASSET_DIR / "peek.mp4",
    "found": ASSET_DIR / "found.mp4",
}


def extract_frames(video_path: Path, output_dir: Path) -> list[Path]:
    """Extract frames at 12fps using ffmpeg."""
    pattern = output_dir / "frame_%04d.png"
    subprocess.run(
        ["ffmpeg", "-y", "-i", str(video_path), "-vf", f"fps={FPS}", str(pattern)],
        check=True,
        capture_output=True,
    )
    frames = sorted(output_dir.glob("frame_*.png"))
    print(f"  Extracted {len(frames)} frames")
    return frames


def remove_green_screen(data: np.ndarray) -> np.ndarray:
    """Remove green-screen pixels."""
    r, g, b = data[:, :, 0], data[:, :, 1], data[:, :, 2]
    green_mask = (g > 100) & (g > r * 1.3) & (g > b * 1.3)
    data[green_mask] = [0, 0, 0, 0]
    return data


def remove_watermark(data: np.ndarray, small: bool = False) -> np.ndarray:
    """Clear watermark area.

    Default: bottom 12%, right 45% (works for most videos).
    small=True: bottom 5%, right 30% (for videos where character feet overlap).
    """
    h, w = data.shape[:2]
    if small:
        data[int(h * 0.95):, int(w * 0.70):] = [0, 0, 0, 0]
    else:
        data[int(h * 0.88):, int(w * 0.55):] = [0, 0, 0, 0]
    return data


def clean_green_fringe(data: np.ndarray) -> np.ndarray:
    """Reduce green fringe on semi-transparent edges."""
    r = data[:, :, 0].astype(np.int16)
    g = data[:, :, 1].astype(np.int16)
    b = data[:, :, 2].astype(np.int16)
    a = data[:, :, 3].astype(np.int16)

    avg_rb = (r + b) // 2
    greenness = g - avg_rb

    # Reduce green channel where fringe is detected
    fringe_mask = (a > 0) & (greenness > 30) & (g > 80)
    data[:, :, 1][fringe_mask] = np.minimum(g, avg_rb + 15).astype(np.uint8)[fringe_mask]

    # Reduce alpha for strong green fringe
    strong_fringe = (a > 0) & (greenness > 50)
    data[:, :, 3][strong_fringe] = np.clip(a - greenness, 0, 255).astype(np.uint8)[strong_fringe]

    return data


def process_frame(frame_path: Path, small_watermark: bool = False) -> Image.Image:
    """Process a single frame: remove green, watermark, clean fringe."""
    img = Image.open(frame_path).convert("RGBA")
    data = np.array(img)
    data = remove_green_screen(data)
    data = remove_watermark(data, small=small_watermark)
    data = clean_green_fringe(data)
    return Image.fromarray(data)


def get_bbox(img: Image.Image) -> tuple[int, int, int, int] | None:
    """Get bounding box of non-transparent pixels."""
    data = np.array(img)
    alpha = data[:, :, 3]
    rows = np.any(alpha > 0, axis=1)
    cols = np.any(alpha > 0, axis=0)
    if not rows.any():
        return None
    rmin, rmax = np.where(rows)[0][[0, -1]]
    cmin, cmax = np.where(cols)[0][[0, -1]]
    return int(cmin), int(rmin), int(cmax + 1), int(rmax + 1)


def compute_union_bbox(frames: list[Image.Image], padding: int = 4) -> tuple[int, int, int, int]:
    """Compute union bounding box across all frames with padding."""
    x1, y1, x2, y2 = float("inf"), float("inf"), 0, 0
    for f in frames:
        bb = get_bbox(f)
        if bb is None:
            continue
        x1 = min(x1, bb[0])
        y1 = min(y1, bb[1])
        x2 = max(x2, bb[2])
        y2 = max(y2, bb[3])

    # Add padding, clamped to image bounds
    w, h = frames[0].size
    x1 = max(0, int(x1) - padding)
    y1 = max(0, int(y1) - padding)
    x2 = min(w, int(x2) + padding)
    y2 = min(h, int(y2) + padding)
    return x1, y1, x2, y2


def measure_char_ratio(frame: Image.Image) -> float:
    """Measure character height ratio relative to frame height."""
    bb = get_bbox(frame)
    if bb is None:
        return 1.0
    char_h = bb[3] - bb[1]
    return char_h / frame.size[1]


def build_spritesheet(
    frames: list[Image.Image], scale_factor: float
) -> tuple[Image.Image, int]:
    """Build a TARGET x TARGET sprite sheet with COLS columns, bottom-aligned."""
    n = len(frames)
    rows = (n + COLS - 1) // COLS

    sheet = Image.new("RGBA", (COLS * TARGET, rows * TARGET), (0, 0, 0, 0))

    for i, frame in enumerate(frames):
        fw, fh = frame.size
        s = min(TARGET / fw, TARGET / fh) * scale_factor
        nw = int(fw * s)
        nh = int(fh * s)
        resized = frame.resize((nw, nh), Image.LANCZOS)

        # Bottom-aligned, horizontally centered
        x_off = (TARGET - nw) // 2
        y_off = TARGET - nh

        col = i % COLS
        row = i // COLS
        sheet.paste(resized, (col * TARGET + x_off, row * TARGET + y_off), resized)

    return sheet, n


def process_video(name: str, video_path: Path):
    """Full pipeline for one video."""
    print(f"\n=== Processing {name} ({video_path.name}) ===")

    with tempfile.TemporaryDirectory() as tmp:
        tmp_dir = Path(tmp)

        # 1. Extract frames
        raw_frames = extract_frames(video_path, tmp_dir)

        # 2-4. Process each frame
        # Use smaller watermark area for 'found' where character feet overlap
        small_wm = name in ("found",)
        print("  Processing frames (green removal, watermark, fringe cleanup)...")
        processed = [process_frame(p, small_watermark=small_wm) for p in raw_frames]

        # 5. Auto crop with union bounding box
        bbox = compute_union_bbox(processed)
        print(f"  Union bbox: {bbox}")
        cropped = [f.crop(bbox) for f in processed]

        # 6. Character size normalization
        char_ratio = measure_char_ratio(cropped[0])
        scale_factor = IDLE_RATIO / char_ratio
        print(f"  char_ratio={char_ratio:.3f}, scale_factor={scale_factor:.3f}")

        # 7. Generate sprite sheet
        sheet, total = build_spritesheet(cropped, scale_factor)

        # Save outputs
        MATERIAL_DIR.mkdir(exist_ok=True)
        sheet_path = MATERIAL_DIR / f"{name}_sheet.png"
        sheet.save(sheet_path)
        print(f"  Saved {sheet_path}")

        backup_path = ASSET_DIR / f"{name}_spritesheet.png"
        sheet.save(backup_path)
        print(f"  Saved {backup_path}")

        fw, fh = cropped[0].size
        meta_path = ASSET_DIR / f"{name}_spritesheet_meta.txt"
        meta_path.write_text(
            f"frames={total}\n"
            f"frame_size={fw}x{fh}\n"
            f"sheet_cell={TARGET}x{TARGET}\n"
            f"cols={COLS}\n"
            f"fps={FPS}\n"
            f"char_ratio={char_ratio:.3f}\n"
            f"scale_factor={scale_factor:.3f}\n"
        )
        print(f"  Saved {meta_path}")


def main():
    names = sys.argv[1:] if len(sys.argv) > 1 else list(VIDEOS.keys())

    for name in names:
        if name not in VIDEOS:
            print(f"Unknown video: {name}. Available: {list(VIDEOS.keys())}")
            continue
        video_path = VIDEOS[name]
        if not video_path.exists():
            print(f"Video not found: {video_path}")
            continue
        process_video(name, video_path)

    print("\nDone!")


if __name__ == "__main__":
    main()
