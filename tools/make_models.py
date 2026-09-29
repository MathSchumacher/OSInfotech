"""Gera assets/models.tsv, o catalogo de modelos usado no campo "Marca e modelo".

Fonte: a lista de dispositivos que o Google publica para desenvolvedores
(https://storage.googleapis.com/play_public/supported_devices.csv). Sao 54 mil
linhas de marca e nome comercial. O que interessa e so o nome do aparelho, que e
informacao factual.

O arquivo bruto tem muita coisa que a loja nunca vai ver: 3.842 marcas, decoders,
placas de desenvolvimento, aparelhos que nunca foram vendidos no Brasil. Aqui ele
e filtrado para as marcas que aparecem no balcao, limpo de repeticao de marca no
nome ("Redmi Redmi 9" vira "Redmi 9") e somado a lista de iPhones, que nao esta
na fonte porque a Apple nao publica na Play.

O resultado fica em assets/models.tsv e vai embutido no executavel: nada disso
depende de internet em uso.

Uso:
    python tools/make_models.py                # baixa e gera
    python tools/make_models.py caminho.csv    # usa um csv ja baixado
"""

from __future__ import annotations

import csv
import io
import os
import sys
import unicodedata
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "assets", "models.tsv")
SOURCE = "https://storage.googleapis.com/play_public/supported_devices.csv"

# Marcas que a loja atende. O nome a esquerda e como aparece na fonte; o da
# direita e como o operador escreve.
BRANDS = {
    "Samsung": "Samsung",
    "Motorola": "Motorola",
    "Xiaomi": "Xiaomi",
    "Redmi": "Redmi",
    "POCO": "POCO",
    "LGE": "LG",
    "TCT (Alcatel)": "Alcatel",
    "Positivo": "Positivo",
    "Multilaser": "Multilaser",
    "ASUS": "Asus",
    "asus": "Asus",
    "Nokia": "Nokia",
    "HMD Global": "Nokia",
    "TCL": "TCL",
    "realme": "realme",
    "OnePlus": "OnePlus",
    "Infinix": "Infinix",
    "Philco": "Philco",
    "Lenovo": "Lenovo",
    "Sony": "Sony",
    "Huawei": "Huawei",
    "Google": "Google",
}

# A Apple nao publica lista. Estes sao os aparelhos que ainda chegam para
# conserto; modelos mais novos entram aqui a mao.
APPLE = [
    "iPhone 6", "iPhone 6 Plus", "iPhone 6s", "iPhone 6s Plus", "iPhone SE",
    "iPhone 7", "iPhone 7 Plus", "iPhone 8", "iPhone 8 Plus", "iPhone X",
    "iPhone XR", "iPhone XS", "iPhone XS Max", "iPhone SE 2", "iPhone 11",
    "iPhone 11 Pro", "iPhone 11 Pro Max", "iPhone 12 mini", "iPhone 12",
    "iPhone 12 Pro", "iPhone 12 Pro Max", "iPhone 13 mini", "iPhone 13",
    "iPhone 13 Pro", "iPhone 13 Pro Max", "iPhone SE 3", "iPhone 14",
    "iPhone 14 Plus", "iPhone 14 Pro", "iPhone 14 Pro Max", "iPhone 15",
    "iPhone 15 Plus", "iPhone 15 Pro", "iPhone 15 Pro Max", "iPhone 16",
    "iPhone 16 Plus", "iPhone 16 Pro", "iPhone 16 Pro Max", "iPhone 16e",
    "iPad", "iPad Air", "iPad Pro", "iPad mini",
]

# Marcas que a loja mais atende, medido no proprio banco. Entram primeiro na
# ordenacao: sem isso um Infinix aparece na frente de um Redmi em "note 11".
PRIORITY = {"Samsung": 0, "Apple": 0, "Motorola": 0, "Xiaomi": 1, "Redmi": 1,
            "POCO": 1, "LG": 2, "Positivo": 2, "Multilaser": 2}

# Nomes que nao sao aparelho de balcao.
SKIP_WORDS = ("tv", "box", "stick", "watch", "band", "tab active", "chromebook",
              "notebook", "monitor", "projector", "soundbar", "smartcam")


def fold(text: str) -> str:
    text = unicodedata.normalize("NFD", text.lower())
    return "".join(c for c in text if unicodedata.category(c) != "Mn")


def display_name(brand: str, name: str) -> str:
    """Nome como o operador ve: sem repetir a marca, sem perder o modelo.

    "Redmi" + "Redmi 9" vira "Redmi 9", e nao "Redmi Redmi 9" nem so "9".
    """
    name = " ".join(name.split())
    if fold(name).startswith(fold(brand)):
        return name
    return f"{brand} {name}"


def usable(display: str) -> bool:
    if not display or len(display) > 44 or len(display) < 4:
        return False
    low = fold(display)
    if any(word in low for word in SKIP_WORDS):
        return False
    return True


def load_source(path: str | None) -> list[list[str]]:
    if path:
        raw = open(path, "rb").read()
    else:
        print("baixando", SOURCE)
        with urllib.request.urlopen(SOURCE, timeout=120) as response:
            raw = response.read()
    text = raw.decode("utf-16")
    return list(csv.reader(io.StringIO(text)))[1:]


def main() -> None:
    rows = load_source(sys.argv[1] if len(sys.argv) > 1 else None)
    models: set[tuple[str, str]] = set()
    for row in rows:
        if len(row) < 4:
            continue
        source_brand, name = row[0].strip(), row[1].strip()
        brand = BRANDS.get(source_brand)
        if not brand:
            continue
        display = display_name(brand, name)
        if usable(display):
            models.add((brand, display))
    for name in APPLE:
        models.add(("Apple", name))

    ordered = sorted(models, key=lambda item: (PRIORITY.get(item[0], 5), item[0], item[1]))
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8", newline="\n") as handle:
        handle.write("# marca\tmodelo — gerado por tools/make_models.py\n")
        for brand, name in ordered:
            handle.write(f"{brand}\t{name}\n")
    size = os.path.getsize(OUT)
    print(f"gerado {OUT}: {len(ordered)} modelos, {size // 1024} KB")


if __name__ == "__main__":
    main()
