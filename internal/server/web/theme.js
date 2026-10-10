// Runs before first paint (loaded in <head>) so the saved theme applies without
// a flash. Same key and first-visit rule as The Analogy Architect.
(function () {
  var t;
  try { t = localStorage.getItem("aa_theme"); } catch (e) {}
  if (!t) t = window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", t);
})();
