// massSetTheme — SDK-provided live theme switcher for standalone MASS apps.
// Swaps the <html>/<body> classes (and every open <sl-dialog>, which renders
// in the browser top layer and so doesn't inherit the <html> theme class) so
// a theme change applies instantly, without a page reload. Registry-driven via
// window.__massThemes (injected by Layout): a theme carries a base
// (dark|light) plus an sl-theme-<name> overlay class for pluggable themes.
// Mirrors MASS's own shell.js applyTheme so apps and MASS behave identically.
(function () {
  "use strict";

  function stripThemeClasses(list) {
    Array.prototype.slice.call(list).forEach(function (c) {
      if (c.indexOf("sl-theme-") === 0) list.remove(c);
    });
  }

  function applyTheme(t) {
    var info = (window.__massThemes || {})[t] || { base: "dark" };
    var base = info.base === "light" ? "light" : "dark";
    var isLight = base === "light";
    var baseClass = "sl-theme-" + base;

    // The active theme's name, readable by anything on the page (dialog check
    // marks, gateway iframe src builders). Seeded server-side by Layout and
    // kept fresh here — the same contract MASS's shell.js maintains.
    document.documentElement.dataset.theme = t;

    // Toggle only theme-related classes; never reassign className on <html> or
    // <body>, which would wipe state classes the app owns (e.g. the FOUC guard's
    // uikit-ready on <body>) and re-hide the whole page until a reload.
    var html = document.documentElement.classList;
    stripThemeClasses(html);
    html.add(baseClass);
    if (t !== base) html.add("sl-theme-" + t);
    html.toggle("dark", !isLight);

    var body = document.body.classList;
    stripThemeClasses(body);
    body.add(baseClass);
    if (t !== base) body.add("sl-theme-" + t);
    body.toggle("bg-neutral-100", isLight);
    body.toggle("text-neutral-900", isLight);
    body.toggle("bg-neutral-950", !isLight);
    body.toggle("text-neutral-100", !isLight);

    document.querySelectorAll("sl-dialog").forEach(function (d) {
      var cl = d.classList;
      stripThemeClasses(cl);
      cl.add(baseClass);
      if (t !== base) cl.add("sl-theme-" + t);
    });
  }

  window.massSetTheme = applyTheme;
})();
