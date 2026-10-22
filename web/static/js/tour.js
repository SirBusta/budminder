// The first-run tour (handlers/tour.go, common/tour.html): four tips shown
// over the page, a few cards each. A card is either a picture whose lines
// fill in one at a time while a purple ring beats around the spot each line
// talks about (spots in /static/tour/img/rings.json), or a short clip that
// plays one step at a time, each step's line first. "Reduce motion" shows
// everything at once. Text: the "tour_" translations, keyed
// tour_<tip>_<card>_<line>.
(() => {
    "use strict";
    const data = document.getElementById("tourData");
    if (!data) return;
    const CFG = JSON.parse(data.textContent || "{}");
    const TOUR = CFG.tour || {};
    const TX = TOUR.t || {};
    const t = (key, vars) => {
        let s = TX[key] || key;
        if (vars) for (const k of Object.keys(vars)) s = s.split("{" + k + "}").join(vars[k]);
        return s;
    };
    const esc = s => String(s).replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    const BASE = "/static/tour/";
    const ASKED_KEY = "bm-tour-asked";
    const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    // Each tip's cards. fig: the picture (on: a second picture it fades to
    // when its first ring lights); rings: one entry per line, the spot it
    // lights or null; clip: a recording with `steps` lines; after: a line
    // under the steps; muted: the index of a line shown smaller.
    const TIPS = [
        [{ fig: "t1-menu", on: "t1-menu-on", rings: [null, null, 0] },
            { clip: "t1-find", steps: 6, after: true },
            { fig: "t1-strain", rings: [0, 1, 2] },
            { fig: "t1-addplant", rings: [null, 0, 1, 2, 3, null, null], muted: 6 }],
        [{ fig: "t2-track", rings: [0, 1, 2] },
            { clip: "t2-stage", steps: 3 },
            { fig: "t2-facts", rings: [0, null, 1, 2] }],
        [{ fig: "t3-notebook", rings: [0, 1] },
            { clip: "t3-log", steps: 4 },
            { fig: "t3-photos", rings: [0, 1, 2, 3] }],
        [{ fig: "t4-settings", rings: [null, 0, 1] },
            { clip: "t4-sensors", steps: 4 },
            { fig: "t4-ready", rings: [0, 1, null] }],
    ];
    const CARDS = TIPS.flatMap((cards, n) => cards.map((c, j) => ({ ...c, tip: n + 1, key: `tour_${n + 1}_${j + 1}` })));

    let RINGS = null;
    const CLIPS = {};
    let loading = null;
    function load() {
        if (!loading) {
            loading = Promise.all([
                fetch(BASE + "img/rings.json").then(r => r.json()).then(j => { RINGS = j; }),
                ...CARDS.filter(c => c.clip).map(c => fetch(`${BASE}clips/${c.clip}.json`).then(r => r.json()).then(j => { CLIPS[c.clip] = j; })),
            ]).catch(() => { /* pictures still show; clips fall back to their stills */ });
        }
        return loading;
    }

    // ---------- The overlay ----------
    let root = null, wrap = null, i = 0, lastFocus = null;
    function show() {
        if (!root) {
            root = document.createElement("div");
            root.className = "bm-tour";
            root.innerHTML = `<div class="bt-veil"></div><div class="bt-wrap"></div>`;
            document.body.appendChild(root);
            wrap = root.querySelector(".bt-wrap");
        }
        if (root.hidden !== false || !document.body.classList.contains("bm-tour-open")) lastFocus = document.activeElement;
        root.hidden = false;
        document.body.classList.add("bm-tour-open");
    }
    function close() {
        run++;
        paused = false;
        if (root) { root.hidden = true; wrap.innerHTML = ""; }
        document.body.classList.remove("bm-tour-open");
        if (lastFocus && lastFocus.focus) lastFocus.focus();
    }
    function start(at = 0) {
        show();
        load().then(() => { i = Math.max(0, Math.min(at, CARDS.length - 1)); render(); });
    }
    const firstCardOf = tip => Math.max(0, CARDS.findIndex(c => c.tip === tip));

    // ---------- Asking first ----------
    function ask() {
        show();
        const demo = !!CFG.demo;
        wrap.innerHTML = `<section class="bt-card bt-ask" role="dialog" aria-modal="true" aria-labelledby="btTitle">
            <div class="bt-top"><span></span><button type="button" class="bt-x" data-bt="no" aria-label="${esc(t("tour_no"))}" title="${esc(t("tour_no"))}"><i class="fa-solid fa-xmark"></i></button></div>
            <div class="bt-body"><div class="bt-divider" aria-hidden="true"><i class="fa-solid fa-seedling"></i></div>
                <h2 id="btTitle">${esc(t(demo ? "tour_ask_demo_title" : "tour_ask_title"))}</h2>
                <p class="bt-ask-text">${esc(t(demo ? "tour_ask_demo_text" : "tour_ask_text"))}</p>
                <p class="bt-after">${esc(t("tour_ask_later"))}</p></div>
            <div class="bt-foot"><span class="me-auto"></span>
                <button type="button" class="btn btn-light" data-bt="no">${esc(t("tour_no"))}</button>
                <button type="button" class="btn btn-success" data-bt="yes"><i class="fa-solid fa-seedling me-1"></i>${esc(t("tour_yes"))}</button></div></section>`;
        wrap.querySelector('[data-bt="yes"]').focus({ preventScroll: true });
        load();
    }
    // The answer, either way, so the question isn't asked again: in this
    // browser for the demo (nothing is saved there), else in the database.
    function answered() {
        if (CFG.demo) {
            try { localStorage.setItem(ASKED_KEY, "1"); } catch (e) { /* private mode: asked again next visit */ }
            return;
        }
        fetch("/tour/answered", { method: "POST", headers: { "Accept": "application/json" } }).catch(() => {});
    }
    function demoAsked() {
        try { return localStorage.getItem(ASKED_KEY) === "1"; } catch (e) { return false; }
    }

    // ---------- A card ----------
    function figHTML(c) {
        const spots = (RINGS || {})[c.fig] || {};
        const alt = esc(t(c.key + "_alt"));
        const one = th => `<img class="bt-${th}" src="${BASE}img/${c.fig}-${th}.webp" alt="${alt}">` +
            (c.on ? `<img class="bt-${th} on" src="${BASE}img/${c.on}-${th}.webp" alt="">` : "") +
            (spots[th] || []).map(([x, y, w, h, round], k) => `<div class="bt-spot bt-${th}${round ? " round" : ""}" data-spot="${k}" style="left:calc(${x * 100}% - 4px);top:calc(${y * 100}% - 4px);width:calc(${w * 100}% + 8px);height:calc(${h * 100}% + 8px)"></div>`).join("");
        // Only the picture for the theme in use (theme.js sets data-bs-theme).
        const theme = document.documentElement.getAttribute("data-bs-theme") === "dark" ? "dark" : "light";
        return `<figure class="bt-fig" id="btFig">${one(theme)}</figure>` +
            c.rings.map((_, k) => `<p class="bt-ln${k === c.muted ? " muted" : ""}">${t(`${c.key}_${k + 1}`)}</p>`).join("");
    }
    function clipHTML(c) {
        const poster = `${BASE}clips/${c.clip}-poster.webp`;
        const steps = `<ul class="bt-caps" aria-live="polite">${Array.from({ length: c.steps }, (_, k) => `<li class="bt-ln">${t(`${c.key}_${k + 1}`)}</li>`).join("")}</ul>`;
        const after = c.after ? `<p class="bt-after">${t(c.key + "_after")}</p>` : "";
        if (still || !CLIPS[c.clip]) return `<figure class="bt-clip"><img src="${poster}" alt=""></figure>${steps}${after}`;
        return `<figure class="bt-clip"><video id="btVid" src="${BASE}clips/${c.clip}.webm" poster="${poster}" muted playsinline preload="auto"></video>
            <div class="bt-ctl"><button type="button" id="btPause" aria-label="${esc(t("tour_pause"))}"><i class="fa-solid fa-pause"></i></button>
            <button type="button" id="btReplay" aria-label="${esc(t("tour_replay"))}"><i class="fa-solid fa-rotate-left"></i></button></div></figure>${steps}${after}`;
    }
    function render() {
        const c = CARDS[i], last = i === CARDS.length - 1, tipEnds = !last && CARDS[i + 1].tip !== c.tip;
        const next = last ? `<i class="fa-solid fa-seedling me-1"></i>${esc(t("tour_finish"))}` : esc(t(tipEnds ? "tour_next_tip" : "tour_next"));
        wrap.innerHTML = `<section class="bt-card" role="dialog" aria-modal="true" aria-labelledby="btTitle">
            <div class="bt-top"><span class="bt-tipno">${esc(t("tour_tip_of", { n: c.tip, total: TIPS.length }))}</span>
                <button type="button" class="bt-x" data-bt="skip" aria-label="${esc(t("tour_skip"))}" title="${esc(t("tour_skip"))}"><i class="fa-solid fa-xmark"></i></button></div>
            <div class="bt-body"><div class="bt-divider" aria-hidden="true"><i class="fa-solid fa-seedling"></i></div>
                <h2 id="btTitle">${esc(t(c.key + "_title"))}</h2>${c.clip ? clipHTML(c) : figHTML(c)}</div>
            <div class="bt-foot">
                <div class="bt-dots" aria-hidden="true">${CARDS.map((x, k) => x.tip === c.tip ? `<i class="${k === i ? "on" : ""}"></i>` : "").join("")}</div>
                <button type="button" class="btn btn-link bt-skip" data-bt="skip">${esc(t("tour_skip"))}</button>
                ${i > 0 ? `<button type="button" class="btn btn-light" data-bt="back">${esc(t("tour_back"))}</button>` : ""}
                <button type="button" class="btn btn-success" data-bt="${last ? "finish" : "next"}">${next}</button>
            </div></section>`;
        wrap.querySelector(`[data-bt="${last ? "finish" : "next"}"]`).focus({ preventScroll: true });
        play(c);
    }

    // ---------- Lines that fill in, rings that beat ----------
    // run changes on every render, so a sequence from an earlier card stops.
    let run = 0, paused = false;
    async function wait(ms, r) {
        for (let w = 0; w < ms;) {
            const step = Math.min(ms - w, 30);
            await new Promise(z => setTimeout(z, step));
            if (r !== run) throw 0;
            if (!paused) w += step;
        }
    }
    // Every character starts see-through, so the card keeps its size while a line fills in.
    function ghost(el) {
        if (el._html == null) el._html = el.innerHTML; else el.innerHTML = el._html;
        el.classList.remove("now", "seen");
        const nodes = [], walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        while (walk.nextNode()) nodes.push(walk.currentNode);
        el._parts = nodes.map(n => {
            const g = document.createElement("span"), typed = document.createTextNode("");
            g.className = "bt-ghost"; g.textContent = n.data; n.replaceWith(typed, g);
            return { typed, g };
        });
    }
    const caret = document.createElement("span");
    caret.className = "bt-caret";
    async function type(el, r, ms) {
        el.classList.add("now");
        el.scrollIntoView({ block: "nearest", behavior: "smooth" });
        try {
            for (const { typed, g } of el._parts) {
                g.before(caret);
                while (g.textContent) { typed.data += g.textContent[0]; g.textContent = g.textContent.slice(1); await wait(ms, r); }
            }
        } finally { caret.remove(); }
    }
    function spot(k) {
        wrap.querySelectorAll(".bt-spot").forEach(s => s.classList.toggle("on", k != null && +s.dataset.spot === k));
        const fig = document.getElementById("btFig");
        if (k === 0 && fig) setTimeout(() => fig.classList.add("lit"), 450);
    }
    async function playLines(c, r) {
        const lines = [...wrap.querySelectorAll(".bt-fig ~ .bt-ln")];
        lines.forEach(ghost);
        await wait(400, r);
        for (const [k, el] of lines.entries()) {
            const ring = c.rings[k];
            if (ring == null) spot(null);
            await type(el, r, 18);
            if (ring != null) { await wait(350, r); spot(ring); await wait(1500, r); } else await wait(450, r);
            el.classList.remove("now");
        }
    }
    // Play the clip up to `end` seconds, then stop there.
    function segment(v, end, r) {
        return new Promise((ok, no) => {
            const check = () => {
                if (r !== run) { v.pause(); return no(0); }
                if (v.currentTime >= end - 0.03 || v.ended) { v.pause(); return ok(); }
                if (paused) { if (!v.paused) v.pause(); } else if (v.paused) v.play().catch(() => {});
                setTimeout(check, 15);
            };
            check();
        });
    }
    async function playClip(c, r) {
        const v = document.getElementById("btVid"), meta = CLIPS[c.clip];
        if (!v || !meta) return;
        // A browser that can't play the clip gets its still and every step.
        v.addEventListener("error", () => {
            if (r !== run) return;
            run++;
            v.closest(".bt-clip").innerHTML = `<img src="${BASE}clips/${c.clip}-poster.webp" alt="">`;
            wrap.querySelectorAll(".bt-caps .bt-ln").forEach(el => { if (el._html != null) el.innerHTML = el._html; el.classList.remove("now", "seen"); });
        });
        const lines = [...wrap.querySelectorAll(".bt-caps .bt-ln")];
        const pb = document.getElementById("btPause");
        pb.addEventListener("click", () => {
            paused = !paused;
            pb.innerHTML = `<i class="fa-solid fa-${paused ? "play" : "pause"}"></i>`;
            pb.setAttribute("aria-label", t(paused ? "tour_play" : "tour_pause"));
        });
        document.getElementById("btReplay").addEventListener("click", () => { paused = false; render(); });
        for (;;) {
            lines.forEach(ghost);
            v.pause();
            v.currentTime = 0;
            await wait(500, r);
            for (const [k, el] of lines.entries()) {
                if (k) lines[k - 1].classList.replace("now", "seen");
                await type(el, r, 22);
                await wait(700, r); // a beat to read it, then the part it describes
                await segment(v, k + 1 < meta.at.length ? meta.at[k + 1] : (v.duration || meta.duration), r);
            }
            lines[lines.length - 1].classList.replace("now", "seen");
            await wait(3500, r); // hold the end, then start over
        }
    }
    function play(c) {
        const r = ++run;
        paused = false;
        if (still) {
            wrap.querySelectorAll(".bt-spot").forEach(s => s.classList.add("on"));
            const fig = document.getElementById("btFig");
            if (fig) fig.classList.add("lit");
            return;
        }
        (c.clip ? playClip(c, r) : playLines(c, r)).catch(() => {});
    }

    // ---------- Events ----------
    document.addEventListener("click", e => {
        const go = e.target.closest("[data-tour-start]");
        if (go) { e.preventDefault(); start(firstCardOf(+go.dataset.tourStart || 1)); return; }
        const a = e.target.closest("[data-bt]");
        if (!a || !root || !root.contains(a)) return;
        const act = a.dataset.bt;
        if (act === "yes") { answered(); start(0); }
        else if (act === "no") { answered(); close(); }
        else if (act === "skip") close();
        else if (act === "next") { i++; render(); }
        else if (act === "back") { i = Math.max(0, i - 1); render(); }
        else if (act === "finish") { close(); window.location.href = "/strains"; }
    });
    document.addEventListener("keydown", e => {
        if (!root || root.hidden) return;
        if (e.key === "Escape") {
            if (wrap.querySelector(".bt-ask")) answered();
            close();
            return;
        }
        if (wrap.querySelector(".bt-ask") || e.target.closest("input, textarea, select")) return;
        if (e.key === "ArrowRight" && i < CARDS.length - 1) { i++; render(); }
        if (e.key === "ArrowLeft" && i > 0) { i--; render(); }
    });

    window.bmTour = { start: tip => start(firstCardOf(tip || 1)) };

    // Ask on Today: a new install once (TourPageData decides), and each demo
    // visitor once in their browser.
    if (TOUR.ask || (CFG.demo && TOUR.askable && !demoAsked())) ask();
})();
