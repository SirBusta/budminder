// Home / Today page (views/index.html), mockup H1: what needs attention,
// each tent with its readings, camera tile and plant cards, recent
// activity, and the Log activity window (which asks for measurements such
// as the water amount). Data: #homePage[data-page], refreshed from
// GET /home/data every polling interval.
(function () {
    "use strict";
    const root = document.getElementById("homePage");
    if (!root) return;
    let D = JSON.parse(root.dataset.page || "{}");
    const T = D.t || {};
    const t = (k, vars) => {
        let s = T[k] || k;
        if (vars) for (const [a, b] of Object.entries(vars)) s = s.split("{" + a + "}").join(b);
        return s;
    };
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const K = window.SensorKinds;
    const LOGGED_IN = !!D.logged_in;
    const locale = D.lang || undefined;
    const $ = id => document.getElementById(id);
    const toast = (msg, level) => window.uiMessages ? uiMessages.showToast(msg, level || "success") : alert(msg);

    // Days without water / feed before "due" (amber) and "overdue" (red);
    // the same as the Plants page.
    const DUE = { water: [3, 6], feed: [10, 18] };
    const PHOTO_DAYS = 14;
    const STAGE_LOOK = {
        Germinating: ["fa-seedling", "#a3b18a"], Planted: ["fa-bucket", "#8fbc5a"], Seedling: ["fa-leaf", "#84cc16"],
        Veg: ["fa-tree", "#16a34a"], Flower: ["fa-cannabis", "#9333ea"], Drying: ["fa-wind", "#d97706"],
        Curing: ["fa-jar", "#92400e"], Success: ["fa-award", "#0284c7"], Dead: ["fa-skull-crossbones", "#64748b"],
    };

    // ---------- Dates ----------
    const days = (a, b) => Math.round((new Date(b + "T12:00:00") - new Date(a + "T12:00:00")) / 864e5);
    const fmtDate = (iso, opts) => new Date(iso + "T12:00:00").toLocaleDateString(locale, opts);
    function dayLabel(iso) {
        const n = days(iso, D.today);
        return n === 0 ? t("hm_today") : n === 1 ? t("hm_yesterday") : fmtDate(iso, { weekday: "short", month: "short", day: "numeric" });
    }
    function ago(ms) {
        const m = Math.max(0, Math.round((Date.now() - ms) / 60000));
        if (m < 1) return t("hm_just_now");
        if (m < 60) return t("hm_min_ago", { n: m });
        if (m < 48 * 60) return t("hm_h_ago", { n: Math.round(m / 60) });
        return t("hm_d_ago", { n: Math.round(m / 1440) });
    }

    // ---------- Derived plant facts ----------
    let plants = [], byId = {};
    const stageLabel = name => ((D.stages || []).find(s => s.name === name) || {}).label || name;
    function derive() {
        plants = (D.plants || []).map(p => ({
            ...p,
            day: p.start ? days(p.start, D.today) + 1 : null,
            stageWeek: p.stage_since ? Math.ceil((days(p.stage_since, D.today) + 1) / 7) : null,
            water: p.last_water ? days(p.last_water, D.today) : null,
            feed: p.last_feed ? days(p.last_feed, D.today) : null,
            photoDays: p.photo_date ? days(p.photo_date, D.today) : null,
            harvest: p.est_max ? [days(D.today, p.est_min || p.est_max), days(D.today, p.est_max)] : null,
        }));
        byId = Object.fromEntries(plants.map(p => [p.id, p]));
    }
    const level = (kind, d) => d == null ? "none" : d >= DUE[kind][1] ? "over" : d >= DUE[kind][0] ? "due" : "ok";
    const types = () => D.types || [];
    const waterType = () => types().find(a => a.is_watering && !a.is_feeding) || types().find(a => a.is_watering);
    const feedType = () => types().find(a => a.is_feeding);

    // ---------- Building blocks ----------
    function care(kind, d) {
        const icon = kind === "water" ? "fa-droplet" : "fa-flask";
        const txt = d == null ? t(kind === "water" ? "hm_not_watered" : "hm_not_fed")
            : d === 0 ? t(kind === "water" ? "hm_watered_today" : "hm_fed_today")
            : d === 1 ? t(kind === "water" ? "hm_watered_yesterday" : "hm_fed_yesterday")
            : t(kind === "water" ? "hm_watered_days" : "hm_fed_days", { n: d });
        return `<span class="hm-care ${level(kind, d)}"><i class="fa-solid ${icon}"></i>${esc(txt)}</span>`;
    }
    function stageBadge(p) {
        const [, color] = STAGE_LOOK[p.stage] || ["", "#64748b"];
        return `<span class="hm-stage" style="--c:${color}">${esc(stageLabel(p.stage))}${p.stageWeek ? " · " + esc(t("hm_week", { n: p.stageWeek })) : ""}</span>`;
    }
    function harvest(p) {
        if (p.harvest) {
            if (p.harvest[1] <= 0) return `<span class="hm-harvest"><i class="fa-solid fa-scissors"></i>${esc(t("hm_harvest_now"))}</span>`;
            const a = Math.max(0, Math.round(p.harvest[0] / 7)), b = Math.round(p.harvest[1] / 7);
            return `<span class="hm-harvest"><i class="fa-solid fa-scissors"></i>${esc(a === b ? t("hm_harvest_in_one", { n: b }) : t("hm_harvest_in", { a, b }))}</span>`;
        }
        return p.stage === "Flower" ? `<span class="hm-harvest muted" title="${esc(t("hm_harvest_not_set_help"))}"><i class="fa-solid fa-scissors"></i>${esc(t("hm_harvest_not_set"))}</span>` : "";
    }
    function typeIcons(list) {
        return (list || []).map(ty => `<span class="hm-ico" style="--c:${esc(ty.color || "#64748b")}" title="${esc(ty.name)}"><i class="fa-solid ${esc(ty.icon || "fa-tag")}"></i></span>`).join("");
    }
    const num = v => Number(v).toLocaleString(locale, { maximumFractionDigits: 2 });

    // ---------- Needs attention ----------
    function alerts() {
        const out = [];
        const name = p => `<b>${esc(p.name)}</b>`;
        plants.forEach(p => {
            const lw = level("water", p.water);
            if (lw === "over" || lw === "due") out.push({ lv: lw === "over" ? "over" : "warn", icon: "fa-droplet", txt: t("hm_alert_water", { plant: "#P#", n: p.water }).replace("#P#", name(p)), act: `<button class="hm-btn-s primary" data-log="${p.id}" data-kind="water"><i class="fa-solid fa-droplet me-1"></i>${esc(t("hm_watered_it"))}</button>` });
            const lf = level("feed", p.feed);
            if (lf === "over" || lf === "due") out.push({ lv: lf === "over" ? "over" : "warn", icon: "fa-flask", txt: t("hm_alert_feed", { plant: "#P#", n: p.feed }).replace("#P#", name(p)), act: `<button class="hm-btn-s primary" data-log="${p.id}" data-kind="feed"><i class="fa-solid fa-flask me-1"></i>${esc(t("hm_fed_it"))}</button>` });
        });
        // Sensors that stopped reporting: one line per tent.
        (D.tents || []).forEach(tent => {
            const stale = [...tent.readings, ...tent.ports].filter(r => r.stale && r.last_at);
            if (!stale.length) return;
            const oldest = Math.min(...stale.map(r => r.last_at)), where = tent.name || t("hm_no_tent");
            const txt = stale.length === 1 ? t("hm_alert_stale", { sensor: stale[0].name, tent: where, ago: ago(oldest) })
                : t("hm_alert_stale_n", { n: stale.length, tent: where, ago: ago(oldest) });
            out.push({ lv: "warn", icon: "fa-satellite-dish", txt: esc(txt), act: `<a class="hm-link" href="/sensors/manage">${esc(t("hm_check_sensors"))}</a>` });
        });
        if (D.old_seed_batches > 0) out.push({ lv: "warn", icon: "fa-vault", cls: "vault", txt: esc(t(D.old_seed_batches === 1 ? "hm_alert_seeds_one" : "hm_alert_seeds", { n: D.old_seed_batches })), act: `<a class="hm-link" href="/strains#vault">${esc(t("hm_open_vault"))}</a>` });
        plants.forEach(p => {
            const none = p.photoDays == null && p.day >= PHOTO_DAYS, old = p.photoDays != null && p.photoDays >= PHOTO_DAYS;
            if (none || old) out.push({ lv: "info", icon: "fa-camera", txt: (none ? t("hm_alert_no_photo", { plant: "#P#" }) : t("hm_alert_photo", { plant: "#P#", n: p.photoDays })).replace("#P#", name(p)), act: `<button class="hm-btn-s" data-photos="${p.id}"><i class="fa-solid fa-camera me-1"></i>${esc(t("hm_add_photo"))}</button>` });
        });
        return out;
    }
    function renderAlerts() {
        const box = $("hmAlerts");
        if (!box) return;
        const list = alerts();
        $("hmAlertCount").textContent = list.length ? String(list.length) : t("hm_all_clear");
        box.innerHTML = list.map(a => `<div class="hm-alert ${a.lv}"><span class="ai ${a.cls || ""}"><i class="fa-solid ${a.icon}"></i></span><span class="tx">${a.txt}</span><span class="acts">${a.act}</span></div>`).join("")
            || `<div class="hm-alert info"><span class="ai"><i class="fa-solid fa-check"></i></span><span class="tx">${esc(t("hm_nothing"))}</span></div>`;
    }

    // ---------- Tents ----------
    const KIND_LABEL = { temp: "hm_temp", hum: "hm_rh", vpd: "hm_vpd", co2: "hm_co2", soil: "hm_soil" };
    function envLine(tent) {
        const shown = {};
        tent.readings.forEach(r => { const k = K.kind(r); if (KIND_LABEL[k] && !shown[k] && r.value != null) shown[k] = r; });
        const parts = ["temp", "hum", "vpd", "co2", "soil"].filter(k => shown[k]).map(k => {
            const r = shown[k], { unit, conv } = K.display(r, D.units), dp = K.decimals(r);
            const v = Number(conv(r.value)).toLocaleString(locale, { minimumFractionDigits: dp, maximumFractionDigits: dp });
            const u = unit === "ppm" ? "" : /^[°%]/.test(unit) ? unit : unit ? " " + unit : "";
            return `<span><small>${esc(t(KIND_LABEL[k]))}</small> <b>${v}</b>${esc(u)}</span>`;
        });
        const light = tent.ports.find(p => K.portIcon(p.name) === "fa-lightbulb" && p.value != null);
        if (light) parts.push(`<span class="sep"></span><span class="hm-light ${light.value > 0 ? "on" : "off"}"><i class="fa-solid fa-lightbulb me-1"></i>${esc(t(light.value > 0 ? "hm_lights_on" : "hm_lights_off"))}</span>`);
        if (!parts.length) return tent.id ? `<span class="hm-empty-t">${esc(t("hm_no_sensors"))} · <a class="hm-link" href="/sensors/manage">${esc(t("hm_add_sensors"))}</a></span>` : "";
        return `<div class="hm-env">${parts.join("")}</div>`;
    }
    const isHls = url => /\.m3u8($|\?)/i.test(url || "");
    function cameraTile(cam) {
        const canLive = isHls(cam.url) && !!window.videojs;
        return `<div class="hm-cam" id="hmCam${cam.id}" data-cam="${cam.id}">
            ${cam.snapshot ? `<img src="${esc(cam.snapshot)}?t=${cam.snap_at}" alt="">` : `<div class="hm-cam-none"><i class="fa-solid fa-video"></i>${esc(t("hm_no_snapshot"))}</div>`}
            <div class="top"><span class="nm"><i class="fa-solid fa-video me-1"></i>${esc(cam.name)}</span><span class="badge-l"><i class="fa-solid fa-circle"></i>${esc(t("hm_live_badge"))}</span></div>
            <div class="bot"><span class="when">${cam.snapshot ? esc(t("hm_snapshot_ago", { ago: ago(cam.snap_at) })) : ""}</span>
                ${canLive ? `<button type="button" class="play" data-live="${cam.id}"><i class="fa-solid fa-play me-1"></i>${esc(t("hm_live"))}</button>` : ""}</div></div>`;
    }
    function plantCard(p) {
        const ph = p.photo ? `<div class="ph" style="background-image:url('${esc(p.photo)}')">` : `<div class="ph none"><span><i class="fa-regular fa-image me-1"></i>${esc(t("hm_no_photos"))}</span>`;
        const water = waterType(), feed = feedType();
        return `<div class="hm-card">
            ${ph}${p.day ? `<span class="day">${esc(t("hm_day", { n: p.day }))}</span>` : ""}
                ${LOGGED_IN ? `<button type="button" class="addph" data-photos="${p.id}" title="${esc(t("hm_add_photos"))}" aria-label="${esc(t("hm_add_photos"))}"><i class="fa-solid fa-camera"></i></button>` : ""}</div>
            <div class="bd">
                <div class="row1"><a class="nm" href="/plant/${p.id}">${esc(p.name)}</a>${stageBadge(p)}</div>
                <div class="sub">${esc(p.strain)}${p.breeder ? " · " + esc(p.breeder) : ""}${p.autoflower ? " · " + esc(t("hm_auto")) : ""}</div>
                <div class="care">${care("water", p.water)}${care("feed", p.feed)}</div>
                ${harvest(p)}
                ${LOGGED_IN ? `<div class="ft">
                    ${water ? `<button type="button" class="hm-btn-s" data-log="${p.id}" data-kind="water"><i class="fa-solid fa-droplet me-1"></i>${esc(t("hm_water"))}</button>` : ""}
                    ${feed ? `<button type="button" class="hm-btn-s" data-log="${p.id}" data-kind="feed"><i class="fa-solid fa-flask me-1"></i>${esc(t("hm_feed"))}</button>` : ""}
                    <button type="button" class="hm-btn-s primary ms-auto" data-log="${p.id}"><i class="fa-solid fa-plus me-1"></i>${esc(t("hm_log"))}</button></div>` : ""}
            </div></div>`;
    }
    function renderTents() {
        const tents = D.tents || [];
        if (!tents.length) {
            $("hmTents").innerHTML = `<div class="hm-none"><i class="fa-solid fa-seedling"></i><p>${esc(t("hm_no_plants"))}</p>${LOGGED_IN ? `<a class="hm-btn" href="/plant/new"><i class="fa-solid fa-plus me-1"></i>${esc(t("hm_add_plant"))}</a>` : ""}</div>`;
            return;
        }
        $("hmTents").innerHTML = tents.map(tent => {
            const icon = /outdoor|outside/i.test(tent.name) ? "fa-tree" : tent.id ? "fa-tent" : "fa-circle-question";
            const charts = tent.id && (tent.readings.length || tent.ports.length) ? `<a class="hm-link" href="/sensors#zone=${tent.id}">${esc(t("hm_charts"))} <i class="fa-solid fa-chart-line ms-1"></i></a>` : "";
            return `<section class="hm-tent"><div class="hm-tent-h"><h2><i class="fa-solid ${icon}"></i>${esc(tent.name || t("hm_no_tent"))}</h2>${envLine(tent)}${charts}</div>
                <div class="hm-tent-b"><div class="hm-grid">${tent.cameras.map(cameraTile).join("")}${tent.plants.map(id => byId[id]).filter(Boolean).map(plantCard).join("")}</div></div></section>`;
        }).join("");
    }

    // ---------- Recent activity ----------
    function measurementText(m) {
        const water = /water/i.test(m.name);
        return water ? `<span class="amt"><i class="fa-solid fa-droplet me-1"></i>${num(m.value)} ${esc(m.unit)}</span>` : `<span class="msr">${esc(m.name)} ${num(m.value)}${m.unit ? " " + esc(m.unit) : ""}</span>`;
    }
    function renderRecent() {
        const list = D.recent || [];
        $("hmRecent").innerHTML = list.length ? list.map(r => {
            const who = r.plant_ids.map(id => byId[id] ? `<a href="/plant/${id}">${esc(byId[id].name)}</a>` : "").filter(Boolean).join(", ");
            const ms = (r.measurements || []).map(measurementText).join(" ");
            return `<div class="it"><span class="d">${esc(dayLabel(r.date))}</span><span class="ics">${typeIcons(r.types)}</span>
                <span><span class="p">${who}</span>${ms ? " · " + ms : ""}${r.note ? " — " + esc(r.note) : ""}</span></div>`;
        }).join("") : `<div class="it empty">${esc(t("hm_recent_none"))}</div>`;
    }

    function render() {
        derive();
        const spaces = (D.tents || []).filter(x => x.id).length;
        $("hmSub").textContent = `${fmtDate(D.today, { weekday: "long", month: "long", day: "numeric" })} · ${t(plants.length === 1 ? "hm_sub_one" : "hm_sub", { n: plants.length, m: spaces })}`;
        renderAlerts();
        renderTents();
        renderRecent();
    }

    // ---------- Camera: Live in the tile ----------
    let player = null, playingCam = null;
    function stopLive() {
        if (player) { try { player.dispose(); } catch (e) { /* gone */ } player = null; }
        playingCam = null;
    }
    function toggleLive(id) {
        const cam = (D.tents || []).flatMap(x => x.cameras).find(c => c.id === id);
        const tile = $("hmCam" + id);
        if (!cam || !tile) return;
        if (playingCam === id) { stopLive(); tile.outerHTML = cameraTile(cam); return; }
        if (playingCam != null) { const prev = (D.tents || []).flatMap(x => x.cameras).find(c => c.id === playingCam); stopLive(); const pt = $("hmCam" + prev.id); if (pt) pt.outerHTML = cameraTile(prev); }
        tile.classList.add("live");
        tile.insertAdjacentHTML("afterbegin", `<video id="hmVideo${id}" class="video-js" muted playsinline></video>`);
        player = window.videojs("hmVideo" + id, { autoplay: true, muted: true, controls: false, fill: true, sources: [{ src: cam.url, type: "application/x-mpegURL" }] });
        playingCam = id;
        const btn = tile.querySelector("[data-live]");
        if (btn) btn.innerHTML = `<i class="fa-solid fa-stop me-1"></i>${esc(t("hm_stop"))}`;
        const when = tile.querySelector(".when");
        if (when) when.textContent = "";
    }

    // ---------- Log activity window ----------
    let logSel = { plants: new Set(), types: new Set() };
    function metricsFor(typeIds) {
        const out = [], seen = new Set();
        types().filter(a => typeIds.has(a.id)).forEach(a => (a.metrics || []).forEach(l => {
            if (seen.has(l.metric_id)) return;
            const m = (D.metrics || []).find(x => x.id === l.metric_id);
            if (!m) return;
            seen.add(l.metric_id);
            out.push({ ...m, activity_id: a.id, required: l.required, care: a.is_watering || a.is_feeding });
        }));
        return out;
    }
    function drawMetrics() {
        const box = $("hmLogMetrics"), ms = metricsFor(logSel.types);
        const prev = {};
        box.querySelectorAll("input[data-metric]").forEach(i => { prev[i.dataset.metric] = i.value; });
        const one = logSel.plants.size === 1 ? byId[[...logSel.plants][0]] : null;
        box.innerHTML = ms.map(m => {
            let v = prev[m.id];
            const last = one && one.last ? one.last[m.id] : undefined;
            if (v == null && m.care && last != null) v = last;   // e.g. the last water amount
            return `<div class="hm-metric"><label class="form-label small mb-0" for="hmM${m.id}">${esc(m.name)}${m.required ? " *" : ""}</label>
                <div class="input-group input-group-sm"><input type="number" step="any" min="0" class="form-control" id="hmM${m.id}" data-metric="${m.id}" data-activity="${m.activity_id}" ${m.required ? "required" : ""} value="${v == null ? "" : esc(v)}">${m.unit ? `<span class="input-group-text">${esc(m.unit)}</span>` : ""}</div>
                ${m.care && last != null && one ? `<small class="text-muted">${esc(t("hm_last_time", { v: num(last) + (m.unit ? " " + m.unit : "") }))}</small>` : ""}</div>`;
        }).join("");
        box.classList.toggle("d-none", !ms.length);
    }
    function openLog(plantIds, typeIds) {
        logSel = { plants: new Set(plantIds), types: new Set(typeIds) };
        const tents = (D.tents || []).filter(x => x.id && x.plants.length > 1);
        $("hmLogBody").innerHTML = `
            <label class="form-label small fw-semibold mb-1">${esc(t("hm_plants"))}</label>
            <div class="hm-chips" id="hmLogPlants">${plants.map(p => `<button type="button" class="hm-chip ${logSel.plants.has(p.id) ? "on" : ""}" data-lp="${p.id}" aria-pressed="${logSel.plants.has(p.id)}"><i class="fa-solid fa-cannabis"></i>${esc(p.name)}</button>`).join("")}</div>
            ${tents.length ? `<div class="mb-3 mt-1">${tents.map(x => `<button type="button" class="hm-link hm-link-btn me-3" data-lt="${x.id}"><i class="fa-solid fa-check-double me-1"></i>${esc(t("hm_select_tent", { tent: x.name }))}</button>`).join("")}</div>` : `<div class="mb-3"></div>`}
            <div class="row g-2 mb-3"><div class="col-sm-5"><label class="form-label small mb-0" for="hmLogDate">${esc(t("hm_date"))}</label>
                <input type="date" class="form-control form-control-sm" id="hmLogDate" value="${D.today}" max="${D.today}"></div></div>
            <label class="form-label small fw-semibold mb-1">${esc(t("hm_activities"))}</label>
            <div class="hm-chips mb-2" id="hmLogTypes">${types().map(a => `<button type="button" class="hm-chip ${logSel.types.has(a.id) ? "on" : ""}" data-lty="${a.id}" style="--c:${esc(a.color || "#16a34a")}" aria-pressed="${logSel.types.has(a.id)}"><i class="fa-solid ${esc(a.icon || "fa-tag")}"></i>${esc(a.name)}</button>`).join("")}</div>
            <div class="hm-metrics d-none" id="hmLogMetrics"></div>
            <label class="form-label small mb-0" for="hmLogNote">${esc(t("hm_note"))}</label>
            <textarea class="form-control" id="hmLogNote" rows="3" placeholder="${esc(t("hm_note_placeholder"))}"></textarea>
            <p class="small text-muted mt-2 mb-0">${esc(t("hm_log_adds"))}</p>
            <div class="text-danger small mt-2 d-none" id="hmLogErr"></div>`;
        drawMetrics();
        bootstrap.Modal.getOrCreateInstance($("hmLog")).show();
        setTimeout(() => { const first = $("hmLogMetrics").querySelector("input"); if (first) first.focus(); }, 350);
    }
    async function saveLog() {
        const err = $("hmLogErr"), fail = msg => { err.textContent = msg; err.classList.remove("d-none"); };
        err.classList.add("d-none");
        if (!logSel.plants.size) return fail(t("hm_pick_plant"));
        const note = $("hmLogNote").value.trim();
        if (!logSel.types.size && !note) return fail(t("hm_pick_activity"));
        const measurements = [];
        for (const i of $("hmLogMetrics").querySelectorAll("input[data-metric]")) {
            if (i.value === "") { if (i.required) { i.focus(); return fail(t("hm_required_value")); } continue; }
            measurements.push({ activity_id: +i.dataset.activity, metric_id: +i.dataset.metric, value: Number(i.value) });
        }
        const btn = $("hmLogSave");
        btn.disabled = true;
        try {
            const resp = await fetch("/plants/day", {
                method: "POST", headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plant_ids: [...logSel.plants], date: $("hmLogDate").value || D.today, activity_ids: [...logSel.types], note, measurements }),
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.error || t("hm_save_failed"));
            bootstrap.Modal.getInstance($("hmLog")).hide();
            const names = [...logSel.plants].map(id => byId[id] && byId[id].name).filter(Boolean).join(", ");
            toast(t("hm_logged", { plants: names }));
            await refresh(true);
        } catch (e) {
            fail(e.message || t("hm_save_failed"));
        } finally { btn.disabled = false; }
    }

    // ---------- Add photos: the shared upload window with a plant picker ----------
    function openPhotos(pid) {
        const modal = $("uploadImagesModal"), hidden = $("plantId");
        if (!modal || !hidden || !plants.length) return;
        let pick = $("hmPhotoPlant");
        if (!pick) {
            modal.querySelector(".modal-body").insertAdjacentHTML("afterbegin", `<div class="mb-3"><label class="form-label small mb-0" for="hmPhotoPlant">${esc(t("hm_photo_plant"))}</label>
                <select class="form-select form-select-sm" id="hmPhotoPlant" style="max-width:320px"></select></div>`);
            pick = $("hmPhotoPlant");
            pick.addEventListener("change", () => { hidden.value = pick.value; });
        }
        pick.innerHTML = plants.map(p => `<option value="${p.id}">${esc(p.name)}</option>`).join("");
        pick.value = String(pid || plants[0].id);
        hidden.value = pick.value;
        bootstrap.Modal.getOrCreateInstance(modal).show();
    }

    // ---------- Events ----------
    document.addEventListener("click", e => {
        const lg = e.target.closest("[data-log]");
        if (lg) {
            const pid = lg.dataset.log ? [+lg.dataset.log] : [];
            const kind = lg.dataset.kind, ty = kind === "water" ? waterType() : kind === "feed" ? feedType() : null;
            openLog(pid, ty ? [ty.id] : []);
            return;
        }
        const ph = e.target.closest("[data-photos]");
        if (ph) { openPhotos(ph.dataset.photos ? +ph.dataset.photos : null); return; }
        const lv = e.target.closest("[data-live]");
        if (lv) { toggleLive(+lv.dataset.live); return; }
        const lp = e.target.closest("[data-lp]");
        if (lp) { const id = +lp.dataset.lp; logSel.plants.has(id) ? logSel.plants.delete(id) : logSel.plants.add(id); lp.classList.toggle("on"); lp.setAttribute("aria-pressed", lp.classList.contains("on")); drawMetrics(); return; }
        const lt = e.target.closest("[data-lt]");
        if (lt) {
            const tent = (D.tents || []).find(x => x.id === +lt.dataset.lt);
            logSel.plants = new Set(tent.plants);
            document.querySelectorAll("#hmLogPlants [data-lp]").forEach(c => { const on = logSel.plants.has(+c.dataset.lp); c.classList.toggle("on", on); c.setAttribute("aria-pressed", on); });
            drawMetrics();
            return;
        }
        const ty = e.target.closest("[data-lty]");
        if (ty) { const id = +ty.dataset.lty; logSel.types.has(id) ? logSel.types.delete(id) : logSel.types.add(id); ty.classList.toggle("on"); ty.setAttribute("aria-pressed", ty.classList.contains("on")); drawMetrics(); return; }
        if (e.target.closest("#hmLogSave")) saveLog();
    });

    // ---------- Refresh: readings and alerts follow new data ----------
    async function refresh(force) {
        if (!force && (document.hidden || playingCam != null || document.querySelector(".modal.show"))) return;
        try {
            const resp = await fetch("/home/data", { cache: "no-store" });
            if (!resp.ok) return;
            const next = await resp.json();
            next.t = T;
            D = next;
            render();
        } catch (e) { /* keep what's shown */ }
    }
    setInterval(() => refresh(false), Math.max(30, D.poll || 60) * 1000);

    render();
})();
