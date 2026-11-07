// Settings > Alerts (settings.html, handlers/alert_settings.go): the line on
// What sends an alert notification, the Email method, Sensor alerts and Recent
// alerts. Email saves when a field changes (POST /settings/alerts/email) and
// Send test email sends with what's typed, saved or not. A blank password field
// keeps the saved password. Provider (not saved) fills in a provider's server,
// port and security and shows its tip. Zone status is in settings-climate.js.
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
            err.code = d.code;
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

    // Sensor alerts card: a list, each with a switch, and a window to add or edit.
    const ruleData = JSON.parse($("alRuleData").textContent || "{}");
    const esc = v => String(v).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
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
        return (r.side === "low" ? T.ruleLineLow : T.ruleLineHigh).replace("{limit}", amount(r.limit, s ? s.unit : ""));
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
                    drawState();
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
                    drawState();
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
            enabled: editing ? editing.enabled : true,
        };
        try {
            const saved = editing ? await ruleSend("PUT", `/settings/alerts/rules/${editing.id}`, body)
                : await ruleSend("POST", "/settings/alerts/rules", body);
            if (editing) Object.assign(editing, saved);
            else rules.push(saved);
            drawRules();
            hideRefuse();
            drawState();
            ruleDlg.hide();
            ruleOut.textContent = "";
            flash();
        } catch (e) {
            ruleErr.textContent = e.demo ? "" : e.message;
            if (e.demo) ruleDlg.hide();
        }
    });
    drawRules();

    // What sends an alert notification: whether notifications are on, and what's missing.
    const setUp = data.dataset.setUp === "true";
    const toLine = () => $("alTo").value.trim();
    const cardLink = `<a href="#grow" data-goto="grow" data-goto-card="climateCard">${esc(T.climateCard)}</a>`;
    const fillLink = s => esc(s).replace("{link}", cardLink);
    function anySet() {
        const c = window.bmClimate;
        return (c ? c.anyLimit() || c.anyLight() : true) || rules.some(r => r.enabled);
    }
    function drawState() {
        const el = $("alState"), any = anySet(), ok = on.checked && any;
        const warn = '<i class="fa-solid fa-triangle-exclamation"></i>';
        const jump = t => `<a href="#alMethods" data-al-jump="alMethods">${esc(t)}</a>`;
        el.classList.toggle("warn", !ok);
        el.classList.toggle("ok", ok);
        el.innerHTML = ok
            ? `<i class="fa-solid fa-circle-check"></i><span>${esc(T.stOn.replace("{to}", toLine()))}</span>`
            : on.checked ? `${warn}<span>${fillLink(T.stNoLimit)}</span>`
            : !any ? `${warn}<span>${fillLink(T.stOffNoLimit)}</span>`
            : setUp ? `${warn}<span>${esc(T.stPaused)} ${jump(T.stTurnOn)}</span>`
            : `${warn}<span>${esc(T.stOff)} ${jump(T.stSetUp)}</span>`;
        $("alMailSt").textContent = on.checked ? T.sendingTo.replace("{to}", toLine()) : T.mailOff;
        const title = $("clAlTitle");
        if (title) title.textContent = ok ? T.boxOn : T.boxOff;
    }
    function hideRefuse() { $("alRefuse").hidden = true; }
    document.addEventListener("bm:climate-change", () => { hideRefuse(); drawState(); });
    $("alWhat").closest("section").addEventListener("click", e => {
        const a = e.target.closest("[data-al-jump]");
        if (!a) return;
        e.preventDefault();
        $(a.dataset.alJump).scrollIntoView({ behavior: "smooth", block: "start" });
    });
    drawState();

    // Notification methods: click a row to open or close its fields.
    card.addEventListener("click", e => {
        const b = e.target.closest(".ax-meth-t");
        if (b) b.setAttribute("aria-expanded", b.closest(".ax-meth").classList.toggle("open"));
    });

    // Recent alerts: two tabs, and Zone and Type filters when the list has two
    // or more of either. Clear and Restore apply to ended alerts only. A row
    // opens the Sensors page at that alert (sensors-page.js, #alert=<id>).
    const evs = JSON.parse($("alEvData").textContent || "{}");
    evs.recent = evs.recent || [];
    evs.cleared = evs.cleared || [];
    let evTab = "recent", zoneF = "", typeF = "";
    const evOut = $("alEvResult");
    const TYPES = ["temp", "rh", "vpd", "rule", "light", "offline"];
    const shown = list => list.filter(ev => (!zoneF || ev.zone === zoneF) && (!typeF || ev.type === typeF));
    function drawFilters() {
        const all = [...evs.recent, ...evs.cleared];
        const zones = [...new Set(all.map(ev => ev.zone).filter(Boolean))].sort((a, b) => a.localeCompare(b));
        const types = TYPES.filter(k => all.some(ev => ev.type === k));
        if (zones.length < 2 || !zones.includes(zoneF)) zoneF = "";
        if (types.length < 2 || !types.includes(typeF)) typeF = "";
        $("alZoneW").hidden = zones.length < 2;
        $("alTypeW").hidden = types.length < 2;
        $("alZone").innerHTML = `<option value="">${esc(T.allZones)}</option>` + zones.map(z => `<option value="${esc(z)}"${z === zoneF ? " selected" : ""}>${esc(z)}</option>`).join("");
        $("alType").innerHTML = `<option value="">${esc(T.allTypes)}</option>` + types.map(k => `<option value="${k}"${k === typeF ? " selected" : ""}>${esc(T.types[k])}</option>`).join("");
    }
    function drawEvents() {
        drawFilters();
        const ul = $("alEv"), list = shown(evs[evTab]);
        ul.replaceChildren();
        for (const ev of list) {
            const li = document.createElement("li");
            li.className = "al-ev-" + ev.severity + (ev.active ? "" : " ended") + (ev.chart ? " al-ev-go" : "");
            if (ev.chart) {
                li.dataset.go = ev.id;
                li.tabIndex = 0;
                li.setAttribute("role", "link");
            }
            li.innerHTML = `<div class="al-ev-main"><b>${esc(ev.title)}${ev.value ? `: <span class="al-ev-val">${esc(ev.value)}</span>` : ""}</b>` + (ev.line ? `<span>${esc(ev.line)}</span>` : "") +
                (ev.error ? `<small class="al-detail">${esc(ev.error)}</small>` : "") + `</div>` +
                `<div class="al-ev-meta"><small>${esc(ev.when)}</small><span class="badge ${ev.active ? "bg-danger" : "bg-secondary"}">${esc(ev.state)}</span>` +
                (ev.delivery ? `<span class="badge al-ev-del al-ev-del-${esc(ev.delivery_key)}">${esc(ev.delivery)}</span>` : "") +
                (ev.active ? "" : `<button type="button" class="st-btn light sm" data-ev="${ev.id}"><i class="fa-solid ${evTab === "recent" ? "fa-check" : "fa-rotate-left"} me-1"></i>${esc(evTab === "recent" ? T.evClear : T.evRestore)}</button>`) +
                (ev.chart ? `<span class="al-ev-chart" title="${esc(T.evView)}" aria-hidden="true"><i class="fa-solid fa-chart-line"></i></span>` : "") + `</div>`;
            ul.append(li);
        }
        if (!list.length) {
            const li = document.createElement("li");
            li.className = "ended";
            const filtered = zoneF || typeF;
            li.innerHTML = `<div class="al-ev-main">${esc(evTab === "recent" ? (filtered ? T.evNoMatchRecent : T.evNoRecent) : (filtered ? T.evNoMatchCleared : T.evNoCleared))}</div>`;
            ul.append(li);
        }
        $("alEvRecentN").textContent = shown(evs.recent).length;
        $("alEvClearedN").textContent = shown(evs.cleared).length;
        $("alClearAll").style.display = evTab === "recent" && list.some(e => !e.active) ? "" : "none";
        $("alEvTabs").querySelectorAll("button").forEach(b => b.classList.toggle("on", b.dataset.t === evTab));
    }
    function evFail(e) {
        evOut.className = "st-ha-result";
        evOut.textContent = "";
        if (e.demo) return;
        evOut.className = "st-ha-result bad";
        evOut.textContent = e.message;
    }
    const newestFirst = (a, b) => (a.active === b.active ? b.id - a.id : a.active ? -1 : 1);
    $("alEvTabs").addEventListener("click", e => {
        const b = e.target.closest("button");
        if (b) { evTab = b.dataset.t; drawEvents(); }
    });
    $("alZone").addEventListener("change", e => { zoneF = e.target.value; drawEvents(); });
    $("alType").addEventListener("change", e => { typeF = e.target.value; drawEvents(); });
    const openChart = li => { location.href = "/sensors#alert=" + li.dataset.go; };
    $("alEv").addEventListener("click", async e => {
        const b = e.target.closest("[data-ev]");
        if (!b) {
            const li = e.target.closest("[data-go]");
            if (li) openChart(li);
            return;
        }
        const id = +b.dataset.ev, from = evs[evTab], to = evTab === "recent" ? evs.cleared : evs.recent;
        try {
            await post(`/settings/alerts/events/${id}/${evTab === "recent" ? "clear" : "restore"}`, {});
            const i = from.findIndex(x => x.id === id);
            if (i >= 0) { to.push(from.splice(i, 1)[0]); to.sort(newestFirst); }
            evOut.textContent = "";
            drawEvents();
        } catch (err) {
            evFail(err);
        }
    });
    $("alEv").addEventListener("keydown", e => {
        const li = e.target.closest("[data-go]");
        if (li && e.target === li && e.key === "Enter") openChart(li);
    });
    // Clear All Ended clears what the filters show; with no filter, every ended alert.
    $("alClearAll").addEventListener("click", async () => {
        const gone = shown(evs.recent).filter(x => !x.active);
        try {
            await post("/settings/alerts/clear-ended", zoneF || typeF ? { ids: gone.map(x => x.id) } : {});
            evs.cleared.push(...gone);
            evs.cleared.sort(newestFirst);
            evs.recent = evs.recent.filter(x => !gone.includes(x));
            evOut.textContent = "";
            drawEvents();
        } catch (err) {
            evFail(err);
        }
    });
    drawEvents();

    let saving = Promise.resolve();
    function save() {
        const body = form();
        if (!$("alLink").value.trim()) $("alLink").value = body.link;
        saving = saving.then(async () => {
            try {
                const d = await post("/settings/alerts/email", body);
                hideRefuse();
                if (body.password) pass.value = "";
                setBadge(d.passwordSet);
                show("");
                flash();
            } catch (e) {
                // Turning email on with something missing: leave it off.
                if (body.on) on.checked = false;
                if (e.code === "nothing_set") {
                    $("alRefuseText").innerHTML = fillLink(T.refuse);
                    $("alRefuse").hidden = false;
                } else if (!e.demo) {
                    // The error shows under Send test email, so open the fields.
                    const m = card.querySelector(".ax-meth");
                    m.classList.add("open");
                    m.querySelector(".ax-meth-t").setAttribute("aria-expanded", "true");
                    show(e.message, false);
                }
            }
            drawState();
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
