// Plant page: header with stage track, the notebook (one entry per plant per
// day, with photos tied to it by date), the photo panel / viewer / gallery,
// the Add activity / Edit day window, and the "All plants" mode (every
// plant's notebook in one list). Data comes from #plantPage[data-page]
// (handlers.BuildPlantPageData); translated strings are in D.t ("pp_" keys).
(() => {
    "use strict";
    const root = document.getElementById("plantPage");
    if (!root) return;
    const D = JSON.parse(root.dataset.page || "{}");
    const P = D.plant || {};
    const TODAY = D.today;
    const LANG = D.lang || undefined;
    const LOGGED_IN = !!D.logged_in;
    const $ = id => document.getElementById(id);

    // ---------- Helpers ----------
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    function tr(key, vars) {
        let s = (D.t && D.t[key]) || key;
        if (vars) for (const k of Object.keys(vars)) s = s.split("{" + k + "}").join(vars[k]);
        return s;
    }
    const asDate = iso => new Date(String(iso).slice(0, 10) + "T12:00:00");
    const fmt = (iso, opts) => asDate(iso).toLocaleDateString(LANG, opts || { month: "short", day: "numeric" });
    const daysBetween = (a, b) => Math.round((asDate(b) - asDate(a)) / 864e5);
    const pad = n => String(n).padStart(2, "0");
    const nowTime = () => { const d = new Date(); return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`; };
    const isDesktop = () => window.matchMedia("(min-width: 992px)").matches;
    const num = v => Number.isInteger(v) ? String(v) : String(Math.round(v * 100) / 100);
    const toast = (msg, level) => window.uiMessages ? uiMessages.showToast(msg, level || "success") : null;
    const store = { get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } }, set(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private mode */ } } };

    async function api(method, url, body) {
        // no-store: a Back navigation must not replay a stale notebook.
        const opts = { method, cache: "no-store", headers: { "Accept": "application/json" } };
        if (body !== undefined) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
        const resp = await fetch(url, opts);
        const data = await resp.json().catch(() => ({}));
        if (!resp.ok) throw new Error(data.error || data.message || resp.statusText);
        return data;
    }

    // Stage icons and colors (same as the mockup's track).
    const STAGE_STYLE = {
        germinating: ["fa-seedling", "#a3b18a"], planted: ["fa-bucket", "#8fbc5a"], seedling: ["fa-leaf", "#84cc16"],
        veg: ["fa-tree", "#16a34a"], flower: ["fa-cannabis", "#9333ea"], drying: ["fa-wind", "#d97706"],
        curing: ["fa-jar", "#92400e"], success: ["fa-award", "#0284c7"], dead: ["fa-skull-crossbones", "#64748b"],
    };
    const stageStyle = name => STAGE_STYLE[String(name || "").toLowerCase()] || ["fa-circle", "#64748b"];
    const STATUS_BY_NAME = new Map((D.statuses || []).map(s => [s.status, s]));
    const statusLabel = name => (STATUS_BY_NAME.get(name) || {}).label || name;

    // Stage (and week within it) on a given day, from a plant's history
    // (oldest first, entries {status, date}).
    function stageOn(history, iso) {
        let h = null;
        for (const x of history) if (String(x.date).slice(0, 10) <= iso) h = x;
        if (!h) return null;
        const [icon, color] = stageStyle(h.status);
        const start = String(h.date).slice(0, 10);
        return { status: h.status, label: statusLabel(h.status), icon, color, start, week: Math.ceil((daysBetween(start, iso) + 1) / 7) };
    }

    // ---------- This plant ----------
    const HISTORY = (D.history || []).slice().sort((a, b) => a.date < b.date ? -1 : a.date > b.date ? 1 : a.id - b.id);
    const STATUS_NOW = (D.statuses || []).find(s => s.id === P.status_id) || { status: P.status, active: true };
    const FINISHED = !STATUS_NOW.active;
    const dayOfGrow = iso => P.start ? daysBetween(P.start, iso) + 1 : null;
    const TYPES = new Map((D.types || []).map(t => [t.id, t]));
    const METRICS = new Map((D.metrics || []).map(m => [m.id, m]));
    // Built-ins first (Water, Feed, Note, Height), then the user's own by name.
    const TYPE_ORDER = (D.types || []).slice().sort((a, b) => {
        const rank = t => t.is_watering && !t.is_feeding ? 0 : t.is_feeding ? 1 : t.name === "Note" ? 2 : t.name === "Height" ? 3 : 4;
        return rank(a) - rank(b) || a.name.localeCompare(b.name);
    });
    let DAYS = D.days || [];
    const dayByDate = date => DAYS.find(d => d.date === date);

    let allMode = location.hash === "#all";
    let view = store.get("pp-view") || "weeks";
    const filter = { types: new Set(), mode: "any" };
    let selected = null; // this plant: a date; all plants: a row key

    // =====================================================================
    // Header
    // =====================================================================
    function harvestText() {
        if (P.est_max) {
            return P.est_min && P.est_min !== P.est_max ? `${fmt(P.est_min)} – ${fmt(P.est_max)}` : fmt(P.est_max);
        }
        return "";
    }
    function endDate(names) {
        const h = HISTORY.find(x => names.includes(String(x.status).toLowerCase()));
        return h ? String(h.date).slice(0, 10) : null;
    }

    function renderHero() {
        const hero = $("ppHero");
        if (allMode) { renderAllHero(); return; }
        hero.classList.remove("all");
        const cur = stageOn(HISTORY, TODAY);
        const facts = [];
        facts.push(P.autoflower ? `<span class="pp-tag"><i class="fa-solid fa-bolt"></i>${esc(tr("pp_autoflower"))}</span>`
            : `<span class="pp-tag photo"><i class="fa-solid fa-sun"></i>${esc(tr("pp_photoperiod"))}</span>`);
        if (P.clone) {
            facts.push(`<span class="pp-tag neutral"><i class="fa-solid fa-clone"></i>${P.parent_id ? `${esc(tr("pp_clone_of"))} <a href="/plant/${P.parent_id}">${esc(P.parent_name)}</a>` : esc(tr("pp_clone"))}</span>`);
        }
        if (P.harvest_weight > 0) facts.push(`<span class="pp-tag neutral"><i class="fa-solid fa-weight-scale"></i>${esc(tr("pp_grams", { n: num(P.harvest_weight) }))}</span>`);
        if (P.start) facts.push(`<span>${esc(tr("pp_started"))} <b>${esc(fmt(P.start))}</b></span>`);
        if (!FINISHED) {
            if (P.start) facts.push(`<span class="sep">·</span><span><b>${esc(tr("pp_day"))} ${dayOfGrow(TODAY)}</b></span>`);
            if (cur) facts.push(`<span class="sep">·</span><span style="color:${cur.color}"><b style="color:inherit">${esc(cur.label)} ${esc(tr("pp_week"))} ${cur.week}</b></span>`);
            const h = harvestText();
            const hint = !P.cycle_time ? tr("pp_not_set_hint") : tr("pp_after_flower_hint");
            facts.push(`<span class="sep">·</span><span>${esc(tr("pp_est_harvest"))} ${h ? `<b>${esc(h)}</b>` : `<b class="text-body-tertiary" title="${esc(hint)}">${esc(tr("pp_not_set"))}</b>`}</span>`);
        } else if (String(P.status).toLowerCase() === "dead") {
            const d = endDate(["dead"]);
            if (d) facts.push(`<span class="sep">·</span><span>${esc(tr("pp_died"))} <b>${esc(fmt(d))}</b></span>`);
        } else {
            const d = endDate(["drying", "curing", "success"]);
            if (d) facts.push(`<span class="sep">·</span><span>${esc(tr("pp_harvested"))} <b>${esc(fmt(d))}</b></span>`);
            if (LOGGED_IN) facts.push(`<span class="sep">·</span><a class="pp-smoke-btn" href="/smoke/new?plant_id=${P.id}"><i class="fa-solid fa-plus"></i>${esc(tr("pp_smoke_report"))}</a>`);
        }

        const sub = [P.strain, P.breeder ? (P.strain_url ? `<a href="${esc(P.strain_url)}" target="_blank" rel="noopener">${esc(P.breeder)}</a>` : esc(P.breeder)) : "", P.zone]
            .map((s, i) => i === 1 ? s : esc(s)).filter(Boolean).join(" · ");

        const live = D.stream ? `<div class="pp-src" role="group" aria-label="${esc(tr("pp_header_picture"))}">
                <button class="on" data-src="photo"><i class="fa-solid fa-image me-1"></i>${esc(tr("pp_photo"))}</button>
                <button data-src="live"><i class="fa-solid fa-video me-1"></i>${esc(tr("pp_live"))}</button></div>
              <span class="pp-live-badge"><i class="fa-solid fa-circle"></i>LIVE</span>
              <button class="pp-live-play" id="ppLivePlay" title="${esc(tr("pp_play"))}" aria-label="${esc(tr("pp_play"))}"><i class="fa-solid fa-play"></i></button>` : "";
        const acts = LOGGED_IN ? `<div class="pp-hacts">
                <a href="/plant/${P.id}/edit" title="${esc(tr("pp_edit_plant"))}" aria-label="${esc(tr("pp_edit_plant"))}"><i class="fa-solid fa-pen-to-square"></i></a>
                ${["dead", "success"].includes(String(P.status).toLowerCase()) ? "" : `<button class="danger" id="ppKill" title="${esc(tr("pp_mark_dead"))}" aria-label="${esc(tr("pp_mark_dead"))}"><i class="fa-solid fa-skull-crossbones"></i></button>`}</div>` : "";

        hero.innerHTML = `
        <div class="pp-htxt">
            <h1>${esc(P.name)}</h1>
            <div class="pp-sub">${sub}</div>
            ${P.description ? `<div class="pp-desc" title="${esc(P.description)}">${esc(P.description)}</div>` : ""}
            <div class="pp-facts">${facts.join("")}</div>
            <div class="pp-track">${trackHTML()}</div>
        </div>
        <div class="pp-hphoto" id="ppHPhoto">
            ${P.photo ? `<img id="ppHImg" src="${esc(P.photo)}" alt="">` : `<div class="pp-nophoto"><div><i class="fa-regular fa-image fa-lg d-block mb-1"></i>${esc(tr("pp_no_photos_yet"))}</div></div>`}
            ${live}${acts}
        </div>`;
    }

    // Stage track: one node per stage; days spent in each; current ringed.
    function trackHTML() {
        const isDead = String(P.status).toLowerCase() === "dead";
        const stages = (D.statuses || []).filter(s => s.status.toLowerCase() !== "dead" || isDead);
        const curIdx = stages.findIndex(s => s.id === P.status_id);
        const durations = {};
        HISTORY.forEach((h, i) => {
            const next = HISTORY[i + 1];
            const end = next ? next.date : (FINISHED ? h.date : TODAY);
            durations[h.status_id] = (durations[h.status_id] || 0) + Math.max(0, daysBetween(h.date, end));
        });
        const reached = new Set(HISTORY.map(h => h.status_id));
        // After the stages: "Smoke", for harvested plants or any plant with a
        // smoke report (its latest score and how many).
        const smoke = D.smoke || {};
        const showSmoke = smoke.count > 0 || ["drying", "curing", "success"].includes(String(P.status).toLowerCase());
        let smokeStep = "";
        if (showSmoke) {
            const href = smoke.count ? `/smoke/${smoke.last_id}` : `/smoke/new?plant_id=${P.id}`;
            const title = smoke.count ? tr("pp_smoke_n", { n: smoke.count, score: smoke.latest != null ? smoke.latest.toFixed(1) : "–" }) : tr("pp_smoke_report");
            const node = smoke.count || LOGGED_IN ? `<a class="pp-node" href="${href}" title="${esc(title)}" aria-label="${esc(title)}"><i class="fa-solid fa-joint"></i></a>`
                : `<span class="pp-node"><i class="fa-solid fa-joint"></i></span>`;
            smokeStep = `<div class="pp-stg pp-smoke ${smoke.count ? "done" : ""}" style="--sc:#ea580c"><div class="pp-stg-row">${node}</div>
                <div class="pp-lbl">${esc(tr("pp_smoke"))}${smoke.count ? `<small>${smoke.latest != null ? smoke.latest.toFixed(1) + " · " : ""}${smoke.count}</small>` : ""}</div></div>`;
        }
        return stages.map((s, i) => {
            const [icon, color] = stageStyle(s.status);
            const isCur = i === curIdx;
            // Every stage before the current one counts as passed, even if it
            // was never logged (e.g. Planted skipped); days only when logged.
            const cls = isCur ? "cur" : (i < curIdx ? "done" : "");
            const days = reached.has(s.id) ? durations[s.id] : null;
            // The stage a growing plant is in shows its day number (the day it
            // entered the stage is Day 1, like the header's week); others how long they lasted.
            const now = isCur && !FINISHED && days != null ? stageOn(HISTORY, TODAY) : null;
            const stageDay = now ? daysBetween(now.start, TODAY) + 1 : null;
            const title = !LOGGED_IN ? s.label : reached.has(s.id) ? tr("pp_change_stage_date", { stage: s.label }) : tr("pp_move_to", { stage: s.label });
            return `<div class="pp-stg ${cls}" style="--sc:${color}">
                <div class="pp-stg-row"><button class="pp-node" ${LOGGED_IN ? `data-stage="${s.id}"` : "disabled"} title="${esc(title)}" aria-label="${esc(title)}"><i class="fa-solid ${icon}"></i></button>${i < stages.length - 1 || smokeStep ? `<div class="pp-seg-line"></div>` : ""}</div>
                <div class="pp-lbl">${esc(s.label)}${stageDay != null ? `<small>${esc(tr("pp_day"))} ${stageDay}</small>` : days != null ? `<small>${days}d</small>` : ""}</div></div>`;
        }).join("") + smokeStep;
    }

    // Stage click: a reached stage edits its date (existing window); any
    // other stage asks first, with the date defaulting to today.
    function onStageClick(statusID) {
        const latest = [...HISTORY].reverse().find(h => h.status_id === statusID);
        if (latest) {
            const el = $("editStatusModal");
            if (!el) return;
            $("statusId").value = latest.id;
            $("editStatusDate").value = String(latest.date).slice(0, 19);
            const del = $("deleteStatus");
            if (del) del.disabled = HISTORY.length <= 1;
            bootstrap.Modal.getOrCreateInstance(el).show();
            return;
        }
        openStageMove(statusID);
    }

    function modal(id, inner, cls = "") {
        let el = $(id);
        if (!el) {
            document.body.insertAdjacentHTML("beforeend", `<div class="modal fade ${cls}" id="${id}" tabindex="-1" aria-hidden="true">${inner}</div>`);
            el = $(id);
        }
        return el;
    }

    function openStageMove(statusID) {
        const s = (D.statuses || []).find(x => x.id === statusID);
        if (!s) return;
        const [icon, color] = stageStyle(s.status);
        const order = (D.statuses || []).map(x => x.id);
        const back = order.indexOf(statusID) < order.indexOf(P.status_id);
        const el = modal("ppStageModal", `<div class="modal-dialog modal-dialog-centered modal-sm"><div class="modal-content pp-dialog">
            <div class="modal-body text-center pt-4" id="ppStageBody"></div>
            <div class="modal-footer justify-content-center"><button class="btn btn-light" data-bs-dismiss="modal">${esc(tr("pp_cancel"))}</button><button class="btn btn-success" id="ppStageGo"></button></div></div></div>`);
        $("ppStageBody").innerHTML = `
            <span class="pp-ico mb-2" style="--c:${color};width:38px;height:38px;border-radius:50%;font-size:.95rem"><i class="fa-solid ${icon}"></i></span>
            <h5 class="fw-bold mb-1">${esc(tr("pp_move_to", { stage: s.label }))}</h5>
            <p class="small text-muted mb-3">${esc(tr("pp_in_stage_now", { plant: P.name, stage: statusLabel(P.status) }))}${back ? " " + esc(tr("pp_moves_back")) : ""}</p>
            <label class="form-label small text-muted mb-1" for="ppStageDate">${esc(tr("pp_date_of_change"))}</label>
            <input type="date" class="form-control form-control-sm mx-auto" style="max-width:170px" id="ppStageDate" value="${TODAY}" max="${TODAY}">`;
        const go = $("ppStageGo");
        go.textContent = tr("pp_move_btn", { stage: s.label });
        go.className = "btn " + (s.status.toLowerCase() === "dead" ? "btn-danger" : "btn-success");
        go.onclick = async () => {
            const date = $("ppStageDate").value || TODAY;
            go.disabled = true;
            try {
                await api("POST", "/plant/status", { plant_id: P.id, status_id: statusID, date: `${date}T${nowTime()}` });
                location.reload();
            } catch (e) {
                go.disabled = false;
                toast(e.message, "danger");
            }
        };
        bootstrap.Modal.getOrCreateInstance(el).show();
    }

    // Photo | Live switch (only when the plant's zone has a camera).
    let player = null;
    function stopLive() {
        if (player) { try { player.dispose(); } catch (e) { /* already gone */ } player = null; }
        const v = $("ppLiveVideo");
        if (v) v.remove();
    }
    function setSource(src) {
        const box = $("ppHPhoto");
        if (!box || !D.stream) return;
        box.querySelectorAll("[data-src]").forEach(b => b.classList.toggle("on", b.dataset.src === src));
        const live = src === "live";
        box.classList.toggle("live", live);
        stopLive();
        let img = $("ppHImg");
        if (!img) { box.insertAdjacentHTML("afterbegin", `<img id="ppHImg" alt="">`); img = $("ppHImg"); }
        img.style.display = "";
        img.src = live ? `/streams/${D.stream.id}/snapshot?t=${Date.now()}` : (P.photo || "");
        if (!live && !P.photo) img.remove();
        const play = $("ppLivePlay");
        if (play) play.style.display = live && /\.m3u8($|\?)/i.test(D.stream.url || "") && window.videojs ? "" : "none";
    }
    function playLive() {
        const box = $("ppHPhoto");
        if (!box || !window.videojs) return;
        stopLive();
        box.insertAdjacentHTML("afterbegin", `<video id="ppLiveVideo" class="video-js" muted playsinline></video>`);
        player = window.videojs("ppLiveVideo", { autoplay: true, muted: true, controls: true, fill: true, sources: [{ src: D.stream.url, type: "application/x-mpegURL" }] });
        const play = $("ppLivePlay");
        if (play) play.style.display = "none";
    }

    // =====================================================================
    // Toolbar
    // =====================================================================
    function renderToolbar() {
        const exp = LOGGED_IN ? `<div class="pp-popwrap"><button class="pp-b" id="ppExportBtn" title="${esc(tr("pp_export"))}" aria-haspopup="true"><i class="fa-solid fa-download"></i><span class="lbl-t ms-1">${esc(tr("pp_export"))}</span></button>
            <div class="pp-pop right" id="ppExportPop"></div></div>` : "";
        const add = LOGGED_IN ? `<button class="pp-b primary" id="ppAddBtn"><i class="fa-solid fa-plus"></i><span class="lbl-t ms-1">${esc(tr("pp_add_activity"))}</span></button>
            <button class="pp-b primary" id="ppUploadBtn" title="${esc(tr("pp_upload_photos"))}" aria-label="${esc(tr("pp_upload_photos"))}"><i class="fa-solid fa-camera"></i></button>` : "";
        $("ppToolbar").innerHTML = `
            <h2 id="ppNotebookTitle">${esc(tr("pp_notebook"))}</h2>
            <div class="pp-segsw" role="group" aria-label="${esc(tr("pp_whose_notebook"))}">
                <button data-mode="one">${esc(tr("pp_this_plant"))}</button>
                <button data-mode="all"><i class="fa-solid fa-cannabis me-1"></i>${esc(tr("pp_all_plants"))}</button>
            </div>
            <div class="pp-segsw" role="group" aria-label="${esc(tr("pp_notebook_view"))}" id="ppViewSw">
                <button data-view="weeks"><i class="fa-solid fa-layer-group me-1"></i>${esc(tr("pp_weeks"))}</button>
                <button data-view="list"><i class="fa-solid fa-list me-1"></i>${esc(tr("pp_list"))}</button>
            </div>
            <div class="pp-srch d-none" id="ppSrchWrap"><i class="fa-solid fa-magnifying-glass"></i><input type="search" id="ppSearch" placeholder="${esc(tr("pp_search_notes"))}" aria-label="${esc(tr("pp_search_notes"))}"></div>
            <div class="pp-popwrap"><button class="pp-b" id="ppFilterBtn" aria-haspopup="true"><i class="fa-solid fa-filter"></i><span class="lbl-t ms-1">${esc(tr("pp_filter"))}</span><span class="n d-none" id="ppFilterCount"></span></button>
                <div class="pp-pop" id="ppFilterPop"></div></div>
            <div class="pp-right">
                <button class="pp-b" id="ppGalleryBtn" title="${esc(tr("pp_all_photos"))}"><i class="fa-solid fa-images"></i><span class="lbl-t ms-1">${esc(tr("pp_all_photos"))}</span></button>
                ${exp}${add}
            </div>`;
        syncToolbar();
    }
    function syncToolbar() {
        root.classList.toggle("pp-all", allMode);
        document.querySelectorAll("[data-mode]").forEach(b => b.classList.toggle("on", (b.dataset.mode === "all") === allMode));
        document.querySelectorAll("[data-view]").forEach(b => b.classList.toggle("on", b.dataset.view === view));
        $("ppViewSw").classList.toggle("d-none", allMode);
        $("ppSrchWrap").classList.toggle("d-none", !allMode);
        const crumb = $("ppCrumbName");
        if (crumb) crumb.textContent = allMode ? tr("pp_all_plants") : P.name;
    }

    function renderExportPop() {
        const q = allMode ? `plant_ids=${[...AS.plants].join(",")}` : `plant_id=${P.id}`;
        $("ppExportPop").innerHTML = `<a class="pp-menu-item" href="/activities/export/csv?${q}"><i class="fa-solid fa-file-csv"></i>${esc(tr("pp_export_csv"))}</a>
            <a class="pp-menu-item" href="/activities/export/xlsx?${q}"><i class="fa-solid fa-file-excel"></i>${esc(tr("pp_export_xlsx"))}</a>`;
    }

    // =====================================================================
    // Notebook (this plant)
    // =====================================================================
    const ALLTYPES = () => [...TYPE_ORDER.map(t => [String(t.id), t]), ["photos", { name: tr("pp_has_photos"), icon: "fa-camera", color: "#0d9488" }]];
    function matchesTypes(day) {
        if (!filter.types.size) return true;
        const has = k => k === "photos" ? day.photos.length > 0 : day.types.some(t => String(t.activity_id) === k);
        return filter.mode === "all" ? [...filter.types].every(has) : [...filter.types].some(has);
    }
    function iconsHTML(day, camAttr) {
        const parts = day.types.map(t => `<span class="pp-ico" style="--c:${esc(t.color)}" title="${esc(t.name)}${t.count > 1 ? " ×" + t.count : ""}"><i class="fa-solid ${esc(t.icon)}"></i>${t.count > 1 ? `<span class="cnt">${t.count}</span>` : ""}</span>`);
        const n = day.photos.length;
        if (n) parts.push(`<button type="button" class="pp-ico cam" ${camAttr} title="${esc(n > 1 ? tr("pp_n_photos", { n }) : tr("pp_one_photo"))}" aria-label="${esc(n > 1 ? tr("pp_n_photos", { n }) : tr("pp_one_photo"))}"><i class="fa-solid fa-camera"></i>${n > 1 ? `<span class="cnt">${n}</span>` : ""}</button>`);
        return parts.join("");
    }
    function measuresHTML(day) {
        return day.measurements.map(m => `<span class="mval">${esc(m.name)} ${esc(num(m.value))}${m.unit ? " " + esc(m.unit) : ""}</span>`).join(" ");
    }
    function textHTML(day, hl) {
        const note = day.note ? (hl ? hl(day.note) : esc(day.note)) : "";
        const ms = measuresHTML(day);
        if (note || ms) return note + (note && ms ? " " : "") + ms;
        return day.types.length ? "" : `<span class="muted">${esc(tr("pp_photos_only"))}</span>`;
    }
    function rowHTML(day) {
        const s = stageOn(HISTORY, day.date);
        const dg = dayOfGrow(day.date);
        return `<div class="pp-row ${selected === day.date ? "is-selected" : ""}" data-date="${day.date}" style="--sc:${s ? s.color : "#94a3b8"}">
            <div class="dd">${esc(fmt(day.date))}<small>${esc(fmt(day.date, { weekday: "short" }))}${dg != null ? ` · ${esc(tr("pp_day"))} ${dg}` : ""}</small></div>
            <div class="ics">${iconsHTML(day, `data-cam="${day.date}"`)}</div>
            <div class="tx">${textHTML(day)}</div>
            ${LOGGED_IN ? `<button class="edit" data-edit="${day.date}" title="${esc(tr("pp_edit_day"))}"><i class="fa-solid fa-pen-to-square me-1"></i>${esc(tr("pp_edit"))}</button>` : "<span></span>"}
        </div>`;
    }

    function renderRows() {
        if (allMode) { renderAllRows(); return; }
        const list = DAYS.filter(matchesTypes);
        const el = $("ppRows");
        renderChips(list.length);
        if (!DAYS.length) {
            el.innerHTML = `<div class="pp-empty"><i class="fa-solid fa-book-open"></i>${esc(LOGGED_IN ? tr("pp_empty") : tr("pp_empty_guest"))}</div>`;
            renderPanel(null);
            return;
        }
        if (!list.length) {
            el.innerHTML = `<div class="pp-empty"><i class="fa-solid fa-filter-circle-xmark"></i>${esc(tr("pp_no_match"))}</div>`;
            renderPanel(null);
            return;
        }
        if (!list.some(d => d.date === selected)) selected = list[0].date;
        if (view === "weeks") {
            const groups = [];
            list.forEach(d => {
                const s = stageOn(HISTORY, d.date);
                const key = s ? s.status + "|" + s.start + "|" + s.week : "none";
                let g = groups.find(x => x.key === key);
                if (!g) groups.push(g = { key, s, days: [] });
                g.days.push(d);
            });
            el.innerHTML = groups.map(g => `<div class="pp-week">
                <div class="pp-week-h" style="--sc:${g.s ? g.s.color : "#94a3b8"}"><span class="dot"></span>${g.s ? `<span class="st">${esc(g.s.label)}</span> · ${esc(tr("pp_week"))} ${g.s.week}` : esc(tr("pp_no_stage"))}</div>
                <div class="pp-week-b">${g.days.map(rowHTML).join("")}</div></div>`).join("");
        } else {
            let month = "";
            el.innerHTML = `<div class="pp-list-b">${list.map(d => {
                const m = asDate(d.date).toLocaleDateString(LANG, { month: "long", year: "numeric" });
                return (m !== month ? `<div class="pp-month">${esc(month = m)}</div>` : "") + rowHTML(d);
            }).join("")}</div>`;
        }
        renderPanel(dayByDate(selected));
    }

    function renderChips(shown) {
        const n = filter.types.size + (allMode && !plantsDefault() ? 1 : 0);
        const cnt = $("ppFilterCount");
        cnt.classList.toggle("d-none", !n);
        cnt.textContent = n;
        $("ppFilterBtn").classList.toggle("active", n > 0);
        const T = Object.fromEntries(ALLTYPES());
        const bits = [];
        if (allMode && !plantsDefault()) {
            const label = AS.plants.size === AS.meta.length ? tr("pp_all_incl_finished") : AS.plants.size === 1 ? (metaOf([...AS.plants][0]) || {}).name : tr("pp_n_plants", { n: AS.plants.size });
            bits.push(`<span class="pp-fchip"><i class="fa-solid fa-cannabis t"></i>${esc(label)}<button data-reset-plants aria-label="${esc(tr("pp_clear"))}">✕</button></span>`);
        }
        filter.types.forEach(k => bits.push(`<span class="pp-fchip"><i class="fa-solid ${esc(T[k].icon)} t" style="--c:${esc(T[k].color)}"></i>${esc(T[k].name)}<button data-unfilter="${k}" aria-label="${esc(tr("pp_clear"))}">✕</button></span>`));
        if (allMode && AS.q.trim()) bits.push(`<span class="pp-fchip"><i class="fa-solid fa-magnifying-glass t"></i>“${esc(AS.q.trim())}”<button data-unsearch aria-label="${esc(tr("pp_clear"))}">✕</button></span>`);
        if (!bits.length) { $("ppChips").innerHTML = ""; return; }
        const lead = filter.types.size > 1 ? `<span>${esc(filter.mode === "all" ? tr("pp_with_all_of") : tr("pp_with_any_of"))}</span>` : "";
        const count = allMode ? tr(shown === 1 ? "pp_one_entry" : "pp_n_entries", { n: shown }) : tr("pp_of_days", { n: shown, total: DAYS.length });
        $("ppChips").innerHTML = lead + bits.join("") + `<span class="ms-1">${esc(count)}</span><button class="pp-link ms-1" data-clear-filter>${esc(tr("pp_clear"))}</button>`;
    }

    function renderFilterPop() {
        $("ppFilterPop").innerHTML = (allMode ? allPlantPop() : "") + `<h6>${esc(tr(allMode ? "pp_show_entries_with" : "pp_show_days_with"))}</h6>` +
            ALLTYPES().map(([k, t]) => `<label><input type="checkbox" class="form-check-input m-0" data-ftype="${k}" ${filter.types.has(k) ? "checked" : ""}>
                <span class="pp-ico" style="--c:${esc(t.color)}"><i class="fa-solid ${esc(t.icon)}"></i></span>${esc(t.name)}</label>`).join("") +
            `<div class="mode"><button data-fmode="any" class="${filter.mode === "any" ? "on" : ""}">${esc(tr("pp_any_selected"))}</button><button data-fmode="all" class="${filter.mode === "all" ? "on" : ""}">${esc(tr("pp_all_selected"))}</button></div>`;
    }

    // Photo panel (desktop): the selected day's photos.
    function renderPanel(day) {
        if (allMode) { renderAllPanel(); return; }
        const el = $("ppPanel");
        if (!day) { el.innerHTML = ""; return; }
        const s = stageOn(HISTORY, day.date);
        const dg = dayOfGrow(day.date);
        const n = day.photos.length;
        const tab = [fmt(day.date, { weekday: "short", month: "short", day: "numeric" }), dg != null ? `${tr("pp_day")} ${dg}` : "", s ? `${s.label} ${tr("pp_wk")} ${s.week}` : "", n > 1 ? tr("pp_n_photos", { n }) : ""].filter(Boolean).join(" · ");
        el.innerHTML = `<div class="pp-ptab"><i class="fa-regular fa-calendar"></i>${esc(tab)}</div>
            <div class="pp-pbody">${n ? day.photos.map((p, i) => `<img src="${esc(p.path)}" data-open-photo="${i}" data-day="${day.date}" alt="${esc(p.description)}" loading="lazy">`).join("")
                : `<div class="pp-pempty"><div><i class="fa-regular fa-image big"></i>${esc(tr("pp_no_photos_day"))}${LOGGED_IN ? `<div class="mt-2"><button class="pp-b" data-upload><i class="fa-solid fa-camera me-1"></i>${esc(tr("pp_add_photo"))}</button></div>` : ""}</div></div>`}</div>`;
    }

    function select(key, { scroll = false } = {}) {
        selected = key;
        document.querySelectorAll(".pp-row").forEach(r => r.classList.toggle("is-selected", (allMode ? r.dataset.k : r.dataset.date) === String(key)));
        if (allMode) renderAllPanel(); else renderPanel(dayByDate(key));
        if (scroll) {
            const row = [...document.querySelectorAll(".pp-row")].find(r => (allMode ? r.dataset.k : r.dataset.date) === String(key));
            if (row) row.scrollIntoView({ behavior: "smooth", block: "center" });
        }
        window.isleyUploadDefaultDate = () => (allMode ? null : selected) || TODAY;
    }

    // =====================================================================
    // Photo viewer and gallery
    // =====================================================================
    let vList = [], vIdx = 0;
    function openViewer(list, idx) {
        if (!list.length) return;
        const el = modal("ppViewer", `<div class="modal-dialog modal-fullscreen"><div class="modal-content">
            <div class="pp-vtop"><span id="ppVCap"></span><span class="acts">
                <button type="button" class="btn-close btn-close-white ms-2" data-bs-dismiss="modal" aria-label="${esc(tr("pp_close"))}"></button></span></div>
            <div class="pp-vbody"><button class="pp-vnav prev" id="ppVPrev" aria-label="${esc(tr("pp_previous"))}"><i class="fa-solid fa-chevron-left"></i></button>
                <img id="ppVImg" alt=""><button class="pp-vnav next" id="ppVNext" aria-label="${esc(tr("pp_next"))}"><i class="fa-solid fa-chevron-right"></i></button></div>
            ${LOGGED_IN ? `<div class="pp-vacts"><button class="btn btn-sm btn-outline-light" id="ppVDecorate"><i class="fa-solid fa-wand-magic-sparkles me-1"></i>${esc(tr("pp_decorate"))}</button>
                <button class="btn btn-sm btn-outline-danger" id="ppVDelete"><i class="fa-solid fa-trash me-1"></i>${esc(tr("pp_delete_photo"))}</button></div>` : ""}
            <div class="pp-vstrip" id="ppVStrip"></div></div></div>`, "pp-viewer");
        if (!el.dataset.wired) {
            el.dataset.wired = "1";
            $("ppVPrev").addEventListener("click", () => stepViewer(-1));
            $("ppVNext").addEventListener("click", () => stepViewer(1));
            document.addEventListener("keydown", e => {
                if (!el.classList.contains("show")) return;
                if (e.key === "ArrowLeft") stepViewer(-1);
                if (e.key === "ArrowRight") stepViewer(1);
            });
            if (LOGGED_IN) {
                $("ppVDecorate").addEventListener("click", () => {
                    const thumb = document.querySelector(`#ppThumbs .thumbnail-img[data-id="${vList[vIdx].id}"]`);
                    bootstrap.Modal.getInstance(el).hide();
                    if (thumb) setTimeout(() => thumb.click(), 300);
                });
                $("ppVDelete").addEventListener("click", async () => {
                    const ok = await uiMessages.showConfirm(tr("pp_delete_photo_confirm"), { danger: true });
                    if (!ok) return;
                    try {
                        await api("DELETE", `/plant/images/${vList[vIdx].id}/delete`);
                        location.reload();
                    } catch (e) { toast(e.message, "danger"); }
                });
            }
        }
        vList = list; vIdx = idx;
        showViewer();
        bootstrap.Modal.getOrCreateInstance(el).show();
    }
    function showViewer() {
        const p = vList[vIdx];
        $("ppVImg").src = p.path;
        $("ppVImg").alt = p.description || "";
        const who = p.plant && allMode ? esc((metaOf(p.plant) || {}).name || "") + " · " : "";
        $("ppVCap").innerHTML = `<strong>${who}${esc(fmt(p.date, { weekday: "short", month: "short", day: "numeric", year: "numeric" }))}</strong>${p.description ? " · " + esc(p.description) : ""} <span class="ms-2 opacity-75">${vIdx + 1} / ${vList.length}</span>`;
        $("ppVStrip").innerHTML = vList.map((q, i) => `<img src="${esc(q.path)}" class="${i === vIdx ? "active" : ""}" data-vi="${i}" alt="" loading="lazy">`).join("");
        $("ppVStrip").querySelectorAll("img").forEach(img => img.addEventListener("click", () => { vIdx = +img.dataset.vi; showViewer(); }));
        $("ppVPrev").style.visibility = $("ppVNext").style.visibility = vList.length > 1 ? "" : "hidden";
    }
    function stepViewer(d) { vIdx = (vIdx + d + vList.length) % vList.length; showViewer(); }
    const photosOf = day => day.photos.map(p => ({ ...p, date: day.date, plant: day.plant_id }));
    function openDayPhotos(day, i = 0) { openViewer(photosOf(day), i); }

    function openGallery() {
        const days = (allMode ? AROWS.map(r => ({ date: r.date, photos: r.photos, types: r.types, key: r.key, plant_id: r.plants[0].id })) : DAYS).filter(d => d.photos.length);
        const el = modal("ppGallery", `<div class="modal-dialog modal-xl modal-dialog-scrollable"><div class="modal-content">
            <div class="modal-header"><h5 class="modal-title" id="ppGalTitle"></h5><button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="${esc(tr("pp_close"))}"></button></div>
            <div class="modal-body" id="ppGalBody"></div></div></div>`);
        const all = days.flatMap(photosOf);
        $("ppGalTitle").innerHTML = `<i class="fa-solid fa-images me-2"></i>${esc(tr("pp_all_photos"))} <span class="text-muted fw-normal fs-6">(${all.length})</span>`;
        let idx = 0;
        $("ppGalBody").innerHTML = days.length ? days.map(d => `<div class="pp-gal-day">
            <div class="pp-gal-date">${esc(fmt(d.date, { weekday: "short", month: "short", day: "numeric", year: "numeric" }))}${!allMode && dayOfGrow(d.date) != null ? ` <span class="text-muted">· ${esc(tr("pp_day"))} ${dayOfGrow(d.date)}</span>` : ""}</div>
            <div class="pp-gal-grid">${d.photos.map(p => `<div class="pp-gal-item"><img src="${esc(p.path)}" data-gi="${idx++}" alt="${esc(p.description)}" loading="lazy">
                ${d.types.length ? `<button class="pp-gal-jump" data-jump="${esc(allMode ? d.key : d.date)}" title="${esc(tr("pp_go_to_day"))}" aria-label="${esc(tr("pp_go_to_day"))}"><i class="fa-solid fa-book-open"></i></button>` : ""}</div>`).join("")}</div></div>`).join("")
            : `<div class="pp-empty"><i class="fa-regular fa-image"></i>${esc(tr("pp_no_photos_yet"))}</div>`;
        $("ppGalBody").querySelectorAll("[data-gi]").forEach(img => img.addEventListener("click", () => openViewer(all, +img.dataset.gi)));
        $("ppGalBody").querySelectorAll("[data-jump]").forEach(b => b.addEventListener("click", () => {
            bootstrap.Modal.getInstance(el).hide();
            filter.types.clear();
            renderRows();
            setTimeout(() => select(b.dataset.jump, { scroll: true }), 250);
        }));
        bootstrap.Modal.getOrCreateInstance(el).show();
    }

    // =====================================================================
    // Add activity / Edit day window (this plant)
    // =====================================================================
    function dayWindow() {
        return modal("ppDayModal", `<div class="modal-dialog modal-dialog-centered"><div class="modal-content pp-dialog">
            <div class="modal-header"><h5 class="modal-title" id="ppDayTitle"></h5><button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="${esc(tr("pp_close"))}"></button></div>
            <div class="modal-body" id="ppDayBody"></div>
            <div class="modal-footer"><button class="btn btn-outline-danger me-auto d-none" id="ppDayDelete"><i class="fa-solid fa-trash me-1"></i>${esc(tr("pp_delete_day"))}</button>
                <button class="btn btn-light" data-bs-dismiss="modal">${esc(tr("pp_cancel"))}</button><button class="btn btn-success" id="ppDaySave"><i class="fa-solid fa-check me-1"></i>${esc(tr("pp_save"))}</button></div></div></div>`);
    }
    // Ticks + number boxes + note. `plants` (All plants) adds plant chips.
    function windowBody(opts) {
        const { date, ticks, values, note, plants, plantPicks, showDate = true, notice = "" } = opts;
        return `${plants ? `<label class="form-label small fw-semibold mb-1">${esc(tr("pp_plants"))}</label>${opts.plantHint || ""}
                <div class="pp-tchips mb-1" id="ppWinPlants">${plants.map(p => `<button type="button" class="pp-tchip ${plantPicks.has(p.id) ? "on" : ""}" data-pid="${p.id}" style="--c:var(--pp-acc-btn,#0d9488)"><i class="fa-solid fa-cannabis"></i> ${esc(p.name)}</button>`).join("")}</div>
                ${opts.tentLinks || ""}` : ""}
            ${showDate ? `<div class="row g-2 mb-1 mt-1"><div class="col-sm-6"><label class="form-label small text-muted mb-0" for="ppWinDate">${esc(tr("pp_date"))}</label>
                <input type="date" class="form-control form-control-sm" id="ppWinDate" value="${date}" max="${TODAY}"></div></div>` : ""}
            <div class="small mb-3" id="ppWinNotice">${notice}</div>
            <label class="form-label small fw-semibold mb-1">${esc(tr("pp_select_all_apply"))}</label>
            <div class="pp-tchips mb-2" id="ppWinTypes">${TYPE_ORDER.map(t => `<button type="button" class="pp-tchip ${ticks.has(t.id) ? "on" : ""}" data-t="${t.id}" style="--c:${esc(t.color)}" aria-pressed="${ticks.has(t.id)}"><i class="fa-solid ${esc(t.icon)}"></i> ${esc(t.name)}</button>`).join("")}</div>
            <div id="ppWinNumbers" class="mb-2"></div>
            <textarea class="form-control" id="ppWinNote" rows="4" placeholder="${esc(tr("pp_notes_placeholder"))}">${esc(note)}</textarea>
            <div class="text-danger small mt-2 d-none" id="ppWinError"></div>
            ${opts.footnote ? `<p class="small text-muted mt-2 mb-0">${opts.footnote}</p>` : ""}`;
    }
    function wireWindow(el, values) {
        const numbers = () => {
            const box = el.querySelector("#ppWinNumbers");
            const keep = {};
            box.querySelectorAll("input[data-metric]").forEach(i => { keep[i.dataset.act + ":" + i.dataset.metric] = i.value; });
            const ticked = [...el.querySelectorAll("#ppWinTypes .on")].map(b => TYPES.get(+b.dataset.t)).filter(Boolean);
            const saved = values || {};
            const has = k => (k in keep && keep[k] !== "") || saved[k] != null;
            // One box per measurement: Water and Feed ticked together ask for the
            // water amount once. A number saved earlier keeps its own box, even
            // under a type that no longer asks for it, so editing never drops it.
            const seen = new Set(), fields = [];
            ticked.forEach(t => {
                const links = [...(t.metrics || [])];
                Object.keys(saved).forEach(k => {
                    const [a, mid] = k.split(":").map(Number);
                    if (a === t.id && saved[k] != null && !links.some(l => l.metric_id === mid)) links.push({ metric_id: mid, required: false });
                });
                links.forEach(l => {
                    const k = t.id + ":" + l.metric_id;
                    if (seen.has(l.metric_id) && !has(k)) return;
                    seen.add(l.metric_id);
                    fields.push([t, l]);
                });
            });
            box.innerHTML = fields.map(([t, l]) => {
                const m = METRICS.get(l.metric_id) || { name: "", unit: "" };
                const k = t.id + ":" + l.metric_id;
                const v = k in keep ? keep[k] : (saved[k] != null ? saved[k] : "");
                return `<div class="input-group input-group-sm mb-1" style="max-width:280px"><span class="input-group-text">${esc(m.name)}${l.required ? " *" : ""}</span>
                    <input type="number" step="any" class="form-control" data-act="${t.id}" data-metric="${l.metric_id}" data-required="${l.required ? 1 : 0}" value="${esc(v)}">
                    ${m.unit ? `<span class="input-group-text">${esc(m.unit)}</span>` : ""}</div>`;
            }).join("");
        };
        el.querySelectorAll("#ppWinTypes .pp-tchip, #ppWinPlants .pp-tchip").forEach(c => c.addEventListener("click", () => {
            c.classList.toggle("on");
            c.setAttribute("aria-pressed", c.classList.contains("on"));
            if (c.dataset.t) numbers();
        }));
        numbers();
    }
    // Reads the window: ticks, numbers (required ones checked), note.
    function readWindow(el) {
        const err = el.querySelector("#ppWinError");
        err.classList.add("d-none");
        const ticks = [...el.querySelectorAll("#ppWinTypes .on")].map(b => +b.dataset.t);
        const measurements = [];
        let missing = false;
        el.querySelectorAll("#ppWinNumbers input[data-metric]").forEach(i => {
            const v = i.value.trim();
            if (v === "" && i.dataset.required === "1") { missing = true; i.classList.add("is-invalid"); } else i.classList.remove("is-invalid");
            measurements.push({ activity_id: +i.dataset.act, metric_id: +i.dataset.metric, value: v === "" ? null : Number(v) });
        });
        const note = el.querySelector("#ppWinNote").value.trim();
        if (missing) { err.textContent = tr("pp_number_required"); err.classList.remove("d-none"); return null; }
        return { activity_ids: ticks, measurements, note };
    }
    function showError(el, msg) {
        const err = el.querySelector("#ppWinError");
        err.textContent = msg;
        err.classList.remove("d-none");
    }

    function openDay(date) {
        const el = dayWindow();
        const load = d => {
            const day = dayByDate(d);
            const existing = day && (day.types.length || day.note || day.measurements.length) ? day : null;
            const nice = fmt(d, { weekday: "short", month: "long", day: "numeric" });
            $("ppDayTitle").innerHTML = existing ? `<i class="fa-solid fa-pen-to-square me-2"></i>${esc(tr("pp_edit_title", { date: fmt(d) }))}` : `<i class="fa-solid fa-plus me-2"></i>${esc(tr("pp_add_activity"))}`;
            const ticks = new Set(existing ? existing.types.map(t => t.activity_id) : []);
            const values = {};
            if (existing) existing.measurements.forEach(m => { values[m.activity_id + ":" + m.metric_id] = num(m.value); });
            $("ppDayBody").innerHTML = windowBody({
                date: d, ticks, values, note: existing ? existing.note : "",
                notice: existing ? `<span class="text-info-emphasis"><i class="fa-solid fa-circle-info me-1"></i>${esc(tr("pp_existing_entry", { date: nice }))}</span>`
                    : `<span class="text-muted">${esc(tr("pp_new_entry", { date: nice }))}</span>`,
            });
            wireWindow(el, values);
            $("ppWinDate").addEventListener("change", e => { if (e.target.value) load(e.target.value); });
            const del = $("ppDayDelete");
            del.classList.toggle("d-none", !existing);
            del.onclick = async () => {
                const ok = await uiMessages.showConfirm(tr("pp_delete_day_confirm", { date: nice }), { danger: true });
                if (!ok) return;
                try {
                    await api("DELETE", `/plant/${P.id}/day/${d}`);
                    const day = dayByDate(d);
                    if (day && day.photos.length) Object.assign(day, { types: [], note: "", measurements: [], rows: [], has_entry: false });
                    else DAYS = DAYS.filter(x => x.date !== d);
                    bootstrap.Modal.getInstance(el).hide();
                    toast(tr("pp_day_deleted"));
                    renderRows();
                } catch (e) { showError(el, e.message); }
            };
            $("ppDaySave").onclick = async () => {
                const body = readWindow(el);
                if (!body) return;
                const target = $("ppWinDate").value || d;
                if (!body.activity_ids.length && !body.note && !dayByDate(target)) { showError(el, tr("pp_pick_activity")); return; }
                $("ppDaySave").disabled = true;
                try {
                    const res = await api("PUT", `/plant/${P.id}/day/${target}`, body);
                    const photos = (dayByDate(target) || {}).photos || [];
                    DAYS = DAYS.filter(x => x.date !== target);
                    if (res.day) DAYS.push(res.day);
                    else if (photos.length) DAYS.push({ plant_id: P.id, date: target, types: [], note: "", has_entry: false, measurements: [], photos, rows: [] });
                    DAYS.sort((a, b) => b.date.localeCompare(a.date));
                    bootstrap.Modal.getInstance(el).hide();
                    toast(tr("pp_saved"));
                    filter.types.clear();
                    selected = target;
                    renderRows();
                    select(target, { scroll: true });
                } catch (e) { showError(el, e.message); } finally { $("ppDaySave").disabled = false; }
            };
        };
        load(date || TODAY);
        bootstrap.Modal.getOrCreateInstance(el).show();
    }

    // =====================================================================
    // All plants mode
    // =====================================================================
    const AS = { loaded: false, meta: [], plants: new Set(), q: "", days: [] };
    let AROWS = [];
    const metaOf = id => AS.meta.find(p => p.id === +id);
    const growingIds = () => AS.meta.filter(p => p.active).map(p => p.id);
    const plantsDefault = () => { const g = growingIds(); return AS.plants.size === g.length && g.every(id => AS.plants.has(id)); };
    const zones = () => [...new Set(AS.meta.map(p => p.zone))];

    async function loadAll(ids) {
        const q = ids ? `?plant_ids=${ids.join(",")}` : "";
        const data = await api("GET", "/notebook" + q);
        AS.meta = data.plants || [];
        AS.days = data.days || [];
        if (!AS.loaded) { AS.plants = new Set(growingIds()); AS.loaded = true; }
    }
    let reloadTimer = null;
    function reloadAll() {
        clearTimeout(reloadTimer);
        reloadTimer = setTimeout(async () => {
            try { await loadAll(AS.plants.size ? [...AS.plants] : [0]); } catch (e) { toast(e.message, "danger"); }
            renderAllHero();
            renderAllRows();
        }, 150);
    }

    function aStage(p, iso) {
        const s = stageOn(p.history || [], iso);
        const dg = p.start ? daysBetween(p.start, iso) + 1 : null;
        return { txt: [s ? `${s.label} · ${tr("pp_wk")} ${s.week}` : "", dg != null ? `${tr("pp_day")} ${dg}` : ""].filter(Boolean).join(" · "), color: s ? s.color : "#94a3b8" };
    }
    const plantLink = p => `<a href="/plant/${p.id}" data-plant-link="${p.id}" title="${esc(tr("pp_open_plant", { plant: p.name }))}">${esc(p.name)}</a>`;

    // Rows logged for several plants at once (same day, ticks, note and
    // numbers) are merged into one row.
    function groupRows() {
        const map = new Map();
        AS.days.filter(d => AS.plants.has(d.plant_id)).forEach(d => {
            const content = d.types.length || d.note || d.measurements.length;
            const sig = d.types.map(t => t.activity_id + "x" + t.count).sort().join(",") + "|" + d.measurements.map(m => m.metric_id + "=" + m.value).sort().join(",");
            const key = content ? [d.date, sig, d.note].join("|") : `${d.date}|photos|${d.plant_id}`;
            if (!map.has(key)) map.set(key, { key, date: d.date, types: d.types, note: d.note, measurements: d.measurements, items: [] });
            map.get(key).items.push(d);
        });
        return [...map.values()].map(r => ({
            ...r,
            plants: r.items.map(d => metaOf(d.plant_id) || { id: d.plant_id, name: "#" + d.plant_id, history: [] }).sort((a, b) => a.name.localeCompare(b.name)),
            photos: r.items.flatMap(d => d.photos.map(p => ({ ...p, plant: d.plant_id }))),
        })).sort((a, b) => b.date.localeCompare(a.date) || a.plants[0].name.localeCompare(b.plants[0].name));
    }
    function aMatches(r) {
        if (!matchesTypes({ types: r.types, photos: r.photos })) return false;
        const q = AS.q.trim().toLowerCase();
        return !q || (r.note || "").toLowerCase().includes(q) || r.plants.some(p => p.name.toLowerCase().includes(q));
    }
    function hl(text) {
        const q = AS.q.trim(), out = esc(text);
        return q ? out.replace(new RegExp(esc(q).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi"), m => `<mark>${m}</mark>`) : out;
    }
    function aWho(r) {
        const lines = r.plants.map(p => aStage(p, r.date));
        if (lines.every(l => l.txt === lines[0].txt)) return r.plants.map(plantLink).join("") + `<span class="pp-stgt" style="--c:${lines[0].color}">${esc(lines[0].txt)}</span>`;
        return r.plants.map((p, i) => plantLink(p) + `<span class="pp-stgt" style="--c:${lines[i].color}">${esc(lines[i].txt)}</span>`).join("");
    }
    function aRowHTML(r, i) {
        return `<div class="pp-row all ${selected === r.key ? "is-selected" : ""}" data-k="${esc(r.key)}" data-i="${i}" style="--sc:${aStage(r.plants[0], r.date).color}">
            <div class="dd">${esc(fmt(r.date))}<small>${esc(fmt(r.date, { weekday: "short" }))}</small></div>
            <div class="who">${aWho(r)}</div>
            <div class="ics">${iconsHTML({ types: r.types, photos: r.photos }, `data-acam="${i}"`)}</div>
            <div class="tx">${textHTML({ types: r.types, note: r.note, measurements: r.measurements }, hl)}</div>
            ${LOGGED_IN ? `<button class="edit" data-aedit="${i}" title="${esc(tr("pp_edit_day"))}"><i class="fa-solid fa-pen-to-square me-1"></i>${esc(tr("pp_edit"))}</button>` : "<span></span>"}
        </div>`;
    }
    function renderAllRows() {
        AROWS = groupRows().filter(aMatches);
        renderChips(AROWS.length);
        const el = $("ppRows");
        if (!AS.loaded) { el.innerHTML = `<div class="pp-empty"><span class="spinner-border spinner-border-sm"></span></div>`; return; }
        if (!AROWS.length) {
            el.innerHTML = `<div class="pp-empty"><i class="fa-solid fa-filter-circle-xmark"></i>${esc(tr("pp_no_entries"))}${AS.plants.size ? "" : " " + esc(tr("pp_pick_plant"))}</div>`;
            $("ppPanel").innerHTML = "";
            return;
        }
        if (!AROWS.some(r => r.key === selected)) selected = AROWS[0].key;
        let month = "";
        el.innerHTML = `<div class="pp-list-b">${AROWS.map((r, i) => {
            const m = asDate(r.date).toLocaleDateString(LANG, { month: "long", year: "numeric" });
            return (m !== month ? `<div class="pp-month">${esc(month = m)}</div>` : "") + aRowHTML(r, i);
        }).join("")}</div>`;
        renderAllPanel();
    }
    function renderAllHero() {
        const hero = $("ppHero");
        hero.classList.add("all");
        const grow = AS.meta.filter(p => p.active);
        hero.innerHTML = `<div class="pp-allh">
            <div class="ttl"><h1>${esc(tr("pp_all_plants"))}</h1>${AS.loaded ? `<span class="pp-sub">${esc(tr("pp_all_sub", { n: grow.length, z: new Set(grow.map(p => p.zone)).size }))}</span>` : ""}</div>
            <div class="pp-pchips">${grow.map(p => {
                const s = stageOn(p.history || [], TODAY);
                const dg = p.start ? daysBetween(p.start, TODAY) + 1 : null;
                return `<a class="pp-pchip ${AS.plants.has(p.id) ? "" : "off"}" href="/plant/${p.id}" data-plant-link="${p.id}" title="${esc(tr("pp_open_plant", { plant: p.name }))}" style="--sc:${s ? s.color : "#94a3b8"}">
                    <span class="av" style="${p.photo ? `background-image:url('${esc(p.photo)}')` : ""}">${p.photo ? "" : `<i class="fa-solid fa-seedling"></i>`}</span>
                    <span><b>${esc(p.name)}</b><small>${s ? `${esc(s.label)} · ${esc(tr("pp_wk"))} ${s.week}` : ""} <span>${dg != null ? `· ${esc(tr("pp_day"))} ${dg}` : ""}${p.zone ? ` · ${esc(p.zone)}` : ""}</span></small></span></a>`;
            }).join("")}</div></div>`;
    }
    function allPlantPop() {
        const fin = AS.meta.filter(p => !p.active);
        return `<h6>${esc(tr("pp_plants"))}</h6><div class="quick"><button data-apsel="growing">${esc(tr("pp_all_growing"))}</button><button data-apsel="all">${esc(tr("pp_everything"))}</button></div>` +
            zones().map(z => {
                const ps = AS.meta.filter(p => p.zone === z && p.active);
                if (!ps.length) return "";
                return (ps.length > 1 ? `<label class="tent"><input type="checkbox" class="form-check-input m-0" data-aptent="${esc(z)}" ${ps.every(p => AS.plants.has(p.id)) ? "checked" : ""}>${esc(z || "—")}<small>${esc(tr("pp_whole_tent"))}</small></label>` : "") +
                    ps.map(p => `<label class="${ps.length > 1 ? "sub" : ""}"><input type="checkbox" class="form-check-input m-0" data-apid="${p.id}" ${AS.plants.has(p.id) ? "checked" : ""}>${esc(p.name)}${ps.length > 1 ? "" : `<small>${esc(z)}</small>`}</label>`).join("");
            }).join("") +
            (fin.length ? `<h6 class="mt-2">${esc(tr("pp_finished"))}</h6>` + fin.map(p => `<label><input type="checkbox" class="form-check-input m-0" data-apid="${p.id}" ${AS.plants.has(p.id) ? "checked" : ""}>${esc(p.name)}<small>${esc(statusLabel(p.status))}</small></label>`).join("") : "") + `<hr>`;
    }
    function renderAllPanel() {
        const el = $("ppPanel");
        const r = AROWS.find(x => x.key === selected);
        if (!r) { el.innerHTML = ""; return; }
        const n = r.photos.length;
        let k = 0;
        const i = AROWS.indexOf(r);
        const photos = r.items.filter(d => d.photos.length).map(d => (r.plants.length > 1 ? `<div class="pp-pcap"><i class="fa-solid fa-camera me-1"></i>${esc((metaOf(d.plant_id) || {}).name || "")}</div>` : "") +
            d.photos.map(p => `<img src="${esc(p.path)}" data-aphoto="${i}:${k++}" alt="${esc(p.description)}" loading="lazy">`).join("")).join("");
        el.innerHTML = `<div class="pp-ptab"><i class="fa-regular fa-calendar"></i>${esc(fmt(r.date, { weekday: "short", month: "short", day: "numeric" }))}${n > 1 ? " · " + esc(tr("pp_n_photos", { n })) : ""}</div>
            <div class="pp-pbody"><div class="pp-pplants">${r.plants.map(p => { const s = aStage(p, r.date); return `<div class="pp-pplant"><span class="pp-stgt" style="--c:${s.color}"></span>${plantLink(p)}<small>${esc(s.txt)}</small></div>`; }).join("")}</div>
                ${n ? photos : `<div class="pp-pempty" style="height:auto"><div><i class="fa-regular fa-image big"></i>${esc(tr("pp_no_photos_day"))}</div></div>`}
                ${r.plants.length > 1 ? `<div class="pp-pnote"><i class="fa-solid fa-link me-1"></i>${esc(tr("pp_logged_for_n", { n: r.plants.length }))}</div>` : ""}</div>`;
    }

    // Log activity for one or several plants (append), or edit a row: one
    // plant → that plant's day; several → untick plants to change it for the
    // others only (each ticked plant's day is set to the new content).
    function openAllLog(row) {
        const el = dayWindow();
        const editing = !!row;
        const grow = AS.meta.filter(p => p.active);
        const list = editing ? row.plants : grow;
        const picks = new Set(editing ? row.plants.map(p => p.id) : []);
        const ticks = new Set(editing ? row.types.map(t => t.activity_id) : []);
        const values = {};
        if (editing) row.measurements.forEach(m => { values[m.activity_id + ":" + m.metric_id] = num(m.value); });
        const tentLinks = editing ? "" : `<div class="mb-2 d-flex gap-3 flex-wrap">${zones().filter(z => grow.filter(p => p.zone === z).length > 1).map(z => `<button type="button" class="pp-link" data-wtent="${esc(z)}">${esc(tr("pp_everything_in", { zone: z }))}</button>`).join("")}</div>`;
        $("ppDayTitle").innerHTML = editing ? `<i class="fa-solid fa-pen-to-square me-2"></i>${esc(tr("pp_edit_title", { date: fmt(row.date) }))}` : `<i class="fa-solid fa-plus me-2"></i>${esc(tr("pp_log_activity"))}`;
        $("ppDayBody").innerHTML = windowBody({
            date: editing ? row.date : TODAY, ticks, values, note: editing ? row.note : "", plants: list, plantPicks: picks, showDate: !editing, tentLinks,
            plantHint: editing && row.plants.length > 1 ? `<div class="small mb-2 p-2 rounded" style="background:var(--pp-acc-soft)"><i class="fa-solid fa-link me-1"></i>${esc(tr("pp_untick_hint", { n: row.plants.length }))}</div>` : "",
            footnote: editing ? "" : esc(tr("pp_logging_several")),
        });
        wireWindow(el, values);
        el.querySelectorAll("[data-wtent]").forEach(b => b.addEventListener("click", () => el.querySelectorAll("#ppWinPlants .pp-tchip").forEach(c => c.classList.toggle("on", (metaOf(c.dataset.pid) || {}).zone === b.dataset.wtent))));
        const del = $("ppDayDelete");
        del.classList.add("d-none");
        $("ppDaySave").onclick = async () => {
            const body = readWindow(el);
            if (!body) return;
            const ids = [...el.querySelectorAll("#ppWinPlants .on")].map(c => +c.dataset.pid);
            if (!ids.length) { showError(el, tr("pp_pick_plant")); return; }
            if (!body.activity_ids.length && !body.note) { showError(el, tr("pp_pick_activity")); return; }
            $("ppDaySave").disabled = true;
            try {
                if (editing) {
                    for (const id of ids) await api("PUT", `/plant/${id}/day/${row.date}`, body);
                } else {
                    await api("POST", "/plants/day", { ...body, plant_ids: ids, date: $("ppWinDate").value || TODAY });
                    ids.forEach(id => AS.plants.add(id));
                }
                bootstrap.Modal.getInstance(el).hide();
                toast(tr("pp_saved"));
                await loadAll([...AS.plants]);
                if (ids.includes(P.id)) refreshThisPlant();
                renderAllHero();
                renderAllRows();
            } catch (e) { showError(el, e.message); } finally { $("ppDaySave").disabled = false; }
        };
        bootstrap.Modal.getOrCreateInstance(el).show();
    }
    // Keep this plant's own notebook in step after edits in All plants mode.
    async function refreshThisPlant() {
        try { DAYS = (await api("GET", `/plant/${P.id}/notebook`)).days || []; } catch (e) { /* shown next load */ }
    }

    async function setMode(all) {
        allMode = all;
        history.replaceState(null, "", all ? "#all" : location.pathname + location.search);
        $("ppFilterPop").classList.remove("open");
        selected = null;
        syncToolbar();
        if (all && !AS.loaded) {
            renderAllHero();
            renderAllRows();
            try { await loadAll(); } catch (e) { toast(e.message, "danger"); }
        }
        if (!all) refreshThisPlant().then(() => { if (!allMode) renderRows(); });
        renderHero();
        renderRows();
        $("ppScroll").scrollTop = 0;
        $("ppHero").classList.remove("slim");
        fitHeight();
    }

    // =====================================================================
    // Events
    // =====================================================================
    function onClick(e) {
        const t = e.target;
        const pop = $("ppFilterPop"), xpop = $("ppExportPop");
        const md = t.closest("[data-mode]");
        if (md) { if ((md.dataset.mode === "all") !== allMode) setMode(md.dataset.mode === "all"); return; }
        const vw = t.closest("[data-view]");
        if (vw) { view = vw.dataset.view; store.set("pp-view", view); syncToolbar(); renderRows(); return; }

        // Filter popup
        if (t.closest("#ppFilterBtn")) { e.stopPropagation(); renderFilterPop(); pop.classList.toggle("open"); if (xpop) xpop.classList.remove("open"); return; }
        if (t.closest("#ppExportBtn")) { renderExportPop(); xpop.classList.toggle("open"); pop.classList.remove("open"); return; }
        const ft = t.closest("[data-ftype]");
        if (ft) { const k = ft.dataset.ftype; filter.types.has(k) ? filter.types.delete(k) : filter.types.add(k); renderRows(); return; }
        const fm = t.closest("[data-fmode]");
        if (fm) { filter.mode = fm.dataset.fmode; renderFilterPop(); renderRows(); return; }
        const ap = t.closest("[data-apid]");
        if (ap) { const id = +ap.dataset.apid; AS.plants.has(id) ? AS.plants.delete(id) : AS.plants.add(id); renderFilterPop(); reloadAll(); return; }
        const at = t.closest("[data-aptent]");
        if (at) { const ps = AS.meta.filter(p => p.zone === at.dataset.aptent && p.active); const all = ps.every(p => AS.plants.has(p.id)); ps.forEach(p => all ? AS.plants.delete(p.id) : AS.plants.add(p.id)); renderFilterPop(); reloadAll(); return; }
        const as = t.closest("[data-apsel]");
        if (as) { AS.plants = new Set(AS.meta.filter(p => as.dataset.apsel === "all" || p.active).map(p => p.id)); renderFilterPop(); reloadAll(); return; }
        if (t.closest("#ppFilterPop")) return;
        if (pop && !t.closest(".pp-popwrap")) pop.classList.remove("open");
        if (xpop && !t.closest(".pp-popwrap")) xpop.classList.remove("open");
        const uf = t.closest("[data-unfilter]");
        if (uf) { filter.types.delete(uf.dataset.unfilter); renderRows(); return; }
        if (t.closest("[data-unsearch]")) { AS.q = ""; $("ppSearch").value = ""; renderRows(); return; }
        if (t.closest("[data-reset-plants]")) { AS.plants = new Set(growingIds()); reloadAll(); return; }
        if (t.closest("[data-clear-filter]")) {
            filter.types.clear();
            if (allMode) { AS.q = ""; $("ppSearch").value = ""; if (!plantsDefault()) { AS.plants = new Set(growingIds()); reloadAll(); return; } }
            renderRows();
            return;
        }

        // Header
        const node = t.closest("[data-stage]");
        if (node) { onStageClick(+node.dataset.stage); return; }
        if (t.closest("#ppKill")) { const dead = (D.statuses || []).find(s => s.status.toLowerCase() === "dead"); if (dead) openStageMove(dead.id); return; }
        const src = t.closest("[data-src]");
        if (src) { setSource(src.dataset.src); return; }
        if (t.closest("#ppLivePlay")) { playLive(); return; }

        // Toolbar
        if (t.closest("#ppGalleryBtn")) { openGallery(); return; }
        if (t.closest("#ppAddBtn")) { allMode ? openAllLog() : openDay(); return; }
        if (t.closest("#ppUploadBtn") || t.closest("[data-upload]")) {
            const el = $("uploadImagesModal");
            if (el) bootstrap.Modal.getOrCreateInstance(el).show();
            return;
        }

        // Rows (this plant)
        const ed = t.closest("[data-edit]");
        if (ed) { openDay(ed.dataset.edit); return; }
        const cam = t.closest("[data-cam]");
        if (cam) { e.stopPropagation(); const day = dayByDate(cam.dataset.cam); if (isDesktop()) select(day.date); else openDayPhotos(day); return; }
        const ph = t.closest("[data-open-photo]");
        if (ph) { openDayPhotos(dayByDate(ph.dataset.day), +ph.dataset.openPhoto); return; }

        // Rows (all plants)
        const pl = t.closest("[data-plant-link]");
        if (pl) {
            if (+pl.dataset.plantLink === P.id) { e.preventDefault(); setMode(false); }
            return; // other plants: follow the link
        }
        const aed = t.closest("[data-aedit]");
        if (aed) {
            const r = AROWS[+aed.dataset.aedit];
            if (r.plants.length === 1 && r.plants[0].id === P.id) { setMode(false).then(() => openDay(r.date)); return; }
            openAllLog(r);
            return;
        }
        const acam = t.closest("[data-acam]");
        if (acam) { const r = AROWS[+acam.dataset.acam]; if (isDesktop()) select(r.key); else openViewer(r.photos.map(p => ({ ...p, date: r.date })), 0); return; }
        const aph = t.closest("[data-aphoto]");
        if (aph) { const [i, k] = aph.dataset.aphoto.split(":").map(Number); openViewer(AROWS[i].photos.map(p => ({ ...p, date: AROWS[i].date })), k); return; }

        // Click a row to select it (never by scrolling).
        const row = t.closest(".pp-row");
        if (row && !t.closest("a,button")) select(allMode ? row.dataset.k : row.dataset.date);
    }

    // Desktop: the notebook and photo panel fill the rest of the window;
    // only the notebook scrolls, and the header slims while it does.
    function fitHeight() {
        const main = $("ppMain");
        if (!main) return;
        if (!isDesktop()) { main.style.removeProperty("--pp-main-h"); return; }
        const top = main.getBoundingClientRect().top + window.scrollY;
        main.style.setProperty("--pp-main-h", Math.max(420, window.innerHeight - top - 16) + "px");
    }

    function init() {
        renderToolbar();
        renderHero();
        renderRows();
        if (D.stream) setSource("photo");
        document.addEventListener("click", onClick);
        $("ppSearch").addEventListener("input", e => { AS.q = e.target.value; renderRows(); });
        $("ppScroll").addEventListener("scroll", e => {
            if (!isDesktop()) return;
            $("ppHero").classList.toggle("slim", e.target.scrollTop > 40);
        }, { passive: true });
        $("ppHero").addEventListener("transitionend", fitHeight);
        window.addEventListener("resize", fitHeight);
        fitHeight();
        // Arrow keys move between plants (not while typing or in a window).
        document.addEventListener("keydown", e => {
            if (document.querySelector(".modal.show")) return;
            const tag = document.activeElement && document.activeElement.tagName;
            if (["INPUT", "TEXTAREA", "SELECT"].includes(tag) || (document.activeElement && document.activeElement.isContentEditable)) return;
            if (e.key === "ArrowLeft" && D.prev) location.href = "/plant/" + D.prev;
            if (e.key === "ArrowRight" && D.next) location.href = "/plant/" + D.next;
        });
        window.isleyUploadDefaultDate = () => selected || TODAY;
        if (allMode) { allMode = false; setMode(true); }
        // Links from the Plants page: #log opens Add activity, #upload the
        // photo upload window.
        if (LOGGED_IN && (location.hash === "#log" || location.hash === "#upload")) {
            const want = location.hash;
            history.replaceState(null, "", location.pathname + location.search);
            if (want === "#log") openDay();
            else { const el = $("uploadImagesModal"); if (el) bootstrap.Modal.getOrCreateInstance(el).show(); }
        }
        // A link to this page's #all (or back) switches without a reload.
        window.addEventListener("hashchange", () => { if ((location.hash === "#all") !== allMode) setMode(location.hash === "#all"); });
    }
    init();
})();
