// What's new after an upgrade (handlers/whats_new.go, common/header2.html): one
// card per release this install hasn't seen, newest first, on the tour's card.
// Closing it, from any card, or following one of its links counts as seen.
(() => {
    "use strict";
    const data = document.getElementById("bmWhatsNew");
    if (!data) return;
    const CFG = JSON.parse(data.textContent || "{}");
    const D = CFG.data || {};
    const CARDS = D.cards || [];
    if (!CARDS.length) return;
    const t = (key, vars) => {
        let s = (D.t || {})[key] || key;
        if (vars) for (const k of Object.keys(vars)) s = s.split("{" + k + "}").join(vars[k]);
        return s;
    };
    const esc = s => String(s).replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));

    let i = 0;
    const lastFocus = document.activeElement;
    const root = document.createElement("div");
    root.className = "bm-tour";
    root.innerHTML = `<div class="bt-veil"></div><div class="bt-wrap"><section class="bt-card wn-card" role="dialog" aria-modal="true" aria-labelledby="wnTitle"></section></div>`;
    const card = root.querySelector(".bt-card");

    function dateText(iso) {
        if (!iso) return "";
        const d = new Date(iso + "T12:00:00");
        return isNaN(d) ? "" : d.toLocaleDateString(CFG.lang || undefined, { year: "numeric", month: "long", day: "numeric" });
    }
    // ---------- Pictures (CSS .wn-fig in isley.css; mockup data/mockups/whatsnew/v3.html) ----------
    // Readings in the settings' units, numbers in the page language.
    const num = (v, dp) => Number(v).toLocaleString(CFG.lang || undefined, { minimumFractionDigits: dp, maximumFractionDigits: dp });
    const temp = (f, dp) => `${num(D.metric ? (f - 32) * 5 / 9 : f, dp)} ${D.metric ? "°C" : "°F"}`;
    // 2.3: an alert in Recent alerts opening its chart on the Sensors page.
    function figAlertChart() {
        const [pre, post = ""] = t("wn_2_3_fig_alert").split("{value}");
        const line = `${t("al_row_highest", { value: temp(89, 0) })} ${t("al_row_limit_high", { limits: temp(86, 0) })}`;
        const big = temp(87.2, 1).split(" ");
        return `<svg class="wn-fig play" viewBox="0 0 600 262" role="img" aria-label="${esc(t("wn_2_3_fig_alt"))}">
  <defs><pattern id="wiDots" width="14" height="14" patternUnits="userSpaceOnUse"><circle class="dot" cx="2" cy="2" r="1.1"/></pattern>
    <clipPath id="wiPlot"><rect x="306" y="156" width="266" height="80"/></clipPath></defs>
  <rect class="bg" width="600" height="262" rx="12"/><rect width="600" height="262" rx="12" fill="url(#wiDots)"/>
  <g class="p1">
    <rect class="panel" x="16" y="18" width="300" height="172" rx="10"/>
    <text class="grn fa" x="30" y="44" font-size="13">&#xf1da;</text>
    <text class="fg" x="49" y="44" font-size="13.5" font-weight="700">${esc(t("al_recent_title"))}</text>
    <g class="a-hov"><rect class="hover" x="24" y="58" width="284" height="40" rx="6"/><rect class="accent" x="24" y="58" width="3" height="40" rx="1.5"/></g>
    <text class="fg" x="36" y="75" font-size="11" font-weight="700">${esc(pre)}<tspan class="red">${esc(temp(87, 0))}</tspan>${esc(post)}</text>
    <text class="mu" x="36" y="89" font-size="9.5">${esc(line)}</text>
    <rect class="pill-red" data-fit="r" x="250" y="66" width="38" height="14" rx="3"/><text class="pill-t" x="269" y="76" text-anchor="middle">${esc(t("al_state_active"))}</text>
    <line class="sep" x1="24" y1="102" x2="308" y2="102"/>
    <rect class="bar" x="36" y="113" width="104" height="7" rx="3.5"/><rect class="pill-red" x="144" y="113" width="22" height="7" rx="3.5" opacity=".75"/>
    <rect class="bar2" x="36" y="126" width="66" height="6" rx="3"/><rect class="pill-end" x="250" y="112" width="38" height="14" rx="3" opacity=".55"/>
    <line class="sep" x1="24" y1="142" x2="308" y2="142"/>
    <rect class="bar" x="36" y="153" width="118" height="7" rx="3.5"/><rect class="pill-red" x="158" y="153" width="22" height="7" rx="3.5" opacity=".75"/>
    <rect class="bar2" x="36" y="166" width="92" height="6" rx="3"/><rect class="pill-end" x="250" y="152" width="38" height="14" rx="3" opacity=".55"/>
  </g>
  <g class="p1">
    <rect class="panel" x="290" y="96" width="294" height="152" rx="10"/>
    <circle class="red" cx="307" cy="114" r="4"/>
    <text class="mu" x="317" y="118" font-size="9.5" font-weight="700" letter-spacing=".6">${esc(t("al_measure_temp").toLocaleUpperCase(CFG.lang || undefined))}</text>
    <text class="fg" x="303" y="142" font-size="17" font-weight="800">${esc(big[0])} <tspan class="mu" font-size="10" font-weight="600">${esc(big[1])}</tspan></text>
    <line class="grid" x1="395" y1="156" x2="395" y2="236"/><line class="grid" x1="484" y1="156" x2="484" y2="236"/>
    <g clip-path="url(#wiPlot)">
      <path class="area" d="M306 228 C330 228 352 226 372 220 S410 200 428 190 S458 180 476 177 S510 168 532 164 S562 165 572 164 L572 236 L306 236 Z"/>
      <rect class="shade a-shade" x="476" y="156" width="96" height="80"/>
    </g>
    <line class="limit" x1="306" y1="182" x2="572" y2="182"/>
    <text class="fg" x="310" y="177" font-size="8.5" font-weight="600">${esc(t("en_al_limit_high", { limit: temp(86, 0) }))}</text>
    <path class="line" d="M306 228 C330 228 352 226 372 220 S410 200 428 190 S458 180 476 177 S510 168 532 164 S562 165 572 164"/>
    <g class="a-mark">
      <line class="start" x1="476" y1="156" x2="476" y2="236"/>
      <circle class="dotr" cx="476" cy="177" r="3.6"/>
      <rect class="tag" data-fit="r" x="426" y="157" width="42" height="16" rx="4"/><text class="fg" x="447" y="168.5" font-size="9" font-weight="700" text-anchor="middle">${esc(temp(87, 0))}</text>
      <rect class="tag2" data-fit="l" x="482" y="212" width="78" height="18" rx="4"/><text class="fg" x="521" y="224.5" font-size="8.5" font-weight="700" text-anchor="middle">${esc(t("en_al_started"))}</text>
    </g>
  </g>
  <path class="arrow a-arrow" d="M296 70 C360 26 470 34 516 146"/>
  <g class="a-head"><path class="head" d="M0 0 L-12 -6.5 L-12 6.5 Z" transform="translate(518 151) rotate(67)"/></g>
  <g class="a-cur"><circle class="halo" cx="214" cy="80" r="11"/><path class="cur" d="M214 80 l0 17 l4.6-4.1 l3.2 7.2 l2.7-1.2 l-3.1-7 l6.2-0.2 Z"/></g>
</svg>`;
    }
    const FIGURES = { alert_chart: figAlertChart };
    // Boxes marked data-fit grow to their words (other languages run longer):
    // "r" keeps the right edge (the box ends at the green line or the panel), "l" the left.
    function fitBoxes(svg) {
        svg.querySelectorAll("rect[data-fit]").forEach(r => {
            const tx = r.nextElementSibling;
            if (!tx || !tx.getComputedTextLength) return;
            const w = tx.getComputedTextLength() + 12, old = +r.getAttribute("width");
            if (w <= old) return;
            const left = r.dataset.fit === "r" ? +r.getAttribute("x") + old - w : +r.getAttribute("x");
            r.setAttribute("x", left);
            r.setAttribute("width", w);
            tx.setAttribute("x", left + w / 2); // its words are centered
        });
    }

    const feature = f => `<li><span class="wn-ic"><i class="fa-solid ${esc(f.icon)}" aria-hidden="true"></i></span><div><b>${esc(f.title)}${f.beta ? ` <em class="st-beta">${esc(t("al_beta"))}</em>` : ""}</b>
        ${FIGURES[f.figure] ? FIGURES[f.figure]() : ""}
        <p>${esc(f.text)}</p>
        ${f.points && f.points.length ? `<ul class="wn-points">${f.points.map(x => `<li>${esc(x)}</li>`).join("")}</ul>` : ""}
        ${f.after ? `<p>${esc(f.after)}</p>` : ""}
        ${f.beta ? `<p class="wn-beta">${esc(t("al_beta_note"))} ${esc(t("al_beta_feedback"))} <a href="mailto:budminder.app@gmail.com?subject=BudMinder%20alerts%20feedback">budminder.app@gmail.com</a></p>` : ""}
        ${f.link ? `<p class="wn-link"><a href="${esc(f.link)}" data-wn="link">${esc(f.link_text)} <i class="fa-solid fa-arrow-right" aria-hidden="true"></i></a></p>` : ""}</div></li>`;
    const section = (title, items) => items && items.length ? `<div class="wn-h">${esc(title)}</div><ul class="wn-list">${items.map(x => `<li>${esc(x)}</li>`).join("")}</ul>` : "";

    function render() {
        const c = CARDS[i], last = i === CARDS.length - 1, date = dateText(c.date);
        const dots = CARDS.length > 1 ? `<span class="wn-dots" aria-hidden="true">${CARDS.map((_, k) => `<i class="${k === i ? "on" : ""}"></i>`).join("")}</span>` : "";
        const back = i > 0 ? `<button type="button" class="btn btn-light" data-wn="back"><i class="fa-solid fa-arrow-left me-1" aria-hidden="true"></i>${esc(CARDS[i - 1].version)}</button>` : "";
        const next = last ? `<button type="button" class="btn btn-success" data-wn="done">${esc(t("wn_got_it"))}</button>`
            : `<button type="button" class="btn btn-success" data-wn="next">${esc(t("wn_title", { v: CARDS[i + 1].version }))}<i class="fa-solid fa-arrow-right ms-2" aria-hidden="true"></i></button>`;
        card.innerHTML = `<div class="bt-top"><span>${esc(D.from ? t("wn_from", { from: D.from, to: D.to }) : t("wn_to", { to: D.to }))}</span>
                <button type="button" class="bt-x" data-wn="done" aria-label="${esc(t("title_close"))}" title="${esc(t("title_close"))}"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button></div>
            <div class="bt-body"><div class="bt-divider" aria-hidden="true"><i class="fa-solid fa-seedling"></i></div>
                <h2 id="wnTitle">${esc(t("wn_title", { v: c.version }))}</h2>
                ${date ? `<p class="wn-date">${esc(date)}</p>` : ""}
                ${c.features && c.features.length ? `<div class="wn-h">${esc(t("wn_features"))}</div><ul class="wn-feats">${c.features.map(feature).join("")}</ul>` : ""}
                ${section(t("wn_other"), c.other)}
                ${section(t("wn_fixes"), c.fixes)}
                <p class="wn-more"><a href="${esc(c.url)}" target="_blank" rel="noopener">${esc(t("wn_see_all", { v: c.version }))}</a></p></div>
            <div class="bt-foot"><div class="wn-foot-l">${back}${dots}</div>${next}</div>`;
        card.querySelectorAll(".wn-fig").forEach(fitBoxes);
        card.querySelector(".bt-body").scrollTop = 0;
        card.querySelector(`[data-wn="${last ? "done" : "next"}"].btn`).focus({ preventScroll: true });
    }

    // keepalive: a link leaves the page right after this.
    function seen() {
        fetch("/whats-new/seen", { method: "POST", keepalive: true, headers: { "Accept": "application/json" } }).catch(() => {});
    }
    function close() {
        seen();
        root.remove();
        document.removeEventListener("keydown", onKey);
        document.body.classList.remove("bm-tour-open");
        if (lastFocus && lastFocus.focus) lastFocus.focus();
    }
    function step(by) {
        const to = i + by;
        if (to < 0 || to >= CARDS.length) return;
        i = to;
        render();
    }
    function onKey(e) {
        if (e.key === "Escape") close();
        else if (e.key === "ArrowRight") step(1);
        else if (e.key === "ArrowLeft") step(-1);
    }

    root.addEventListener("click", e => {
        const a = e.target.closest("[data-wn]");
        if (!a) return;
        const act = a.dataset.wn;
        if (act === "done" || act === "link") close(); // a link then goes on to its page
        else if (act === "next") step(1);
        else if (act === "back") step(-1);
    });
    document.addEventListener("keydown", onKey);

    document.body.appendChild(root);
    document.body.classList.add("bm-tour-open");
    render();
})();
