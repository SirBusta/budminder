// Export readings window on the Environment page (views/sensors.html, mockup
// data/mockups/sensors/export/v6.html). Signed in only, never in the demo.
// Data: #envPage[data-page] (sensors, zones, units, t, and export: the
// Settings time zone's name, now and each sensor's first reading as wall
// clock in that zone, Data Retention days). The preview and the file come
// from GET /sensors/export/preview and GET /sensors/export (handlers/sensor_export.go).
(function () {
    "use strict";
    const root = document.getElementById("envPage"), modalEl = document.getElementById("exModal");
    if (!root || !modalEl) return;
    const P = JSON.parse(root.dataset.page || "{}"), E = P.export;
    if (!E) return;
    const T = P.t || {};
    const t = (k, vars) => {
        let s = T[k] || k;
        if (vars) for (const [a, b] of Object.entries(vars)) s = s.split("{" + a + "}").join(b);
        return s;
    };
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const $ = id => document.getElementById(id);
    const locale = P.lang || undefined;
    const prefs = {
        get() { try { return JSON.parse(localStorage.getItem("isley.export.prefs") || "{}") || {}; } catch (e) { return {}; } },
        set(v) { try { localStorage.setItem("isley.export.prefs", JSON.stringify(v)); } catch (e) { /* private window */ } },
    };
    const K = window.SensorKinds;
    const XLSX_MAX = 1048576, DAY = 864e5;

    // ---------- Wall clock in the Settings time zone ----------
    // From, To and the first readings are "YYYY-MM-DDTHH:MM" in the Settings
    // time zone; sums are done on them as if they were UTC.
    const W = s => Date.parse(s + ":00Z");
    const S = ms => new Date(ms).toISOString().slice(0, 16);
    const loadedAt = Date.now();
    const nowWall = () => Math.floor((W(E.now) + Date.now() - loadedAt) / 6e4) * 6e4;
    const shortDT = ms => new Date(ms).toLocaleString(locale, { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit", timeZone: "UTC" });

    // ---------- Sensors, grouped by zone in the order the window lists them ----------
    const ORDER = { temp: 0, hum: 1, vpd: 2, co2: 3, level: 4, soil: 5, ec: 6, light: 7, other: 8 };
    const isHidden = s => s.hidden || !s.show_env;
    const all = (P.sensors || []).map(s => ({ ...s, shown: K.display(s, P.units).unit }));
    const groups = [...(P.zones || []), { id: 0, name: t("en_ex_no_zone") }].map(z => {
        const list = all.filter(s => (s.zone_id || 0) === z.id);
        const air = list.filter(s => !K.isPort(s)).sort((a, b) => (K.isOutside(a) - K.isOutside(b)) || (ORDER[K.kind(a)] - ORDER[K.kind(b)]) || a.name.localeCompare(b.name));
        const eq = list.filter(s => K.isPort(s)).sort((a, b) => a.type.localeCompare(b.type, undefined, { numeric: true }));
        return { ...z, air, eq, icon: /outdoor|outside/i.test(z.name) ? "fa-tree" : "fa-tent" };
    }).filter(g => g.air.length || g.eq.length);
    const ordered = groups.flatMap(g => [...g.air, ...g.eq]);
    const firstOf = id => E.first && E.first[id] ? W(E.first[id]) : null;
    const earliest = ids => {
        const v = ids.map(firstOf).filter(x => x != null);
        return v.length ? Math.min(...v) : null;
    };

    // ---------- State ----------
    const INTERVALS = [[0, "en_ex_i0"], [15, "en_ex_i15"], [30, "en_ex_i30"], [60, "en_ex_i60"], [1440, "en_ex_i1440"]];
    const p0 = prefs.get();
    const st = {
        from: 0, to: 0, quick: "7", picked: new Set(), showHidden: false,
        every: INTERVALS.some(i => i[0] === p0.every) ? p0.every : 0,
        layout: p0.layout === "cols" ? "cols" : "rows",
        format: p0.format === "xlsx" ? "xlsx" : "csv",
    };
    const picked = () => ordered.filter(s => st.picked.has(s.id));
    const needsRaw = () => st.every < 60;
    const badSpan = () => !(st.from < st.to);
    function quickSpan(q) {
        st.quick = q; st.to = nowWall();
        if (q === "all") {
            const ids = picked().length ? picked().map(s => s.id) : ordered.map(s => s.id);
            st.from = Math.min(earliest(ids) ?? st.to - 7 * DAY, st.to - 6e4);
        } else st.from = st.to - (+q) * DAY;
    }
    function reset() {
        const m = /zone=(\d+)/.exec(location.hash);
        let zone = m ? +m[1] : null;
        if (zone == null) { try { zone = +(localStorage.getItem("isley.env.zone") || 0); } catch (e) { zone = 0; } }
        if (!groups.some(g => g.id === zone && g.id)) zone = (groups.find(g => g.id) || groups[0] || {}).id;
        st.picked = new Set(ordered.filter(s => (s.zone_id || 0) === zone && !isHidden(s)).map(s => s.id));
        st.showHidden = false;
        quickSpan("7");
    }

    // ---------- Drawing ----------
    function setSeg(id, v) {
        modalEl.querySelectorAll(`#${id} label`).forEach(l => { const on = l.dataset.v === v; l.classList.toggle("on", on); l.querySelector("input").checked = on; });
    }
    function drawZones() {
        $("exZones").innerHTML = groups.map(g => {
            const vis = s => st.showHidden || !isHidden(s);
            const air = g.air.filter(vis), eq = g.eq.filter(vis), list = [...air, ...eq];
            if (!list.length) return "";
            const n = list.filter(s => st.picked.has(s.id)).length;
            const item = s => `<label title="${esc(s.name)}"><input type="checkbox" class="form-check-input" data-id="${s.id}" ${st.picked.has(s.id) ? "checked" : ""}><span>${esc(s.name)}</span>${s.shown ? `<small>${esc(s.shown)}</small>` : ""}${isHidden(s) ? `<span class="ex-tag">${esc(t("en_ex_hidden"))}</span>` : ""}</label>`;
            return `<div class="ex-zone">
                <div class="ex-zone-h"><i class="fa-solid ${g.icon}"></i>${esc(g.name)}<span class="ex-n">${esc(t("en_ex_count", { n, m: list.length }))}</span>
                    <span class="ex-all"><button type="button" class="sx-link" data-all="${g.id}">${esc(t("en_ex_all"))}</button><button type="button" class="sx-link" data-none="${g.id}">${esc(t("en_ex_none"))}</button></span></div>
                <div class="ex-list">${air.map(item).join("")}${eq.length ? `<div class="ex-sub">${esc(t("en_ex_equipment"))}</div>${eq.map(item).join("")}` : ""}</div>
            </div>`;
        }).join("");
    }
    function draw() {
        const now = nowWall(), first = earliest(ordered.map(s => s.id));
        $("exFrom").value = S(st.from); $("exTo").value = S(st.to);
        $("exFrom").max = $("exTo").max = S(now);
        if (first != null) $("exFrom").min = $("exTo").min = S(first);
        modalEl.querySelectorAll("#exQuick button").forEach(b => { const on = b.dataset.q === st.quick; b.classList.toggle("on", on); b.setAttribute("aria-pressed", on); });
        const bad = badSpan();
        $("exSpan").textContent = bad ? t("en_ex_bad_span") : t("en_ex_tz", { tz: E.tz });
        $("exSpan").classList.toggle("text-danger", bad);
        // Data Retention: every reading, 15 and 30 minutes reach back only as far as the kept readings.
        const cut = E.retention > 0 ? now - E.retention * DAY : null, ret = cut != null && needsRaw() && st.from < cut;
        $("exRet").hidden = !ret;
        if (ret) $("exRet").innerHTML = `<i class="fa-solid fa-circle-info me-1"></i>${esc(t("en_ex_ret", { days: E.retention, date: shortDT(cut) }))}`;

        const nh = ordered.filter(isHidden).length;
        $("exShowWrap").hidden = !nh; $("exShowHidden").checked = st.showHidden; $("exShowTxt").textContent = t("en_ex_show_hidden", { n: nh });
        drawZones();

        $("exInt").innerHTML = INTERVALS.map(([m, k]) => `<button type="button" data-m="${m}" class="${m === st.every ? "on" : ""}" aria-pressed="${m === st.every}">${esc(t(k))}</button>`).join("");
        $("exIntHelp").textContent = st.every === 0 ? t("en_ex_i_help0") : st.every === 60 ? t("en_ex_i_help60") : st.every === 1440 ? t("en_ex_i_help1440") : t("en_ex_i_help_min", { n: st.every });
        setSeg("exLayout", st.layout); setSeg("exFormat", st.format);
        $("exColsNote").textContent = t(st.every ? "en_ex_cols_note" : "en_ex_cols_note0");
        preview();
    }

    // ---------- Preview ----------
    let pv = null, reqNo = 0, timer = null, loading = false;
    const query = () => new URLSearchParams({ ids: picked().map(s => s.id).join(","), from: S(st.from), to: S(st.to), every: st.every, layout: st.layout, format: st.format }).toString();
    function message(key) {
        $("exPrev").innerHTML = `<tbody><tr><td class="ex-msg">${esc(t(key))}</td></tr></tbody>`;
    }
    function sizeText(bytes) {
        const b = st.format === "xlsx" ? bytes * 0.45 : bytes;
        return b < 1e6 ? `${Math.max(1, Math.round(b / 1e3)).toLocaleString(locale)} KB` : `${(b / 1e6).toLocaleString(locale, { maximumFractionDigits: b < 1e7 ? 1 : 0 })} MB`;
    }
    function drawPreview() {
        const n = picked().length, bad = badSpan();
        let tooMany = false, empty = false;
        $("exSum").innerHTML = "";
        if (!n) message("en_ex_pick");
        else if (bad) $("exPrev").innerHTML = "";
        else if (pv && pv.failed) message("en_ex_preview_failed");
        else if (pv) {
            empty = pv.count === 0;
            tooMany = st.format === "xlsx" && pv.count + 1 > XLSX_MAX;
            if (empty) message("en_ex_no_readings");
            else {
                $("exSum").innerHTML = `<span><b>${esc(t("en_ex_about", { n: pv.count.toLocaleString(locale) }))}</b> (${esc(sizeText(pv.bytes))}) · ${esc(n === 1 ? t("en_ex_one_sensor") : t("en_ex_n_sensors", { n }))}</span>`;
                const numCol = i => st.layout === "cols" ? i > 0 : (st.every ? i >= 3 && i <= 5 : i === 3);
                $("exPrev").innerHTML = `<thead><tr>${pv.head.map(h => `<th>${esc(h)}</th>`).join("")}</tr></thead><tbody>${pv.rows.map(r => `<tr>${r.map((c, i) => `<td class="${numCol(i) ? "num" : ""}">${esc(c)}</td>`).join("")}</tr>`).join("")}${pv.count > pv.rows.length ? `<tr class="more"><td colspan="${pv.head.length}">…</td></tr>` : ""}</tbody>`;
            }
        }
        $("exBad").hidden = !tooMany;
        if (tooMany) $("exBad").innerHTML = `<i class="fa-solid fa-triangle-exclamation me-1"></i>${esc(t("en_ex_xlsx_max", { n: XLSX_MAX.toLocaleString(locale) }))}`;
        $("exGo").disabled = !n || bad || loading || !pv || pv.failed || empty || tooMany;
        $("exPrev").style.opacity = loading ? ".55" : "";
    }
    function preview() {
        clearTimeout(timer);
        const my = ++reqNo;
        if (!picked().length || badSpan()) { loading = false; pv = null; drawPreview(); return; }
        loading = true; drawPreview();
        timer = setTimeout(async () => {
            try {
                const resp = await fetch("/sensors/export/preview?" + query(), { cache: "no-store" });
                if (!resp.ok) throw new Error(resp.status);
                const data = await resp.json();
                if (my !== reqNo) return;
                pv = data;
            } catch (e) {
                if (my !== reqNo) return;
                pv = { failed: true };
            }
            loading = false;
            drawPreview();
        }, 250);
    }

    // ---------- Events ----------
    const savePrefs = () => prefs.set({ every: st.every, layout: st.layout, format: st.format });
    $("exQuick").addEventListener("click", e => { const b = e.target.closest("button"); if (!b) return; quickSpan(b.dataset.q); draw(); });
    ["exFrom", "exTo"].forEach(id => $(id).addEventListener("change", e => {
        const v = W(e.target.value);
        if (!isNaN(v)) st[id === "exFrom" ? "from" : "to"] = v;
        st.quick = ""; draw();
    }));
    $("exInt").addEventListener("click", e => { const b = e.target.closest("button"); if (!b) return; st.every = +b.dataset.m; savePrefs(); draw(); });
    $("exShowHidden").addEventListener("change", e => {
        st.showHidden = e.target.checked;
        if (!st.showHidden) ordered.filter(isHidden).forEach(s => st.picked.delete(s.id));
        draw();
    });
    // A change of sensors keeps All dates matching their first reading.
    const sensorsChanged = () => { if (st.quick === "all") quickSpan("all"); draw(); };
    $("exZones").addEventListener("click", e => {
        const a = e.target.closest("[data-all]"), none = e.target.closest("[data-none]");
        if (a) ordered.filter(s => (s.zone_id || 0) === +a.dataset.all && (st.showHidden || !isHidden(s))).forEach(s => st.picked.add(s.id));
        if (none) ordered.filter(s => (s.zone_id || 0) === +none.dataset.none).forEach(s => st.picked.delete(s.id));
        if (a || none) sensorsChanged();
    });
    $("exZones").addEventListener("change", e => {
        const id = +e.target.dataset.id;
        if (!id) return;
        e.target.checked ? st.picked.add(id) : st.picked.delete(id);
        sensorsChanged();
    });
    [["exLayout", "layout"], ["exFormat", "format"]].forEach(([id, k]) => $(id).addEventListener("click", e => {
        const l = e.target.closest("label");
        if (!l) return;
        e.preventDefault();
        if (st[k] === l.dataset.v) return;
        st[k] = l.dataset.v; savePrefs(); draw();
    }));
    $("exGo").addEventListener("click", () => {
        const a = document.createElement("a");
        a.href = "/sensors/export?" + query();
        a.download = "";
        document.body.appendChild(a); a.click(); a.remove();
        const btn = $("exGo"), html = btn.innerHTML;
        btn.innerHTML = `<i class="fa-solid fa-check me-1"></i>${esc(t("en_ex_downloading"))}`;
        setTimeout(() => { btn.innerHTML = html; }, 1500);
    });
    $("exOpen").addEventListener("click", () => {
        reset(); draw();
        bootstrap.Modal.getOrCreateInstance(modalEl).show();
    });
})();
