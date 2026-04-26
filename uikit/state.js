// massState — SDK-provided sessionStorage API for MASS modules.
// Scopes keys by module name (extracted from URL path /modules/{name}/).
// Survives theme switches (iframe reloads) within the same browser session.
(function () {
  "use strict";

  // Extract module name from path: /modules/{name}/...
  var parts = window.location.pathname.split("/");
  var idx = parts.indexOf("modules");
  var moduleName = (idx >= 0 && parts.length > idx + 1) ? parts[idx + 1] : "unknown";
  var prefix = "mass_" + moduleName + "_";

  window.massState = {
    /** The current module's name (e.g. "pdf2text", "playground"). */
    moduleName: moduleName,

    /** Get a stored value. Returns empty string if not found. */
    get: function (key) {
      try { return sessionStorage.getItem(prefix + key) || ""; }
      catch (_) { return ""; }
    },

    /** Store a value. Returns false if storage quota is exceeded. */
    set: function (key, val) {
      try { sessionStorage.setItem(prefix + key, val); return true; }
      catch (_) { return false; }
    },

    /** Remove a stored value. */
    del: function (key) {
      try { sessionStorage.removeItem(prefix + key); }
      catch (_) { /* noop */ }
    },

    /** Remove all values for this module. */
    clear: function () {
      try {
        var toRemove = [];
        for (var i = 0; i < sessionStorage.length; i++) {
          var k = sessionStorage.key(i);
          if (k && k.indexOf(prefix) === 0) toRemove.push(k);
        }
        toRemove.forEach(function (k) { sessionStorage.removeItem(k); });
      } catch (_) { /* noop */ }
    }
  };
})();
