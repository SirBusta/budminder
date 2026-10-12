// The single-strain page (mockup SD1+): the family diagram (grandparents →
// parents → this strain → strains bred from it), Seed Vault + / −, and the
// wish-list button. Texts come from data-* attributes on #sdPage (files
// under /static aren't templated).
(function () {
    "use strict";
    const page = document.getElementById("sdPage");
    if (!page) return;
    const D = page.dataset;
    const ID = D.strainId;
    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const fill = (tpl, vars) => String(tpl || "").replace(/\{(\w+)\}/g, (m, k) => (k in vars ? vars[k] : m));

    async function api(url, body) {
        const resp = await fetch(url, { method: "POST", cache: "no-store", headers: { "Content-Type": "application/json", "Accept": "application/json" }, body: JSON.stringify(body) });
        const data = await resp.json().catch(() => ({}));
        if (!resp.ok) throw new Error(data.error || data.message || resp.statusText);
        return data;
    }
    const fail = e => {
        if (typeof uiMessages !== "undefined") uiMessages.showToast(e && e.message ? e.message : D.failed, "danger");
    };

    // ---------- Family diagram ----------
    const fam = document.getElementById("sdFamily");
    let lineage = [], kids = [], showAll = false;

    function node(n, small, extra) {
        const name = esc(n.parent_name);
        let link;
        if (n.parent_strain_id) {
            link = `<a class="sd-node lib" href="/strain/${n.parent_strain_id}">${name}${small ? `<small>${esc(small)}</small>` : ""}</a>`;
        } else {
            link = `<a class="sd-node ext" href="/strain/new?name=${encodeURIComponent(n.parent_name)}" title="${esc(fill(D.addAs, { name: n.parent_name }))}">${name}${small ? `<small>${esc(small)}</small>` : ""}</a>`;
        }
        if (D.straincompass) {
            link = `<div class="sd-node-wrap">${link}<a class="sd-node-sc" href="https://straincompass.com/en/strains?q=${encodeURIComponent(n.parent_name)}" target="_blank" rel="noopener noreferrer" title="${esc(fill(D.searchSc, { name: n.parent_name }))}" aria-label="${esc(fill(D.searchSc, { name: n.parent_name }))}"><i class="fa-solid fa-arrow-up-right-from-square"></i></a></div>`;
        }
        return link + (extra || "");
    }

    function drawFamily() {
        if (!fam) return;
        // Generations: [parents], [grandparents (via parent)], …
        const gens = [];
        let level = lineage.map(p => ({ n: p, via: "" }));
        while (level.length) {
            gens.push(level);
            level = level.flatMap(x => (x.n.children || []).map(c => ({ n: c, via: x.n.parent_name })));
        }
        const more = gens.length > 2;
        const shown = showAll ? gens : gens.slice(0, 2);
        const arrow = `<span class="sd-arr" aria-hidden="true"><i class="fa-solid fa-arrow-right"></i></span>`;
        const cols = shown.slice().reverse().map((g, i) => {
            const depth = shown.length - 1 - i; // 0 = parents
            return `<div class="sd-col">${g.map(x => node(x.n, depth === 0 ? (x.n.parent_strain_id ? D.parentMine : D.parent) : fill(D.via, { name: x.via }))).join("")}</div>`;
        });
        const me = `<div class="sd-col"><div class="sd-node me">${esc(D.name)}${D.breeder ? `<small>${esc(D.breeder)}</small>` : ""}</div></div>`;
        const kidCol = kids.length ? `<div class="sd-col">${kids.map(k => `<a class="sd-node kid" href="/strain/${k.id}">${esc(k.name)}<small>${esc(k.other_parents ? `${D.name} × ${k.other_parents}` : (k.breeder || D.bredFrom))}</small></a>`).join("")}</div>` : "";
        if (!gens.length && !kids.length) {
            fam.innerHTML = D.hasNote ? "" : `<p class="sd-muted mb-0">${esc(D.none)}</p>`;
        } else {
            fam.innerHTML = `<div class="sd-fam">${[...cols, me].join(arrow)}${kidCol ? arrow + kidCol : ""}</div>
                ${lineage.some(function missing(p) { return !p.parent_strain_id || (p.children || []).some(missing); }) ? `<p class="sd-muted small mt-2 mb-0">${esc(D.familyNote)}</p>` : ""}`;
        }
        const btn = document.getElementById("sdMoreGens");
        if (btn) {
            btn.hidden = !more;
            btn.textContent = showAll ? D.fewer : D.more;
        }
    }

    if (fam) {
        Promise.all([
            fetch(`/strains/${ID}/lineage`, { cache: "no-store" }).then(r => r.ok ? r.json() : []).catch(() => []),
            fetch(`/strains/${ID}/descendants`, { cache: "no-store" }).then(r => r.ok ? r.json() : []).catch(() => []),
        ]).then(([l, k]) => { lineage = l || []; kids = k || []; drawFamily(); });
        document.getElementById("sdMoreGens")?.addEventListener("click", e => { e.preventDefault(); showAll = !showAll; drawFamily(); });
    }

    // ---------- Seed Vault ----------
    page.addEventListener("click", async e => {
        const b = e.target.closest("[data-seeds]");
        if (!b) return;
        b.disabled = true;
        try {
            const up = b.dataset.seeds === "add";
            await api(`/strains/${ID}/seeds${up ? "" : "/use"}`, { count: 1 });
            location.reload(); // the count, date, stock state and Start a grow all follow it
        } catch (err) {
            fail(err);
            b.disabled = false;
        }
    });

    // ---------- Wish list (only while out of stock) ----------
    const want = document.getElementById("sdWanted");
    want?.addEventListener("click", async () => {
        want.disabled = true;
        try {
            const data = await api(`/strains/${ID}/wanted`, { wanted: want.dataset.wanted !== "true" });
            want.dataset.wanted = String(data.wanted);
            want.innerHTML = `<i class="fa-${data.wanted ? "solid" : "regular"} fa-star me-1"></i>${esc(data.wanted ? D.wantedRemove : D.wantedAdd)}`;
            document.getElementById("sdWantedBadge")?.classList.toggle("d-none", !data.wanted);
        } catch (err) {
            fail(err);
        } finally {
            want.disabled = false;
        }
    });
})();
