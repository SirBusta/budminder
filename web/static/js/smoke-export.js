// Smoke report export (the Export button on a report's page). Draws the report
// as a one-page magazine page in two styles, Humidor (.v-a) and Botanical (.v-c),
// and saves it as PNG or PDF, all in the browser. Page data: #srExportData
// (handlers/smoke_export.go). Page styles: /static/css/smoke-sheet.css, used inside
// shadow roots for the previews and inside an SVG for the files, so both look alike.
// Design and decisions: data/mockups/smoke/export/v6.html and DECISIONS.md.
(function () {
    "use strict";
    const page = document.getElementById("smokeReport");
    const dataEl = document.getElementById("srExportData");
    const button = document.getElementById("srExport");
    if (!page || !dataEl || !button) return;
    const D = JSON.parse(dataEl.textContent);
    const T = D.t || {};
    const lang = page.dataset.lang || "en";
    const W = 816, H = 1056; // US Letter at 96 px per inch

    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    const fill = (s, vals) => String(s || "").replace(/\{(\w+)\}/g, (m, k) => (k in vals ? vals[k] : m));
    const fmtDate = iso => /^\d{4}-\d{2}-\d{2}$/.test(iso || "") ? new Date(iso + "T12:00:00").toLocaleDateString(lang, { month: "short", day: "numeric", year: "numeric" }) : (iso || "");
    // Values that are dates come as "@date:YYYY-MM-DD" and are shown in the reader's format.
    const val = v => String(v).startsWith("@date:") ? fmtDate(String(v).slice(6)) : v;

    // ---- Fonts (web/static/vendor/fonts, with their licenses) ----
    const FONT_DIR = "/static/vendor/fonts/";
    const FONTS = [
        { look: "a", family: "BM Pagella", file: "texgyrepagella-regular.otf", weight: "400", style: "normal", type: "opentype" },
        { look: "a", family: "BM Pagella", file: "texgyrepagella-italic.otf", weight: "400", style: "italic", type: "opentype" },
        { look: "a", family: "BM Pagella", file: "texgyrepagella-bold.otf", weight: "700", style: "normal", type: "opentype" },
        { look: "c", family: "BM Source Serif", file: "SourceSerif4-Latin.ttf", weight: "200 900", style: "normal", type: "truetype" },
        { look: "c", family: "BM Playfair", file: "PlayfairDisplay.ttf", weight: "400 900", style: "normal", type: "truetype" },
        { look: "c", family: "BM Barlow", file: "Barlow-Regular.ttf", weight: "400", style: "normal", type: "truetype" },
        { look: "c", family: "BM Barlow", file: "Barlow-Bold.ttf", weight: "700", style: "normal", type: "truetype" },
    ];
    let fontsReady = null;
    function loadFonts() {
        if (!fontsReady) fontsReady = Promise.all(FONTS.map(f => {
            const face = new FontFace(f.family, `url(${FONT_DIR}${f.file})`, { weight: f.weight, style: f.style });
            document.fonts.add(face);
            return face.load();
        })).catch(e => { fontsReady = null; throw e; }); // "Try again" must load again
        return fontsReady;
    }

    let sheetText = null, sheetCSS = null;
    async function loadSheet() {
        if (sheetText) return;
        const r = await fetch("/static/css/smoke-sheet.css");
        if (!r.ok) throw new Error("smoke-sheet.css " + r.status);
        sheetText = await r.text();
        sheetCSS = new CSSStyleSheet();
        sheetCSS.replaceSync(sheetText);
    }
    const hostCSS = new CSSStyleSheet();
    hostCSS.replaceSync(":host { all: initial; display: block; position: relative; }");
    function shadow(host) {
        const root = host.shadowRoot || host.attachShadow({ mode: "open" });
        root.adoptedStyleSheets = [hostCSS, sheetCSS];
        return root;
    }

    // ---- Icons (inline SVG: the files can't use the icon font) ----
    const ICONS = {
        yes: '<path d="M3 8.6l3.4 3.4L13 4.6"/>',
        no: '<path d="M4 4l8 8M12 4l-8 8"/>',
        maybe: '<path d="M5.6 5.6a2.4 2.4 0 1 1 3.4 2.2c-.6.3-1 .8-1 1.5v.5"/><circle class="fill" cx="8" cy="12.8" r="1.1"/>',
        seed: '<path d="M8 14.5V8.5"/><path class="fill" d="M8 9.2C8 6 5.9 4 2.6 4c0 3.2 2.1 5.2 5.4 5.2zM8 8c0-2.9 1.9-4.6 5.2-4.6C13.2 6.3 11.3 8 8 8z"/>',
        arrow: '<path d="M2.5 8h10.5M9.5 4.5L13 8l-3.5 3.5"/>',
    };
    const icon = k => `<svg class="ic" viewBox="0 0 16 16" aria-hidden="true">${ICONS[k] || ""}</svg>`;
    const againIcon = () => icon(D.again_kind === "no" ? "no" : D.again_kind === "maybe" ? "maybe" : "yes");

    // ---- The pages (from mockup v6) ----
    const PARA = "¶";
    const tok = s => String(s || "").replace(/\n\s*\n/g, ` ${PARA} `).split(/\s+/).filter(Boolean);
    function paras(t) {
        const ps = [[]];
        for (const w of t) { if (w === PARA) { if (ps[ps.length - 1].length) ps.push([]); } else ps[ps.length - 1].push(w); }
        return ps.filter(p => p.length).map(p => `<p>${esc(p.join(" "))}</p>`).join("");
    }
    const sub = () => (D.sub || []).map(s => s.b ? `<b>${esc(s.t)}</b>` : esc(s.t)).join("");
    const smoked = () => D.smoked ? esc(fill(T.sr_x_smoked, { date: fmtDate(D.smoked) })) : "";
    const MAST = () => `<b>${esc(T.sr_x_mast)}</b><span>${smoked()}</span>`;
    const FOOT = () => `<div class="foot"><span>BudMinder</span><span>${esc(T.sr_x_foot)}</span></div>`;
    const CONT = `<div class="cont"><span class="cont-t"></span>${icon("arrow")}</div>`;
    const score = () => esc(D.score || "–");
    const lead = (k, v, extra = "") => `<div class="lead"><span>${esc(k)}</span><span class="dots"></span><b>${esc(val(v))}</b>${extra}</div>`;
    const pips = n => `<span class="pips">${Array.from({ length: 10 }, (_, i) => `<i class="${i < n ? "f" : ""}"></i>`).join("")}</span>`;
    const tag = k => (D.tags || []).find(t => t.key === k);
    // "Would grow again" on two lines in the ribbon: the last two words go below.
    function ribbonText(s) {
        const w = String(s).split(" ");
        return w.length > 2 ? esc(w.slice(0, -2).join(" ")) + "<br>" + esc(w.slice(-2).join(" ")) : esc(s);
    }
    function photoA(light) {
        if (!D.photo) return `<div class="photo"><div class="none">${icon("seed")}</div></div>`;
        const img = `<img src="${esc(D.photo)}" alt="">`;
        return `<div class="photo">${light ? `<div class="plate">${img}</div>` : img}</div>`;
    }
    const growA = () => `<div class="grow"><h3>${esc(D.box.title)}</h3><div class="gl">${D.box.rows.map(([k, v]) => lead(k, v)).join("")}</div></div>`;
    function tagsA() {
        const t = D.tags || [];
        if (!t.length) return "";
        return `<div class="tags" style="grid-template-columns:repeat(${t.length},1fr)">${t.map(g => `<div><h3>${esc(g.label)}</h3><div>${esc(g.words.join(", "))}</div></div>`).join("")}</div>`;
    }
    function lookA(light, o) {
        const notes = o.notes.length ? `<div><h3 style="margin-top:0">${esc(T.sr_x_tasting)}</h3><div class="notes">${paras(o.notes)}</div>${o.cont ? CONT : ""}</div>` : "";
        const box = D.box && !o.split ? growA() : "";
        return `
        <div class="mast">${MAST()}</div>
        <div class="head">
            <div>
                ${D.kicker ? `<div class="kick">${esc(D.kicker)}</div>` : ""}
                <h1 class="fit" data-min="62">${esc(D.name)}</h1>
                ${D.sub && D.sub.length ? `<div class="sub">${sub()}</div>` : ""}
            </div>
            ${D.again ? `<div class="ribbon">${againIcon()}${ribbonText(D.again)}</div>` : ""}
        </div>
        <div class="row2">
            ${photoA(light)}
            <div>
                <div class="seal">
                    <div class="ring"><span class="n">${score()}</span><span class="of">${esc(T.sr_x_out_of)}</span></div>
                    <p>${(D.seal || []).map(esc).join("<br>")}</p>
                </div>
                ${(D.glance || []).length ? `<div><h3>${esc(T.sr_x_glance)}</h3>${D.glance.map(([k, v]) => lead(k, v)).join("")}</div>` : ""}
                <div><h3>${esc(T.sr_scores)}</h3>${(D.scores || []).map(s => lead(s.label, s.value || "–", pips(s.value))).join("")}</div>
            </div>
        </div>
        ${notes || box ? `<div class="row3 ${notes && box ? "" : "solo"}">${notes}${box}</div>` : ""}
        ${tagsA()}
        ${FOOT()}`;
    }
    // Page 2 (Humidor): the rest of the notes, then the grow details.
    function lookA2(o) {
        return `
        <div class="mast">${MAST()}</div>
        <div class="p2h"><div><div class="kick"><span class="cont-f"></span></div><div class="p2n">${esc(D.name)}</div></div>
            <div class="ring sm"><span class="n">${score()}</span><span class="of">${esc(T.sr_x_out_of)}</span></div></div>
        ${o.notes.length ? `<div class="notes">${paras(o.notes)}</div>` : ""}
        ${D.box ? growA() : ""}
        ${FOOT()}`;
    }
    function strip() {
        const s = D.strip;
        return `<div class="strip"><h2>${esc(s.title)}</h2><div class="tl" style="grid-template-columns:repeat(${s.shots.length},1fr)">${s.shots.map(x =>
            `<figure><img src="${esc(x.url)}" alt=""><figcaption>${x.day ? `<b>${esc(x.day)}</b>` : ""}${x.day && x.stage ? " &middot; " : ""}${esc(x.stage)}</figcaption></figure>`).join("")}</div></div>`;
    }
    const pillGroup = (g, warm) => g ? `<div class="grp"><div class="lab">${esc(g.label)}</div><div class="pills">${g.words.map(w => `<span class="pill${warm ? " warm" : ""}">${esc(w)}</span>`).join("")}</div></div>` : "";
    function gridC(o) {
        const mt = h => h.replace("<h2>", '<h2 style="margin-top:14px">');
        const notes = o.notes.length ? `<h2>${esc(T.sr_x_tasting)}</h2><div class="notes">${paras(o.notes)}</div>` : "";
        const scores = `<h2>${esc(T.sr_scores)}</h2>${(D.scores || []).map(s => `<div class="bar"><span>${esc(s.label)}</span><span class="t"><i style="width:${s.value * 10}%"></i></span><b>${s.value || "–"}</b></div>`).join("")}`;
        const fe = tag("effect"), si = tag("side"), sm = tag("smell"), ta = tag("taste"), bu = tag("bud");
        const felt = fe || si ? `<h2>${esc(T.sr_how_it_felt)}</h2>${pillGroup(fe)}${pillGroup(si, true)}` : "";
        const nose = sm || ta || bu ? `<h2>${esc(T.sr_nose)}</h2>${pillGroup(sm)}${pillGroup(ta)}${pillGroup(bu)}` : "";
        const box = D.box_c ? `<h2>${esc(D.box_c.title)}</h2><dl>${D.box_c.rows.map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(val(v))}</dd></div>`).join("")}</dl>` : "";
        // Compact: notes sit above the grid, so the columns are rebalanced to keep the page short.
        return o.compact
            ? `<div class="grid"><div>${scores}${felt ? mt(felt) : ""}</div><div>${nose}</div><div>${box}</div></div>`
            : `<div class="grid"><div>${notes}${notes ? mt(scores) : scores}</div><div>${felt}${felt && nose ? mt(nose) : nose}</div><div>${box}</div></div>`;
    }
    function chipsC() {
        const c = (D.chips || []).map(x => `<span class="chip">${x.b ? icon("seed") : ""}${esc(x.t)}</span>`).join("");
        return c + (D.again ? `<span class="chip${D.again_kind === "yes" ? " go" : ""}">${againIcon()}${esc(D.again)}</span>` : "");
    }
    function lookC(light, o) {
        return `
        <div class="hero${D.photo ? "" : " none"}">
            ${D.photo ? `<img src="${esc(D.photo)}" alt="">` : ""}
            <div class="mast">${MAST()}</div>
            <div class="score"><span class="n">${score()}<small>/10</small></span><span class="l">${esc(T.sr_x_overall)}</span></div>
            <div class="title">
                ${D.kicker ? `<div class="kick">${esc(D.kicker)}</div>` : ""}
                <h1 class="${light ? "fitw" : "fit"}" data-min="${light ? 48 : 64}">${esc(D.name)}</h1>
                <div class="chips">${chipsC()}</div>
            </div>
        </div>
        ${o.compact && o.notes.length ? `<div class="cnotes"><h2>${esc(T.sr_x_tasting)}</h2><div class="notes">${paras(o.notes)}</div>${o.cont ? CONT : ""}</div>` : ""}
        ${o.split ? "" : gridC(o)}
        ${D.strip && !o.noStrip ? strip() : ""}
        ${FOOT()}`;
    }
    // Page 2 (Botanical): the rest of the notes, then the details and the plant photos.
    function lookC2(o) {
        return `
        <div class="p2top"><div class="mast2">${MAST()}</div>
            <div class="p2h"><div><div class="kick"><span class="cont-f"></span></div><div class="p2n">${esc(D.name)}</div></div>
                <div class="score sm"><span class="n">${score()}<small>/10</small></span><span class="l">${esc(T.sr_x_overall)}</span></div></div></div>
        ${o.notes.length ? `<div class="cnotes"><div class="notes">${paras(o.notes)}</div></div>` : ""}
        ${gridC({ notes: [], compact: true })}
        ${D.strip ? strip() : ""}
        ${FOOT()}`;
    }

    // ---- Layout: measured at full size in a hidden shadow root ----
    let measureRoot = null;
    function fitTitles(sheet) {
        sheet.querySelectorAll("h1.fit").forEach(h => {
            h.style.fontSize = "";
            let fs = parseFloat(getComputedStyle(h).fontSize); const min = +h.dataset.min;
            while (h.scrollWidth > h.clientWidth + 1 && fs > min) { fs -= 2; h.style.fontSize = fs + "px"; }
            if (h.scrollWidth > h.clientWidth + 1) h.style.whiteSpace = "normal";
        });
        // Titles that may wrap (Botanical on a light cover): shrink until the longest word fits.
        sheet.querySelectorAll("h1.fitw").forEach(h => {
            h.style.fontSize = "";
            let fs = parseFloat(getComputedStyle(h).fontSize); const min = +h.dataset.min;
            while (h.scrollWidth > h.clientWidth + 1 && fs > min) { fs -= 2; h.style.fontSize = fs + "px"; }
        });
    }
    // Space left between the content and the footer (negative = overflow).
    function bottomOf(s) { const top = s.getBoundingClientRect().top; let b = 0; for (const c of s.children) if (!c.classList.contains("foot")) b = Math.max(b, c.getBoundingClientRect().bottom - top); return b; }
    function free(s) { return s.querySelector(".foot").getBoundingClientRect().top - s.getBoundingClientRect().top - 26 - bottomOf(s); }
    // Leftover space goes to the photo (Humidor: 60%, the rest spreads the sections).
    function fillGap(s, look) {
        const f = Math.floor(free(s)); if (f <= 4) return;
        if (look === "c" && !D.photo) { const h = s.querySelector(".hero"); h.style.height = h.offsetHeight + Math.min(f, 160) + "px"; return; }
        if (look === "a") {
            const ph = s.querySelector(".photo"), toPhoto = Math.round(f * .6), rest = f - toPhoto;
            ph.style.height = ph.offsetHeight + toPhoto + "px";
            for (const sel of [".row3", ".tags"]) { const e = s.querySelector(sel); if (e) e.style.marginTop = parseFloat(getComputedStyle(e).marginTop) + rest / 2 + "px"; }
        } else { const h = s.querySelector(".hero"); h.style.height = h.offsetHeight + f + "px"; }
    }
    function mk(look, light, o, p2, pdf) {
        const cls = ["sheet", "v-" + look];
        if (light && !p2) cls.push("light");
        if (!D.strip) cls.push("noplant");
        if (o.compact) cls.push("compact");
        if (p2) cls.push("p2");
        if (pdf) cls.push("pdf");
        measureRoot.innerHTML = `<article class="${cls.join(" ")}" lang="${esc(lang)}"></article>`;
        const s = measureRoot.firstChild;
        s.innerHTML = p2 ? (look === "a" ? lookA2(o) : lookC2(o)) : (look === "a" ? lookA(light, o) : lookC(light, o));
        fitTitles(s);
        return s;
    }
    const snap = (s, h) => ({ cls: s.className, html: s.innerHTML, h });
    const CACHE = {};
    // Short notes: one page (a page over by up to 70 px shrinks the photo). Longer:
    // compact (smaller photo, notes in two columns). Still too long: a second page.
    function compose(look, light, pdf) {
        const key = [look, light, pdf].join("|"); if (CACHE[key]) return CACHE[key];
        const t = tok(D.notes); let s = mk(look, light, { notes: t }, false, pdf), out;
        const over = -Math.floor(free(s));
        if (over > 0 && over <= 70) { const e = s.querySelector(look === "a" ? ".photo" : ".hero"); e.style.height = e.offsetHeight - over + "px"; }
        if (free(s) >= 0) { fillGap(s, look); out = [snap(s, H)]; }
        else {
            s = mk(look, light, { notes: t, compact: true }, false, pdf);
            if (free(s) >= 0) { fillGap(s, look); out = [snap(s, H)]; }
            else {
                let lo = 1, hi = t.length, best = 1;
                const part = n => mk(look, light, { notes: t.slice(0, n), compact: true, cont: true, noStrip: true, split: true }, false, pdf);
                while (lo <= hi) { const mid = (lo + hi) >> 1; s = part(mid); if (free(s) >= 0) { best = mid; lo = mid + 1; } else hi = mid - 1; }
                s = part(best); fillGap(s, look);
                const p1 = snap(s, H);
                const s2 = mk(look, light, { notes: t.slice(best) }, true, pdf);
                out = [p1, snap(s2, Math.max(Math.ceil(bottomOf(s2)) + 64, 380))];
            }
        }
        return CACHE[key] = out;
    }

    // ---- The window ----
    const STORE = "bm.smokeExport";
    const saved = (() => { try { return JSON.parse(localStorage.getItem(STORE) || "{}"); } catch (e) { return {}; } })();
    const st = {
        look: saved.look === "c" ? "c" : "a", blend: !!saved.blend,
        format: saved.format === "pdf" ? "pdf" : "png", pages: saved.pages === "tall" ? "tall" : "two",
    };
    const remember = () => { try { localStorage.setItem(STORE, JSON.stringify(st)); } catch (e) { /* private mode */ } };
    let lightPhoto = false, modal = null, busy = false;

    function seg(name, value, iconCls, label, note, on) {
        return `<label data-v="${value}" class="${on ? "on" : ""}"><input type="radio" name="${name}" value="${value}" ${on ? "checked" : ""}><i class="fa-solid ${iconCls}"></i><span><b>${esc(label)}</b><small>${esc(note)}</small></span></label>`;
    }
    function buildModal() {
        const el = document.createElement("div");
        el.className = "modal fade sx";
        el.id = "xpModal";
        el.tabIndex = -1;
        el.setAttribute("aria-labelledby", "xpTitle");
        el.setAttribute("aria-hidden", "true");
        el.innerHTML = `
        <div class="modal-dialog modal-dialog-centered modal-dialog-scrollable modal-xl">
            <div class="modal-content sp-dialog ex-dlg xp-dlg">
                <div class="modal-header">
                    <h5 class="modal-title" id="xpTitle"><i class="fa-solid fa-file-export me-2"></i>${esc(T.sr_x_title)}</h5>
                    <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="${esc(T.cancel)}"></button>
                </div>
                <div class="modal-body">
                    <div class="xp-grid">
                        <div>
                            <div class="ex-sec">
                                <h6>${esc(T.sr_x_style)}</h6>
                                <div class="xp-styles" id="xpStyles" role="radiogroup" aria-label="${esc(T.sr_x_style)}">
                                    ${[["a", T.sr_x_humidor], ["c", T.sr_x_botanical]].map(([v, n]) => `<label data-v="${v}"><input type="radio" name="xpStyle" value="${v}"><span class="xp-badge" hidden>${esc(T.sr_x_badge)}</span><div class="xp-pg"></div><b><i class="fa-regular fa-circle"></i>${esc(n)}</b></label>`).join("")}
                                </div>
                                <label class="fm-sw form-switch xp-light" for="xpBlend" id="xpLightWrap" hidden>
                                    <span><b>${esc(T.sr_x_blend)}</b><small>${esc(T.sr_x_blend_note)}</small></span>
                                    <input type="checkbox" class="form-check-input" role="switch" id="xpBlend">
                                </label>
                            </div>
                            <div class="ex-sec">
                                <h6>${esc(T.sr_x_file_type)}</h6>
                                <div class="fm-seg" id="xpFormat" role="radiogroup" aria-label="${esc(T.sr_x_file_type)}">
                                    ${seg("xpFormat", "png", "fa-file-image", T.sr_x_png, T.sr_x_png_note, st.format === "png")}
                                    ${seg("xpFormat", "pdf", "fa-file-pdf", T.sr_x_pdf, T.sr_x_pdf_note, st.format === "pdf")}
                                </div>
                                <div class="ex-help xp-twonote" id="xpTwoNote" hidden>${esc(T.sr_x_two_pages)}</div>
                            </div>
                            <div class="ex-sec" id="xpPagesSec" hidden>
                                <h6>${esc(T.sr_x_pages)}</h6>
                                <div class="fm-seg" id="xpPages" role="radiogroup" aria-label="${esc(T.sr_x_pages)}">
                                    ${seg("xpPages", "tall", "fa-image", T.sr_x_tall, T.sr_x_tall_note, st.pages === "tall")}
                                    ${seg("xpPages", "two", "fa-images", T.sr_x_two, T.sr_x_two_note, st.pages === "two")}
                                </div>
                            </div>
                        </div>
                        <div class="xp-big" id="xpBig"><div class="xp-loading"><span class="spinner-border spinner-border-sm" role="status"></span></div></div>
                    </div>
                </div>
                <div class="modal-footer">
                    <button type="button" class="btn btn-light" data-bs-dismiss="modal">${esc(T.cancel)}</button>
                    <button type="button" class="btn btn-success" id="xpDownload"><i class="fa-solid fa-download me-1"></i><span>${esc(T.sr_x_download)}</span></button>
                </div>
            </div>
        </div>
        <div class="xp-measure" aria-hidden="true"></div>`;
        document.body.appendChild(el);

        el.querySelector("#xpStyles").addEventListener("change", e => { st.look = e.target.value; remember(); render(); });
        el.querySelector("#xpBlend").addEventListener("change", e => { st.blend = e.target.checked; remember(); render(); });
        for (const [id, key] of [["xpFormat", "format"], ["xpPages", "pages"]]) {
            el.querySelector("#" + id).addEventListener("change", e => {
                st[key] = e.target.value; remember();
                el.querySelectorAll(`#${id} label`).forEach(l => l.classList.toggle("on", l.dataset.v === st[key]));
                render();
            });
        }
        el.querySelector("#xpDownload").addEventListener("click", download);
        let rt;
        addEventListener("resize", () => { if (el.classList.contains("show")) { clearTimeout(rt); rt = setTimeout(render, 120); } });
        el.addEventListener("shown.bs.modal", render);
        return el;
    }
    const mode = () => st.format === "pdf" ? "pdf" : st.pages;
    const contLabels = () => mode() === "tall" ? [T.sr_x_cont_below, T.sr_x_cont] : [T.sr_x_cont_next, T.sr_x_cont_prev];
    function setLabels(root, labels) {
        root.querySelectorAll(".cont-t").forEach(e => e.textContent = labels[0]);
        root.querySelectorAll(".cont-f").forEach(e => e.textContent = labels[1]);
    }
    function drawPage(host, p, h) {
        const k = host.clientWidth / W;
        host.style.height = Math.round(h * k) + "px";
        const root = shadow(host);
        root.innerHTML = `<article class="${p.cls}" lang="${esc(lang)}">${p.html}</article>`;
        const a = root.firstChild;
        a.style.position = "absolute"; a.style.left = "0"; a.style.top = "0";
        a.style.height = h + "px"; a.style.transformOrigin = "0 0"; a.style.transform = `scale(${k})`;
        setLabels(a, contLabels());
    }
    function render() {
        if (!modal || !sheetCSS) return;
        const light = lightPhoto && st.blend, pdf = st.format === "pdf";
        modal.querySelector("#xpLightWrap").hidden = !lightPhoto;
        modal.querySelector("#xpBlend").checked = st.blend;
        modal.querySelectorAll("#xpStyles label").forEach(l => {
            const on = l.dataset.v === st.look;
            l.classList.toggle("on", on);
            l.querySelector("input").checked = on;
            l.querySelector("b i").className = on ? "fa-solid fa-circle-check" : "fa-regular fa-circle";
            const pages = compose(l.dataset.v, light, pdf);
            l.querySelector(".xp-badge").hidden = pages.length < 2;
            drawPage(l.querySelector(".xp-pg"), pages[0], H);
        });
        const pages = compose(st.look, light, pdf), two = pages.length > 1;
        modal.querySelector("#xpTwoNote").hidden = !two;
        modal.querySelector("#xpPagesSec").hidden = !(two && !pdf);
        const big = modal.querySelector("#xpBig");
        big.innerHTML = "";
        pages.forEach((p, i) => {
            if (two && mode() !== "tall") {
                const c = document.createElement("div");
                c.className = "xp-cap";
                c.textContent = fill(mode() === "pdf" ? T.sr_x_page_n : T.sr_x_image_n, { n: i + 1 });
                big.appendChild(c);
            }
            const pg = document.createElement("div");
            pg.className = "xp-pg" + (two && mode() === "tall" ? (i ? " joined" : " top") : "");
            big.appendChild(pg);
            drawPage(pg, p, i && pdf ? H : p.h);
        });
    }

    // A light photo background (white backdrop): the outer tenth is mostly light and
    // colorless. Only then the window offers to blend it into the page.
    function isLightPhoto(img) {
        const n = 96, c = document.createElement("canvas");
        c.width = n; c.height = n;
        const x = c.getContext("2d", { willReadFrequently: true });
        x.drawImage(img, 0, 0, n, n);
        const d = x.getImageData(0, 0, n, n).data, edge = Math.round(n / 10);
        let light = 0, all = 0;
        for (let y = 0; y < n; y++) for (let xx = 0; xx < n; xx++) {
            if (xx >= edge && xx < n - edge && y >= edge && y < n - edge) continue;
            const i = (y * n + xx) * 4, r = d[i], g = d[i + 1], b = d[i + 2];
            all++;
            if (0.2126 * r + 0.7152 * g + 0.0722 * b >= 190 && Math.max(r, g, b) - Math.min(r, g, b) <= 40) light++;
        }
        return all > 0 && light / all >= 0.6;
    }
    function loadImage(src) {
        return new Promise((ok, fail) => { const i = new Image(); i.onload = () => ok(i); i.onerror = fail; i.src = src; });
    }

    async function open() {
        if (!modal) modal = buildModal();
        bootstrap.Modal.getOrCreateInstance(modal).show();
        try {
            await Promise.all([loadFonts(), loadSheet()]);
            if (D.photo && !open.checked) {
                open.checked = true;
                try { lightPhoto = isLightPhoto(await loadImage(D.photo)); } catch (e) { lightPhoto = false; }
            }
            measureRoot = shadow(modal.querySelector(".xp-measure"));
            if (modal.classList.contains("show")) render();
        } catch (e) {
            console.error(e);
            modal.querySelector("#xpBig").innerHTML = `<div class="xp-loading">${esc(T.sr_x_failed)}</div>`;
        }
    }
    button.addEventListener("click", open);

    // ---- Files: the page goes into an SVG (foreignObject) and onto a canvas ----
    const dataURLs = {};
    function blobToDataURL(b) { return new Promise((ok, fail) => { const r = new FileReader(); r.onload = () => ok(r.result); r.onerror = fail; r.readAsDataURL(b); }); }
    // Photos as JPEG data URLs, no larger than the file needs.
    async function imageData(src, max) {
        const key = src + "|" + max;
        if (dataURLs[key]) return dataURLs[key];
        const img = await loadImage(src);
        const k = Math.min(1, max / Math.max(img.naturalWidth, img.naturalHeight));
        const c = document.createElement("canvas");
        c.width = Math.max(1, Math.round(img.naturalWidth * k)); c.height = Math.max(1, Math.round(img.naturalHeight * k));
        c.getContext("2d").drawImage(img, 0, 0, c.width, c.height);
        return dataURLs[key] = c.toDataURL("image/jpeg", 0.92);
    }
    async function fontFaces(look) {
        const out = [];
        for (const f of FONTS.filter(x => x.look === look)) {
            const key = "font|" + f.file;
            if (!dataURLs[key]) {
                const r = await fetch(FONT_DIR + f.file);
                if (!r.ok) throw new Error(f.file + " " + r.status);
                dataURLs[key] = await blobToDataURL(new Blob([await r.arrayBuffer()], { type: "font/" + (f.type === "opentype" ? "otf" : "ttf") }));
            }
            out.push(`@font-face { font-family: "${f.family}"; src: url(${dataURLs[key]}) format("${f.type}"); font-weight: ${f.weight}; font-style: ${f.style}; }`);
        }
        return out.join("\n");
    }
    async function toCanvas(p, h, look, scale) {
        const XHTML = "http://www.w3.org/1999/xhtml";
        const wrap = document.createElementNS(XHTML, "div");
        wrap.setAttribute("style", `width:${W}px;height:${h}px;`);
        const style = document.createElementNS(XHTML, "style");
        style.textContent = (await fontFaces(look)) + "\n" + sheetText;
        const art = document.createElementNS(XHTML, "article");
        art.setAttribute("class", p.cls);
        art.setAttribute("lang", lang);
        art.setAttribute("style", `height:${h}px;`);
        art.innerHTML = p.html;
        setLabels(art, contLabels());
        for (const img of art.querySelectorAll("img")) {
            const big = img.closest(".photo, .hero");
            img.setAttribute("src", await imageData(img.getAttribute("src"), big ? 1600 * scale / 2 : 480));
        }
        wrap.append(style, art);
        const xml = new XMLSerializer().serializeToString(wrap);
        const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${W * scale}" height="${h * scale}" viewBox="0 0 ${W} ${h}"><foreignObject x="0" y="0" width="${W}" height="${h}">${xml}</foreignObject></svg>`;
        const img = await loadImage("data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg));
        await img.decode().catch(() => null);
        const c = document.createElement("canvas");
        c.width = W * scale; c.height = Math.round(h * scale);
        const ctx = c.getContext("2d");
        ctx.drawImage(img, 0, 0, c.width, c.height);
        return c;
    }
    const toBlob = (c, type, q) => new Promise((ok, fail) => c.toBlob(b => b ? ok(b) : fail(new Error("toBlob")), type, q));
    function save(blob, name) {
        const a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = name;
        document.body.appendChild(a);
        a.click();
        a.remove();
        setTimeout(() => URL.revokeObjectURL(a.href), 60000);
    }
    // A PDF of JPEG pages, each filling a US Letter page (612 x 792 pt).
    async function pdfOf(canvases) {
        const enc = new TextEncoder(), parts = [], offsets = [];
        let size = 0;
        const put = x => { const b = typeof x === "string" ? enc.encode(x) : x; parts.push(b); size += b.length; };
        const obj = (n, body) => { offsets[n] = size; put(`${n} 0 obj\n`); body(); put("\nendobj\n"); };
        put("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n");
        const n = canvases.length, kids = [];
        for (let i = 0; i < n; i++) kids.push(`${3 + i * 3} 0 R`);
        obj(1, () => put("<< /Type /Catalog /Pages 2 0 R >>"));
        obj(2, () => put(`<< /Type /Pages /Count ${n} /Kids [${kids.join(" ")}] >>`));
        for (let i = 0; i < n; i++) {
            const c = canvases[i], jpg = new Uint8Array(await (await toBlob(c, "image/jpeg", 0.9)).arrayBuffer());
            const pg = 3 + i * 3, content = `q 612 0 0 792 0 0 cm /Im0 Do Q`;
            obj(pg, () => put(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Im0 ${pg + 2} 0 R >> >> /Contents ${pg + 1} 0 R >>`));
            obj(pg + 1, () => { put(`<< /Length ${content.length} >>\nstream\n${content}\nendstream`); });
            obj(pg + 2, () => {
                put(`<< /Type /XObject /Subtype /Image /Width ${c.width} /Height ${c.height} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${jpg.length} >>\nstream\n`);
                put(jpg);
                put("\nendstream");
            });
        }
        const count = 3 + n * 3, xref = size;
        let x = `xref\n0 ${count}\n0000000000 65535 f \n`;
        for (let i = 1; i < count; i++) x += String(offsets[i]).padStart(10, "0") + " 00000 n \n";
        put(x + `trailer\n<< /Size ${count} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`);
        return new Blob(parts, { type: "application/pdf" });
    }
    async function download() {
        if (busy || !sheetCSS) return;
        busy = true;
        const btn = modal.querySelector("#xpDownload"), label = btn.querySelector("span");
        btn.disabled = true; label.textContent = T.sr_x_working;
        try {
            const light = lightPhoto && st.blend, pdf = st.format === "pdf";
            const pages = compose(st.look, light, pdf);
            if (pdf) {
                const canvases = [];
                for (const p of pages) canvases.push(await toCanvas(p, H, st.look, 3)); // 300 dpi
                save(await pdfOf(canvases), D.file + ".pdf");
            } else {
                const canvases = [];
                for (const p of pages) canvases.push(await toCanvas(p, p.h, st.look, 2));
                if (canvases.length > 1 && st.pages === "tall") {
                    const c = document.createElement("canvas");
                    c.width = canvases[0].width; c.height = canvases.reduce((s, x) => s + x.height, 0);
                    let y = 0;
                    for (const x of canvases) { c.getContext("2d").drawImage(x, 0, y); y += x.height; }
                    save(await toBlob(c, "image/png"), D.file + ".png");
                } else if (canvases.length > 1) {
                    for (let i = 0; i < canvases.length; i++) {
                        save(await toBlob(canvases[i], "image/png"), `${D.file} (${i + 1}).png`);
                        await new Promise(r => setTimeout(r, 400));
                    }
                } else save(await toBlob(canvases[0], "image/png"), D.file + ".png");
            }
        } catch (e) {
            console.error(e);
            if (window.uiMessages) uiMessages.showToast(T.sr_x_failed, "danger");
        } finally {
            busy = false; btn.disabled = false; label.textContent = T.sr_x_download;
        }
    }
})();
