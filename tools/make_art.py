"""Gera a arte de fundo e os recortes da marca para o app DBS.

Saidas em D:/Codes/OSInfotech/assets/brand:
  bg-dark.png / bg-light.png        fundo da janela (1920x1200)
  hero-art-dark.png / -light.png    arte do banner de boas-vindas (recorte da logo, bordas suaves)
  logo-dark-lockup.png / -light     emblema + DBS para a barra lateral (bordas suaves)
"""
import math
import random

from PIL import Image, ImageDraw, ImageFilter

ROOT = r"D:/Codes/OSInfotech/assets"
OUT = ROOT + "/brand"
W, H = 1920, 1200
RED = (229, 20, 28)
random.seed(7)


def beam_layer(color, alpha_scale, positions, blur_scale=1.0):
    """Feixes diagonais de luz ("/") com brilho."""
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    for (cx, cy, length, width, alpha, blur) in positions:
        angle = math.radians(-34)
        dx, dy = math.cos(angle) * length / 2, math.sin(angle) * length / 2
        nx, ny = -math.sin(angle) * width / 2, math.cos(angle) * width / 2
        poly = [(cx - dx - nx, cy - dy - ny), (cx + dx - nx, cy + dy - ny),
                (cx + dx + nx, cy + dy + ny), (cx - dx + nx, cy - dy + ny)]
        stroke = Image.new("RGBA", (W, H), (0, 0, 0, 0))
        ImageDraw.Draw(stroke).polygon(poly, fill=color + (int(alpha * alpha_scale),))
        if blur:
            stroke = stroke.filter(ImageFilter.GaussianBlur(blur * blur_scale))
        layer = Image.alpha_composite(layer, stroke)
    return layer


def circuit_layer(color, nodes_alpha):
    """Trilhas de circuito com nos, nos cantos."""
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)

    def trace(points, width=2):
        d.line(points, fill=color, width=width, joint="curve")
        for i in (0, len(points) - 1):
            x, y = points[i]
            d.ellipse((x - 5, y - 5, x + 5, y + 5), outline=color, width=2)

    # canto inferior esquerdo
    base_x, base_y = 40, H - 60
    for k in range(7):
        x0 = base_x + k * 38
        pts = [(x0, base_y), (x0, base_y - 120 - k * 30), (x0 + 60 + k * 10, base_y - 180 - k * 40), (x0 + 60 + k * 10, base_y - 300 - k * 20)]
        trace(pts)
    for k in range(4):
        y0 = base_y - 40 - k * 46
        pts = [(base_x + 280, y0), (base_x + 420 + k * 20, y0), (base_x + 480 + k * 20, y0 - 60), (base_x + 640, y0 - 60)]
        trace(pts)
    # canto superior direito
    bx, by = W - 60, 60
    for k in range(6):
        x0 = bx - k * 40
        pts = [(x0, by), (x0, by + 110 + k * 26), (x0 - 70 - k * 8, by + 180 + k * 34), (x0 - 70 - k * 8, by + 290 + k * 20)]
        trace(pts)
    for k in range(3):
        y0 = by + 30 + k * 48
        pts = [(bx - 300, y0), (bx - 440 - k * 20, y0), (bx - 500 - k * 20, y0 + 56), (bx - 660, y0 + 56)]
        trace(pts)
    glow = layer.filter(ImageFilter.GaussianBlur(6))
    glow.putalpha(glow.split()[3].point(lambda a: int(a * nodes_alpha)))
    return Image.alpha_composite(glow, layer)


def dots_layer(color, patches):
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    for (x, y, w, h, alpha) in patches:
        for i in range(0, w, 14):
            for j in range(0, h, 14):
                fade = 1.0 - (i / w) * 0.6
                d.ellipse((x + i, y + j, x + i + 3, y + j + 3), fill=color + (int(alpha * fade),))
    return layer


def radial_glow(color, cx, cy, radius, alpha):
    layer = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    steps = 24
    for s in range(steps, 0, -1):
        r = radius * s / steps
        a = int(alpha * (1 - s / steps) ** 1.6)
        d.ellipse((cx - r, cy - r, cx + r, cy + r), fill=color + (a,))
    return layer.filter(ImageFilter.GaussianBlur(40))


def make_background(dark=True):
    if dark:
        base = Image.new("RGBA", (W, H), (11, 11, 14, 255))
        beams = beam_layer(RED, 1.0, [
            (1500, 120, 1300, 34, 210, 22), (1620, 60, 1100, 12, 255, 4), (1720, 210, 1000, 90, 120, 44),
            (1350, 320, 900, 8, 230, 3), (1250, 40, 700, 20, 170, 16), (1560, 380, 1200, 140, 70, 70),
            (1440, 200, 1400, 4, 200, 1), (1800, 300, 600, 5, 220, 2),
            (260, 1080, 1100, 30, 200, 22), (120, 1000, 800, 10, 240, 4), (420, 1140, 900, 70, 100, 40),
            (330, 980, 1200, 4, 190, 1), (760, 60, 600, 5, 120, 2), (980, 160, 900, 3, 90, 1),
        ])
        circuits = circuit_layer((190, 24, 34, 230), 0.8)
        dots = dots_layer(RED, [(1180, 760, 220, 130, 150), (60, 620, 180, 110, 120), (1500, 480, 160, 90, 100), (900, 1040, 200, 100, 90)])
        glows = Image.alpha_composite(radial_glow(RED, 1780, 40, 760, 170), radial_glow(RED, 120, 1180, 640, 120))
        glows = Image.alpha_composite(glows, radial_glow((120, 8, 12), 1100, 120, 900, 90))
        vignette = Image.new("RGBA", (W, H), (0, 0, 0, 0))
        img = base
        for layer in (glows, beams, circuits, dots, vignette):
            img = Image.alpha_composite(img, layer)
        # escurece levemente a faixa central onde ficam os cartoes
        shade = Image.new("RGBA", (W, H), (0, 0, 0, 0))
        ImageDraw.Draw(shade).rectangle((0, 0, W, H), fill=(6, 6, 8, 40))
        img = Image.alpha_composite(img, shade)
    else:
        base = Image.new("RGBA", (W, H), (245, 246, 248, 255))
        beams = beam_layer(RED, 0.42, [
            (1500, 120, 1300, 28, 120, 26), (1620, 60, 1100, 10, 190, 6), (1720, 210, 1000, 60, 60, 40),
            (1350, 320, 900, 6, 150, 3), (260, 1080, 1100, 24, 110, 24), (120, 1000, 800, 8, 170, 5),
            (420, 1140, 900, 50, 50, 36),
        ])
        circuits = circuit_layer((225, 70, 80, 120), 0.4)
        dots = dots_layer(RED, [(1180, 760, 220, 130, 60), (60, 620, 180, 110, 50), (1500, 480, 160, 90, 40)])
        glows = Image.alpha_composite(radial_glow((255, 120, 128), 1780, 40, 620, 80), radial_glow((255, 120, 128), 120, 1180, 560, 60))
        img = base
        for layer in (glows, beams, circuits, dots):
            img = Image.alpha_composite(img, layer)
    return img.convert("RGB")


def feather(img, margin):
    """Aplica esmaecimento nas bordas (alpha) para o recorte fundir com o fundo."""
    w, h = img.size
    mask = Image.new("L", (w, h), 0)
    d = ImageDraw.Draw(mask)
    steps = margin
    for s in range(steps):
        a = int(255 * (s / steps) ** 0.8)
        d.rounded_rectangle((s, s, w - 1 - s, h - 1 - s), radius=margin, fill=a)
    d.rounded_rectangle((margin, margin, w - 1 - margin, h - 1 - margin), radius=margin // 2, fill=255)
    mask = mask.filter(ImageFilter.GaussianBlur(margin / 3))
    out = img.convert("RGBA")
    out.putalpha(mask)
    return out


def make_crops(src_path, suffix):
    src = Image.open(src_path).convert("RGB")
    # arte do banner: aparelhos + controle + chave (sem o texto DBS)
    hero = src.crop((60, 290, 605, 850))
    hero = feather(hero, 56)
    hero = hero.resize((545, 560), Image.LANCZOS)
    hero.save(f"{OUT}/hero-art-{suffix}.png", optimize=True)
    # lockup: emblema + DBS + assistencia tecnica
    lockup = src.crop((40, 285, 1230, 865))
    lockup = feather(lockup, 90)
    lockup = lockup.resize((820, int(820 * lockup.size[1] / lockup.size[0])), Image.LANCZOS)
    lockup.save(f"{OUT}/logo-{suffix}-lockup.png", optimize=True)
    print(suffix, hero.size, lockup.size)


make_background(True).save(f"{OUT}/bg-dark.png", optimize=True)
make_background(False).save(f"{OUT}/bg-light.png", optimize=True)
make_crops(ROOT + "/logo.png", "dark")
make_crops(ROOT + "/lightlogo.png", "light")
print("ok")
