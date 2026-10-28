// Settings > Alerts (settings.html, handlers/alert_settings.go): the Email
// card saves when a field changes (POST /settings/alerts/email) and Send
// test email sends with what's typed, saved or not. A blank password field
// keeps the saved password.
(() => {
    "use strict";
    const card = document.getElementById("alCard");
    const data = document.getElementById("alText");
    if (!card || !data) return;
    const T = JSON.parse(data.textContent || "{}");
    const $ = id => document.getElementById(id);
    const out = $("alResult"), on = $("alOn"), pass = $("alPass"), badge = $("alPassBadge");

    // Fill the link with this page's address the first time.
    if (!$("alLink").value) $("alLink").placeholder = location.origin;

    function form() {
        return {
            on: on.checked,
            host: $("alHost").value.trim(),
            port: parseInt($("alPort").value, 10) || 0,
            security: $("alSecurity").value,
            username: $("alUser").value.trim(),
            password: pass.value,
            from: $("alFrom").value.trim(),
            to: $("alTo").value.trim(),
            link: $("alLink").value.trim() || location.origin,
        };
    }

    function show(text, ok) {
        out.className = "st-ha-result" + (ok === undefined ? "" : ok ? " ok" : " bad");
        out.textContent = text || "";
    }

    async function post(url, body) {
        const r = await fetch(url, {
            method: "POST",
            headers: { "Content-Type": "application/json", Accept: "application/json" },
            body: JSON.stringify(body),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) {
            const err = new Error(d.error || T.failed);
            err.demo = d.demo;
            throw err;
        }
        return d;
    }

    const saved = $("stSaved");
    let flashTimer = null;
    function flash() {
        if (!saved) return;
        saved.classList.add("show");
        clearTimeout(flashTimer);
        flashTimer = setTimeout(() => saved.classList.remove("show"), 1800);
    }

    function setBadge(set) {
        badge.className = "badge " + (set ? "bg-success" : "bg-secondary");
        badge.textContent = set ? T.saved : T.none;
        pass.placeholder = set ? "••••••••" : "";
    }

    let saving = Promise.resolve();
    function save() {
        const body = form();
        if (!$("alLink").value.trim()) $("alLink").value = body.link;
        saving = saving.then(async () => {
            try {
                const d = await post("/settings/alerts/email", body);
                if (body.password) pass.value = "";
                setBadge(d.passwordSet);
                show("");
                flash();
            } catch (e) {
                // Turning email on with something missing: leave it off.
                if (body.on) on.checked = false;
                if (!e.demo) show(e.message, false);
            }
        });
        return saving;
    }

    card.addEventListener("change", e => { if (e.target.matches("[data-al]")) save(); });
    card.addEventListener("keydown", e => {
        if (e.key === "Enter" && e.target.matches("input[data-al]")) { e.preventDefault(); e.target.blur(); }
    });

    // Port follows the security choice while it's still a default.
    $("alSecurity").addEventListener("change", () => {
        const p = $("alPort");
        const defaults = { starttls: "587", tls: "465", none: "25" };
        if (Object.values(defaults).includes(p.value) || !p.value) p.value = defaults[$("alSecurity").value];
    });

    const test = $("alTest");
    test.addEventListener("click", async () => {
        await saving;
        test.disabled = true;
        show(T.sending);
        try {
            const d = await post("/settings/alerts/email/test", form());
            show(d.message, true);
        } catch (e) {
            if (!e.demo) show(e.message, false);
            else show("");
        } finally {
            test.disabled = false;
        }
    });
})();
