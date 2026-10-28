// Settings > Alerts (settings.html, handlers/alert_settings.go): the Email
// card saves when a field changes (POST /settings/alerts/email) and Send
// test email sends with what's typed, saved or not. A blank password field
// keeps the saved password. Provider (not saved) fills in a provider's
// server, port and security and shows its tip.
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

    // show puts a result under Send test email; a hint (plain advice) goes
    // first, the mail server's own words under it.
    function show(text, ok, hint) {
        out.className = "st-ha-result" + (ok === undefined ? "" : ok ? " ok" : " bad");
        out.replaceChildren();
        if (hint) {
            out.append(hint, document.createElement("br"));
            const detail = document.createElement("span");
            detail.className = "al-detail";
            detail.textContent = text || "";
            out.append(detail);
        } else {
            out.textContent = text || "";
        }
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
            err.hint = d.hint;
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

    // Providers: server, port, security, and where app passwords are made.
    // hosts are other servers of the same provider, for picking it on load.
    const PROVIDERS = {
        gmail: { host: "smtp.gmail.com", port: 587, security: "starttls", link: "https://myaccount.google.com/apppasswords" },
        outlook: { host: "smtp-mail.outlook.com", port: 587, security: "starttls", hosts: ["smtp.office365.com"], warn: true },
        yahoo: { host: "smtp.mail.yahoo.com", port: 465, security: "tls", link: "https://login.yahoo.com/account/security" },
        aol: { host: "smtp.aol.com", port: 465, security: "tls", link: "https://login.aol.com/account/security" },
        icloud: { host: "smtp.mail.me.com", port: 587, security: "starttls", link: "https://account.apple.com" },
        fastmail: { host: "smtp.fastmail.com", port: 465, security: "tls" },
        zoho: { host: "smtp.zoho.com", port: 465, security: "tls", hosts: ["smtp.zoho.eu", "smtp.zoho.in", "smtp.zoho.com.au", "smtp.zoho.jp", "smtppro.zoho.com", "smtppro.zoho.eu"] },
    };
    // Email-sending services' servers (Brevo, SMTP2GO, Mailgun, SES, Postmark, Resend, SendGrid).
    const SERVICES = [/(^|\.)brevo\.com$/, /(^|\.)sendinblue\.com$/, /(^|\.)smtp2go\.com$/, /(^|\.)mailgun\.org$/,
        /^email-smtp\..*\.amazonaws\.com$/, /(^|\.)postmarkapp\.com$/, /(^|\.)resend\.com$/, /(^|\.)sendgrid\.net$/];

    const provider = $("alProvider"), tip = $("alTip");
    function providerFor(host) {
        host = host.trim().toLowerCase();
        if (!host) return "";
        for (const [k, p] of Object.entries(PROVIDERS)) {
            if (p.host === host || (p.hosts || []).includes(host)) return k;
        }
        return SERVICES.some(re => re.test(host)) ? "service" : "other";
    }
    function showTip() {
        const k = provider.value, p = PROVIDERS[k] || {};
        const text = (T.tips || {})[k];
        tip.hidden = !text;
        if (!text) return;
        tip.classList.toggle("warn", !!p.warn);
        tip.querySelector("i").className = "fa-solid " + (p.warn ? "fa-triangle-exclamation" : "fa-circle-info");
        const span = tip.querySelector("span");
        span.textContent = text + " ";
        if (p.link) {
            const a = document.createElement("a");
            a.href = p.link;
            a.target = "_blank";
            a.rel = "noopener";
            a.textContent = T.tipLink;
            span.append(a);
        }
    }
    provider.value = providerFor($("alHost").value);
    showTip();
    provider.addEventListener("change", () => {
        const p = PROVIDERS[provider.value];
        showTip();
        if (!p) {
            // Sending service or other: the server comes from the user.
            if (providerFor($("alHost").value) !== "other" && providerFor($("alHost").value) !== provider.value) $("alHost").value = "";
            $("alHost").focus();
            save();
            return;
        }
        $("alHost").value = p.host;
        $("alPort").value = p.port;
        $("alSecurity").value = p.security;
        save();
    });
    $("alHost").addEventListener("change", () => {
        const k = providerFor($("alHost").value);
        // A typed server keeps "Sending service" unless it's a known provider's.
        if (!(k === "other" && provider.value === "service")) provider.value = k;
        showTip();
    });

    // The From address is usually the sign-in address.
    $("alUser").addEventListener("change", () => {
        const u = $("alUser").value.trim();
        if (!$("alFrom").value.trim() && u.includes("@")) $("alFrom").value = u;
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
            if (!e.demo) show(e.message, false, e.hint);
            else show("");
        } finally {
            test.disabled = false;
        }
    });
})();
