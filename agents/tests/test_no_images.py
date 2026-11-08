"""ADR-0027: no image is ever parsed. The PDFs hold text only; Pillow, a native dependency
of fpdf2, must stay unreachable. Ruff bans importing PIL and fpdf2's HTML and SVG modules
(TID251); this test bans the calls that decode images, anywhere in the agents' code."""

import ast
from pathlib import Path

SRC = Path(__file__).parents[1] / "src"
# fpdf2 methods that load or decode an image: image() itself, write_html() (<img>),
# set_page_background() (an image path), and the SVG and template entry points.
BANNED_CALLS = {"image", "write_html", "set_page_background", "template", "svg"}
BANNED_MODULES = ("PIL", "fpdf.html", "fpdf.svg", "fpdf.image_parsing", "fpdf.template")


def offences(tree: ast.AST, name: str) -> list[str]:
    out: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            f = node.func
            called = f.attr if isinstance(f, ast.Attribute) else getattr(f, "id", "")
            if called in BANNED_CALLS:
                out.append(f"{name}:{node.lineno} calls {called}()")
        elif isinstance(node, ast.Import | ast.ImportFrom):
            mods = (
                [a.name for a in node.names]
                if isinstance(node, ast.Import)
                else [node.module or ""]
            )
            out += [
                f"{name}:{node.lineno} imports {m}"
                for m in mods
                if any(m == b or m.startswith(b + ".") for b in BANNED_MODULES)
            ]
    return out


def test_no_image_is_ever_parsed() -> None:
    found = [
        o
        for path in sorted(SRC.rglob("*.py"))
        for o in offences(ast.parse(path.read_text()), str(path.relative_to(SRC)))
    ]
    assert found == []


def test_the_rule_bites() -> None:
    bad = (
        "import PIL.Image\nfrom fpdf.html import HTMLMixin\n"
        "d.image('x.png')\nd.write_html('<img>')\n"
    )
    assert len(offences(ast.parse(bad), "x")) == 4
