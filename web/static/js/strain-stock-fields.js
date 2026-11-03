// Stock fields on Add/Edit Strain (and the import review page): "Wanted" and
// "Seeds added on" follow the seed count, matching the server's rules —
// seeds in stock can't be wanted, and with no seeds there's nothing to date.
// Stocking from zero fills in today's date (still editable). Pack price and
// Freebie describe the latest pack, so they're only sent with seeds on hand.
window.strainStockFields = function () {
    const count = document.getElementById("editSeedCount");
    const added = document.getElementById("editSeedsAddedOn");
    const wanted = document.getElementById("editWanted");
    const price = document.getElementById("editPackPrice");
    const free = document.getElementById("editPackFree");
    const earlier = document.getElementById("editPackEarlier");
    if (!count || !added || !wanted) return null;

    const today = () => {
        const d = new Date();
        const pad = (n) => String(n).padStart(2, "0");
        return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
    };
    const seeds = () => parseInt(count.value, 10) || 0;
    const startSeeds = seeds();
    let lastSeeds = startSeeds;
    const lang = document.documentElement.lang || undefined;
    const money = c => "$" + (c / 100).toLocaleString(lang, { minimumFractionDigits: c % 100 ? 2 : 0, maximumFractionDigits: 2 });
    if (earlier) {
        // The packs before the newest one (the fields edit the newest):
        // "$180 (Aug 24, 2024), Freebie $150 (Oct 2, 2026)".
        let list = [];
        try { list = JSON.parse(earlier.dataset.packs || "[]").filter(p => p.free || p.price_cents != null); } catch (e) { /* none */ }
        const date = iso => new Date(iso + "T12:00:00").toLocaleDateString(lang, { month: "short", day: "numeric", year: "numeric" });
        const one = p => [p.free ? earlier.dataset.free : "", p.price_cents != null ? money(p.price_cents) : ""].filter(Boolean).join(" ") + ` (${date(p.added_on)})`;
        earlier.textContent = earlier.dataset.text.split("{packs}").join(list.map(one).join(", "));
    }

    function sync() {
        const inStock = seeds() > 0;
        if (inStock && lastSeeds <= 0 && !added.value) added.value = today();
        if (inStock) wanted.checked = false;
        wanted.disabled = inStock;
        added.disabled = !inStock;
        if (price) price.disabled = !inStock;
        if (free) free.disabled = !inStock;
        // Earlier packs only count while their seeds are still in stock.
        if (earlier) earlier.hidden = !inStock || startSeeds <= 0;
        lastSeeds = seeds();
    }
    count.addEventListener("input", sync);
    count.addEventListener("change", sync);
    sync();

    function pack() {
        const v = price ? parseFloat(price.value) : NaN;
        return { price_cents: v >= 0 ? Math.round(v * 100) : null, free: !!(free && free.checked) };
    }

    return {
        collect: () => ({
            wanted: wanted.checked && seeds() <= 0,
            seeds_added_on: seeds() > 0 ? added.value : "",
            ...(seeds() > 0 && price ? { pack: pack() } : {}),
        }),
    };
};
