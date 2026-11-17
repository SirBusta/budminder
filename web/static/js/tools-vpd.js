// Tools > VPD chart (built 2026-11-17 from mockup data/mockups/tools/vpd/v3.html).
// A leaf VPD chart: rows of air temperature, columns of humidity, each cell
// the leaf VPD for the leaf temperature offset shown, colored by fixed ranges
// per stage. The ranges are the common published ones (Seedling = clone
// ranges, Veg, Flower for every phase); Grow setup's Targets never change
// them, and the chart sends no alerts. Each cell is colored by the number it
// shows (one decimal), so a number and its color never disagree.
// Page data comes from handlers.VPDChartPageData.
(function () {
    "use strict";
    const page = document.getElementById("vpPage");
    if (!page) return;
    const D = JSON.parse(page.dataset.page || "{}");
    const T = D.t || {};
    const ZONES = D.zones || [];
    const METRIC = D.units === "metric";
    const DEFAULT_OFF_F = typeof D.default_offset_f === "number" ? D.default_offset_f : -2;

    // In tenths of a kPa: [amber from, light green from, dark green from,
    // dark green to, light green to, amber to]. Published ranges: Clone ideal
    // 0.6-0.8, good 0.4-1.0, acceptable 0.2-1.4; Veg 0.9-1.1, 0.7-1.3,
    // 0.45-1.7; Flower 1.2-1.5, 1.0-1.7, 0.7-2.3. Veg's 0.45 edge starts at the
    // cells showing 0.5.
    const RANGES = { seedling: [2, 4, 6, 8, 10, 14], veg: [5, 7, 9, 11, 13, 17], flower: [7, 10, 12, 15, 17, 23] };
    const STAGES = ["seedling", "veg", "flower"];
    const STAGE_NAME = { seedling: T.seedling_label, veg: T.veg_label, flower: T.flower_label, drying: T.drying_label };
    const PHASE_NAME = { stretch: T.cl_stretch, bloom: T.cl_bloom, ripen: T.cl_ripen };

    const $ = id => document.getElementById(id);
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    const fill = (s, vals) => String(s || "").replace(/\{(\w+)\}/g, (m, k) => (k in vals ? vals[k] : m));
    const minus = s => String(s).replace(/^-/, "−");

    // Saturation vapor pressure (kPa), as utils/vpd.go.
    const svp = c => 0.6107 * Math.pow(10, 7.5 * c / (237.3 + c));
    const fToC = f => (f - 32) * 5 / 9;
    const leafVPD = (airC, rh, offC) => svp(airC + offC) - svp(airC) * rh / 100;

    const st = { zone: null, stage: "veg", offsetF: DEFAULT_OFF_F };
    const zoneById = id => ZONES.find(z => z.id === id) || null;
    const zoneOffF = z => (z && typeof z.offset_f === "number" ? z.offset_f : DEFAULT_OFF_F);
    const chartStage = z => (z.stage === "seedling" || z.stage === "veg" ? z.stage : z.stage === "flower" ? "flower" : null);
    const hasReading = z => z && typeof z.temp_c === "number" && typeof z.rh === "number";

    // Offsets: kept in °F (Edit Zone saves °F), shown in the Settings units.
    const round1 = v => Math.round(v * 10) / 10;
    const offText = f => minus(String(METRIC ? round1(f * 5 / 9).toFixed(1) : round1(f))) + (METRIC ? " °C" : " °F");
    const offValue = f => minus(String(METRIC ? round1(f * 5 / 9).toFixed(1) : round1(f)));

    function band(v) {
        const n = Math.round(v * 10), r = RANGES[st.stage];
        if (n >= r[2] && n <= r[3]) return "g1";
        if (n >= r[1] && n <= r[4]) return "g2";
        if (n >= r[0] && n <= r[5]) return "am";
        return "rd";
    }

    const rows = [];
    if (METRIC) for (let c = 16; c <= 32.0001; c += 0.5) rows.push({ c, label: c.toFixed(1), step: 0.5 });
    else for (let f = 60; f <= 90; f++) rows.push({ c: fToC(f), label: String(f), step: 5 / 9 });
    const RH = [];
    for (let h = 90; h >= 30; h -= 2) RH.push(h);

    function stageLine(z) {
        if (!z.stage) return "";
        let s = STAGE_NAME[z.stage] || "";
        if (z.stage === "flower" && z.week) s += ", " + fill(T.cl_week_n, { n: z.week });
        if (z.phase && PHASE_NAME[z.phase]) s += " · " + PHASE_NAME[z.phase];
        return s;
    }

    function drawZoneMenu() {
        $("vpZone").innerHTML = ZONES.map(z => `<option value="${z.id}">${esc(z.name)}</option>`).join("") +
            `<option value="">${esc(T.vp_no_zone)}</option>`;
        $("vpZone").value = st.zone ? String(st.zone) : "";
    }

    function drawControls() {
        const z = zoneById(st.zone);
        const here = z ? chartStage(z) : null;
        $("vpStage").innerHTML = STAGES.map(s =>
            `<button type="button" data-s="${s}" class="${s === st.stage ? "on" : ""}" aria-pressed="${s === st.stage}">${esc(STAGE_NAME[s])}` +
            (s === here ? `<i class="fa-solid fa-tent zt" title="${esc(fill(T.vp_in_stage, { zone: z.name }))}" aria-hidden="true"></i>` : "") + `</button>`).join("");
        $("vpOff").value = offValue(st.offsetF);
        $("vpOffU").textContent = METRIC ? "°C" : "°F";
        const base = zoneOffF(z);
        let src;
        if (Math.abs(st.offsetF - base) > 0.001) {
            const back = z && typeof z.offset_f === "number" ? fill(T.vp_off_back, { v: offText(base) }) : fill(T.vp_off_default, { v: offText(base) });
            src = esc(T.vp_off_only) + ` <a href="#" id="vpOffBack">${esc(back)}</a>`;
        } else if (z && typeof z.offset_f === "number") {
            src = esc(fill(T.vp_off_zone, { zone: z.name }));
        } else {
            src = esc(T.vp_off_only);
        }
        $("vpOffSrc").innerHTML = src;
        const link = $("vpOffBack");
        if (link) link.addEventListener("click", e => { e.preventDefault(); st.offsetF = base; draw(); });
        $("vpWhy2").textContent = fill(T.vp_why2, { d: METRIC ? "1.1 °C" : "2 °F", off: METRIC ? minus("-1.1 °C") : minus("-2 °F") });
    }

    function reading() {
        const z = zoneById(st.zone);
        if (!hasReading(z)) return null;
        return { z, airC: z.temp_c, rh: z.rh, vpd: leafVPD(z.temp_c, z.rh, st.offsetF * 5 / 9) };
    }

    function drawNow() {
        const z = zoneById(st.zone);
        if (!z) { $("vpNow").innerHTML = `<span class="muted">${esc(T.vp_pick_zone)}</span>`; return; }
        const parts = [];
        const sl = stageLine(z);
        if (sl) parts.push(`<span class="muted">${esc(sl)}</span>`);
        const r = reading();
        if (!r) {
            // Edit Zone opens from Settings (the Today page's Choose sensors link does the same).
            const link = `<a href="/settings?zone=${encodeURIComponent(z.id)}#grow">${esc(T.edit_zone)}</a>`;
            parts.push(`<span class="muted">${esc(T.vp_no_reading).replace("{link}", link)}</span>`);
        } else {
            const air = METRIC ? round1(r.airC).toFixed(1) + " °C" : round1(r.airC * 9 / 5 + 32).toFixed(1) + " °F";
            parts.push(`<span><b>${esc(T.vp_now)}</b> ${esc(air)} · ${esc(String(Math.round(r.rh)))}% · <span class="dot b-${band(r.vpd)}" aria-hidden="true"></span> ` +
                `<b>${esc(fill(T.vp_leaf_vpd, { v: r.vpd.toFixed(2) }))}</b>` +
                (marked(r) ? ` <span class="nowbox" title="${esc(T.vp_outlined)}" aria-hidden="true"></span>` : "") + `</span>`);
        }
        $("vpNow").innerHTML = parts.join(" ");
    }

    // The cell nearest the reading, or null when it is off the chart.
    function hitCell(r) {
        let row = -1, col = -1, best = Infinity;
        rows.forEach((x, i) => { const d = Math.abs(x.c - r.airC); if (d < best) { best = d; row = i; } });
        if (best > rows[row].step / 2 + 1e-9) return null;
        best = Infinity;
        RH.forEach((h, j) => { const d = Math.abs(h - r.rh); if (d < best) { best = d; col = j; } });
        if (best > 1) return null;
        return { row, col };
    }
    const marked = r => !!hitCell(r);

    function drawChart() {
        const r = reading(), hit = r ? hitCell(r) : null, offC = st.offsetF * 5 / 9;
        const u = METRIC ? "°C" : "°F";
        let h = `<thead><tr><th class="corner" scope="col">${esc(T.vp_air)} ${u} ↓<br>${esc(T.vp_rh)} →</th>` +
            RH.map((x, j) => `<th scope="col" class="${hit && j === hit.col ? "hit" : ""}">${x}</th>`).join("") + `</tr></thead><tbody>`;
        rows.forEach((row, i) => {
            h += `<tr><th scope="row" class="${hit && i === hit.row ? "hit" : ""}">${row.label}</th>`;
            RH.forEach((x, j) => {
                const v = leafVPD(row.c, x, offC);
                h += `<td class="b-${band(v)}${hit && i === hit.row && j === hit.col ? " now" : ""}">${(Math.round(v * 10) / 10).toFixed(1)}</td>`;
            });
            h += `</tr>`;
        });
        $("vpT").innerHTML = h + `</tbody>`;
        $("vpTitle").textContent = fill(T.vp_chart_title, { stage: STAGE_NAME[st.stage], off: offText(st.offsetF) });
    }

    function drawKey() {
        const r = RANGES[st.stage], v = n => (n / 10).toFixed(1);
        const cells = [
            ["rd", T.vp_too_low, fill(T.vp_below, { v: v(r[0]) })],
            ["am", T.vp_slight_low, `${v(r[0])}–${v(r[1] - 1)}`],
            ["g2", T.vp_good, `${v(r[1])}–${v(r[2] - 1)}`],
            ["g1", T.vp_ideal, `${v(r[2])}–${v(r[3])}`, "mid"],
            ["g2", T.vp_good, `${v(r[3] + 1)}–${v(r[4])}`],
            ["am", T.vp_slight_high, `${v(r[4] + 1)}–${v(r[5])}`],
            ["rd", T.vp_too_high, fill(T.vp_above, { v: v(r[5]) })],
        ];
        $("vpStrip").innerHTML = cells.map(c => `<div class="b-${c[0]} ${c[3] || ""}"><small>${esc(c[1])}</small>${esc(c[2])}</div>`).join("");
    }

    function draw() { drawControls(); drawNow(); drawChart(); drawKey(); }

    // Bring the outlined cell into view inside the chart box.
    function scrollToNow() {
        const n = page.querySelector(".vp-t td.now"), w = $("vpTw");
        if (!n) return;
        w.scrollTop = Math.max(0, n.offsetTop - w.clientHeight / 2);
        w.scrollLeft = Math.max(0, n.offsetLeft - w.clientWidth / 2);
    }

    function pickZone(id) {
        st.zone = id;
        const z = zoneById(id);
        if (z) {
            const s = chartStage(z);
            if (s) st.stage = s;
            else if (z.stage === "drying") st.stage = "flower";
            st.offsetF = zoneOffF(z);
        }
        draw();
        scrollToNow();
    }

    $("vpZone").addEventListener("change", e => pickZone(e.target.value ? Number(e.target.value) : null));
    $("vpStage").addEventListener("click", e => {
        const b = e.target.closest("button[data-s]");
        if (!b) return;
        st.stage = b.dataset.s;
        draw();
    });
    const STEP_F = METRIC ? 0.9 : 0.5; // 0.5 °C or 0.5 °F
    const setOff = f => { st.offsetF = Math.max(-20, Math.min(20, Math.round(f * 100) / 100)); draw(); };
    $("vpOffDn").addEventListener("click", () => setOff(st.offsetF - STEP_F));
    $("vpOffUp").addEventListener("click", () => setOff(st.offsetF + STEP_F));
    $("vpOff").addEventListener("change", e => {
        const v = parseFloat(String(e.target.value).replace("−", "-").replace(",", "."));
        if (isNaN(v)) { draw(); return; }
        setOff(METRIC ? v * 9 / 5 : v);
    });

    // Start on the first zone with plants and a reading, else with plants.
    const first = ZONES.find(z => z.stage && hasReading(z)) || ZONES.find(z => z.stage) || null;
    drawZoneMenu();
    pickZone(first ? first.id : null);
    drawZoneMenu();
})();
