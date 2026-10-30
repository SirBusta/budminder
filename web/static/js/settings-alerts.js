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

    // Climate alerts card: saves when a field changes. Send email with the
    // Email card off or incomplete still lists alerts, with a warning here.
    const climateCard = $("alClimateCard"), mode = $("alMode"), modeWarn = $("alModeWarn");
    const climateOut = $("alClimateResult");
    function showModeWarn() {
        modeWarn.hidden = !(mode.value === "email" && climateCard.dataset.emailReady !== "true");
    }
    showModeWarn();
    const num = id => parseInt($(id).value, 10);
    async function saveClimate() {
        showModeWarn();
        climateOut.textContent = "";
        climateOut.className = "st-ha-result";
        try {
            await post("/settings/alerts/climate", {
                mode: mode.value, safety_readings: num("alSafety"), window_minutes: num("alWindow"), drift_hours: num("alDrift"),
                pause_minutes: num("alPause"), repeat_critical_minutes: num("alRepeatCrit"), repeat_warning_hours: num("alRepeatWarn"),
            });
            flash();
        } catch (e) {
            if (!e.demo) {
                climateOut.className = "st-ha-result bad";
                climateOut.textContent = e.message;
            }
        }
    }
    climateCard.addEventListener("change", e => { if (e.target.matches("[data-alc]")) saveClimate(); });
    climateCard.addEventListener("keydown", e => {
        if (e.key === "Enter" && e.target.matches("input[data-alc]")) { e.preventDefault(); e.target.blur(); }
    });

    // Sensor alerts card: a list, each with a switch, and a window to add or edit.
    const ruleData = JSON.parse($("alRuleData").textContent || "{}");
    const rules = ruleData.rules || [], sensors = ruleData.sensors || [];
    const ruleList = $("alRuleList"), ruleOut = $("alRuleResult"), ruleErr = $("alRuleErr");
    const ruleDlg = new bootstrap.Modal($("alRuleDlg"));
    const sensorOf = id => sensors.find(s => s.id === id);
    const amount = (v, unit) => unit === "%" ? `${v}%` : unit ? `${v} ${unit}` : `${v}`;
    const sensorLabel = s => (s.zone ? `${s.name} (${s.zone})` : s.name) + (s.unit ? ` · ${s.unit}` : "");
    let editing = null;

    async function ruleSend(method, url, body) {
        const r = await fetch(url, { method, headers: { "Content-Type": "application/json" }, body: body ? JSON.stringify(body) : undefined });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) {
            const err = new Error(d.error || d.message || T.failed);
            err.demo = !!d.demo;
            throw err;
        }
        return d;
    }
    function ruleFail(e) {
        ruleOut.className = "st-ha-result";
        ruleOut.textContent = "";
        if (e.demo) return;
        ruleOut.className = "st-ha-result bad";
        ruleOut.textContent = e.message;
    }
    function ruleLine(r) {
        const s = sensorOf(r.sensor_id);
        return (r.side === "low" ? T.ruleLineLow : T.ruleLineHigh)
            .replace("{limit}", amount(r.limit, s ? s.unit : "")).replace("{n}", r.hold_minutes);
    }
    function drawRules() {
        ruleList.replaceChildren();
        $("alRuleEmpty").hidden = rules.length > 0;
        for (const r of rules) {
            const s = sensorOf(r.sensor_id);
            const li = document.createElement("li");
            li.classList.toggle("off", !r.enabled);
            const main = document.createElement("div");
            main.className = "al-rule-main";
            const name = document.createElement("b");
            name.textContent = s ? (s.zone ? `${s.name} (${s.zone})` : s.name) : "";
            const line = document.createElement("span");
            line.textContent = ruleLine(r);
            main.append(name, line);
            const sw = document.createElement("div");
            sw.className = "form-check form-switch m-0";
            const box = document.createElement("input");
            box.type = "checkbox";
            box.className = "form-check-input";
            box.setAttribute("role", "switch");
            box.title = T.ruleOn;
            box.setAttribute("aria-label", T.ruleOn);
            box.checked = r.enabled;
            box.addEventListener("change", async () => {
                try {
                    await ruleSend("PUT", `/settings/alerts/rules/${r.id}`, { ...r, enabled: box.checked });
                    r.enabled = box.checked;
                    li.classList.toggle("off", !r.enabled);
                    ruleOut.textContent = "";
                    flash();
                } catch (e) {
                    box.checked = r.enabled;
                    ruleFail(e);
                }
            });
            sw.append(box);
            const edit = document.createElement("button");
            edit.type = "button";
            edit.className = "al-rule-btn";
            edit.title = T.ruleEdit;
            edit.setAttribute("aria-label", T.ruleEdit);
            edit.innerHTML = '<i class="fa-solid fa-pen"></i>';
            edit.addEventListener("click", () => openRule(r));
            const del = document.createElement("button");
            del.type = "button";
            del.className = "al-rule-btn";
            del.title = T.ruleDelete;
            del.setAttribute("aria-label", T.ruleDelete);
            del.innerHTML = '<i class="fa-solid fa-trash"></i>';
            del.addEventListener("click", async () => {
                if (!await uiMessages.showConfirm(T.ruleConfirm, { danger: true })) return;
                try {
                    await ruleSend("DELETE", `/settings/alerts/rules/${r.id}`);
                    rules.splice(rules.indexOf(r), 1);
                    drawRules();
                    flash();
                } catch (e) {
                    ruleFail(e);
                }
            });
            li.append(main, sw, edit, del);
            ruleList.append(li);
        }
    }
    function ruleUnit() {
        const s = sensorOf(parseInt($("alRuleSensor").value, 10));
        $("alRuleUnit").textContent = s ? s.unit : "";
    }
    function openRule(r) {
        editing = r;
        const pick = $("alRuleSensor");
        pick.replaceChildren(...sensors.map(s => new Option(sensorLabel(s), s.id)));
        if (r) pick.value = r.sensor_id;
        $("alRuleSide").value = r ? r.side : "high";
        $("alRuleLimit").value = r ? r.limit : "";
        $("alRuleHold").value = r ? r.hold_minutes : 10;
        $("alRuleDlgT").textContent = r ? T.ruleEdit : T.ruleAdd;
        ruleErr.textContent = "";
        ruleUnit();
        ruleDlg.show();
    }
    $("alRuleSensor").addEventListener("change", ruleUnit);
    $("alRuleAdd").addEventListener("click", () => {
        if (!sensors.length) {
            ruleOut.className = "st-ha-result bad";
            ruleOut.textContent = T.ruleNoSensors;
            return;
        }
        openRule(null);
    });
    $("alRuleSave").addEventListener("click", async () => {
        const limit = parseFloat($("alRuleLimit").value);
        if (Number.isNaN(limit)) {
            ruleErr.textContent = T.ruleBadValue;
            return;
        }
        const body = {
            sensor_id: parseInt($("alRuleSensor").value, 10), side: $("alRuleSide").value, limit,
            hold_minutes: num("alRuleHold"), enabled: editing ? editing.enabled : true,
        };
        try {
            const saved = editing ? await ruleSend("PUT", `/settings/alerts/rules/${editing.id}`, body)
                : await ruleSend("POST", "/settings/alerts/rules", body);
            if (editing) Object.assign(editing, saved);
            else rules.push(saved);
            drawRules();
            ruleDlg.hide();
            ruleOut.textContent = "";
            flash();
        } catch (e) {
            ruleErr.textContent = e.demo ? "" : e.message;
            if (e.demo) ruleDlg.hide();
        }
    });
    drawRules();

    let saving = Promise.resolve();
    function save() {
        const body = form();
        if (!$("alLink").value.trim()) $("alLink").value = body.link;
        saving = saving.then(async () => {
            try {
                const d = await post("/settings/alerts/email", body);
                climateCard.dataset.emailReady = d.on ? "true" : "false";
                showModeWarn();
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
