// New / Edit smoke report (views/smoke-form.html; mockup data/mockups/smoke,
// screens 6-7). "From my plants" fills in what BudMinder knows about the
// plant (GET /smoke/prefill); "Bought flower" looks the strain up in your
// strains and on StrainCompass. Saving: POST /smoke or PUT /smoke/:id, then
// the photo changes, then the report page.
(function () {
    "use strict";
    const root = document.getElementById("smokeForm");
    if (!root) return;
    const D = JSON.parse(root.dataset.page || "{}");
    const box = document.getElementById("srForm");
    const $ = id => document.getElementById(id);
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    function tr(key, vars) {
        let s = (D.t && D.t[key]) || key;
        if (vars) for (const k of Object.keys(vars)) s = s.split("{" + k + "}").join(vars[k]);
        return s;
    }
    const HIDDEN = (D.settings && D.settings.hidden) || {};
    const W = (D.settings && D.settings.weights) || { bud: 1, taste: 1, smell: 1, effects: 1 };
    const shown = f => !HIDDEN[f];
    const fmtDate = d => d ? new Date(d + "T12:00:00").toLocaleDateString(D.lang || undefined, { month: "short", day: "numeric", year: "numeric" }) : "";
    const toast = (m, l) => window.uiMessages && uiMessages.showToast(m, l || "danger");

    // ---------- state ----------
    const R = D.report || null;
    const KINDS = ["effect", "side", "smell", "taste", "bud"];
    const S = {
        source: R ? R.source : (D.strain_id && !D.plant_id ? "bought" : "grown"),
        plant_id: R ? R.plant_id : (D.plant_id || null),
        strain_id: R ? R.strain_id : (D.strain_id || null),
        sc: null,               // the StrainCompass listing picked: {slug, breeder, thc}
        add_to_strains: false,
        f: {},                  // text/number fields by name
        from: {},               // field → provenance label (shown next to its name)
        scores: { bud: null, taste: null, smell: null, effects: null },
        tags: {},               // kind → [{name, custom}]
        method: "", again: "",
        curingOn: "", cureTouched: !!R,
        photos: R ? R.photos.map(p => ({ id: p.id, url: p.url })) : [],  // existing, in order
        removed: [],            // existing photo ids to delete
        journal: [],            // the plant's journal photos
        tab: "upload",
    };
    KINDS.forEach(k => { S.tags[k] = []; });
    const FIELDS = ["strain_name", "breeder_name", "smoked_on", "thc", "cbd", "harvested_on", "grow_space", "lights", "container", "medium",
        "nutrients", "days_to_harvest", "dry_days", "cure_days", "dispensary", "price", "amount_g", "batch", "notes"];
    if (R) {
        FIELDS.forEach(k => { S.f[k] = R[k] == null ? "" : String(R[k]); });
        S.scores = { bud: R.score_bud, taste: R.score_taste, smell: R.score_smell, effects: R.score_effects };
        (R.tags || []).forEach(t => { if (S.tags[t.kind]) S.tags[t.kind].push({ name: t.name, custom: !!t.custom }); });
        S.method = R.method || "";
        S.again = R.again || "";
    } else {
        FIELDS.forEach(k => { S.f[k] = ""; });
        S.f.smoked_on = D.today || "";
    }
    const strainByID = id => (D.strains || []).find(s => s.id === id);
    if (!R && S.strain_id) {
        const s = strainByID(S.strain_id);
        if (s) Object.assign(S.f, { strain_name: s.name, breeder_name: s.breeder, thc: s.thc, cbd: s.cbd });
    }

    // ---------- small builders ----------
    const label = (text, f) => `<label class="f" for="srF_${f}">${esc(text)}${S.from[f] ? `<span class="sr-from ${S.from[f].sug ? "sug" : ""}" data-from="${f}">${esc(S.from[f].text)}</span>` : ""}</label>`;
    function field(f, text, type = "text", extra = "") {
        return `<div>${label(text, f)}<input class="form-control" id="srF_${f}" data-f="${f}" type="${type}" value="${esc(S.f[f])}" ${extra}></div>`;
    }
    function chips(kind, list, cls) {
        const on = S.tags[kind];
        const isOn = k => on.some(t => !t.custom && t.name === k);
        return `<div class="sr-pick" data-kind="${kind}">${list.map(k => `<button type="button" data-tag="${k}" class="${isOn(k) ? "on " + cls : ""}" aria-pressed="${isOn(k)}">${esc(tr("sr_w_" + k))}</button>`).join("")}
            ${on.filter(t => t.custom).map(t => `<button type="button" data-custom="${esc(t.name)}" class="on ${cls}" title="${esc(tr("sr_remove"))}">${esc(t.name)} <i class="fa-solid fa-xmark ms-1"></i></button>`).join("")}
            <input class="otherin" data-other="${kind}" placeholder="${esc(tr("sr_other"))}: ${esc(tr("sr_other_ph"))}" aria-label="${esc(tr("sr_other"))}"></div>`;
    }
    // 1 → 10: red, amber, green (the picked number fills with its color).
    const SCALE = ["#b91c1c", "#dc2626", "#ea580c", "#d97706", "#ca8a04", "#65a30d", "#4d7c0f", "#16a34a", "#15803d", "#166534"];
    function ten(k) {
        const v = S.scores[k];
        let h = "";
        for (let i = 1; i <= 10; i++) h += `<button type="button" data-score="${k}" data-v="${i}" style="--n:${SCALE[i - 1]}" class="${v === i ? "on" : v && i < v ? "in" : ""}" aria-pressed="${v === i}">${i}</button>`;
        return `<div class="sr-ten" role="group" aria-label="${esc(tr("sr_" + k))}">${h}</div><div class="wt">×${W[k]}</div>`;
    }
    function overall() {
        let n = 0, d = 0;
        for (const k of ["bud", "taste", "smell", "effects"]) if (S.scores[k] && W[k] > 0) { n += S.scores[k] * W[k]; d += W[k]; }
        return d ? Math.round(n / d * 10) / 10 : null;
    }
    const scoreBadge = v => v == null ? `<span class="sd-muted">–</span>` : `<span class="sr-score ${v >= 8 ? "" : v >= 6 ? "mid" : "low"}">${v.toFixed(1)}<small>/10</small></span>`;

    // ---------- sections ----------
    function plantOptions() {
        const opts = (D.plants || []).map(p => `<option value="${p.id}" ${S.plant_id === p.id ? "selected" : ""}>${esc(p.name)}${p.zone ? " — " + esc(p.zone) : ""}${p.harvested ? " · " + esc(tr("sr_harvested_on", { date: fmtDate(p.harvested) })) : ""}</option>`).join("");
        return `<label class="f" for="srPlant">${esc(tr("sr_plant"))}</label>
            <select class="form-select" id="srPlant"><option value="">${esc(tr("sr_pick_plant"))}</option>${opts}</select>
            ${S.plant_id && !R ? `<p class="small sd-muted mt-1 mb-0"><i class="fa-solid fa-wand-magic-sparkles text-success me-1"></i>${esc(tr("sr_filled_from_bm"))}</p>` : ""}`;
    }
    function lookupBox() {
        const linked = S.strain_id && strainByID(S.strain_id);
        return `<label class="f" for="srLookup">${esc(tr("sr_strain"))}</label>
            <div class="position-relative"><input class="form-control" id="srLookup" autocomplete="off" placeholder="${esc(tr("sr_strain_name"))}…" value="">
              <div class="sr-acdd d-none" id="srLookupDD"></div></div>
            ${linked ? `<p class="small mt-1 mb-0"><i class="fa-solid fa-link text-success me-1"></i>${esc(tr("sr_linked", { name: linked.name }))} <a href="#" id="srUnlink">${esc(tr("sr_unlink"))}</a></p>` : ""}`;
    }
    function smokeSection() {
        const g = S.source === "grown";
        let h = `<div class="grid">${field("strain_name", tr("sr_strain_name"))}${field("breeder_name", tr("sr_breeder"))}${field("smoked_on", tr("sr_date_smoked"), "date", `max="${esc(D.today)}"`)}`;
        if (shown("thc_cbd")) h += field("thc", tr("sr_thc_pct")) + field("cbd", tr("sr_cbd_pct"));
        if (!g && shown("dispensary")) {
            h += field("dispensary", tr("sr_dispensary")) + field("price", tr("sr_price") + (D.t && D.t.sr_currency ? ` (${D.t.sr_currency})` : ""), "number", `min="0" step="0.01"`)
                + field("amount_g", tr("sr_amount"), "number", `min="0" step="0.1"`)
                + `<div><label class="f">${esc(tr("sr_ppg"))}</label><div class="form-control bg-body-tertiary" id="srPPG">${ppgText() || "–"}</div></div>` + field("batch", tr("sr_batch"));
        }
        h += `</div>`;
        if (!g && !S.strain_id) {
            const name = S.f.strain_name || "…";
            h += `<div class="form-check mt-3"><input class="form-check-input" type="checkbox" id="srAddStrain" ${S.add_to_strains ? "checked" : ""}>
                <label class="form-check-label" for="srAddStrain"><b>${esc(tr("sr_add_to_strains", { name }))}</b>
                <span class="sd-muted d-block small">${esc(tr("sr_add_to_strains_hint"))}${S.sc ? " " + esc(tr("sr_add_to_strains_sc")) : ""}</span></label></div>`;
        }
        if (shown("method")) {
            const custom = S.method && !(D.vocab.method || []).includes(S.method) ? S.method : "";
            h += `<label class="f mt-3">${esc(tr("sr_how_smoked"))}</label><div class="sr-pick" data-method>
                ${(D.vocab.method || []).map(k => `<button type="button" data-m="${k}" class="${S.method === k ? "on" : ""}" aria-pressed="${S.method === k}">${esc(tr("sr_m_" + k))}</button>`).join("")}
                <input class="otherin" id="srMethodOther" value="${esc(custom)}" placeholder="${esc(tr("sr_other"))}" aria-label="${esc(tr("sr_other"))}"></div>`;
        }
        return h;
    }
    function growSection() {
        const rows = [["harvested", "harvested_on", "sr_f_harvested", "date"], ["grow_space", "grow_space", "sr_f_grow_space"], ["lights", "lights", "sr_f_lights"],
            ["container", "container", "sr_f_container"], ["medium", "medium", "sr_f_medium"], ["nutrients", "nutrients", "sr_f_nutrients"],
            ["days_to_harvest", "days_to_harvest", "sr_f_days_to_harvest", "number"], ["dry_cure", "dry_days", "sr_f_dry", "number"], ["dry_cure", "cure_days", "sr_f_cure", "number"]];
        const f = rows.filter(r => shown(r[0])).map(([, k, l, t]) => field(k, tr(l) + (t === "number" && k !== "days_to_harvest" ? "" : ""), t || "text", t === "number" ? `min="0" step="1"` : "")).join("");
        return f ? `<div class="sd-card sr-c sr-c-grow"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-clipboard-list"></i></span>${esc(tr("sr_grow_info"))}<a class="sd-h-r" href="/settings#grow"><i class="fa-solid fa-sliders me-1"></i>${esc(tr("sr_customize"))}</a></h2><div class="grid">${f}</div></div>` : "";
    }
    function scoresSection() {
        const row = (k, hint) => `<div class="sr-scorerow"><div class="lbl"><b>${esc(tr("sr_" + k))}</b>${hint ? `<small>${esc(tr(hint))}</small>` : ""}</div>${ten(k)}</div>`;
        return `<div class="sd-card sr-c sr-c-scores"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-star-half-stroke"></i></span>${esc(tr("sr_scores"))}<span class="sd-h-r">${esc(tr("sr_scores_hint"))}</span></h2>
            ${row("bud", "sr_bud_hint")}${row("taste")}${row("smell")}${row("effects", "sr_effects_hint")}
            <div class="sr-overall">${esc(tr("sr_overall"))} <span id="srOverall">${scoreBadge(overall())}</span></div></div>`;
    }
    function describeSection() {
        const V = D.vocab || {};
        // Each group's label carries a dot in the group's chip color.
        const lbl = (key, color, first) => `<label class="f${first ? "" : " mt-3"}" data-dot style="--dot:${color}">${esc(tr(key))}</label>`;
        let h = lbl("sr_effects", "#166534", true) + chips("effect", V.effect || [], "");
        if (shown("side")) h += lbl("sr_side_effects", "#c2410c") + chips("side", V.side || [], "side");
        h += lbl("sr_smell", "#7c3aed") + chips("smell", V.smell || [], "smell");
        h += lbl("sr_taste", "#be185d") + chips("taste", V.taste || [], "taste");
        if (shown("bud")) h += lbl("sr_bud", "#0369a1") + chips("bud", V.bud || [], "bud");
        return `<div class="sd-card sr-c sr-c-describe"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-tags"></i></span>${esc(tr("sr_describe"))}</h2>${h}</div>`;
    }
    function againSection() {
        if (!shown("again")) return "";
        const b = (k, l) => `<button type="button" data-again="${k}" class="sr-ag-${k} ${S.again === k ? "on" : ""}" aria-pressed="${S.again === k}">${esc(tr(l))}</button>`;
        return `<div class="sd-card sr-c sr-c-again"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-rotate-right"></i></span>${esc(tr(S.source === "bought" ? "sr_again_q_buy" : "sr_again_q_grow"))}</h2>
            <div class="sr-seg">${b("yes", "sr_yes")}${b("maybe", "sr_maybe")}${b("no", "sr_no")}</div></div>`;
    }
    function photosSection() {
        const list = [...S.photos.map(p => ({ ...p, kind: "saved" })), ...pending.map(p => ({ ...p, kind: "new" }))];
        const items = list.map((p, i) => `<div class="sr-pitem${i === 0 ? " main" : ""}">
            <img src="${esc(p.preview || p.url)}" alt="">${i === 0 ? `<span class="sr-pmain">${esc(tr("sr_main"))}</span>` : ""}
            <div class="sr-pacts">${i ? `<button type="button" data-pmain="${i}" title="${esc(tr("sr_make_main"))}" aria-label="${esc(tr("sr_make_main"))}"><i class="fa-solid fa-star"></i></button>` : ""}
              <button type="button" data-premove="${i}" title="${esc(tr("sr_remove"))}" aria-label="${esc(tr("sr_remove"))}"><i class="fa-solid fa-xmark"></i></button></div></div>`).join("");
        const tab = (k, l) => `<button type="button" data-tab="${k}" class="${S.tab === k ? "on" : ""}">${esc(tr(l))}</button>`;
        let body = "";
        if (S.tab === "upload") body = `<label class="sr-drop d-block" for="srFiles"><i class="fa-solid fa-cloud-arrow-up fa-lg me-1"></i>${esc(tr("sr_choose_files"))}</label><input type="file" id="srFiles" accept="image/*" multiple class="d-none">`;
        if (S.tab === "url") body = `<div class="input-group"><input class="form-control" id="srURL" type="url" placeholder="${esc(tr("sr_url_ph"))}"><button type="button" class="btn btn-success" id="srURLAdd">${esc(tr("sr_add"))}</button></div><p class="small sd-muted mt-2 mb-0">${esc(tr("sr_url_note"))}</p>`;
        if (S.tab === "journal") body = !S.plant_id ? `<p class="sd-muted small mb-0">${esc(tr("sr_pick_plant_first"))}</p>` : !S.journal.length ? `<p class="sd-muted small mb-0">${esc(tr("sr_no_journal"))}</p>`
            : `<div class="sr-jgrid">${S.journal.map(j => `<img src="${esc(j.url)}" data-journal="${j.id}" class="${pending.some(p => p.journal === j.id) ? "on" : ""}" alt="" tabindex="0">`).join("")}</div>`;
        return `<div class="sd-card sr-c sr-c-photos"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-image"></i></span>${esc(tr("sr_photos"))}<span class="sd-h-r">${esc(tr("sr_photos_hint"))}</span></h2>
            ${items ? `<div class="sr-plist mb-3">${items}</div>` : ""}
            <div class="sr-tabs">${tab("upload", "sr_upload")}${tab("url", "sr_image_url")}${S.source === "grown" ? tab("journal", "sr_from_journal") : ""}</div>${body}</div>`;
    }
    // New photos, saved after the report: {file, preview} | {url, preview} | {journal: id, preview}
    const pending = [];

    function render() {
        const g = S.source === "grown";
        box.innerHTML = `
          <div class="sd-card sr-c sr-c-source"><div class="sr-seg sr-srcseg mb-3" role="group">
              <button type="button" data-source="grown" class="${g ? "on" : ""}"><i class="fa-solid fa-seedling me-1"></i>${esc(tr("sr_from_my_plants"))}</button>
              <button type="button" data-source="bought" class="${g ? "" : "on"}"><i class="fa-solid fa-store me-1"></i>${esc(tr("sr_bought_flower"))}</button></div>
            ${g ? plantOptions() : lookupBox()}</div>
          <div class="sd-card sr-c sr-c-smoke"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-joint"></i></span>${esc(tr("sr_the_smoke"))}</h2>${smokeSection()}</div>
          ${g ? growSection() : ""}
          ${scoresSection()}
          ${describeSection()}
          ${againSection()}
          <div class="sd-card sr-c sr-c-notes"><h2 class="sd-h"><span class="sr-ic"><i class="fa-solid fa-note-sticky"></i></span>${esc(tr("sr_notes"))}</h2><textarea class="form-control" rows="4" id="srF_notes" data-f="notes">${esc(S.f.notes)}</textarea></div>
          <div id="srPhotos">${photosSection()}</div>
          <div class="sr-save mb-4"><a class="sd-btn light" href="${R ? "/smoke/" + R.id : "/smoke"}">${esc(tr("sr_cancel"))}</a>
            <button type="button" class="sd-btn" id="srSave"><i class="fa-solid fa-check"></i>${esc(tr("sr_save"))}</button></div>`;
    }
    const rerenderPhotos = () => { $("srPhotos").innerHTML = photosSection(); };

    // ---------- price per gram, cure time ----------
    function ppgText() {
        const p = parseFloat(S.f.price), a = parseFloat(S.f.amount_g);
        return p > 0 && a > 0 ? `${(D.t && D.t.sr_currency) || ""}${(p / a).toFixed(2)} / g` : "";
    }
    function updateCure() {
        if (S.cureTouched || !S.curingOn || !S.f.smoked_on) return;
        const d = Math.round((new Date(S.f.smoked_on + "T12:00:00") - new Date(S.curingOn + "T12:00:00")) / 864e5);
        if (d >= 0) {
            S.f.cure_days = String(d);
            S.from.cure_days = { text: tr("sr_tag_cure") };
            const el = $("srF_cure_days");
            if (el) el.value = S.f.cure_days;
        }
    }

    // ---------- prefill from a plant ----------
    async function prefill(id) {
        S.plant_id = id;
        S.journal = [];
        if (!id) { render(); return; }
        try {
            const [p, j] = await Promise.all([
                fetch("/smoke/prefill?plant_id=" + id, { headers: { Accept: "application/json" } }).then(r => r.ok ? r.json() : null),
                fetch("/smoke/journal-photos?plant_id=" + id, { headers: { Accept: "application/json" } }).then(r => r.ok ? r.json() : { photos: [] }),
            ]);
            S.journal = j.photos || [];
            if (p && !R) {
                S.strain_id = p.strain_id;
                const bm = { text: tr("sr_tag_bm") };
                const set = (k, v, from) => { if (v != null && v !== "") { S.f[k] = String(v); S.from[k] = from; } };
                set("strain_name", p.strain_name, bm);
                set("breeder_name", p.breeder_name, bm);
                set("grow_space", p.grow_space, bm);
                set("harvested_on", p.harvested_on, bm);
                set("days_to_harvest", p.days_to_harvest, bm);
                set("dry_days", p.dry_days, bm);
                set("thc", p.thc, { text: tr("sr_tag_strain") });
                set("cbd", p.cbd, { text: tr("sr_tag_strain") });
                const last = { text: tr("sr_tag_last"), sug: true };
                if (p.lights) set("lights", p.lights, last);
                else set("lights", p.light_port, { text: tr("sr_tag_port"), sug: true });
                set("container", p.container, last);
                set("medium", p.medium, last);
                set("nutrients", p.nutrients, last);
                S.curingOn = p.curing_on || "";
                S.cureTouched = false;
                updateCure();
            }
        } catch (e) { /* keep what's there */ }
        render();
    }

    // ---------- strain lookup (bought flower) ----------
    let lookTimer = null, scResults = [], lookQ = "";
    function showLookup(q) {
        const dd = $("srLookupDD");
        if (!dd) return;
        lookQ = q;
        if (!q) { dd.classList.add("d-none"); return; }
        const ql = q.toLowerCase();
        const mine = (D.strains || []).filter(s => s.name.toLowerCase().includes(ql)).slice(0, 6);
        let h = `<div class="h">${esc(tr("sr_in_your_strains"))}</div>` + (mine.length ? mine.map(s => `<div class="it" data-mine="${s.id}"><span><b>${esc(s.name)}</b> — ${esc(s.breeder)}</span><small>${s.thc ? "THC " + esc(s.thc) + "%" : ""}</small></div>`).join("")
            : `<div class="it"><span class="sd-muted">${esc(tr("sr_none"))}</span></div>`);
        if (D.straincompass) {
            h += `<div class="h">${esc(tr("sr_sc"))}</div>` + (scResults === null ? `<div class="it"><span class="sd-muted">${esc(tr("sr_searching"))}</span></div>`
                : scResults.length ? scResults.slice(0, 6).map((r, i) => `<div class="it" data-sc="${i}"><span><b>${esc(r.name)}</b> — ${esc(r.breeder || "")}</span><small>${esc(r.type || "")}${r.thcMax != null ? " · THC " + esc(r.thcMax) + "%" : ""}</small></div>`).join("")
                : `<div class="it"><span class="sd-muted">${esc(tr("sr_none"))}</span></div>`);
        }
        h += `<div class="it" data-typed="1"><span>${esc(tr("sr_use_typed", { q }))}</span><small>${esc(tr("sr_enter_yourself"))}</small></div>`;
        dd.innerHTML = h;
        dd.classList.remove("d-none");
    }
    function lookup(q) {
        clearTimeout(lookTimer);
        scResults = D.straincompass && q.length >= 2 ? null : [];
        showLookup(q);
        if (!D.straincompass || q.length < 2) return;
        lookTimer = setTimeout(async () => {
            try {
                const r = await fetch("/strains/straincompass/search?q=" + encodeURIComponent(q), { headers: { Accept: "application/json" } });
                const d = r.ok ? await r.json() : { results: [] };
                if (q === lookQ) { scResults = d.results || []; showLookup(q); }
            } catch (e) { scResults = []; showLookup(q); }
        }, 300);
    }
    function pickMine(id) {
        const s = strainByID(id);
        if (!s) return;
        S.strain_id = s.id;
        S.sc = null;
        Object.assign(S.f, { strain_name: s.name, breeder_name: s.breeder, thc: s.thc || S.f.thc, cbd: s.cbd || S.f.cbd });
        S.from = {};
        render();
    }
    function pickSC(i) {
        const r = scResults[i];
        if (!r) return;
        S.strain_id = null;
        S.sc = { slug: r.slug, breeder: r.breeder || "", thc: r.thcMax != null ? String(r.thcMax) : "" };
        const sc = { text: tr("sr_tag_sc") };
        S.f.strain_name = r.name; S.from.strain_name = sc;
        S.f.breeder_name = r.breeder || ""; S.from.breeder_name = sc;
        if (S.sc.thc) { S.f.thc = S.sc.thc; S.from.thc = sc; }
        render();
    }

    // ---------- events ----------
    box.addEventListener("click", e => {
        const t = e.target;
        const src = t.closest("[data-source]");
        if (src) { S.source = src.dataset.source; if (S.source === "grown") S.sc = null; render(); return; }
        const sc = t.closest("[data-score]");
        if (sc) {
            const k = sc.dataset.score, v = +sc.dataset.v;
            S.scores[k] = S.scores[k] === v ? null : v;
            const row = sc.closest(".sr-scorerow");
            row.querySelector(".sr-ten").outerHTML = ten(k).replace(/<div class="wt">.*$/, "");
            $("srOverall").innerHTML = scoreBadge(overall());
            return;
        }
        const tag = t.closest("[data-tag]");
        if (tag) {
            const kind = tag.closest("[data-kind]").dataset.kind, name = tag.dataset.tag;
            const list = S.tags[kind], i = list.findIndex(x => !x.custom && x.name === name);
            if (i >= 0) list.splice(i, 1); else list.push({ name, custom: false });
            const on = i < 0, cls = { effect: "", side: "side", smell: "smell", taste: "taste", bud: "bud" }[kind];
            tag.className = on ? ("on " + cls).trim() : "";
            tag.setAttribute("aria-pressed", on);
            return;
        }
        const cu = t.closest("[data-custom]");
        if (cu) {
            const kind = cu.closest("[data-kind]").dataset.kind;
            S.tags[kind] = S.tags[kind].filter(x => !(x.custom && x.name === cu.dataset.custom));
            cu.remove();
            return;
        }
        const m = t.closest("[data-m]");
        if (m) {
            S.method = S.method === m.dataset.m ? "" : m.dataset.m;
            box.querySelectorAll("[data-m]").forEach(b => { b.classList.toggle("on", b.dataset.m === S.method); b.setAttribute("aria-pressed", b.dataset.m === S.method); });
            const o = $("srMethodOther");
            if (o && S.method) o.value = "";
            return;
        }
        const ag = t.closest("[data-again]");
        if (ag) {
            S.again = S.again === ag.dataset.again ? "" : ag.dataset.again;
            box.querySelectorAll("[data-again]").forEach(b => { b.classList.toggle("on", b.dataset.again === S.again); b.setAttribute("aria-pressed", b.dataset.again === S.again); });
            return;
        }
        const tab = t.closest("[data-tab]");
        if (tab) { S.tab = tab.dataset.tab; rerenderPhotos(); return; }
        const pm = t.closest("[data-pmain]");
        if (pm) { movePhoto(+pm.dataset.pmain); rerenderPhotos(); return; }
        const pr = t.closest("[data-premove]");
        if (pr) { removePhoto(+pr.dataset.premove); rerenderPhotos(); return; }
        const jn = t.closest("[data-journal]");
        if (jn) { toggleJournal(+jn.dataset.journal, jn.src); rerenderPhotos(); return; }
        if (t.closest("#srURLAdd")) { addURL(); return; }
        const mine = t.closest("[data-mine]");
        if (mine) { pickMine(+mine.dataset.mine); return; }
        const scp = t.closest("[data-sc]");
        if (scp) { pickSC(+scp.dataset.sc); return; }
        if (t.closest("[data-typed]")) {
            S.strain_id = null; S.sc = null; S.from = {};
            S.f.strain_name = lookQ;
            render();
            return;
        }
        if (t.closest("#srUnlink")) { e.preventDefault(); S.strain_id = null; render(); return; }
        if (t.closest("#srSave")) save();
    });
    box.addEventListener("input", e => {
        const f = e.target.dataset.f;
        if (f) {
            S.f[f] = e.target.value;
            if (S.from[f]) {
                // A changed StrainCompass THC keeps a note of the original.
                if (f === "thc" && S.sc && S.sc.thc && e.target.value !== S.sc.thc) S.from.thc = { text: tr("sr_tag_edited", { v: S.sc.thc }), sug: true };
                else if (!(f === "thc" && S.sc)) delete S.from[f];
                const tagEl = box.querySelector(`[data-from="${f}"]`);
                if (tagEl) {
                    if (S.from[f]) { tagEl.textContent = S.from[f].text; tagEl.classList.toggle("sug", !!S.from[f].sug); } else tagEl.remove();
                }
            }
            if (f === "cure_days") S.cureTouched = true;
            if (f === "smoked_on") updateCure();
            if (f === "price" || f === "amount_g") { const el = $("srPPG"); if (el) el.textContent = ppgText() || "–"; }
            if (f === "strain_name" && S.source === "bought") {
                const lbl = box.querySelector('label[for="srAddStrain"] b');
                if (lbl) lbl.textContent = tr("sr_add_to_strains", { name: S.f.strain_name || "…" });
            }
            return;
        }
        if (e.target.id === "srLookup") lookup(e.target.value.trim());
        if (e.target.id === "srMethodOther") {
            S.method = e.target.value.trim();
            box.querySelectorAll("[data-m]").forEach(b => b.classList.remove("on"));
        }
    });
    box.addEventListener("change", e => {
        if (e.target.id === "srPlant") prefill(e.target.value ? +e.target.value : null);
        if (e.target.id === "srAddStrain") S.add_to_strains = e.target.checked;
        if (e.target.id === "srFiles") addFiles(e.target.files);
    });
    box.addEventListener("keydown", e => {
        const other = e.target.dataset && e.target.dataset.other;
        if (other && e.key === "Enter") {
            e.preventDefault();
            const v = e.target.value.trim();
            if (v && !S.tags[other].some(x => x.name.toLowerCase() === v.toLowerCase())) {
                S.tags[other].push({ name: v, custom: true });
                const cls = { effect: "", side: "side", smell: "smell", taste: "taste", bud: "bud" }[other];
                e.target.insertAdjacentHTML("beforebegin", `<button type="button" data-custom="${esc(v)}" class="on ${cls}" title="${esc(tr("sr_remove"))}">${esc(v)} <i class="fa-solid fa-xmark ms-1"></i></button>`);
            }
            e.target.value = "";
        }
        if (e.target.id === "srURL" && e.key === "Enter") { e.preventDefault(); addURL(); }
        if (e.target.dataset && e.target.dataset.journal && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); e.target.click(); }
    });
    document.addEventListener("click", e => {
        const dd = $("srLookupDD");
        if (dd && !e.target.closest("#srLookupDD") && e.target.id !== "srLookup") dd.classList.add("d-none");
    });

    // ---------- photos ----------
    function allPhotos() { return [...S.photos.map(p => ({ saved: p })), ...pending.map(p => ({ pend: p }))]; }
    function movePhoto(i) {
        // Make photo i the main one: saved and pending lists stay separate, so
        // a pending photo can only lead when no saved photo is kept.
        const all = allPhotos();
        const [x] = all.splice(i, 1);
        all.unshift(x);
        S.photos = all.filter(a => a.saved).map(a => a.saved);
        pending.splice(0, pending.length, ...all.filter(a => a.pend).map(a => a.pend));
        S.pendingFirst = !!x.pend;
    }
    function removePhoto(i) {
        const all = allPhotos();
        const [x] = all.splice(i, 1);
        if (x.saved) { S.removed.push(x.saved.id); S.photos = S.photos.filter(p => p !== x.saved); }
        else pending.splice(pending.indexOf(x.pend), 1);
    }
    function addFiles(files) {
        for (const f of files || []) {
            if (!f.type.startsWith("image/")) continue;
            pending.push({ file: f, preview: URL.createObjectURL(f) });
        }
        rerenderPhotos();
    }
    // A picture from a web address is downloaded straight away (POST
    // /smoke/draft-photo) so it shows here before saving; saving moves it in.
    async function addURL() {
        const el = $("srURL"), btn = $("srURLAdd");
        const u = el && el.value.trim();
        if (!u || !/^https?:\/\//i.test(u)) { if (el) el.reportValidity(); return; }
        btn.disabled = true;
        btn.innerHTML = `<span class="spinner-border spinner-border-sm"></span>`;
        try {
            const d = await api("POST", "/smoke/draft-photo", { url: u });
            pending.push({ draft: d.token, preview: d.url });
            rerenderPhotos();
        } catch (e) {
            toast(e.message);
            btn.disabled = false;
            btn.textContent = tr("sr_add");
        }
    }
    function toggleJournal(id, url) {
        const i = pending.findIndex(p => p.journal === id);
        if (i >= 0) pending.splice(i, 1); else pending.push({ journal: id, preview: url });
    }

    // ---------- save ----------
    const num = v => v === "" || v == null ? null : Number(v);
    const intOrNull = v => { const n = num(v); return n == null || isNaN(n) ? null : Math.round(n); };
    async function api(method, url, body) {
        const r = await fetch(url, { method, headers: { "Content-Type": "application/json", Accept: "application/json" }, body: body === undefined ? undefined : JSON.stringify(body) });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(d.error || d.message || r.statusText);
        return d;
    }
    async function save() {
        if (!S.f.strain_name.trim()) {
            const el = $("srF_strain_name");
            toast(tr("sr_need_strain"));
            if (el) el.focus();
            return;
        }
        const btn = $("srSave");
        btn.disabled = true;
        btn.innerHTML = `<span class="spinner-border spinner-border-sm"></span>${esc(tr("sr_saving"))}`;
        const tags = [];
        KINDS.forEach(k => S.tags[k].forEach(t => tags.push({ kind: k, name: t.name, custom: t.custom })));
        const grown = S.source === "grown";
        const body = {
            source: S.source, plant_id: grown ? S.plant_id : null, strain_id: S.strain_id,
            strain_name: S.f.strain_name, breeder_name: S.f.breeder_name, smoked_on: S.f.smoked_on, thc: S.f.thc, cbd: S.f.cbd,
            harvested_on: S.f.harvested_on, grow_space: S.f.grow_space, lights: S.f.lights, container: S.f.container, medium: S.f.medium, nutrients: S.f.nutrients,
            days_to_harvest: intOrNull(S.f.days_to_harvest), dry_days: intOrNull(S.f.dry_days), cure_days: intOrNull(S.f.cure_days),
            dispensary: S.f.dispensary, price: num(S.f.price), amount_g: num(S.f.amount_g), batch: S.f.batch,
            method: S.method, again: S.again, notes: S.f.notes,
            score_bud: S.scores.bud, score_taste: S.scores.taste, score_smell: S.scores.smell, score_effects: S.scores.effects,
            tags, add_to_strains: !grown && !S.strain_id && S.add_to_strains,
            sc_slug: S.sc ? S.sc.slug : "", sc_listing_breeder: S.sc ? S.sc.breeder : "",
        };
        let id;
        try {
            const res = R ? await api("PUT", "/smoke/" + R.id, body) : await api("POST", "/smoke", body);
            id = R ? R.id : res.id;
        } catch (e) {
            toast(e.message);
            btn.disabled = false;
            btn.innerHTML = `<i class="fa-solid fa-check"></i>${esc(tr("sr_save"))}`;
            return;
        }
        // Photos: removals, then additions in the order shown; then the order.
        const problems = [];
        for (const pid of S.removed) await api("DELETE", `/smoke/${id}/photos/${pid}`).catch(e => problems.push(e.message));
        const newIDs = [];
        for (const p of pending) {
            try {
                let res;
                if (p.file) {
                    const fd = new FormData();
                    fd.append("photos", p.file);
                    const r = await fetch(`/smoke/${id}/photos`, { method: "POST", body: fd, headers: { Accept: "application/json" } });
                    res = await r.json().catch(() => ({}));
                    if (!r.ok) throw new Error(res.error || r.statusText);
                } else if (p.draft) res = await api("POST", `/smoke/${id}/photos/draft`, { token: p.draft });
                else res = await api("POST", `/smoke/${id}/photos/journal`, { image_id: p.journal });
                (res.photos || []).forEach(x => newIDs.push(x.id));
            } catch (e) { problems.push(e.message); }
        }
        const order = S.pendingFirst ? [...newIDs, ...S.photos.map(p => p.id)] : [...S.photos.map(p => p.id), ...newIDs];
        if (order.length > 1) await api("PUT", `/smoke/${id}/photos/order`, { ids: order }).catch(() => {});
        if (problems.length) {
            toast(tr("sr_photo_failed", { msg: problems[0] }), "warning");
            setTimeout(() => { location.href = "/smoke/" + id; }, 2500);
        } else location.href = "/smoke/" + id;
    }

    render();
    if (S.source === "grown" && S.plant_id) prefill(S.plant_id);
})();
