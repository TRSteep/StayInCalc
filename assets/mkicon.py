import sys
from PIL import Image, ImageDraw, ImageFont
out = sys.argv[1]
S = 1024
img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
# фон: скруглённый квадрат с вертикальным градиентом
bg = Image.new("RGBA", (S, S))
top, bot = (37, 99, 235), (13, 148, 136)
for y in range(S):
    t = y / (S - 1)
    c = tuple(int(top[i] * (1 - t) + bot[i] * t) for i in range(3)) + (255,)
    ImageDraw.Draw(bg).line([(0, y), (S, y)], fill=c)
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle([40, 40, S - 40, S - 40], radius=200, fill=255)
img.paste(bg, (0, 0), mask)
d = ImageDraw.Draw(img)
# тень и лист календаря
x0, y0, x1, y1 = 200, 250, 824, 840
d.rounded_rectangle([x0 + 14, y0 + 22, x1 + 14, y1 + 22], radius=70, fill=(0, 0, 0, 60))
d.rounded_rectangle([x0, y0, x1, y1], radius=70, fill=(255, 255, 255, 255))
d.rounded_rectangle([x0, y0, x1, y0 + 170], radius=70, fill=(239, 68, 68, 255))
d.rectangle([x0, y0 + 100, x1, y0 + 170], fill=(239, 68, 68, 255))
# кольца
for cx in (360, 664):
    d.rounded_rectangle([cx - 30, 190, cx + 30, 330], radius=30, fill=(30, 41, 59, 255))
# число
font = ImageFont.truetype("C:/Windows/Fonts/segoeuib.ttf", 340)
txt = "90"
bb = d.textbbox((0, 0), txt, font=font)
tw, th = bb[2] - bb[0], bb[3] - bb[1]
d.text(((x0 + x1) / 2 - tw / 2 - bb[0], (y0 + 170 + y1) / 2 - th / 2 - bb[1]), txt, font=font, fill=(30, 41, 59, 255))
img.save(out + "/icon.png")
img.resize((256, 256), Image.LANCZOS).save(out + "/icon256.png")
img.save(out + "/icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
