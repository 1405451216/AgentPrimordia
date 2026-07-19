# AgentPrimordia Browser Extension

> Manifest V3 browser extension (Chrome / Firefox) that lets developers inspect
> and interact with AgentPrimordia agents on any web page.

## Features

- **Agent Detection** — Automatically detects AgentPrimordia agents embedded in a
  page via `<meta>` tags or `window.__AP_AGENT__` global variable.
- **Floating Button** — When an agent is detected, a small purple "AP" button
  appears in the bottom-right corner of the page as a quick-entry point.
- **Popup** — Click the toolbar icon to see agent status, Studio connection,
  a quick-chat input, recent runs, and shortcut actions.
- **DevTools Panel** — Open Chrome DevTools and find the *AgentPrimordia* tab
  for real-time trace timelines, agent state, memory info, cost estimates,
  and JSON trace export.
- **Studio Backend** — Connects to a running AgentPrimordia Studio instance
  (default `http://localhost:8765`) with automatic polling and graceful
  degradation when unreachable.

## Architecture

```
sdk/browser-extension/
├── manifest.json            # MV3 manifest (permissions, content scripts, devtools page)
├── src/
│   ├── background.ts        # Service worker — message routing, state, Studio polling
│   ├── content.ts           # Content script — agent detection, floating button
│   ├── popup/               # Popup UI (toolbar click)
│   ├── devtools/            # DevTools panel (custom DevTools tab)
│   └── shared/
│       ├── types.ts         # Shared TypeScript interfaces
│       ├── api.ts           # Studio REST API client
│       └── bridge.ts        # Message-passing helpers between content script and extension
├── icons/                   # Extension icons (SVG + generated PNGs)
├── scripts/
│   └── generate-icons.mjs   # Builds PNG icons from scratch (no external deps)
├── package.json
├── tsconfig.json
└── README.md
```

### Message flow

```
webpage ──► content.ts ──► background service worker ◄──► popup.ts
                                    │                      ◄──► panel.ts
                              (chrome.storage.local)
                                    │
                              Studio REST API
```

All cross-boundary communication goes through `chrome.runtime.sendMessage` and
the background service worker acts as the single source of truth for state.

## Install (Load Unpacked)

1. Build the extension (see [Build](#build) below).
2. Open Chrome and navigate to `chrome://extensions`.
3. Enable **Developer mode** (top-right toggle).
4. Click **Load unpacked** and select the `sdk/browser-extension` directory
   *(the directory that contains `manifest.json`)*.
5. The extension icon will appear in the toolbar. Click it to open the popup.
6. Open DevTools (F12) on any page to see the *AgentPrimordia* panel.

> **Firefox** — the same MV3 extension loads in Firefox via
> `about:debugging#/runtime/this-firefox` → **Load Temporary Add-on**.

## Build

Requirements: Node.js ≥ 18.

```bash
cd sdk/browser-extension
npm install
npm run build          # Compile TypeScript → dist/
```

Output goes to `dist/` (e.g. `dist/background.js`, `dist/content.js`,
`dist/popup/popup.html`, etc.).

### Watch mode

```bash
npm run watch          # Recompile on change
```

### Icons

Pre-built PNG icons are already committed in `icons/`. To regenerate them:

```bash
node scripts/generate-icons.mjs
```

This uses only Node.js built-in modules (`zlib`) — no external dependencies
required. The source design lives in `icons/icon.svg`. See
[`icons/README.md`](icons/README.md) for ImageMagick / Inkscape alternatives.

### Package for distribution

```bash
npm run package        # Builds then zips dist/ + icons/ + manifest.json
```

The resulting `extension-v0.1.0.zip` can be uploaded to the Chrome Web Store
or Firefox Add-ons.

## Configuration

The extension talks to the Studio backend configured in the extension state
(default `http://localhost:8765`). To point it at a different URL, update the
`studioUrl` field stored under `ap_extension_state` in `chrome.storage.local`
via the background service worker's DevTools console, or call
`chrome.storage.local.set({ ap_extension_state: { studioUrl: '...' } })`.

### Per-page agent detection

The content script recognizes two embedding styles:

**1. Meta tags** — add `<meta>` tags with the `ap-agent-` prefix:

```html
<meta name="ap-agent-id" content="agent-007" />
<meta name="ap-agent-name" content="My Agent" />
<meta name="ap-agent-endpoint" content="http://localhost:8765/agents/agent-007" />
```

**2. Global variable** — set `window.__AP_AGENT__` before the content script
runs (e.g. in a `<script>` tag in `<head>`):

```html
<script>
    window.__AP_AGENT__ = {
        id: 'agent-007',
        name: 'My Agent',
        endpoint: 'http://localhost:8765/agents/agent-007',
    };
</script>
```

## Tech notes

- **TypeScript only** — compiled directly with `tsc`; no bundler needed because
  MV3 service workers and content scripts work fine with ES2020 modules up to a
  few hundred lines.
- **Strict mode** — `strict: true` enabled; all code is well-typed.
- **MV3 compliant** — no inline scripts in HTML; no `eval`; CSP-friendly.
- **Graceful degradation** — if the Studio backend is unreachable the UI shows
  a disconnected state but the rest of the extension (detection, traces kept in
  memory) remains usable.
- **File size budget** — each source file is intentionally kept under 200 lines
  and focused on a single responsibility.

## License

Apache-2.0 — see the repository root [`LICENSE`](../../LICENSE).
