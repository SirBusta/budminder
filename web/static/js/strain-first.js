// "Add the strain first." Add Plant and Edit plant send people to the Strains
// page for a strain that hasn't been added yet, and bring them back afterwards
// with what they'd already filled in. That draft is kept in this browser tab
// only (sessionStorage) and is forgotten after two hours.
//
//   StrainFirst.noMatchHTML(name)     the Strain field's "hasn't been added yet" note
//   StrainFirst.strainField(select)   the Strain autocomplete set up that way
//   StrainFirst.showError(ac, name) / clearError(ac)   the same, under the field on Save
//   StrainFirst.wire(form, returnTo)  remember the form when a Strains link is followed
//   StrainFirst.take(returnTo)        the saved draft for this page (once), or null
//   StrainFirst.peek() / clear()      used by the Strains and strain pages
window.StrainFirst = (function () {
    "use strict";
    const KEY = "budminder.plantDraft";
    const MAX_AGE = 2 * 60 * 60 * 1000;

    const esc = s => String(s == null ? "" : s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
    const t = k => (window.uiMessages ? uiMessages.t(k) : k);
    // A translated sentence with {name} in bold.
    const withName = (k, name) => esc(t(k)).split("{name}").join(`<b>${esc(name)}</b>`);

    function strainsURL(name) {
        return "/strains" + (name ? "?add=" + encodeURIComponent(name) : "");
    }

    function noMatchHTML(name) {
        return `<i class="fa-solid fa-circle-info me-1 text-primary"></i>${withName("sf_not_added", name)}
            <a href="${esc(strainsURL(name))}" data-strain-first class="d-block mt-1 fw-semibold">${esc(t("sf_add_on_strains"))} <i class="fa-solid fa-arrow-right ms-1"></i></a>`;
    }

    // The Strain picker on Add Plant / Edit plant: no "Add new" choice; a name
    // that isn't there gets the note, and typos get "Did you mean".
    function strainField(select) {
        const ac = new IsleyAutocomplete(select, {
            placeholder: select.dataset.acPlaceholder || "",
            allowNew: false,
            typoSuggest: true,
            noMatch: name => noMatchHTML(name),
            labels: { didYouMean: select.dataset.acDidYouMean || "Did you mean {name}?" },
        });
        ac.input.addEventListener("input", () => clearError(ac));
        select.addEventListener("change", () => clearError(ac));
        return ac;
    }

    function showError(ac, name) {
        const box = ac.wrapper.parentNode;
        let fb = box.querySelector(".sf-feedback");
        if (!fb) {
            fb = document.createElement("div");
            fb.className = "invalid-feedback d-block sf-feedback";
            box.appendChild(fb);
        }
        fb.innerHTML = `${withName("sf_not_added", name)} <a href="${esc(strainsURL(name))}" data-strain-first>${esc(t("sf_add_on_strains"))}</a>`;
        ac.input.classList.add("is-invalid");
        ac.input.focus();
    }

    function clearError(ac) {
        const fb = ac.wrapper.parentNode.querySelector(".sf-feedback");
        if (fb) fb.remove();
        ac.input.classList.remove("is-invalid");
    }

    // What's filled in on the form, by element id (the strain itself is chosen
    // again on the way back).
    function formValues(form) {
        const values = {};
        form.querySelectorAll("input[id], select[id], textarea[id]").forEach(el => {
            if (el.id === "strainSelect" || el.type === "file" || el.type === "hidden" || el.classList.contains("isley-ac-input")) return;
            values[el.id] = el.type === "checkbox" ? el.checked : el.value;
        });
        return values;
    }

    function save(form, returnTo, plantName) {
        try {
            sessionStorage.setItem(KEY, JSON.stringify({ returnTo, plantName: plantName || "", values: formValues(form), at: Date.now() }));
        } catch (e) { /* storage off: the way back just isn't offered */ }
    }

    function peek() {
        try {
            const d = JSON.parse(sessionStorage.getItem(KEY) || "null");
            if (d && d.returnTo && Date.now() - d.at < MAX_AGE) return d;
            sessionStorage.removeItem(KEY);
        } catch (e) { /* nothing saved */ }
        return null;
    }

    function clear() {
        try { sessionStorage.removeItem(KEY); } catch (e) { /* nothing saved */ }
    }

    function take(returnTo) {
        const d = peek();
        if (!d || d.returnTo !== returnTo) return null;
        clear();
        return d;
    }

    // Puts a draft's values back (the page then refreshes any autocomplete
    // fields it has, since their visible text follows the hidden select).
    function restore(d) {
        Object.entries(d.values || {}).forEach(([id, v]) => {
            const el = document.getElementById(id);
            if (!el) return;
            if (el.type === "checkbox") el.checked = !!v;
            else el.value = v;
            el.dispatchEvent(new Event("change", { bubbles: true }));
        });
    }

    // Following any Strains link marked data-strain-first (the note at the top,
    // the Strain field's note, the error under it) saves the form first.
    function wire(form, returnTo, plantName) {
        document.addEventListener("click", e => {
            if (e.target.closest("a[data-strain-first]")) save(form, returnTo, typeof plantName === "function" ? plantName() : plantName);
        }, true);
    }

    function backURL(d, strainId) {
        return d.returnTo + "?strain_id=" + encodeURIComponent(strainId) + "&resume=1";
    }

    return { noMatchHTML, strainsURL, withName, esc, strainField, showError, clearError, wire, peek, clear, take, restore, backURL };
})();
