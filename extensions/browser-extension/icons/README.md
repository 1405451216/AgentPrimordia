# Icons

Place the following PNG icon files in this directory for use by the Manifest V3 extension:

- `icon16.png` — 16×16 px
- `icon48.png` — 48×48 px
- `icon128.png`— 128×128 px

## Generating the PNGs

A reference SVG design is provided in `icon.svg`. Convert it to PNG at the three required sizes using any of the following methods:

### Using ImageMagick (recommended)

```bash
magick -background none icon.svg -resize 16x16 icon16.png
magick -background none icon.svg -resize 48x48 icon48.png
magick -background none icon.svg -resize 128x128 icon128.png
```

### Using rsvg-convert

```bash
rsvg-convert -w 16 -h 16 icon.svg > icon16.png
rsvg-convert -w 48 -h 48 icon.svg > icon48.png
rsvg-convert -w 128 -h 128 icon.svg > icon128.png
```

### Using Inkscape

```bash
inkscape icon.svg --export-type=png --export-width=16 --export-height=16 --export-filename=icon16.png
inkscape icon.svg --export-type=png --export-width=48 --export-height=48 --export-filename=icon48.png
inkscape icon.svg --export-type=png --export-width=128 --export-height=128 --export-filename=icon128.png
```

## Design

The icon is a rounded square with a purple gradient background (`#6C5CE7 → #a29bfe`)
and the letters "AP" in white — matching the AgentPrimordia brand identity.
