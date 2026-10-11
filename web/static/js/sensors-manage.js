// Manage sensors (views/sensors-manage.html): integration status, scan →
// "Found N new sensors" → add each to a tent, sensors grouped by tent with
// a reporting dot and "Shows on" chips, and the Edit sensor window.
(function () {
    "use strict";
    const root = document.getElementById("smPage");
    if (!root) return;
    const P = JSON.parse(root.dataset.page || "{}");
    const T = P.t || {};
    const K = window.SensorKinds;
    const t = (k, vars) => {
        let s = T[k] || k;
        if (vars) for (const [a, b] of Object.entries(vars)) s = s.split("{" + a + "}").join(b);
        return s;
    };
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const toast = (msg, level) => window.uiMessages ? uiMessages.showToast(msg, level || "success") : alert(msg);
    const locale = P.lang || undefined;
    const MIN = 60000;

    let sensors = P.sensors || [];
    let zones = P.zones || [];
    const plants = P.plants || [];
    let found = [];
    const SOURCE_NAMES = { acinfinity: "AC Infinity", ecowitt: "Ecowitt" };
    const sourceName = s => SOURCE_NAMES[s] || s;

    // ---------- Formatting ----------
    function ago(ms) {
        const m = Math.max(0, Math.round((Date.now() - ms) / MIN));
        if (m < 1) return t("sm_just_now");
        if (m < 60) return t("sx_ago_min", { n: m });
        if (m < 48 * 60) return t("sx_ago_h", { n: Math.round(m / 60) });
        return t("sx_ago_d", { n: Math.round(m / 1440) });
    }
    function typeLabel(s) {
        const k = K.kind(s);
        if (k === "port") return t("sm_type_port", { n: s.type.split(".")[1] || "" });
        return t("sm_type_" + k);
    }
    function valueText(s) {
        if (s.value == null) return `<span class="sm-none">${esc(t("sm_no_reading"))}</span>`;
        if (K.isPort(s)) return s.value > 0 ? `${Math.round(s.value)}<small> %</small>` : esc(t("sm_off"));
        const { unit, conv } = K.display(s, P.units);
        const v = Number(conv(s.value)).toLocaleString(locale, { minimumFractionDigits: K.decimals(s), maximumFractionDigits: K.decimals(s) });
        return `${v}<small> ${esc(unit)}</small>`;
    }
    const zoneName = id => (zones.find(z => z.id === id) || {}).name || t("sm_no_tent");
    const zoneIcon = name => /outdoor|outside/i.test(name || "") ? "fa-tree" : "fa-tent";

    // ---------- Integration cards ----------
    function renderInteg() {
        const aci = sensors.filter(s => s.source === "acinfinity"), eco = sensors.filter(s => s.source === "ecowitt");
        const last = list => Math.max(0, ...list.map(s => s.last_at || 0));
        const card = (cls, icon, title, status, statusCls, meta) => `<div class="sm-ic">
            <div class="logo ${cls}"><i class="fa-solid ${icon}"></i></div>
            <div class="sm-ic-b"><h3>${esc(title)}</h3><div class="st ${statusCls}"><i class="fa-solid fa-circle"></i>${esc(status)}</div><div class="meta">${meta}</div></div>
            <div class="acts"><a href="/settings#integrations">${esc(t("sm_settings_link"))}</a></div></div>`;
        const A = P.aci || {}, E = P.ecowitt || {};
        const aciStatus = !A.enabled ? ["sm_status_off", "off"] : A.token_set ? ["sm_status_connected", "ok"] : ["sm_status_no_token", "warn"];
        const aciLast = last(aci), ecoLast = last(eco);
        const aciMeta = [t("sm_n_sensors_ports", { n: aci.length }), aciLast ? t("sm_updated", { ago: ago(aciLast) }) : ""].filter(Boolean).join(" · ");
        const gw = (E.devices || []).join(", ");
        const ecoMeta = [gw ? t("sm_gateway", { addr: gw }) : "", t("sm_n_sensors", { n: eco.length }), ecoLast ? t("sm_updated", { ago: ago(ecoLast) }) : ""].filter(Boolean).join(" · ");
        document.getElementById("smInteg").innerHTML =
            card("aci", "fa-fan", "AC Infinity", t(aciStatus[0]), aciStatus[1], esc(aciMeta)) +
            card("eco", "fa-satellite-dish", "Ecowitt", t(E.enabled ? "sm_status_on" : "sm_status_off"), E.enabled ? "ok" : "off", esc(ecoMeta));
    }

    // ---------- Found sensors (scan preview) ----------
    const foundKey = f => f.source + "|" + f.device + "|" + f.type;
    function zoneOptions(selected) {
        return zones.map(z => `<option value="${z.id}" ${z.id === selected ? "selected" : ""}>${esc(z.name)}</option>`).join("") +
            `<option value="new">${esc(t("sm_new_tent"))}</option>`;
    }
    function renderFound() {
        const el = document.getElementById("smFound");
        if (!found.length) { el.innerHTML = ""; return; }
        const guess = f => { const same = sensors.find(s => s.source === f.source && s.device === f.device && s.zone_id); return same ? same.zone_id : (zones[0] && zones[0].id); };
        el.innerHTML = `<div class="sm-found"><h3><i class="fa-solid fa-circle-plus me-1"></i>${esc(t(found.length === 1 ? "sm_found_one" : "sm_found_n", { n: found.length }))}</h3>
            ${found.map((f, i) => `<div class="sm-found-row">
                <div class="nm"><b>${esc(f.name)}</b><small>${esc(sourceName(f.source))} · ${esc(f.device)} · ${esc(typeLabel(f))}</small></div>
                <select class="form-select form-select-sm" id="smFz${i}" aria-label="${esc(t("sm_tent"))}">${zoneOptions(guess(f))}</select>
                <input class="form-control form-control-sm d-none" id="smFn${i}" placeholder="${esc(t("sm_new_tent_name"))}" maxlength="255" aria-label="${esc(t("sm_new_tent_name"))}">
                <button type="button" class="sm-btn small" data-add="${i}">${esc(t("sm_add"))}</button>
                <button type="button" class="sm-ebtn" data-ignore="${i}">${esc(t("sm_ignore"))}</button></div>`).join("")}
            ${found.length > 1 ? `<div class="sm-found-all"><button type="button" class="sx-link" data-addall>${esc(t("sm_add_all"))}</button></div>` : ""}</div>`;
        found.forEach((f, i) => document.getElementById("smFn" + i).classList.toggle("d-none", document.getElementById("smFz" + i).value !== "new"));
    }

    // ---------- Sensor list ----------
    function chips(s) {
        if (s.hidden) return `<span class="sm-chip hid"><i class="fa-solid fa-eye-slash me-1"></i>${esc(t("sm_chip_hidden"))}</span>`;
        const n = (s.plant_ids || []).length;
        const names = (s.plant_ids || []).map(id => (plants.find(p => p.id === id) || {}).name).filter(Boolean).join(", ");
        return (s.show_env ? `<span class="sm-chip env">${esc(t("sm_chip_env"))}</span>` : "") +
            (s.show_home ? `<span class="sm-chip home">${esc(t("sm_chip_home"))}</span>` : "") +
            (n ? `<span class="sm-chip plant" title="${esc(names)}">${esc(t(n === 1 ? "sm_one_plant" : "sm_n_plants", { n }))}</span>` : "");
    }
    function row(s) {
        const stale = s.stale || !s.last_at;
        const lastTxt = s.last_at ? t("sm_last_reading", { ago: ago(s.last_at) }) : t("sm_no_reading");
        return `<div class="sm-row ${s.hidden ? "hidden-s" : ""}">
            <span class="live ${stale ? "stale" : ""}" title="${esc(stale ? lastTxt : t("sm_reporting"))}" aria-label="${esc(stale ? lastTxt : t("sm_reporting"))}"></span>
            <div class="nm"><b>${esc(s.name)}</b><small>${esc(sourceName(s.source))} · ${esc(s.device)}${stale && s.last_at ? ` · <span class="warn">${esc(lastTxt)}</span>` : ""}</small></div>
            <div class="ty"><i class="fa-solid ${K.icon(s)}"></i>${esc(typeLabel(s))}</div>
            <div class="val">${valueText(s)}</div>
            <div class="sm-chips">${chips(s)}</div>
            <button type="button" class="sm-ebtn" data-edit="${s.id}"><i class="fa-solid fa-pen me-1"></i>${esc(t("sm_edit"))}</button>
        </div>`;
    }
    // Within a tent: readings, then ports, then hidden; inside before outside.
    const KIND_ORDER = { temp: 0, hum: 1, vpd: 2, co2: 3, soil: 4, ec: 5, other: 6, port: 7 };
    const rowOrder = (a, b) => (a.hidden - b.hidden) || (K.isPort(a) - K.isPort(b)) || (K.isOutside(a) - K.isOutside(b)) ||
        (KIND_ORDER[K.kind(a)] - KIND_ORDER[K.kind(b)]) || a.type.localeCompare(b.type, undefined, { numeric: true }) || a.name.localeCompare(b.name);
    function renderList() {
        const q = document.getElementById("smQ").value.trim().toLowerCase();
        const src = document.getElementById("smSrc").value, showH = document.getElementById("smHidden").checked;
        const filtering = q || src || !showH;
        const list = sensors.filter(s => (!q || (s.name + " " + s.device + " " + typeLabel(s)).toLowerCase().includes(q)) && (!src || s.source === src) && (showH || !s.hidden));
        const groups = [...zones.map(z => ({ id: z.id, name: z.name })), { id: 0, name: t("sm_no_tent") }];
        const html = groups.map(z => {
            const rows = list.filter(s => (s.zone_id || 0) === z.id).sort(rowOrder);
            const total = sensors.filter(s => (s.zone_id || 0) === z.id).length;
            if (!rows.length && (filtering || z.id === 0 || total)) return "";
            const head = `<div class="sm-zone-h"><i class="fa-solid ${z.id ? zoneIcon(z.name) : "fa-circle-question"}"></i><h2>${esc(z.name)}</h2>
                <span class="n">${esc(rows.length ? t(rows.length === 1 ? "sm_n_sensor_one" : "sm_n_sensors", { n: rows.length }) : t("sm_no_sensors_yet"))}</span>
                ${z.id && rows.some(s => s.show_env && !s.hidden) ? `<a href="/sensors#zone=${z.id}">${esc(t("sm_view_charts"))} <i class="fa-solid fa-arrow-right ms-1"></i></a>` : ""}</div>`;
            if (!rows.length) return `<div class="sm-zone">${head}<div class="sm-zone-empty">${esc(t("sm_zone_empty"))}</div></div>`;
            return `<div class="sm-zone">${head}${rows.map(row).join("")}</div>`;
        }).join("");
        document.getElementById("smZones").innerHTML = html || `<div class="sx-empty"><i class="fa-solid fa-temperature-half"></i><b>${esc(t(sensors.length ? "sm_nothing_matches" : "sm_none_title"))}</b><p>${esc(t(sensors.length ? "sm_nothing_matches_help" : "sm_none_help"))}</p></div>`;
    }
    function renderSources() {
        const sel = document.getElementById("smSrc"), cur = sel.value;
        const sources = [...new Set(sensors.map(s => s.source))].sort();
        sel.innerHTML = `<option value="">${esc(t("sm_all_sources"))}</option>` + sources.map(s => `<option value="${esc(s)}" ${s === cur ? "selected" : ""}>${esc(sourceName(s))}</option>`).join("");
    }
    function render() { renderInteg(); renderFound(); renderSources(); renderList(); }

    // ---------- Requests ----------
    async function send(method, url, body) {
        const resp = await fetch(url, { method, headers: { "Content-Type": "application/json" }, body: body ? JSON.stringify(body) : undefined, cache: "no-store" });
        const data = await resp.json().catch(() => ({}));
        if (!resp.ok) throw new Error(data.error || data.message || t("sm_failed"));
        return data;
    }
    async function scan(btn, url, body) {
        const label = btn.innerHTML;
        btn.disabled = true;
        btn.innerHTML = `<span class="spinner-border spinner-border-sm me-1" role="status"></span>${esc(t("sm_scanning"))}`;
        try {
            const data = await send("POST", url, { ...body, preview: true });
            const known = new Set(found.map(foundKey));
            const fresh = (data.sensors || []).filter(f => !known.has(foundKey(f)));
            found.push(...fresh);
            toast(fresh.length ? t(fresh.length === 1 ? "sm_found_one" : "sm_found_n", { n: fresh.length }) : t("sm_found_none"), fresh.length ? "success" : "info");
            renderFound();
        } catch (e) {
            toast(e.message || t("sm_scan_failed"), "danger");
        } finally {
            btn.disabled = false; btn.innerHTML = label;
        }
    }
    async function addFound(i) {
        const f = found[i], sel = document.getElementById("smFz" + i), nameIn = document.getElementById("smFn" + i);
        const body = { source: f.source, device: f.device, type: f.type, name: f.name, unit: f.unit };
        if (sel.value === "new") {
            const nm = nameIn.value.trim();
            if (!nm) { nameIn.focus(); return false; }
            body.new_zone = nm;
        } else body.zone_id = +sel.value;
        const data = await send("POST", "/sensors/add", body);
        if (body.new_zone && data.zone_id && !zones.some(z => z.id === data.zone_id)) {
            zones.push({ id: data.zone_id, name: body.new_zone });
            zones.sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()));
        }
        if (!sensors.some(s => s.id === data.id)) {
            sensors.push({ id: data.id, name: f.name, source: f.source, device: f.device, type: f.type, unit: f.unit, zone_id: data.zone_id || 0,
                zone: zoneName(data.zone_id), show_env: true, show_home: true, hidden: false, value: null, last_at: 0, stale: true, plant_ids: [] });
        }
        found.splice(i, 1);
        return f.name + " → " + zoneName(data.zone_id);
    }

    // ---------- Edit window ----------
    let editing = null;
    function plantChips(zid, linked) {
        const list = plants.filter(p => p.zone_id === zid || linked.includes(p.id));
        if (!list.length) return `<span class="small text-muted">${esc(t("sm_no_plants_in_tent"))}</span>`;
        return list.map(p => `<button type="button" class="sm-pchip ${linked.includes(p.id) ? "on" : ""}" data-p="${p.id}" aria-pressed="${linked.includes(p.id)}"><i class="fa-solid fa-cannabis"></i>${esc(p.name)}</button>`).join("");
    }
    function openEdit(id) {
        const s = sensors.find(x => x.id === id);
        if (!s) return;
        editing = s;
        const now = s.value == null ? esc(t("sm_no_reading")) : valueText(s).replace(/<\/?small>/g, "");
        document.getElementById("smEditBody").innerHTML = `
            <div class="sm-idbox mb-3"><span>${esc(t("sm_source"))}</span><b>${esc(sourceName(s.source))}</b><span>${esc(t("sm_device"))}</span><b>${esc(s.device)}</b>
                <span>${esc(t("sm_type"))}</span><b>${esc(typeLabel(s))} <small class="text-muted">(${esc(s.type)})</small></b><span>${esc(t("sm_now"))}</span><b>${now}${s.last_at ? ` <small class="text-muted">· ${esc(ago(s.last_at))}</small>` : ""}</b></div>
            <label class="form-label small mb-0" for="smEName">${esc(t("sm_name"))}</label>
            <input class="form-control mb-2" id="smEName" maxlength="255" value="${esc(s.name)}">
            <div class="row g-2 mb-2">
                <div class="col-7"><label class="form-label small mb-0" for="smEZone">${esc(t("sm_tent"))}</label>
                    <select class="form-select" id="smEZone"><option value="0">${esc(t("sm_no_tent"))}</option>${zones.map(z => `<option value="${z.id}" ${z.id === s.zone_id ? "selected" : ""}>${esc(z.name)}</option>`).join("")}</select></div>
                <div class="col-5"><label class="form-label small mb-0" for="smEUnit">${esc(t("sm_unit"))}</label>
                    <input class="form-control" id="smEUnit" maxlength="50" value="${esc(s.unit)}" title="${esc(t("sm_unit_help"))}"></div>
            </div>
            <div class="form-label small fw-semibold mb-1 mt-1">${esc(t("sm_shows_on"))}</div>
            <label class="form-check"><input type="checkbox" class="form-check-input" id="smEEnv" ${s.show_env ? "checked" : ""}> <span class="form-check-label">${esc(t("sm_show_env"))}</span></label>
            <label class="form-check"><input type="checkbox" class="form-check-input" id="smEHome" ${s.show_home ? "checked" : ""}> <span class="form-check-label">${esc(t("sm_show_home"))}</span></label>
            <label class="form-check mb-2"><input type="checkbox" class="form-check-input" id="smEHid" ${s.hidden ? "checked" : ""}> <span class="form-check-label">${esc(t("sm_hide_all"))}</span></label>
            <div class="form-label small fw-semibold mb-1">${esc(t("sm_linked_plants"))}</div>
            <div id="smEPlants">${plantChips(s.zone_id, s.plant_ids || [])}</div>
            <div class="small text-muted mt-1">${esc(t("sm_linked_help"))}</div>
            <div class="text-danger small mt-2 d-none" id="smEErr"></div>`;
        renderDelete(false);
        const hidBox = document.getElementById("smEHid");
        const syncHidden = () => ["smEEnv", "smEHome"].forEach(id => { document.getElementById(id).disabled = hidBox.checked; });
        hidBox.addEventListener("change", syncHidden);
        syncHidden();
        document.getElementById("smEZone").addEventListener("change", e => {
            const linked = [...document.querySelectorAll("#smEPlants .sm-pchip.on")].map(c => +c.dataset.p);
            document.getElementById("smEPlants").innerHTML = plantChips(+e.target.value, linked);
        });
        bootstrap.Modal.getOrCreateInstance(document.getElementById("smEdit")).show();
    }
    function renderDelete(asking) {
        document.getElementById("smDelWrap").innerHTML = asking
            ? `<span class="small me-2">${esc(t("sm_delete_confirm"))}</span><button type="button" class="btn btn-danger btn-sm" id="smDelYes">${esc(t("sm_delete_yes"))}</button> <button type="button" class="btn btn-light btn-sm" id="smDelNo">${esc(t("sm_cancel"))}</button>`
            : `<button type="button" class="btn btn-outline-danger btn-sm" id="smDel"><i class="fa-solid fa-trash me-1"></i>${esc(t("sm_delete"))}</button>`;
    }
    async function saveEdit() {
        const s = editing, err = document.getElementById("smEErr");
        const body = {
            name: document.getElementById("smEName").value.trim(),
            zone_id: +document.getElementById("smEZone").value,
            unit: document.getElementById("smEUnit").value.trim(),
            show_env: document.getElementById("smEEnv").checked,
            show_home: document.getElementById("smEHome").checked,
            hidden: document.getElementById("smEHid").checked,
            plant_ids: [...document.querySelectorAll("#smEPlants .sm-pchip.on")].map(c => +c.dataset.p),
        };
        if (!body.name) { err.textContent = t("sm_name_required"); err.classList.remove("d-none"); return; }
        const btn = document.getElementById("smSave");
        btn.disabled = true;
        try {
            await send("PUT", "/sensors/" + s.id, body);
            Object.assign(s, body, { zone: zoneName(body.zone_id) });
            bootstrap.Modal.getInstance(document.getElementById("smEdit")).hide();
            toast(t("sm_saved", { name: s.name }));
            render();
        } catch (e) {
            err.textContent = e.message; err.classList.remove("d-none");
        } finally { btn.disabled = false; }
    }
    async function deleteSensor() {
        const s = editing;
        try {
            await send("DELETE", "/sensors/delete/" + s.id);
            sensors = sensors.filter(x => x !== s);
            bootstrap.Modal.getInstance(document.getElementById("smEdit")).hide();
            toast(t("sm_deleted", { name: s.name }));
            render();
        } catch (e) {
            toast(e.message, "danger");
        }
    }

    // ---------- Raw AC Infinity data (troubleshooting) ----------
    async function fetchDump() {
        const status = document.getElementById("smDumpStatus"), out = document.getElementById("smDumpContent");
        status.textContent = t("sm_fetching"); out.textContent = "";
        try {
            const resp = await fetch("/sensors/dumpACI?redact=" + (document.getElementById("smDumpRedact").checked ? "true" : "false"), { cache: "no-store" });
            if (!resp.ok) throw new Error();
            out.textContent = await resp.text();
            status.textContent = "";
        } catch (e) {
            status.innerHTML = `<span class="text-danger">${esc(t("sm_dump_failed"))}</span>`;
        }
    }

    // ---------- Events ----------
    document.addEventListener("click", async e => {
        const el = e.target.closest("button, a");
        if (!el) return;
        if (el.id === "smScanACI") return scan(el, "/sensors/scanACI", {});
        if (el.id === "smScanEco") {
            const addr = document.getElementById("smEcoAddr");
            if (!addr.value) addr.value = ((P.ecowitt || {}).devices || [])[0] || "";
            bootstrap.Modal.getOrCreateInstance(document.getElementById("smEco")).show();
            return;
        }
        if (el.id === "smEcoGo") {
            const addr = document.getElementById("smEcoAddr").value.trim();
            if (!addr) return;
            bootstrap.Modal.getInstance(document.getElementById("smEco")).hide();
            return scan(document.getElementById("smScanEco"), "/sensors/scanEC", { server_address: addr });
        }
        if (el.dataset.add !== undefined) {
            el.disabled = true;
            try { const msg = await addFound(+el.dataset.add); if (msg) { toast(t("sm_added", { what: msg })); render(); } else el.disabled = false; }
            catch (err) { el.disabled = false; toast(err.message, "danger"); }
            return;
        }
        if (el.hasAttribute("data-addall")) {
            el.disabled = true;
            let added = 0;
            for (let i = found.length - 1; i >= 0; i--) {
                try { if (await addFound(i)) added++; } catch (err) { toast(err.message, "danger"); break; }
            }
            if (added) toast(t("sm_added_n", { n: added }));
            render();
            return;
        }
        if (el.dataset.ignore !== undefined) { found.splice(+el.dataset.ignore, 1); renderFound(); return; }
        if (el.dataset.edit !== undefined) return openEdit(+el.dataset.edit);
        if (el.classList.contains("sm-pchip")) { el.classList.toggle("on"); el.setAttribute("aria-pressed", el.classList.contains("on")); return; }
        if (el.id === "smSave") return saveEdit();
        if (el.id === "smDel") return renderDelete(true);
        if (el.id === "smDelNo") return renderDelete(false);
        if (el.id === "smDelYes") return deleteSensor();
        if (el.id === "smDumpOpen") { bootstrap.Modal.getOrCreateInstance(document.getElementById("smDump")).show(); return fetchDump(); }
        if (el.id === "smDumpRefresh") return fetchDump();
        if (el.id === "smDumpCopy") {
            const text = document.getElementById("smDumpContent").textContent;
            if (!text) return;
            try { await navigator.clipboard.writeText(text); toast(t("sm_copied")); } catch (err) { toast(t("sm_copy_failed"), "danger"); }
        }
    });
    document.addEventListener("change", e => {
        if (e.target.id === "smDumpRedact") return fetchDump();
        if (["smSrc", "smHidden"].includes(e.target.id)) return renderList();
        const m = /^smFz(\d+)$/.exec(e.target.id);
        if (m) document.getElementById("smFn" + m[1]).classList.toggle("d-none", e.target.value !== "new");
    });
    document.getElementById("smQ").addEventListener("input", renderList);
    document.getElementById("smEcoAddr").addEventListener("keydown", e => { if (e.key === "Enter") document.getElementById("smEcoGo").click(); });

    render();
})();
