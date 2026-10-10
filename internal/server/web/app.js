// Page behaviour. Kept in a file (not inline handlers) so pages can run under
// a strict Content-Security-Policy.

// Confirm destructive actions.
document.addEventListener("submit", (e) => {
  const msg = e.target.dataset.confirm;
  if (msg && !window.confirm(msg)) e.preventDefault();
});

// Light/dark toggle, remembered across visits.
const themeBtn = document.getElementById("themeBtn");
if (themeBtn) {
  themeBtn.addEventListener("click", () => {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("aa_theme", next); } catch (e) { /* private mode */ }
  });
}
