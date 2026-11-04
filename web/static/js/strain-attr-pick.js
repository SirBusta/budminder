// Effects, flavors, terpenes and "may help with" on Add strain, Edit strain
// and the import review: each group is a list of names to click (select /
// clear), A to Z in the user's language, plus an Other box for names not in
// the list. Saved names are the English ones (handlers/strain_attr_choices.go),
// so a StrainCompass import lights up its buttons. Names that aren't listed
// show after the list, selected, with a ×. Imported terpene levels and effect
// strengths ride along and are saved as they came.
//
// window.strainAttrPick.collect() returns { effects: [...], flavors: [...], ... }.
(function () {
    "use strict";

    const root = document.getElementById("strainAttrPick");
    if (!root) return;

    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    const same = (a, b) => String(a).toLowerCase() === String(b).toLowerCase();
    const cap = s => (typeof IsleyAutocomplete !== "undefined" && IsleyAutocomplete.capitalizeName)
        ? IsleyAutocomplete.capitalizeName(s) : String(s).trim();

    let groups;
    try { groups = JSON.parse(root.dataset.groups || "[]"); } catch (e) { groups = []; }
    let collator;
    try { collator = new Intl.Collator(root.dataset.lang || undefined, { sensitivity: "base" }); } catch (e) { collator = new Intl.Collator(); }

    const levelText = lv => root.dataset["level" + String(lv).charAt(0).toUpperCase() + String(lv).slice(1).toLowerCase()] || lv;
    const DOTS = { low: 1, medium: 2, high: 3 };

    // Per group: the picked items (with any strength/level), and every item
    // ever picked or imported, so clearing and re-picking keeps its level.
    const state = {};
    groups.forEach(g => {
        g.options = (g.options || []).slice().sort((a, b) => collator.compare(a.label, b.label));
        state[g.group] = { on: (g.items || []).map(t => ({ ...t })), memo: {} };
        state[g.group].on.forEach(t => { state[g.group].memo[t.name.toLowerCase()] = t; });
    });

    function dots(t) {
        const n = t && t.level ? DOTS[String(t.level).toLowerCase()] : 0;
        if (!n) return "";
        return ` <span class="fm-pick-dots" aria-hidden="true">${[1, 2, 3].map(i => `<i class="${n >= i ? "on" : ""}"></i>`).join("")}</span>`;
    }
    function levelTip(t) {
        if (!t || !t.level || !root.dataset.level) return "";
        return ` title="${esc(root.dataset.level.replace("{level}", levelText(t.level)))}"`;
    }

    function groupHTML(g) {
        const S = state[g.group];
        const find = name => S.on.find(t => same(t.name, name));
        const listed = g.options.map(o => {
            const t = find(o.name);
            return `<button type="button" data-name="${esc(o.name)}" class="${t ? "on" : ""}" aria-pressed="${t ? "true" : "false"}"${levelTip(t)}>${esc(o.label)}${dots(t)}</button>`;
        }).join("");
        const extra = S.on.filter(t => !g.options.some(o => same(o.name, t.name))).map(t =>
            `<button type="button" data-name="${esc(t.name)}" data-extra="1" class="on" aria-pressed="true" title="${esc(root.dataset.remove)}">${esc(t.name)}${dots(t)} <i class="fa-solid fa-xmark" aria-hidden="true"></i></button>`
        ).join("");
        const other = esc(root.dataset.other || "Other");
        return `<div class="fm-lbl">${esc(g.label)}</div>
            <div class="fm-pick" role="group" aria-label="${esc(g.label)}">${listed}${extra}<input type="text" class="fm-pick-other" maxlength="255" placeholder="${other}: ${esc(root.dataset.otherPh || "")}" aria-label="${esc(g.label)}: ${other}"></div>`;
    }

    function draw(g) {
        const el = root.querySelector(`.fm-pick-group[data-group="${g.group}"]`);
        el.innerHTML = groupHTML(g);
        return el;
    }

    root.innerHTML = groups.map(g => `<div class="fm-pick-group" data-group="${esc(g.group)}"></div>`).join("");
    groups.forEach(draw);

    const groupOf = el => groups.find(g => g.group === el.closest(".fm-pick-group").dataset.group);

    function pick(g, name) {
        const S = state[g.group];
        if (!S.on.some(t => same(t.name, name))) S.on.push(S.memo[name.toLowerCase()] || { name });
    }

    // A typed name: one already listed (by its saved name or its label in
    // the user's language) selects that button instead of adding a copy.
    function addTyped(g, raw) {
        const typed = cap(raw);
        if (!typed) return null;
        const listed = g.options.find(o => same(o.name, typed) || same(o.label, typed));
        const name = listed ? listed.name : typed;
        pick(g, name);
        return name;
    }

    function flash(el, name) {
        const b = [...el.querySelectorAll(".fm-pick button")].find(x => same(x.dataset.name, name));
        if (!b) return;
        b.classList.remove("flash");
        void b.offsetWidth;
        b.classList.add("flash");
    }

    root.addEventListener("click", e => {
        const b = e.target.closest(".fm-pick button");
        if (!b) return;
        const g = groupOf(b);
        const S = state[g.group];
        const name = b.dataset.name;
        if (S.on.some(t => same(t.name, name))) S.on = S.on.filter(t => !same(t.name, name));
        else pick(g, name);
        const el = draw(g);
        const again = [...el.querySelectorAll(".fm-pick button")].find(x => same(x.dataset.name, name));
        (again || el.querySelector(".fm-pick-other")).focus();
    });

    function commit(input, keepFocus) {
        const v = input.value;
        if (!v.trim()) return;
        const g = groupOf(input);
        input.value = "";
        const name = addTyped(g, v);
        const el = draw(g);
        if (name) flash(el, name);
        if (keepFocus) el.querySelector(".fm-pick-other").focus();
    }
    root.addEventListener("keydown", e => {
        if (e.target.matches(".fm-pick-other") && (e.key === "Enter" || e.key === ",")) {
            e.preventDefault();
            commit(e.target, true);
        }
    });
    root.addEventListener("change", e => {
        if (e.target.matches(".fm-pick-other")) commit(e.target, false);
    });

    window.strainAttrPick = {
        collect() {
            // Text left in an Other box still counts.
            root.querySelectorAll(".fm-pick-other").forEach(i => {
                if (i.value.trim()) { addTyped(groupOf(i), i.value); i.value = ""; }
            });
            const out = {};
            groups.forEach(g => {
                out[g.group] = state[g.group].on.map(t => {
                    const a = { name: t.name };
                    if (t.intensity != null) a.intensity = t.intensity;
                    if (t.level) a.level = t.level;
                    return a;
                });
            });
            return out;
        },
    };
})();
