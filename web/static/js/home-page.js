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

    // Needs attention settings (signed in only) and the days without water /
    // feed before "due" (amber) and "overdue" (red), also used by the Plants
    // page; the tag colors follow the days even when a reminder type is off.
    const NA = () => D.attention || {};
    const NS = () => NA().settings || { water: { on: true }, feed: { on: true }, photo: { on: true, days: 14 }, seeds: { on: true }, sensors: { on: true }, burp: { on: false } };
    const careDays = kind => (D.care_days || {})[kind] || (kind === "water" ? [3, 6] : [10, 18]);
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
    // A reminder type turned off in Needs attention settings shows gray, with no Due/Overdue color.
    const careOff = kind => ((D.care_days || {}).off || []).includes(kind);
    const level = (kind, d) => d == null ? "none" : careOff(kind) ? "off" : d >= careDays(kind)[1] ? "over" : d >= careDays(kind)[0] ? "due" : "ok";
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
    // Each line has a key (what a hide is saved under) and a basis: what it
    // was drawn from, so "Hide until watered" etc. end when that changes.
    // A subset basis (sensors, seeds) is a list: the line stays hidden while
    // every item now is one that was there when it was hidden.
    function listHtml(names) {
        try {
            return new Intl.ListFormat(locale, { type: "conjunction" }).formatToParts(names)
                .map(p => p.type === "element" ? `<b>${esc(p.value)}</b>` : esc(p.value)).join("");
        } catch (e) { return names.map(n => `<b>${esc(n)}</b>`).join(", "); }
    }
    const zones = () => NA().zones || [];
    const zoneName = id => (zones().find(z => z.id === id) || {}).name || "";
    // Curing plants whose jars are due a burp: daily for the first days of
    // curing, then every few days, counted from the last burp (or the day
    // the plant entered Curing).
    function burpDue() {
        const B = NS().burp, burped = NA().burped || {};
        return (NA().curing || []).filter(p => p.since).map(p => {
            const cday = days(p.since, D.today) + 1, every = cday <= B.daily ? 1 : B.then;
            const b = burped[p.id], last = b && b > p.since ? b : p.since;
            return { ...p, cday, due: days(last, D.today) >= every };
        }).filter(p => p.due);
    }
    function alerts() {
        const out = [], S = NS();
        const name = p => `<b>${esc(p.name)}</b>`;
        plants.forEach(p => {
            const lw = level("water", p.water);
            if (S.water.on && (lw === "over" || lw === "due")) out.push({ key: "water:" + p.id, basis: p.last_water || "", until: "water", lv: lw === "over" ? "over" : "warn", icon: "fa-droplet", txt: t("hm_alert_water", { plant: "#P#", n: p.water }).replace("#P#", name(p)), act: `<button class="hm-btn-s primary" data-log="${p.id}" data-kind="water"><i class="fa-solid fa-droplet me-1"></i>${esc(t("hm_watered_it"))}</button>` });
            const lf = level("feed", p.feed);
            if (S.feed.on && (lf === "over" || lf === "due")) out.push({ key: "feed:" + p.id, basis: p.last_feed || "", until: "feed", lv: lf === "over" ? "over" : "warn", icon: "fa-flask", txt: t("hm_alert_feed", { plant: "#P#", n: p.feed }).replace("#P#", name(p)), act: `<button class="hm-btn-s primary" data-log="${p.id}" data-kind="feed"><i class="fa-solid fa-flask me-1"></i>${esc(t("hm_fed_it"))}</button>` });
        });
        // Burp curing jars: one line for every plant due.
        if (S.burp.on) {
            const due = burpDue();
            if (due.length) out.push({ key: "burp", task: true, lv: "warn", icon: "fa-jar", cls: "task",
                txt: due.length === 1 ? esc(t("hm_na_burp_one", { plant: "#P#", n: due[0].cday })).replace("#P#", name(due[0])) : esc(t("hm_na_burp_many", { plants: "#P#" })).replace("#P#", listHtml(due.map(p => p.name))),
                act: `<button class="hm-btn-s primary" data-done="burp"><i class="fa-solid fa-check me-1"></i>${esc(t("hm_na_burped"))}</button>` });
        }
        // Repeating tasks: a line per tent once due, red when late.
        const inTent = zones().length > 1;
        (NA().tasks || []).forEach(tk => Object.entries(tk.zones || {}).forEach(([zid, last]) => {
            const d = days(last, D.today);
            if (d < tk.every) return;
            const late = d - tk.every, sfx = inTent ? "_in" : "";
            const k = late === 0 ? "hm_na_task_due" : late === 1 ? "hm_na_task_late_one" : "hm_na_task_late";
            out.push({ key: `task:${tk.id}:${zid}`, task: true, every: tk.every, lv: late ? "over" : "warn", icon: tk.icon || "fa-repeat", cls: "task",
                txt: esc(t(k + sfx, { task: "#T#", tent: zoneName(+zid), n: late })).replace("#T#", `<b>${esc(tk.name)}</b>`),
                act: `<button class="hm-btn-s primary" data-done="task:${tk.id}:${zid}"><i class="fa-solid fa-check me-1"></i>${esc(t("hm_na_done"))}</button>` });
        }));
        // An integration that refuses BudMinder or doesn't answer (stale
        // readings alone are covered by the per-tent line below). Always on.
        const NAMES = { acinfinity: "AC Infinity", ecowitt: "Ecowitt", homeassistant: "Home Assistant", vivosun: "Vivosun" };
        Object.entries(D.integrations || {}).forEach(([src, st]) => {
            if (st.state !== "auth" && st.state !== "failing") return;
            const key = st.state === "auth" ? ({ acinfinity: "hm_alert_aci_auth", vivosun: "hm_alert_vs_auth" }[src] || "hm_alert_int_auth") : "hm_alert_int_failing";
            const txt = t(key, { name: NAMES[src] || src, ago: st.since ? ago(st.since) : "—" });
            const act = st.state === "auth" ? `<a class="hm-link" href="/settings#integrations">${esc(t("hm_sign_in_again"))}</a>` : `<a class="hm-link" href="/sensors/manage">${esc(t("hm_check_sensors"))}</a>`;
            out.push({ key: "int:" + src.toLowerCase().replace(/[^a-z]/g, ""), basis: st.state + "@" + (st.since || 0), until: "int", lv: "over", icon: "fa-plug-circle-exclamation", txt: esc(txt), act });
        });
        // Sensors that stopped reporting: one line per tent.
        if (S.sensors.on) (D.tents || []).forEach(tent => {
            const stale = [...tent.readings, ...tent.ports, ...(tent.quiet || [])].filter(r => r.stale && r.last_at);
            if (!stale.length) return;
            const oldest = Math.min(...stale.map(r => r.last_at)), where = tent.name || t("hm_no_tent");
            const txt = stale.length === 1 ? t("hm_alert_stale", { sensor: stale[0].name, tent: where, ago: ago(oldest) })
                : t("hm_alert_stale_n", { n: stale.length, tent: where, ago: ago(oldest) });
            out.push({ key: "sensor:" + tent.id, basis: stale.map(r => r.id + "@" + r.last_at).join(","), subset: true, until: stale.length === 1 ? "sensor" : "sensors", lv: "warn", icon: "fa-satellite-dish", txt: esc(txt), act: `<a class="hm-link" href="/sensors/manage">${esc(t("hm_check_sensors"))}</a>` });
        });
        const seeds = D.old_seeds || [];
        if (S.seeds.on && seeds.length) out.push({ key: "seeds", basis: seeds.map(s => s.id + "@" + s.added).join(","), subset: true, until: "seeds", lv: "warn", icon: "fa-vault", cls: "vault", txt: esc(t(seeds.length === 1 ? "hm_alert_seeds_one" : "hm_alert_seeds", { n: seeds.length })), act: `<a class="hm-link" href="/strains#vault">${esc(t("hm_open_vault"))}</a>` });
        if (S.photo.on) plants.forEach(p => {
            const none = p.photoDays == null && p.day >= S.photo.days, old = p.photoDays != null && p.photoDays >= S.photo.days;
            if (none || old) out.push({ key: "photo:" + p.id, basis: p.photo_date || "none", until: "photo", lv: "info", icon: "fa-camera", txt: (none ? t("hm_alert_no_photo", { plant: "#P#" }) : t("hm_alert_photo", { plant: "#P#", n: p.photoDays })).replace("#P#", name(p)), act: `<button class="hm-btn-s" data-photos="${p.id}"><i class="fa-solid fa-camera me-1"></i>${esc(t("hm_add_photo"))}</button>` });
        });
        return out;
    }

    // How a line is hidden now ("tomorrow" or "until"), or null.
    function hiddenHow(a) {
        const h = (NA().hidden || {})[a.key];
        if (!h) return null;
        if (h.how === "tomorrow") return h.until > D.today ? "tomorrow" : null;
        if (a.task) return null;
        if (a.subset) {
            const had = new Set((h.basis || "").split(","));
            return a.basis.split(",").every(x => had.has(x)) ? "until" : null;
        }
        return h.basis === a.basis ? "until" : null;
    }
    const UNTIL = { water: "hm_na_until_water", feed: "hm_na_until_feed", photo: "hm_na_until_photo", sensor: "hm_na_until_sensor", sensors: "hm_na_until_sensors", int: "hm_na_until_int", seeds: "hm_na_until_seeds" };
    const HIDDEN_AS = { water: "hm_na_hidden_water", feed: "hm_na_hidden_feed", photo: "hm_na_hidden_photo", sensor: "hm_na_hidden_sensor", sensors: "hm_na_hidden_sensors", int: "hm_na_hidden_int", seeds: "hm_na_hidden_seeds" };
    function lineMenu(a) {
        const k = esc(a.key);
        const second = a.task
            ? `<li><button type="button" class="dropdown-item" data-skip="${k}"><i class="fa-solid fa-forward"></i>${esc(t("hm_na_skip"))}</button></li>`
            : `<li><button type="button" class="dropdown-item" data-hide="${k}" data-how="until"><i class="fa-solid fa-eye-slash"></i>${esc(t(UNTIL[a.until]))}</button></li>`;
        return `<div class="dropdown"><button type="button" class="hm-x" data-bs-toggle="dropdown" aria-expanded="false" aria-label="${esc(t("hm_na_hide"))}" title="${esc(t("hm_na_hide"))}"><i class="fa-solid fa-xmark"></i></button>
            <ul class="dropdown-menu dropdown-menu-end"><li><button type="button" class="dropdown-item" data-hide="${k}" data-how="tomorrow"><i class="fa-solid fa-clock"></i>${esc(t("hm_na_tomorrow"))}</button></li>${second}</ul></div>`;
    }
    let showHidden = false, lineByKey = {};
    function renderAlerts() {
        const box = $("hmAlerts");
        if (!box) return;
        const all = alerts();
        lineByKey = Object.fromEntries(all.map(a => [a.key, a]));
        all.forEach(a => { a.hidden = hiddenHow(a); });
        const vis = all.filter(a => !a.hidden), hid = all.filter(a => a.hidden);
        $("hmAlertCount").textContent = vis.length ? String(vis.length) : t("hm_all_clear");
        box.innerHTML = vis.map(a => `<div class="hm-alert ${a.lv}"><span class="ai ${a.cls || ""}"><i class="fa-solid ${esc(a.icon)}"></i></span><span class="tx">${a.txt}</span><span class="acts">${a.act}</span>${lineMenu(a)}</div>`).join("")
            || `<div class="hm-alert info"><span class="ai"><i class="fa-solid fa-check"></i></span><span class="tx">${esc(t("hm_nothing"))}</span></div>`;
        const tog = $("naHidTog"), list = $("naHidden");
        if (!hid.length) showHidden = false;
        tog.hidden = !hid.length;
        tog.setAttribute("aria-expanded", String(showHidden));
        tog.innerHTML = `<i class="fa-solid fa-eye-slash me-1"></i>${esc(t("hm_na_hidden_n", { n: hid.length }))}<i class="fa-solid fa-chevron-down"></i>`;
        list.hidden = !showHidden;
        list.innerHTML = hid.map(a => `<div class="hm-alert ${a.lv}"><span class="ai ${a.cls || ""}"><i class="fa-solid ${esc(a.icon)}"></i></span><span class="tx"><span>${a.txt}</span><small>${esc(t(a.hidden === "tomorrow" ? "hm_na_hidden_tomorrow" : HIDDEN_AS[a.until]))}</small></span><span class="acts"><button class="hm-btn-s" data-unhide="${esc(a.key)}"><i class="fa-solid fa-eye me-1"></i>${esc(t("hm_na_show_again"))}</button></span></div>`).join("");
    }

    // ---------- Needs attention: hiding lines, Done and Undo ----------
    async function post(url, body) {
        const resp = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
        const data = await resp.json().catch(() => ({}));
        if (!resp.ok) throw new Error(data.error || t("hm_save_failed"));
        return data;
    }
    let undoFn = null, undoTimer = 0;
    function undoToast(text, undo) {
        $("naToastText").textContent = text;
        undoFn = undo || null;
        $("naUndo").hidden = !undo;
        $("naToast").hidden = false;
        clearTimeout(undoTimer);
        undoTimer = setTimeout(() => { $("naToast").hidden = true; undoFn = null; }, 8000);
    }
    const fail = e => toast(e.message || t("hm_save_failed"), "danger");
    async function setHidden(url, body) {
        const r = await post(url, body);
        D.attention.hidden = r.hidden || {};
        renderAlerts();
    }
    async function hideLine(key, how) {
        const a = lineByKey[key];
        if (!a) return;
        try {
            await setHidden("/home/attention/hide", { key, how, basis: how === "until" ? a.basis || "" : "" });
            undoToast(t(how === "tomorrow" ? "hm_na_hidden_tomorrow" : HIDDEN_AS[a.until]), () => setHidden("/home/attention/show", { key }).catch(fail));
        } catch (e) { fail(e); }
    }
    async function doneLine(key, skip) {
        try {
            if (key === "burp") {
                const due = burpDue();
                if (!due.length) return;
                const set = list => { D.attention.burped = D.attention.burped || {}; list.forEach(b => { if (b.day) D.attention.burped[b.plant_id] = b.day; else delete D.attention.burped[b.plant_id]; }); renderAlerts(); };
                const body = due.map(p => ({ plant_id: p.id, day: D.today }));
                const r = await post("/home/attention/done", { burp: body });
                set(body);
                const prev = (r.previous || []).map(x => ({ plant_id: x.plant_id, day: x.day || "" }));
                undoToast(t(skip ? "hm_na_burp_skipped" : "hm_na_burp_done"), () => post("/home/attention/done", { burp: prev }).then(() => set(prev)).catch(fail));
                return;
            }
            const [, tid, zid] = key.split(":").map(Number);
            const tk = (NA().tasks || []).find(x => x.id === tid);
            if (!tk) return;
            const set = day => { tk.zones[zid] = day; renderAlerts(); };
            const r = await post("/home/attention/done", { task_id: tid, zone_id: zid, day: D.today });
            set(D.today);
            const msg = tk.every === 1 ? (skip ? "hm_na_skipped_one" : "hm_na_marked_done_one") : (skip ? "hm_na_skipped" : "hm_na_marked_done");
            undoToast(t(msg, { n: tk.every }), () => post("/home/attention/done", { task_id: tid, zone_id: zid, day: r.previous }).then(() => set(r.previous)).catch(fail));
        } catch (e) { fail(e); }
    }

    // ---------- Tents ----------
    const KIND_LABEL = { temp: "hm_temp", hum: "hm_rh", vpd: "hm_vpd", co2: "hm_co2", soil: "hm_soil" };
    function envLine(tent) {
        const shown = {};
        // A zone's chosen climate sensor wins over others of its kind: it carries the Climate targets color.
        tent.readings.forEach(r => { const k = K.kind(r); if (KIND_LABEL[k] && r.value != null && (!shown[k] || !shown[k].climate && r.climate)) shown[k] = r; });
        const parts = ["temp", "hum", "vpd", "co2", "soil"].filter(k => shown[k]).map(k => {
            const r = shown[k], { unit, conv } = K.display(r, D.units), dp = K.decimals(r);
            const v = Number(conv(r.value)).toLocaleString(locale, { minimumFractionDigits: dp, maximumFractionDigits: dp });
            const u = unit === "ppm" ? "" : /^[°%]/.test(unit) ? unit : unit ? " " + unit : "";
            return `<span${r.climate ? ` class="cl-state-${r.climate}"` : ""}><small>${esc(t(KIND_LABEL[k]))}</small> <b>${v}</b>${esc(u)}</span>`;
        });
        const light = tent.ports.find(p => K.portIcon(p) === "fa-lightbulb" && p.value != null);
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
        const ph = p.photo ? `<div class="ph" style="background-image:url('${esc(bmThumb(p.photo))}')">` : `<div class="ph none"><span><i class="fa-regular fa-image me-1"></i>${esc(t("hm_no_photos"))}</span>`;
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
            // Every grow starts from a strain: with none yet, that's the first step.
            const firstStrain = LOGGED_IN && D.strain_count === 0;
            const acts = !LOGGED_IN ? "" : firstStrain
                ? `<div class="hm-none-acts"><a class="hm-btn" href="/strains"><i class="fa-solid fa-seedling me-1"></i>${esc(t("hm_add_first_strain"))}</a><button type="button" class="hm-btn light" data-tour-start="1"><i class="fa-solid fa-play me-1"></i>${esc(t("hm_take_tour"))}</button></div>`
                : `<a class="hm-btn" href="/plant/new"><i class="fa-solid fa-plus me-1"></i>${esc(t("hm_add_plant"))}</a>`;
            $("hmTents").innerHTML = `<div class="hm-none"><i class="fa-solid fa-seedling"></i><p>${esc(t(firstStrain ? "hm_first_strain" : "hm_no_plants"))}</p>${acts}</div>`;
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

    // ---------- Needs attention settings window ----------
    // A working copy (W) is edited and only saved by the window's Save:
    // reminder types with their days, and the repeating tasks.
    let W = null, editing = null, formTents = [], formIcon = "fa-repeat";
    const clone = o => JSON.parse(JSON.stringify(o));
    const presetName = p => t(p.key);
    function numIn(k, v, aria, max, id) {
        return `<input type="number" class="form-control" min="1" max="${max || 90}" ${id ? `id="${id}"` : `data-k="${k}"`} value="${v == null ? "" : esc(v)}" aria-label="${esc(t(aria))}">`;
    }
    // A sentence with a number box in it, e.g. "Due after [3] days".
    const withInput = (key, input) => esc(t(key, { input: "#I#" })).replace("#I#", input);
    // Icon colors: Water and Feed use their activity type's color (as on the
    // plant page's notebook), the others fixed colors.
    const NA_COLORS = { photo: "#f59e0b", seeds: "#64748b", sensors: "#0891b2", int: "#dc2626", burp: "#16a34a" };
    function naColor(type) {
        const a = type === "water" ? waterType() : type === "feed" ? feedType() : null;
        return (a && a.color) || NA_COLORS[type] || (type === "water" ? "#2f80ed" : "#27ae60");
    }
    const naIco = (type, icon) => `<span class="hm-ico" style="--c:${esc(naColor(type))}"><i class="fa-solid ${icon}"></i></span>`;
    function typeBox(type, icon, extra) {
        const on = W.S[type].on;
        return `<div class="na-type${on ? "" : " off"}" data-type="${type}">
            <label class="fm-sw form-switch" for="na_${type}"><span><b>${naIco(type, icon)}${esc(t("hm_na_" + type))}</b><small>${esc(t("hm_na_" + type + "_note"))}</small></span><input type="checkbox" class="form-check-input" role="switch" id="na_${type}"${on ? " checked" : ""}></label>
            ${extra || ""}</div>`;
    }
    const careRow = type => `<div class="na-days">
        <label><span class="dot due"></span>${withInput("hm_na_due_after", numIn("due", W.S[type].due, `hm_na_aria_${type}_due`))}</label>
        <label><span class="dot over"></span>${withInput("hm_na_over_after", numIn("over", W.S[type].over, `hm_na_aria_${type}_over`))}</label>
        <span class="na-err" hidden></span></div>`;
    function drawDialog() {
        const noZones = !zones().length;
        $("naBody").innerHTML = `
            <div class="na-tabs" role="tablist" id="naTabs"><button type="button" role="tab" data-p="rem" class="on" aria-selected="true">${esc(t("hm_na_tab_rem"))}</button><button type="button" role="tab" data-p="tasks" aria-selected="false">${esc(t("hm_na_tab_tasks"))}</button></div>
            <div class="na-pane" id="naPaneRem">
                <p class="na-intro mb-2">${esc(t("hm_na_rem_intro"))}</p>
                ${typeBox("water", "fa-droplet", careRow("water"))}
                ${typeBox("feed", "fa-flask", careRow("feed"))}
                ${typeBox("photo", "fa-camera", `<div class="na-days"><label>${withInput("hm_na_photo_after", numIn("days", W.S.photo.days, "hm_na_aria_photo"))}</label><span class="na-err" hidden></span></div>`)}
                ${typeBox("seeds", "fa-vault")}
                ${typeBox("sensors", "fa-satellite-dish")}
                <div class="na-lock"><span><b>${naIco("int", "fa-plug-circle-exclamation")}${esc(t("hm_na_int"))}</b><small>${esc(t("hm_na_int_note"))}</small></span>
                    <span class="tag"><i class="fa-solid fa-lock me-1"></i>${esc(t("hm_na_always_on"))}</span></div>
                <p class="na-foot">${esc(t("hm_na_foot"))}</p>
            </div>
            <div class="na-pane" id="naPaneTasks" hidden>
                <p class="na-intro mb-2">${esc(t("hm_na_tasks_intro"))}</p>
                ${typeBox("burp", "fa-jar", `<div class="na-days"><label>${withInput("hm_na_burp_daily", numIn("daily", W.S.burp.daily, "hm_na_aria_burp_daily"))}</label><label>${withInput("hm_na_burp_then", numIn("then", W.S.burp.then, "hm_na_aria_burp_then"))}</label><span class="na-err" hidden></span></div>`)}
                <div class="na-h">${esc(t("hm_na_your_tasks"))}</div>
                <div id="naTaskList"></div>
                <div class="na-form" id="naForm" hidden>
                    <label class="lbl" for="naTName">${esc(t("hm_na_task"))}</label>
                    <input type="text" class="form-control form-control-sm" id="naTName" maxlength="60" autocomplete="off">
                    <div class="na-chips" id="naChips" aria-label="${esc(t("hm_na_suggestions"))}"></div>
                    <label class="na-every" for="naTEvery">${withInput("hm_na_repeat_every", numIn("", "", "hm_na_aria_every", 365, "naTEvery"))}</label>
                    <small class="hint">${esc(t("hm_na_every_hint"))}</small>
                    <div id="naTTentsWrap"><span class="lbl mt-3">${esc(t("hm_na_tents"))}</span><div class="na-chips mt-0" id="naTTents"></div></div>
                    <span class="na-err" id="naTErr" hidden></span>
                    <div class="na-form-acts">
                        <button type="button" class="na-del" id="naTDel" hidden><i class="fa-solid fa-trash me-1"></i>${esc(t("hm_na_delete_task"))}</button>
                        <span class="sp"></span>
                        <button type="button" class="btn btn-light btn-sm" id="naTCancel">${esc(t("hm_cancel"))}</button>
                        <button type="button" class="btn btn-success btn-sm" id="naTSave">${esc(t("hm_na_save_task"))}</button>
                    </div>
                </div>
                ${noZones ? `<p class="na-empty">${esc(t("hm_na_no_zones"))}</p>` : `<button type="button" class="st-btn light" id="naAdd"><i class="fa-solid fa-plus me-1"></i>${esc(t("hm_na_add_task"))}</button>`}
            </div>`;
        ["water", "feed", "photo", "burp"].forEach(ty => document.querySelectorAll(`.na-type[data-type="${ty}"] .na-days input`).forEach(i => { i.disabled = !W.S[ty].on; }));
        drawTasks();
    }
    function tab(p) {
        document.querySelectorAll("#naTabs [data-p]").forEach(b => { const on = b.dataset.p === p; b.classList.toggle("on", on); b.setAttribute("aria-selected", String(on)); });
        $("naPaneRem").hidden = p !== "rem";
        $("naPaneTasks").hidden = p !== "tasks";
    }
    const everyText = n => n === 1 ? t("hm_na_every_one") : t("hm_na_every_n", { n });
    function drawTasks() {
        const many = zones().length > 1;
        $("naTaskList").innerHTML = W.tasks.map((tk, i) => {
            const where = many ? " · " + Object.keys(tk.zones).map(z => zoneName(+z)).filter(Boolean).join(", ") : "";
            return `<div class="na-task"><span class="ic"><i class="fa-solid ${esc(tk.icon || "fa-repeat")}"></i></span><span class="nm"><b>${esc(tk.name)}</b><small>${esc(everyText(tk.every) + where)}</small></span>
                <button type="button" class="ed" data-edit="${i}" aria-label="${esc(t("hm_na_edit", { task: tk.name }))}" title="${esc(t("hm_na_edit_title"))}"><i class="fa-solid fa-pen"></i></button></div>`;
        }).join("") || `<div class="na-empty">${esc(t("hm_na_no_tasks"))}</div>`;
    }
    function drawFormTents() {
        $("naTTents").innerHTML = zones().map(z => {
            const on = formTents.includes(z.id);
            return `<button type="button" class="hm-chip${on ? " on" : ""}" data-tent="${z.id}" aria-pressed="${on}"><i class="fa-solid fa-tent"></i>${esc(z.name)}</button>`;
        }).join("");
    }
    function openForm(i) {
        const tk = i == null ? null : W.tasks[i];
        editing = i == null ? -1 : i;
        $("naTName").value = tk ? tk.name : "";
        $("naTEvery").value = tk ? tk.every : "";
        formIcon = tk ? tk.icon || "fa-repeat" : "fa-repeat";
        formTents = tk ? Object.keys(tk.zones).map(Number) : zones().length === 1 ? [zones()[0].id] : [];
        const presets = NA().presets || [];
        $("naChips").innerHTML = tk ? "" : presets.map((p, n) => `<button type="button" class="hm-chip" data-preset="${n}"><i class="fa-solid ${esc(p.icon)}"></i>${esc(presetName(p))}</button>`).join("");
        $("naChips").hidden = !!tk;
        $("naTTentsWrap").hidden = zones().length < 2;
        drawFormTents();
        $("naTDel").hidden = !tk;
        $("naTErr").hidden = true;
        $("naForm").hidden = false;
        if ($("naAdd")) $("naAdd").hidden = true;
        $("naSave").disabled = true;
        $("naTName").focus();
    }
    function closeForm() {
        editing = null;
        $("naForm").hidden = true;
        if ($("naAdd")) $("naAdd").hidden = false;
        $("naSave").disabled = false;
    }
    function saveForm() {
        const name = $("naTName").value.trim().replace(/\s+/g, " "), every = Number($("naTEvery").value), err = $("naTErr");
        const msg = !name ? "hm_na_err_name" : !(Number.isInteger(every) && every >= 1 && every <= 365) ? "hm_na_err_every" : !formTents.length ? "hm_na_err_tents" : "";
        if (msg) { err.textContent = t(msg); err.hidden = false; return; }
        const old = editing >= 0 ? W.tasks[editing] : null, zonesNow = {};
        formTents.forEach(z => { zonesNow[z] = old && old.zones[z] ? old.zones[z] : D.today; });
        const tk = { id: old ? old.id : 0, name, icon: formIcon, every, zones: zonesNow };
        if (old) W.tasks[editing] = tk; else W.tasks.push(tk);
        closeForm();
        drawTasks();
    }
    // Due must come before Overdue; every box 1 to 90 days.
    function checkDays(type) {
        const box = document.querySelector(`.na-type[data-type="${type}"]`), err = box.querySelector(".na-err");
        if (!box.querySelector(".form-check-input").checked) { if (err) err.hidden = true; return true; }
        const ins = [...box.querySelectorAll(".na-days input")], ok = v => Number.isInteger(v) && v >= 1 && v <= 90;
        const vals = ins.map(i => Number(i.value)), range = vals.every(ok);
        const order = !(type === "water" || type === "feed") || vals[1] > vals[0];
        const bad = !range || !order;
        if (err) { err.textContent = t(range ? "hm_na_err_over" : "hm_na_err_days"); err.hidden = !bad; }
        ins.forEach((i, n) => i.classList.toggle("is-bad", bad && (!range ? !ok(vals[n]) : n === 1)));
        return !bad;
    }
    async function saveDialog() {
        const S = clone(W.S);
        let ok = true;
        ["water", "feed", "photo", "seeds", "sensors", "burp"].forEach(type => {
            const box = document.querySelector(`.na-type[data-type="${type}"]`);
            S[type].on = box.querySelector(".form-check-input").checked;
            if (box.querySelector(".na-days")) ok = checkDays(type) && ok;
            if (S[type].on) box.querySelectorAll(".na-days input").forEach(i => { S[type][i.dataset.k] = Number(i.value); });
        });
        if (!ok) { tab(document.querySelector("#naPaneRem .na-err:not([hidden])") ? "rem" : "tasks"); return; }
        const btn = $("naSave");
        btn.disabled = true;
        try {
            const r = await post("/home/attention/settings", { settings: S, tasks: W.tasks.map(tk => ({ id: tk.id, name: tk.name, icon: tk.icon, every: tk.every, zones: Object.keys(tk.zones).map(Number) })) });
            D.attention.settings = r.settings;
            D.attention.tasks = r.tasks;
            D.care_days = { water: [r.settings.water.due, r.settings.water.over], feed: [r.settings.feed.due, r.settings.feed.over],
                off: ["water", "feed"].filter(k => !r.settings[k].on) };
            bootstrap.Modal.getInstance($("naDlg")).hide();
            render();
        } catch (e) { fail(e); } finally { btn.disabled = false; }
    }
    const dlg = $("naDlg");
    if (dlg) {
        dlg.addEventListener("show.bs.modal", () => {
            W = { S: clone(NS()), tasks: clone(NA().tasks || []) };
            editing = null;
            drawDialog();
            $("naSave").disabled = false;
        });
        dlg.addEventListener("change", e => {
            const sw = e.target.closest(".fm-sw .form-check-input");
            if (!sw) return;
            const box = sw.closest(".na-type");
            box.classList.toggle("off", !sw.checked);
            box.querySelectorAll(".na-days input").forEach(i => { i.disabled = !sw.checked; });
            if (!sw.checked) { const err = box.querySelector(".na-err"); if (err) err.hidden = true; box.querySelectorAll(".is-bad").forEach(i => i.classList.remove("is-bad")); }
        });
        dlg.addEventListener("input", e => {
            const box = e.target.closest(".na-type");
            if (box && box.querySelector(".na-err") && !box.querySelector(".na-err").hidden) checkDays(box.dataset.type);
            if (e.target.id === "naTName") {
                // A suggestion's name keeps its icon; any other name gets the plain one.
                const v = e.target.value.trim().toLowerCase(), presets = NA().presets || [];
                const hit = presets.findIndex(p => presetName(p).toLowerCase() === v);
                formIcon = hit >= 0 ? presets[hit].icon : "fa-repeat";
                document.querySelectorAll("#naChips [data-preset]").forEach(c => c.classList.toggle("on", +c.dataset.preset === hit));
            }
        });
        dlg.addEventListener("click", e => {
            const g = sel => e.target.closest(sel);
            let el;
            if ((el = g("#naTabs [data-p]"))) { tab(el.dataset.p); return; }
            if (g("#naAdd")) { openForm(null); return; }
            if ((el = g("[data-edit]"))) { openForm(+el.dataset.edit); return; }
            if ((el = g("[data-preset]"))) {
                const p = (NA().presets || [])[+el.dataset.preset];
                $("naTName").value = presetName(p);
                $("naTEvery").value = p.days;
                formIcon = p.icon;
                document.querySelectorAll("#naChips [data-preset]").forEach(c => c.classList.toggle("on", c === el));
                return;
            }
            if ((el = g("[data-tent]"))) {
                const id = +el.dataset.tent;
                formTents = formTents.includes(id) ? formTents.filter(x => x !== id) : formTents.concat(id);
                drawFormTents();
                return;
            }
            if (g("#naTCancel")) { closeForm(); return; }
            if (g("#naTDel")) { W.tasks.splice(editing, 1); closeForm(); drawTasks(); return; }
            if (g("#naTSave")) { saveForm(); return; }
            if (g("#naSave")) saveDialog();
        });
    }

    // ---------- Events ----------
    document.addEventListener("click", e => {
        const hd = e.target.closest("[data-hide]");
        if (hd) { hideLine(hd.dataset.hide, hd.dataset.how); return; }
        const uh = e.target.closest("[data-unhide]");
        if (uh) { setHidden("/home/attention/show", { key: uh.dataset.unhide }).catch(fail); return; }
        const dn = e.target.closest("[data-done], [data-skip]");
        if (dn) { doneLine(dn.dataset.done || dn.dataset.skip, !!dn.dataset.skip); return; }
        if (e.target.closest("#naHidTog")) { showHidden = !showHidden; renderAlerts(); return; }
        if (e.target.closest("#naUndo")) { const f = undoFn; undoFn = null; $("naToast").hidden = true; if (f) f(); return; }
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
        if (!force && (document.hidden || playingCam != null || document.querySelector(".modal.show, #hmAlerts .dropdown-menu.show"))) return;
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
