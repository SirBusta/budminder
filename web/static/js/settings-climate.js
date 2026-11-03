// Settings → Grow setup: the Climate targets and limits card (Low · Target ·
// High per stage for VPD, temperature and humidity, saved straight away to
// POST /settings/climate), the Zones table's Light schedule and Now columns,
// the Edit zone window (light schedule, light check, plant stage and climate
// sensors, saved to PUT /zones/:id), and Settings → Alerts' Zone status, which
// follows edits on the card. Data comes from #clData, strings from #clText.
// Temperatures are stored in °C and shown in the measurement units chosen in
// Settings. A value that doesn't fit its row stays in its box, in red, and the
// last valid value stays in use.
(function () {
    "use strict";
    const $ = id => document.getElementById(id);
    const card = $("climateCard");
    if (!card) return;
    const D = JSON.parse($("clData").textContent || "{}");
    const T = JSON.parse($("clText").textContent || "{}");
    const S = D.settings || {};
    const ZONES = D.zones || [];
    const esc = v => String(v).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const fill = (s, o) => String(s || "").replace(/\{(\w+)\}/g, (m, k) => (k in o ? o[k] : m));
    const imperial = D.units !== "metric";

    // ---------- units ----------
    const UNIT = {
        vpd: { unit: "kPa", step: 0.05, dp: 2 },
        temp: imperial ? { unit: "°F", step: 1, dp: 0 } : { unit: "°C", step: 0.5, dp: 1 },
        rh: { unit: "%", step: 1, dp: 0 },
    };
    const toDisp = (m, v) => v == null ? null : m === "temp" && imperial ? v * 9 / 5 + 32 : v;
    const fromDisp = (m, v) => m === "temp" && imperial ? Math.round((v - 32) * 5 / 9 * 10000) / 10000 : v;
    const fmt = (m, v) => v == null || isNaN(v) ? "" : toDisp(m, v).toFixed(UNIT[m].dp);

    // ---------- model ----------
    const MEASURES = ["vpd", "temp", "rh"];
    const key = (stage, phase, k, lights) => `${stage}|${phase}|${k}|${lights}`;
    const rows = {};
    (S.targets || []).forEach(t => { rows[key(t.stage, t.phase, t.measure, t.lights)] = [t.low, t.target, t.high]; });
    const limits = { temp: (S.temp_limits || [null, null]).slice(), rh: (S.rh_limits || [null, null]).slice() };
    const byStage = { vpd: true, temp: !!S.temp_by_stage, rh: !!S.rh_by_stage };
    const split = {}, phases = {};
    MEASURES.forEach((k, i) => { split[k] = !!(S.split_lights || [])[i]; phases[k] = !!(S.flower_phases || [])[i]; });
    let weeks = (S.phase_weeks || [4, 8]).slice(); // one set for all three measures
    let m = "vpd";

    const phaseOf = w => w <= weeks[0] ? "stretch" : w <= weeks[1] ? "bloom" : "ripen";
    // Zones whose readings are compared: a stage and at least one climate sensor.
    const watched = ZONES.filter(z => z.now && z.now.stage && z.now.stage.stage && Object.keys(z.now.sensors || {}).length);
    function rowOf(z, k) {
        const st = z.now.stage.stage;
        if (st !== "flower" || !phases[k]) return { stage: st, phase: "" };
        return { stage: st, phase: phaseOf(z.now.stage.week || 1) };
    }
    const lightsPart = (z, stage, k) => stage === "drying" ? "off" : (!z.now.lights_on && split[k] ? "off" : "on");
    const limitsOnly = k => k !== "vpd" && !byStage[k];
    // The Low, Target and High in use for a zone now, in stored units.
    function rangeFor(z, k) {
        if (limitsOnly(k)) return [limits[k][0], null, limits[k][1]];
        const r = rowOf(z, k);
        return rows[key(r.stage, r.phase, k, lightsPart(z, r.stage, k))] || [null, null, null];
    }

    function stages() {
        const s = [{ stage: "seedling", phase: "", name: T.seedling }, { stage: "veg", phase: "", name: T.veg }];
        if (phases[m]) {
            s.push({ head: true, name: T.flower });
            s.push({ stage: "flower", phase: "stretch", name: T.stretch, sub: true, wk: fill(T.weeksFrom, { n: 1 }) + `<input class="wk" data-wk="0" value="${weeks[0]}" aria-label="${esc(T.stretch)}">` });
            s.push({ stage: "flower", phase: "bloom", name: T.bloom, sub: true, wk: fill(T.weeksFrom, { n: weeks[0] + 1 }) + `<input class="wk" data-wk="1" value="${weeks[1]}" aria-label="${esc(T.bloom)}">` });
            s.push({ stage: "flower", phase: "ripen", name: T.ripen, sub: true, wk: esc(fill(T.weekOn, { n: weeks[1] + 1 })) });
        } else s.push({ stage: "flower", phase: "", name: T.flower });
        if (m !== "vpd") s.push({ stage: "drying", phase: "", name: T.drying, wk: esc(T.lightsOff) });
        return s;
    }

    // ---------- grid ----------
    const NB = `<span class="ax-nb" title="${esc(T.notifies)}"><i class="fa-solid fa-envelope"></i><i class="fa-solid fa-bell"></i></span>`;
    function cell(stage, phase, lights, label) {
        const v = rows[key(stage, phase, m, lights)] || [null, null, null];
        return `<div class="ct-cell" data-stage="${stage}" data-phase="${phase}" data-lights="${lights}"><span class="ct-colsub">${label}</span>` +
            ["lh", "t", "lh"].map((c, i) => `<input class="${c}" type="number" step="${UNIT[m].step}" value="${fmt(m, v[i])}" aria-label="${esc([T.low, T.target, T.high][i])}">`)
                .join('<span class="dot">·</span>') + `<span class="u">${UNIT[m].unit}</span></div>`;
    }
    function labels(names) {
        return `<div class="ct-cell ct-lbl"><span class="ct-colsub"></span>` + names.map(n =>
            `<span class="l"><span>${esc(n)}</span>${n === T.target ? "" : NB}</span>`).join('<span class="dot">·</span>') +
            `<span class="u">${UNIT[m].unit}</span></div>`;
    }
    // A column heading centered over the three boxes, not the whole cell: the hidden copy sets the width.
    const hdc = html => `<div class="ct-hdc"><span>${html}</span><div class="ct-cell ct-hdg" aria-hidden="true">` +
        [0, 1, 2].map(() => '<input tabindex="-1">').join('<span class="dot">·</span>') + `</div></div>`;
    function limitsHtml() {
        const [lo, hi] = limits[m];
        return `<div class="ct-row hd"><div>${esc(T.limits)}</div><div class="ct-span">${esc(T.limitsAll)}</div></div>` +
            `<div class="ct-row lb"><div class="ct-alk">${NB}<span>${esc(T.limitsKey)}</span></div><div class="ct-span">${labels([T.low, T.high])}</div></div>` +
            `<div class="ct-row"><div class="ct-st"><b>${esc(T[m])}</b><small>${esc(T.leaveEmpty)}</small></div>` +
            `<div class="ct-span"><div class="ct-cell" data-lim="1"><span class="ct-colsub"></span>` +
            `<input class="lim" type="number" step="${UNIT[m].step}" value="${fmt(m, lo)}" aria-label="${esc(T.low)}">` +
            `<span class="dot">·</span><input class="lim" type="number" step="${UNIT[m].step}" value="${fmt(m, hi)}" aria-label="${esc(T.high)}">` +
            `<span class="u">${UNIT[m].unit}</span></div></div></div>`;
    }
    function tags(s) {
        return watched.filter(z => { const r = rowOf(z, m); return r.stage === s.stage && r.phase === s.phase; })
            .map(z => `<span class="ct-nowtag" title="${esc(fill(T.inStage, { zone: z.name }))}"><i class="fa-solid fa-tent"></i>${esc(z.name)}</span>`).join("");
    }
    function render() {
        const lim = limitsOnly(m);
        $("clByStageL").style.display = m === "vpd" ? "none" : "";
        $("clByStage").checked = byStage[m];
        $("clNight").checked = split[m];
        $("clPhases").checked = phases[m];
        $("clNightL").style.display = lim ? "none" : "";
        $("clPhasesL").style.display = lim ? "none" : "";
        $("clLimNote").classList.toggle("d-none", !lim);
        $("clStageNote").classList.toggle("d-none", m !== "vpd");
        if (lim) { $("clGrid").innerHTML = limitsHtml(); preview(); return; }
        const night = split[m];
        let h = `<div class="ct-row hd"><div>${esc(T.stageHead)}</div>` +
            (night ? `<div>${hdc(`<i class="fa-solid fa-sun"></i>${esc(T.lightsOn)}`)}</div><div>${hdc(`<i class="fa-solid fa-moon"></i>${esc(T.lightsOff)}`)}</div>`
                : `<div class="ct-span">${hdc(esc(T.lightsOnOff))}</div>`) + `</div>`;
        const lt = labels([T.low, T.target, T.high]);
        h += `<div class="ct-row lb"><div class="ct-alk">${NB}<span>${esc(T.key)}</span></div>` +
            (night ? `<div>${lt}</div><div>${lt}</div>` : `<div class="ct-span">${lt}</div>`) + `</div>`;
        const on = `<i class="fa-solid fa-sun"></i> ${esc(T.on)}`, off = `<i class="fa-solid fa-moon"></i> ${esc(T.off)}`;
        for (const s of stages()) {
            const tg = s.head ? "" : tags(s);
            h += `<div class="ct-row${tg ? " now" : ""}"><div class="ct-st${s.sub ? " ct-sub" : ""}"><b>${esc(s.name)}</b>${tg}` +
                (s.wk ? `<small>${s.wk}</small>` : "") + `</div>`;
            if (s.head) h += `<div class="ct-span"></div>`;
            else if (s.stage === "drying") h += `<div class="ct-span">${cell("drying", "", "off", off)}</div>`;
            else if (night) h += `<div>${cell(s.stage, s.phase, "on", on)}</div><div>${cell(s.stage, s.phase, "off", off)}</div>`;
            else h += `<div class="ct-span">${cell(s.stage, s.phase, "on", "")}</div>`;
            h += `</div>`;
        }
        $("clGrid").innerHTML = h;
        preview();
    }

    // A Target sits strictly between its limits, because a reading at a limit starts an alert.
    function rowError([lo, t, hi]) {
        if (lo != null && hi != null && lo >= hi) return T.errLoHi;
        if (t == null) return "";
        if (lo != null && hi != null) return t > lo && t < hi ? "" : T.errBetween;
        if (hi != null && t >= hi) return T.errBelow;
        if (lo != null && t <= lo) return T.errAbove;
        return "";
    }
    // Checks a row as typed and keeps what fits. Returns whether anything changed.
    function checkRow(row) {
        let msg = "", changed = false;
        row.querySelectorAll(".ct-cell:not(.ct-lbl)").forEach(c => {
            const boxes = [...c.querySelectorAll("input")], vals = boxes.map(b => b.value === "" ? null : parseFloat(b.value));
            const err = rowError(boxes.length === 2 ? [vals[0], null, vals[1]] : vals);
            boxes.forEach((b, i) => b.classList.toggle("ct-bad", !!err && String(i) === c.dataset.last));
            msg = msg || err;
            if (err || !c.dataset.last) return;
            const stored = c.dataset.lim ? limits[m] : (rows[key(c.dataset.stage, c.dataset.phase, m, c.dataset.lights)] ||= [null, null, null]);
            boxes.forEach((b, i) => {
                // Only a box that changed: showing a value rounds it.
                if (b.value !== fmt(m, stored[i])) { stored[i] = vals[i] == null ? null : fromDisp(m, vals[i]); changed = true; }
            });
        });
        const next = row.nextElementSibling, has = next && next.classList.contains("ct-err");
        if (has && !msg) next.remove();
        else if (has) next.lastChild.textContent = msg;
        else if (msg) row.insertAdjacentHTML("afterend", `<div class="ct-err" role="alert"><i class="fa-solid fa-circle-exclamation me-1"></i>${esc(msg)}</div>`);
        return changed;
    }

    // ---------- bars: the same rule for the preview and Zone status ----------
    // Red at Low and High, green near Target, amber between the two, the track
    // everywhere else. All in the units shown.
    const NEAR = { vpd: 0.2, temp: imperial ? 5 : 2.8, rh: 7 };   // "near Target" when a side has no limit
    const SPAN = { vpd: 0.5, temp: imperial ? 15 : 8, rh: 20 };   // how far the bar reaches from Target when a side has no limit
    const shown = (k, r) => r.map(v => toDisp(k, v));
    function scale(k, r, v) {
        const [lo, t, hi] = r, c = t ?? v ?? 0, w = SPAN[k], pad = lo != null && hi != null ? (hi - lo) * 0.2 : w * 0.25;
        const min = (lo ?? c - w) - pad, max = (hi ?? c + w) + pad;
        return x => Math.max(0, Math.min(100, (x - min) / (max - min) * 100));
    }
    function layers(k, r, pos) {
        const [lo, t, hi] = r, N = NEAR[k];
        const seg = (a, b, cls) => `<i class="${cls}" style="left:${pos(a)}%;width:${pos(b) - pos(a)}%"></i>`;
        let h = "";
        if (lo != null) h += seg(-1e9, lo, "c-mail");
        if (hi != null) h += seg(hi, 1e9, "c-mail");
        if (t == null) return h;
        const nl = lo != null ? t - (t - lo) / 2 : t - N, nh = hi != null ? t + (hi - t) / 2 : t + N;
        if (lo != null) h += seg(lo, nl, "c-toward");
        if (hi != null) h += seg(nh, hi, "c-toward");
        // A side with no limit fades out, so its end does not look like a limit.
        const a = pos(lo != null ? nl : t - N * 1.5), b = pos(hi != null ? nh : t + N * 1.5), w = (b - a) || 1;
        const left = lo != null ? "var(--ax-ok)" : `transparent,var(--ax-ok) ${(pos(t - N * 0.6) - a) / w * 100}%`;
        const right = hi != null ? "var(--ax-ok)" : `var(--ax-ok) ${(pos(t + N * 0.6) - a) / w * 100}%,transparent`;
        return h + `<i style="left:${a}%;width:${w}%;background:linear-gradient(90deg,${left},${right})"></i>`;
    }
    // Names the parts of the bar. Amber shows only where a row has both a Target and a limit.
    function legend(caution) {
        const it = (cls, t) => `<span><u class="${cls}"></u>${esc(t)}</span>`;
        return it("c-ok", T.legTarget) + (caution ? it("c-toward", T.legCaution) : "") + it("c-track", T.legNone) +
            it("c-mail", T.legAlert) + it("ax-bkk", T.legReading) + it("ax-tkk", T.legTick);
    }
    const hasCaution = r => r[1] != null && (r[0] != null || r[2] != null);

    // ---------- preview: the first watched zone with a reading ----------
    function preview() {
        const z = watched.find(z => z.readings && z.readings[m] && z.readings[m].value != null && !z.readings[m].stale);
        if (!z) { $("clPv").innerHTML = `<div class="ct-pv-h text-muted">${esc(T.pvNone)}</div>`; return; }
        const now = toDisp(m, z.readings[m].value), rw = rowOf(z, m), part = lightsPart(z, rw.stage, m);
        const stageTxt = rw.stage === "flower"
            ? (rw.phase ? fill(T.flowerPhase, { phase: T[rw.phase] }) : T.flower) + " (" + fill(T.week, { n: z.now.stage.week }) + ")"
            : T[rw.stage];
        const lightTxt = z.now.scheduled || rw.stage === "drying"
            ? ` · <i class="fa-solid ${part === "off" && rw.stage !== "drying" || !z.now.lights_on ? "fa-moon" : "fa-sun ct-sun"}"></i> ${esc(z.now.lights_on && rw.stage !== "drying" ? T.lightsOn : T.lightsOff)}` : "";
        const f = v => v.toFixed(UNIT[m].dp);
        const head = (cls, ic, txt) => `<div class="ct-pv-h"><span>${esc(fill(T.zoneNow, { zone: z.name }))} · ${esc(stageTxt)}${lightTxt}</span>` +
            `<span class="st ${cls}"><i class="fa-solid ${ic} me-1"></i>${esc(T[m])} ${f(now)}${m === "rh" ? "" : " "}${UNIT[m].unit}: ${esc(txt)}</span></div>`;
        const r = shown(m, rangeFor(z, m)), [lo, t, hi] = r;
        if (lo == null && t == null && hi == null) {
            $("clPv").innerHTML = head("", "fa-circle-minus", T.pvNoneSet) + `<div class="ct-band tr"><div class="ax-b"></div><span class="mk" style="left:50%">${f(now)}</span></div>`;
            return;
        }
        const pos = scale(m, r, now), reached = (lo != null && now <= lo) || (hi != null && now >= hi);
        const nl = t == null ? null : lo != null ? t - (t - lo) / 2 : t - NEAR[m], nh = t == null ? null : hi != null ? t + (hi - t) / 2 : t + NEAR[m];
        const near = t != null && now >= nl && now <= nh, limSide = t != null && (now < t ? lo : hi) != null;
        const txt = reached ? T.pvAlert : near ? T.pvTarget : t == null ? T.pvNoTarget : limSide ? T.pvCaution : T.pvFar;
        const cls = reached ? "cl-r" : near ? "cl-g" : limSide ? "cl-a" : "";
        const ic = reached ? "fa-circle-exclamation" : near ? "fa-circle-check" : limSide ? "fa-circle-half-stroke" : "fa-circle-minus";
        $("clPv").innerHTML = head(cls, ic, txt) +
            `<div class="ct-band tr"><div class="ax-b">${layers(m, r, pos)}</div>${t != null ? `<span class="ax-tk" style="left:${pos(t)}%"></span>` : ""}<span class="mk" style="left:${pos(now)}%">${f(now)}</span></div>` +
            `<div class="ct-ticks">` + (lo != null ? `<span style="left:${pos(lo)}%">${esc(T.low)} ${f(lo)}</span>` : "") +
            (t != null ? `<span style="left:${pos(t)}%">${esc(T.target)} ${f(t)}</span>` : "") +
            (hi != null ? `<span style="left:${pos(hi)}%">${esc(T.high)} ${f(hi)}</span>` : "") + `</div>` +
            `<div class="ax-legend mt-2">${legend(hasCaution(r))}</div>`;
    }

    // ---------- Settings → Alerts: Zone status ----------
    const cardLink = `<a href="#grow" data-goto="grow" data-goto-card="climateCard">${esc(T.cardName)}</a>`;
    const zoneLink = z => `<a href="#" data-zone-open="${z.id}">${esc(T.editZone)}</a>`;
    function zonePill(z) {
        const list = z.alerts || [];
        if (list.length) {
            const words = list.map(a => a.kind === "offline" ? fill(T.zOffline, { time: a.since })
                : a.kind === "light" ? T.zLightBad : a.kind === "clock" ? T.zClock
                : fill(a.side === "low" ? T.zLow : T.zHigh, { measure: T[a.measure] || "" }));
            const a = list[0];
            const how = a.delivery === "sent" ? (a.kind === "clock" ? fill(T.zSentOnce, { sent: a.sent }) : fill(T.zSent, { sent: a.sent, next: a.next }))
                : a.delivery === "off" ? T.zNotSent : a.delivery === "paused" ? T.zNotSentPaused : a.delivery === "failed" ? T.zFailed : a.delivery === "pending" ? T.zSending : "";
            return `<span class="ax-pill bad"><i class="fa-solid fa-circle-exclamation me-1"></i>${esc(words.join(" "))} ${esc(how)}</span>`;
        }
        const any = Object.keys(z.now.sensors).some(k => { const r = rangeFor(z, k); return r[0] != null || r[2] != null; });
        return any || z.light_sensor_id ? `<span class="ax-pill ok"><i class="fa-solid fa-circle-check me-1"></i>${esc(T.pillOk)}</span>`
            : `<span class="ax-pill"><i class="fa-solid fa-circle-minus me-1"></i>${esc(T.pillNone)}</span>`;
    }
    function zoneBar(k, r, v) {
        const pos = scale(k, r, v), [lo, t, hi] = r, f = x => x.toFixed(UNIT[k].dp);
        return `<div class="ax-bw"><div class="ax-b${v == null ? " none" : ""}">${layers(k, r, pos)}</div>` +
            (t != null ? `<span class="ax-tk" style="left:${pos(t)}%"></span>` : "") +
            (v == null ? "" : `<span class="ax-bk" style="left:${pos(v)}%"></span>`) +
            `</div><div class="ax-bt">` + (lo != null ? `<span style="left:${pos(lo)}%">${f(lo)}</span>` : "") +
            (t != null ? `<span style="left:${pos(t)}%">${esc(T.target)} ${f(t)}</span>` : "") +
            (hi != null ? `<span style="left:${pos(hi)}%">${f(hi)}</span>` : "") + `</div>`;
    }
    function zoneHtml(z) {
        const icon = /outdoor|outside/i.test(z.name) ? "fa-tree" : "fa-tent";
        const name = `<b><i class="fa-solid ${icon}"></i>${esc(z.name)}</b>`;
        const stage = z.now.stage.stage;
        if (!stage) return `<div class="ax-zone"><div class="ax-zone-h">${name}<span class="ax-pill"><i class="fa-solid fa-circle-minus me-1"></i>${esc(T.zNoPlants)}</span></div></div>`;
        let stageTxt = T[stage];
        if (stage === "flower") stageTxt += ", " + fill(T.week, { n: z.now.stage.week });
        const lit = z.now.scheduled || stage === "drying";
        const lightTxt = lit ? ` · <i class="fa-solid ${z.now.lights_on ? "fa-sun" : "fa-moon"}"></i> ${esc(z.now.lights_on ? T.lightsOn : T.lightsOff)}` : "";
        const sensors = Object.keys(z.now.sensors || {});
        if (!sensors.length && !z.light_sensor_id) {
            return `<div class="ax-zone"><div class="ax-zone-h">${name}<span>${esc(stageTxt)}</span><span class="ax-pill"><i class="fa-solid fa-circle-minus me-1"></i>${esc(T.zNoSensors)} ${zoneLink(z)}</span></div></div>`;
        }
        let h = `<div class="ax-zone"><div class="ax-zone-h">${name}<span>${esc(stageTxt)}${lightTxt}</span>${zonePill(z)}</div>`;
        for (const k of MEASURES) {
            if (!z.now.sensors[k] || (k === "vpd" && stage === "drying")) continue;
            const rd = z.readings[k], v = rd && rd.value != null && !rd.stale ? toDisp(k, rd.value) : null;
            const r = shown(k, rangeFor(z, k)), bad = v != null && ((r[0] != null && v <= r[0]) || (r[2] != null && v >= r[2]));
            const label = `<div class="n"><b>${esc(T[k])}</b>${v == null ? `<span class="text-muted">—</span>` : `<span class="${bad ? "bad" : ""}">${v.toFixed(UNIT[k].dp)}${k === "rh" ? "" : " "}${UNIT[k].unit}</span>`}</div>`;
            if (r[0] == null && r[1] == null && r[2] == null) { h += `<div class="ax-m">${label}<div class="plain">${esc(T.rowNone)} ${cardLink}</div></div>`; continue; }
            h += `<div class="ax-m">${label}<div>${zoneBar(k, r, v)}</div></div>`;
        }
        let light;
        if (!z.light_sensor_id || !z.now.scheduled) light = `<i class="fa-solid fa-circle-minus me-1"></i>${esc(T.zNoLight)} ${zoneLink(z)}`;
        else if (z.light && z.light.clock) light = `<i class="fa-solid fa-circle-exclamation me-1"></i>${esc(fill(z.light.early ? T.liveClockEarly : T.liveClockLate, { minutes: z.light.minutes }))}`;
        else if (z.light && z.light.mismatch) light = `<i class="fa-solid fa-circle-exclamation me-1"></i>${esc(z.light.seen_on_now ? T.liveMismatchOn : T.liveMismatchOff)}`;
        else if (z.now.light_seen == null) light = `<i class="fa-solid fa-circle-minus me-1"></i>${esc(T.zLightStale)}`;
        else light = `<i class="fa-solid fa-circle-check me-1"></i>${esc(z.now.light_seen ? T.zLightOn : T.zLightOff)}`;
        h += `<div class="ax-m"><div class="n"><b>${esc(T.zLights)}</b></div><div class="plain">${light}</div></div>`;
        let sens;
        const off = (z.alerts || []).find(a => a.kind === "offline" && a.sensors);
        if (off) sens = `<i class="fa-solid fa-circle-exclamation me-1"></i>${esc(fill(T.zOfflineFrom, { sensors: off.sensors }))}`;
        else if (!z.last_at) sens =`<i class="fa-solid fa-circle-minus me-1"></i>${esc(T.zNoReadings)}`;
        else if (z.offline) sens = `<i class="fa-solid fa-circle-exclamation me-1"></i>${esc(fill(T.zLastAt, { time: z.last_clock }))}`;
        else sens = `<i class="fa-solid fa-circle-check me-1"></i>${esc(fill(T.zLastAgo, { n: Math.max(1, Math.round((Date.now() - z.last_at) / 60000)) }))}`;
        return h + `<div class="ax-m"><div class="n"><b>${esc(T.zSensors)}</b></div><div class="plain">${sens}</div></div></div>`;
    }
    function drawZones() {
        const box = $("alZones");
        if (!box) return;
        box.innerHTML = ZONES.map(zoneHtml).join("");
        const ranges = watched.flatMap(z => Object.keys(z.now.sensors).map(k => [k, rangeFor(z, k)]));
        const vpd = ranges.filter(([k]) => k === "vpd");
        $("alLegend").innerHTML = legend(ranges.some(([, r]) => hasCaution(r)));
        const noVpdLimit = vpd.length && vpd.every(([, r]) => r[0] == null && r[2] == null);
        $("alZNote").innerHTML = esc(noVpdLimit ? T.zNoteVpd : T.zNoteSet).replace("{link}", cardLink);
    }
    $("alZones") && $("alZones").addEventListener("click", e => {
        const a = e.target.closest("[data-zone-open]");
        if (a) { e.preventDefault(); openZone(+a.dataset.zoneOpen); }
    });

    // What Settings → Alerts needs to say whether anything can send a notification.
    window.bmClimate = {
        anyLimit() {
            for (const k of ["temp", "rh"]) if (limitsOnly(k) && (limits[k][0] != null || limits[k][1] != null)) return true;
            return Object.entries(rows).some(([rk, v]) => {
                const [stage, phase, k, lights] = rk.split("|");
                if (limitsOnly(k) || (v[0] == null && v[2] == null)) return false;
                if (lights === "off" && stage !== "drying" && !split[k]) return false;
                return stage !== "flower" || (phase !== "") === phases[k];
            });
        },
        anyLight: () => ZONES.some(z => z.light_sensor_id && z.light_schedule && Object.keys(z.light_schedule).length),
    };
    const changed = () => { drawZones(); document.dispatchEvent(new CustomEvent("bm:climate-change")); };

    // ---------- saving ----------
    let timer = null;
    function payload() {
        const targets = [];
        Object.entries(rows).forEach(([k, v]) => {
            const [stage, phase, measure, lights] = k.split("|");
            targets.push({ stage, phase, measure, lights, low: v[0], target: v[1], high: v[2] });
        });
        return {
            split_lights: MEASURES.map(k => split[k]), flower_phases: MEASURES.map(k => phases[k]), phase_weeks: weeks,
            temp_by_stage: byStage.temp, rh_by_stage: byStage.rh,
            temp_limits: limits.temp, rh_limits: limits.rh, targets,
        };
    }
    function save() {
        fetch("/settings/climate", {
            method: "POST",
            headers: { "Content-Type": "application/json", Accept: "application/json" },
            body: JSON.stringify(payload()),
        }).then(async r => {
            if (!r.ok) { const d = await r.json().catch(() => ({})); throw new Error(d.error || T.failed); }
            const saved = $("stSaved");
            if (saved) { saved.classList.add("show"); setTimeout(() => saved.classList.remove("show"), 1800); }
            refreshZones();
        }).catch(e => window.uiMessages && uiMessages.showToast(e.message || T.failed, "danger"));
    }
    // Zone status shows the readings from when the page loaded: fetch fresh
    // ones after each save.
    function refreshZones() {
        fetch("/settings/climate/zones", { headers: { Accept: "application/json" } })
            .then(r => r.ok ? r.json() : null)
            .then(d => {
                if (!d || !d.zones) return;
                d.zones.forEach(f => { const z = zoneById(f.id); if (z) Object.assign(z, f); });
                changed();
            }).catch(() => {});
    }
    const queue = () => { clearTimeout(timer); timer = setTimeout(save, 600); changed(); };

    $("clGrid").addEventListener("input", e => {
        const i = e.target;
        if (i.dataset.wk) return;
        const c = i.closest(".ct-cell");
        c.dataset.last = [...c.querySelectorAll("input")].indexOf(i);
        if (checkRow(c.closest(".ct-row"))) { preview(); queue(); }
    });
    $("clGrid").addEventListener("change", e => {
        const i = e.target;
        if (!i.dataset.wk) return;
        const v = parseInt(i.value, 10);
        if (v > 0) {
            if (i.dataset.wk === "0") weeks[0] = Math.min(v, weeks[1] - 1);
            else weeks[1] = Math.min(30, Math.max(v, weeks[0] + 1));
        }
        render(); queue(); zonesTable();
    });
    $("clMeasure").addEventListener("click", e => {
        const b = e.target.closest("button");
        if (!b) return;
        m = b.dataset.m;
        $("clMeasure").querySelectorAll("button").forEach(x => x.classList.toggle("on", x === b));
        render();
    });
    $("clNight").addEventListener("change", () => { split[m] = $("clNight").checked; render(); queue(); });
    $("clPhases").addEventListener("change", () => { phases[m] = $("clPhases").checked; render(); queue(); zonesTable(); });
    $("clByStage").addEventListener("change", () => { byStage[m] = $("clByStage").checked; render(); queue(); });

    // ---------- climate sensors by zone ----------
    function sensorsList() {
        const box = $("clZs");
        box.querySelectorAll(".ct-zs-r").forEach(r => r.remove());
        ZONES.forEach(z => {
            const names = [];
            const opt = id => (z.sensor_options || []).find(s => s.id === id);
            [z.vpd_temp_sensor_id, z.vpd_humidity_sensor_id].forEach(id => { const s = id && opt(id); if (s) names.push(s.name); });
            if (z.vpd_source === "sensor" && opt(z.vpd_sensor_id)) names.push(opt(z.vpd_sensor_id).name);
            if (z.vpd_source === "calc" && (z.now.sensors || {}).vpd) names.push(T.calcVpd);
            const row = document.createElement("div");
            row.className = "ct-zs-r";
            row.innerHTML = `<b>${esc(z.name)}</b>` + (names.length
                ? `<span><i class="fa-solid fa-circle-check cl-g me-1"></i>${esc(names.join(", "))}</span>`
                : `<span class="text-muted"><i class="fa-solid fa-circle-minus me-1"></i>${esc(T.zsNone)}</span>`) +
                `<a href="#" data-zone-open="${z.id}">${esc(T.editZone)}</a>`;
            box.appendChild(row);
        });
        box.addEventListener("click", e => {
            const a = e.target.closest("[data-zone-open]");
            if (a) { e.preventDefault(); openZone(+a.dataset.zoneOpen); }
        });
    }

    // ---------- zones table ----------
    const clock = t => (t || "").replace(/^0(\d)/, "$1");
    const statusLabel = s => T[(s || "").toLowerCase()] || s;
    function zonesTable() {
        ZONES.forEach(z => {
            const sc = document.querySelector(`.zn-sched[data-zone-id="${z.id}"]`), nc = document.querySelector(`.zn-nowc[data-zone-id="${z.id}"]`);
            if (!sc || !nc) return;
            const stage = z.now.stage.stage;
            const sched = z.light_schedule;
            if (!sched || !Object.keys(sched).length) sc.innerHTML = `<span class="zn-src">${esc(T.noSchedule)}</span>`;
            else {
                const t = sched[stage] || sched.flower || sched.veg || sched.seedling;
                const hrs = (() => { const [a, b] = t.map(x => { const [h, mi] = x.split(":").map(Number); return h * 60 + mi; }); return ((b - a + 1440) % 1440 || 1440) / 60; })();
                sc.innerHTML = `<span class="zn-src"><i class="fa-regular fa-clock"></i>${esc(fill(T.schedShort, { on: clock(t[0]), h: +hrs.toFixed(1) }))}` +
                    (z.light_sensor_id ? ` <i class="fa-solid fa-lightbulb ms-1 cl-g" title="${esc(T.lightChecked)}"></i>` : "") + `</span>`;
            }
            if (!stage) { nc.innerHTML = `<span class="zn-now text-muted">—</span>`; return; }
            let txt = T[stage];
            if (stage === "flower") txt += ", " + fill(T.week, { n: z.now.stage.week });
            const lit = z.now.scheduled || stage === "drying";
            nc.innerHTML = `<span class="zn-now">` + (lit ? `<i class="fa-solid ${z.now.lights_on ? "fa-sun" : "fa-moon"}"></i>${esc(z.now.lights_on ? T.lightsOn : T.lightsOff)} · ` : "") + `${esc(txt)}</span>`;
        });
    }

    // ---------- Edit zone ----------
    const zoneModalEl = $("editZoneModal");
    const zoneById = id => ZONES.find(z => z.id === id);
    const kindOf = s => window.SensorKinds ? SensorKinds.kind(s) : "other";
    let cur = null;
    function options(sel, list, selected, noneLabel) {
        sel.innerHTML = (noneLabel != null ? `<option value="">${esc(noneLabel)}</option>` : "") +
            list.map(s => `<option value="${s.id}"${s.id === selected ? " selected" : ""}>${esc(s.name)}${s.unit ? " (" + esc(s.unit) + ")" : ""}</option>`).join("");
    }
    const DEFAULT_SCHED = { seedling: ["06:00", "00:00"], veg: ["06:00", "00:00"], flower: ["06:00", "18:00"] };
    const mins = s => { const [h, mi] = (s || "0:0").split(":").map(Number); return h * 60 + mi; };
    function hours() {
        $("dzRows").querySelectorAll("tr").forEach(tr => {
            const a = tr.querySelector(".on").value, b = tr.querySelector(".off").value;
            tr.querySelector(".hrs").textContent = a && b ? fill(T.hours, { h: +(((mins(b) - mins(a) + 1440) % 1440 || 1440) / 60).toFixed(1) }) : "";
        });
    }
    let src = "sched", vpdSrc = "calc";
    function setSrc(v) {
        src = v;
        $("dzSrc").querySelectorAll("button").forEach(x => x.classList.toggle("on", x.dataset.v === v));
        $("dzNone").style.display = v === "none" ? "" : "none";
        $("dzSched").style.display = v === "none" ? "none" : "";
        $("dzCheckSec").style.display = v === "none" ? "none" : "";
        hours(); live();
    }
    function setVpdSrc(v) {
        vpdSrc = v;
        $("dzVpdSrc").querySelectorAll("button").forEach(x => x.classList.toggle("on", x.dataset.v === v));
        $("dzVpdSensorRow").style.display = v === "sensor" ? "" : "none";
        $("dzVpdCalcRow").style.display = v === "calc" ? "" : "none";
    }
    // Lights the edited schedule says now, for the zone's current stage.
    function expectedOn() {
        if (src === "none" || !cur) return null;
        const st = cur.now.stage.stage || "veg";
        if (st === "drying") return false;
        const tr = $("dzRows").querySelector(`tr[data-s="${st}"]`);
        if (!tr) return null;
        const a = tr.querySelector(".on").value, b = tr.querySelector(".off").value;
        if (!a || !b) return null;
        const s = mins(a), e = mins(b), n = D.minutes;
        return s === e ? true : s < e ? n >= s && n < e : n >= s || n < e;
    }
    function live() {
        const id = parseInt($("dzLight").value, 10);
        $("dzSensor").style.display = id ? "" : "none";
        const el = $("dzLive");
        el.classList.remove("bad", "warn");
        if (!id || !cur) return;
        const s = (cur.sensor_options || []).find(x => x.id === id);
        if (!s || s.value == null || s.stale) { el.innerHTML = `<i class="fa-solid fa-circle-info me-1"></i>${esc(T.liveNoReading)}`; return; }
        const above = parseFloat($("dzAbove").value) || 0, seen = s.value > above, want = expectedOn();
        // What the background check found for the saved sensor and schedule.
        const lt = cur.light;
        if (lt && id === cur.light_sensor_id && (lt.clock || lt.mismatch)) {
            if (lt.clock) {
                el.classList.add("warn");
                el.innerHTML = `<i class="fa-solid fa-clock me-1"></i><b>${esc(fill(lt.early ? T.liveClockEarly : T.liveClockLate, { minutes: lt.minutes }))}</b> ` +
                    esc(fill(T.liveClockDays, { seen_on: lt.seen_on, seen_off: lt.seen_off, sched_on: lt.sched_on, sched_off: lt.sched_off }));
            } else {
                el.classList.add("bad");
                let h = `<i class="fa-solid fa-circle-exclamation me-1"></i><b>${esc(lt.seen_on_now ? T.liveMismatchOn : T.liveMismatchOff)}</b> ` +
                    esc(fill(T.liveSince, { time: lt.since, duration: lt.duration }));
                if (lt.alert === "sent") h += " " + esc(T.liveAlerted);
                else if (lt.alert) h += " " + esc(T.liveListed);
                if (lt.last_from) h += " " + esc(fill(T.liveSeen, { state: lt.seen_on_now ? T.readsOff : T.readsOn, from: lt.last_from, to: lt.last_to }));
                el.innerHTML = h;
            }
            return;
        }
        const st = seen ? T.readsOn : T.readsOff;
        const o = { state: `<b>${esc(st)}</b>`, sensor: esc(s.name), value: esc(+s.value.toFixed(2)) };
        if (want == null || want === seen) el.innerHTML = `<i class="fa-solid fa-circle-check me-1 cl-g"></i>` + fill(esc(T.liveOk), o);
        else {
            el.classList.add("bad");
            el.innerHTML = `<i class="fa-solid fa-circle-exclamation me-1"></i>` + fill(esc(T.liveBad), Object.assign(o, { expected: esc(want ? T.readsOn : T.readsOff) }));
        }
    }
    function stageSec(z) {
        const zs = z.now.stage, ps = zs.plants || [];
        $("dzStageRow").classList.toggle("d-none", !zs.mixed);
        $("dzStageLeft").classList.toggle("d-none", !zs.left || !zs.plant);
        const desc = p => statusLabel(p.status) + (p.status === "Flower" ? ", " + fill(T.week, { n: p.week }) : "");
        if (!zs.plant) { $("dzStageTxt").textContent = T.stageEmpty; return; }
        if (zs.left) $("dzStageLeft").innerHTML = `<i class="fa-solid fa-circle-info me-1"></i>` + esc(fill(T.stageLeft, { plant: zs.plant.name }));
        if (!zs.mixed) {
            const n = ps.length, o = { n, stage: desc(ps[0]) };
            $("dzStageTxt").textContent = fill(n === 1 ? T.stageOne : n === 2 ? T.stageTwo : T.stageAll, o);
            return;
        }
        $("dzStageTxt").textContent = T.stageMixed;
        const chosen = z.stage_plant_id && ps.some(p => p.id === z.stage_plant_id) ? z.stage_plant_id : null;
        $("dzStage").innerHTML = ps.map((p, i) => `<option value="${i === 0 ? "" : p.id}"${(chosen ? p.id === chosen : i === 0) ? " selected" : ""}>` +
            `${esc(p.name)} (${esc(desc(p))})${i === 0 ? " – " + esc(T.furthest) : ""}</option>`).join("");
    }
    function openZone(id) {
        const z = zoneById(id);
        if (!z) return;
        cur = z;
        $("zoneId").value = z.id;
        $("editZoneName").value = z.name;
        const sched = z.light_schedule && Object.keys(z.light_schedule).length ? z.light_schedule : null;
        $("dzRows").querySelectorAll("tr").forEach(tr => {
            const t = (sched && sched[tr.dataset.s]) || DEFAULT_SCHED[tr.dataset.s];
            tr.querySelector(".on").value = t[0];
            tr.querySelector(".off").value = t[1];
        });
        $("dzClock").innerHTML = fill(esc(T.clock), { time: `<b>${esc(D.clock)}</b>`, zone: esc(D.time_zone || "") }).replace(/,\s*\./, ".");
        const opts = z.sensor_options || [];
        const temps = opts.filter(s => kindOf(s) === "temp"), hums = opts.filter(s => kindOf(s) === "hum"), vpds = opts.filter(s => kindOf(s) === "vpd");
        const lights = opts.filter(s => ["light", "port", "other"].includes(kindOf(s)));
        options($("dzLight"), lights, z.light_sensor_id, T.none);
        $("dzAbove").value = z.light_on_above != null ? z.light_on_above : 0;
        stageSec(z);
        const has = opts.length > 0;
        $("dzRd").style.display = has ? "" : "none";
        $("dzRdNone").style.display = has ? "none" : "";
        options($("editZoneVpdTempSensor"), temps, z.vpd_temp_sensor_id, T.none);
        options($("editZoneVpdHumSensor"), hums, z.vpd_humidity_sensor_id, T.none);
        options($("dzVpdSensor"), vpds, z.vpd_sensor_id, null);
        $("dzVpdSrc").querySelector('[data-v="sensor"]').disabled = !vpds.length;
        $("dzVpdHint").textContent = vpds.length ? T.vpdHintSensor : T.vpdHintNone;
        $("editZoneLeafOffset").value = z.leaf_temp_offset != null ? z.leaf_temp_offset : -2;
        setVpdSrc(z.vpd_source === "sensor" && vpds.length ? "sensor" : z.vpd_source === "calc" ? "calc" : vpds.length ? "sensor" : "calc");
        setSrc(sched ? "sched" : "none");
        bootstrap.Modal.getOrCreateInstance(zoneModalEl).show();
    }
    document.querySelectorAll(".zone-row").forEach(row => row.addEventListener("click", () => {
        const z = JSON.parse(row.getAttribute("data-zone"));
        openZone(z.ID != null ? z.ID : z.id);
    }));
    $("dzSrc").addEventListener("click", e => { const b = e.target.closest("button"); if (b) setSrc(b.dataset.v); });
    $("dzVpdSrc").addEventListener("click", e => { const b = e.target.closest("button"); if (b && !b.disabled) setVpdSrc(b.dataset.v); });
    $("dzRows").addEventListener("input", () => { hours(); live(); });
    $("dzLight").addEventListener("change", live);
    $("dzAbove").addEventListener("input", live);

    const intOrNull = v => v === "" || v == null ? null : parseInt(v, 10);
    $("editZoneForm").addEventListener("submit", e => {
        e.preventDefault();
        const id = $("zoneId").value;
        let schedule = null;
        if (src === "sched") {
            schedule = {};
            $("dzRows").querySelectorAll("tr").forEach(tr => {
                const a = tr.querySelector(".on").value, b = tr.querySelector(".off").value;
                if (a && b) schedule[tr.dataset.s] = [a, b];
            });
        }
        const offset = parseFloat($("editZoneLeafOffset").value);
        const body = {
            zone_name: $("editZoneName").value,
            light_schedule: schedule,
            light_sensor_id: src === "sched" ? intOrNull($("dzLight").value) : null,
            light_on_above: parseFloat($("dzAbove").value) || 0,
            stage_plant_id: $("dzStageRow").classList.contains("d-none") ? (cur && cur.stage_plant_id) || null : intOrNull($("dzStage").value),
            vpd_temp_sensor_id: intOrNull($("editZoneVpdTempSensor").value),
            vpd_humidity_sensor_id: intOrNull($("editZoneVpdHumSensor").value),
            vpd_source: $("dzRd").style.display === "none" ? null : vpdSrc,
            vpd_sensor_id: vpdSrc === "sensor" ? intOrNull($("dzVpdSensor").value) : null,
            leaf_temp_offset: vpdSrc === "calc" ? (isNaN(offset) ? -2 : offset) : null,
        };
        // A plant that already left the zone can't be kept as the choice.
        if (body.stage_plant_id && cur && !(cur.now.stage.plants || []).some(p => p.id === body.stage_plant_id)) body.stage_plant_id = null;
        fetch(`/zones/${id}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        }).then(async r => {
            const d = await r.json().catch(() => ({}));
            if (!r.ok) throw new Error(d.error || "");
            location.reload();
        }).catch(err => uiMessages.showToast(err.message || uiMessages.t("failed_to_update_zone"), "danger"));
    });

    render();
    sensorsList();
    zonesTable();
    changed();
})();
