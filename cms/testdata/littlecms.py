#!/usr/bin/env python3
"""Writes littlecms.json: what LittleCMS 2 makes of the transforms that
TestAgainstLittleCMS checks, through Pillow's ImageCms and without its
optimisations, so that the reference is LittleCMS's exact pipeline, and
how far its optimised tables are from that.

Run it from this directory with Pillow installed, after the synthetic
profiles have been written:

    go test ./cms -run TestWriteSyntheticProfiles -cms.write-profiles
    python3 cms/testdata/littlecms.py

Profiles named system: are those of macOS, those named modcache: come with
github.com/kovidgoyal/imaging in the Go module cache. Neither is in the
repository; the test skips a case whose profile is missing or differs.
"""
import base64
import hashlib
import json
import os

from PIL import Image, ImageCms

HERE = os.path.dirname(os.path.abspath(__file__))
MODCACHE = os.environ.get("GOMODCACHE") or os.path.expanduser("~/go/pkg/mod")

RGB = [0, 51, 102, 153, 204, 255]
CMYK = [0, 85, 170, 255]
GRAY = list(range(0, 256, 5))


def path(name):
    if name.startswith("system:"):
        return name[len("system:"):]
    if name.startswith("modcache:"):
        return os.path.join(MODCACHE, name[len("modcache:"):])
    return os.path.join(HERE, "profiles", name + ".icc")


def space(data):
    return data[16:20].decode("ascii").strip()


def pixels(sp):
    if sp == "RGB":
        return "RGB", bytes(v for r in RGB for g in RGB for b in RGB for v in (r, g, b))
    if sp == "CMYK":
        return "CMYK", bytes(v for c in CMYK for m in CMYK for y in CMYK for k in CMYK for v in (c, m, y, k))
    return "L", bytes(GRAY)


def outputs(data):
    """Whether a profile can be a destination: tables or a matrix."""
    n = int.from_bytes(data[128:132], "big")
    sigs = {data[132 + 12 * i:136 + 12 * i] for i in range(n)}
    return bool(sigs & {b"B2A0", b"B2A1"}) or b"rTRC" in sigs or b"kTRC" in sigs


SYNTHETIC_MATRIX = ["displayp3", "adobergb", "prophoto", "rec2020", "linear-srgb", "sgray"]
SYNTHETIC_TABLES = ["rgb-mab", "rgb-lut8"]
LOCAL = [
    "system:/System/Library/ColorSync/Profiles/Display P3.icc",
    "system:/System/Library/ColorSync/Profiles/AdobeRGB1998.icc",
    "system:/System/Library/ColorSync/Profiles/ROMM RGB.icc",
    "system:/System/Library/ColorSync/Profiles/ITU-2020.icc",
    "system:/System/Library/ColorSync/Profiles/sRGB Profile.icc",
    "system:/System/Library/ColorSync/Profiles/Generic Gray Gamma 2.2 Profile.icc",
    "system:/System/Library/ColorSync/Profiles/Generic CMYK Profile.icc",
]
PRISM = "modcache:github.com/kovidgoyal/imaging@v1.9.0/prism/meta/icc/test-profiles/"
LOCAL += [PRISM + n for n in [
    "cmyk.icc", "lcms-stress.icc", "sRGB_ICC_v4_Appearance.icc", "jpegli.icc",
    "lcms-check-lut.icc", "lcms-check-full.icc", "gray-v2-lut.icc", "gray-v4-lut.icc",
    "gray-v4-lab-gamma2.2.icc", "sRGB-v2-micro.icc", "prophoto.icc",
]]


def cases():
    s = "srgb"
    for d in SYNTHETIC_MATRIX:
        for i in (0, 1):
            yield s, d, i, False
            yield d, s, i, False
    yield "adobergb", "displayp3", 1, False
    for i in (0, 1, 2, 3):
        for bpc in (False, True):
            yield "cmyk-lut16", s, i, bpc
    for i in (0, 1, 3):
        for bpc in (False, True):
            yield s, "cmyk-lut16", i, bpc
    for t in SYNTHETIC_TABLES:
        for i in (0, 1):
            for bpc in (False, True):
                yield t, s, i, bpc
                yield s, t, i, bpc
    for i in (0, 1, 3):
        yield "gray-xyz", s, i, False
    yield s, "gray-xyz", 1, False
    yield "cmyk-lut16", "rgb-mab", 0, True
    for p in LOCAL:
        for i in (0, 1):
            for bpc in (False, True):
                yield p, s, i, bpc
                yield s, p, i, bpc
        if "CMYK" in p or "cmyk" in p:
            yield p, s, 3, False


def main():
    profiles, out = {}, []
    for src, dst, intent, bpc in cases():
        data = {}
        for n in (src, dst):
            with open(path(n), "rb") as f:
                data[n] = f.read()
            if ":" in n:
                profiles[n] = hashlib.sha256(data[n]).hexdigest()
        if not outputs(data[dst]):
            continue
        mode, px = pixels(space(data[src]))
        dmode = {"RGB": "RGB", "CMYK": "CMYK", "GRAY": "L"}[space(data[dst])]
        flags = ImageCms.Flags.NOOPTIMIZE
        if bpc:
            flags |= ImageCms.Flags.BLACKPOINTCOMPENSATION
        try:
            t = ImageCms.buildTransform(
                ImageCms.ImageCmsProfile(path(src)), ImageCms.ImageCmsProfile(path(dst)),
                mode, dmode, renderingIntent=ImageCms.Intent(intent), flags=flags)
        except ImageCms.PyCMSError as e:
            print("LittleCMS refuses", src, "to", dst, intent, bpc, ":", e)
            continue
        im = Image.frombytes(mode, (len(px) // len(mode), 1), px)
        res = ImageCms.applyTransform(im, t).tobytes()
        # How far LittleCMS's own optimised tables are from its exact
        # pipeline: the yardstick for ours.
        t = ImageCms.buildTransform(
            ImageCms.ImageCmsProfile(path(src)), ImageCms.ImageCmsProfile(path(dst)),
            mode, dmode, renderingIntent=ImageCms.Intent(intent), flags=flags & ~ImageCms.Flags.NOOPTIMIZE)
        opt = ImageCms.applyTransform(im, t).tobytes()
        out.append({"src": src, "dst": dst, "intent": intent, "bpc": bpc,
                    "optimized": max(abs(a - b) for a, b in zip(res, opt)),
                    "out": base64.b64encode(res).decode("ascii")})
    with open(os.path.join(HERE, "littlecms.json"), "w") as f:
        json.dump({"lcms": ImageCms.core.littlecms_version, "profiles": profiles, "cases": out}, f, indent=0)
        f.write("\n")


main()
