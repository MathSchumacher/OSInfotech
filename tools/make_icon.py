"""Gera o icone do app a partir de assets/logo.png.

O icone do Windows carrega varios tamanhos e o sistema escolhe o que couber:
16 e 32 px na barra de tarefas e no titulo da janela, 48 no atalho da area de
trabalho, 256 na visualizacao grande do Explorer. Um unico desenho nao atende
todos: o emblema inteiro (celular + monitor + controle + chave + notebook) e
bonito em 256 px e vira um borrao vermelho em 16.

Por isso o .ico e "responsivo": os tamanhos pequenos usam um recorte fechado no
controle, que tem silhueta forte e sobrevive a reducao, e os grandes usam o
emblema completo. E a mesma arte, so que mais perto.

A palavra DBS foi testada nos tamanhos pequenos e descartada: com tres letras em
16 px cada uma fica com 4 px de largura e o resultado le "nBC".

Uso:  python tools/make_icon.py
Saida: assets/icons/dbs-*.png, assets/icons/dbs.ico, assets/icons/dbs.png
"""

from __future__ import annotations

import io
import os
import struct

from PIL import Image, ImageEnhance

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOGO = os.path.join(ROOT, "assets", "logo.png")
OUT = os.path.join(ROOT, "assets", "icons")

# Recortes medidos no logo de 1269x1240. Conferir se o logo for trocado.
EMBLEM_BOX = (20, 280, 600, 860)   # emblema completo, sem alcancar o texto
GAMEPAD_BOX = (227, 430, 517, 720)  # controle e a faixa vermelha

SMALL_SIZES = (16, 24, 32, 48, 64)
LARGE_SIZES = (128, 256)


def downscale(img: Image.Image, size: int) -> Image.Image:
    """Reduz em dois passos e devolve o contraste que a reducao come."""
    step = img.resize((size * 3, size * 3), Image.LANCZOS)
    step = ImageEnhance.Contrast(step).enhance(1.15)
    step = ImageEnhance.Color(step).enhance(1.20)
    small = step.resize((size, size), Image.LANCZOS)
    return ImageEnhance.Sharpness(small).enhance(1.40)


def dib_entry(img: Image.Image) -> bytes:
    """Imagem no formato antigo do .ico (BMP de 32 bits, de baixo para cima).

    Os tamanhos pequenos precisam vir assim: o Windows so aceita PNG dentro do
    .ico a partir de 128 px com seguranca.
    """
    w, h = img.size
    px = img.convert("RGBA").load()
    xor = bytearray()
    for y in range(h - 1, -1, -1):
        for x in range(w):
            r, g, b, a = px[x, y]
            xor += bytes((b, g, r, a))
    row = ((w + 31) // 32) * 4       # mascara 1 bpp, linhas alinhadas em 4 bytes
    and_mask = bytes(row * h)
    header = struct.pack(
        "<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, len(xor) + len(and_mask), 0, 0, 0, 0
    )
    return header + bytes(xor) + and_mask


def write_ico(path: str, images: list[Image.Image]) -> None:
    """Monta o .ico com uma arte diferente por tamanho.

    O PIL nao faz isso: ele reduz a mesma imagem para todos os tamanhos.
    """
    payloads = []
    for img in images:
        if img.width >= 128:
            buf = io.BytesIO()
            img.save(buf, format="PNG")
            payloads.append(buf.getvalue())
        else:
            payloads.append(dib_entry(img))

    out = struct.pack("<HHH", 0, 1, len(images))
    offset = 6 + 16 * len(images)
    for img, data in zip(images, payloads):
        w = 0 if img.width >= 256 else img.width
        h = 0 if img.height >= 256 else img.height
        out += struct.pack("<BBBBHHII", w, h, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
    with open(path, "wb") as fh:
        fh.write(out + b"".join(payloads))


def main() -> None:
    os.makedirs(OUT, exist_ok=True)
    logo = Image.open(LOGO).convert("RGB")
    emblem = logo.crop(EMBLEM_BOX)
    gamepad = logo.crop(GAMEPAD_BOX)

    frames: list[Image.Image] = []
    for size in SMALL_SIZES:
        img = downscale(gamepad, size)
        img.save(os.path.join(OUT, f"dbs-{size}.png"))
        frames.append(img)
    for size in LARGE_SIZES:
        img = emblem.resize((size, size), Image.LANCZOS)
        img.save(os.path.join(OUT, f"dbs-{size}.png"))
        frames.append(img)

    # 512 px para a janela do app, o recibo impresso e a loja Android.
    emblem.resize((512, 512), Image.LANCZOS).save(os.path.join(OUT, "dbs-512.png"))
    emblem.resize((512, 512), Image.LANCZOS).save(os.path.join(OUT, "dbs.png"))

    ico_path = os.path.join(OUT, "dbs.ico")
    write_ico(ico_path, frames)
    print("gerado:", ico_path, "com", len(frames), "tamanhos")


if __name__ == "__main__":
    main()
