"""The 3D mascot (BRD-001): turns the supplied robot into the models the clients load.

The source is `Postitheka_Mascot_Web_NoTablet.glb` from the owner's kit: Y-up, +Z front, one
mesh per material, no textures, no rig. It is untrusted input. Copy only that file into an
empty directory of its own, never run the kit's scripts, and run this with `python3 -I`, the
path passed as an argument. Standard library only, except the USDZ step, which calls Apple's
`usdcat` and `usdzip` (/usr/bin on macOS); without them the .usda is kept and no .usdz made.

  1. checks the file: glTF 2 magic and lengths, a size cap, no node transforms, no textures,
     triangles only, every index inside its mesh, every coordinate finite;
  2. paints each material with a brand token read from `web/src/styles.css`, so the model
     cannot drift from the palette (the PAINT table below; a material missing from it aborts);
  3. makes it one unit tall, standing on the origin, centred on X and Z, facing +Z, which is
     what `scene.ts` and `Mascot3DView.swift` frame;
  4. writes `mascot.glb` for the web, positions as int16 and normals as int8
     (`KHR_mesh_quantization`), and `Mascot.usdz` for iOS;
  5. prints what it made: KB, meshes, draw calls, triangles.

Usage, from the repo root:

    python3 -I brand/tools/build_mascot3d.py \\
        --src ~/Work/production/apps/postitheka-3d-src/Postitheka_Mascot_Web_NoTablet.glb \\
        --out out/mascot3d --ios ios/Postitheka/Mascot3D

Both outputs are git-ignored; rerun this after any change to the paint or the source.
"""

import argparse
import json
import math
import re
import shutil
import struct
import subprocess
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
TOKENS_CSS = REPO / "web" / "src" / "styles.css"
MAX_SOURCE_BYTES = 32 << 20

# Each source material, by name, becomes a brand token. Metalness and roughness are the
# source's unless given; a material that glowed in the source glows in its token, at the
# source's strength unless `glow` gives one (0 for none). The lights (eyes, stabiliser) have a
# night surface and only glow in their token: the postmark cyan is pale, and as a lit surface
# it washed out to white under the web's tone mapping. The robot reads as the key art's: a gold body, night-dark visor and joints, the
# hologram's cyan in the eyes and the stabiliser (the approved postman's eyes are cyan too).
PAINT = {
    # `flip`: the mesh is inside out in the source, triangles and normals both facing in (of the
    # vertices in front of the model, 98% of the shell's and 97% of the eyes' point backwards,
    # measured 2026-11-13). three drew it anyway as doubleSided; RealityKit culls it.
    "ceramic": {"token": "--gold", "metallic": 0.85, "roughness": 0.32, "flip": True},
    "ceramic_light": {"token": "--gold-bright", "metallic": 0.85, "roughness": 0.28},
    "ceramic_shadow": {"token": "--gold", "metallic": 0.7, "roughness": 0.45},
    "edge_silver": {"token": "--gold-bright", "metallic": 0.9, "roughness": 0.2},
    "titanium": {"token": "--muted"},
    "graphite": {"token": "--sheet"},
    "gunmetal": {"token": "--stamp"},
    "matte_black": {"token": "--paper"},
    "rubber": {"token": "--paper"},
    "visor": {"token": "--paper", "glow": 0},
    "visor_reflection": {"token": "--sheet", "glow": 0},
    "blue_dark": {"token": "--postmark", "surface": "--paper", "glow": 0.5, "flip": True},
    "blue_soft": {"token": "--postmark", "surface": "--paper", "glow": 1.0, "flip": True},
    "blue": {"token": "--postmark", "surface": "--paper", "glow": 0.8},
    "orange": {"token": "--gold"},
}

# RealityKit's virtual camera has no environment for metal to reflect (the web lights it with
# three's RoomEnvironment), so at the web's metalness the gold body renders near black on iOS.
# The USD keeps the colour and caps the metalness.
USD_MAX_METALLIC = 0.3

FLOAT, UINT16, UINT32, INT16, INT8 = 5126, 5123, 5125, 5122, 5120


def fail(message):
    raise SystemExit(f"build_mascot3d: {message}")


# ---------------------------------------------------------------- tokens


def read_tokens():
    """The `--name: #rrggbb` colours on `:root` in styles.css, as linear RGB (glTF's space)."""
    css = TOKENS_CSS.read_text()
    root = re.search(r":root\s*\{(.*?)\}", css, re.S)
    if not root:
        fail(f"no :root block in {TOKENS_CSS}")
    tokens = {}
    for name, hex_ in re.findall(r"(--[\w-]+)\s*:\s*#([0-9a-fA-F]{6})\s*;", root.group(1)):
        srgb = [int(hex_[i : i + 2], 16) / 255 for i in (0, 2, 4)]
        tokens[name] = [c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4 for c in srgb]
    return tokens


# ---------------------------------------------------------------- reading the source


def read_glb(path):
    data = Path(path).read_bytes()
    if len(data) > MAX_SOURCE_BYTES:
        fail(f"{path} is {len(data)} bytes, over the {MAX_SOURCE_BYTES} cap")
    if len(data) < 20:
        fail(f"{path} is too short to be a GLB")
    magic, version, length = struct.unpack_from("<4sII", data, 0)
    if magic != b"glTF" or version != 2 or length != len(data):
        fail(f"{path} is not a glTF 2 binary (magic {magic!r}, version {version}, length {length})")
    chunks, offset = {}, 12
    while offset < len(data):
        if offset + 8 > len(data):
            fail("truncated chunk header")
        size, kind = struct.unpack_from("<I4s", data, offset)
        if offset + 8 + size > len(data):
            fail("chunk runs past the end of the file")
        chunks[kind] = data[offset + 8 : offset + 8 + size]
        offset += 8 + size
    if b"JSON" not in chunks or b"BIN\0" not in chunks:
        fail("missing JSON or BIN chunk")
    gltf = json.loads(chunks[b"JSON"])
    blob = chunks[b"BIN\0"]

    if gltf.get("images") or gltf.get("textures") or gltf.get("extensionsRequired"):
        fail("the source has textures or required extensions; this script expects neither")
    for node in gltf.get("nodes", []):
        if any(key in node for key in ("matrix", "translation", "rotation", "scale")):
            fail(f"node {node.get('name')!r} carries a transform; this script expects none")

    def accessor(index, want_type, want_components):
        acc = gltf["accessors"][index]
        if acc["type"] != want_type or acc["componentType"] not in want_components or acc.get("sparse"):
            fail(f"accessor {index} is {acc['type']}/{acc['componentType']}, expected {want_type}")
        view = gltf["bufferViews"][acc["bufferView"]]
        if view.get("buffer", 0) != 0 or view.get("byteStride"):
            fail(f"accessor {index}: only tightly packed data in the GLB's own buffer is read")
        width = {"SCALAR": 1, "VEC3": 3}[want_type]
        fmt = {FLOAT: "f", UINT32: "I", UINT16: "H"}[acc["componentType"]]
        start = view.get("byteOffset", 0) + acc.get("byteOffset", 0)
        count = acc["count"] * width
        end = start + count * struct.calcsize(fmt)
        if end > view.get("byteOffset", 0) + view["byteLength"] or end > len(blob):
            fail(f"accessor {index} runs past its buffer view")
        values = struct.unpack_from(f"<{count}{fmt}", blob, start)
        return [values[i : i + width] for i in range(0, count, width)] if width > 1 else list(values)

    meshes = []
    for node in gltf["nodes"]:
        if "mesh" not in node:
            continue
        for primitive in gltf["meshes"][node["mesh"]]["primitives"]:
            if primitive.get("mode", 4) != 4 or "indices" not in primitive:
                fail("only indexed triangles are read")
            material = gltf["materials"][primitive["material"]]
            points = accessor(primitive["attributes"]["POSITION"], "VEC3", {FLOAT})
            normals = accessor(primitive["attributes"]["NORMAL"], "VEC3", {FLOAT})
            indices = accessor(primitive["indices"], "SCALAR", {UINT16, UINT32})
            if len(normals) != len(points) or len(indices) % 3:
                fail(f"{material['name']}: normals or triangles do not match the points")
            if any(i >= len(points) for i in indices):
                fail(f"{material['name']}: an index points past the mesh")
            if not all(math.isfinite(v) for p in points + normals for v in p):
                fail(f"{material['name']}: a coordinate is not finite")
            meshes.append({"material": material, "points": points, "normals": normals, "indices": indices})
    if not meshes:
        fail("no meshes")
    return meshes


# ---------------------------------------------------------------- shaping and painting


def normalise(meshes):
    """One unit tall, standing on the origin, centred on X and Z."""
    every = [p for m in meshes for p in m["points"]]
    low = [min(p[i] for p in every) for i in range(3)]
    high = [max(p[i] for p in every) for i in range(3)]
    height = high[1] - low[1]
    if height <= 0:
        fail("the model has no height")
    centre = [(low[0] + high[0]) / 2, low[1], (low[2] + high[2]) / 2]
    for mesh in meshes:
        mesh["points"] = [tuple((p[i] - centre[i]) / height for i in range(3)) for p in mesh["points"]]
        mesh["normals"] = [unit(n) for n in mesh["normals"]]
    return height


def orient(mesh):
    """Turns round every triangle whose winding disagrees with its own vertex normals, and
    returns how many. The source mixes the two, which a renderer drawing both sides (three,
    with doubleSided) hides and RealityKit, which culls back faces regardless, does not."""
    points, normals, indices = mesh["points"], mesh["normals"], mesh["indices"]
    turned = 0
    for t in range(0, len(indices), 3):
        a, b, c = indices[t : t + 3]
        ab = [points[b][i] - points[a][i] for i in range(3)]
        ac = [points[c][i] - points[a][i] for i in range(3)]
        face = (ab[1] * ac[2] - ab[2] * ac[1], ab[2] * ac[0] - ab[0] * ac[2], ab[0] * ac[1] - ab[1] * ac[0])
        smooth = [normals[a][i] + normals[b][i] + normals[c][i] for i in range(3)]
        if sum(face[i] * smooth[i] for i in range(3)) < 0:
            indices[t + 1], indices[t + 2] = c, b
            turned += 1
    return turned


def unit(v):
    length = math.sqrt(sum(c * c for c in v))
    return tuple(c / length for c in v) if length > 0 else (0.0, 1.0, 0.0)


def paint(meshes, tokens):
    looks = {}
    for mesh in meshes:
        source = mesh["material"]
        name = source["name"]
        rule = PAINT.get(name)
        if rule is None:
            fail(f"material {name!r} is not in PAINT; give it a token")
        if rule.get("flip"):
            i = mesh["indices"]
            mesh["indices"] = [v for t in range(0, len(i), 3) for v in (i[t], i[t + 2], i[t + 1])]
            mesh["normals"] = [tuple(-c for c in n) for n in mesh["normals"]]
        for key in ("token", "surface"):
            if key in rule and rule[key] not in tokens:
                fail(f"{rule[key]} is not a colour on :root in {TOKENS_CSS}")
        pbr = source.get("pbrMetallicRoughness", {})
        colour = tokens[rule["token"]]
        strength = rule.get("glow", max(source.get("emissiveFactor", [0, 0, 0])))
        looks[name] = {
            "token": rule["token"],
            "color": tokens[rule.get("surface", rule["token"])],
            "metallic": rule.get("metallic", pbr.get("metallicFactor", 1.0)),
            "roughness": rule.get("roughness", pbr.get("roughnessFactor", 1.0)),
            "emissive": [c * strength for c in colour],
        }
    return looks


# ---------------------------------------------------------------- writing the GLB


def write_glb(path, meshes, looks):
    blob = bytearray()
    views, accessors = [], []

    def add(data, target, stride=None):
        blob.extend(b"\0" * (-len(blob) % 4))
        view = {"buffer": 0, "byteOffset": len(blob), "byteLength": len(data), "target": target}
        if stride:
            view["byteStride"] = stride
        blob.extend(data)
        views.append(view)
        return len(views) - 1

    # One scale for the whole model, carried by the node: positions are stored as int16 parts
    # of it, which glTF's KHR_mesh_quantization reads back as floats.
    reach = max(abs(c) for m in meshes for p in m["points"] for c in p)
    gl_meshes, gl_materials, nodes = [], [], []
    for index, mesh in enumerate(meshes):
        name = mesh["material"]["name"]
        look = looks[name]
        quantised = [tuple(round(c / reach * 32767) for c in p) for p in mesh["points"]]
        # Attribute elements start on 4-byte boundaries: three int16 plus padding, three int8
        # plus padding.
        position = add(b"".join(struct.pack("<3hxx", *q) for q in quantised), 34962, 8)
        normal = add(
            b"".join(struct.pack("<3bx", *(round(c * 127) for c in n)) for n in mesh["normals"]), 34962, 4
        )
        wide = len(mesh["points"]) > 65535
        indices = add(struct.pack(f"<{len(mesh['indices'])}{'I' if wide else 'H'}", *mesh["indices"]), 34963)
        mins = [min(q[i] for q in quantised) for i in range(3)]
        maxs = [max(q[i] for q in quantised) for i in range(3)]
        accessors += [
            {"bufferView": position, "componentType": INT16, "normalized": True, "count": len(quantised),
             "type": "VEC3", "min": [v / 32767 for v in mins], "max": [v / 32767 for v in maxs]},
            {"bufferView": normal, "componentType": INT8, "normalized": True, "count": len(quantised), "type": "VEC3"},
            {"bufferView": indices, "componentType": UINT32 if wide else UINT16, "count": len(mesh["indices"]),
             "type": "SCALAR"},
        ]  # fmt: skip
        gl_materials.append({
            "name": name,
            "pbrMetallicRoughness": {"baseColorFactor": [*look["color"], 1.0], "metallicFactor": look["metallic"],
                                     "roughnessFactor": look["roughness"]},
            "emissiveFactor": look["emissive"],
            # The source's eye meshes face backwards (their normals point -Z), so drawn
            # one-sided the eyes vanish; the kit's own renders drew both sides.
            "doubleSided": True,
        })  # fmt: skip
        base = len(accessors) - 3
        gl_meshes.append({
            "name": name,
            "primitives": [{"attributes": {"POSITION": base, "NORMAL": base + 1}, "indices": base + 2, "material": index}],
        })  # fmt: skip
        nodes.append({"name": f"Mascot_{name}", "mesh": index})

    nodes.append({"name": "Mascot", "scale": [reach] * 3, "children": list(range(len(meshes)))})
    gltf = {
        "asset": {"version": "2.0", "generator": "postitheka brand/tools/build_mascot3d.py"},
        "extensionsUsed": ["KHR_mesh_quantization"],
        "extensionsRequired": ["KHR_mesh_quantization"],
        "scene": 0,
        "scenes": [{"nodes": [len(nodes) - 1]}],
        "nodes": nodes,
        "meshes": gl_meshes,
        "materials": gl_materials,
        "accessors": accessors,
        "bufferViews": views,
        "buffers": [{"byteLength": len(blob)}],
    }
    text = json.dumps(gltf, separators=(",", ":")).encode()
    text += b" " * (-len(text) % 4)
    blob.extend(b"\0" * (-len(blob) % 4))
    total = 12 + 8 + len(text) + 8 + len(blob)
    Path(path).write_bytes(
        struct.pack("<4sII", b"glTF", 2, total)
        + struct.pack("<I4s", len(text), b"JSON") + text
        + struct.pack("<I4s", len(blob), b"BIN\0") + bytes(blob)
    )  # fmt: skip
    return total


# ---------------------------------------------------------------- writing the USDZ


def write_usdz(path, meshes, looks):
    """`Mascot.usdz`: the same model, floats, one Mesh per material under the Xform `Mascot`.
    Returns the file written: the .usdz, or the .usda when Apple's USD tools are missing."""

    def tuples(values, digits):
        return ", ".join("(" + ", ".join(f"{v:.{digits}g}" for v in t) + ")" for t in values)

    scene = "Mascot"
    lines = ["#usda 1.0", "(", f'    defaultPrim = "{scene}"', "    metersPerUnit = 1", '    upAxis = "Y"', ")", "",
             f'def Xform "{scene}" (', '    kind = "component"', ")", "{", '    def Scope "Materials"', "    {"]  # fmt: skip
    for name, look in looks.items():
        root = f"/{scene}/Materials/{name}"
        lines += [
            f'        def Material "{name}"', "        {",
            f"            token outputs:surface.connect = <{root}/Surface.outputs:surface>",
            '            def Shader "Surface"', "            {",
            '                uniform token info:id = "UsdPreviewSurface"',
            f"                color3f inputs:diffuseColor = {tuples([look['color']], 5)}",
            f"                float inputs:metallic = {min(look['metallic'], USD_MAX_METALLIC)}",
            f"                float inputs:roughness = {look['roughness']}",
            f"                color3f inputs:emissiveColor = {tuples([look['emissive']], 5)}",
            "                token outputs:surface", "            }", "        }",
        ]  # fmt: skip
    lines.append("    }")
    for mesh in meshes:
        name = mesh["material"]["name"]
        faces = len(mesh["indices"]) // 3
        lines += [
            f'    def Mesh "{scene}_{name}" (', '        prepend apiSchemas = ["MaterialBindingAPI"]', "    )", "    {",
            "        uniform bool doubleSided = 1",
            f"        int[] faceVertexCounts = [{', '.join('3' for _ in range(faces))}]",
            f"        int[] faceVertexIndices = [{', '.join(str(i) for i in mesh['indices'])}]",
            f"        point3f[] points = [{tuples(mesh['points'], 6)}]",
            f"        normal3f[] normals = [{tuples(mesh['normals'], 4)}] (", '            interpolation = "vertex"', "        )",
            '        uniform token subdivisionScheme = "none"',
            f"        rel material:binding = </{scene}/Materials/{name}>",
            "    }",
        ]  # fmt: skip
    lines.append("}")

    usda = Path(path).with_suffix(".usda")
    usda.write_text("\n".join(lines) + "\n")
    usdcat, usdzip = Path("/usr/bin/usdcat"), Path("/usr/bin/usdzip")
    if not (usdcat.exists() and usdzip.exists()):
        print("build_mascot3d: no usdcat/usdzip; kept", usda)
        return usda
    with tempfile.TemporaryDirectory() as work:
        usdc = Path(work) / "Mascot.usdc"
        subprocess.run([str(usdcat), str(usda), "-o", str(usdc)], check=True)
        Path(path).unlink(missing_ok=True)
        subprocess.run([str(usdzip), str(Path(path).resolve()), usdc.name], check=True, cwd=work)
    usda.unlink()
    return Path(path)


# ---------------------------------------------------------------- main


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--src", required=True, help="Postitheka_Mascot_Web_NoTablet.glb, in a directory of its own")
    parser.add_argument("--out", default="out/mascot3d", help="where mascot.glb and Mascot.usdz go")
    parser.add_argument("--ios", help="also copy Mascot.usdz here (ios/Postitheka/Mascot3D)")
    args = parser.parse_args()

    meshes = read_glb(args.src)
    tokens = read_tokens()
    source_height = normalise(meshes)
    looks = paint(meshes, tokens)
    turned = {m["material"]["name"]: orient(m) for m in meshes}

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    glb_bytes = write_glb(out / "mascot.glb", meshes, looks)
    usd = write_usdz(out / "Mascot.usdz", meshes, looks)
    if args.ios and usd.suffix == ".usdz":
        shutil.copyfile(usd, Path(args.ios) / "Mascot.usdz")

    triangles = sum(len(m["indices"]) // 3 for m in meshes)
    print(f"source height {source_height:.3f} -> 1 unit")
    for mesh in meshes:
        name = mesh["material"]["name"]
        print(f"  {name:<17} {looks[name]['token']:<14} {len(mesh['indices']) // 3:>6} triangles, {turned[name]:>5} turned round")
    print(f"mascot.glb  {glb_bytes / 1024:.0f} KB, {len(meshes)} meshes, {len(meshes)} draw calls, {triangles} triangles")
    print(f"{usd.name:<11} {usd.stat().st_size / 1024:.0f} KB")


if __name__ == "__main__":
    main()
