// Environment page (views/sensors.html), from mockup E4: one tent at a time,
// a readings strip (click to show/hide a chart), stacked charts that share
// a hover line and drag-to-zoom, and the AC Infinity device lane. Data:
// the JSON in #envPage[data-page] plus GET /sensors/series.
(function () {
    "use strict";
    const root = document.getElementById("envPage");
    if (!root) return;
    const P = JSON.parse(root.dataset.page || "{}");
    const T = P.t || {};
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
    const MIN = 60000;
    const locale = P.lang || undefined;

    // ---------- What each sensor is (shared: sensor-kinds.js) ----------
    const K = window.SensorKinds;
    const isPort = K.isPort, isOutside = K.isOutside;
    const ORDER = { temp: 0, hum: 1, vpd: 2, co2: 3, soil: 4, ec: 5, other: 6 };
    const OTHER_COLORS = ["#0ea5e9", "#14b8a6", "#64748b", "#eab308", "#ec4899"];
    function look(s, i) {
        const k = K.kind(s), out = isOutside(s);
        const color = { temp: out ? "#f97316" : "#ef4444", hum: out ? "#a78bfa" : "#6366f1", vpd: "#10b981", co2: "#d946ef", soil: "#a16207", ec: "#0d9488" }[k] || OTHER_COLORS[i % OTHER_COLORS.length];
        const { unit, conv } = K.display(s, P.units);
        const span = { temp: unit === "°C" ? 2 : 4, hum: 10, vpd: 0.4, co2: 80, soil: 10 }[k] || 0;
        return { k, color, dp: K.decimals(s), unit, conv, span };
    }

    // ---------- Formatting ----------
    const fmt = (v, dp) => (v == null || isNaN(v)) ? "—" : Number(v).toLocaleString(locale, { minimumFractionDigits: dp, maximumFractionDigits: dp });
    const fmtTime = ms => new Date(ms).toLocaleString(locale, { month: "numeric", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" });
    const fmtHover = ms => new Date(ms).toLocaleString(locale, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
    function ago(ms) {
        const m = Math.max(0, Math.round((Date.now() - ms) / MIN));
        if (m < 60) return t("sx_ago_min", { n: m });
        if (m < 48 * 60) return t("sx_ago_h", { n: Math.round(m / 60) });
        return t("sx_ago_d", { n: Math.round(m / 1440) });
    }
    const pad = n => String(n).padStart(2, "0");
    const toLocalInput = ms => { const d = new Date(ms); return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`; };
    const cssVar = (name, fallback) => getComputedStyle(root).getPropertyValue(name).trim() || fallback;

    // ---------- Tents and their sensors ----------
    const sensors = (P.sensors || []).filter(s => s.show_env && !s.hidden && s.zone_id);
    const zones = (P.zones || []).filter(z => sensors.some(s => s.zone_id === z.id));
    const latest = {};   // id -> {v, at} in the sensor's own unit
    sensors.forEach(s => { if (s.value != null) latest[s.id] = { v: s.value, at: s.last_at }; });

    function zoneSensors(zid) {
        const list = sensors.filter(s => s.zone_id === zid);
        const env = list.filter(s => !isPort(s)).map((s, i) => ({ ...s, ...look(s, i) }))
            .sort((a, b) => (isOutside(a) - isOutside(b)) || (ORDER[a.k] - ORDER[b.k]) || a.name.localeCompare(b.name));
        const devices = list.filter(isPort).sort((a, b) => a.type.localeCompare(b.type, undefined, { numeric: true }))
            .map(s => ({ ...s, icon: K.portIcon(s.name) }));
        return { env, devices };
    }

    let zoneId = null;
    const hashZone = () => { const m = /zone=(\d+)/.exec(location.hash); return m ? +m[1] : null; };
    function pickZone() {
        const want = hashZone() ?? +(store.get("isley.env.zone") || 0);
        zoneId = zones.some(z => z.id === want) ? want : (zones[0] && zones[0].id);
    }
    const hiddenKey = () => "isley.env.hidden." + zoneId;
    let hidden = new Set();
    const loadHidden = () => { try { hidden = new Set(JSON.parse(store.get(hiddenKey()) || "[]")); } catch (e) { hidden = new Set(); } };
    const saveHidden = () => store.set(hiddenKey(), JSON.stringify([...hidden]));

    // ---------- Time range ----------
    const SPANS = [["en_span_1h", 60], ["en_span_6h", 360], ["en_span_24h", 1440], ["en_span_7d", 10080]];
    const newRange = span => { const to = Date.now(); return { span, from: to - span * MIN, to, live: true, custom: false, zoomed: false }; };
    let r = newRange(+(store.get("isley.env.span") || 1440));
    function stepRange(dir) {
        const w = r.to - r.from, now = Date.now();
        let from = r.from + dir * w, to = r.to + dir * w, live = false;
        if (to >= now) { to = now; from = to - w; live = true; }
        r = { ...r, from, to, live, zoomed: false, baseFrom: undefined, baseTo: undefined };
    }

    // ---------- Data ----------
    let data = {};      // id -> [[ms, v]] (converted)
    let loading = false, failed = false, reqNo = 0;
    async function fetchSeries(ids, from, to) {
        const resp = await fetch(`/sensors/series?ids=${ids.join(",")}&from=${Math.round(from)}&to=${Math.round(to)}`, { cache: "no-store" });
        if (!resp.ok) throw new Error(resp.status);
        return (await resp.json()).series || {};
    }
    async function load() {
        const { env, devices } = zoneSensors(zoneId);
        const all = [...env, ...devices];
        if (!all.length) return;
        const my = ++reqNo;
        if (markX != null && (markX < r.from || markX > r.to)) markX = null;
        loading = true; failed = false; render();
        try {
            const series = await fetchSeries(all.map(s => s.id), r.from, r.to);
            if (my !== reqNo) return;
            data = {};
            all.forEach(s => {
                const conv = s.conv || (v => v);
                data[s.id] = (series[s.id] || []).map(p => ({ x: p[0], y: conv(p[1]) }));
            });
            if (r.live) await refreshLatest(all);
        } catch (e) {
            if (my !== reqNo) return;
            failed = true;
        }
        loading = false;
        render();
    }
    // Latest readings for the tiles: the last few minutes of raw data.
    async function refreshLatest(all) {
        try {
            const now = Date.now();
            const series = await fetchSeries(all.map(s => s.id), now - Math.max(30, (P.poll || 60) / 20) * MIN, now);
            all.forEach(s => { const pts = series[s.id] || []; if (pts.length) latest[s.id] = { v: pts[pts.length - 1][1], at: pts[pts.length - 1][0] }; });
        } catch (e) { /* keep what we had */ }
    }
    const nowValue = s => latest[s.id] ? (s.conv || (v => v))(latest[s.id].v) : null;
    function stats(pts) {
        if (!pts || !pts.length) return null;
        let mn = Infinity, mx = -Infinity, sum = 0;
        pts.forEach(p => { if (p.y < mn) mn = p.y; if (p.y > mx) mx = p.y; sum += p.y; });
        return { min: mn, max: mx, avg: sum / pts.length };
    }
    function valueAt(pts, x) {
        if (!pts || !pts.length) return null;
        let lo = 0, hi = pts.length - 1;
        while (lo < hi) { const mid = (lo + hi) >> 1; if (pts[mid].x < x) lo = mid + 1; else hi = mid; }
        const a = pts[Math.max(0, lo - 1)], b = pts[lo];
        return Math.abs(a.x - x) < Math.abs(b.x - x) ? a.y : b.y;
    }

    // ---------- Drawing ----------
    let charts = [], lane = null;
    // The marked moment (click a chart; drag the line to move it). Mouse
    // movement alone never moves it, like the AC Infinity app.
    let markX = null;
    function tag(ctx, text, x, y, color, area) {
        ctx.font = "600 11px system-ui, -apple-system, Segoe UI, sans-serif";
        const w = ctx.measureText(text).width + 12, h = 18;
        let left = x + 7;
        if (left + w > area.right) left = x - 7 - w;
        const top = Math.max(area.top, Math.min(area.bottom - h, y - h / 2));
        ctx.fillStyle = cssVar("--sx-card", "#fff");
        ctx.strokeStyle = color; ctx.lineWidth = 1;
        ctx.beginPath(); ctx.roundRect ? ctx.roundRect(left, top, w, h, 4) : ctx.rect(left, top, w, h); ctx.fill(); ctx.stroke();
        ctx.fillStyle = cssVar("--sx-ink", "#172322"); ctx.textBaseline = "middle";
        ctx.fillText(text, left + 6, top + h / 2 + 0.5);
    }
    const marker = {
        id: "sxMarker",
        afterDraw(chart) {
            const o = chart.$sx;
            if (markX == null) return;
            const area = chart.chartArea, x = chart.scales.x.getPixelForValue(markX);
            if (x < area.left || x > area.right) return;
            const ctx = chart.ctx;
            ctx.save();
            ctx.strokeStyle = "#16a34a"; ctx.lineWidth = 1.5;
            ctx.beginPath(); ctx.moveTo(x, area.top); ctx.lineTo(x, area.bottom); ctx.stroke();
            if (o && o.series) {
                const s = o.series, v = valueAt(data[s.id], markX);
                if (v != null) {
                    const y = chart.scales.y.getPixelForValue(v);
                    ctx.fillStyle = s.color; ctx.beginPath(); ctx.arc(x, y, 3.5, 0, Math.PI * 2); ctx.fill();
                    tag(ctx, `${fmt(v, s.dp)} ${s.unit}`, x, y, s.color, area);
                }
            }
            if (o && o.devices) {
                o.devices.forEach((d, i) => {
                    const v = valueAt(data[d.id], markX);
                    if (v == null) return;
                    const y = chart.scales.y.getPixelForValue(o.devices.length - 1 - i + 0.4);
                    tag(ctx, v > 0 ? Math.round(v) + "%" : t("en_off"), x, y, d.color, area);
                });
            }
            ctx.restore();
        },
    };
    // Tick spacing by range length, so the grid stays readable (charts
    // without labels would otherwise draw a line for every generated tick).
    function tickUnit(ms) {
        const h = ms / 3600000;
        if (h <= 1.5) return { unit: "minute", stepSize: 10 };
        if (h <= 8) return { unit: "hour", stepSize: 1 };
        if (h <= 30) return { unit: "hour", stepSize: 3 };
        if (h <= 96) return { unit: "hour", stepSize: 12 };
        return { unit: "day", stepSize: Math.max(1, Math.round(h / 24 / 8)) };
    }
    const timeScale = (from, to, showTicks) => ({
        type: "time", min: from, max: to,
        time: { unit: tickUnit(to - from).unit, displayFormats: locale === "en" || !locale
            ? { minute: "h:mm a", hour: "h a", day: "MMM d" }
            : { minute: "HH:mm", hour: "HH:mm", day: "d MMM" } },
        ticks: { stepSize: tickUnit(to - from).stepSize, display: showTicks, maxRotation: 0, autoSkipPadding: 18, align: "inner", color: cssVar("--sx-muted", "#64748b"), font: { size: 11 } },
        grid: { color: "rgba(100,116,139,.13)" },
    });
    const zoomOpts = {
        zoom: {
            drag: { enabled: true, threshold: 6, backgroundColor: "rgba(22,163,74,.12)", borderColor: "#16a34a", borderWidth: 1 }, mode: "x",
            onZoomComplete: ({ chart }) => {
                const from = chart.scales.x.min, to = chart.scales.x.max;
                if (to - from < MIN) return;
                r = { ...r, baseFrom: r.baseFrom ?? r.from, baseTo: r.baseTo ?? r.to, from, to, zoomed: true, live: false };
                load();
            },
        },
    };

    function showMarker() {
        charts.forEach(c => c.draw());
        if (lane) lane.draw();
        zoneSensors(zoneId).env.filter(s => !hidden.has(s.id)).forEach(s => {
            const el = root.querySelector(`[data-v="${s.id}"]`), w = root.querySelector(`[data-w="${s.id}"]`);
            if (!el) return;
            const v = markX == null ? nowValue(s) : valueAt(data[s.id], markX);
            el.innerHTML = `${fmt(v, s.dp)}<small>${esc(s.unit)}</small>`;
            w.textContent = markX == null ? t("en_now") : fmtHover(markX);
        });
        const chip = document.getElementById("sxMark");
        if (chip) {
            chip.hidden = markX == null;
            if (markX != null) document.getElementById("sxMarkTime").textContent = fmtHover(markX);
        }
    }

    function renderZones() {
        document.getElementById("sxZones").innerHTML = zones.map(z =>
            `<button type="button" data-zone="${z.id}" class="${z.id === zoneId ? "on" : ""}" aria-pressed="${z.id === zoneId}"><i class="fa-solid ${/outdoor|outside/i.test(z.name) ? "fa-tree" : "fa-tent"} me-1"></i>${esc(z.name)}</button>`).join("");
    }
    function renderReads(env) {
        document.getElementById("sxReads").innerHTML = `<div class="sx-readings">${env.map(s => {
            const v = nowValue(s), last = latest[s.id];
            const stale = s.stale || (last && Date.now() - last.at > Math.max(5 * MIN, 3 * (P.poll || 60) * 1000));
            return `<button type="button" class="sx-read ${hidden.has(s.id) ? "off" : ""}" data-toggle="${s.id}" style="--c:${s.color}" title="${esc(t("en_toggle"))}" aria-pressed="${!hidden.has(s.id)}">
                <span class="lbl"><span class="dot"></span>${esc(s.name)}</span>
                <span class="val">${fmt(v, s.dp)}<small>${esc(s.unit)}</small></span>
                ${stale && last ? `<span class="st">${esc(t("en_last_reading", { ago: ago(last.at) }))}</span>` : ""}
            </button>`;
        }).join("")}</div>`;
    }
    function renderRange() {
        const now = Date.now();
        document.getElementById("sxRange").innerHTML = `<div class="sx-range">
            <button type="button" class="sx-step" data-step="-1" aria-label="${esc(t("en_earlier"))}" title="${esc(t("en_earlier"))}"><i class="fa-solid fa-chevron-left"></i></button>
            <div class="sx-when"><b>${fmtTime(r.from)}</b><span>–</span><b>${r.live ? esc(t("en_now")) : fmtTime(r.to)}</b>${r.zoomed ? `<span class="sx-zoomed">${esc(t("en_zoomed"))}</span>` : ""}${loading ? `<span class="spinner-border spinner-border-sm text-success ms-1" role="status" aria-label="${esc(t("en_loading"))}"></span>` : ""}</div>
            <button type="button" class="sx-step" data-step="1" ${r.to >= now - MIN ? "disabled" : ""} aria-label="${esc(t("en_later"))}" title="${esc(t("en_later"))}"><i class="fa-solid fa-chevron-right"></i></button>
            <div class="sx-seg">${SPANS.map(([k, m]) => `<button type="button" data-span="${m}" class="${r.span === m && !r.custom && !r.zoomed ? "on" : ""}">${esc(t(k))}</button>`).join("")}<button type="button" data-custom class="${r.custom ? "on" : ""}">${esc(t("en_custom"))}</button></div>
            ${r.zoomed ? `<button type="button" class="sx-link" data-resetzoom><i class="fa-solid fa-magnifying-glass-minus me-1"></i>${esc(t("en_reset_zoom"))}</button>` : ""}
            <span class="sx-mark" id="sxMark" ${markX == null ? "hidden" : ""}><i class="fa-solid fa-location-crosshairs"></i><span id="sxMarkTime" title="${esc(t("en_marker"))}"></span><button type="button" data-clearmark aria-label="${esc(t("en_marker_clear"))}" title="${esc(t("en_marker_clear"))}"><i class="fa-solid fa-xmark"></i></button></span>
        </div>
        <div class="sx-custom ${r.custom ? "" : "d-none"}">
            <label>${esc(t("en_from"))} <input type="datetime-local" id="sxFrom" value="${toLocalInput(r.from)}"></label>
            <label>${esc(t("en_to"))} <input type="datetime-local" id="sxTo" value="${toLocalInput(r.to)}"></label>
            <button type="button" class="sx-btn" data-applycustom>${esc(t("en_apply"))}</button>
        </div>`;
    }

    function render() {
        renderZones();
        const empty = document.getElementById("sxEmpty"), card = document.getElementById("sxCard"), reads = document.getElementById("sxReads");
        if (!zones.length) {
            card.hidden = true; reads.innerHTML = "";
            const any = (P.sensors || []).length > 0;
            empty.innerHTML = `<div class="sx-empty"><i class="fa-solid fa-temperature-half"></i>
                <b>${esc(t(any ? "en_none_shown_title" : "en_no_sensors_title"))}</b>
                <p>${esc(t(any ? "en_none_shown" : "en_no_sensors"))}</p>
                <a class="sx-btn" href="/sensors/manage"><i class="fa-solid fa-sliders me-1"></i>${esc(t("en_manage"))}</a></div>`;
            return;
        }
        empty.innerHTML = ""; card.hidden = false;
        const { env, devices } = zoneSensors(zoneId);
        renderReads(env);
        renderRange();

        charts.forEach(c => c.destroy()); charts = [];
        const vis = env.filter(s => !hidden.has(s.id));
        const mults = document.getElementById("sxMults");
        const anyData = [...env, ...devices].some(s => data[s.id] && data[s.id].length);
        if (!vis.length && env.length) {
            mults.innerHTML = `<div class="sx-note">${esc(t("en_all_hidden"))}</div>`;
        } else if (failed) {
            mults.innerHTML = `<div class="sx-note text-danger">${esc(t("en_load_failed"))}</div>`;
        } else if (!loading && !anyData) {
            mults.innerHTML = `<div class="sx-note">${esc(t("en_no_data"))}</div>`;
        } else {
            mults.innerHTML = vis.map((s, i) => {
                const st = stats(data[s.id]);
                return `<div class="sx-mult ${i === vis.length - 1 ? "last" : ""}">
                    <div class="sx-m-lbl"><span class="nm"><span class="dot" style="background:${s.color}"></span>${esc(s.name)}</span>
                        <span class="v" data-v="${s.id}">${fmt(nowValue(s), s.dp)}<small>${esc(s.unit)}</small></span>
                        <span class="when" data-w="${s.id}">${esc(t("en_now"))}</span>
                        <span class="mm">${st ? `${fmt(st.min, s.dp)} – ${fmt(st.max, s.dp)} · ${esc(t("en_avg"))} ${fmt(st.avg, s.dp)}` : "—"}</span></div>
                    <div class="sx-m-chart"><canvas id="sxc_${s.id}" role="img" aria-label="${esc(s.name)}"></canvas></div></div>`;
            }).join("");
            vis.forEach((s, i) => {
                const pts = data[s.id] || [], st = stats(pts) || { min: 0, max: 1 };
                const half = Math.max((st.max - st.min) / 2 * 1.2, s.span / 2, 1e-6), mid = (st.max + st.min) / 2;
                const c = new Chart(document.getElementById("sxc_" + s.id), {
                    type: "line",
                    data: { datasets: [{ data: pts, borderColor: s.color, backgroundColor: s.color + "22", fill: true, borderWidth: 1.8, pointRadius: 0, tension: .3 }] },
                    plugins: [marker],
                    options: {
                        animation: false, maintainAspectRatio: false, parsing: false, normalized: true,
                        plugins: { legend: { display: false }, tooltip: { enabled: false }, zoom: zoomOpts },
                        // No y numbers: keeps every chart and the device lane lined up; min/max/avg are in the label column.
                        scales: { x: timeScale(r.from, r.to, i === vis.length - 1), y: { min: mid - half, max: mid + half, display: false } },
                    },
                });
                c.$sx = { series: s };
                charts.push(c);
            });
        }
        document.getElementById("sxHint").hidden = !vis.length;

        if (lane) { lane.destroy(); lane = null; }
        const laneWrap = document.getElementById("sxLane");
        laneWrap.hidden = !devices.length;
        if (devices.length) {
            document.getElementById("sxLaneLabels").innerHTML = devices.map(d => {
                const v = latest[d.id] ? latest[d.id].v : null;
                return `<div class="sx-dev"><i class="fa-solid ${d.icon}"></i><span title="${esc(d.name)}">${esc(d.name)}</span><b>${v > 0 ? Math.round(v) + "%" : esc(t("en_off"))}</b></div>`;
            }).join("");
            document.getElementById("sxLaneBox").style.height = (devices.length * 28 + 8) + "px";
            const n = devices.length;
            const colors = ["#eab308", "#0ea5e9", "#6366f1", "#f97316", "#14b8a6", "#64748b", "#ec4899", "#94a3b8"];
            devices.forEach((d, i) => { d.color = colors[i % colors.length]; });
            lane = new Chart(document.getElementById("sxLaneCanvas"), {
                type: "line",
                data: { datasets: devices.map((d, i) => {
                    const col = colors[i % colors.length], base = n - 1 - i;
                    return { label: d.name, stepped: true, borderColor: col, backgroundColor: col + "33", fill: { target: { value: base } },
                        borderWidth: 1.5, pointRadius: 0,
                        data: (data[d.id] || []).map(p => ({ x: p.x, y: base + Math.max(0, Math.min(100, p.y)) / 100 * 0.78, raw: p.y })) };
                }) },
                plugins: [marker],
                options: {
                    animation: false, maintainAspectRatio: false, parsing: false,
                    plugins: { legend: { display: false }, zoom: zoomOpts, tooltip: { enabled: false } },
                    scales: { x: { ...timeScale(r.from, r.to, false), grid: { color: "rgba(100,116,139,.08)" } }, y: { min: 0, max: n, display: false } },
                    layout: { padding: { top: 2, bottom: 2 } },
                },
            });
            lane.$sx = { devices };
            root.querySelectorAll(".sx-dev").forEach((el, i) => el.style.setProperty("--c", colors[i % colors.length]));
        }
        if (markX != null) showMarker();
    }

    // ---------- Events ----------
    function selectZone(id) {
        zoneId = id;
        store.set("isley.env.zone", String(id));
        history.replaceState(null, "", "#zone=" + id);
        loadHidden();
        data = {};
        r = r.live || r.zoomed ? newRange(r.span) : r;
        load();
    }
    root.addEventListener("click", e => {
        const z = e.target.closest("[data-zone]");
        if (z) { if (+z.dataset.zone !== zoneId) selectZone(+z.dataset.zone); return; }
        const tg = e.target.closest("[data-toggle]");
        if (tg) { const id = +tg.dataset.toggle; hidden.has(id) ? hidden.delete(id) : hidden.add(id); saveHidden(); render(); return; }
        if (e.target.closest("[data-clearmark]")) { markX = null; showMarker(); return; }
        const b = e.target.closest("[data-span],[data-step],[data-custom],[data-applycustom],[data-resetzoom]");
        if (!b) return;
        if (b.dataset.span) { r = newRange(+b.dataset.span); store.set("isley.env.span", b.dataset.span); load(); }
        else if (b.dataset.step) { stepRange(+b.dataset.step); load(); }
        else if (b.hasAttribute("data-custom")) { r = { ...r, custom: !r.custom }; render(); }
        else if (b.hasAttribute("data-applycustom")) {
            const f = new Date(document.getElementById("sxFrom").value).getTime(), to = new Date(document.getElementById("sxTo").value).getTime();
            if (f < to) { const now = Date.now(); r = { ...r, from: f, to: Math.min(to, now), live: to >= now, custom: true, zoomed: false }; load(); }
        } else if (b.hasAttribute("data-resetzoom")) {
            r = { ...r, from: r.baseFrom ?? r.from, to: r.baseTo ?? r.to, zoomed: false, baseFrom: undefined, baseTo: undefined };
            if (!r.custom && r.to >= Date.now() - 2 * MIN) r = newRange(r.span);
            load();
        }
    });
    // Click (or tap) a chart to mark that moment; grab the line to drag it.
    // Dragging anywhere else is left to the zoom plugin (drag to zoom).
    const card = document.getElementById("sxCard");
    const GRAB = 8;   // px either side of the line that count as grabbing it
    const chartAt = target => {
        const canvas = target && target.closest && target.closest(".sx-m-chart canvas, .sx-lane-chart canvas");
        return canvas ? Chart.getChart(canvas) : null;
    };
    const pxIn = (chart, clientX) => clientX - chart.canvas.getBoundingClientRect().left;
    const nearLine = (chart, clientX) => markX != null && Math.abs(pxIn(chart, clientX) - chart.scales.x.getPixelForValue(markX)) <= GRAB;
    function moveMarkTo(chart, clientX) {
        const area = chart.chartArea, px = Math.max(area.left, Math.min(area.right, pxIn(chart, clientX)));
        markX = chart.scales.x.getValueForPixel(px);
        showMarker();
    }
    let dragging = null, down = null;
    card.addEventListener("mousedown", e => {
        const chart = chartAt(e.target);
        if (!chart || e.button !== 0) return;
        if (nearLine(chart, e.clientX)) {
            dragging = chart;
            e.stopPropagation(); e.preventDefault();   // keeps the zoom plugin from starting a zoom
        } else down = { chart, x: e.clientX, y: e.clientY };
    }, true);
    document.addEventListener("mousemove", e => {
        if (dragging) { moveMarkTo(dragging, e.clientX); return; }
        const chart = chartAt(e.target);
        if (chart) chart.canvas.style.cursor = nearLine(chart, e.clientX) ? "ew-resize" : "crosshair";
    });
    document.addEventListener("mouseup", e => {
        if (dragging) { dragging = null; return; }
        if (down && Math.abs(e.clientX - down.x) < 5 && Math.abs(e.clientY - down.y) < 5) {
            const area = down.chart.chartArea, px = pxIn(down.chart, e.clientX);
            if (px >= area.left && px <= area.right) moveMarkTo(down.chart, e.clientX);
        }
        down = null;
    });
    // Touch: tap to mark, drag the line with a finger.
    let touch = null;
    card.addEventListener("touchstart", e => {
        const chart = chartAt(e.target), p = e.touches[0];
        if (!chart || e.touches.length !== 1) return;
        touch = { chart, x: p.clientX, y: p.clientY, drag: nearLine(chart, p.clientX), moved: false };
    }, { passive: true });
    card.addEventListener("touchmove", e => {
        if (!touch) return;
        const p = e.touches[0];
        if (Math.abs(p.clientX - touch.x) > 5 || Math.abs(p.clientY - touch.y) > 5) touch.moved = true;
        if (touch.drag) { e.preventDefault(); moveMarkTo(touch.chart, p.clientX); }
    }, { passive: false });
    card.addEventListener("touchend", () => {
        if (touch && !touch.drag && !touch.moved) moveMarkTo(touch.chart, touch.x);
        touch = null;
    });
    document.addEventListener("keydown", e => { if (e.key === "Escape" && markX != null) { markX = null; showMarker(); } });

    window.addEventListener("hashchange", () => { const h = hashZone(); if (h && h !== zoneId && zones.some(z => z.id === h)) selectZone(h); });

    // Live: while the range ends now, follow new readings.
    const every = Math.max(30, P.poll || 60) * 1000;
    setInterval(() => {
        if (document.hidden || !zoneId || !r.live || r.zoomed || loading) return;
        r = { ...newRange(r.span), custom: false };
        load();
    }, every);

    pickZone();
    if (zoneId) { loadHidden(); history.replaceState(null, "", "#zone=" + zoneId); load(); }
    else render();
})();
