// goli.theanalogyarchitect.com — theme, copy buttons, download button and the
// three live demos (browser, phone, terminal). Every demo pauses when it's off
// screen or the tab is hidden, and shows a still picture under reduced motion.

const reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const $ = (id) => document.getElementById(id);

// ── Light/dark toggle (same key as The Analogy Architect) ─────────────────
const themeBtn = $("themeBtn");
if (themeBtn) {
  themeBtn.addEventListener("click", () => {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("aa_theme", next); } catch (e) { /* private mode */ }
  });
}

// ── Copy buttons ───────────────────────────────────────────────────────────
async function copyText(btn, text) {
  try {
    await navigator.clipboard.writeText(text);
    btn.textContent = "Copied";
    setTimeout(() => (btn.textContent = "Copy"), 1500);
  } catch (e) { btn.textContent = "Select & copy"; }
}
for (const pre of document.querySelectorAll("pre[data-copy]")) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "copy";
  b.textContent = "Copy";
  b.addEventListener("click", () => {
    const code = pre.querySelector("code").cloneNode(true);
    code.querySelectorAll(".p, .c").forEach((el) => el.remove());
    copyText(b, code.textContent.split("\n").map((l) => l.trimEnd()).join("\n").trim());
  });
  pre.appendChild(b);
}

// ── Download button for the visitor's OS ───────────────────────────────────
const dl = $("dl-btn");
if (dl) {
  const base = "https://github.com/chetanparab/goli/releases/latest/download/";
  const ua = navigator.userAgent || "";
  const uad = navigator.userAgentData;
  const plat = (uad && uad.platform) || navigator.platform || "";
  // iPadOS Safari presents itself as a Mac; a touch screen gives it away.
  const mobile = /Android|iPhone|iPad|iPod/i.test(ua) || (/Mac/i.test(plat) && navigator.maxTouchPoints > 1);
  const note = $("dl-note");
  const offer = (label, file, extra) => {
    dl.href = base + file;
    dl.removeAttribute("rel");
    dl.firstChild.textContent = label + " ";
    if (note && extra) note.innerHTML = extra;
  };
  const pick = (arch) => {
    const arm = /arm/i.test(arch || "");
    if (/Mac/i.test(plat)) {
      // Browsers don't reliably say which Mac chip they run on, so offer both.
      if (arch && !arm) offer("Download for Mac (Intel)", "goli_darwin_amd64.tar.gz",
        'Apple silicon Mac? <a href="' + base + 'goli_darwin_arm64.tar.gz">Get that build</a> · free &amp; open source');
      else offer("Download for Mac (Apple silicon)", "goli_darwin_arm64.tar.gz",
        'Intel Mac? <a href="' + base + 'goli_darwin_amd64.tar.gz">Get the Intel build</a> · free &amp; open source');
    } else if (/Win/i.test(plat)) {
      offer("Download for Windows", arm ? "goli_windows_arm64.zip" : "goli_windows_amd64.zip");
    } else if (/Linux/i.test(plat)) {
      offer("Download for Linux", arm ? "goli_linux_arm64.tar.gz" : "goli_linux_amd64.tar.gz");
    }
  };
  if (mobile) {
    if (note) note.textContent = "goli runs on your computer — install it there, then use it from this phone.";
  } else if (uad && uad.getHighEntropyValues) {
    pick("");
    uad.getHighEntropyValues(["architecture"]).then((v) => pick(v.architecture)).catch(() => {});
  } else {
    pick("");
  }
}

// ── Pausing: only animate what's on screen in a visible tab ────────────────
function visibilityGate(el) {
  let onScreen = true;
  if ("IntersectionObserver" in window && el) {
    new IntersectionObserver((es) => { onScreen = es[0].isIntersecting; }, { threshold: 0.2 }).observe(el);
  }
  return async () => { while (!onScreen || document.visibilityState !== "visible") await sleep(400); };
}

// ── 1. The browser demo ────────────────────────────────────────────────────
const scenes = [
  { typed: "go/standup", url: "meet.google.com/abc-defg-hij", title: "Daily stand-up", fav: "meet", page: "meet",
    caption: "Type <b>go/standup</b> — land in your 10 am meeting. No digging through calendar invites." },
  { typed: "go/pr/482", url: "github.com/acme/app/pull/482", title: "Fix checkout timeout · #482", fav: "pr", page: "pr",
    caption: "One link template, any number: <b>go/pr/482</b> opens pull request 482." },
  { typed: "go/roadmp", url: "go/roadmp", title: "go/roadmp · goli", fav: "goli", page: "typo",
    caption: "A typo? goli shows what you probably meant — or lets you create <b>go/roadmp</b> on the spot." },
  { typed: "go/flight/6E-2134", url: "flightradar24.com/data/flights/6e2134", title: "6E 2134 · Flight status", fav: "flight", page: "flight",
    caption: "Mum's flight lands at seven. <b>go/flight/</b> works with any flight number." },
  { typed: "go/?q3 sales", url: "go/?q3 sales", title: "Search · goli", fav: "goli", page: "search",
    caption: "Can't remember the name? Describe it. goli finds <b>go/sales</b> by its description (optional AI)." },
];

const browser = $("browser");
if (browser) {
  const url = $("b-url"), caret = $("b-caret"), omni = $("b-omni"), key = $("b-key");
  const bar = $("b-progress"), title = $("b-title"), fav = $("b-fav"), caption = $("scene-caption");
  const pages = browser.querySelectorAll(".pg");
  const tabs = document.querySelectorAll(".scene");
  const gate = visibilityGate(browser);
  let run = 0; // bumps whenever a new sequence starts; older ones stop

  const steps = ["st-type", "st-enter", "st-land"].map((id) => $(id));
  const step = (n) => steps.forEach((el, i) => {
    if (!el) return;
    el.classList.toggle("on", i === n);
    el.classList.toggle("done", i < n);
  });
  const showPage = (name) => pages.forEach((p) => p.classList.toggle("on", p.dataset.page === name));
  const select = (i) => tabs.forEach((t, n) => t.setAttribute("aria-pressed", String(n === i)));
  let startTimer = 0;

  function land(s) {
    caret.classList.add("off");
    omni.classList.remove("focus");
    key.classList.remove("on");
    url.classList.remove("sel");
    url.textContent = s.url;
    title.textContent = s.title;
    fav.className = "b-fav " + s.fav;
    showPage(s.page);
    caption.innerHTML = s.caption;
    step(2);
  }

  async function play(i, id) {
    const s = scenes[i];
    select(i);
    if (reduced) { land(s); return; }
    await gate();
    if (id !== run) return;
    // Click the address bar: the current URL gets selected…
    step(0);
    omni.classList.add("focus");
    url.classList.add("sel");
    await sleep(450);
    if (id !== run) return;
    // …then type the go/ link.
    url.textContent = "";
    url.classList.remove("sel");
    caret.classList.remove("off");
    for (const ch of s.typed) {
      if (id !== run) return;
      url.textContent += ch;
      await sleep(65 + Math.random() * 55);
    }
    await sleep(300);
    if (id !== run) return;
    key.classList.add("on");
    step(1);
    await sleep(420);
    if (id !== run) return;
    bar.classList.remove("run");
    void bar.offsetWidth; // restart the transition
    bar.classList.add("run");
    await sleep(520);
    if (id !== run) return;
    land(s);
    await sleep(250);
    bar.classList.remove("run");
  }

  async function loop(start) {
    const id = ++run;
    for (let i = start; ; i = (i + 1) % scenes.length) {
      await play(i, id);
      if (id !== run || reduced) return;
      await sleep(3600);
      if (id !== run) return;
    }
  }

  tabs.forEach((t) => t.addEventListener("click", () => {
    clearTimeout(startTimer);
    // Announce captions only once someone is interacting, not on every autoplay step.
    caption.setAttribute("aria-live", "polite");
    const i = Number(t.dataset.scene);
    if (reduced) { ++run; play(i, run); } else { loop(i); }
  }));
  // First paint already shows goli at work; the typing loop picks up from there.
  land(scenes[0]);
  select(0);
  if (!reduced) startTimer = setTimeout(() => loop(1), 2800);
}

// ── 2. The phone demo ──────────────────────────────────────────────────────
const phone = $("phone");
if (phone) {
  const screens = phone.querySelectorAll(".scr");
  const icon = $("ios-goli"), input = $("m-input"), go = $("m-go");
  const show = (name) => screens.forEach((s) => s.classList.toggle("on", s.dataset.scr === name));
  const placeholder = input.textContent;
  const gate = visibilityGate(phone);

  if (reduced) {
    show("app");
  } else {
    (async () => {
      for (;;) {
        await gate();
        show("home");
        input.textContent = placeholder;
        input.classList.remove("typing");
        await sleep(1600);
        icon.classList.add("tap");
        await sleep(220);
        icon.classList.remove("tap");
        show("app");
        await sleep(900);
        input.textContent = "";
        input.classList.add("typing");
        for (const ch of "standup") { input.textContent += ch; await sleep(110); }
        await sleep(450);
        go.classList.add("press");
        await sleep(160);
        go.classList.remove("press");
        await sleep(250);
        show("dest");
        await sleep(3200);
      }
    })();
  }
}

// ── 3. The terminal demo ───────────────────────────────────────────────────
const term = $("term");
if (term && !reduced) {
  const P = '<span class="p">$</span> ';
  const sessions = [
    ['goli standup        <span class="m"># opens it in your browser</span>', "https://meet.google.com/abc-defg-hij"],
    ["goli add pr https://github.com/acme/app/pull/{1}", "go/pr → https://github.com/acme/app/pull/{1}"],
    ["goli find revenue", '<span class="m"> 85%</span>  <span class="k">forecast</span>             Revenue forecast sheet'],
    ["goli check", '<span class="m">✗</span>  <span class="k">oldwiki</span>   404 Not Found  https://wiki.example.com/old\n<span class="m">✓</span>  <span class="k">standup</span>   200 OK         https://meet.google.com/abc-defg-hij\ngoli: 1 broken link(s)'],
  ];
  const gate = visibilityGate(term);
  (async () => {
    for (;;) {
      let done = "";
      for (const [cmd, out] of sessions) {
        await gate();
        const plain = cmd.replace(/<[^>]+>/g, "");
        let typed = "";
        for (const ch of plain) {
          typed += ch;
          term.innerHTML = done + P + typed + '<span class="t-caret"></span>';
          await sleep(45 + Math.random() * 40);
        }
        await sleep(350);
        done += P + cmd + "\n" + out + "\n\n";
        term.innerHTML = done + P + '<span class="t-caret"></span>';
        await sleep(1500);
      }
      await sleep(2500);
    }
  })();
}
