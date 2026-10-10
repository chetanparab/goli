// Light/dark toggle, remembered across visits (same key as The Analogy Architect).
const themeBtn = document.getElementById("themeBtn");
if (themeBtn) {
  themeBtn.addEventListener("click", () => {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("aa_theme", next); } catch (e) { /* private mode */ }
  });
}

// Copy buttons on code blocks.
for (const pre of document.querySelectorAll("pre[data-copy]")) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "copy";
  b.textContent = "Copy";
  b.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(pre.innerText.replace(/^Copy$/m, "").trim());
      b.textContent = "Copied";
      setTimeout(() => (b.textContent = "Copy"), 1500);
    } catch (e) { b.textContent = "Select & copy"; }
  });
  pre.appendChild(b);
}
