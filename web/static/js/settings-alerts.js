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

    // Notification methods besides email (handlers/alert_methods.go): each saves
    // when a field changes (POST /settings/alerts/methods/<name>); Send test uses
    // what's typed. MD is what the server knows (secrets only as saved or not),
    // replaced after each save.
    const MT = T.meth || {};
    let MD = JSON.parse(($("alMethData") || {}).textContent || "{}");
    MD.ready = MD.ready || {};
    const secs = {};
    card.querySelectorAll(".ax-meth").forEach(s => { secs[s.dataset.m] = s; });
    const inSec = (m, sel) => secs[m].querySelector(sel);
    const sw = m => inSec(m, "[data-on]");
    const field = (m, f) => { const el = inSec(m, `[data-f="${f}"]`); return el ? el.value.trim() : ""; };
    const fill = (s, vals) => Object.entries(vals).reduce((t, [k, v]) => t.split(`{${k}}`).join(v), s);
    // Regular / Wake me up (common/alert-wake.html): each choice is stored as
    // the app's own priority in the method's hidden [data-pri] input.
    const WAKE = MT.wake || {};
    const CHOICES = {
        pushover: [["regular", "0"], ["wake", "1"], ["repeat", "2"]],
        ha: [["regular", "normal"], ["wake", "critical"]],
        ntfy: [["regular", "4"], ["wake", "5"]],
    };
    const CHOICE_ICON = { regular: "fa-bell", wake: "fa-moon", repeat: "fa-repeat" };
    const CHOICE_ST = { regular: WAKE.stRegular, wake: WAKE.stWake, repeat: WAKE.stRepeat };
    const choiceOf = m => {
        const v = inSec(m, "[data-pri]").value, c = CHOICES[m].find(x => x[1] === v);
        return c ? c[0] : "regular";
    };
    // md: escaped text with **bold** (the al_wake_* keys).
    const md = s => esc(s || "").replace(/\*\*(.+?)\*\*/g, "<b>$1</b>");
    // haPhones: every phone the list shows (saved ones, then any a refresh found).
    let haPhones = (MD.haPhones || []).map(p => ({ ...p, on: true }));
    const methodSetUp = {
        email: () => !!$("alHost").value.trim(),
        pushover: () => !!field("pushover", "user") || MD.pushoverTokenSet,
        ha: () => haPhones.some(p => p.on),
        ntfy: () => !!MD.ntfyTopic,
        discord: () => MD.discordURLSet,
        webhook: () => MD.webhookURLSet,
    };
    // methodOn: switched on (and, for the other methods, complete when last saved).
    const methodOn = m => m === "email" ? on.checked : sw(m).checked && !!MD.ready[m];
    function methodStatus(m) {
        if (m === "ha" && !MD.haConnected) return MT.haNotConnected;
        if (m === "email") return on.checked ? T.sendingTo.replace("{to}", toLine()) : methodSetUp.email() ? T.mailOff : MT.notSetUp;
        if (!sw(m).checked) return methodSetUp[m]() ? T.mailOff : MT.notSetUp;
        if (!MD.ready[m]) return MT.incomplete;
        const choice = CHOICES[m] ? CHOICE_ST[choiceOf(m)] : "";
        switch (m) {
            case "pushover": {
                const d = field("pushover", "devices");
                return d ? fill(MT.sendingPo, { devices: d, choice }) : fill(MT.sendingPoAll, { choice });
            }
            case "ha": {
                const chosen = haPhones.filter(p => p.on);
                return chosen.length === 1 ? fill(MT.sendingHa, { phones: chosen[0].name, choice })
                    : fill(MT.sendingHaN, { n: chosen.length, choice });
            }
            case "ntfy": return fill(MT.sendingNtfy, { topic: MD.ntfyTopic, choice });
            case "discord": return MT.sendingDiscord;
            case "webhook": return fill(MT.sendingWebhook, { host: MD.webhookHost });
        }
        return "";
    }
    function drawMethods() {
        for (const m of Object.keys(secs)) {
            const st = inSec(m, "[data-st]") || $("alMailSt");
            st.textContent = methodStatus(m);
            secs[m].classList.toggle("off-row", !(m === "email" ? on.checked : sw(m).checked));
        }
        // Saved / Not set badges, and Remove on the optional secrets.
        card.querySelectorAll("[data-badge]").forEach(b => {
            const set = !!MD[b.dataset.badge], optional = !!card.querySelector(`[data-remove="${b.dataset.badge}"]`);
            b.className = "badge " + (set ? "bg-success" : "bg-secondary");
            b.textContent = set ? T.saved : T.none;
            b.hidden = optional && !set;
            const input = card.querySelector(`[data-secret="${b.dataset.badge}"]`);
            if (input) input.placeholder = set ? "••••••••" : "";
        });
        card.querySelectorAll("[data-remove]").forEach(r => { r.hidden = !MD[r.dataset.remove]; });
        const ha = secs.ha;
        ha.querySelector('[data-ha="yes"]').hidden = !MD.haConnected;
        ha.querySelector('[data-ha="no"]').hidden = MD.haConnected;
        sw("ha").disabled = !MD.haConnected && !sw("ha").checked;
    }

    // Home Assistant: the integrations link, and the phone list.
    const intLink = `<a href="#integrations" data-goto="integrations">${esc(MT.intLink)}</a>`;
    secs.ha.querySelector("[data-ha-note]").innerHTML = esc(MT.haNote).replace("{link}", intLink).replace("{url}", esc(MD.haURL || ""));
    secs.ha.querySelector("[data-ha-connect]").innerHTML = esc(MT.haConnect).replace("{link}", intLink);
    function drawPhones(msg) {
        const list = secs.ha.querySelector("[data-list]");
        list.replaceChildren();
        for (const p of haPhones) {
            const label = document.createElement("label");
            const box = document.createElement("input");
            box.type = "checkbox";
            box.className = "form-check-input m-0";
            box.checked = p.on;
            box.addEventListener("change", () => { p.on = box.checked; saveMethod("ha"); });
            const code = document.createElement("code");
            code.textContent = p.id;
            label.append(box, " " + p.name + " ", code);
            list.append(label);
        }
        if (!haPhones.length || msg) {
            const small = document.createElement("small");
            small.textContent = msg || MT.haNoPhones;
            list.append(small);
        }
    }
    drawPhones();
    secs.ha.querySelector("[data-refresh]").addEventListener("click", async () => {
        const out = inSec("ha", "[data-out]");
        try {
            const r = await fetch("/settings/alerts/ha-phones", { headers: { Accept: "application/json" } });
            const d = await r.json().catch(() => ({}));
            if (!r.ok) throw Object.assign(new Error(d.error || T.failed), { demo: d.demo });
            const found = d.phones || [];
            const was = new Map(haPhones.map(p => [p.id, p]));
            haPhones = found.map(p => ({ ...p, on: was.has(p.id) && was.get(p.id).on }));
            for (const p of was.values()) if (p.on && !found.some(f => f.id === p.id)) haPhones.push(p);
            out.className = "st-ha-result";
            out.textContent = "";
            drawPhones(found.length ? (found.length === 1 ? MT.haFoundOne : MT.haFound.replace("{n}", found.length)) : MT.haFoundNone);
            if (haPhones.some(p => p.on)) saveMethod("ha"); // keeps the names fresh
            drawMethods();
        } catch (e) {
            if (e.demo) return;
            out.className = "st-ha-result bad";
            out.textContent = e.message;
        }
    });

    // ntfy: Generate a hard-to-guess topic.
    inSec("ntfy", "[data-gen]").addEventListener("click", () => {
        const abc = "abcdefghjkmnpqrstuvwxyz23456789", r = new Uint32Array(10);
        crypto.getRandomValues(r);
        inSec("ntfy", '[data-f="topic"]').value = "budminder-" + Array.from(r, x => abc[x % abc.length]).join("");
        saveMethod("ntfy");
    });

    // The webhook's example request.
    const hookEx = $("alHookExample");
    if (hookEx) hookEx.textContent = JSON.stringify({
        event: "start", title: "Temperature high in 3x3 Tent", text: "Temperature is 87 °F, at or above the High limit (86 °F). Since 1:18 PM.",
        kind: "stage", zone: "3x3 Tent", measure: "temp", value: 87, limit: 86, unit: "°F",
        started_at: "2026-11-08T21:18:00Z", url: location.origin + "/sensors#alert=42",
    }, null, 2);

    function methodBody(m, remove) {
        const b = { on: sw(m).checked };
        switch (m) {
            case "pushover":
                Object.assign(b, { user: field(m, "user"), token: inSec(m, '[data-f="token"]').value, devices: field(m, "devices"), priority: inSec(m, "[data-pri]").value });
                break;
            case "ha": {
                const chosen = haPhones.filter(p => p.on);
                Object.assign(b, { phones: chosen.map(p => p.id), names: chosen.map(p => p.name), priority: inSec(m, "[data-pri]").value });
                break;
            }
            case "ntfy":
                Object.assign(b, { server: field(m, "server"), topic: field(m, "topic"), token: inSec(m, '[data-f="token"]').value,
                    priority: inSec(m, "[data-pri]").value, remove_token: remove === "ntfyTokenSet" });
                break;
            case "discord":
                b.url = inSec(m, '[data-f="url"]').value;
                break;
            case "webhook":
                Object.assign(b, { url: inSec(m, '[data-f="url"]').value, auth: inSec(m, '[data-f="auth"]').value, remove_auth: remove === "webhookAuthSet" });
                break;
        }
        return b;
    }
    function methodOut(m, text, ok) {
        const out = inSec(m, "[data-out]");
        out.className = "st-ha-result" + (ok === undefined ? "" : ok ? " ok" : " bad");
        out.textContent = text || "";
    }
    function openMethod(m) {
        secs[m].classList.add("open");
        inSec(m, ".ax-meth-t").setAttribute("aria-expanded", "true");
    }
    function saveMethod(m, remove) {
        const body = methodBody(m, remove);
        if (!$("alLink").value.trim()) { $("alLink").value = location.origin; saveLink(); }
        saving = saving.then(async () => {
            try {
                const d = await post(`/settings/alerts/methods/${m}`, body);
                hideRefuse();
                MD = d.methods || MD;
                MD.ready = MD.ready || {};
                secs[m].querySelectorAll("[data-secret]").forEach(i => { i.value = ""; });
                methodOut(m, "");
                flash();
            } catch (e) {
                // Turning a method on with something missing: leave it off.
                if (body.on) sw(m).checked = false;
                if (e.code === "nothing_set") {
                    $("alRefuseText").innerHTML = fillLink(T.refuse);
                    $("alRefuse").hidden = false;
                } else if (!e.demo) {
                    openMethod(m);
                    methodOut(m, e.message, false);
                }
            }
            drawState();
        });
        return saving;
    }
    for (const m of Object.keys(secs)) {
        if (m === "email") continue;
        secs[m].addEventListener("change", e => { if (e.target.matches("[data-on], [data-f]")) saveMethod(m); });
        secs[m].addEventListener("keydown", e => {
            if (e.key === "Enter" && e.target.matches("input[data-f]")) { e.preventDefault(); e.target.blur(); }
        });
        secs[m].querySelectorAll("[data-remove]").forEach(b => b.addEventListener("click", () => saveMethod(m, b.dataset.remove)));
        const test = inSec(m, "[data-test]"), delay = inSec(m, "[data-delay]");
        if (!test) continue;
        // Send in 30 seconds: the server waits, the page counts down meanwhile.
        const sendTest = async wait => {
            await saving;
            test.disabled = true;
            if (delay) delay.disabled = true;
            let left = wait, timer = 0;
            if (wait) {
                methodOut(m, fill(WAKE.countdown, { n: left }), true);
                timer = setInterval(() => {
                    left -= 1;
                    methodOut(m, left > 0 ? fill(WAKE.countdown, { n: left }) : T.sending, left > 0 || undefined);
                    if (left <= 0) clearInterval(timer);
                }, 1000);
            } else {
                methodOut(m, T.sending);
            }
            try {
                const d = await post(`/settings/alerts/methods/${m}/test`, { ...methodBody(m), delay: wait });
                methodOut(m, wait ? WAKE.sent : d.message, true);
            } catch (e) {
                methodOut(m, e.demo ? "" : e.message, e.demo ? undefined : false);
            } finally {
                clearInterval(timer);
                test.disabled = false;
                if (delay) delay.disabled = false;
            }
        };
        test.addEventListener("click", () => sendTest(0));
        if (delay) delay.addEventListener("click", () => sendTest(30));
    }

    // The Regular / Wake me up choice and the phone setup steps.
    const phoneTab = {};
    const iPhone = /iPhone|iPad|iPod/.test(navigator.userAgent);
    const ol = (...keys) => `<ol>${keys.map(k => `<li>${md(WAKE[k])}</li>`).join("")}</ol>`;
    const SETUP = {
        pushover: {
            android: () => ol("poAnd1", "poAnd2") + `<p class="mt-1">${md(WAKE.poAndNote)}</p>`,
            iphone: c => ol("poIos1", c === "repeat" ? "poIos2Repeat" : "poIos2"),
        },
        ha: {
            android: () => `<p class="ax-none">${md(WAKE.haAnd)}</p>`,
            iphone: () => ol("haIos1", "haIos2"),
        },
        ntfy: {
            android: () => `<h5>${md(WAKE.ntfySamsung)}</h5>` + ol("ntfySam1", "ntfySam2") + `<p class="mt-1">${md(WAKE.ntfySamNote)}</p>` +
                `<h5>${md(WAKE.ntfyOther)}</h5>` + ol("ntfyOth1", "ntfyOth2"),
            iphone: () => `<p class="ax-none">${md(WAKE.ntfyIos)}</p>`,
        },
    };
    function drawWake(m) {
        const now = choiceOf(m), wake = now !== "regular";
        const box = inSec(m, "[data-choice]");
        box.innerHTML = `<span class="ax-choice-t" id="alWakeT-${m}">${esc(WAKE.label)}</span><small>${esc(WAKE.clears)}</small>` +
            `<div class="fm-seg" role="radiogroup" aria-labelledby="alWakeT-${m}">` + CHOICES[m].map(([k]) =>
                `<label class="${k === now ? "on" : ""}"><input type="radio" name="alWake-${m}" value="${k}"${k === now ? " checked" : ""}>` +
                `<i class="fa-solid ${CHOICE_ICON[k]}"></i><span><b>${esc(WAKE[k])}</b><small>${esc(WAKE[k + "Note"])}</small></span></label>`).join("") + `</div>`;
        const setup = inSec(m, "[data-setup]"), p = phoneTab[m] || (iPhone ? "iphone" : "android");
        setup.hidden = !wake;
        setup.innerHTML = !wake ? "" : `<div class="ax-setup-h"><b>${esc(WAKE.setupTitle)}</b><span class="ax-tabs" role="tablist">` +
            ["android", "iphone"].map(t => `<button type="button" role="tab" aria-selected="${t === p}" data-phone-tab="${t}" class="${t === p ? "on" : ""}">` +
                `<i class="fa-brands fa-${t === "android" ? "android" : "apple"} me-1"></i>${esc(WAKE[t])}</button>`).join("") +
            `</span></div><div role="tabpanel">${SETUP[m][p](now)}</div>`;
        inSec(m, "[data-wake-hint]").hidden = !wake;
        inSec(m, "[data-delay]").hidden = !wake;
    }
    for (const m of Object.keys(CHOICES)) {
        if (!secs[m]) continue;
        drawWake(m);
        inSec(m, "[data-choice]").addEventListener("change", e => {
            const c = CHOICES[m].find(x => x[0] === e.target.value);
            if (!c) return;
            inSec(m, "[data-pri]").value = c[1];
            drawWake(m);
            drawMethods();
            saveMethod(m);
        });
        inSec(m, "[data-setup]").addEventListener("click", e => {
            const t = e.target.closest("[data-phone-tab]");
            if (!t) return;
            phoneTab[m] = t.dataset.phoneTab;
            drawWake(m);
            inSec(m, `[data-phone-tab="${phoneTab[m]}"]`).focus();
        });
    }

    // Text with **bold** (the tip above the methods).
    card.querySelectorAll("[data-md]").forEach(el => { el.innerHTML = md(el.dataset.md); });

    // A show/hide button on every hidden field (API token, passwords, URLs).
    card.querySelectorAll('input[type="password"]').forEach(input => {
        const wrap = document.createElement("span");
        wrap.className = "fm-pw ax-pw";
        input.replaceWith(wrap);
        const b = document.createElement("button");
        b.type = "button";
        b.title = MT.show;
        b.setAttribute("aria-label", MT.show);
        b.innerHTML = '<i class="fa-regular fa-eye"></i>';
        b.addEventListener("click", () => {
            const show = input.type === "password";
            input.type = show ? "text" : "password";
            b.title = show ? MT.hide : MT.show;
            b.setAttribute("aria-label", b.title);
            b.querySelector("i").className = show ? "fa-regular fa-eye-slash" : "fa-regular fa-eye";
        });
        wrap.append(input, b);
    });

    // BudMinder address: used by every method's links.
    function saveLink() {
        const res = $("alLinkResult");
        saving = saving.then(async () => {
            try {
                await post("/settings/alerts/link", { link: $("alLink").value.trim() });
                res.className = "st-ha-result";
                res.textContent = "";
                flash();
            } catch (e) {
                res.className = "st-ha-result" + (e.demo ? "" : " bad");
                res.textContent = e.demo ? "" : e.message;
            }
        });
        return saving;
    }
    $("alLink").addEventListener("change", saveLink);
    $("alLink").addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); e.target.blur(); } });

    // What sends an alert notification: whether notifications are on, and what's missing.
    const toLine = () => $("alTo").value.trim();
    const cardLink = `<a href="#grow" data-goto="grow" data-goto-card="climateCard">${esc(T.climateCard)}</a>`;
    const fillLink = s => esc(s).replace("{link}", cardLink);
    function anySet() {
        const c = window.bmClimate;
        return (c ? c.anyLimit() || c.anyLight() : true) || rules.some(r => r.enabled);
    }
    const BY = { email: () => MT.byEmail.replace("{to}", toLine()), pushover: () => MT.byPushover, ha: () => MT.byHa,
        ntfy: () => MT.byNtfy, discord: () => MT.byDiscord, webhook: () => MT.byWebhook };
    function listWords(items) {
        try {
            return new Intl.ListFormat(document.documentElement.lang || "en", { type: "conjunction" }).format(items);
        } catch {
            return items.join(", ");
        }
    }
    function drawState() {
        const ons = Object.keys(secs).filter(methodOn);
        const el = $("alState"), any = anySet(), ok = ons.length > 0 && any;
        const setUp = Object.keys(secs).some(m => methodSetUp[m]());
        const warn = '<i class="fa-solid fa-triangle-exclamation"></i>';
        const jump = t => `<a href="#alMethods" data-al-jump="alMethods">${esc(t)}</a>`;
        el.classList.toggle("warn", !ok);
        el.classList.toggle("ok", ok);
        el.innerHTML = ok
            ? `<i class="fa-solid fa-circle-check"></i><span>${esc(T.stOn.replace("{methods}", listWords(ons.map(m => BY[m]()))))}</span>`
            : ons.length ? `${warn}<span>${fillLink(T.stNoLimit)}</span>`
            : !any ? `${warn}<span>${fillLink(T.stOffNoLimit)}</span>`
            : setUp ? `${warn}<span>${esc(T.stPaused)} ${jump(T.stTurnOn)}</span>`
            : `${warn}<span>${esc(T.stOff)} ${jump(T.stSetUp)}</span>`;
        drawMethods();
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
    const TYPES = ["temp", "rh", "vpd", "rule", "light", "offline", "integration"];
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
            li.className = (ev.active ? "" : "ended") + (ev.chart ? " al-ev-go" : "");
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
