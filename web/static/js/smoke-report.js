// One smoke report (views/smoke-report.html): photo gallery, delete, and
// dates shown in the reader's own format.
(function () {
    "use strict";
    const page = document.getElementById("smokeReport");
    if (!page) return;
    const D = page.dataset;
    const lang = D.lang || undefined;

    const fmt = (iso, opts) => new Date(iso + "T12:00:00").toLocaleDateString(lang, opts);
    page.querySelectorAll("[data-date]").forEach(el => {
        if (/^\d{4}-\d{2}-\d{2}$/.test(el.dataset.date)) el.textContent = fmt(el.dataset.date, { month: "short", day: "numeric", year: "numeric" });
    });
    page.querySelectorAll("[data-date-short]").forEach(el => {
        if (/^\d{4}-\d{2}-\d{2}$/.test(el.dataset.dateShort)) el.textContent = fmt(el.dataset.dateShort, { month: "short", day: "numeric" });
    });

    // Gallery: a thumbnail shows that photo as the main one.
    const main = document.getElementById("srMain");
    const thumbs = document.getElementById("srThumbs");
    if (main && thumbs) {
        const show = img => {
            main.src = img.dataset.full;
            thumbs.querySelectorAll("img").forEach(t => t.classList.toggle("on", t === img));
        };
        thumbs.addEventListener("click", e => { const img = e.target.closest("img"); if (img) show(img); });
        thumbs.addEventListener("keydown", e => {
            const img = e.target.closest("img");
            if (img && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); show(img); }
        });
    }

    const del = document.getElementById("srDelete");
    if (del) del.addEventListener("click", async () => {
        const ok = await uiMessages.showConfirm(D.confirm, { danger: true, okLabel: D.delete });
        if (!ok) return;
        const resp = await fetch("/smoke/" + D.id, { method: "DELETE" });
        if (resp.ok) location.href = "/smoke";
        else {
            const data = await resp.json().catch(() => ({}));
            uiMessages.showToast(data.error || resp.statusText, "danger");
        }
    });
})();
