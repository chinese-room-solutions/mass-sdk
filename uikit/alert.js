// massAlert — Shoelace-styled replacement for window.alert.
// Self-contained: lazily injects a single <sl-dialog> on first call and
// reuses it for every subsequent message. Returns a Promise that resolves
// when the user dismisses the dialog.
//
//   window.massAlert("Install failed: bad request");
//   window.massAlert("Done", { title: "Success", variant: "success" });
//
// Options:
//   title   — header label                     (default: "Notice")
//   variant — primary | success | danger       (default: "primary")
//   ok      — OK button label                  (default: "OK")
(function () {
  "use strict";

  var DIALOG_ID = "mass-alert-dialog";
  var dialog = null;
  var msgEl = null;
  var okBtn = null;
  var resolveFn = null;

  // Shoelace dialogs render in the browser top layer, which breaks CSS
  // variable inheritance from <html>. The host app must mirror its theme
  // class onto every <sl-dialog>; we do the same on create + on each open.
  function currentThemeClass() {
    return document.documentElement.classList.contains("sl-theme-light")
      ? "sl-theme-light"
      : "sl-theme-dark";
  }

  function ensureDialog() {
    if (dialog) return;
    dialog = document.createElement("sl-dialog");
    dialog.id = DIALOG_ID;
    dialog.className = "mass-dialog-centered-title " + currentThemeClass();
    // z-index bump: sit above other open sl-dialogs (default --sl-z-index-dialog=800).
    // Body bg pinned to --mass-bg-panel so the panel reads correctly even when
    // Shoelace's neutral scale isn't fully overridden.
    dialog.style.cssText =
      "--width:440px;" +
      "--sl-z-index-dialog:1000;" +
      "--sl-panel-background-color:var(--mass-bg-panel);" +
      "--sl-panel-border-color:var(--mass-border);" +
      "--header-spacing:var(--sl-spacing-small) var(--sl-spacing-large);" +
      "--body-spacing:var(--sl-spacing-x-small) var(--sl-spacing-large) var(--sl-spacing-small);" +
      "--footer-spacing:var(--sl-spacing-x-small) var(--sl-spacing-large);";

    msgEl = document.createElement("p");
    msgEl.className = "text-sm text-center";
    msgEl.style.cssText = "color:var(--mass-text);white-space:pre-wrap;word-break:break-word";
    dialog.appendChild(msgEl);

    var footer = document.createElement("div");
    footer.setAttribute("slot", "footer");
    footer.className = "flex justify-center";
    okBtn = document.createElement("sl-button");
    okBtn.setAttribute("variant", "primary");
    okBtn.setAttribute("size", "small");
    okBtn.textContent = "OK";
    okBtn.addEventListener("click", function () { dialog.hide(); });
    footer.appendChild(okBtn);
    dialog.appendChild(footer);

    dialog.addEventListener("sl-after-hide", function (e) {
      if (e.target !== dialog) return;
      var r = resolveFn;
      resolveFn = null;
      if (r) r();
    });

    document.body.appendChild(dialog);
  }

  window.massAlert = function (message, opts) {
    opts = opts || {};
    ensureDialog();
    dialog.classList.remove("sl-theme-dark", "sl-theme-light");
    dialog.classList.add(currentThemeClass());
    dialog.label = opts.title || "Notice";
    okBtn.setAttribute("variant", opts.variant || "primary");
    okBtn.textContent = opts.ok || "OK";
    msgEl.textContent = String(message == null ? "" : message);
    return new Promise(function (resolve) {
      resolveFn = resolve;
      dialog.show();
    });
  };

  // Extract a user-facing message from a server response body. MASS APIs
  // return {"error":"..."} on failure; some endpoints reply with plain text.
  window.massErrorText = function (raw) {
    var s = String(raw == null ? "" : raw).trim();
    if (s === "") return "";
    if (s.charAt(0) === "{" || s.charAt(0) === "[") {
      try {
        var j = JSON.parse(s);
        if (j && typeof j.error === "string" && j.error !== "") return j.error;
        if (j && typeof j.message === "string" && j.message !== "") return j.message;
      } catch (_) { /* fall through */ }
    }
    return s;
  };

  // Pre-flight a precondition by GET-probing a URL. Calls onOK on 2xx;
  // otherwise reads the server's error envelope ({"error":"..."}) and pops
  // a danger alert with that text. Network errors fall through to onOK so
  // the caller's own request can surface the real failure (don't let a
  // transient blip block the user from trying).
  //
  //   window.massPreflight("/api/v1/browse?ext=*runtime*", "Cannot Browse", openDialog)
  //
  // Use this whenever a UI flow is pointless if a server-side check fails.
  // Keeping the message string on the server (one source of truth) avoids
  // drift between the precondition error and the popup text.
  window.massPreflight = function (probeURL, alertTitle, onOK) {
    fetch(probeURL).then(function (r) {
      if (r.ok) { onOK(); return; }
      r.text().then(function (t) {
        window.massAlert(window.massErrorText(t) || ("HTTP " + r.status),
          { title: alertTitle, variant: "danger" });
      });
    }).catch(function () { onOK(); });
  };
})();
