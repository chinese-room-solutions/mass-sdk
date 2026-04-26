# mass-sdk

Go SDK for building MASS apps and the MASS host itself. Built on top of
[mass-client-go](../mass-client-go) (the lightweight client) and adds the
ecosystem opinions: app-broker contract, Datastar/Tailwind UI kit, login
flow, native webview, HuggingFace search, etc.

## Packages

- **`app/`** — go-plugin contract every MASS app implements
  (`AppInterface`, `AppInfo`, `Handshake`, `AppGRPCPlugin`, `Metadata`,
  `ReadMetadata`, model-config types). MASS imports this to host apps;
  apps import this to register themselves.
- **`huggingface/`** — HuggingFace Hub model search client.
- **`uikit/`** — shared HTML/Datastar/Tailwind layout, theme, helpers,
  HF result rendering, model-config widgets.
- **`auth/`** — per-endpoint token store (file-backed JSON in
  `os.UserCacheDir()/mass/auth.json`).
- **`gui/`** — login page handler + auth-gate middleware for app GUIs.
- **`webview/`** — native window opener for app standalone mode.

## Module path

```
import (
    mass "github.com/chinese-room-solutions/mass-client-go"
    "github.com/chinese-room-solutions/mass-sdk/app"
    "github.com/chinese-room-solutions/mass-sdk/auth"
    "github.com/chinese-room-solutions/mass-sdk/gui"
    "github.com/chinese-room-solutions/mass-sdk/uikit"
    "github.com/chinese-room-solutions/mass-sdk/webview"
)
```

The `app` package depends only on stdlib + go-plugin + protobuf; the GUI
packages additionally depend on Datastar/Tailwind conventions.
