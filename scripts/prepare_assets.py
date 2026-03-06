#!/usr/bin/env python3
"""Prepare deskpet assets: extract frames, remove backgrounds, resize."""

import os
import subprocess
import tempfile
from pathlib import Path

from PIL import Image
from rembg import remove

PROJECT_ROOT = Path(__file__).resolve().parent.parent
ASSETS_DIR = PROJECT_ROOT / "assets"
MATERIAL_DIR = PROJECT_ROOT / "material"
TARGET_SIZE = 128
FRAME_STEP = 5  # Take every 5th frame


def extract_frames(video_path: Path, output_dir: Path) -> list[Path]:
    """Extract every Nth frame from a video using ffmpeg."""
    pattern = output_dir / "frame_%04d.png"
    subprocess.run(
        [
            "ffmpeg", "-i", str(video_path),
            "-vf", f"select=not(mod(n\\,{FRAME_STEP}))",
            "-vsync", "vfr",
            str(pattern),
        ],
        check=True,
        capture_output=True,
    )
    frames = sorted(output_dir.glob("frame_*.png"))
    print(f"  Extracted {len(frames)} frames")
    return frames


def remove_background(img: Image.Image) -> Image.Image:
    """Remove background from a PIL Image using rembg."""
    return remove(img)


def center_crop_square(img: Image.Image) -> Image.Image:
    """Center-crop image to a square."""
    w, h = img.size
    side = min(w, h)
    left = (w - side) // 2
    top = (h - side) // 2
    return img.crop((left, top, left + side, top + side))


def process_image(input_path: Path, output_path: Path) -> None:
    """Remove background, center-crop, resize, and save."""
    img = Image.open(input_path).convert("RGBA")
    img = remove_background(img)
    img = center_crop_square(img)
    img = img.resize((TARGET_SIZE, TARGET_SIZE), Image.LANCZOS)
    img.save(output_path, "PNG")


def main():
    MATERIAL_DIR.mkdir(exist_ok=True)

    # Process idle image
    print("Processing idle image...")
    idle_src = ASSETS_DIR / "image.png"
    idle_dst = MATERIAL_DIR / "idle.png"
    process_image(idle_src, idle_dst)
    print(f"  Saved {idle_dst}")

    # Extract and process happy animation frames
    print("Processing happy animation...")
    with tempfile.TemporaryDirectory() as tmp:
        tmp_dir = Path(tmp)
        frames = extract_frames(ASSETS_DIR / "happy.mp4", tmp_dir)
        for i, frame_path in enumerate(frames):
            output_path = MATERIAL_DIR / f"happy_{i:02d}.png"
            print(f"  Processing frame {i}/{len(frames)-1}...")
            process_image(frame_path, output_path)

    # Extract and process walk animation frames
    print("Processing walk animation...")
    with tempfile.TemporaryDirectory() as tmp:
        tmp_dir = Path(tmp)
        frames = extract_frames(ASSETS_DIR / "walk.mp4", tmp_dir)
        for i, frame_path in enumerate(frames):
            output_path = MATERIAL_DIR / f"walk_{i:02d}.png"
            print(f"  Processing frame {i}/{len(frames)-1}...")
            process_image(frame_path, output_path)

    # Report results
    happy_count = len(list(MATERIAL_DIR.glob("happy_*.png")))
    walk_count = len(list(MATERIAL_DIR.glob("walk_*.png")))
    print(f"\nDone! Created:")
    print(f"  idle.png")
    print(f"  happy_00.png - happy_{happy_count-1:02d}.png ({happy_count} frames)")
    print(f"  walk_00.png - walk_{walk_count-1:02d}.png ({walk_count} frames)")


if __name__ == "__main__":
    main()
