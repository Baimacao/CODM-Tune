"""Generate the application ICO from the real KRM262 silhouette.

Run with the bundled Python. It writes every size Windows asks for, including
the 20/40/96 px frames Explorer uses at 125%%/150%% DPI; resizing only one
source frame makes those slots keep the old icon cache entry.
"""
from PIL import Image, ImageDraw
from pathlib import Path

root = Path(__file__).resolve().parents[1]
src = Image.open(root / "internal/appui/logo-ink.png").convert("RGBA")
alpha = src.getchannel("A")
box = alpha.getbbox()
if not box:
    raise SystemExit("logo-ink.png is empty")
crop = src.crop(box)

base = Image.new("RGBA", (256, 256), (0, 0, 0, 0))
d = ImageDraw.Draw(base)
d.rounded_rectangle((4, 4, 252, 252), radius=44, fill=(233, 231, 225, 255), outline=(38, 36, 33, 255), width=10)
d.polygon([(188, 4), (252, 4), (252, 68)], fill=(214, 69, 69, 255))
gun = crop.rotate(22, expand=True, resample=Image.BICUBIC)
s = 226.0 / max(gun.size)
gun = gun.resize((max(1, int(gun.width * s)), max(1, int(gun.height * s))), Image.LANCZOS)
base.alpha_composite(gun, ((256 - gun.width) // 2, (256 - gun.height) // 2 + 6))

sizes = [16, 20, 24, 32, 40, 48, 64, 96, 128, 256]
out = root / "internal/win32/app.ico"
base.save(out, format="ICO", sizes=[(s, s) for s in sizes])
print("wrote", out, out.stat().st_size, "bytes")
