// Confirm destructive actions. Kept in a file (not inline handlers) so the
// page can run under a strict Content-Security-Policy.
document.addEventListener("submit", (e) => {
  const msg = e.target.dataset.confirm;
  if (msg && !window.confirm(msg)) e.preventDefault();
});
