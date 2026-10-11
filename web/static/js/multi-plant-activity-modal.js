// "Log activity" window from the top menu: one or several plants, ticks for
// every activity type (number boxes for types that record one), one note.
// Saved with POST /plants/day, which adds to whatever each plant already has
// that day. Same format as the Plant page's window.
document.addEventListener("DOMContentLoaded", () => {
    const modalEl = document.getElementById("addMultiPlantActivityModal");
    if (!modalEl) return;
    const $ = id => document.getElementById(id);
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    const plantsBox = $("mpPlants"), typesBox = $("mpTypes"), numbers = $("mpNumbers"), err = $("mpError");
    let metrics = null;

    // Built-ins first (Water, Feed, Note, Height), then the rest by name.
    const rank = b => b.dataset.watering === "true" && b.dataset.feeding !== "true" ? 0 : b.dataset.feeding === "true" ? 1 : b.dataset.name === "Note" ? 2 : b.dataset.name === "Height" ? 3 : 4;
    [...typesBox.children].sort((a, b) => rank(a) - rank(b) || a.dataset.name.localeCompare(b.dataset.name)).forEach(b => typesBox.appendChild(b));

    // "Everything in the <tent>" for tents with two or more plants.
    const zones = {};
    plantsBox.querySelectorAll("[data-pid]").forEach(b => { if (b.dataset.zone) zones[b.dataset.zone] = (zones[b.dataset.zone] || 0) + 1; });
    const label = $("mpTents").dataset.label || "Everything in the {zone}";
    $("mpTents").innerHTML = Object.keys(zones).filter(z => zones[z] > 1).map(z =>
        `<button type="button" class="btn btn-link btn-sm p-0" data-mptent="${esc(z)}">${esc(label.split("{zone}").join(z))}</button>`).join("");

    async function loadMetrics() {
        if (metrics) return metrics;
        try { const r = await fetch("/metrics"); metrics = r.ok ? await r.json() : []; } catch (e) { metrics = []; }
        return metrics;
    }

    async function renderNumbers() {
        const keep = {};
        numbers.querySelectorAll("input[data-metric]").forEach(i => { keep[i.dataset.act + ":" + i.dataset.metric] = i.value; });
        const all = await loadMetrics();
        numbers.innerHTML = [...typesBox.querySelectorAll(".pp-tchip.on")].map(b => {
            let links = [];
            try { links = JSON.parse(b.dataset.metrics || "[]") || []; } catch (e) { /* none */ }
            return links.map(l => {
                const m = all.find(x => x.id === l.metric_id) || { name: "", unit: "" };
                const k = b.dataset.t + ":" + l.metric_id;
                return `<div class="input-group input-group-sm mb-1" style="max-width:280px"><span class="input-group-text">${esc(m.name)}${l.required ? " *" : ""}</span>
                    <input type="number" step="any" class="form-control" data-act="${b.dataset.t}" data-metric="${l.metric_id}" data-required="${l.required ? 1 : 0}" value="${esc(keep[k] || "")}">
                    ${m.unit ? `<span class="input-group-text">${esc(m.unit)}</span>` : ""}</div>`;
            }).join("");
        }).join("");
    }

    modalEl.addEventListener("click", e => {
        const chip = e.target.closest(".pp-tchip");
        if (chip) {
            chip.classList.toggle("on");
            chip.setAttribute("aria-pressed", chip.classList.contains("on"));
            if (chip.dataset.t) renderNumbers();
            return;
        }
        const tent = e.target.closest("[data-mptent]");
        if (tent) plantsBox.querySelectorAll("[data-pid]").forEach(b => b.classList.toggle("on", b.dataset.zone === tent.dataset.mptent));
    });

    // Start empty every time (browsers restore form values on reload).
    modalEl.addEventListener("show.bs.modal", () => {
        modalEl.querySelectorAll(".pp-tchip.on").forEach(b => b.classList.remove("on"));
        numbers.innerHTML = "";
        $("mpNote").value = "";
        err.classList.add("d-none");
        const d = new Date(), pad = n => String(n).padStart(2, "0");
        const today = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
        $("mpDate").value = today;
        $("mpDate").max = today;
    });

    const fail = msg => { err.textContent = msg; err.classList.remove("d-none"); };

    $("mpSave").addEventListener("click", async () => {
        err.classList.add("d-none");
        const plantIDs = [...plantsBox.querySelectorAll(".pp-tchip.on")].map(b => +b.dataset.pid);
        const activityIDs = [...typesBox.querySelectorAll(".pp-tchip.on")].map(b => +b.dataset.t);
        const note = $("mpNote").value.trim();
        if (!plantIDs.length) return fail(err.dataset.pickPlant);
        if (!activityIDs.length && !note) return fail(err.dataset.pickActivity);
        const measurements = [];
        let missing = false;
        numbers.querySelectorAll("input[data-metric]").forEach(i => {
            const v = i.value.trim();
            if (v === "" && i.dataset.required === "1") { missing = true; i.classList.add("is-invalid"); } else i.classList.remove("is-invalid");
            measurements.push({ activity_id: +i.dataset.act, metric_id: +i.dataset.metric, value: v === "" ? null : Number(v) });
        });
        if (missing) return fail(err.dataset.number);
        const save = $("mpSave");
        save.disabled = true;
        try {
            const resp = await fetch("/plants/day", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plant_ids: plantIDs, date: $("mpDate").value, activity_ids: activityIDs, note, measurements }),
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.error || resp.statusText);
            bootstrap.Modal.getInstance(modalEl).hide();
            location.reload();
        } catch (e) {
            fail(e.message || (uiMessages.t("error_recording_activity") || "Error recording activity. Please try again."));
        } finally {
            save.disabled = false;
        }
    });
});
