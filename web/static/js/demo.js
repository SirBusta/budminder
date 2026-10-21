/**
 * Demo mode, page side (the server side is handlers/demo.go, and it alone
 * decides what is allowed; this only makes the demo pleasant to use).
 *
 * - Says why when the server refuses something ("Not available in the demo").
 * - Notes can be typed (up to 500 characters); other text boxes stay as
 *   they are. Search and filter boxes still work.
 * - The strain lookups type "Grape OG" by themselves.
 * - Uploads and exports are turned off.
 */
(function () {
    const textEl = document.getElementById("bmDemoText");
    const T = textEl ? JSON.parse(textEl.textContent || "{}") : {};
    window.BM_DEMO = {};

    // The Plants list opens "Oldest first", so the demo's two showcase plants
    // (Permanent Marker #1 and #2) are at the top. A visitor's own choice wins.
    try { if (!localStorage.getItem("isley.plants.sort")) localStorage.setItem("isley.plants.sort", "oldest"); } catch (e) { /* private mode */ }
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
    // Notes (text areas) can be typed in, up to the server's limit; they stay
    // in this visitor's private copy. Other text boxes (names and the like)
    // stay as they are. Search and filter boxes still work.
    const MAX_NOTE = 500;
    const TYPES = ["text", "url", "email", "tel", "password", ""];
    const free = el => el.type === "search" || el.classList.contains("isley-ac-input") ||
        el.id === "spFindQ" || el.id === "srLookup" || el.closest("[data-demo-free]");
    document.addEventListener("focusin", e => {
        const el = e.target;
        if (el.tagName === "TEXTAREA") {
            el.maxLength = MAX_NOTE;
            if (el.value.length > MAX_NOTE) say(T.noteTooLong || "Notes in the demo are limited to 500 characters.");
            return;
        }
        if (el.tagName !== "INPUT" || !TYPES.includes(el.type) || free(el)) return;
        el.readOnly = true;
        say(T.noTyping || "Typing is turned off in the demo.");
    });

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
