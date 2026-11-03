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
    const feature = f => `<li><span class="wn-ic"><i class="fa-solid ${esc(f.icon)}" aria-hidden="true"></i></span><div><b>${esc(f.title)}${f.beta ? ` <em class="st-beta">${esc(t("al_beta"))}</em>` : ""}</b>
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
