"""Writes cmyk.jpg, a CMYK JPEG of four flat patches without a profile, and
prints what LittleCMS makes of its patches with cms/testdata/profiles/
cmyk-lut16.icc, perceptual, into sRGB. TestCMYKJPEGWithProfile embeds that
profile and compares.

Go's image/jpeg writes no CMYK, so Pillow does, the way Photoshop does: Adobe
APP14 marker, inverted samples. Run with Pillow 11:

    python3 asset/testdata/cmyk.py
"""

import io
import os

from PIL import Image, ImageCms

here = os.path.dirname(os.path.abspath(__file__))
patches = [(0, 0, 0, 0), (255, 0, 0, 0), (0, 0, 0, 128), (40, 120, 200, 30)]

img = Image.new("CMYK", (32, 16))
for i, c in enumerate(patches):
    img.paste(c, (8 * i, 0, 8 * i + 8, 16))
img.save(os.path.join(here, "cmyk.jpg"), quality=100, subsampling=0)

with open(os.path.join(here, "..", "..", "cms", "testdata", "profiles", "cmyk-lut16.icc"), "rb") as f:
    icc = f.read()
back = Image.open(os.path.join(here, "cmyk.jpg"))
src = ImageCms.ImageCmsProfile(io.BytesIO(icc))
dst = ImageCms.createProfile("sRGB")
rgb = ImageCms.profileToProfile(back, src, dst, renderingIntent=ImageCms.Intent.PERCEPTUAL, outputMode="RGB")
for i in range(len(patches)):
    print(back.getpixel((8 * i + 4, 8)), rgb.getpixel((8 * i + 4, 8)))
