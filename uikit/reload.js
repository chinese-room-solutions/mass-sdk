// Reload keys. A MASS window is a webview with no browser chrome, so F5 and
// Ctrl/Cmd+R have to be bound by hand — on macOS nothing reloads the view
// otherwise, since the app's main menu deliberately leaves those keys to the
// page. Shift variants included, the browser convention for a hard reload.
//
// The top-level window is what reloads: module UIs are iframes, a keydown
// inside one never reaches the host document, and refreshing the frame alone
// would leave the surrounding app on its old state. A cross-origin frame can't
// reach top, so it falls back to reloading itself.
//
// Registered at script load rather than on DOMContentLoaded, so the keys work
// even if boot stalls — which is exactly when a refresh is wanted.
(function () {
  "use strict";

  document.addEventListener("keydown", function (e) {
    if (e.altKey) return;
    var reloadKey = e.key === "F5" ||
      ((e.ctrlKey || e.metaKey) && (e.key === "r" || e.key === "R"));
    if (!reloadKey) return;
    e.preventDefault();
    try {
      (window.top || window).location.reload();
    } catch (_) {
      location.reload();
    }
  });
})();
