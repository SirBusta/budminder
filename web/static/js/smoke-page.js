// Smoke Reports page (mockup data/mockups/smoke, screens 1-2): every report as
// cards or a sortable list, with filters. Data from GET /smoke/all.
(function () {
    "use strict";
    const root = document.getElementById("smokePage");
    if (!root) return;
    const D = JSON.parse(root.dataset.page || "{}");
    const LANG = D.lang || undefined;
    const $ = id => document.getElementById(id);
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    function tr(key, vars) {
        let s = (D.t && D.t[key]) || key;
        if (vars) for (const k of Object.keys(vars)) s = s.split("{" + k + "}").join(vars[k]);
        return s;
    }
    const store = { get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } }, set(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private mode */ } } };

    const word = t => t.custom ? t.name : tr("sr_w_" + t.name);
    const fmtDate = d => d ? new Date(d + "T12:00:00").toLocaleDateString(LANG, { month: "short", day: "numeric", year: "numeric" }) : "";
    const cls = v => v == null ? "low" : v >= 8 ? "" : v >= 6 ? "mid" : "low";
    const scoreHTML = v => v == null ? `<span class="sd-muted">–</span>` : `<span class="sr-score ${cls(v)}">${v.toFixed(1)}</span>`;
    const money = v => tr("sr_currency") + v.toLocaleString(LANG, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    const ppg = r => r.price_per_gram != null ? money(r.price_per_gram) + "/g" : "";
    const srcBadge = r => r.source === "bought"
        ? `<span class="sr-src bought"><i class="fa-solid fa-store"></i>${esc(tr("sr_bought"))}</span>`
        : `<span class="sr-src grown"><i class="fa-solid fa-seedling"></i>${esc(tr("sr_grown"))}</span>`;
    function againBadge(r) {
        if (!r.again) return "";
        const bought = r.source === "bought";
        const [c, i, k] = { yes: ["yes", "fa-check", bought ? "sr_buy_again_yes" : "sr_again_yes"], maybe: ["maybe", "fa-question", "sr_again_maybe"],
            no: ["no", "fa-xmark", bought ? "sr_buy_again_no" : "sr_again_no"] }[r.again];
        return `<span class="sr-again ${c}"><i class="fa-solid ${i}"></i>${esc(tr(k))}</span>`;
    }
    const photo = r => r.photos.length ? `<img src="${esc(r.photos[0].url)}" alt="" loading="lazy">`
        : `<div class="d-flex h-100 align-items-center justify-content-center text-body-tertiary"><i class="fa-solid fa-cannabis fa-3x"></i></div>`;
    const from = r => r.source === "bought" ? [r.dispensary, ppg(r)].filter(Boolean).join(" · ") : [r.plant_name, r.breeder_name].filter(Boolean).join(" · ");
    const n = v => v == null ? "–" : v;

    let REPORTS = [], WEIGHTS = { bud: 1, taste: 1, smell: 1, effects: 1 };
    const S = { src: "all", q: "", strain: "", again: "", sort: store.get("sr-sort") || "overall", desc: true, layout: store.get("sr-layout") === "list" ? "list" : "cards" };

    // Sort keys: value getter and whether "first click" is high-first.
    const SORTS = {
        overall: [r => r.overall, true], newest: [r => r.smoked_on, true], strain: [r => r.strain_name.toLowerCase(), false],
        ppg: [r => r.price_per_gram, false], bud: [r => r.score_bud, true], taste: [r => r.score_taste, true],
        smell: [r => r.score_smell, true], effects: [r => r.score_effects, true], from: [r => from(r).toLowerCase(), false], again: [r => ({ yes: 0, maybe: 1, no: 2 })[r.again] ?? 3, false],
    };
    function apply() {
        const q = S.q.trim().toLowerCase();
        let list = REPORTS.filter(r => (S.src === "all" || r.source === S.src) && (!S.strain || r.strain_name === S.strain) && (!S.again || r.again === S.again) &&
            (!q || [r.strain_name, r.breeder_name, r.plant_name, r.dispensary, r.notes, r.batch].join(" ").toLowerCase().includes(q)));
        const [get] = SORTS[S.sort] || SORTS.overall;
        list = list.slice().sort((a, b) => {
            const x = get(a), y = get(b);
            if (x == null && y == null) return 0;
            if (x == null) return 1; // blanks last either way
            if (y == null) return -1;
            const c = x < y ? -1 : x > y ? 1 : 0;
            return S.desc ? -c : c;
        });
        return list;
    }

    function renderBar() {
        const strains = [...new Set(REPORTS.map(r => r.strain_name))].sort((a, b) => a.localeCompare(b));
        const seg = (k, l) => `<button type="button" data-src="${k}" class="${S.src === k ? "on" : ""}">${esc(l)}</button>`;
        $("srBar").innerHTML = `<div class="sr-seg">${seg("all", tr("sr_all"))}${seg("grown", tr("sr_grown"))}${seg("bought", tr("sr_bought"))}</div>
            <div class="sr-search"><i class="fa-solid fa-magnifying-glass text-body-tertiary"></i><input id="srQ" type="search" value="${esc(S.q)}" placeholder="${esc(tr("sr_search"))}" aria-label="${esc(tr("sr_search"))}"></div>
            <select class="sr-sel" id="srStrain" aria-label="${esc(tr("sr_col_strain"))}"><option value="">${esc(tr("sr_all_strains"))}</option>${strains.map(s => `<option ${s === S.strain ? "selected" : ""}>${esc(s)}</option>`).join("")}</select>
            <select class="sr-sel" id="srAgain" aria-label="${esc(tr("sr_col_again"))}"><option value="">${esc(tr("sr_again_any"))}</option>${["yes", "maybe", "no"].map(k => `<option value="${k}" ${S.again === k ? "selected" : ""}>${esc(tr("sr_" + k))}</option>`).join("")}</select>
            <span class="ms-auto"></span>
            ${S.layout === "cards" ? `<select class="sr-sel" id="srSort" aria-label="${esc(tr("sr_sort"))}">${[["overall", "sr_sort_overall"], ["newest", "sr_sort_newest"], ["strain", "sr_sort_strain"], ["bud", "sr_bud"], ["taste", "sr_taste"], ["smell", "sr_smell"], ["effects", "sr_effects"], ["ppg", "sr_sort_ppg"]]
                .map(([k, l]) => `<option value="${k}" ${S.sort === k ? "selected" : ""}>${esc(tr("sr_sort"))}: ${esc(tr(l))}</option>`).join("")}</select>` : ""}
            <div class="sr-seg"><button type="button" data-layout="cards" class="${S.layout === "cards" ? "on" : ""}"><i class="fa-solid fa-grip me-1"></i>${esc(tr("sr_cards"))}</button><button type="button" data-layout="list" class="${S.layout === "list" ? "on" : ""}"><i class="fa-solid fa-list me-1"></i>${esc(tr("sr_list"))}</button></div>`;
    }

    function weightsNote() {
        const names = { bud: "sr_bud", taste: "sr_taste", smell: "sr_smell", effects: "sr_effects" };
        const odd = Object.keys(names).filter(k => WEIGHTS[k] !== 1);
        if (!odd.length) return "";
        const note = odd.map(k => tr("sr_weights_note", { what: tr(names[k]), n: WEIGHTS[k] })).join(" · ");
        return ` · ${esc(note)}${D.logged_in ? ` (<a href="/settings#grow">${esc(tr("sr_change"))}</a>)` : ""}`;
    }

    function card(r) {
        const eff = r.tags.filter(t => t.kind === "effect").slice(0, 3);
        return `<a class="sr-card" href="/smoke/${r.id}">
            <div class="ph">${photo(r)}${srcBadge(r)}${r.overall != null ? `<span class="sr-score ${cls(r.overall)}">${r.overall.toFixed(1)}</span>` : ""}</div>
            <div class="bd"><h3>${esc(r.strain_name)}</h3><div class="by">${esc(from(r)) || "&nbsp;"}</div>
              <div class="mini"><div><b>${n(r.score_bud)}</b>${esc(tr("sr_bud"))}</div><div><b>${n(r.score_taste)}</b>${esc(tr("sr_taste"))}</div><div><b>${n(r.score_smell)}</b>${esc(tr("sr_smell"))}</div><div><b>${n(r.score_effects)}</b>${esc(tr("sr_effects"))}</div></div>
              ${eff.length ? `<div class="sr-chips mb-2">${eff.map(t => `<span class="sr-chip eff">${esc(word(t))}</span>`).join("")}</div>` : ""}
              <div class="ft"><span>${r.smoked_on ? esc(tr("sr_smoked_on", { date: fmtDate(r.smoked_on) })) : ""}${r.cure_days != null ? " · " + esc(tr("sr_cured_n", { n: r.cure_days })) : ""}</span>${againBadge(r)}</div></div></a>`;
    }

    function table(list) {
        const th = (k, l, cl = "") => `<th class="${S.sort === k ? "on" : ""} ${cl}" data-sort="${k}" scope="col">${esc(l)} <i class="fa-solid fa-sort${S.sort === k ? (S.desc ? "-down" : "-up") : ""} small"></i></th>`;
        return `<div class="table-responsive"><table class="sr-tbl"><thead><tr><th></th>${th("strain", tr("sr_col_strain"))}${th("from", tr("sr_col_from"))}${th("newest", tr("sr_col_smoked"))}${th("overall", tr("sr_col_overall"))}
            ${th("bud", tr("sr_bud"))}${th("taste", tr("sr_taste"))}${th("smell", tr("sr_smell"))}${th("effects", tr("sr_effects"))}${th("ppg", tr("sr_col_ppg"))}${th("again", tr("sr_col_again"))}</tr></thead><tbody>
            ${list.map(r => `<tr><td>${r.photos.length ? `<img class="thumb" src="${esc(r.photos[0].url)}" alt="" loading="lazy">` : `<div class="thumb d-flex align-items-center justify-content-center bg-body-tertiary"><i class="fa-solid fa-cannabis text-body-tertiary"></i></div>`}</td>
              <td><a href="/smoke/${r.id}">${esc(r.strain_name)}</a><div class="small sd-muted">${esc(r.breeder_name)}</div></td>
              <td>${srcBadge(r)}<div class="small sd-muted mt-1">${esc(r.source === "bought" ? r.dispensary : r.plant_name)}</div></td>
              <td class="text-nowrap">${esc(fmtDate(r.smoked_on))}</td><td class="n">${scoreHTML(r.overall)}</td>
              <td class="n">${n(r.score_bud)}</td><td class="n">${n(r.score_taste)}</td><td class="n">${n(r.score_smell)}</td><td class="n">${n(r.score_effects)}</td>
              <td class="n">${r.price_per_gram != null ? esc(money(r.price_per_gram)) : "–"}</td><td>${againBadge(r)}</td></tr>`).join("")}</tbody></table></div>`;
    }

    function render() {
        const list = apply();
        const scored = list.filter(r => r.overall != null);
        const avg = scored.length ? (scored.reduce((a, r) => a + r.overall, 0) / scored.length).toFixed(1) : "–";
        $("srSum").innerHTML = REPORTS.length ? esc(tr("sr_summary", { n: list.length, s: new Set(list.map(r => r.strain_name.toLowerCase())).size, avg })) + weightsNote() : "";
        if (!REPORTS.length) {
            $("srOut").innerHTML = `<div class="sd-card text-center py-5"><i class="fa-solid fa-joint fa-2x text-body-tertiary mb-2"></i><h2 class="h5 fw-bold">${esc(tr("sr_empty_title"))}</h2><p class="sd-muted">${esc(tr("sr_empty"))}</p>
                ${D.logged_in ? `<a class="sd-btn" href="/smoke/new"><i class="fa-solid fa-plus"></i>${esc(tr("sr_new"))}</a>` : ""}</div>`;
            return;
        }
        if (!list.length) {
            $("srOut").innerHTML = `<p class="sd-muted py-4 text-center">${esc(tr("sr_no_match_filters"))}</p>`;
            return;
        }
        $("srOut").innerHTML = S.layout === "list" ? table(list) : `<div class="sr-grid">${list.map(card).join("")}</div>`;
    }

    root.addEventListener("click", e => {
        const src = e.target.closest("[data-src]");
        if (src) { S.src = src.dataset.src; renderBar(); render(); return; }
        const lay = e.target.closest("[data-layout]");
        if (lay) { S.layout = lay.dataset.layout; store.set("sr-layout", S.layout); renderBar(); render(); return; }
        const th = e.target.closest("th[data-sort]");
        if (th) {
            const k = th.dataset.sort;
            if (S.sort === k) S.desc = !S.desc; else { S.sort = k; S.desc = SORTS[k][1]; }
            store.set("sr-sort", S.sort);
            render();
        }
    });
    root.addEventListener("input", e => { if (e.target.id === "srQ") { S.q = e.target.value; render(); } });
    root.addEventListener("change", e => {
        if (e.target.id === "srStrain") S.strain = e.target.value;
        else if (e.target.id === "srAgain") S.again = e.target.value;
        else if (e.target.id === "srSort") { S.sort = e.target.value; S.desc = SORTS[S.sort][1]; store.set("sr-sort", S.sort); }
        else return;
        render();
    });

    S.desc = (SORTS[S.sort] || SORTS.overall)[1];
    fetch("/smoke/all", { cache: "no-store", headers: { Accept: "application/json" } })
        .then(r => r.ok ? r.json() : { reports: [] })
        .then(d => { REPORTS = d.reports || []; WEIGHTS = d.weights || WEIGHTS; renderBar(); render(); })
        .catch(() => { renderBar(); render(); });
})();
