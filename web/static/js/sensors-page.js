// Environment page (views/sensors.html), from mockup E4: one tent at a time,
// a readings strip (click to show/hide a chart), stacked charts that share
// one marker line (click or drag on any chart), and the AC Infinity device lane. Data:
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
    const ONOFF = { on: t("en_on"), off: t("en_off") };
    const ORDER = { temp: 0, hum: 1, vpd: 2, co2: 3, level: 4, soil: 5, ec: 6, light: 7, other: 8 };
    const OTHER_COLORS = ["#0ea5e9", "#14b8a6", "#64748b", "#eab308", "#ec4899"];
    function look(s, i) {
        const k = K.kind(s), out = isOutside(s);
        const color = { temp: out ? "#f97316" : "#ef4444", hum: out ? "#a78bfa" : "#6366f1", vpd: "#10b981", co2: "#d946ef", level: "#0ea5e9", soil: "#a16207", ec: "#0d9488", light: "#eab308" }[k] || OTHER_COLORS[i % OTHER_COLORS.length];
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
            .map(s => ({ ...s, icon: K.portIcon(s) }));
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
    const newRange = span => { const to = Date.now(); return { span, from: to - span * MIN, to, live: true, custom: false }; };
    let r = newRange(+(store.get("isley.env.span") || 1440));
    function stepRange(dir) {
        const w = r.to - r.from, now = Date.now();
        let from = r.from + dir * w, to = r.to + dir * w, live = false;
        if (to >= now) { to = now; from = to - w; live = true; }
        r = { ...r, from, to, live };
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
    // Where tag() puts its pill.
    function tagBox(ctx, text, x, y, area) {
        ctx.font = "600 11px system-ui, -apple-system, Segoe UI, sans-serif";
        const w = ctx.measureText(text).width + 12, h = 18;
        let left = x + 7;
        if (left + w > area.right) left = x - 7 - w;
        return { left, w, top: Math.max(area.top, Math.min(area.bottom - h, y - h / 2)), h };
    }
    function tag(ctx, text, x, y, color, area) {
        const { left, w, top, h } = tagBox(ctx, text, x, y, area);
        ctx.fillStyle = cssVar("--sx-card", "#fff");
        ctx.strokeStyle = color; ctx.lineWidth = 1;
        ctx.beginPath(); ctx.roundRect ? ctx.roundRect(left, top, w, h, 4) : ctx.rect(left, top, w, h); ctx.fill(); ctx.stroke();
        ctx.fillStyle = cssVar("--sx-ink", "#172322"); ctx.textBaseline = "middle";
        ctx.fillText(text, left + 6, top + h / 2 + 0.5);
    }
    // The marker's reading on a sensor chart: its text and height, or null.
    function markPill(chart) {
        const o = chart.$sx;
        if (markX == null || !o || !o.series) return null;
        const s = o.series, v = valueAt(data[s.id], markX);
        if (v == null) return null;
        return { text: `${fmt(v, s.dp)} ${s.unit}`, y: chart.scales.y.getPixelForValue(v) };
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
            const p = markPill(chart);
            if (p) {
                ctx.fillStyle = o.series.color; ctx.beginPath(); ctx.arc(x, p.y, 3.5, 0, Math.PI * 2); ctx.fill();
                tag(ctx, p.text, x, p.y, o.series.color, area);
            }
            if (o && o.devices) {
                o.devices.forEach((d, i) => {
                    const v = valueAt(data[d.id], markX);
                    if (v == null) return;
                    const y = chart.scales.y.getPixelForValue(o.devices.length - 1 - i + 0.4);
                    tag(ctx, K.portText(d, v, ONOFF), x, y, d.color, area);
                });
            }
            ctx.restore();
        },
    };
    // ---------- An alert opened from Settings > Alerts > Recent alerts ----------
    // GET /settings/alerts/events/:id/chart (handlers/alert_chart.go). Its time is
    // shaded on the charts it belongs to, with labels at the edges that say why
    // it starts and ends where it does, and the limit is drawn on the chart;
    // other alerts of the same kind in view are shaded more lightly.
    let alertView = null, alertID = null;
    const hashAlert = () => { const m = /alert=(\d+)/.exec(location.hash); return m ? +m[1] : null; };
    const AL_TEXT = {
        limit: { start: ["en_al_started", "en_al_start_limit"], end: ["en_al_ended", "en_al_end_limit"] },
        offline: { start: ["en_al_started", "en_al_start_offline"], end: ["en_al_ended", "en_al_end_offline"] },
        light: { start: ["en_al_started", "en_al_start_light"], end: ["en_al_ended", "en_al_end_light"] },
    };
    const isDark = () => document.documentElement.getAttribute("data-bs-theme") === "dark";
    // Labels are in the page's text color; the shading and dashed limit line stay red.
    const alInk = () => cssVar("--sx-ink", "#172322");
    const FONT = "system-ui, -apple-system, Segoe UI, sans-serif";
    // Whether a chart (one sensor, or the device lane) shows the alert.
    function alertOn(o) {
        const a = alertView;
        if (!a || !o) return false;
        if (a.kind === "offline") return true;
        if (o.series) return a.sensors.includes(o.series.id);
        return !!(o.devices && o.devices.some(d => a.sensors.includes(d.id)));
    }
    // A limit in the chart's unit.
    function alertLimit(s, v) {
        const a = alertView;
        if (a.limit_in === "sensor") return s.conv ? s.conv(v) : v;
        if (a.limit_in === "c" && s.unit === "°F") return v * 9 / 5 + 32;
        return v;
    }
    const limitFmt = (v, dp) => Number(v).toLocaleString(locale, { maximumFractionDigits: dp });
    const overlaps = (a, b) => a.left < b.left + b.w && a.left + a.w > b.left && a.top < b.top + b.h && a.top + a.h > b.top;
    // Where a label on a soft box goes, at the top or foot of the chart (align
    // "right" ends it at x), clear of the boxes in avoid: its usual spot, then
    // the other sides in order, then the nearest spot beside whatever is in the way.
    function placeLabel(ctx, lines, x, area, sides, align, avoid) {
        ctx.font = "700 10px " + FONT;
        const w1 = ctx.measureText(lines[0]).width;
        ctx.font = "500 10px " + FONT;
        const w2 = lines[1] ? ctx.measureText(lines[1]).width : 0;
        const w = Math.max(w1, w2) + 8, h = lines[1] ? 27 : 15;
        const clamp = l => Math.max(area.left + 2, Math.min(area.right - w - 2, l));
        const want = clamp(align === "right" ? x - w : x);
        const topOf = side => side === "bottom" ? area.bottom - h - 3 : area.top + 3;
        const lefts = [want, ...avoid.flatMap(o => [o.left + o.w + 4, o.left - 4 - w])].map(clamp)
            .filter((l, i, all) => all.indexOf(l) === i).sort((a, b) => Math.abs(a - want) - Math.abs(b - want));
        for (const left of lefts) for (const side of sides) {
            const b = { left, w, top: topOf(side), h, lines };
            if (!avoid.some(o => overlaps(b, o))) return b;
        }
        return { left: want, w, top: topOf(sides[0]), h, lines };
    }
    function drawLabel(ctx, b) {
        ctx.save();
        ctx.globalAlpha *= .88; ctx.fillStyle = cssVar("--sx-card", "#fff");
        ctx.beginPath(); ctx.roundRect ? ctx.roundRect(b.left, b.top, b.w, b.h, 3) : ctx.rect(b.left, b.top, b.w, b.h); ctx.fill();
        ctx.restore();
        ctx.fillStyle = alInk(); ctx.textBaseline = "top";
        ctx.font = "700 10px " + FONT; ctx.fillText(b.lines[0], b.left + 4, b.top + 2);
        if (b.lines[1]) { ctx.font = "500 10px " + FONT; ctx.fillText(b.lines[1], b.left + 4, b.top + 14); }
        return b;
    }
    // The green line and its reading boxes, where marker.afterDraw puts them.
    function pillBoxes(chart) {
        const o = chart.$sx, area = chart.chartArea, ctx = chart.ctx;
        if (markX == null) return [];
        const mx = chart.scales.x.getPixelForValue(markX);
        if (mx < area.left || mx > area.right) return [];
        const out = [{ left: mx - 2, w: 4, top: area.top, h: area.bottom - area.top }];
        const p = markPill(chart);
        if (p) out.push(tagBox(ctx, p.text, mx, p.y, area));
        if (o && o.devices) o.devices.forEach((d, i) => {
            const v = valueAt(data[d.id], markX);
            if (v != null) out.push(tagBox(ctx, K.portText(d, v, ONOFF), mx, chart.scales.y.getPixelForValue(o.devices.length - 1 - i + 0.4), area));
        });
        return out;
    }
    // The label beside one stretch of limit line. With more than one stretch in
    // view, it says why that value applied (handlers/alert_chart.go).
    function limitText(a, lim, inView, amount) {
        const side = a.side === "low" ? "low" : "high";
        if (a.rule) return t("en_al_limit_rule", { limit: amount });
        if (inView > 1) {
            if (lim.why === "lights_on" || lim.why === "lights_off") return t(`en_al_limit_${side}_${lim.why.slice(7)}`, { limit: amount });
            if (lim.why === "stage" && lim.stage) return t(`en_al_limit_${side}_in`, { limit: amount, stage: lim.stage });
            if (lim.why === "edit") return t(`en_al_limit_${side}_changed`, { limit: amount });
        }
        return t(`en_al_limit_${side}`, { limit: amount });
    }
    // Each limit in use while the chart's time runs: [from, to, value in the chart's unit].
    function limitSegs(chart) {
        const a = alertView, s = chart.$sx.series, x = chart.scales.x;
        return a.limits.map((lim, i) => [Math.max(x.min, lim.at), a.limits[i + 1] ? a.limits[i + 1].at : x.max, alertLimit(s, lim.v), lim])
            .filter(sg => sg[1] > sg[0] && sg[0] < x.max);
    }
    const alertMark = {
        id: "sxAlert",
        beforeDatasetsDraw(chart) {
            const a = alertView, o = chart.$sx;
            if (!alertOn(o)) return;
            const area = chart.chartArea, x = chart.scales.x, ctx = chart.ctx, now = Date.now();
            ctx.save();
            a.others.forEach(ev => {
                const l = Math.max(area.left, x.getPixelForValue(ev.start)), rt = Math.min(area.right, x.getPixelForValue(ev.end ?? now));
                if (rt <= l) return;
                ctx.fillStyle = isDark() ? "rgba(248,113,113,.06)" : "rgba(220,38,38,.035)";
                ctx.fillRect(l, area.top, rt - l, area.bottom - area.top);
                ctx.strokeStyle = isDark() ? "rgba(248,113,113,.3)" : "rgba(220,38,38,.22)"; ctx.lineWidth = 1; ctx.setLineDash([2, 3]);
                [l, rt].forEach(px => { if (px > area.left && px < area.right) { ctx.beginPath(); ctx.moveTo(px, area.top); ctx.lineTo(px, area.bottom); ctx.stroke(); } });
                ctx.setLineDash([]);
            });
            const l = Math.max(area.left, x.getPixelForValue(a.start)), rt = Math.min(area.right, x.getPixelForValue(a.end ?? now));
            if (rt > l) {
                ctx.fillStyle = isDark() ? "rgba(248,113,113,.12)" : "rgba(220,38,38,.07)";
                ctx.fillRect(l, area.top, rt - l, area.bottom - area.top);
            }
            // The limit: a dashed line for each limit in use (it can change during the alert).
            if (o.series && a.limits.length) {
                const y = chart.scales.y;
                ctx.strokeStyle = "#dc2626"; ctx.lineWidth = 1.2; ctx.setLineDash([5, 4]);
                limitSegs(chart).forEach(([f, to, v]) => { const py = y.getPixelForValue(v); ctx.beginPath(); ctx.moveTo(x.getPixelForValue(f), py); ctx.lineTo(x.getPixelForValue(to), py); ctx.stroke(); });
                ctx.setLineDash([]);
            }
            ctx.restore();
        },
        afterDatasetsDraw(chart) {
            const a = alertView, o = chart.$sx;
            if (!alertOn(o)) return;
            const area = chart.chartArea, x = chart.scales.x, ctx = chart.ctx, now = Date.now();
            // High alerts have their readings near the top, so their labels go at the foot.
            const side = a.kind === "limit" && a.side === "high" ? "bottom" : "top", away = side === "bottom" ? "top" : "bottom";
            // Labels are placed clear of the green line and its reading boxes,
            // then of each label already drawn (they re-place on every draw, so
            // they move out of the way while the line is dragged).
            const boxes = pillBoxes(chart);
            ctx.save();
            a.others.forEach(ev => {
                const l = x.getPixelForValue(ev.start), rt = x.getPixelForValue(ev.end ?? now);
                if (rt <= area.left || l >= area.right) return;
                const lines = [t(ev.start < a.start ? "en_al_earlier" : "en_al_later")];
                // At its start, or at its end when it started before the chart (the limit's label is at the left).
                const b = l > area.left + 4 ? placeLabel(ctx, lines, l + 4, area, [away, side], "left", boxes)
                    : placeLabel(ctx, lines, Math.min(area.right, rt) - 4, area, [away, side], "right", boxes);
                ctx.globalAlpha = .75; boxes.push(drawLabel(ctx, b)); ctx.globalAlpha = 1;
            });
            // Sensors offline shades every chart; its labels go on the first one only.
            if (a.kind !== "offline" || o.first) {
                const TX = AL_TEXT[a.kind] || AL_TEXT.limit, l = x.getPixelForValue(a.start), rt = x.getPixelForValue(a.end ?? now);
                if (l >= area.left - 1 && l < area.right) {
                    boxes.push(drawLabel(ctx, placeLabel(ctx, [t(TX.start[0]), t(TX.start[1])], l + 5, area, [side, away], "left", boxes)));
                    if (a.end != null && rt < area.right) {
                        const lines = [t(TX.end[0]), t(TX.end[1])];
                        ctx.font = "700 10px " + FONT;
                        const w = Math.max(ctx.measureText(lines[0]).width, ctx.measureText(lines[1]).width) + 10;
                        // After the end when there is room, otherwise just inside it.
                        const out = rt + 5 + w <= area.right - 2;
                        boxes.push(drawLabel(ctx, placeLabel(ctx, lines, out ? rt + 5 : rt - 5, area, [side, away], out ? "left" : "right", boxes)));
                    }
                }
            }
            // The limit's labels last, clear of the labels above: beside its line
            // (above a high limit, below a low one), else across the line, else
            // after the label in the way.
            if (o.series && a.limits.length) {
                const s = o.series, y = chart.scales.y, h = 12;
                const hits = (cx, cy, w) => boxes.some(b => cx < b.left + b.w && cx + w > b.left && cy < b.top + b.h && cy + h > b.top);
                ctx.font = "600 10px " + FONT; ctx.fillStyle = alInk(); ctx.textBaseline = "top";
                const segs = limitSegs(chart);
                segs.forEach(([f, to, v, lim]) => {
                    const py = y.getPixelForValue(v), text = limitText(a, lim, segs.length, `${limitFmt(v, s.dp)} ${s.unit}`);
                    const w = ctx.measureText(text).width, fx = x.getPixelForValue(f) + 4, end = x.getPixelForValue(to);
                    const near = a.side === "low" ? py + 3 : py - 3 - h, across = a.side === "low" ? py - 3 - h : py + 3;
                    const tries = [[fx, near], [fx, across], ...boxes.map(b => [b.left + b.w + 4, near]), ...boxes.map(b => [b.left + b.w + 4, across])];
                    // Within its own stretch of line, so it can't be read as another limit's.
                    const fits = ([cx, cy]) => cx >= fx && cx < end && cx + w <= area.right - 2 && cy >= area.top && cy + h <= area.bottom && !hits(cx, cy, w);
                    const [cx, cy] = tries.find(fits) || tries[0];
                    ctx.fillText(text, cx, cy);
                    boxes.push({ left: cx, w, top: cy, h });
                });
            }
            ctx.restore();
        },
    };
    // Opens the Sensors page at an alert: its zone, the time around it, and the
    // marker line at the moment it started. False when it can't be shown here.
    async function openAlert(id) {
        try {
            const resp = await fetch(`/settings/alerts/events/${id}/chart`, { cache: "no-store" });
            if (!resp.ok) throw new Error(resp.status);
            const a = await resp.json();
            if (!zones.some(z => z.id === a.zone_id)) return false;
            alertView = a; alertID = id;
            zoneId = a.zone_id;
            store.set("isley.env.zone", String(zoneId));
            loadHidden();
            // Show the alert's charts even when they were hidden.
            zoneSensors(zoneId).env.forEach(s => { if (a.kind === "offline" || a.sensors.includes(s.id)) hidden.delete(s.id); });
            const now = Date.now();
            r = { span: r.span, from: a.from, to: Math.min(a.to, now), live: a.to >= now - MIN, custom: true };
            markX = a.start;
            data = {};
            load();
            return true;
        } catch (e) {
            return false;
        }
    }

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
    // Climate targets color for a zone's chosen climate sensors, worked out
    // again for each new reading (range and near are in the sensor's unit).
    function climateClass(s, v) {
        const c = s.climate;
        if (!c || v == null) return "";
        const r = c.range || {};
        let st = "";
        if ((r.low != null && v <= r.low) || (r.high != null && v >= r.high)) st = "reached";
        else if (r.target != null) {
            const lim = v < r.target ? r.low : r.high, d = Math.abs(v - r.target);
            if (lim == null) st = d <= c.near + 1e-9 ? "ok" : "";
            else st = d <= Math.abs(lim - r.target) / 2 + 1e-9 ? "ok" : "toward";
        }
        return st ? " cl-state-" + st : "";
    }
    function renderReads(env) {
        document.getElementById("sxReads").innerHTML = `<div class="sx-readings">${env.map(s => {
            const v = nowValue(s), last = latest[s.id];
            const stale = s.stale || (last && Date.now() - last.at > Math.max(5 * MIN, 3 * (P.poll || 60) * 1000));
            return `<button type="button" class="sx-read ${hidden.has(s.id) ? "off" : ""}" data-toggle="${s.id}" style="--c:${s.color}" title="${esc(t("en_toggle"))}" aria-pressed="${!hidden.has(s.id)}">
                <span class="lbl"><span class="dot"></span>${esc(s.name)}</span>
                <span class="val${stale ? "" : climateClass(s, last ? last.v : null)}">${fmt(v, s.dp)}<small>${esc(s.unit)}</small></span>
                ${stale && last ? `<span class="st">${esc(t("en_last_reading", { ago: ago(last.at) }))}</span>` : ""}
            </button>`;
        }).join("")}</div>`;
    }
    function renderRange() {
        const now = Date.now();
        document.getElementById("sxRange").innerHTML = `<div class="sx-range">
            <button type="button" class="sx-step" data-step="-1" aria-label="${esc(t("en_earlier"))}" title="${esc(t("en_earlier"))}"><i class="fa-solid fa-chevron-left"></i></button>
            <div class="sx-when"><b>${fmtTime(r.from)}</b><span>–</span><b>${r.live ? esc(t("en_now")) : fmtTime(r.to)}</b>${loading ? `<span class="spinner-border spinner-border-sm text-success ms-1" role="status" aria-label="${esc(t("en_loading"))}"></span>` : ""}</div>
            <button type="button" class="sx-step" data-step="1" ${r.to >= now - MIN ? "disabled" : ""} aria-label="${esc(t("en_later"))}" title="${esc(t("en_later"))}"><i class="fa-solid fa-chevron-right"></i></button>
            <div class="sx-seg">${SPANS.map(([k, m]) => `<button type="button" data-span="${m}" class="${r.span === m && !r.custom ? "on" : ""}">${esc(t(k))}</button>`).join("")}<button type="button" data-custom class="${r.custom ? "on" : ""}">${esc(t("en_custom"))}</button></div>
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
                const pts = data[s.id] || [], st = { ...(stats(pts) || { min: 0, max: 1 }) };
                // Keep an opened alert's limit in view.
                if (alertOn({ series: s }) && alertView.limits.length) {
                    alertView.limits.forEach(l => { const v = alertLimit(s, l.v); st.min = Math.min(st.min, v); st.max = Math.max(st.max, v); });
                }
                const half = Math.max((st.max - st.min) / 2 * 1.2, s.span / 2, 1e-6), mid = (st.max + st.min) / 2;
                const c = new Chart(document.getElementById("sxc_" + s.id), {
                    type: "line",
                    data: { datasets: [{ data: pts, borderColor: s.color, backgroundColor: s.color + "22", fill: true, borderWidth: 1.8, pointRadius: 0, tension: .3 }] },
                    plugins: [alertMark, marker],
                    options: {
                        animation: false, maintainAspectRatio: false, parsing: false, normalized: true,
                        plugins: { legend: { display: false }, tooltip: { enabled: false } },
                        // No y numbers: keeps every chart and the device lane lined up; min/max/avg are in the label column.
                        scales: { x: timeScale(r.from, r.to, i === vis.length - 1), y: { min: mid - half, max: mid + half, display: false } },
                    },
                });
                c.$sx = { series: s, first: i === 0 };
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
                return `<div class="sx-dev"><i class="fa-solid ${d.icon}"></i><span title="${esc(d.name)}">${esc(d.name)}</span><b>${v == null ? esc(t("en_off")) : esc(K.portText(d, v, ONOFF))}</b></div>`;
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
                        data: (data[d.id] || []).map(p => ({ x: p.x, y: base + K.portLevel(d, p.y) / 100 * 0.78, raw: p.y })) };
                }) },
                plugins: [alertMark, marker],
                options: {
                    animation: false, maintainAspectRatio: false, parsing: false,
                    plugins: { legend: { display: false }, tooltip: { enabled: false } },
                    scales: { x: { ...timeScale(r.from, r.to, false), grid: { color: "rgba(100,116,139,.08)" } }, y: { min: 0, max: n, display: false } },
                    layout: { padding: { top: 2, bottom: 2 } },
                },
            });
            lane.$sx = { devices, first: !vis.length };
            root.querySelectorAll(".sx-dev").forEach((el, i) => el.style.setProperty("--c", colors[i % colors.length]));
        }
        if (markX != null) showMarker();
    }

    // ---------- Events ----------
    function selectZone(id) {
        zoneId = id;
        alertView = null; alertID = null;
        store.set("isley.env.zone", String(id));
        history.replaceState(null, "", "#zone=" + id);
        loadHidden();
        data = {};
        r = r.live ? newRange(r.span) : r;
        load();
    }
    root.addEventListener("click", e => {
        const z = e.target.closest("[data-zone]");
        if (z) { if (+z.dataset.zone !== zoneId) selectZone(+z.dataset.zone); return; }
        const tg = e.target.closest("[data-toggle]");
        if (tg) { const id = +tg.dataset.toggle; hidden.has(id) ? hidden.delete(id) : hidden.add(id); saveHidden(); render(); return; }
        if (e.target.closest("[data-clearmark]")) { markX = null; showMarker(); return; }
        const b = e.target.closest("[data-span],[data-step],[data-custom],[data-applycustom]");
        if (!b) return;
        if (b.dataset.span) { r = newRange(+b.dataset.span); store.set("isley.env.span", b.dataset.span); load(); }
        else if (b.dataset.step) { stepRange(+b.dataset.step); load(); }
        else if (b.hasAttribute("data-custom")) { r = { ...r, custom: !r.custom }; render(); }
        else if (b.hasAttribute("data-applycustom")) {
            const f = new Date(document.getElementById("sxFrom").value).getTime(), to = new Date(document.getElementById("sxTo").value).getTime();
            if (f < to) { const now = Date.now(); r = { ...r, from: f, to: Math.min(to, now), live: to >= now, custom: true }; load(); }
        }
    });
    // Press anywhere on a chart to put the marker line there and drag to move
    // it; the line stays put when the button is let go (moving the mouse alone
    // never moves it). On a touch screen a sideways drag moves the line and an
    // up-and-down swipe still scrolls the page.
    const card = document.getElementById("sxCard");
    const chartAt = target => {
        const canvas = target && target.closest && target.closest(".sx-m-chart canvas, .sx-lane-chart canvas");
        return canvas ? Chart.getChart(canvas) : null;
    };
    const pxIn = (chart, clientX) => clientX - chart.canvas.getBoundingClientRect().left;
    const inArea = (chart, clientX) => { const a = chart.chartArea, px = pxIn(chart, clientX); return px >= a.left && px <= a.right; };
    function moveMarkTo(chart, clientX) {
        const area = chart.chartArea, px = Math.max(area.left, Math.min(area.right, pxIn(chart, clientX)));
        markX = chart.scales.x.getValueForPixel(px);
        showMarker();
    }
    let dragging = null;
    card.addEventListener("mousedown", e => {
        const chart = chartAt(e.target);
        if (!chart || e.button !== 0 || !inArea(chart, e.clientX)) return;
        dragging = chart;
        e.preventDefault();   // no text selection while dragging
        moveMarkTo(chart, e.clientX);
    });
    document.addEventListener("mousemove", e => { if (dragging) moveMarkTo(dragging, e.clientX); });
    document.addEventListener("mouseup", () => { dragging = null; });
    let touch = null;
    card.addEventListener("touchstart", e => {
        const chart = chartAt(e.target), p = e.touches[0];
        if (!chart || e.touches.length !== 1 || !inArea(chart, p.clientX)) return;
        touch = { chart, x: p.clientX, y: p.clientY, mode: null };   // mode: "line" or "scroll", decided by the first move
    }, { passive: true });
    card.addEventListener("touchmove", e => {
        if (!touch) return;
        const p = e.touches[0], dx = Math.abs(p.clientX - touch.x), dy = Math.abs(p.clientY - touch.y);
        if (!touch.mode && (dx > 6 || dy > 6)) touch.mode = dx > dy ? "line" : "scroll";
        if (touch.mode === "line") { e.preventDefault(); moveMarkTo(touch.chart, p.clientX); }
    }, { passive: false });
    card.addEventListener("touchend", () => {
        if (touch && !touch.mode) moveMarkTo(touch.chart, touch.x);   // a tap marks that moment
        touch = null;
    });
    document.addEventListener("keydown", e => { if (e.key === "Escape" && markX != null) { markX = null; showMarker(); } });

    window.addEventListener("hashchange", () => {
        const al = hashAlert();
        if (al) { if (al !== alertID) openAlert(al); return; }
        const h = hashZone(); if (h && h !== zoneId && zones.some(z => z.id === h)) selectZone(h);
    });

    // Live: while the range ends now, follow new readings.
    const every = Math.max(30, P.poll || 60) * 1000;
    setInterval(() => {
        if (document.hidden || !zoneId || !r.live || loading) return;
        // An opened alert that's still active keeps its start and follows new readings.
        r = alertView && r.custom ? { ...r, to: Date.now() } : { ...newRange(r.span), custom: false };
        load();
    }, every);

    (async () => {
        const al = hashAlert();
        if (al && await openAlert(al)) return;
        pickZone();
        if (zoneId) { loadHidden(); history.replaceState(null, "", "#zone=" + zoneId); load(); }
        else render();
    })();
})();
