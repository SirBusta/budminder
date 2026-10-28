// Widgets of the new form look (.fm-*, isley.css): sign in, setup, account,
// Add/Edit plant and strain. Page scripts keep reading the same inputs; these
// only drive the controls around them.
document.addEventListener("DOMContentLoaded", () => {
    // Show/hide a password.
    document.querySelectorAll(".fm-pw button").forEach(b => b.addEventListener("click", () => {
        const input = b.parentElement.querySelector("input");
        const show = input.type === "password";
        input.type = show ? "text" : "password";
        b.querySelector("i").className = show ? "fa-regular fa-eye-slash" : "fa-regular fa-eye";
        if (b.dataset.show && b.dataset.hide) b.setAttribute("aria-label", show ? b.dataset.hide : b.dataset.show);
    }));

    // Live password checks: <ul class="fm-rules" data-rules="pwId" data-confirm="confirmId">.
    document.querySelectorAll(".fm-rules[data-rules]").forEach(ul => {
        const pw = document.getElementById(ul.dataset.rules);
        const cf = document.getElementById(ul.dataset.confirm);
        if (!pw) return;
        const update = () => {
            const len = ul.querySelector("[data-r=len]");
            if (len) len.classList.toggle("ok", pw.value.length >= 8);
            const match = ul.querySelector("[data-r=match]");
            if (match && cf) match.classList.toggle("ok", pw.value.length > 0 && pw.value === cf.value);
            ul.querySelectorAll("li").forEach(li => {
                li.querySelector("i").className = li.classList.contains("ok") ? "fa-solid fa-circle-check" : "fa-regular fa-circle";
            });
        };
        pw.addEventListener("input", update);
        if (cf) cf.addEventListener("input", update);
        update();
    });

    // Segmented choice. With data-for="id" it sets that (hidden) select or
    // checkbox and fires its change event, so page scripts keep working.
    document.querySelectorAll(".fm-seg").forEach(seg => {
        const target = seg.dataset.for ? document.getElementById(seg.dataset.for) : null;
        const paint = () => seg.querySelectorAll("label").forEach(l => l.classList.toggle("on", l.querySelector("input").checked));
        seg.addEventListener("change", () => {
            paint();
            const picked = seg.querySelector("input:checked");
            if (target && picked) {
                if (target.type === "checkbox") target.checked = picked.value === "1";
                else target.value = picked.value;
                target.dispatchEvent(new Event("change", { bubbles: true }));
            }
        });
        // Follow the target when a script sets it (e.g. a restored draft).
        const follow = () => {
            const now = target.type === "checkbox" ? (target.checked ? "1" : "0") : target.value;
            seg.querySelectorAll("input").forEach(i => { i.checked = i.value === now; });
            paint();
        };
        if (target) {
            target.addEventListener("change", follow);
            follow();
        } else {
            paint();
        }
    });

    // Number steppers: the buttons change the input and fire its input event.
    document.querySelectorAll(".fm-step").forEach(st => st.addEventListener("click", e => {
        const b = e.target.closest("button");
        if (!b) return;
        const input = st.querySelector("input");
        input.value = Math.max(0, (parseInt(input.value, 10) || 0) + Number(b.dataset.d));
        input.dispatchEvent(new Event("input", { bubbles: true }));
    }));

    // Write / Preview tabs.
    document.querySelectorAll(".fm-tabs").forEach(tabs => tabs.addEventListener("click", e => {
        const b = e.target.closest("button");
        if (!b) return;
        tabs.querySelectorAll("button").forEach(x => {
            x.classList.toggle("on", x === b);
            x.setAttribute("aria-selected", x === b ? "true" : "false");
        });
        const card = tabs.closest(".fm-card");
        card.querySelectorAll("[data-pane]").forEach(p => { p.hidden = p.dataset.pane !== b.dataset.p; });
    }));

    // "On this page" follows the scroll.
    const jump = document.querySelector(".fm-jump");
    if (jump) {
        const links = [...jump.querySelectorAll("a")];
        const secs = links.map(a => document.querySelector(a.getAttribute("href")));
        const onScroll = () => {
            let cur = 0;
            // At the bottom the last sections can't reach the top: mark the last one shown.
            const atEnd = innerHeight + scrollY >= document.documentElement.scrollHeight - 4;
            secs.forEach((s, i) => { if (s && s.offsetParent !== null && (atEnd || s.getBoundingClientRect().top < 140)) cur = i; });
            links.forEach((a, i) => a.classList.toggle("on", i === cur));
        };
        addEventListener("scroll", onScroll, { passive: true });
        onScroll();
    }

    // "Unsaved changes" on the save bar of an edit form.
    document.querySelectorAll(".fm-save .msg[hidden]").forEach(msg => {
        const form = msg.closest("form");
        if (form) form.addEventListener("input", () => { msg.hidden = false; }, { once: true });
    });
});
