// Settings → Grow setup → Smoke reports: score weights (0-5) and which
// optional fields the smoke report form shows. Saved straight away to
// POST /smoke/settings; "Saved" flashes by the title like the other cards.
(function () {
    "use strict";
    const card = document.getElementById("smokeSettingsCard");
    if (!card) return;
    let timer = null;
    const weights = () => {
        const w = {};
        card.querySelectorAll("[data-w]").forEach(b => { w[b.dataset.w] = parseInt(b.textContent, 10) || 0; });
        return w;
    };
    const hidden = () => [...card.querySelectorAll("[data-field]")].filter(c => !c.checked).map(c => c.dataset.field);
    function save() {
        fetch("/smoke/settings", {
            method: "POST",
            headers: { "Content-Type": "application/json", Accept: "application/json" },
            body: JSON.stringify({ weights: weights(), hidden: hidden() }),
        }).then(r => {
            if (!r.ok) throw new Error(r.statusText);
            const saved = document.getElementById("stSaved");
            if (saved) { saved.classList.add("show"); setTimeout(() => saved.classList.remove("show"), 1800); }
        }).catch(e => window.uiMessages && uiMessages.showToast(e.message, "danger"));
    }
    const queue = () => { clearTimeout(timer); timer = setTimeout(save, 400); };
    card.addEventListener("click", e => {
        const b = e.target.closest("[data-wstep]");
        if (!b) return;
        const out = card.querySelector(`[data-w="${b.dataset.wstep}"]`);
        const n = Math.max(0, Math.min(5, (parseInt(out.textContent, 10) || 0) + parseInt(b.dataset.d, 10)));
        out.textContent = n + "×";
        queue();
    });
    card.addEventListener("change", e => { if (e.target.matches("[data-field]")) queue(); });
})();
