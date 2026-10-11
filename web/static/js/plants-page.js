// Plants page (views/plants.html), mockup PL3: Growing / Harvested / Dead
// tabs with a summary, filters, and a row per plant with a progress bar from
// planting to the estimated harvest. "Cards" shows photo tiles. Data: the
// JSON in #plantsPage[data-page] (handlers.PlantsPageData).
(function () {
    "use strict";
    const root = document.getElementById("plantsPage");
    if (!root) return;
    const D = JSON.parse(root.dataset.page || "{}");
    const T = D.t || {};
    const t = (k, vars) => {
        let s = T[k] || k;
        if (vars) for (const [a, b] of Object.entries(vars)) s = s.split("{" + a + "}").join(b);
        return s;
    };
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const store = {
        get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
        set(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private window */ } },
    };
    const LOGGED_IN = !!D.logged_in;
    const TODAY = D.today;

    // ---------- Stages ----------
    const STAGE_LOOK = {
        Germinating: ["fa-seedling", "#a3b18a"], Planted: ["fa-bucket", "#8fbc5a"], Seedling: ["fa-leaf", "#84cc16"],
        Veg: ["fa-tree", "#16a34a"], Flower: ["fa-cannabis", "#9333ea"], Drying: ["fa-wind", "#d97706"],
        Curing: ["fa-jar", "#92400e"], Success: ["fa-award", "#0284c7"], Dead: ["fa-skull", "#64748b"],
    };
    const look = name => STAGE_LOOK[name] || ["fa-circle", "#64748b"];
    const stageLabel = name => ((D.stages || []).find(s => s.name === name) || {}).label || name;
    const stageOrder = name => { const i = (D.stages || []).findIndex(s => s.name === name); return i < 0 ? 99 : i; };

    // ---------- Dates ----------
    const days = (a, b) => Math.round((new Date(b + "T12:00:00") - new Date(a + "T12:00:00")) / 864e5);
    const since = d => d ? days(d, TODAY) : null;
    const plants = (D.plants || []).map(p => {
        const day = p.start ? days(p.start, TODAY) + 1 : null;
        const harvest = p.group === "living" && p.est_max ? [days(TODAY, p.est_min || p.est_max), days(TODAY, p.est_max)] : null;
        return {
            ...p, day, week: day ? Math.ceil(day / 7) : null,
            stageWeek: p.stage_since ? Math.ceil((days(p.stage_since, TODAY) + 1) / 7) : null,
            water: since(p.last_water), feed: since(p.last_feed), harvest,
        };
    });

    // ---------- State (layout and tab remembered) ----------
    const GROUPS = [["living", "pl_growing", "fa-seedling"], ["harvested", "pl_harvested", "fa-scissors"], ["dead", "pl_dead", "fa-skull"]];
    const fromHash = () => { const h = location.hash.slice(1); return GROUPS.some(g => g[0] === h) ? h : null; };
    const S = {
        group: fromHash() || "living", q: "", zone: "", breeder: "", stage: "",
        sort: store.get("isley.plants.sort") || "name",
        layout: store.get("isley.plants.layout") || "list",
    };
    const count = g => plants.filter(p => p.group === g).length;

    function filtered() {
        const q = S.q.toLowerCase();
        const list = plants.filter(p => p.group === S.group && (!S.zone || p.zone === S.zone) && (!S.breeder || p.breeder === S.breeder) &&
            (!S.stage || p.stage === S.stage) && (!q || [p.name, p.strain, p.breeder, p.zone, stageLabel(p.stage)].join(" ").toLowerCase().includes(q)));
        const byName = (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true });
        const sorts = {
            name: byName,
            newest: (a, b) => (b.start || "").localeCompare(a.start || "") || byName(a, b),
            oldest: (a, b) => (a.start || "").localeCompare(b.start || "") || byName(a, b),
            water: (a, b) => (b.water ?? -1) - (a.water ?? -1) || byName(a, b),
            harvest: (a, b) => ((a.harvest || [1e9])[0]) - ((b.harvest || [1e9])[0]) || byName(a, b),
        };
        return list.sort(sorts[S.sort] || byName);
    }

    // ---------- Building blocks ----------
    const typeTag = p => p.autoflower
        ? `<span class="pl-tag auto"><i class="fa-solid fa-bolt"></i>${esc(t("pl_autoflower"))}</span>`
        : `<span class="pl-tag photo"><i class="fa-solid fa-sun"></i>${esc(t("pl_photoperiod"))}</span>`;
    function stagePill(p) {
        const [icon, color] = look(p.stage);
        const wk = p.group === "living" && p.stageWeek ? ` · ${esc(t("pl_wk", { n: p.stageWeek }))}` : "";
        return `<span class="pl-stage" style="--c:${color}"><i class="fa-solid ${icon}"></i>${esc(stageLabel(p.stage))}${wk}</span>`;
    }
    function ago(d) { return d === 0 ? t("pl_today") : d === 1 ? t("pl_day_ago") : t("pl_days_ago", { n: d }); }
    function wf(kind, d) {
        if (d == null) return "";
        const lim = kind === "water" ? [3, 6] : [10, 18];
        const cls = d >= lim[1] ? "over" : d >= lim[0] ? "due" : "ok";
        const icon = kind === "water" ? "fa-droplet" : "fa-flask";
        const title = t(kind === "water" ? "pl_watered" : "pl_fed", { when: ago(d) });
        return `<span class="pl-wf ${cls}" title="${esc(title)}" aria-label="${esc(title)}"><i class="fa-solid ${icon}"></i>${d === 0 ? esc(t("pl_today")) : d + "d"}</span>`;
    }
    function harvestTxt(p) {
        if (!p.harvest) return "";
        const [a, b] = p.harvest.map(x => Math.round(x / 7));
        if (p.harvest[1] <= 0) return t("pl_harvest_now");
        if (Math.max(0, a) === b) return t("pl_harvest_in_one", { n: b });
        return t("pl_harvest_in", { a: Math.max(0, a), b });
    }
    const photo = p => p.photo ? `<img src="${esc(p.photo)}" alt="" loading="lazy">` : `<div class="pl-noph"><i class="fa-solid fa-cannabis"></i></div>`;
    const url = p => "/plant/" + p.id;
    function actions(p) {
        if (!LOGGED_IN || p.group !== "living") return "";
        return `<a class="pl-qa primary" href="${url(p)}#log"><i class="fa-solid fa-plus"></i>${esc(t("pl_log"))}</a>
            <a class="pl-qa icon" href="${url(p)}#upload" title="${esc(t("pl_upload"))}" aria-label="${esc(t("pl_upload"))}"><i class="fa-solid fa-camera"></i></a>
            <a class="pl-qa icon" href="${url(p)}/edit" title="${esc(t("pl_edit"))}" aria-label="${esc(t("pl_edit"))}"><i class="fa-solid fa-pen"></i></a>`;
    }
    function progress(p) {
        if (p.group !== "living") {
            const w = p.harvest_weight > 0 ? `<b>${esc(t("pl_grams", { n: Number(p.harvest_weight).toLocaleString(D.lang || undefined) }))}</b>` : "";
            return `<div class="pl-prog"><div class="lbls"><span>${stagePill(p)}</span><span>${w}</span></div></div>`;
        }
        const left = p.harvest ? Math.max(0, (p.harvest[0] + p.harvest[1]) / 2) : null;
        const total = p.day && left != null ? p.day + left : null;
        const pct = total ? Math.min(100, Math.round(p.day / total * 100)) : null;
        const color = look(p.stage)[1];
        const h = harvestTxt(p);
        return `<div class="pl-prog"><div class="lbls"><span>${stagePill(p)}</span><span>${p.day ? `<b>${esc(t("pl_day", { n: p.day }))}</b>` : ""}${h ? ` · ${esc(h)}` : ""}</span></div>
            <div class="bar" title="${esc(h || t("pl_no_estimate"))}">${pct != null ? `<span style="width:${pct}%;background:linear-gradient(90deg, #84cc16, ${color})"></span>` : `<span class="unknown"></span>`}</div></div>`;
    }
    function row(p) {
        const zoneIcon = /outdoor|outside/i.test(p.zone) ? "fa-tree" : "fa-tent";
        return `<div class="pl-r">
            <a class="ph" href="${url(p)}" aria-hidden="true" tabindex="-1">${photo(p)}</a>
            <div class="pl-who"><a class="nm" href="${url(p)}">${esc(p.name)}</a><div class="sub">${esc(p.strain)}${p.breeder ? ` · ${esc(p.breeder)}` : ""}</div>
                <div class="meta">${p.zone ? `<span class="pl-tag zone"><i class="fa-solid ${zoneIcon}"></i>${esc(p.zone)}</span>` : ""}${typeTag(p)}${p.clone ? `<span class="pl-tag zone"><i class="fa-solid fa-clone"></i>${esc(t("pl_clone"))}</span>` : ""}</div></div>
            ${progress(p)}
            <div class="wfs">${p.group === "living" ? wf("water", p.water) + wf("feed", p.feed) : ""}</div>
            <div class="acts">${actions(p)}</div>
        </div>`;
    }
    const tile = p => `<a class="pl-t" href="${url(p)}">${photo(p)}${stagePill(p)}
        ${p.group === "living" && p.water != null ? `<span class="wfx">${wf("water", p.water)}</span>` : ""}
        <span class="ov"><b>${esc(p.name)}</b><small>${esc(p.zone)}${p.group === "living" && p.day ? ` · ${esc(t("pl_day", { n: p.day }))}` : ""}</small></span></a>`;

    // ---------- Drawing ----------
    function renderTabs() {
        document.getElementById("plTabs").innerHTML = GROUPS.map(([k, label, icon]) =>
            `<button type="button" role="tab" aria-selected="${S.group === k}" data-group="${k}" class="${S.group === k ? "on" : ""}"><i class="fa-solid ${icon}"></i>${esc(t(label))}<span class="n">${count(k)}</span></button>`).join("");
        const living = plants.filter(p => p.group === "living");
        const flowering = living.filter(p => p.stage === "Flower").length;
        const soon = living.filter(p => p.harvest && p.harvest[0] <= 21).length;
        const thirsty = living.filter(p => p.water != null && p.water >= 3).length;
        // A summary chip: the translated text with its number in bold.
        const chip = (key, v, cls = "", icon = "") => v
            ? `<span class="${cls}">${icon}${esc(t(key, { n: "#N#" })).replace("#N#", `<b>${v}</b>`)}</span>` : "";
        document.getElementById("plSum").innerHTML = S.group === "living" && living.length
            ? chip("pl_sum_growing", living.length) + chip("pl_sum_flower", flowering) + chip("pl_sum_harvest_soon", soon) +
              chip("pl_sum_thirsty", thirsty, "warn", `<i class="fa-solid fa-droplet me-1"></i>`)
            : "";
    }
    function renderBar() {
        const inGroup = plants.filter(p => p.group === S.group);
        const uniq = key => [...new Set(inGroup.map(p => p[key]).filter(Boolean))].sort((a, b) => a.localeCompare(b));
        const stages = [...new Set(inGroup.map(p => p.stage).filter(Boolean))].sort((a, b) => stageOrder(a) - stageOrder(b));
        const opt = (v, l, cur) => `<option value="${esc(v)}" ${v === cur ? "selected" : ""}>${esc(l)}</option>`;
        const n = filtered().length;
        const sorts = [["name", "pl_sort_name"], ["newest", "pl_sort_newest"], ["oldest", "pl_sort_oldest"], ["water", "pl_sort_water"], ["harvest", "pl_sort_harvest"]];
        document.getElementById("plBar").innerHTML = `<div class="pl-bar">
            <div class="pl-search"><i class="fa-solid fa-magnifying-glass"></i><input type="search" id="plQ" placeholder="${esc(t("pl_filter"))}" aria-label="${esc(t("pl_filter"))}" value="${esc(S.q)}"></div>
            <select class="form-select form-select-sm" data-f="zone" aria-label="${esc(t("pl_all_tents"))}">${opt("", t("pl_all_tents"), S.zone)}${uniq("zone").map(z => opt(z, z, S.zone)).join("")}</select>
            <select class="form-select form-select-sm" data-f="breeder" aria-label="${esc(t("pl_all_breeders"))}">${opt("", t("pl_all_breeders"), S.breeder)}${uniq("breeder").map(b => opt(b, b, S.breeder)).join("")}</select>
            <select class="form-select form-select-sm" data-f="stage" aria-label="${esc(t("pl_all_stages"))}">${opt("", t("pl_all_stages"), S.stage)}${stages.map(s => opt(s, stageLabel(s), S.stage)).join("")}</select>
            <select class="form-select form-select-sm" data-f="sort" aria-label="${esc(t("pl_sort"))}">${sorts.map(([v, k]) => opt(v, t("pl_sort") + ": " + t(k), S.sort)).join("")}</select>
            ${S.zone || S.breeder || S.stage || S.q ? `<button type="button" class="pl-clear" data-clear><i class="fa-solid fa-xmark me-1"></i>${esc(t("pl_clear"))}</button>` : ""}
            <span class="spacer"></span>
            <span class="pl-count">${esc(n === 1 ? t("pl_count_one") : t("pl_count", { n }))}</span>
            <div class="pl-seg" role="group" aria-label="${esc(t("pl_layout"))}">
                <button type="button" data-layout="list" class="${S.layout === "list" ? "on" : ""}" aria-pressed="${S.layout === "list"}"><i class="fa-solid fa-list"></i>${esc(t("pl_list"))}</button>
                <button type="button" data-layout="cards" class="${S.layout === "cards" ? "on" : ""}" aria-pressed="${S.layout === "cards"}"><i class="fa-solid fa-grip"></i>${esc(t("pl_cards"))}</button>
            </div>
        </div>`;
    }
    function renderOut() {
        const list = filtered();
        const out = document.getElementById("plOut");
        if (!list.length) {
            const nothing = !count(S.group);
            const msg = nothing ? t("pl_none_" + S.group) : t("pl_none");
            out.innerHTML = `<div class="pl-empty"><i class="fa-solid fa-seedling"></i><p>${esc(msg)}</p>${nothing && S.group === "living" && LOGGED_IN ? `<a class="pl-add" href="/plant/new"><i class="fa-solid fa-plus me-1"></i>${esc(t("pl_add_plant"))}</a>` : ""}</div>`;
            return;
        }
        out.innerHTML = S.layout === "cards" ? `<div class="pl-tiles">${list.map(tile).join("")}</div>` : `<div class="pl-rows">${list.map(row).join("")}</div>`;
    }
    function render() { renderTabs(); renderBar(); renderOut(); }

    // ---------- Events ----------
    root.addEventListener("click", e => {
        const g = e.target.closest("[data-group]");
        if (g) { S.group = g.dataset.group; S.stage = ""; history.replaceState(null, "", "#" + S.group); render(); return; }
        const l = e.target.closest("[data-layout]");
        if (l) { S.layout = l.dataset.layout; store.set("isley.plants.layout", S.layout); render(); return; }
        if (e.target.closest("[data-clear]")) { Object.assign(S, { q: "", zone: "", breeder: "", stage: "" }); render(); }
    });
    root.addEventListener("change", e => {
        const k = e.target.dataset && e.target.dataset.f;
        if (!k) return;
        S[k] = e.target.value;
        if (k === "sort") store.set("isley.plants.sort", S.sort);
        render();
    });
    root.addEventListener("input", e => {
        if (e.target.id !== "plQ") return;
        S.q = e.target.value;
        renderTabs(); renderOut();
        const c = root.querySelector(".pl-count"), n = filtered().length;
        if (c) c.textContent = n === 1 ? t("pl_count_one") : t("pl_count", { n });
        const clear = root.querySelector("[data-clear]");
        if (!!clear !== !!(S.zone || S.breeder || S.stage || S.q)) { const pos = e.target.selectionStart; renderBar(); const i = document.getElementById("plQ"); i.focus(); i.setSelectionRange(pos, pos); }
    });
    window.addEventListener("hashchange", () => { const h = fromHash(); if (h && h !== S.group) { S.group = h; S.stage = ""; render(); } });

    render();
})();
