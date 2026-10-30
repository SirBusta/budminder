// Settings → Grow setup: the Climate targets card (Low · Target · High per
// stage for VPD, temperature and humidity, saved straight away to
// POST /settings/climate), the Zones table's Light schedule and Now columns,
// and the Edit zone window (light schedule, light check, plant stage and
// climate sensors, saved to PUT /zones/:id). Data comes from #clData,
// strings from #clText. Temperatures are stored in °C and shown in the
// measurement units chosen in Settings.
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
    const key = (stage, phase, m, lights) => `${stage}|${phase}|${m}|${lights}`;
    const rows = {};
    (S.targets || []).forEach(t => { rows[key(t.stage, t.phase, t.measure, t.lights)] = [t.low, t.target, t.high]; });
    const limits = { temp: (S.temp_limits || [null, null]).slice(), rh: (S.rh_limits || [null, null]).slice() };
    const byStage = { vpd: true, temp: !!S.temp_by_stage, rh: !!S.rh_by_stage };
    let weeks = (S.phase_weeks || [4, 8]).slice();
    let m = "vpd";
    $("clNight").checked = !!S.split_lights;
    $("clPhases").checked = !!S.flower_phases;

    const phaseOf = w => w <= weeks[0] ? "stretch" : w <= weeks[1] ? "bloom" : "ripen";
    // Zones whose readings are compared: a stage and at least one climate sensor.
    const watched = ZONES.filter(z => z.now && z.now.stage && z.now.stage.stage && Object.keys(z.now.sensors || {}).length);
    function rowOf(z) {
        const st = z.now.stage.stage;
        if (st !== "flower" || !$("clPhases").checked) return { stage: st, phase: "" };
        return { stage: st, phase: phaseOf(z.now.stage.week || 1) };
    }
    const lightsPart = (z, stage) => stage === "drying" ? "off" : (!z.now.lights_on && $("clNight").checked ? "off" : "on");

    function stages() {
        const s = [{ stage: "seedling", phase: "", name: T.seedling }, { stage: "veg", phase: "", name: T.veg }];
        if ($("clPhases").checked) {
            s.push({ head: true, name: T.flower });
            s.push({ stage: "flower", phase: "stretch", name: T.stretch, sub: true, wk: fill(T.weeksFrom, { n: 1 }) + `<input class="wk" data-wk="0" value="${weeks[0]}" aria-label="${esc(T.stretch)}">` });
            s.push({ stage: "flower", phase: "bloom", name: T.bloom, sub: true, wk: fill(T.weeksFrom, { n: weeks[0] + 1 }) + `<input class="wk" data-wk="1" value="${weeks[1]}" aria-label="${esc(T.bloom)}">` });
            s.push({ stage: "flower", phase: "ripen", name: T.ripen, sub: true, wk: esc(fill(T.weekOn, { n: weeks[1] + 1 })) });
        } else s.push({ stage: "flower", phase: "", name: T.flower });
        s.push({ stage: "drying", phase: "", name: T.drying, wk: esc(T.lightsOff) });
        return s;
    }

    // ---------- grid ----------
    function cell(stage, phase, lights, label) {
        const v = rows[key(stage, phase, m, lights)] || [null, null, null];
        return `<div class="ct-cell" data-stage="${stage}" data-phase="${phase}" data-lights="${lights}"><span class="ct-colsub">${label}</span>` +
            ["lh", "t", "lh"].map((c, i) => `<input class="${c}" type="number" step="${UNIT[m].step}" value="${fmt(m, v[i])}" data-i="${i}" aria-label="${esc([T.low, T.target, T.high][i])}">`)
                .join('<span class="dot">·</span>') + `<span class="u">${UNIT[m].unit}</span></div>`;
    }
    function labels(names) {
        const bell = `<i class="fa-solid fa-bell" title="${esc(T.alertLimit)}"></i>`;
        return `<div class="ct-cell ct-lbl"><span class="ct-colsub"></span>` + names.map(n =>
            `<span class="l"><span>${esc(n)}</span>${n === T.target ? "" : bell}</span>`).join('<span class="dot">·</span>') +
            `<span class="u">${UNIT[m].unit}</span></div>`;
    }
    const limitsOnly = () => m !== "vpd" && !byStage[m];
    function limitsHtml() {
        const [lo, hi] = limits[m];
        return `<div class="ct-row hd"><div>${esc(T.safety)}</div><div class="ct-span">${esc(T.safetyAll)}</div></div>` +
            `<div class="ct-row lb"><div class="ct-alk"><i class="fa-solid fa-bell"></i>${esc(T.safetyAlert)}</div><div class="ct-span">${labels([T.low, T.high])}</div></div>` +
            `<div class="ct-row"><div class="ct-st"><b>${esc(T[m])}</b><small>${esc(T.leaveEmpty)}</small></div>` +
            `<div class="ct-span"><div class="ct-cell" data-lim="1"><span class="ct-colsub"></span>` +
            `<input class="lim" type="number" step="${UNIT[m].step}" value="${fmt(m, lo)}" data-i="0" aria-label="${esc(T.low)}">` +
            `<span class="dot">·</span><input class="lim" type="number" step="${UNIT[m].step}" value="${fmt(m, hi)}" data-i="1" aria-label="${esc(T.high)}">` +
            `<span class="u">${UNIT[m].unit}</span></div></div></div>`;
    }
    function tags(s) {
        return watched.filter(z => { const r = rowOf(z); return r.stage === s.stage && r.phase === s.phase; })
            .map(z => `<span class="ct-nowtag" title="${esc(fill(T.inStage, { zone: z.name }))}"><i class="fa-solid fa-tent"></i>${esc(z.name)}</span>`).join("");
    }
    function render() {
        const lim = limitsOnly();
        $("clByStageL").style.display = m === "vpd" ? "none" : "";
        $("clByStage").checked = byStage[m];
        $("clNightL").style.display = lim ? "none" : "";
        $("clPhasesL").style.display = lim ? "none" : "";
        $("clLimNote").classList.toggle("d-none", !lim);
        if (lim) { $("clGrid").innerHTML = limitsHtml(); check(); preview(); return; }
        const night = $("clNight").checked;
        let h = (m === "vpd" ? "" : limitsHtml()) + `<div class="ct-row hd"><div>${esc(T.stageTargets)}</div>` +
            (night ? `<div><i class="fa-solid fa-sun"></i>${esc(T.lightsOn)}</div><div><i class="fa-solid fa-moon"></i>${esc(T.lightsOff)}</div>`
                : `<div class="ct-span">${esc(T.lightsOnOff)}</div>`) + `</div>`;
        const lt = labels([T.low, T.target, T.high]);
        h += `<div class="ct-row lb"><div class="ct-alk"><i class="fa-solid fa-bell"></i>${esc(T.alertLimits)}</div>` +
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
        check(); preview();
    }

    // Low ≤ Target ≤ High, every box filled (safety limits may be empty).
    function check() {
        let bad = false;
        $("clGrid").querySelectorAll(".ct-cell:not(.ct-lbl)").forEach(c => {
            const ins = c.querySelectorAll("input"), v = [...ins].map(i => parseFloat(i.value));
            const wrong = c.dataset.lim ? v[0] > v[1] : v.some(isNaN) || v[0] > v[1] || v[1] > v[2];
            ins.forEach(i => i.classList.toggle("bad", wrong));
            bad = bad || wrong;
        });
        $("clErr").classList.toggle("d-none", !bad);
        return !bad;
    }

    // ---------- preview: the first watched zone with a reading ----------
    function preview() {
        const z = watched.find(z => z.readings && z.readings[m] && z.readings[m].value != null && !z.readings[m].stale);
        if (!z) { $("clPv").innerHTML = `<div class="ct-pv-h text-muted">${esc(T.pvNone)}</div>`; return; }
        const now = z.readings[m].value, r = rowOf(z), part = lightsPart(z, r.stage);
        const stageTxt = r.stage === "flower"
            ? (r.phase ? fill(T.flowerPhase, { phase: T[r.phase] }) : T.flower) + " (" + fill(T.week, { n: z.now.stage.week }) + ")"
            : T[r.stage];
        const lightTxt = z.now.scheduled || r.stage === "drying"
            ? ` · <i class="fa-solid ${part === "off" && r.stage !== "drying" || !z.now.lights_on ? "fa-moon" : "fa-sun ct-sun"}"></i> ${esc(z.now.lights_on && r.stage !== "drying" ? T.lightsOn : T.lightsOff)}` : "";
        const head = `<span>${esc(fill(T.zoneNow, { zone: z.name }))} · ${esc(stageTxt)}${lightTxt}</span>`;
        const show = v => fmt(m, v);
        if (limitsOnly()) {
            const [lo, hi] = limits[m];
            const a = lo ?? now - 10, b = hi ?? now + 10, span = (b - a) || 1, min = a - span * 0.4, max = b + span * 0.4;
            const pos = v => Math.max(0, Math.min(100, (v - min) / (max - min) * 100));
            const out = (lo != null && now <= lo) || (hi != null && now >= hi);
            const state = out ? (lo != null && now <= lo ? T.reachedLow : T.reachedHigh) : lo == null && hi == null ? T.notChecked : T.withinLimits;
            $("clPv").innerHTML =
                `<div class="ct-pv-h">${head}<span class="st ${out ? "cl-r" : ""}"><i class="fa-solid ${out ? "fa-circle-exclamation" : "fa-circle"} me-1" style="${out ? "" : "color:var(--sx-muted)"}"></i>${esc(T[m])} ${show(now)} ${UNIT[m].unit}: ${esc(state)}</span></div>` +
                `<div class="ct-band lim"><div class="in" style="left:${lo == null ? 0 : pos(lo)}%;right:${hi == null ? 0 : 100 - pos(hi)}%"></div>` +
                `<span class="mk" style="left:${pos(now)}%">${show(now)}</span></div>` +
                `<div class="ct-ticks">${lo != null ? `<span style="left:${pos(lo)}%">${esc(T.low)} ${show(lo)}</span>` : ""}${hi != null ? `<span style="left:${pos(hi)}%">${esc(T.high)} ${show(hi)}</span>` : ""}</div>` +
                `<div class="ct-legend"><span><i class="fa-solid fa-circle" style="color:#94a3b8"></i>${esc(T.legWithin)}</span><span><i class="fa-solid fa-circle cl-r"></i>${esc(T.legLimit)}</span></div>`;
            return;
        }
        const v = rows[key(r.stage, r.phase, m, part)] || [];
        const [lo, t, hi] = v;
        if (lo == null || t == null || hi == null) { $("clPv").innerHTML = ""; return; }
        const span = (hi - lo) || 1, min = lo - span * 0.5, max = hi + span * 0.5;
        const pos = x => Math.max(0, Math.min(100, (x - min) / (max - min) * 100));
        let state, cls, ic;
        if (now <= lo || now >= hi) { state = now <= lo ? T.reachedLow : T.reachedHigh; cls = "cl-r"; ic = "fa-circle-exclamation"; }
        else if (Math.abs(now - t) <= (now < t ? t - lo : hi - t) * 0.5 + 1e-9) { state = T.near; cls = "cl-g"; ic = "fa-circle-check"; }
        else { state = now < t ? T.towardLow : T.towardHigh; cls = "cl-a"; ic = "fa-circle-half-stroke"; }
        $("clPv").innerHTML =
            `<div class="ct-pv-h">${head}<span class="st ${cls}"><i class="fa-solid ${ic} me-1"></i>${esc(T[m])} ${show(now)} ${UNIT[m].unit}: ${esc(state)}</span></div>` +
            `<div class="ct-band"><div class="in" style="left:${pos(lo)}%;right:${100 - pos(hi)}%"></div>` +
            `<span class="mk" style="left:${pos(now)}%">${show(now)}</span></div>` +
            `<div class="ct-ticks"><span style="left:${pos(lo)}%">${esc(T.low)} ${show(lo)}</span><span style="left:${pos(t)}%">${esc(T.target)} ${show(t)}</span><span style="left:${pos(hi)}%">${esc(T.high)} ${show(hi)}</span></div>` +
            `<div class="ct-legend"><span><i class="fa-solid fa-circle cl-g"></i>${esc(T.near)}</span><span><i class="fa-solid fa-circle cl-a"></i>${esc(T.legToward)}</span><span><i class="fa-solid fa-circle cl-r"></i>${esc(T.legReached)}</span></div>`;
    }

    // ---------- saving ----------
    let timer = null;
    function payload() {
        const targets = [];
        Object.entries(rows).forEach(([k, v]) => {
            const [stage, phase, measure, lights] = k.split("|");
            targets.push({ stage, phase, measure, lights, low: v[0], target: v[1], high: v[2] });
        });
        return {
            split_lights: $("clNight").checked, flower_phases: $("clPhases").checked, phase_weeks: weeks,
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
        }).catch(e => window.uiMessages && uiMessages.showToast(e.message || T.failed, "danger"));
    }
    const queue = () => { clearTimeout(timer); if (check()) timer = setTimeout(save, 600); };

    $("clGrid").addEventListener("input", e => {
        const i = e.target;
        if (i.dataset.wk) return;
        const c = i.closest(".ct-cell"), raw = parseFloat(i.value), v = isNaN(raw) ? null : fromDisp(m, raw);
        if (c.dataset.lim) limits[m][+i.dataset.i] = v;
        else {
            const k = key(c.dataset.stage, c.dataset.phase, m, c.dataset.lights);
            rows[k] = (rows[k] || [null, null, null]).slice();
            rows[k][+i.dataset.i] = v;
        }
        preview(); queue();
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
    $("clNight").addEventListener("change", () => { render(); queue(); });
    $("clPhases").addEventListener("change", () => { render(); queue(); zonesTable(); });
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
        el.classList.remove("bad");
        if (!id || !cur) return;
        const s = (cur.sensor_options || []).find(x => x.id === id);
        if (!s || s.value == null || s.stale) { el.innerHTML = `<i class="fa-solid fa-circle-info me-1"></i>${esc(T.liveNoReading)}`; return; }
        const above = parseFloat($("dzAbove").value) || 0, seen = s.value > above, want = expectedOn();
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
})();
