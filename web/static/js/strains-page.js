// Strains page (mockup S4): one record per strain, shown in three tabs —
// Seed Vault (seeds on hand), Wanted (no seeds, on the wish list) and All
// strains. Two ways to add (Find on StrainCompass → Review Strain Info, or
// Enter manually), quick card actions (Start a grow, + Seeds, Want), filters,
// Cards | List. Data from GET /strains/all; page settings and translated
// strings from #strainsPage[data-page] ("sp_" keys).
(() => {
    "use strict";
    const root = document.getElementById("strainsPage");
    if (!root) return;
    const D = JSON.parse(root.dataset.page || "{}");
    const LOGGED_IN = !!D.logged_in;
    const TODAY = D.today;
    const LANG = D.lang || undefined;
    const $ = id => document.getElementById(id);
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    function tr(key, vars) {
        let s = (D.t && D.t[key]) || key;
        if (vars) for (const k of Object.keys(vars)) s = s.split("{" + k + "}").join(vars[k]);
        return s;
    }
    // Icons: Font Awesome names, or our own "ic-" icons (CSS masks, e.g. the vault).
    const iconHTML = (name, extra = "") => `<i class="${name.startsWith("ic-") ? name : "fa-solid " + name}${extra ? " " + extra : ""}"></i>`;
    const store = { get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } }, set(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private mode */ } } };
    const toast = (msg, level) => window.uiMessages ? uiMessages.showToast(msg, level || "success") : null;

    async function api(method, url, body) {
        const opts = { method, cache: "no-store", headers: { "Accept": "application/json" } };
        if (body !== undefined) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
        const resp = await fetch(url, opts);
        const data = await resp.json().catch(() => ({}));
        if (!resp.ok) throw new Error(data.error || data.message || resp.statusText);
        return data;
    }

    // ---------- Strain states ----------
    const inVault = s => s.seed_count > 0;
    const isWanted = s => s.seed_count === 0 && !!s.wanted;
    const OLD_MONTHS = 24; // seeds this old get the orange "aging" flag
    const SEEDTYPE = { feminized: tr("sp_feminized"), regular: tr("sp_regular"), clone: tr("sp_clone") };
    function ageMonths(added) {
        const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(added || "");
        if (!m) return null;
        const now = new Date();
        let months = (now.getFullYear() - +m[1]) * 12 + (now.getMonth() + 1 - +m[2]);
        if (now.getDate() < +m[3]) months--;
        return Math.max(0, months);
    }
    function ageText(added) {
        const n = ageMonths(added);
        if (n == null) return "";
        if (n < 1) return tr("sp_age_new");
        if (n < 24) return tr("sp_age_months", { n });
        return tr("sp_age_years", { y: Math.floor(n / 12), m: n % 12 });
    }
    const isOld = s => inVault(s) && (ageMonths(s.seeds_added_on) || 0) >= OLD_MONTHS;
    const fmtDate = iso => new Date(iso + "T12:00:00").toLocaleDateString(LANG, { month: "short", day: "numeric", year: "numeric" });
    const fmtMonth = iso => iso ? new Date(iso + "T12:00:00").toLocaleDateString(LANG, { month: "short", year: "numeric" }) : "—";
    const weeks = days => String(Math.round(days / 7 * 10) / 10);
    function floweringText(s) {
        if (!(s.cycle_time > 0)) return "";
        const w = (s.cycle_time_min > 0 && s.cycle_time_min < s.cycle_time ? weeks(s.cycle_time_min) + "–" : "") + weeks(s.cycle_time);
        return tr("sp_weeks_short", { w });
    }

    // ---------- Badges / pieces ----------
    const typeBadge = s => s.autoflower ? `<span class="sp-b auto"><i class="fa-solid fa-bolt"></i>${esc(tr("sp_auto"))}</span>`
        : `<span class="sp-b photo"><i class="fa-solid fa-sun"></i>${esc(tr("sp_photo"))}</span>`;
    function flower(s) {
        const t = floweringText(s);
        return t ? `<span class="sp-b muted" title="${esc(tr(s.autoflower ? "sp_seed_to_harvest" : "sp_flowering_time"))}"><i class="fa-solid fa-clock"></i>${esc(t)}</span>` : "";
    }
    function stock(s) {
        if (inVault(s)) {
            const old = isOld(s);
            return `<span class="sp-b seeds"><i class="fa-solid fa-seedling"></i>${esc(tr(s.seed_count === 1 ? "sp_one_seed" : "sp_n_seeds", { n: s.seed_count }))}</span>` +
                (s.seed_type && SEEDTYPE[s.seed_type] ? `<span class="sp-b muted">${esc(SEEDTYPE[s.seed_type])}</span>` : "") +
                (s.seeds_added_on ? `<span class="sp-b ${old ? "old" : "muted"}" title="${esc(tr("sp_seeds_added", { date: fmtDate(s.seeds_added_on) }))}"><i class="fa-${old ? "solid fa-hourglass-half" : "regular fa-calendar"}"></i>${esc(ageText(s.seeds_added_on))}</span>` : "");
        }
        if (isWanted(s)) return `<span class="sp-b wanted"><i class="fa-solid fa-star"></i>${esc(tr("sp_wanted"))}</span>`;
        return `<span class="sp-b none">${esc(tr("sp_no_seeds"))}</span>${s.seed_type === "clone" ? `<span class="sp-b muted">${esc(SEEDTYPE.clone)}</span>` : ""}`;
    }
    const grows = s => (s.growing_count ? `<span class="sp-b grow"><i class="fa-solid fa-cannabis"></i>${esc(tr("sp_growing_now", { n: s.growing_count }))}</span>` : "") +
        (s.grown_count ? `<span class="sp-b muted"><i class="fa-solid fa-clock-rotate-left"></i>${esc(tr("sp_grown_n", { n: s.grown_count }))}</span>` : "");
    function growURL(s) {
        const q = new URLSearchParams({ strain_id: s.id, name: `${s.name} #${(s.growing_count || 0) + (s.grown_count || 0) + 1}` });
        if (inVault(s)) q.set("use_seed", "1");
        return "/plant/new?" + q.toString();
    }
    function actions(s, compact) {
        if (!LOGGED_IN) return "";
        const canGrow = inVault(s) || s.seed_type === "clone";
        const lbl = k => compact ? "" : " " + esc(tr(k));
        return `<div class="sp-acts">
            ${canGrow ? `<a class="sp-btn primary" href="${esc(growURL(s))}" title="${esc(tr("sp_start_grow"))}"><i class="fa-solid fa-play"></i>${lbl("sp_start_grow")}</a>` : ""}
            <button class="sp-btn" data-seeds="${s.id}" title="${esc(tr("sp_add_seeds"))}"><i class="fa-solid fa-plus"></i>${lbl("sp_seeds")}</button>
            ${inVault(s) ? "" : `<button class="sp-btn ${s.wanted ? "on" : ""}" data-want="${s.id}" title="${esc(tr(s.wanted ? "sp_remove_wanted" : "sp_add_wanted"))}"><i class="fa-${s.wanted ? "solid" : "regular"} fa-star"></i>${lbl(s.wanted ? "sp_wanted" : "sp_want")}</button>`}
        </div>`;
    }

    // ---------- State ----------
    let STRAINS = [];
    let loaded = false;
    const TABS = {
        vault: { icon: "ic-vault", label: "sp_tab_vault", desc: "sp_tab_vault_desc", pred: inVault },
        wanted: { icon: "fa-star", label: "sp_tab_wanted", desc: "sp_tab_wanted_desc", pred: isWanted },
        all: { icon: "fa-book", label: "sp_tab_all", desc: "sp_tab_all_desc", pred: () => true },
    };
    let tab = TABS[location.hash.slice(1)] ? location.hash.slice(1) : "vault";
    let layout = store.get("sp-layout") || "cards";
    const filter = { q: "", breeder: "", type: "", seedType: "", sort: "name" };
    // Clicking a sorted column again reverses it (where a reverse exists).
    const PAIR = { oldest: "newest", type: "typedesc" };

    function apply(list) {
        const q = filter.q.trim().toLowerCase();
        const out = list.filter(s =>
            (!q || [s.name, s.breeder, s.short_desc, s.lineage, s.seed_location].join(" ").toLowerCase().includes(q)) &&
            (!filter.breeder || s.breeder === filter.breeder) &&
            (!filter.type || (filter.type === "auto") === !!s.autoflower) &&
            (!filter.seedType || s.seed_type === filter.seedType));
        const name = (a, b) => a.name.localeCompare(b.name);
        const added = s => inVault(s) ? (s.seeds_added_on || "") : "";
        const by = {
            name,
            breeder: (a, b) => (a.breeder || "").localeCompare(b.breeder || "") || name(a, b),
            seeds: (a, b) => b.seed_count - a.seed_count || name(a, b),
            oldest: (a, b) => (added(a) || "9999").localeCompare(added(b) || "9999") || name(a, b),
            newest: (a, b) => { const x = added(a), y = added(b); if (!x || !y) return (x ? 0 : 1) - (y ? 0 : 1) || name(a, b); return y.localeCompare(x); },
            type: (a, b) => (a.autoflower - b.autoflower) || name(a, b),
            typedesc: (a, b) => (b.autoflower - a.autoflower) || name(a, b),
            flower: (a, b) => (a.autoflower - b.autoflower) || ((a.cycle_time || 9999) - (b.cycle_time || 9999)) || name(a, b),
        }[filter.sort] || name;
        return out.sort(by);
    }

    // ---------- Render ----------
    function renderFilters() {
        const breeders = [...new Set(STRAINS.map(s => s.breeder).filter(Boolean))].sort((a, b) => a.localeCompare(b));
        if (filter.breeder && !breeders.includes(filter.breeder)) filter.breeder = "";
        const opt = (v, l, cur) => `<option value="${esc(v)}" ${v === cur ? "selected" : ""}>${esc(l)}</option>`;
        $("spFilters").innerHTML = `
            <div class="sp-search"><i class="fa-solid fa-filter"></i><input type="search" id="spQ" placeholder="${esc(tr("sp_filter_my"))}" aria-label="${esc(tr("sp_filter_my"))}" value="${esc(filter.q)}"></div>
            <select class="form-select form-select-sm" data-f="breeder" aria-label="${esc(tr("sp_breeder"))}">${opt("", tr("sp_all_breeders"), filter.breeder)}${breeders.map(b => opt(b, b, filter.breeder)).join("")}</select>
            <select class="form-select form-select-sm" data-f="type" aria-label="${esc(tr("sp_type"))}">${opt("", tr("sp_photo_and_auto"), filter.type)}${opt("photo", tr("sp_photoperiod"), filter.type)}${opt("auto", tr("sp_autoflower"), filter.type)}</select>
            <select class="form-select form-select-sm" data-f="seedType" aria-label="${esc(tr("sp_seed_type"))}">${opt("", tr("sp_any_seed_type"), filter.seedType)}${Object.entries(SEEDTYPE).map(([k, l]) => opt(k, l, filter.seedType)).join("")}</select>
            <select class="form-select form-select-sm" data-f="sort" aria-label="${esc(tr("sp_sort"))}">${[["name", "sp_sort_name"], ["breeder", "sp_sort_breeder"], ["seeds", "sp_sort_seeds"], ["oldest", "sp_sort_oldest"], ["newest", "sp_sort_newest"], ["flower", "sp_sort_flower"], ["type", "sp_sort_type"], ["typedesc", "sp_sort_typedesc"]].map(([k, l]) => opt(k, tr(l), filter.sort)).join("")}</select>`;
        $("spQ").addEventListener("input", e => { filter.q = e.target.value; renderList(); });
    }

    function card(s) {
        return `<div class="sp-card">
            <div><a href="/strain/${s.id}" class="sp-nm">${esc(s.name)}</a><div class="sp-br">${esc(s.breeder || tr("sp_unknown_breeder"))}</div></div>
            <div class="sp-bs">${stock(s)}</div>
            ${s.lineage ? `<div class="sp-lin" title="${esc(tr("sp_lineage"))}: ${esc(s.lineage)}"><i class="fa-solid fa-sitemap"></i>${esc(s.lineage)}</div>` : ""}
            ${s.short_desc ? `<div class="sp-ds">${esc(s.short_desc)}</div>` : ""}
            <div class="sp-bs">${typeBadge(s)}${flower(s)}${grows(s)}</div>
            ${s.seed_location && inVault(s) ? `<div class="sp-loc"><i class="fa-solid fa-box-archive me-1"></i>${esc(s.seed_location)}</div>` : ""}
            ${LOGGED_IN ? `<div class="sp-foot">${actions(s, false)}</div>` : ""}
        </div>`;
    }

    function table(list) {
        const cols = [["name", "sp_col_strain"], [null, "sp_lineage"], ["type", "sp_type"], ["flower", "sp_col_flowering"], ["seeds", "sp_col_seeds"], ["oldest", "sp_col_seed_age"], [null, "sp_col_grows"]];
        if (LOGGED_IN) cols.push([null, ""]);
        const sort = filter.sort;
        return `<div class="sp-tbl"><table><thead><tr>${cols.map(([k, l]) => k
            ? `<th class="s ${sort === k || PAIR[k] === sort ? "on" : ""}" data-sort="${k}">${esc(tr(l))}<i class="fa-solid ${sort === k ? "fa-sort-up" : PAIR[k] === sort ? "fa-sort-down" : "fa-sort"}"></i></th>`
            : `<th>${l ? esc(tr(l)) : ""}</th>`).join("")}</tr></thead><tbody>${list.map(s => `
            <tr class="${isOld(s) ? "old" : ""}">
                <td><a href="/strain/${s.id}" class="sp-nm">${esc(s.name)}</a><div class="sp-br">${esc(s.breeder || "")}</div></td>
                <td class="sp-lin-c" title="${esc(s.lineage)}">${esc(s.lineage) || "—"}</td>
                <td>${typeBadge(s)}</td>
                <td class="text-nowrap">${esc(floweringText(s)) || "—"}</td>
                <td class="text-nowrap">${inVault(s) ? `<b>${s.seed_count}</b> <span class="text-muted small">${esc(SEEDTYPE[s.seed_type] || "")}</span>`
                    : isWanted(s) ? `<span class="sp-b wanted"><i class="fa-solid fa-star"></i>${esc(tr("sp_wanted"))}</span>` : `<span class="text-muted">0</span>`}</td>
                <td class="text-nowrap">${inVault(s) && s.seeds_added_on ? `${esc(fmtMonth(s.seeds_added_on))}<div class="small ${isOld(s) ? "sp-age-o" : "text-muted"}">${esc(ageText(s.seeds_added_on))}</div>` : "—"}</td>
                <td>${grows(s) || `<span class="text-muted">—</span>`}</td>
                ${LOGGED_IN ? `<td class="text-end">${actions(s, true)}</td>` : ""}
            </tr>`).join("")}</tbody></table></div>`;
    }

    function renderTabs() {
        const counts = { vault: STRAINS.filter(inVault).length, wanted: STRAINS.filter(isWanted).length, all: STRAINS.length };
        $("spTabs").innerHTML = Object.entries(TABS).map(([k, t]) => `
            <button class="sp-tab ${k === tab ? "on" : ""}" data-tab="${k}" role="tab" aria-selected="${k === tab}">
                <span class="ic">${iconHTML(t.icon)}</span>
                <span><b>${esc(tr(t.label))}</b><small>${esc(tr(t.desc))}</small></span><span class="n">${loaded ? counts[k] : ""}</span></button>`).join("");
    }

    function renderList() {
        const list = apply(STRAINS.filter(TABS[tab].pred));
        $("spQuickSort").innerHTML = tab === "vault" ? `${esc(tr("sp_sort_label"))}
            <span class="sp-seg"><button data-qsort="name" class="${filter.sort === "name" ? "on" : ""}">${esc(tr("sp_name"))}</button><button data-qsort="oldest" class="${filter.sort === "oldest" ? "on" : ""}"><i class="fa-solid fa-hourglass-half me-1"></i>${esc(tr("sp_oldest_first"))}</button></span>` : "";
        const vault = STRAINS.filter(inVault);
        const old = vault.filter(isOld).length;
        $("spSummary").innerHTML = !loaded ? "" : tab === "vault"
            ? `${tr("sp_summary_vault", { seeds: `<b>${vault.reduce((a, s) => a + s.seed_count, 0)}</b>`, strains: `<b>${vault.length}</b>` })}${old ? ` · <span class="warn"><i class="fa-solid fa-hourglass-half me-1"></i>${esc(tr("sp_n_old", { n: old }))}</span>` : ""}`
            : tab === "wanted" ? esc(tr("sp_summary_wanted")) : esc(tr("sp_summary_all", { n: list.length, total: STRAINS.length }));
        document.querySelectorAll("[data-layout]").forEach(b => b.classList.toggle("on", b.dataset.layout === layout));
        const out = $("spOut");
        if (!loaded) { out.innerHTML = `<div class="sp-empty"><span class="spinner-border spinner-border-sm"></span></div>`; return; }
        if (!list.length) {
            const anyFilter = filter.q || filter.breeder || filter.type || filter.seedType;
            out.innerHTML = `<div class="sp-empty">${iconHTML(TABS[tab].icon)}${esc(tr(anyFilter ? "sp_no_match" : tab === "vault" ? "sp_empty_vault" : tab === "wanted" ? "sp_empty_wanted" : "sp_empty_all"))}</div>`;
            return;
        }
        out.innerHTML = layout === "cards" ? `<div class="sp-grid">${list.map(card).join("")}</div>` : table(list);
    }

    function render() { renderTabs(); renderList(); }

    async function load() {
        try {
            STRAINS = await api("GET", "/strains/all");
            loaded = true;
        } catch (e) {
            $("spOut").innerHTML = `<div class="sp-empty text-danger">${esc(e.message)}</div>`;
            return;
        }
        renderFilters();
        render();
    }

    // ---------- Windows ----------
    function modal(id, title, body, footer, size = "") {
        let el = $(id);
        if (!el) {
            document.body.insertAdjacentHTML("beforeend", `<div class="modal fade" id="${id}" tabindex="-1" aria-hidden="true"><div class="modal-dialog modal-dialog-centered ${size}"><div class="modal-content sp-dialog">
                <div class="modal-header"><h5 class="modal-title"></h5><button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="${esc(tr("sp_close"))}"></button></div>
                <div class="modal-body"></div><div class="modal-footer"></div></div></div></div>`);
            el = $(id);
        }
        el.querySelector(".modal-title").innerHTML = title;
        el.querySelector(".modal-body").innerHTML = body;
        el.querySelector(".modal-footer").innerHTML = footer;
        bootstrap.Modal.getOrCreateInstance(el).show();
        return el;
    }
    const closeModal = el => { const m = bootstrap.Modal.getInstance(el); if (m) m.hide(); };
    const err = (el, msg) => { const box = el.querySelector(".sp-err"); if (box) { box.textContent = msg; box.classList.remove("d-none"); } };

    // "Add it to": Seed Vault (count, type, date) | Wanted | Library only.
    // Preset from the tab you're on.
    function destPicker() {
        const dest = tab === "all" ? "library" : tab;
        return `<label class="form-label small fw-semibold mb-1">${esc(tr("sp_add_it_to"))}</label>
            <div class="sp-dest" role="radiogroup">
                ${[["vault", "ic-vault", "sp_tab_vault", "sp_dest_vault"], ["wanted", "fa-star", "sp_tab_wanted", "sp_dest_wanted"], ["library", "fa-book", "sp_library_only", "sp_dest_library"]]
                    .map(([v, i, t, d]) => `<label class="${v === dest ? "on" : ""}"><input type="radio" name="spDest" value="${v}" ${v === dest ? "checked" : ""}>${iconHTML(i)}<b>${esc(tr(t))}</b><small>${esc(tr(d))}</small></label>`).join("")}
            </div>
            <div class="sp-vault-fields ${dest === "vault" ? "" : "d-none"}"><div class="row g-2 mt-1">
                <div class="col-4"><label class="form-label small mb-0">${esc(tr("sp_seeds"))}</label><input type="number" min="1" value="5" class="form-control form-control-sm" data-k="seeds"></div>
                <div class="col-4"><label class="form-label small mb-0">${esc(tr("sp_seed_type"))}</label><select class="form-select form-select-sm" data-k="seedType">${Object.entries(SEEDTYPE).map(([k, l]) => `<option value="${k}">${esc(l)}</option>`).join("")}</select></div>
                <div class="col-4"><label class="form-label small mb-0">${esc(tr("sp_added_on"))}</label><input type="date" value="${TODAY}" max="${TODAY}" class="form-control form-control-sm" data-k="added"></div>
            </div></div>`;
    }
    function wireDest(el) {
        el.querySelectorAll("input[name=spDest]").forEach(r => r.addEventListener("change", () => {
            el.querySelectorAll(".sp-dest label").forEach(l => l.classList.toggle("on", l.querySelector("input").checked));
            el.querySelector(".sp-vault-fields").classList.toggle("d-none", el.querySelector("input[name=spDest]:checked").value !== "vault");
        }));
    }
    function readDest(el) {
        const dest = el.querySelector("input[name=spDest]:checked").value;
        const v = k => el.querySelector(`[data-k=${k}]`).value;
        return { dest, seeds: Math.max(1, parseInt(v("seeds"), 10) || 1), seedType: v("seedType"), added: v("added") };
    }

    const findMine = (name, breeder) => STRAINS.find(s => s.name.trim().toLowerCase() === String(name).trim().toLowerCase() &&
        (s.breeder || "").trim().toLowerCase() === String(breeder || "").trim().toLowerCase());
    function haveText(s) {
        return inVault(s) ? tr("sp_have_vault", { n: s.seed_count }) : isWanted(s) ? tr("sp_have_wanted") : tr("sp_have_library");
    }
    const haveBox = s => `<div class="sp-have"><i class="fa-solid fa-circle-info me-1"></i>${esc(haveText(s))}
        <a href="/strain/${s.id}">${esc(tr("sp_open_it"))}</a> · <a href="#" data-seeds="${s.id}">${esc(tr("sp_add_seeds"))}</a></div>`;

    // Find on StrainCompass → Add → where it goes → Review Strain Info.
    function openFind() {
        const el = modal("spFind", `${esc(tr("sp_find_sc"))}`, `
            <form class="input-group mb-2" id="spFindForm"><span class="input-group-text"><i class="fa-solid fa-magnifying-glass"></i></span>
                <input class="form-control" id="spFindQ" placeholder="${esc(tr("sp_find_placeholder"))}" autocomplete="off">
                <button class="btn btn-success" type="submit">${esc(tr("sp_search"))}</button></form>
            <div class="small text-muted mb-2" id="spFindStatus"></div>
            <div id="spFindRes" class="sp-results"></div>`, `<button class="btn btn-light" data-bs-dismiss="modal">${esc(tr("sp_close"))}</button>`, "modal-lg");
        let results = [];
        const show = () => {
            $("spFindRes").innerHTML = results.map((r, i) => {
                const have = findMine(r.name, r.breeder);
                return `<div class="sp-res ${have ? "have" : ""}">
                    <div class="flex-grow-1"><b>${esc(r.name)}</b> <span class="text-muted">— ${esc(r.breeder || tr("sp_unknown_breeder"))}</span>
                        ${r.type ? `<span class="badge text-bg-light border">${esc(r.type)}</span>` : ""}
                        ${r.thcMax != null ? `<div class="small text-muted">THC ${esc(r.thcMax)}%</div>` : ""}
                        ${have ? haveBox(have) : ""}</div>
                    ${have ? "" : `<button class="btn btn-sm btn-success" data-pick="${i}">${esc(tr("sp_add"))}</button>`}</div>`;
            }).join("");
        };
        $("spFindForm").addEventListener("submit", async e => {
            e.preventDefault();
            const q = $("spFindQ").value.trim();
            if (q.length < 2) { $("spFindStatus").textContent = tr("sp_query_short"); return; }
            $("spFindStatus").textContent = tr("sp_searching");
            $("spFindRes").innerHTML = "";
            try {
                const data = await api("GET", "/strains/straincompass/search?q=" + encodeURIComponent(q));
                results = data.results || [];
                $("spFindStatus").innerHTML = !results.length ? `${esc(tr("sp_no_results"))} <a href="#" data-manual>${esc(tr("sp_enter_manually"))}</a>`
                    : data.total > results.length ? esc(tr("sp_showing", { shown: results.length, total: data.total })) : "";
                show();
            } catch (ex) { $("spFindStatus").textContent = ex.message; }
        });
        $("spFindRes").addEventListener("click", e => {
            const b = e.target.closest("[data-pick]");
            if (b) openDestForImport(el, results[+b.dataset.pick]);
        });
        setTimeout(() => $("spFindQ").focus(), 300);
    }
    function openDestForImport(el, r) {
        el.querySelector(".modal-title").innerHTML = esc(tr("sp_add_named", { name: r.name }));
        el.querySelector(".modal-body").innerHTML = `
            <div class="sp-pick mb-3"><b>${esc(r.name)}</b> <span class="text-muted">— ${esc(r.breeder || tr("sp_unknown_breeder"))}</span><div class="small text-muted">${esc(tr("sp_from_sc"))}</div></div>
            ${destPicker()}
            <p class="small text-muted mt-3 mb-0"><i class="fa-solid fa-circle-info me-1"></i>${esc(tr("sp_review_next"))}</p>`;
        el.querySelector(".modal-footer").innerHTML = `<button class="btn btn-light" data-bs-dismiss="modal">${esc(tr("sp_cancel"))}</button><button class="btn btn-success" id="spReviewGo">${esc(tr("sp_continue_review"))}</button>`;
        wireDest(el);
        $("spReviewGo").addEventListener("click", () => {
            const d = readDest(el);
            const q = new URLSearchParams({ source: "straincompass", slug: r.slug, name: r.name, listing_breeder: r.breeder || "", dest: d.dest });
            if (d.dest === "vault") { q.set("seed_count", d.seeds); q.set("seed_type", d.seedType); q.set("seeds_added_on", d.added); }
            location.href = "/strain/import/review?" + q.toString();
        });
    }

    // Enter manually: the essentials + where it goes; the rest on the strain page.
    let breederAc = null;
    function openManual() {
        const el = $("spManual");
        if (!el) return;
        el.querySelector("#spManualDest").innerHTML = destPicker();
        wireDest(el);
        el.querySelectorAll(".sp-err").forEach(x => x.classList.add("d-none"));
        el.querySelector("#spManualName").value = "";
        el.querySelector("#spManualFrom").value = "";
        el.querySelector("#spManualTo").value = "";
        el.querySelector("#spManualType").value = "";
        el.querySelector(".sp-dup").classList.add("d-none");
        if (!breederAc && window.IsleyAutocomplete) breederAc = IsleyAutocomplete.breederPicker($("spManualBreeder"), $("spManualNewBreeder"));
        if (breederAc) breederAc.reset();
        bootstrap.Modal.getOrCreateInstance(el).show();
    }
    function manualBreederName() {
        const sel = $("spManualBreeder");
        if (sel.value === "new") return $("spManualNewBreeder").value;
        const o = sel.options[sel.selectedIndex];
        return o && sel.value ? o.textContent.trim() : (breederAc && breederAc.input ? breederAc.input.value : "");
    }
    function manualDupCheck() {
        const el = $("spManual");
        const have = findMine($("spManualName").value, manualBreederName());
        const box = el.querySelector(".sp-dup");
        box.classList.toggle("d-none", !have || !$("spManualName").value.trim());
        if (have) box.innerHTML = `<div class="sp-have"><i class="fa-solid fa-circle-info me-1"></i>${esc(tr("sp_dup_manual", { name: have.name, breeder: have.breeder }))} <a href="#" data-seeds="${have.id}">${esc(tr("sp_add_seeds_instead"))}</a></div>`;
    }
    async function saveManual() {
        const el = $("spManual");
        el.querySelectorAll(".sp-err").forEach(x => x.classList.add("d-none"));
        if (breederAc) breederAc.commit();
        const name = $("spManualName").value.trim();
        const sel = $("spManualBreeder");
        if (!name) { $("spManualName").reportValidity(); return; }
        if (!sel.value) { if (breederAc) breederAc.input.reportValidity(); return; }
        const from = parseFloat($("spManualFrom").value) || 0, to = parseFloat($("spManualTo").value) || 0;
        const longEnd = Math.max(from, to), shortEnd = from && to ? Math.min(from, to) : 0;
        const d = readDest(el);
        const body = {
            name, indica: 50, sativa: 50, autoflower: $("spManualType").value === "auto", short_desc: "", description: "",
            breeder_id: sel.value === "new" ? null : parseInt(sel.value, 10),
            new_breeder: sel.value === "new" ? $("spManualNewBreeder").value : "",
            cycle_time: Math.round(longEnd * 7), cycle_time_min: shortEnd && shortEnd < longEnd ? Math.round(shortEnd * 7) : 0,
            seed_count: d.dest === "vault" ? d.seeds : 0, seed_type: d.dest === "vault" ? d.seedType : "",
            seeds_added_on: d.dest === "vault" ? d.added : "", wanted: d.dest === "wanted",
        };
        $("spManualSave").disabled = true;
        try {
            const res = await api("POST", "/strains", body);
            location.href = "/strain/" + res.id;
        } catch (e) { err(el, e.message); } finally { $("spManualSave").disabled = false; }
    }

    // + Seeds: how many, type, date, where; a Wanted strain moves to the vault.
    function openSeeds(id) {
        const s = STRAINS.find(x => x.id === +id);
        if (!s) return;
        const el = modal("spSeeds", `<i class="fa-solid fa-seedling me-2"></i>${esc(tr("sp_add_seeds_to", { name: s.name }))}`, `
            ${isWanted(s) ? `<div class="sp-have mb-3"><i class="fa-solid fa-star me-1"></i>${esc(tr("sp_got_them", { name: s.name }))}</div>` : ""}
            <div class="row g-2">
                <div class="col-4"><label class="form-label small mb-0" for="spSeedsN">${esc(tr("sp_how_many"))}</label><input type="number" min="1" value="5" class="form-control" id="spSeedsN"></div>
                <div class="col-4"><label class="form-label small mb-0" for="spSeedsT">${esc(tr("sp_seed_type"))}</label><select class="form-select" id="spSeedsT">${Object.entries(SEEDTYPE).map(([k, l]) => `<option value="${k}" ${k === (s.seed_type || "feminized") ? "selected" : ""}>${esc(l)}</option>`).join("")}</select></div>
                <div class="col-4"><label class="form-label small mb-0" for="spSeedsD">${esc(tr("sp_added_on"))}</label><input type="date" value="${inVault(s) && s.seeds_added_on ? s.seeds_added_on : TODAY}" max="${TODAY}" class="form-control" id="spSeedsD"></div>
                <div class="col-12"><label class="form-label small mb-0" for="spSeedsL">${esc(tr("sp_stored_in"))}</label><input class="form-control" id="spSeedsL" value="${esc(s.seed_location || "")}" placeholder="${esc(tr("sp_stored_placeholder"))}"></div>
            </div>
            ${inVault(s) && s.seeds_added_on ? `<p class="small text-muted mt-2 mb-0">${esc(tr("sp_have_already", { n: s.seed_count, date: fmtDate(s.seeds_added_on) }))}</p>` : ""}
            <div class="text-danger small mt-2 d-none sp-err"></div>`,
            `<button class="btn btn-light" data-bs-dismiss="modal">${esc(tr("sp_cancel"))}</button><button class="btn btn-success" id="spSeedsGo">${esc(tr("sp_add_seeds"))}</button>`);
        $("spSeedsGo").addEventListener("click", async () => {
            const n = parseInt($("spSeedsN").value, 10);
            if (!(n >= 1)) { $("spSeedsN").reportValidity(); return; }
            $("spSeedsGo").disabled = true;
            try {
                await api("POST", `/strains/${s.id}/seeds`, { count: n, seed_type: $("spSeedsT").value, seeds_added_on: $("spSeedsD").value, seed_location: $("spSeedsL").value });
                closeModal(el);
                toast(tr(inVault(s) ? "sp_seeds_added_toast" : "sp_moved_to_vault", { n, name: s.name }));
                await load();
            } catch (e) { err(el, e.message); } finally { $("spSeedsGo").disabled = false; }
        });
    }

    async function toggleWant(id) {
        const s = STRAINS.find(x => x.id === +id);
        if (!s) return;
        try {
            const res = await api("POST", `/strains/${s.id}/wanted`, { wanted: !s.wanted });
            s.wanted = !!res.wanted;
            toast(tr(s.wanted ? "sp_added_wanted_toast" : "sp_removed_wanted_toast", { name: s.name }));
            render();
        } catch (e) { toast(e.message, "danger"); }
    }

    // ---------- Events ----------
    document.addEventListener("click", e => {
        const t = e.target;
        const tb = t.closest("[data-tab]");
        if (tb) { tab = tb.dataset.tab; history.replaceState(null, "", "#" + tab); render(); return; }
        const q = t.closest("[data-qsort]");
        if (q) { filter.sort = q.dataset.qsort; renderFilters(); renderList(); return; }
        const th = t.closest("th[data-sort]");
        if (th) { const k = th.dataset.sort; filter.sort = PAIR[k] && filter.sort === k ? PAIR[k] : k; renderFilters(); renderList(); return; }
        const ly = t.closest("[data-layout]");
        if (ly) { layout = ly.dataset.layout; store.set("sp-layout", layout); renderList(); return; }
        const sd = t.closest("[data-seeds]");
        if (sd) {
            e.preventDefault();
            const open = document.querySelector(".modal.show");
            if (open) closeModal(open);
            setTimeout(() => openSeeds(sd.dataset.seeds), open ? 300 : 0);
            return;
        }
        const w = t.closest("[data-want]");
        if (w) { toggleWant(w.dataset.want); return; }
        const m = t.closest("[data-manual]");
        if (m) {
            e.preventDefault();
            const open = document.querySelector(".modal.show");
            if (open) closeModal(open);
            setTimeout(openManual, open ? 300 : 0);
            return;
        }
        if (t.closest("#spFindBtn")) { openFind(); return; }
        if (t.closest("#spManualBtn")) { openManual(); return; }
        if (t.closest("#spManualSave")) { saveManual(); }
    });
    document.addEventListener("change", e => {
        const f = e.target.closest("#spFilters [data-f]");
        if (f) { filter[f.dataset.f] = f.value; renderList(); return; }
        if (e.target.closest("#spManualBreeder")) manualDupCheck();
    });
    document.addEventListener("input", e => { if (e.target.id === "spManualName") manualDupCheck(); });
    window.addEventListener("hashchange", () => { if (TABS[location.hash.slice(1)] && location.hash.slice(1) !== tab) { tab = location.hash.slice(1); render(); } });
    // A Back/Forward restore from the bfcache skips the load; refresh the list.
    window.addEventListener("pageshow", e => { if (e.persisted) load(); });

    renderFilters();
    render();
    load();
})();
