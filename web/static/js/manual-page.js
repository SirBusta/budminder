// The Help manual page (views/manual.html): search, the phone section
// picker, "On this page" highlighting, and full-size pictures.
(() => {
    const $ = id => document.getElementById(id);
    const esc = s => String(s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

    // Phone: the section drop-down.
    $("mnPick")?.addEventListener("change", e => { location.href = "/manual/" + encodeURIComponent(e.target.value); });

    // Search: titles and text of every section, loaded on first use.
    let index = null;
    const box = $("mnSearch"), out = $("mnResults"), nav = $("mnNav");
    async function search() {
        const q = box.value.trim().toLowerCase();
        if (!q) { out.classList.add("d-none"); nav.classList.remove("d-none"); return; }
        if (!index) {
            try { index = await (await fetch("/manual/index.json")).json(); } catch { index = []; }
        }
        const words = q.split(/\s+/);
        const hits = index.map(s => {
            const title = s.title.toLowerCase(), text = s.text.toLowerCase();
            if (!words.every(w => title.includes(w) || text.includes(w))) return null;
            const score = words.reduce((n, w) => n + (title.includes(w) ? 10 : 0) + (text.split(w).length - 1), 0);
            // A short bit of text around the first match.
            const at = text.indexOf(words[0]);
            const snip = at < 0 ? "" : (at > 40 ? "…" : "") + s.text.slice(Math.max(0, at - 40), at + 80) + "…";
            return { s, score, snip };
        }).filter(Boolean).sort((a, b) => b.score - a.score);
        out.innerHTML = hits.length ? hits.slice(0, 12).map(h =>
            `<a href="/manual/${encodeURIComponent(h.s.slug)}"><b>${esc(h.s.title)}</b><small>${esc(h.s.group)}</small>${h.snip ? `<span>${esc(h.snip)}</span>` : ""}</a>`).join("")
            : `<p class="mn-none">${esc(out.dataset.none)}</p>`;
        out.classList.remove("d-none");
        nav.classList.add("d-none");
    }
    box?.addEventListener("input", search);
    box?.addEventListener("keydown", e => {
        if (e.key === "Enter") { const a = out.querySelector("a"); if (a) location.href = a.href; }
        if (e.key === "Escape") { box.value = ""; search(); }
    });

    // On this page: highlight the heading being read.
    const links = [...document.querySelectorAll(".mn-toc a")];
    const heads = links.map(a => document.getElementById(a.getAttribute("href").slice(1))).filter(Boolean);
    function mark() {
        let cur = heads[0];
        for (const h of heads) if (h.getBoundingClientRect().top < 120) cur = h;
        links.forEach(a => a.classList.toggle("on", cur && a.getAttribute("href") === "#" + cur.id));
    }
    if (heads.length) { addEventListener("scroll", mark, { passive: true }); mark(); }

    // Pictures: tap for full size (they're taken at double size); tap or Esc closes.
    document.querySelector(".mn-body")?.addEventListener("click", e => {
        const img = e.target.closest(".mn-fig img");
        if (!img) return;
        const z = document.createElement("div");
        z.className = "mn-zoom";
        z.setAttribute("role", "dialog");
        z.innerHTML = `<img src="${esc(img.currentSrc || img.src)}" alt="${esc(img.alt)}">`;
        z.firstChild.style.width = Math.min(img.naturalWidth * 0.62, img.naturalWidth) + "px";
        const close = () => { z.remove(); document.removeEventListener("keydown", onKey); };
        const onKey = ev => { if (ev.key === "Escape") close(); };
        z.addEventListener("click", close);
        document.addEventListener("keydown", onKey);
        document.body.appendChild(z);
    });
})();
