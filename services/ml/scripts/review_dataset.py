#!/usr/local/python/current/bin/python
"""
Dataset review and cleaning script for TikTok rotation captcha samples.
Works in headless environments (devcontainers, SSH, WSL) via a local web UI.

For each sample the script applies X = original_angle / 2.
If the validated formula  new_angle = (original_angle - 2*X) % 360  is correct,
every correctly-labelled sample should land at 0° and appear visually aligned.
Samples that look misaligned are candidates for removal.

Usage
-----
    python review_dataset.py [DATASET_DIR]

If DATASET_DIR is omitted the script resolves:
    <repo_root>/services/discovery/dataset/rotation_captcha

Then open  http://localhost:7878  in your browser (VS Code forwards the port
automatically).

Keyboard controls (browser window must be focused):
    ANY KEY  ->  next sample
    d        ->  mark current sample as CORRUPTED and advance
    q / ESC  ->  quit

At the end a summary is printed and corrupted timestamps are saved to
    <DATASET_DIR>/corrupted_samples.txt
"""

from __future__ import annotations

import json
import sys
import threading
from pathlib import Path
from typing import NamedTuple

import cv2
import numpy as np
from flask import Flask, Response, jsonify
from tqdm import tqdm

# ─────────────────────────────────────────────────────────────────────────────
# Constants
# ─────────────────────────────────────────────────────────────────────────────

PANEL_SIZE: int = 300
PADDING: int = 16
FONT = cv2.FONT_HERSHEY_SIMPLEX
PORT: int = 7878

# ─────────────────────────────────────────────────────────────────────────────
# Browser UI (single-file, no templates)
# ─────────────────────────────────────────────────────────────────────────────

_HTML = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Captcha Dataset Reviewer</title>
  <style>
    * { box-sizing: border-box; }
    body {
      background: #111; color: #ddd;
      font-family: 'Courier New', monospace;
      margin: 0; display: flex; flex-direction: column;
      align-items: center; padding: 24px 16px; min-height: 100vh;
    }
    h2 { margin: 0 0 16px; font-size: 1rem; color: #888; letter-spacing: 1px; }
    #frame { display: block; border: 1px solid #333; border-radius: 6px; max-width: 100%; }
    #bar { margin: 14px 0 6px; font-size: 0.82rem; color: #aaa; text-align: center; }
    #bar .hi  { color: #00e87a; }
    #bar .bad { color: #ff4444; }
    #bar .dim { color: #555; }
    #hint { margin-top: 18px; font-size: 0.78rem; color: #444; }
    kbd { background: #222; border: 1px solid #444; border-radius: 3px;
          padding: 1px 6px; font-family: inherit; color: #ccc; }
    #toast {
      position: fixed; bottom: 24px; right: 24px;
      background: #1e3a2f; border: 1px solid #0e7a4a; color: #00e87a;
      padding: 8px 16px; border-radius: 6px; font-size: 0.82rem;
      opacity: 0; transition: opacity 0.3s; pointer-events: none;
    }
    #done-msg {
      display: none; margin-top: 40px; font-size: 1.1rem;
      color: #00e87a; text-align: center; line-height: 1.8;
    }
  </style>
</head>
<body>
  <h2>ROTATION CAPTCHA — DATASET REVIEWER</h2>
  <img id="frame" src="" alt="captcha">
  <div id="bar">&nbsp;</div>
  <div id="hint">
    <kbd>any key</kbd> next &nbsp;|&nbsp;
    <kbd>D</kbd> mark corrupted &nbsp;|&nbsp;
    <kbd>Q</kbd> / <kbd>Esc</kbd> quit
  </div>
  <div id="toast"></div>
  <div id="done-msg"></div>

  <script>
    const img = document.getElementById('frame');
    const bar = document.getElementById('bar');
    const toast = document.getElementById('toast');
    const doneMsg = document.getElementById('done-msg');
    let busy = false, toastTimer = null;

    function showToast(msg, colour) {
      toast.textContent = msg;
      toast.style.borderColor = colour || '#0e7a4a';
      toast.style.color = colour || '#00e87a';
      toast.style.background = colour ? '#3a1e1e' : '#1e3a2f';
      toast.style.opacity = '1';
      if (toastTimer) clearTimeout(toastTimer);
      toastTimer = setTimeout(() => { toast.style.opacity = '0'; }, 1600);
    }

    function refresh() {
      fetch('/state').then(r => r.json()).then(s => {
        if (s.done) {
          img.style.display = 'none';
          bar.style.display = 'none';
          doneMsg.style.display = 'block';
          doneMsg.innerHTML =
            'Review complete.<br>' +
            'Reviewed: <b>' + s.reviewed + '</b> &nbsp;|&nbsp; ' +
            'Corrupted: <b style="color:#ff6666">' + s.corrupted_count + '</b><br>' +
            '<span style="color:#555;font-size:.8rem">Check the terminal for details.</span>';
          return;
        }
        img.src = '/image?t=' + Date.now() + '&idx=' + s.index;
        const isCorrupted = s.is_corrupted;
        const label = isCorrupted
          ? '<span class="bad">[CORRUPTED]</span> ' + s.timestamp
          : s.timestamp;
        const angleOk = Math.abs(s.new_angle) < 1 || Math.abs(s.new_angle - 360) < 1;
        bar.innerHTML =
          '[<span class="hi">' + s.index + '</span>/<span class="dim">' + s.total + '</span>]  ' +
          label + '&nbsp;&nbsp;' +
          'label=<span class="hi">' + s.original_angle.toFixed(1) + '\u00b0</span>  ' +
          'X=' + s.x_applied.toFixed(1) + '\u00b0  ' +
          'new_angle=<span class="' + (angleOk ? 'hi' : 'bad') + '">' + s.new_angle.toFixed(1) + '\u00b0</span>' +
          ' <span class="dim">(target 0\u00b0)</span>';
        busy = false;
      }).catch(() => { busy = false; });
    }

    document.addEventListener('keydown', e => {
      if (busy) return;
      const key = e.key.toLowerCase();
      if (['f5','f11','f12','tab'].includes(key)) return;
      e.preventDefault();

      if (key === 'q' || key === 'escape') {
        busy = true;
        fetch('/action/quit').then(refresh);
        return;
      }
      if (key === 'd') {
        busy = true;
        fetch('/action/corrupt').then(r => r.json()).then(s => {
          if (s.marked) showToast('Marked as corrupted', '#ff4444');
          else          showToast('Unmarked');
          refresh();
        });
        return;
      }
      busy = true;
      fetch('/action/next').then(refresh);
    });

    refresh();
  </script>
</body>
</html>
"""


# ─────────────────────────────────────────────────────────────────────────────
# Data model
# ─────────────────────────────────────────────────────────────────────────────

class Sample(NamedTuple):
    timestamp: str
    inner: np.ndarray   # BGR image
    outer: np.ndarray   # BGR image
    angle: float        # original label angle (degrees)


# ─────────────────────────────────────────────────────────────────────────────
# I/O helpers
# ─────────────────────────────────────────────────────────────────────────────

def collect_timestamps(dataset_dir: Path) -> list[str]:
    """Return sorted list of timestamps that have a *_label.json file."""
    return sorted(
        p.name[: -len("_label.json")]
        for p in dataset_dir.glob("*_label.json")
    )


def load_sample(dataset_dir: Path, timestamp: str) -> Sample | None:
    """Load a single sample; returns None if any file is missing or unreadable."""
    inner_path = dataset_dir / f"{timestamp}_inner.jpg"
    outer_path = dataset_dir / f"{timestamp}_outer.jpg"
    label_path = dataset_dir / f"{timestamp}_label.json"

    if not all(p.exists() for p in (inner_path, outer_path, label_path)):
        return None

    inner = cv2.imread(str(inner_path))
    outer = cv2.imread(str(outer_path))
    if inner is None or outer is None:
        return None

    with label_path.open() as fh:
        angle = float(json.load(fh)["angle"])

    return Sample(timestamp=timestamp, inner=inner, outer=outer, angle=angle)


# ─────────────────────────────────────────────────────────────────────────────
# Image processing
# ─────────────────────────────────────────────────────────────────────────────

def apply_circular_mask(image: np.ndarray) -> np.ndarray:
    """Zero pixels outside the largest inscribed circle (anti-shortcut-learning)."""
    h, w = image.shape[:2]
    mask = np.zeros((h, w), dtype=np.uint8)
    cv2.circle(mask, (w // 2, h // 2), min(w, h) // 2, 255, thickness=-1)
    result = image.copy()
    result[mask == 0] = 0
    return result


def rotate_image(image: np.ndarray, angle_deg: float, clockwise: bool) -> np.ndarray:
    """
    Rotate image around its centre.
    cv2 positive angle = CCW; negate for CW.
    """
    h, w = image.shape[:2]
    cv2_angle = -abs(angle_deg) if clockwise else abs(angle_deg)
    M = cv2.getRotationMatrix2D((w / 2.0, h / 2.0), cv2_angle, scale=1.0)
    return cv2.warpAffine(image, M, (w, h),
                          flags=cv2.INTER_LINEAR,
                          borderMode=cv2.BORDER_CONSTANT,
                          borderValue=0)


def prepare_piece(image: np.ndarray, angle_deg: float, clockwise: bool) -> np.ndarray:
    """Rotate then apply circular mask."""
    return apply_circular_mask(rotate_image(image, angle_deg, clockwise=clockwise))


# ─────────────────────────────────────────────────────────────────────────────
# Canvas assembly
# ─────────────────────────────────────────────────────────────────────────────

def build_canvas(
    inner_proc: np.ndarray,
    outer_proc: np.ndarray,
    *,
    is_corrupted: bool = False,
) -> np.ndarray:
    """Side-by-side canvas: INNER (cw) | OUTER (ccw)."""
    def sq(img: np.ndarray) -> np.ndarray:
        return cv2.resize(img, (PANEL_SIZE, PANEL_SIZE), interpolation=cv2.INTER_LINEAR)

    sub_h = 22
    W = PADDING * 3 + PANEL_SIZE * 2
    H = PADDING * 2 + PANEL_SIZE + sub_h
    canvas = np.full((H, W, 3), 28, dtype=np.uint8)

    y0 = PADDING
    xi, xo = PADDING, PADDING * 2 + PANEL_SIZE

    canvas[y0:y0 + PANEL_SIZE, xi:xi + PANEL_SIZE] = sq(inner_proc)
    canvas[y0:y0 + PANEL_SIZE, xo:xo + PANEL_SIZE] = sq(outer_proc)

    if is_corrupted:
        for x in (xi, xo):
            cv2.rectangle(canvas, (x, y0), (x + PANEL_SIZE, y0 + PANEL_SIZE),
                          (0, 0, 220), 3)

    sy = y0 + PANEL_SIZE + 16
    cv2.putText(canvas, "INNER  (+X cw)",  (xi + 28, sy), FONT, 0.45, (170, 170, 170), 1, cv2.LINE_AA)
    cv2.putText(canvas, "OUTER  (-X ccw)", (xo + 20, sy), FONT, 0.45, (170, 170, 170), 1, cv2.LINE_AA)

    return canvas


def encode_jpeg(canvas: np.ndarray, quality: int = 88) -> bytes:
    ok, buf = cv2.imencode(".jpg", canvas, [cv2.IMWRITE_JPEG_QUALITY, quality])
    if not ok:
        raise RuntimeError("cv2.imencode failed")
    return buf.tobytes()


# ─────────────────────────────────────────────────────────────────────────────
# Shared state
# ─────────────────────────────────────────────────────────────────────────────

class ReviewState:
    def __init__(self, dataset_dir: Path, timestamps: list[str]) -> None:
        self.dataset_dir = dataset_dir
        self.timestamps = timestamps
        self.total = len(timestamps)
        self.index: int = 0
        self.corrupted: list[str] = []
        self.corrupted_set: set[str] = set()
        self.done: bool = False
        self.pbar = tqdm(total=self.total, desc="Reviewed", unit="sample", ncols=72)
        self._lock = threading.Lock()
        self._current_sample: Sample | None = None
        self._current_jpeg: bytes | None = None
        self._advance_to(0)

    # ── private ───────────────────────────────────────────────────────────────

    def _advance_to(self, idx: int) -> None:
        while idx < self.total:
            ts = self.timestamps[idx]
            sample = load_sample(self.dataset_dir, ts)
            if sample is not None:
                self._current_sample = sample
                self._rebuild_jpeg()
                self.index = idx
                return
            tqdm.write(f"  [SKIP] {ts}  — missing/unreadable file(s)")
            self.pbar.update(1)
            idx += 1
        # exhausted
        self._current_sample = None
        self._current_jpeg = None
        self.index = self.total
        self.done = True
        self.pbar.close()
        self._print_summary()

    def _rebuild_jpeg(self) -> None:
        if self._current_sample is None:
            return
        s = self._current_sample
        x = s.angle / 2.0
        canvas = build_canvas(
            prepare_piece(s.inner, x, clockwise=True),
            prepare_piece(s.outer, x, clockwise=False),
            is_corrupted=(s.timestamp in self.corrupted_set),
        )
        self._current_jpeg = encode_jpeg(canvas)

    def _print_summary(self) -> None:
        print(f"\n{'─' * 52}")
        print(f"Review complete. {len(self.corrupted)} / {self.total} marked as corrupted.")
        if self.corrupted:
            print("\nCorrupted timestamps:")
            for ts in self.corrupted:
                print(f"  {ts}")
            out = self.dataset_dir / "corrupted_samples.txt"
            out.write_text("\n".join(self.corrupted) + "\n", encoding="utf-8")
            print(f"\nSaved -> {out}")
        else:
            print("No corrupted samples marked.")

    # ── public API ────────────────────────────────────────────────────────────

    def get_state_dict(self) -> dict:
        with self._lock:
            if self.done or self._current_sample is None:
                return {"done": True,
                        "reviewed": self.index,
                        "corrupted_count": len(self.corrupted)}
            s = self._current_sample
            x = s.angle / 2.0
            return {
                "done": False,
                "index": self.index + 1,
                "total": self.total,
                "timestamp": s.timestamp,
                "original_angle": s.angle,
                "x_applied": x,
                "new_angle": round((s.angle - 2.0 * x) % 360.0, 2),
                "is_corrupted": s.timestamp in self.corrupted_set,
            }

    def get_jpeg(self) -> bytes | None:
        with self._lock:
            return self._current_jpeg

    def action_next(self) -> None:
        with self._lock:
            if self.done:
                return
            self.pbar.update(1)
            self._advance_to(self.index + 1)

    def action_corrupt(self) -> bool:
        """Toggle mark. Returns True if now marked."""
        with self._lock:
            if self.done or self._current_sample is None:
                return False
            ts = self._current_sample.timestamp
            if ts in self.corrupted_set:
                self.corrupted.remove(ts)
                self.corrupted_set.discard(ts)
                tqdm.write(f"  [UNMARK]    {ts}")
                marked = False
            else:
                self.corrupted.append(ts)
                self.corrupted_set.add(ts)
                tqdm.write(f"  [CORRUPTED] {ts}  label={self._current_sample.angle:.1f}deg")
                marked = True
            self._rebuild_jpeg()
            return marked

    def action_quit(self) -> None:
        with self._lock:
            if not self.done:
                tqdm.write("\nQuitting early ...")
                self.pbar.close()
                self._print_summary()
                self.done = True


# ─────────────────────────────────────────────────────────────────────────────
# Flask app
# ─────────────────────────────────────────────────────────────────────────────

def create_app(state: ReviewState) -> Flask:
    import logging
    logging.getLogger("werkzeug").setLevel(logging.ERROR)

    app = Flask(__name__)
    app.logger.disabled = True

    @app.get("/")
    def index():
        return _HTML, 200, {"Content-Type": "text/html; charset=utf-8"}

    @app.get("/image")
    def image():
        data = state.get_jpeg() or encode_jpeg(np.zeros((1, 1, 3), dtype=np.uint8))
        return Response(data, mimetype="image/jpeg",
                        headers={"Cache-Control": "no-store"})

    @app.get("/state")
    def get_state():
        return jsonify(state.get_state_dict())

    @app.get("/action/next")
    def action_next():
        state.action_next()
        return "", 204

    @app.get("/action/corrupt")
    def action_corrupt():
        marked = state.action_corrupt()
        return jsonify({"marked": marked})

    @app.get("/action/quit")
    def action_quit():
        state.action_quit()
        return "", 204

    return app


# ─────────────────────────────────────────────────────────────────────────────
# Entry point
# ─────────────────────────────────────────────────────────────────────────────

def _default_dataset_dir() -> Path:
    # script:   <root>/services/ml/scripts/review_dataset.py
    # dataset:  <root>/services/discovery/dataset/rotation_captcha
    root = Path(__file__).resolve().parents[3]   # parents[0]=scripts, [1]=ml, [2]=services, [3]=root
    return root / "services" / "discovery" / "dataset" / "rotation_captcha"


def review_dataset(dataset_dir: Path) -> None:
    timestamps = collect_timestamps(dataset_dir)
    if not timestamps:
        print(f"[ERROR] No labelled samples found in: {dataset_dir}", file=sys.stderr)
        sys.exit(1)

    print(f"Dataset : {dataset_dir}")
    print(f"Samples : {len(timestamps)}")
    print()
    print(f"  Open in browser ->  http://localhost:{PORT}")
    print(f"  (VS Code forwards the port automatically)")
    print()

    state = ReviewState(dataset_dir, timestamps)
    app = create_app(state)
    app.run(host="0.0.0.0", port=PORT, debug=False, use_reloader=False)


def main() -> None:
    if len(sys.argv) > 1:
        dataset_path = Path(sys.argv[1]).resolve()
    else:
        dataset_path = _default_dataset_dir()

    if not dataset_path.is_dir():
        print(f"[ERROR] Dataset directory not found: {dataset_path}", file=sys.stderr)
        sys.exit(1)

    review_dataset(dataset_path)


if __name__ == "__main__":
    main()
