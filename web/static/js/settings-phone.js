// Settings > Phone & tablet (handlers/phone.go, mockup data/mockups/settings/phone/v1.html):
// the address a phone opens, its QR code, Copy/Change, and the home screen steps.
// Change saves the same setting as Alerts' BudMinder address (POST /settings/alerts/link).
(function () {
    "use strict";
    const sec = document.getElementById("phSec");
    if (!sec) return;
    const $ = id => document.getElementById(id);
    const D = sec.dataset;
    const esc = v => String(v).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
    const md = s => esc(s || "").replace(/\*\*(.+?)\*\*/g, "<b>$1</b>");
    // (not data-src-*: html/template treats a "src" attribute as a URL and escapes it)
    const SRC = { saved: D.fromSaved, browser: D.fromBrowser, computer: D.fromComputer };
    const HINT = $("phEditHint").textContent;

    // What the server worked out; settings-alerts.js reads it for the
    // BudMinder address it fills in when a method is first turned on.
    const now = { address: $("phAddrText").textContent.trim(), source: $("phSrc").dataset.source || "" };
    window.bmPhone = now;

    // Text with **bold**, an icon at {icon}, a Help link at {link}.
    sec.querySelectorAll("[data-md]").forEach(el => {
        let h = md(el.dataset.md);
        if (el.dataset.icon) {
            const label = el.dataset.iconLabel ? ` role="img" aria-label="${esc(el.dataset.iconLabel)}"` : ' aria-hidden="true"';
            h = h.replace("{icon}", `<i class="fa-solid ${el.dataset.icon}"${label}></i>`);
        }
        if (el.dataset.link) h = h.replace("{link}", `<a href="${esc(el.dataset.link)}">${esc(el.dataset.linkText)}</a>`);
        el.innerHTML = h;
    });

    function draw(editing) {
        const a = now.address;
        editing = editing || !a;
        $("phAddr").hidden = !a;
        $("phAddrText").textContent = a;
        $("phSrc").textContent = a ? SRC[now.source] || "" : "";
        $("phSrc").hidden = !a;
        $("phEdit").hidden = !editing;
        $("phCancel").hidden = !a;
        if (!a) $("phQr").innerHTML = `<div class="ph-qr-empty">${esc(D.qrEmpty)}</div>`;
    }
    function hint(text, bad) {
        const h = $("phEditHint");
        h.textContent = text;
        h.classList.toggle("bad", !!bad);
    }

    async function refresh() {
        try {
            const r = await fetch("/settings/phone", { headers: { Accept: "application/json" } });
            if (!r.ok) return;
            const d = await r.json();
            now.address = d.address || "";
            now.source = d.source || "";
            if (d.qr) $("phQr").innerHTML = d.qr;
            draw(false);
        } catch (e) { /* keep what's shown */ }
    }

    $("phCopy").addEventListener("click", async () => {
        const b = $("phCopy"), span = b.querySelector("span"), was = span.textContent;
        try {
            await navigator.clipboard.writeText(now.address);
        } catch (e) {
            // http pages on another device have no clipboard API: select the text instead.
            const range = document.createRange();
            range.selectNodeContents($("phAddrText"));
            const s = getSelection();
            s.removeAllRanges();
            s.addRange(range);
            try { document.execCommand("copy"); } catch (e2) { return; }
        }
        b.querySelector("i").className = "fa-solid fa-check me-1";
        span.textContent = D.copied;
        setTimeout(() => { b.querySelector("i").className = "fa-solid fa-copy me-1"; span.textContent = was; }, 1500);
    });
    $("phChange").addEventListener("click", () => {
        $("phInput").value = now.address;
        hint(HINT);
        draw(true);
        $("phInput").focus();
    });
    $("phCancel").addEventListener("click", () => { hint(HINT); draw(false); });
    async function save() {
        const v = $("phInput").value.trim().replace(/\/+$/, "");
        let host = "";
        try { host = new URL(v).hostname.replace(/^\[|\]$/g, ""); } catch (e) { /* checked below */ }
        if (!/^https?:\/\/\S+$/i.test(v) || !host) { hint(D.errAddr, true); return; }
        if (/^(localhost|.*\.localhost|127(\.\d+){3}|::1|0\.0\.0\.0)$/i.test(host)) { hint(D.errLocal, true); return; }
        try {
            const r = await fetch("/settings/alerts/link", {
                method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" },
                body: JSON.stringify({ link: v }),
            });
            const d = await r.json().catch(() => ({}));
            if (!r.ok) {
                if (!d.demo) hint(d.error || D.errAddr, true);
                return;
            }
            const link = $("alLink");
            if (link) link.value = d.link;
            hint(HINT);
            await refresh();
            const saved = $("stSaved");
            if (saved) { saved.classList.add("show"); setTimeout(() => saved.classList.remove("show"), 1500); }
        } catch (e) {
            hint(D.errAddr, true);
        }
    }
    $("phSave").addEventListener("click", save);
    $("phInput").addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); save(); } });
    // Alerts' BudMinder address is the same setting.
    document.addEventListener("bm:link-change", refresh);

    // Opened on the phone itself: no code to scan, the steps below say what's next.
    const ua = navigator.userAgent;
    const iOS = /iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1);
    const onPhone = iOS || /Android|Mobile/.test(ua) || (navigator.userAgentData && navigator.userAgentData.mobile);
    if (onPhone) {
        $("phOpenCard").hidden = true;
        $("phOnPhone").hidden = false;
    }

    // Home screen steps: Android or iPhone & iPad.
    const tabs = $("phTabs");
    function tab(p) {
        tabs.querySelectorAll("[data-p]").forEach(b => { const on = b.dataset.p === p; b.classList.toggle("on", on); b.setAttribute("aria-selected", on); });
        $("phAndroid").hidden = p !== "android";
        $("phIphone").hidden = p !== "iphone";
    }
    tabs.addEventListener("click", e => { const b = e.target.closest("[data-p]"); if (b) tab(b.dataset.p); });
    if (iOS) tab("iphone");

    draw(false);
})();
