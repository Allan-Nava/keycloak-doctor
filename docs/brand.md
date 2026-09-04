# Brand

**The mark, the palette and the two rules for using them.**

## The name

`keycloak-doctor`, one word, lowercase, hyphenated, in monospace wherever a monospace font is available. Not "Keycloak Doctor", not "KeycloakDoctor": the name is a command you type. It is a third-party tool and it is not affiliated with, endorsed by, or a product of Keycloak or Red Hat — never present it as one.

## The mark

| File | Use |
|---|---|
| [`docs/assets/logo.svg`](assets/logo.svg) | the mark, everywhere it is shown at 24px or larger |
| [`docs/assets/favicon.svg`](assets/favicon.svg) | browser tabs and anything under 24px |
| [`docs/assets/og-card.svg`](assets/og-card.svg) | source of the link-preview card, 1200×630 |
| `docs/assets/og-card.png` | the card itself — what a search result, a Slack unfurl or a tweet shows |

Three elements, each carrying one of the tool's claims:

- a **shield** — the tool exists for the security posture of a realm, not for its inventory;
- a **keyhole** — that posture is a Keycloak configuration, read as it is;
- a **pulse** crossing the shield — the output is a diagnosis with a rationale and a fix, which is why the shield is not a checkmark. A tick would promise a verdict the tool deliberately does not give: `OK` means a rule ran and passed, never "this realm is safe".

`logo.svg` and `favicon.svg` are hand-written SVG with no external references, no embedded raster and no font dependency, so they scale from a favicon to a slide and stay a few hundred bytes. The `favicon.svg` variant is the same mark with the detail a 16px tab cannot resolve taken out — a larger keyhole, one thicker pulse spike. Keep them in step: a change to one is a change to both.

## Palette

| Token | Hex | Where |
|---|---|---|
| Deep blue | `#0B3A66` | the tile, and the keyhole punched out of the shield |
| Shield | `#EAF2FB` | the shield face |
| Pulse | `#F0A02A` | the pulse, and nothing else — it is the accent, not a fill |

The site uses its own interface palette (`internal/site/assets/style.css`), which is theme-aware and independent of these three: the mark has to survive on a light page, on a dark page and on a coloured slide, so it carries its own background instead of borrowing one.

## Using it

- **Do** put the mark on a solid tile as it ships, keep its corner radius, and leave clear space of at least a quarter of its width around it.
- **Do** pair it with the name set in monospace, to its right.
- **Don't** recolour it, rotate it, add a gradient or a shadow, stretch it to a non-square box, or place it on a busy photograph.
- **Don't** use it to imply that a realm passed an audit — it identifies the tool, not a result.

## The link-preview card

`og-card.png` is the only raster file here, and it is committed rather than generated at build time on purpose: **no crawler and no chat client renders SVG**, so an SVG in `og:image` means no preview at all. The PNG is a render of `og-card.svg` at exactly 1200×630 — the size every consumer crops from — and the site serves it next to its pages, with `og:image:width`/`height` declared so a client does not have to download it to lay the card out.

After editing `og-card.svg`, re-render it:

```bash
# wrap the SVG in a page so the render has no margin, then screenshot it at 1200x630
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
printf '<!doctype html><style>html,body{margin:0;background:#071F35}svg{display:block}</style>%s' \
  "$(cat docs/assets/og-card.svg)" > /tmp/og.html
"$CHROME" --headless --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --screenshot=docs/assets/og-card.png --window-size=1200,630 "file:///tmp/og.html"
sips -g pixelWidth -g pixelHeight docs/assets/og-card.png   # must say 1200 x 630
```

Text in the card is real SVG text, so it is rendered with the fonts of the machine that renders it — one more reason the PNG is committed instead of built in CI, where those fonts are not the same.

## Regenerating a preview of the mark

The SVG is the source; there is nothing to build. To look at a change before committing it, render a large preview and a tab-sized one:

```bash
qlmanage -t -s 384 -o /tmp docs/assets/logo.svg      # macOS
qlmanage -t -s 32  -o /tmp docs/assets/favicon.svg
```

The site copies the served files verbatim next to its pages (`cmd/gen-site` reads them from `docs/assets/`), and the README points at the same mark. One mark, one copy of it: there is no second place to update.
