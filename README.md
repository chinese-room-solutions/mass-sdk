# mass-sdk

[![CI](https://github.com/chinese-room-solutions/mass-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/chinese-room-solutions/mass-sdk/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/chinese-room-solutions/mass-sdk.svg)](https://pkg.go.dev/github.com/chinese-room-solutions/mass-sdk)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)

Go helpers shared across MASS-ecosystem apps (MASS, Grimoire, runtime
gateways): GUI machinery, terminal UI, installers, and pure-Go utilities.

## Packages

- **`connstore/`** — per-MASS-endpoint connection storage (token + optional
  custom CA) in a JSON file at `os.UserConfigDir()/mass/auth.json`.
- **`format/`** — small, allocation-free string formatters; no UI/HTML
  dependencies.
- **`fsutil/`** — small filesystem helpers (e.g. atomic file writes).
- **`gatewayhttp/`** — tunnels a runtime gateway's `HandleRequest` gRPC
  stream into a normal `http.Handler`, trailers preserved.
- **`ggufutil/`** — pure-Go helpers for GGUF model filenames.
- **`gui/`** — shared HTTP machinery for app GUIs: the MASS connection
  settings control, validated against the gateway and persisted via
  `connstore`.
- **`hfui/`** — HuggingFace model-search widget: result rows,
  variant-picker overlay, download buttons + progress JS.
- **`huggingface/`** — HuggingFace Hub API client: model search, file
  listing, pagination, optional caching.
- **`install/`** — OS-aware install/uninstall of a desktop GUI app:
  staging, launchers, install record; parameterized over an `AppSpec`.
- **`manifest/`** — reads a runtime gateway's `runtime.yml` (name,
  version, display name, description).
- **`masstls/`** — client TLS configuration for talking to a MASS gateway,
  including a private or self-signed CA.
- **`modelui/`** — llama model-config form rendering and Datastar signal
  decoding; builds on `uikit`.
- **`selfextract/`** — self-extracting installer format (MWPLOAD1) shared
  by the packer and the setup binary.
- **`term/`** — dependency-free terminal styling; ANSI only on a real TTY,
  honors NO_COLOR.
- **`tray/`** — system-tray icon facade over `fyne.io/systray` with per-OS
  threading handled inside.
- **`tui/`** — arrow-key navigable terminal UI engine: raw-mode input,
  generic forms, two-button modals; styled via `term`.
- **`uikit/`** — reusable HTML-generating helpers for MASS module UIs
  (layout, theme, widgets).
- **`webview/`** — cross-platform native webview window for app standalone
  UIs (CGO on Linux/macOS).

## Module path

```go
import (
    "github.com/chinese-room-solutions/mass-sdk/connstore"
    "github.com/chinese-room-solutions/mass-sdk/gui"
    "github.com/chinese-room-solutions/mass-sdk/uikit"
    "github.com/chinese-room-solutions/mass-sdk/webview"
)
```

## License

[Apache-2.0](LICENSE)
