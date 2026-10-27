// The line under the top menu while existing photos are being compressed
// (header2.html, handlers/photo_compress_service.go): green with the photos
// left while running, amber when paused, gone when finished. Polls the
// status every 2 s only while running. Settings > Photos listens to the
// "bm-photo-job" event this sends with each status, and calls
// bmPhotoJob.watch() after starting or resuming a run.
(() => {
    "use strict";
    const line = document.getElementById("bmPhotoJob");
    const fill = document.getElementById("bmPhotoJobFill");
    const txt = document.getElementById("bmPhotoJobTxt");
    const data = document.getElementById("bmPhotoJobText");
    if (!line || !data) return;
    const T = JSON.parse(data.textContent || "{}");
    const esc = s => String(s).replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    let timer = null;

    function show(s) {
        const on = s.state === "running" || s.state === "paused";
        line.classList.toggle("gone", !on);
        line.classList.toggle("paused", s.state === "paused");
        line.dataset.state = s.state || "";
        fill.style.width = (s.percent || 0).toFixed(1) + "%";
        if (s.state === "paused") {
            txt.innerHTML = `${esc(T.paused)} · <a href="/settings#photos">${esc(T.resume)}</a>`;
        } else if (s.state === "running") {
            txt.textContent = s.phase === "backup" ? T.backing : String(T.running).split("{n}").join(s.left);
        }
        document.dispatchEvent(new CustomEvent("bm-photo-job", { detail: s }));
    }

    function poll() {
        clearTimeout(timer);
        fetch("/settings/photos/existing/status", { cache: "no-store", headers: { Accept: "application/json" } })
            .then(r => r.ok ? r.json() : Promise.reject())
            .then(s => {
                show(s);
                if (s.state === "running") timer = setTimeout(poll, 2000);
            })
            .catch(() => { timer = setTimeout(poll, 5000); });
    }

    window.bmPhotoJob = { watch: poll, show };
    if (line.dataset.state === "running") timer = setTimeout(poll, 2000);
})();
