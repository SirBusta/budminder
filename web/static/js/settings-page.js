// Settings (views/settings.html), mockup ST1: side menu of sections, a
// search box that shows matching cards from every section, and auto-save
// (each change is saved straight away; "Saved" flashes by the title).
// The card scripts in the template (zones, activities, backups, …) are
// unchanged; this file only adds the page behavior around them.
(function () {
    "use strict";
    const page = document.getElementById("stPage");
    if (!page) return;
    const sections = [...page.querySelectorAll(".st-sec")];
    const side = document.getElementById("stSide");
    const q = document.getElementById("stQ");
    const title = document.getElementById("stTitle"), desc = document.getElementById("stDesc"), none = document.getElementById("stNone");
    const L = page.dataset;
    const fold = s => (s || "").toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");

    // ---------- Sections ----------
    const keys = sections.map(s => s.dataset.sec);
    let current = keys.includes(location.hash.slice(1)) ? location.hash.slice(1) : "general";
    const shown = new Set();
    function showSection(key, push) {
        current = key;
        sections.forEach(s => { s.hidden = s.dataset.sec !== key; });
        const sec = sections.find(s => s.dataset.sec === key);
        title.textContent = sec.dataset.title;
        desc.textContent = sec.dataset.desc;
        side.querySelectorAll("a[data-sec]").forEach(a => { a.classList.toggle("on", a.dataset.sec === key); a.classList.remove("dim"); });
        if (push) history.pushState(null, "", "#" + key);
        else history.replaceState(null, "", location.pathname + "#" + key);
        // Cards that load on demand (logs, saved backups) listen for this.
        document.dispatchEvent(new CustomEvent("st:section", { detail: key }));
        shown.add(key);
    }

    // ---------- Search ----------
    // Each card's searchable text: its translated title, description and
    // labels, skipping big data blocks (log lines, the timezone list).
    function cardText(card) {
        if (card._st == null) {
            let out = "";
            const walk = n => {
                if (n.nodeType === 3) { out += " " + n.nodeValue; return; }
                if (n.nodeType !== 1 || n.classList.contains("st-nosearch")) return;
                n.childNodes.forEach(walk);
            };
            walk(card);
            card._st = fold(out.replace(/\s+/g, " "));
        }
        return card._st;
    }
    let searching = false;
    function search() {
        const raw = q.value.trim(), term = fold(raw);
        if (!term) {
            if (searching) {
                searching = false;
                page.classList.remove("searching");
                page.querySelectorAll(".st-card").forEach(c => { c.hidden = false; });
                none.classList.add("d-none");
                showSection(current, false);
            }
            return;
        }
        searching = true;
        page.classList.add("searching");
        title.textContent = L.searchResults || raw;
        let total = 0;
        sections.forEach(sec => {
            let n = 0;
            sec.querySelectorAll(".st-card").forEach(c => {
                const hit = cardText(c).includes(term);
                c.hidden = !hit;
                if (hit) n++;
            });
            sec.hidden = !n;
            total += n;
            const link = side.querySelector(`a[data-sec="${sec.dataset.sec}"]`);
            if (link) { link.classList.toggle("dim", !n); link.classList.remove("on"); }
        });
        desc.textContent = total ? (total === 1 ? L.searchCountOne : L.searchCount).replace("{n}", total).replace("{q}", raw) : "";
        none.classList.toggle("d-none", total > 0);
        // A section that wasn't opened yet may need its data (e.g. logs).
        sections.filter(s => !s.hidden && !shown.has(s.dataset.sec)).forEach(s => {
            shown.add(s.dataset.sec);
            document.dispatchEvent(new CustomEvent("st:section", { detail: s.dataset.sec }));
        });
    }
    q.closest("form").addEventListener("submit", e => e.preventDefault());
    q.addEventListener("input", search);
    q.addEventListener("keydown", e => { if (e.key === "Escape") { q.value = ""; search(); } });

    side.addEventListener("click", e => {
        const a = e.target.closest("a[data-sec]");
        if (!a) return;
        e.preventDefault();
        q.value = "";
        search();
        showSection(a.dataset.sec, true);
        window.scrollTo(0, 0);
    });
    page.addEventListener("click", e => {
        const g = e.target.closest("[data-goto]");
        if (!g) return;
        e.preventDefault();
        q.value = "";
        search();
        showSection(g.dataset.goto, true);
        window.scrollTo(0, 0);
    });
    window.addEventListener("popstate", () => {
        const k = location.hash.slice(1);
        if (keys.includes(k)) { q.value = ""; search(); showSection(k, false); }
    });

    // ---------- Auto-save ----------
    const val = id => document.getElementById(id);
    function payload() {
        return {
            aci: { enabled: val("aciEnabled").checked },
            ec: { enabled: val("ecEnabled").checked },
            cannadb: { enabled: val("cannadbEnabled").checked, base_url: val("cannadbBaseUrl").value.trim() },
            straincompass: {
                enabled: val("straincompassEnabled").checked,
                base_url: val("straincompassBaseUrl").value.trim(),
                api_key: val("straincompassApiKey").value.trim(),
            },
            homeassistant: {
                enabled: val("haEnabled").checked,
                url: val("haUrl").value.trim(),
                token: val("haToken").value.trim(),
            },
            polling_interval: val("pollingInterval").value,
            guest_mode: val("guestMode").checked,
            stream_grab_enabled: val("streamGrabEnabled").checked,
            stream_grab_interval: String(val("streamGrabInterval").value * 60),
            api_key: "",
            disable_api_ingest: val("disableApiIngest").checked,
            sensor_retention_days: val("sensorRetentionDays").value,
            log_level: val("logLevel").value,
            max_backup_size_mb: val("maxBackupSizeMB").value,
            timezone: val("timezone").value,
            units: val("units").value,
        };
    }
    const saved = document.getElementById("stSaved");
    let timer = null, flashTimer = null, saving = Promise.resolve();
    function flash() {
        saved.classList.add("show");
        clearTimeout(flashTimer);
        flashTimer = setTimeout(() => saved.classList.remove("show"), 1800);
    }
    function save() {
        const body = payload();
        const sentKey = body.straincompass.api_key, sentHAToken = body.homeassistant.token;
        saving = saving.then(() => fetch("/settings", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        }).then(r => {
            if (!r.ok) throw new Error();
            flash();
            if (sentKey) {
                // The key is never shown again; the badge says it's set.
                const input = val("straincompassApiKey"), badge = val("straincompassKeyBadge");
                if (input.value.trim() === sentKey) input.value = "";
                input.placeholder = "••••••••";
                badge.textContent = L.tokenSet;
                badge.className = "badge bg-success";
            }
            if (sentHAToken) {
                const input = val("haToken"), badge = val("haTokenBadge");
                if (input.value.trim() === sentHAToken) input.value = "";
                input.placeholder = "••••••••";
                badge.textContent = L.tokenSet;
                badge.className = "badge bg-success";
            }
        }).catch(() => {
            if (window.uiMessages) uiMessages.showToast(L.saveFailed || "Failed to save settings", "danger");
        }));
    }
    function queueSave() { clearTimeout(timer); timer = setTimeout(save, 400); }

    // ---------- Home Assistant: Test connection ----------
    // Tests what's typed (address and a newly pasted token), falling back
    // to the saved settings on the server.
    const haTest = val("haTest");
    if (haTest) haTest.addEventListener("click", async () => {
        const out = val("haTestResult"), label = haTest.innerHTML;
        haTest.disabled = true;
        out.className = "st-ha-result";
        out.textContent = L.haTesting;
        try {
            const resp = await fetch("/settings/homeassistant/test", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ url: val("haUrl").value.trim(), token: val("haToken").value.trim() }),
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) throw new Error(data.error || L.haTestFailed);
            out.textContent = L.haTestOk.replace("{n}", data.sensors).replace("{g}", data.grow);
            out.classList.add("ok");
        } catch (e) {
            out.textContent = e.message || L.haTestFailed;
            out.classList.add("bad");
        } finally {
            haTest.disabled = false;
            haTest.innerHTML = label;
        }
    });
    page.addEventListener("change", e => { if (e.target.matches("[data-autosave]")) queueSave(); });
    // Text fields save when you leave them or press Enter (change), not per keystroke.
    page.addEventListener("keydown", e => {
        if (e.key === "Enter" && e.target.matches("input[data-autosave]")) { e.preventDefault(); e.target.blur(); }
    });

    // ---------- Theme (per browser; same storage as the header button) ----------
    const THEME_KEY = "isley-theme";
    const themeBox = document.getElementById("stTheme");
    function storedTheme() { try { return localStorage.getItem(THEME_KEY); } catch (e) { return null; } }
    function applyTheme(choice) {
        const theme = choice === "auto" ? (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light") : choice;
        try { choice === "auto" ? localStorage.removeItem(THEME_KEY) : localStorage.setItem(THEME_KEY, choice); } catch (e) { /* private window */ }
        document.documentElement.setAttribute("data-bs-theme", theme);
        const icon = document.getElementById("themeToggleIcon");
        if (icon) { icon.classList.toggle("fa-moon", theme === "dark"); icon.classList.toggle("fa-sun", theme !== "dark"); }
        themeBox.querySelectorAll("button").forEach(b => b.classList.toggle("on", b.dataset.theme === choice));
    }
    if (themeBox) {
        themeBox.querySelectorAll("button").forEach(b => b.classList.toggle("on", b.dataset.theme === (storedTheme() || "auto")));
        themeBox.addEventListener("click", e => { const b = e.target.closest("[data-theme]"); if (b) applyTheme(b.dataset.theme); });
        document.getElementById("themeToggle")?.addEventListener("click", () => setTimeout(() => {
            themeBox.querySelectorAll("button").forEach(b => b.classList.toggle("on", b.dataset.theme === (storedTheme() || "auto")));
        }));
    }

    // ---------- Language (per browser; same as the globe menu) ----------
    // Each language listed in its own name ("Deutsch", "Español", …).
    document.querySelectorAll("#stLang option").forEach(o => {
        try {
            const n = new Intl.DisplayNames([o.value], { type: "language" }).of(o.value);
            if (n) o.textContent = n.charAt(0).toUpperCase() + n.slice(1);
        } catch (e) { /* keep the code */ }
    });
    document.getElementById("stLang")?.addEventListener("change", e => {
        const lang = e.target.value;
        if (!/^[a-zA-Z-]+$/.test(lang)) return;
        try { localStorage.setItem("language", lang); } catch (err) { /* private window */ }
        location.href = `/settings?lang=${encodeURIComponent(lang)}#${current}`;
    });

    // ---------- Activity types: measurement names ----------
    const metricNames = {};
    page.querySelectorAll(".metric-row").forEach(r => { try { const m = JSON.parse(r.dataset.metric); metricNames[m.id] = m.name; } catch (e) { /* skip */ } });
    page.querySelectorAll(".st-act-metrics").forEach(td => {
        const names = (td.dataset.ids || "").split(",").filter(Boolean).map(id => metricNames[id]).filter(Boolean);
        td.textContent = names.length ? names.join(", ") : "—";
    });

    // ---------- Logs: level filter and search over the loaded lines ----------
    const logOut = document.getElementById("logOutput"), logQ = document.getElementById("stLogQ"), lvlBox = document.getElementById("stLogLevel");
    function filterLogs() {
        if (!logOut || logOut._raw == null) return;
        const lvl = (lvlBox.querySelector(".on") || {}).dataset?.lvl || "", term = logQ.value.trim().toLowerCase();
        if (!lvl && !term) { logOut.textContent = logOut._raw || "(empty)"; return; }
        const levels = lvl === "error" ? /level=(error|fatal|panic)/i : /level=(warn|warning|error|fatal|panic)/i;
        const lines = logOut._raw.split("\n").filter(l => l && (!lvl || levels.test(l)) && (!term || l.toLowerCase().includes(term)));
        logOut.textContent = lines.length ? lines.join("\n") : (L.logNothing || "—");
        logOut.scrollTop = logOut.scrollHeight;
    }
    document.addEventListener("st:logs-loaded", filterLogs);
    logQ?.addEventListener("input", filterLogs);
    lvlBox?.addEventListener("click", e => {
        const b = e.target.closest("[data-lvl]");
        if (!b) return;
        lvlBox.querySelectorAll("button").forEach(x => x.classList.toggle("on", x === b));
        filterLogs();
    });

    // ---------- Start ----------
    document.addEventListener("DOMContentLoaded", () => {
        const pre = new URLSearchParams(location.search).get("q");
        showSection(current, false);
        if (pre) { q.value = pre.slice(0, 60); search(); }
    });
})();
