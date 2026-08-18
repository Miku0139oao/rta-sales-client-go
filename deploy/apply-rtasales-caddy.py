from pathlib import Path

caddy = Path("/etc/caddy/Caddyfile")
text = caddy.read_text()
block = Path("/tmp/rtasales.caddy").read_text().strip() + "\n"
tokens = (
    "rtasales.com, www.rtasales.com {",
    "www.rtasales.com, rtasales.com {",
    "rtasales.com {",
    "www.rtasales.com {",
    "pre-rtasales.on9.uk {",
)


def remove_block(src: str, token: str) -> str:
    start = src.find(token)
    if start < 0:
        return src
    depth = 0
    end = start
    for i, ch in enumerate(src[start:], start):
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                end = i + 1
                break
    return src[:start] + src[end:]


changed = True
while changed:
    changed = False
    for token in tokens:
        nxt = remove_block(text, token)
        if nxt != text:
            text = nxt
            changed = True

caddy.write_text(text.rstrip() + "\n\n" + block)
print("Caddyfile updated")
