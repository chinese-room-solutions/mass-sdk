# mass-sdk

Go helpers shared across MASS-ecosystem apps. Standalone HTTP services
(playground, pdf2doc) drop in the auth gate, login flow, and native webview
from here without rolling their own.

## Packages

- **`huggingface/`** — HuggingFace Hub model search client.
- **`uikit/`** — shared HTML/Datastar/Tailwind layout, theme, helpers,
  HF result rendering, model-config widgets.
- **`auth/`** — per-endpoint token store (file-backed JSON in
  `os.UserCacheDir()/mass/auth.json`).
- **`gui/`** — login page handler + auth-gate middleware for app GUIs.
  Client-agnostic: callers supply `Endpoint` / `SetToken` / `SetEndpoint`
  / `NewValidator` so the package works against any HTTP-shaped backend
  (e.g. `llama-cpp-openai-client-go`).
- **`webview/`** — native window opener for app standalone mode.

## Module path

```go
import (
    "github.com/chinese-room-solutions/mass-sdk/auth"
    "github.com/chinese-room-solutions/mass-sdk/gui"
    "github.com/chinese-room-solutions/mass-sdk/uikit"
    "github.com/chinese-room-solutions/mass-sdk/webview"
)
```
