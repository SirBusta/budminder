// Compare photo qualities (views/photo-compare.html,
// handlers/photo_settings_http.go): the original and a copy compressed at
// the chosen quality, made by the server the same way new photos are. Hold
// and drag to move, scroll to zoom, Space or the green button switches.
// "Use quality NN" saves the quality (and turns compression on).
(() => {
    "use strict";
    const D = JSON.parse(document.getElementById("pccData").textContent || "{}");
    const T = D.t || {};
    const $ = id => document.getElementById(id);
    const fill = (s, v) => Object.keys(v).reduce((t, k) => t.split("{" + k + "}").join(v[k]), String(s || ""));
    const esc = s => String(s).replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
    const lang = document.documentElement.lang || undefined;
    const mb = n => (n / 1e6).toLocaleString(lang, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + " MB";
    const day = iso => iso ? new Date(iso + "T00:00:00").toLocaleDateString(lang, { month: "short", day: "numeric" }) : "";
    const QS = D.qualities || [90, 85, 80, 75, 70, 65, 60, 55, 50];

    const SAMPLES = (D.samples || []).map((s, i) => ({
        src: "sample:" + (i + 1), name: s.name, date: s.date, sample: true,
        thumb: `/static/img/compare-samples/sample-${i + 1}-thumb.jpg`,
    }));
    const MINE = [];
    let page = 0;

    const A = $("imgA"), B = $("imgB"), stage = $("stage"), label = $("label");
    const toggle = $("toggle"), q = $("q"), use = $("use"), pick = $("pick"), busy = $("busy");
    let photo = null, sizes = null, showB = false, scale = 1, x = 0, y = 0, loading = 0;
    q.innerHTML = QS.map(v => `<option value="${v}">${esc(fill(T.option, { q: v, s: "…" }))}</option>`).join("");
    q.value = String(D.quality || 75);
    if (D.locked) use.disabled = true;

    const imageURL = (p, quality) => `/settings/photos/compare/image?src=${encodeURIComponent(p.src)}&q=${quality}`;

    function fillQualities() {
        q.querySelectorAll("option").forEach(o => {
            const n = sizes && sizes.sizes[o.value];
            o.textContent = fill(T.option, { q: o.value, s: n ? mb(n) : "…" });
        });
    }

    function waitFor(img, url) {
        return new Promise(res => {
            if (img.getAttribute("src") === url && img.complete) return res();
            img.addEventListener("load", res, { once: true });
            img.addEventListener("error", res, { once: true });
            img.src = url;
        });
    }

    async function load(p) {
        photo = p;
        sizes = null;
        const mine = ++loading;
        busy.classList.add("on");
        fillQualities();
        apply();
        const first = waitFor(A, imageURL(p, 0)).then(() => { if (mine === loading) fit(); });
        try {
            const r = await fetch(`/settings/photos/compare/sizes?src=${encodeURIComponent(p.src)}`, { headers: { Accept: "application/json" } });
            if (mine !== loading) return;
            sizes = r.ok ? await r.json() : null;
        } catch (e) { sizes = null; }
        fillQualities();
        await Promise.all([first, waitFor(B, imageURL(p, q.value))]);
        if (mine !== loading) return;
        busy.classList.remove("on");
        apply();
    }

    function apply() {
        for (const img of [A, B]) img.style.transform = `translate(${x}px, ${y}px) scale(${scale})`;
        B.style.visibility = showB ? "visible" : "hidden";
        toggle.textContent = showB ? T.showOriginal : T.showCompressed;
        use.innerHTML = `<i class="fa-solid fa-check"></i> ${esc(fill(T.use, { q: q.value }))}`;
        if (!photo) return;
        const who = [photo.name, day(photo.date), photo.sample ? T.sample : ""].filter(Boolean).join(" · ");
        const orig = sizes ? sizes.original : 0, size = sizes ? sizes.sizes[q.value] : 0;
        if (showB) {
            let note = "";
            if (orig && size) {
                if (size >= orig) note = `<span class="warn">${esc(T.bigger)}</span>`;
                else if (size * 100 > orig * 90) note = `<span class="warn">${esc(T.under10)}</span>`;
                else note = esc(fill(T.share, { p: Math.round(size / orig * 100) }));
            }
            label.innerHTML = `${esc(fill(T.compressed, { q: q.value, s: size ? mb(size) : "…" }))} <small>${esc(who)}${note ? " · " + note : ""}</small>`;
        } else {
            label.innerHTML = `${esc(fill(T.original, { s: orig ? mb(orig) : "…" }))} <small>${esc(who)}</small>`;
        }
    }

    function fit() {
        const w = A.naturalWidth || 4000, h = A.naturalHeight || 3000;
        scale = Math.min(stage.clientWidth / w, stage.clientHeight / h);
        x = (stage.clientWidth - w * scale) / 2;
        y = (stage.clientHeight - h * scale) / 2;
        apply();
    }
    function zoomTo(s, cx = stage.clientWidth / 2, cy = stage.clientHeight / 2) {
        x = cx - (cx - x) * (s / scale);
        y = cy - (cy - y) * (s / scale);
        scale = s;
        apply();
    }

    // ---------- Photo picker ----------
    function tile(p) {
        const size = p.bytes ? " · " + mb(p.bytes) : "";
        return `<button class="ph" type="button" data-src="${esc(p.src)}">${p.sample ? `<span class="tag">${esc(T.sampleTag)}</span>` : ""}<img src="${esc(p.thumb)}" alt="" loading="lazy"><span>${esc(p.name)}<br><small>${esc(day(p.date))}${size}</small></span></button>`;
    }
    function drawPicker() {
        $("mine").innerHTML = MINE.map(tile).join("");
        $("samples").innerHTML = SAMPLES.map(tile).join("");
        $("mineBox").hidden = MINE.length === 0;
        pick.querySelectorAll(".ph").forEach(b => b.classList.toggle("on", photo && b.dataset.src === photo.src));
    }
    async function morePhotos() {
        try {
            const r = await fetch(`/settings/photos/compare/photos?page=${page}`, { headers: { Accept: "application/json" } });
            const d = r.ok ? await r.json() : { photos: [] };
            for (const p of d.photos || []) {
                MINE.push({ src: "plant:" + p.id, name: p.plant, date: p.date, bytes: p.bytes,
                    thumb: `/settings/photos/compare/thumb?src=plant:${p.id}` });
            }
            $("more").hidden = !d.more;
            page++;
        } catch (e) { /* the samples still work */ }
        drawPicker();
    }
    pick.addEventListener("click", e => {
        const b = e.target.closest(".ph");
        if (b) {
            const p = MINE.concat(SAMPLES).find(x => x.src === b.dataset.src);
            pick.classList.remove("open");
            if (p) load(p);
        } else if (e.target === pick || e.target.closest("#close")) pick.classList.remove("open");
    });
    $("more").addEventListener("click", morePhotos);
    $("choose").addEventListener("click", () => { drawPicker(); pick.classList.add("open"); });

    // ---------- Controls ----------
    toggle.addEventListener("click", () => { showB = !showB; apply(); });
    document.addEventListener("keydown", e => {
        if (e.code === "Space" && !pick.classList.contains("open") && e.target.tagName !== "SELECT" && e.target.tagName !== "BUTTON") {
            e.preventDefault(); showB = !showB; apply();
        }
        if (e.code === "Escape") pick.classList.remove("open");
    });
    q.addEventListener("change", () => {
        if (!photo) return;
        busy.classList.add("on");
        apply();
        waitFor(B, imageURL(photo, q.value)).then(() => { busy.classList.remove("on"); apply(); });
    });
    let toastTimer = null;
    function toast(text, err) {
        const t = $("toast");
        $("toastTxt").textContent = text;
        t.classList.toggle("err", !!err);
        t.style.display = "block";
        clearTimeout(toastTimer);
        toastTimer = setTimeout(() => { t.style.display = "none"; }, 3500);
    }
    use.addEventListener("click", async () => {
        const quality = +q.value;
        use.disabled = true;
        try {
            const r = await fetch("/settings/photos", {
                method: "POST",
                headers: { "Content-Type": "application/json", Accept: "application/json" },
                body: JSON.stringify({ compress: true, quality }),
            });
            const d = await r.json().catch(() => ({}));
            if (r.ok) toast(fill(T.saved, { q: quality }));
            else if (!d.demo) toast(r.status === 409 ? T.locked : (d.error || T.failed), true);
        } catch (e) { toast(T.failed, true); }
        use.disabled = false;
    });
    $("fit").addEventListener("click", fit);
    $("one").addEventListener("click", () => zoomTo(1));
    $("three").addEventListener("click", () => zoomTo(3));
    stage.addEventListener("wheel", e => {
        e.preventDefault();
        zoomTo(Math.min(8, Math.max(0.05, scale * (e.deltaY < 0 ? 1.2 : 1 / 1.2))), e.offsetX, e.offsetY);
    }, { passive: false });

    // Press, move, release: the drag only lasts while the button is held.
    let drag = null;
    function endDrag() { drag = null; stage.classList.remove("dragging"); }
    stage.addEventListener("pointerdown", e => {
        if (e.button !== 0) return;
        e.preventDefault();
        drag = { sx: e.clientX - x, sy: e.clientY - y };
        stage.classList.add("dragging");
    });
    window.addEventListener("pointermove", e => {
        if (!drag) return;
        if (e.buttons === 0) { endDrag(); return; } // released outside the window
        x = e.clientX - drag.sx; y = e.clientY - drag.sy; apply();
    });
    window.addEventListener("pointerup", endDrag);
    window.addEventListener("pointercancel", endDrag);
    window.addEventListener("blur", endDrag);
    stage.addEventListener("dragstart", e => e.preventDefault());
    window.addEventListener("resize", fit);

    // Your newest photo, or the first sample on a fresh install.
    morePhotos().then(() => load(MINE[0] || SAMPLES[0]));
})();
