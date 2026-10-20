/**
 * Demo mode, page side (the server side is handlers/demo.go, and it alone
 * decides what is allowed; this only makes the demo pleasant to use).
 *
 * - Says why when the server refuses something ("Not available in the demo").
 * - Text boxes can't be typed in: notes are picked from sample notes, other
 *   text stays as it is. Search and filter boxes still work.
 * - The strain lookups type "Grape OG" by themselves.
 * - Uploads and exports are turned off.
 */
(function () {
    const notesEl = document.getElementById("bmDemoNotes");
    const NOTES = notesEl ? JSON.parse(notesEl.textContent || "[]") : [];
    const textEl = document.getElementById("bmDemoText");
    const T = textEl ? JSON.parse(textEl.textContent || "{}") : {};
    window.BM_DEMO = { notes: NOTES };
    const SCRIPTED = "Grape OG";

    // ---------- Messages ----------
    let toastTimer;
    function say(msg) {
        let el = document.getElementById("bmDemoToast");
        if (!el) {
            el = document.createElement("div");
            el.id = "bmDemoToast";
            el.className = "bm-demo-toast";
            el.setAttribute("role", "status");
            document.body.appendChild(el);
        }
        el.textContent = msg;
        el.classList.add("show");
        clearTimeout(toastTimer);
        toastTimer = setTimeout(() => el.classList.remove("show"), 3500);
    }

    // The server marks its demo refusals with demo: true.
    const fetch0 = window.fetch;
    window.fetch = async function () {
        const resp = await fetch0.apply(this, arguments);
        if (resp.status === 403 || resp.status === 429) {
            resp.clone().json().then(d => { if (d && d.demo) say(d.error || T.notAvailable); }).catch(() => {});
        }
        return resp;
    };

    // ---------- Text boxes ----------
    const TYPES = ["text", "url", "email", "tel", "password", ""];
    const free = el => el.type === "search" || el.classList.contains("isley-ac-input") ||
        el.id === "spFindQ" || el.id === "srLookup" || el.closest("[data-demo-free]");
    const lockable = el => (el.tagName === "TEXTAREA" || (el.tagName === "INPUT" && TYPES.includes(el.type))) && !free(el);

    let picker;
    function closePicker() { if (picker) { picker.remove(); picker = null; } }
    function openPicker(area) {
        closePicker();
        picker = document.createElement("div");
        picker.className = "bm-demo-picker";
        picker.innerHTML = `<div class="h">${T.pickNote || "Pick a note"}</div>`;
        ["", ...NOTES].forEach(n => {
            const b = document.createElement("button");
            b.type = "button";
            b.textContent = n || (T.noNote || "No note");
            b.addEventListener("mousedown", e => {
                e.preventDefault();
                area.value = n;
                area.dispatchEvent(new Event("input", { bubbles: true }));
                area.dispatchEvent(new Event("change", { bubbles: true }));
                closePicker();
            });
            picker.appendChild(b);
        });
        const r = area.getBoundingClientRect();
        picker.style.left = (r.left + window.scrollX) + "px";
        picker.style.top = (r.bottom + window.scrollY + 4) + "px";
        picker.style.width = Math.max(260, r.width) + "px";
        document.body.appendChild(picker);
    }
    document.addEventListener("focusin", e => {
        const el = e.target;
        if (!lockable(el)) return;
        el.readOnly = true;
        if (el.tagName === "TEXTAREA") openPicker(el);
        else say(T.noTyping || "Typing is turned off in the demo.");
    });
    document.addEventListener("focusout", e => { if (e.target.tagName === "TEXTAREA") setTimeout(closePicker, 150); });

    // ---------- Scripted strain lookups ----------
    function typeInto(input, then) {
        if (input.dataset.demoTyped) return;
        input.dataset.demoTyped = "1";
        input.readOnly = true;
        input.value = "";
        let i = 0;
        const step = () => {
            input.value = SCRIPTED.slice(0, ++i);
            input.dispatchEvent(new Event("input", { bubbles: true }));
            if (i < SCRIPTED.length) setTimeout(step, 90);
            else if (then) setTimeout(then, 250);
        };
        setTimeout(step, 300);
    }
    function watch() {
        const sp = document.getElementById("spFindQ");
        if (sp) typeInto(sp, () => { const f = document.getElementById("spFindForm"); if (f) f.requestSubmit(); });
        const sr = document.getElementById("srLookup");
        if (sr && document.activeElement === sr) typeInto(sr);
    }
    new MutationObserver(watch).observe(document.documentElement, { childList: true, subtree: true });
    document.addEventListener("focusin", e => { if (e.target.id === "srLookup") watch(); });

    // ---------- Uploads and exports ----------
    document.addEventListener("click", e => {
        const off = e.target.closest('input[type=file], a[href*="/export/"], [data-export]');
        if (!off) return;
        e.preventDefault();
        e.stopPropagation();
        say(T.notAvailable || "Not available in the demo.");
    }, true);
    document.addEventListener("drop", e => {
        if (!e.dataTransfer || !Array.from(e.dataTransfer.types || []).includes("Files")) return;
        e.preventDefault();
        e.stopPropagation();
        say(T.notAvailable || "Not available in the demo.");
    }, true);
})();
