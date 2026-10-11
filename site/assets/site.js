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

// Copy buttons on code blocks (copies the commands, not the "$ " prompts).
for (const pre of document.querySelectorAll("pre[data-copy]")) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "copy";
  b.textContent = "Copy";
  b.addEventListener("click", async () => {
    const code = pre.querySelector("code").cloneNode(true);
    code.querySelectorAll(".p, .c").forEach((el) => el.remove());
    const text = code.textContent.split("\n").map((l) => l.trimEnd()).join("\n").trim();
    try {
      await navigator.clipboard.writeText(text);
      b.textContent = "Copied";
      setTimeout(() => (b.textContent = "Copy"), 1500);
    } catch (e) { b.textContent = "Select & copy"; }
  });
  pre.appendChild(b);
}

// Inline copy buttons (the install one-liner).
for (const b of document.querySelectorAll("[data-copy-target]")) {
  b.addEventListener("click", async () => {
    const text = document.getElementById(b.dataset.copyTarget).textContent.trim();
    try {
      await navigator.clipboard.writeText(text);
      b.textContent = "Copied";
      setTimeout(() => (b.textContent = "Copy"), 1500);
    } catch (e) { b.textContent = "Select & copy"; }
  });
}

// The hero address bar: types an example, shows where it lands, moves on.
const typed = document.getElementById("typed");
const dest = document.getElementById("dest");
const destUrl = document.getElementById("dest-url");
const destNote = document.getElementById("dest-note");
const reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

if (typed && dest && !reduced) {
  const examples = [
    ["standup", "meet.google.com/abc-defg-hij", "your 10 am stand-up"],
    ["budget", "docs.google.com/spreadsheets/d/1xQ7…", "no more digging through Drive"],
    ["pr/482", "github.com/acme/app/pull/482", "any pull request, by number"],
    ["flight/6E-2134", "flightradar24.com/data/flights/6e2134", "one template, any flight"],
    ["itr", "incometax.gov.in", "the real portal, not a look-alike"],
    ["?wedding venue shortlist", "docs.google.com/document/d/9Fh2…", "found by meaning, not by name"],
  ];
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const visible = () => document.visibilityState === "visible";

  (async function loop() {
    let i = 0;
    await sleep(2600); // let the first example be read
    for (;;) {
      while (!visible()) await sleep(500);
      dest.classList.add("hidden");
      for (let n = typed.textContent.length; n > 0; n--) {
        typed.textContent = typed.textContent.slice(0, -1);
        await sleep(28);
      }
      i = (i + 1) % examples.length;
      const [name, url, note] = examples[i];
      await sleep(260);
      for (const ch of name) {
        typed.textContent += ch;
        await sleep(70 + Math.random() * 60);
      }
      await sleep(320);
      destUrl.textContent = url;
      destNote.textContent = note;
      dest.classList.remove("hidden");
      await sleep(2600);
    }
  })();
}
