"""Prefetch fixed tarot or fishing art from local manifests; requires Pillow."""
import argparse
import concurrent.futures
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import time
import urllib.request
import urllib.parse
from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
FOLDER = ROOT / "data/Tarot"
REPORT = ROOT / ".local/tarot-cache"
BASE = "https://raw.githubusercontent.com/FloatTech/zbpdata/main/Tarot/"
MIN_SIDE = 100

def validate(data):
    if not data or len(data) > 8 * 1024 * 1024:
        raise ValueError("Invalid image size")
    with Image.open(io.BytesIO(data)) as img:
        if img.format not in ("PNG", "JPEG"):
            raise ValueError("Expected PNG or JPEG")
        img.verify()
    with Image.open(io.BytesIO(data)) as img:
        img.load()
        if min(img.size) < MIN_SIDE:
            raise ValueError("Unexpectedly small card art")
        return list(img.size)

def fetch(name):
    dest = FOLDER / name
    old = dest.read_bytes() if dest.exists() else None
    try:
        dims = validate(old)
        data, source = old, "cache"
    except Exception:
        data = None
        for attempt in range(3):
            try:
                with urllib.request.urlopen(BASE + urllib.parse.quote(name, safe="/"), timeout=25) as response:
                    data = response.read(8 * 1024 * 1024 + 1)
                dims = validate(data)
                break
            except Exception as exc:
                data = None
                error = type(exc).__name__ + ": " + str(exc)
                time.sleep(attempt + 1)
        if data is None:
            return {"name": name, "passed": False, "error": error}
        dest.parent.mkdir(parents=True, exist_ok=True)
        if old is not None:
            backup = REPORT / "replaced" / name
            backup.parent.mkdir(parents=True, exist_ok=True)
            backup.write_bytes(old)
        tmp = dest.with_suffix(".prefetch.tmp")
        tmp.write_bytes(data)
        os.replace(tmp, dest)
        source = "download"
    return {"name": name, "passed": True, "source": source, "bytes": len(data),
            "sha256": hashlib.sha256(data).hexdigest(), "dimensions": dims}

def main():
    global FOLDER, REPORT, BASE, MIN_SIDE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kind", choices=["tarot", "mcfish"], default="tarot")
    args = parser.parse_args()
    names = set()
    if args.kind == "tarot":
        cards = json.loads((FOLDER / "tarots.json").read_text(encoding="utf-8"))
        for card in cards.values():
            name = card["info"]["imgUrl"]
            names.update([name, "Reverse/" + name])
        if len(cards) != 78 or len(names) != 156:
            raise ValueError("Expected 78 cards and 156 upright/reversed images")
    else:
        FOLDER = ROOT / "data/McFish"
        REPORT = ROOT / ".local/mcfish-cache"
        BASE = "https://raw.githubusercontent.com/FloatTech/zbpdata/main/McFish/"
        MIN_SIDE = 1
        items = json.loads((FOLDER / "articlesInfo.json").read_text(encoding="utf-8"))["物品"]
        names = {item["名称"] + ".png" for item in items if item["类型"] in ["pole", "fish", "waste"]}
        names.update(name + ".png" for name in ["美西螈", "唱片", "海豚", "book"])
    for name in names:
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts or "\\" in name or ":" in name:
            raise ValueError("Unsafe image path")
    REPORT.mkdir(parents=True, exist_ok=True)
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        for result in pool.map(fetch, sorted(names)):
            results.append(result)
            if len(results) % 20 == 0:
                print(f"Checked {len(results)}/{len(names)}", flush=True)
    report = {"passed": all(x["passed"] for x in results), "expected": len(names),
              "valid": sum(x["passed"] for x in results), "files": results}
    (REPORT / "result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({k:v for k,v in report.items() if k != "files"}))
    raise SystemExit(0 if report["passed"] else 1)

if __name__ == "__main__":
    main()
