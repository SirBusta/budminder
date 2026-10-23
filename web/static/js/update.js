// The "New release" window (header2.html, handlers/updates.go): your version
// and the new one, what's new, and Update now for the downloadable
// programs: install, wait for BudMinder to restart, then reload.
(() => {
    "use strict";
    const btn = document.getElementById("bmUpdateBtn");
    const data = document.getElementById("bmUpdateText");
    if (!btn || !data) return;
    const T = JSON.parse(data.textContent || "{}");
    const esc = s => String(s).replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    const fill = (s, v) => String(s || "").split("{v}").join(v);
    let modal = null;

    function open() {
        let el = document.getElementById("bmUpdateModal");
        if (!el) {
            document.body.insertAdjacentHTML("beforeend", `<div class="modal fade" id="bmUpdateModal" tabindex="-1" aria-labelledby="bmUpdateTitle" aria-hidden="true">
                <div class="modal-dialog modal-dialog-centered modal-lg modal-dialog-scrollable"><div class="modal-content">
                <div class="modal-header"><h5 class="modal-title" id="bmUpdateTitle"><i class="fa-solid fa-circle-arrow-up text-success me-2"></i>${esc(T.heading)}</h5>
                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="${esc(T.close)}"></button></div>
                <div class="modal-body" id="bmUpdateBody"><div class="text-center py-3"><span class="spinner-border spinner-border-sm"></span></div></div>
                <div class="modal-footer" id="bmUpdateFoot"></div></div></div></div>`);
            el = document.getElementById("bmUpdateModal");
        }
        modal = bootstrap.Modal.getOrCreateInstance(el);
        modal.show();
        fetch("/updates/status", { cache: "no-store", headers: { Accept: "application/json" } })
            .then(r => r.json()).then(show).catch(() => message(T.failed, "danger"));
    }

    function show(s) {
        const body = document.getElementById("bmUpdateBody"), foot = document.getElementById("bmUpdateFoot");
        if (!s.available) {
            body.innerHTML = `<dl class="bm-upd-vers"><dt>${esc(T.current)}</dt><dd>${esc(s.current || "")}</dd></dl>`;
            foot.innerHTML = `<button type="button" class="btn btn-light" data-bs-dismiss="modal">${esc(T.close)}</button>`;
            return;
        }
        const how = s.can_install ? `<p class="small text-muted mb-0">${esc(T.safety)}</p>` : `<p class="mb-0">${esc(s.docker ? T.docker : T.manual)}</p>`;
        body.innerHTML = `<dl class="bm-upd-vers"><dt>${esc(T.current)}</dt><dd>${esc(s.current)}</dd><dt>${esc(T.latest)}</dt><dd>${esc(s.latest)}</dd></dl>
            ${s.notes_html ? `<h6>${esc(T.whatsNew)}</h6><div class="bm-upd-notes">${s.notes_html}</div>` : ""}
            <div class="bm-upd-msg">${how}</div>`;
        foot.innerHTML = `${s.url ? `<a class="btn btn-link me-auto" href="${esc(s.url)}" target="_blank" rel="noopener">${esc(T.onGitHub)} <i class="fa-solid fa-arrow-up-right-from-square ms-1"></i></a>` : ""}
            <button type="button" class="btn btn-light" data-bs-dismiss="modal">${esc(T.later)}</button>
            ${s.can_install ? `<button type="button" class="btn btn-success" id="bmUpdateNow"><i class="fa-solid fa-download me-1"></i>${esc(T.now)}</button>` : ""}`;
        const now = document.getElementById("bmUpdateNow");
        if (now) now.addEventListener("click", () => install(s.latest));
    }

    function message(text, kind, spin) {
        const box = document.querySelector("#bmUpdateBody .bm-upd-msg") || document.getElementById("bmUpdateBody");
        box.innerHTML = `<div class="alert alert-${kind || "info"} mb-0">${spin ? `<span class="spinner-border spinner-border-sm me-2"></span>` : ""}${esc(text)}</div>`;
    }

    async function install(version) {
        const foot = document.getElementById("bmUpdateFoot");
        foot.querySelectorAll("button, a").forEach(b => { b.disabled = true; b.classList.add("disabled"); });
        message(fill(T.downloading, version), "info", true);
        try {
            const r = await fetch("/updates/install", { method: "POST", headers: { Accept: "application/json" } });
            const d = await r.json().catch(() => ({}));
            if (!r.ok) throw new Error(d.detail || d.error || "");
        } catch (e) {
            message(T.failed + (e.message ? " (" + e.message + ")" : ""), "danger");
            foot.querySelectorAll("button, a").forEach(b => { b.disabled = false; b.classList.remove("disabled"); });
            return;
        }
        message(T.restarting, "info", true);
        // Wait for BudMinder to go away and come back as the new version.
        const started = Date.now();
        for (;;) {
            await new Promise(z => setTimeout(z, 1500));
            try {
                const s = await fetch("/updates/status", { cache: "no-store", headers: { Accept: "application/json" } }).then(r => r.json());
                if (s.current === version) {
                    message(fill(T.done, version), "success");
                    setTimeout(() => location.reload(), 1200);
                    return;
                }
            } catch (e) { /* restarting: not answering yet */ }
            if (Date.now() - started > 120000) {
                message(T.slow, "warning");
                return;
            }
        }
    }

    btn.addEventListener("click", open);
})();
