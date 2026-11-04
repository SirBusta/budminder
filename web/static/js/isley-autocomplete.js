/**
 * IsleyAutocomplete — Progressive enhancement that converts a standard <select>
 * into a searchable autocomplete dropdown.
 *
 * Usage:
 *   const ac = new IsleyAutocomplete(selectElement, {
 *       placeholder: "Search strains...",    // input placeholder
 *       sublabelKey: null,                   // optional: function(option) => sublabel string
 *       onSelect:    null,                   // optional: callback(value, label, isNew)
 *       allowNew:    true,                   // whether the "Add New" option is supported
 *       newValue:    "new",                  // the <option> value that triggers "add new"
 *       minChars:    1,                      // minimum chars before showing suggestions
 *       maxResults:  10,                     // max dropdown items
 *
 *       // Free-text "add new" (e.g. breeders):
 *       sublabel:        null,   // optional: function(option) => muted text shown after the label
 *       newFromText:     false,  // "Add new" uses the typed text; typing a name and leaving
 *                                // the field also counts as choosing it
 *       newNameInput:    null,   // element whose value receives the chosen new name
 *       referenceSource: null,   // () => Promise<string[]>: extra suggestions that are not
 *                                // yet rows; picking one counts as a new name
 *       labels: { newText, didYouMean, suggested, results },  // "{name}" / "{count}" placeholders
 *
 *       // When nothing matches (e.g. strains, which are added on the Strains page):
 *       noMatch:     null,   // optional: function(query) => HTML for a note shown instead of
 *                            // results (not selectable; links in it work)
 *       typoSuggest: false,  // offer "Did you mean X?" for names one or two letters off
 *
 *       // Names typed by the user (newFromText): how "Use ..." shows and saves them.
 *       formatNew:   null,   // optional: function(name) => name, e.g. IsleyAutocomplete.capitalizeName
 *
 *       // A second, slower source searched as you type (e.g. StrainCompass), shown in its
 *       // own group under the options; picking one counts as a new name.
 *       remote: null,        // { fetch: q => Promise<[{label, sublabel}]>, minChars: 2, delay: 300,
 *                            //   labels: { mine, title, searching, none, failed } }
 *   });
 *
 *   An <option data-name="Grape OG">Grape OG (Cali Connection)</option> is also found by
 *   its bare name: typing exactly "Grape OG" selects it when only one option has that name.
 *
 *   // Programmatic control:
 *   ac.reset();            // clear the input + selection
 *   ac.setValue(id);       // select an option by value
 *   ac.setNewName(name);   // choose a new (not-yet-existing) name; setNewName(name, false) skips matching existing options
 *   ac.getValue();         // get current select value
 *   ac.getNewName();       // the chosen new name ("" if an existing option is selected)
 *   ac.destroy();          // tear down and restore original select
 */

let _isleyAcUid = 0;

// Short forms in strain names that are written in capitals ("sfv og" -> "SFV OG").
const ISLEY_AC_SHORT_FORMS = new Set([
    "og", "gsc", "gg", "gdp", "gmo", "ak", "sfv", "cbd", "thc", "cbg", "bx", "ibl",
    "xl", "xxl", "uk", "usa", "la", "ny", "nyc", "mac", "dj",
]);

// Words that don't distinguish one breeder from another ("Sensi Seeds" vs
// "Sensi Seed Co.") when looking for near-duplicates.
const ISLEY_AC_NOISE_WORDS = new Set([
    "the", "seed", "seeds", "seedbank", "bank", "co", "company", "genetics",
    "gen", "cannabis", "inc", "ltd", "llc",
]);

class IsleyAutocomplete {
    constructor(selectEl, opts = {}) {
        if (!selectEl || selectEl.tagName !== "SELECT") {
            console.warn("IsleyAutocomplete: expected a <select> element", selectEl);
            return;
        }

        this.select = selectEl;
        this.opts = Object.assign({
            placeholder: "Type to search...",
            sublabelKey: null,
            onSelect: null,
            allowNew: true,
            newValue: "new",
            minChars: 1,
            maxResults: 10,
            sublabel: null,
            newFromText: false,
            newNameInput: null,
            referenceSource: null,
            noMatch: null,
            typoSuggest: false,
            formatNew: null,
            remote: null,
        }, opts);
        this.labels = Object.assign({
            newText: 'Add "{name}"',
            didYouMean: "Did you mean {name}?",
            suggested: "suggested",
            results: "{count} results available",
        }, opts.labels || {});

        this._uid = ++_isleyAcUid;
        this._items = [];
        this._refItems = [];
        this._refState = "idle"; // idle | loading | loaded
        this._newLabel = "";
        this._newName = "";
        // Remote search: the query it belongs to, idle | wait | done | failed, its items.
        this._remote = { q: "", state: "idle", items: [] };
        this._remoteSeq = 0;
        this._remoteTimer = null;
        this._built = false;
        this._activeIdx = -1;
        this._build();
    }

    /** Cached per-page fetch of the breeder reference list. */
    static fetchBreederReference() {
        if (!IsleyAutocomplete._breederRefPromise) {
            IsleyAutocomplete._breederRefPromise = fetch("/breeders/reference")
                .then(r => (r.ok ? r.json() : { names: [] }))
                .then(d => (d && Array.isArray(d.names) ? d.names : []))
                .catch(() => []);
        }
        return IsleyAutocomplete._breederRefPromise;
    }

    /**
     * Breeder combobox: the user's breeders, then reference suggestions, plus
     * "Add '<typed>'". Localized labels come from data-ac-* attributes on the
     * select; the chosen new name is written to newNameInput.
     */
    static breederPicker(selectEl, newNameInput, opts = {}) {
        if (!selectEl) return null;
        const d = selectEl.dataset;
        const labels = {};
        [["newText", d.acNewText], ["didYouMean", d.acDidYouMean],
            ["suggested", d.acSuggested], ["results", d.acResults]]
            .forEach(([k, v]) => { if (v) labels[k] = v; });
        return new IsleyAutocomplete(selectEl, Object.assign({
            placeholder: d.acPlaceholder || "",
            newFromText: true,
            newNameInput: newNameInput || null,
            referenceSource: IsleyAutocomplete.fetchBreederReference,
            maxResults: 12,
            labels,
        }, opts));
    }

    /**
     * A typed strain name with a capital at the start of each word: "hazy donkey" ->
     * "Hazy Donkey". Words with a capital already stay as typed (McFly); short forms
     * become capitals (OG, SFV, BX1, S1); a word after an apostrophe ('n') and
     * numbers (#33) stay as they are.
     */
    static capitalizeName(str) {
        return String(str || "").trim().replace(/\s+/g, " ").replace(/[^\s-]+/g, w => {
            if (/[A-Z]/.test(w) || /^['’]/.test(w)) return w;
            if (ISLEY_AC_SHORT_FORMS.has(w) || /^(bx|s|f|r|v)\d+$/.test(w)) return w.toUpperCase();
            return w.charAt(0).toUpperCase() + w.slice(1);
        });
    }

    /** Lowercased, punctuation- and noise-word-free key for near-duplicate checks. */
    static normalizeName(str) {
        const words = String(str || "").toLowerCase().replace(/&/g, " and ")
            .split(/[^a-z0-9]+/).filter(Boolean);
        const kept = words.filter(w => !ISLEY_AC_NOISE_WORDS.has(w));
        return (kept.length ? kept : words).join("");
    }

    /* ---- public API ---- */

    /** Reset to blank / no selection */
    reset() {
        this.input.value = "";
        this.select.value = "";
        this._setNewName("");
        this._hideDropdown();
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
    }

    /** Programmatically select a value */
    setValue(val) {
        const item = this._items.find(i => String(i.value) === String(val));
        if (item) {
            this.input.value = item.label;
            this.select.value = item.value;
            this._setNewName("");
        } else if (val === this.opts.newValue && this.opts.allowNew) {
            this.input.value = "";
            this.select.value = this.opts.newValue;
        } else {
            this.select.value = val;
            const opt = this.select.querySelector(`option[value="${CSS.escape(String(val))}"]`);
            if (opt) this.input.value = opt.textContent.trim();
        }
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
    }

    /** Choose a name that isn't an existing option (an existing exact match wins unless matchExisting is false). */
    setNewName(name, matchExisting = true) {
        name = String(name || "").trim();
        if (!name) return;
        const own = matchExisting ? this._uniqueExact(this._items, name) : null;
        if (own) {
            this._selectItem(own, true);
            return;
        }
        this._chooseNew(name, true);
    }

    getValue() {
        return this.select.value;
    }

    /** Settle typed-but-unchosen text into a selection (call before submitting a form). */
    commit() {
        this._hideDropdown();
        this._resolveInput();
    }

    getNewName() {
        return this.select.value === this.opts.newValue ? this._newName : "";
    }

    /** Tear down and restore original select visibility */
    destroy() {
        if (!this._built) return;
        if (this.wrapper && this.wrapper.parentNode) {
            this.wrapper.parentNode.insertBefore(this.select, this.wrapper);
            this.wrapper.remove();
        }
        this.select.style.display = "";
        this.select.removeAttribute("tabindex");
        this._built = false;
    }

    /** Refresh items list from the current select options (call after dynamically adding options) */
    refreshItems() {
        this._extractItems();
        this._mergeRefItems();
    }

    /* ---- private ---- */

    _build() {
        this._extractItems();

        // Hide the original <select> but keep it in the DOM for form submission
        this.select.style.display = "none";
        this.select.setAttribute("tabindex", "-1");

        this.wrapper = document.createElement("div");
        this.wrapper.className = "isley-ac-wrapper position-relative";

        const listboxId = `isley-ac-listbox-${this._uid}`;

        this.input = document.createElement("input");
        this.input.type = "text";
        this.input.className = "form-control isley-ac-input";
        this.input.placeholder = this.opts.placeholder;
        this.input.autocomplete = "off";
        this.input.setAttribute("role", "combobox");
        this.input.setAttribute("aria-autocomplete", "list");
        this.input.setAttribute("aria-expanded", "false");
        this.input.setAttribute("aria-haspopup", "listbox");
        this.input.setAttribute("aria-controls", listboxId);

        // Mirror the required attribute so the visible input validates
        if (this.select.required) {
            this.input.required = true;
            this.select.required = false; // prevent hidden-field validation quirks
        }

        const preselected = this.select.value;
        if (preselected && preselected !== this.opts.newValue) {
            const item = this._items.find(i => String(i.value) === String(preselected));
            if (item) this.input.value = item.label;
        }

        this.dropdown = document.createElement("div");
        this.dropdown.className = "isley-ac-dropdown";
        this.dropdown.id = listboxId;
        this.dropdown.setAttribute("role", "listbox");

        this._liveRegion = document.createElement("div");
        this._liveRegion.setAttribute("aria-live", "polite");
        this._liveRegion.setAttribute("aria-atomic", "true");
        this._liveRegion.className = "visually-hidden";

        this.wrapper.appendChild(this.input);
        this.wrapper.appendChild(this.dropdown);
        this.wrapper.appendChild(this._liveRegion);

        this.select.parentNode.insertBefore(this.wrapper, this.select.nextSibling);

        // ---- Event listeners ----

        let debounceTimer = null;

        this.input.addEventListener("input", () => {
            clearTimeout(debounceTimer);
            this._loadReferences();
            const query = this.input.value.trim();
            this._searchRemote(query);

            if (query.length < this.opts.minChars) {
                this._hideDropdown();
                if (query.length === 0) {
                    this.select.value = "";
                    this._setNewName("");
                    this.select.dispatchEvent(new Event("change", { bubbles: true }));
                }
                return;
            }

            debounceTimer = setTimeout(() => this._showFor(query), 150);
        });

        this.input.addEventListener("focus", () => {
            this._loadReferences();
            const query = this.input.value.trim();
            if (this._remote.q !== query) this._searchRemote(query);
            if (query.length >= this.opts.minChars) {
                this._showFor(query);
            } else if (query.length === 0) {
                this._renderDropdown(this._items.slice(0, this.opts.maxResults), "");
            }
        });

        this.input.addEventListener("blur", () => {
            setTimeout(() => {
                this._hideDropdown();
                this._resolveInput();
            }, 200);
        });

        this.input.addEventListener("keydown", (e) => {
            if (this.dropdown.style.display !== "block") return;
            const items = this.dropdown.querySelectorAll(".isley-ac-item");
            let idx = this._activeIdx;

            if (e.key === "ArrowDown") {
                e.preventDefault();
                idx = Math.min(idx + 1, items.length - 1);
                this._setActiveItem(items, idx);
            } else if (e.key === "ArrowUp") {
                e.preventDefault();
                idx = Math.max(idx - 1, 0);
                this._setActiveItem(items, idx);
            } else if (e.key === "Enter") {
                e.preventDefault();
                const active = items[this._activeIdx];
                if (active) {
                    active.dispatchEvent(new Event("mousedown"));
                } else {
                    this._hideDropdown();
                    this._resolveInput();
                }
            } else if (e.key === "Tab") {
                const active = items[this._activeIdx];
                if (active) active.dispatchEvent(new Event("mousedown"));
            } else if (e.key === "Escape") {
                this._hideDropdown();
            }
        });

        this._built = true;
    }

    _extractItems() {
        this._items = [];
        const options = this.select.querySelectorAll("option");
        options.forEach(opt => {
            const val = opt.value;
            if (!val || val === this.opts.newValue) {
                if (val === this.opts.newValue) {
                    this._newLabel = opt.textContent.trim();
                }
                return;
            }
            if (opt.disabled) return;

            const label = this.opts.sublabelKey
                ? this.opts.sublabelKey(opt)
                : opt.textContent.trim();
            this._items.push({
                value: val,
                name: (opt.dataset.name || "").trim(),
                label: opt.textContent.trim(),
                displayLabel: label,
                sublabel: this.opts.sublabel ? (this.opts.sublabel(opt) || "") : "",
            });
        });
    }

    _loadReferences() {
        if (!this.opts.referenceSource || this._refState !== "idle") return;
        this._refState = "loading";
        Promise.resolve(this.opts.referenceSource()).then(names => {
            this._refNames = Array.isArray(names) ? names : [];
            this._refState = "loaded";
            this._mergeRefItems();
            // Refresh an open dropdown so late-arriving suggestions appear.
            if (this.dropdown.style.display === "block" && document.activeElement === this.input) {
                const q = this.input.value.trim();
                if (q.length >= this.opts.minChars) this._showFor(q);
            }
        });
    }

    /** Reference names minus anything the user already has, case-folded to one entry each. */
    _mergeRefItems() {
        const seen = new Set(this._items.map(i => i.label.toLowerCase()));
        this._refItems = [];
        (this._refNames || []).forEach(name => {
            const key = String(name).toLowerCase();
            if (!name || seen.has(key)) return;
            seen.add(key);
            this._refItems.push({ value: null, label: name, ref: true });
        });
    }

    _showFor(query) {
        this._renderDropdown(this._search(query), query);
    }

    /** Start the remote search for query after a pause in typing; a newer query cancels it. */
    _searchRemote(query) {
        const r = this.opts.remote;
        if (!r) return;
        clearTimeout(this._remoteTimer);
        const seq = ++this._remoteSeq;
        if (query.length < (r.minChars || 2)) {
            this._remote = { q: query, state: "idle", items: [] };
            return;
        }
        this._remote = { q: query, state: "wait", items: [] };
        this._remoteTimer = setTimeout(() => {
            Promise.resolve()
                .then(() => r.fetch(query))
                .then(items => ({ state: "done", items: Array.isArray(items) ? items : [] }))
                .catch(() => ({ state: "failed", items: [] }))
                .then(res => {
                    if (seq !== this._remoteSeq) return;
                    this._remote = { q: query, state: res.state, items: res.items };
                    if (this.dropdown.style.display === "block" && document.activeElement === this.input &&
                        this.input.value.trim() === query) {
                        this._showFor(query);
                    }
                });
        }, r.delay == null ? 300 : r.delay);
    }

    /** The remote results for query, minus names that are already options. */
    _remoteItems(query) {
        if (!this.opts.remote || this._remote.q !== query || this._remote.state !== "done") return [];
        const own = new Set(this._items.map(i => i.label.toLowerCase()));
        return this._remote.items
            .filter(i => i && i.label && !own.has(String(i.label).toLowerCase()))
            .map(i => ({ value: null, label: String(i.label), sublabel: i.sublabel || "", remote: true }));
    }

    /** How a typed name is shown and saved: a remote result's spelling if it matches, else formatNew. */
    _typedName(text) {
        const remote = this._findExact(this._remoteItems(text), text);
        if (remote) return remote.label;
        return this.opts.formatNew ? this.opts.formatNew(text) : text;
    }

    _matches(item, lower, norm) {
        const label = item.label.toLowerCase();
        if (label.includes(lower)) return true;
        return norm !== "" && IsleyAutocomplete.normalizeName(item.label).includes(norm);
    }

    _rank(list, lower, norm) {
        return list
            .filter(i => this._matches(i, lower, norm))
            .sort((a, b) => {
                const as = a.label.toLowerCase().startsWith(lower) ? 0 : 1;
                const bs = b.label.toLowerCase().startsWith(lower) ? 0 : 1;
                return as - bs;
            });
    }

    _search(query) {
        const lower = query.toLowerCase();
        const norm = this.opts.newFromText ? IsleyAutocomplete.normalizeName(query) : "";
        const own = this._rank(this._items, lower, norm);
        const refs = this._rank(this._refItems, lower, norm);
        return own.concat(refs).slice(0, this.opts.maxResults);
    }

    _findExact(list, text) {
        const lower = text.toLowerCase();
        return list.find(i => i.label.toLowerCase() === lower);
    }

    /** The only item whose label equals text, or null when none or several do (e.g. one strain from two breeders). */
    _uniqueExact(list, text) {
        const lower = text.toLowerCase();
        const hits = list.filter(i => i.label.toLowerCase() === lower);
        return hits.length === 1 ? hits[0] : null;
    }

    /** Bare-name match (option data-name) when exactly one option has that name. */
    _uniqueName(text) {
        const lower = text.toLowerCase();
        const hits = this._items.filter(i => i.name && i.name.toLowerCase() === lower);
        return hits.length === 1 ? hits[0] : null;
    }

    /** Options whose name is one or two letters away from text (typos), closest first. */
    _typos(text) {
        const squash = t => String(t).toLowerCase().replace(/[^a-z0-9]+/g, "");
        const q = squash(text);
        if (q.length < 4) return [];
        const max = q.length >= 8 ? 2 : 1;
        return this._items
            .map(i => ({ i, d: IsleyAutocomplete.editDistance(q, squash(i.name || i.label), max) }))
            .filter(x => x.d <= max)
            .sort((a, b) => a.d - b.d)
            .slice(0, 3)
            .map(x => x.i);
    }

    /** Levenshtein distance, giving up (returning max + 1) once it can't stay within max. */
    static editDistance(a, b, max) {
        if (Math.abs(a.length - b.length) > max) return max + 1;
        let prev = Array.from({ length: b.length + 1 }, (_, j) => j);
        for (let i = 1; i <= a.length; i++) {
            const cur = [i];
            let best = i;
            for (let j = 1; j <= b.length; j++) {
                cur[j] = Math.min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1));
                best = Math.min(best, cur[j]);
            }
            if (best > max) return max + 1;
            prev = cur;
        }
        return prev[b.length];
    }

    /** Nothing matches: "Did you mean...?" choices, then the noMatch note. */
    _renderNoMatch(query) {
        const typos = this.opts.typoSuggest ? this._typos(query) : [];
        typos.forEach((item, i) => {
            const div = document.createElement("div");
            div.className = "isley-ac-item isley-ac-typo";
            div.id = `isley-ac-opt-${this._uid}-t${i}`;
            div.setAttribute("role", "option");
            div.setAttribute("aria-selected", "false");
            const name = this._escapeHtml(item.name || item.label);
            div.innerHTML = `<i class="fa-solid fa-wand-magic-sparkles me-1 text-warning"></i>` +
                this._escapeHtml(this.labels.didYouMean).replace("{name}", `<b>${name}</b>`) +
                (item.name && item.label !== item.name ? `<div class="small text-muted ms-4">${this._escapeHtml(item.label.slice(item.name.length).trim().replace(/^\(|\)$/g, ""))}</div>` : "");
            div.addEventListener("mousedown", (e) => {
                e.preventDefault();
                this._selectItem(item, false);
            });
            this.dropdown.appendChild(div);
        });
        const html = this.opts.noMatch ? this.opts.noMatch(query) : "";
        if (html) {
            if (typos.length) {
                const sep = document.createElement("div");
                sep.className = "isley-ac-separator";
                sep.setAttribute("role", "separator");
                this.dropdown.appendChild(sep);
            }
            const note = document.createElement("div");
            note.className = "isley-ac-note";
            note.innerHTML = html;
            // Keep the field focused: a blur would close the dropdown and take
            // the note's link away before a slow click lands on it.
            note.addEventListener("mousedown", (e) => e.preventDefault());
            this.dropdown.appendChild(note);
        }
        if (!typos.length && !html) {
            this._hideDropdown();
            return;
        }
        this.dropdown.style.display = "block";
        this.input.setAttribute("aria-expanded", "true");
        this._announce(this.dropdown.textContent.trim());
    }

    _renderDropdown(matches, query) {
        this.dropdown.innerHTML = "";
        this._activeIdx = -1;

        if (matches.length === 0 && query !== "" && (this.opts.noMatch || this.opts.typoSuggest)) {
            this._renderNoMatch(query);
            return;
        }

        // The remote group (e.g. StrainCompass) shows once the query is long enough.
        const remote = this.opts.remote;
        const showRemote = !!remote && query !== "" && query.length >= (remote.minChars || 2);
        const remoteItems = showRemote ? this._remoteItems(query) : [];
        const remoteState = !showRemote ? "idle" : (this._remote.q === query ? this._remote.state : "wait");
        const rl = (remote && remote.labels) || {};

        const showTextNew = this.opts.allowNew && this.opts.newFromText && query !== "" &&
            !this._findExact(this._items, query) && !this._findExact(this._refItems, query) &&
            !this._findExact(remoteItems, query);
        const showFixedNew = this.opts.allowNew && !this.opts.newFromText && this._newLabel;

        if (matches.length === 0 && !showTextNew && !showFixedNew && !showRemote) {
            this._hideDropdown();
            return;
        }
        this.dropdown.classList.toggle("isley-ac-grouped", !!remote);

        const addGroup = (icon, title, status) => {
            const g = document.createElement("div");
            g.className = "isley-ac-group";
            g.setAttribute("role", "presentation");
            g.innerHTML = `<i class="fa-solid ${icon}" aria-hidden="true"></i>${this._escapeHtml(title || "")}` +
                (status ? `<span class="isley-ac-group-status">${status}</span>` : "");
            g.addEventListener("mousedown", (e) => e.preventDefault());
            this.dropdown.appendChild(g);
        };
        const addItem = (item, id) => {
            const div = document.createElement("div");
            div.className = "isley-ac-item";
            div.id = `isley-ac-opt-${this._uid}-${id}`;
            div.setAttribute("role", "option");
            div.setAttribute("aria-selected", "false");
            div.innerHTML = this._highlightMatch(item.label, query) +
                (item.sublabel ? ` <small class="text-muted">${this._escapeHtml(item.sublabel)}</small>` : "") +
                (item.ref ? ` <small class="text-muted isley-ac-ref-tag">${this._escapeHtml(this.labels.suggested)}</small>` : "");
            div.addEventListener("mousedown", (e) => {
                e.preventDefault();
                if (item.ref || item.remote) {
                    this._chooseNew(item.label, false);
                } else {
                    this._selectItem(item, false);
                }
            });
            this.dropdown.appendChild(div);
        };

        if (showRemote && matches.length > 0) addGroup("fa-seedling", rl.mine);
        matches.forEach((item, i) => addItem(item, i));

        if (showRemote) {
            const spin = `<i class="fa-solid fa-spinner fa-spin me-1" aria-hidden="true"></i>${this._escapeHtml(rl.searching || "")}`;
            addGroup("fa-flask", rl.title, remoteState === "wait" ? spin : "");
            if (remoteState === "done" && remoteItems.length) {
                remoteItems.forEach((item, i) => addItem(item, `r${i}`));
            } else if (remoteState === "done" || remoteState === "failed") {
                const note = document.createElement("div");
                note.className = "isley-ac-status";
                note.innerHTML = remoteState === "failed"
                    ? `<i class="fa-solid fa-triangle-exclamation me-1 text-warning" aria-hidden="true"></i>${this._escapeHtml(rl.failed || "")}`
                    : this._escapeHtml(rl.none || "");
                note.addEventListener("mousedown", (e) => e.preventDefault());
                this.dropdown.appendChild(note);
            }
        }

        if (showTextNew || showFixedNew) {
            if (matches.length > 0 || showRemote) {
                const sep = document.createElement("div");
                sep.className = "isley-ac-separator";
                sep.setAttribute("role", "separator");
                this.dropdown.appendChild(sep);
            }

            const addNew = document.createElement("div");
            addNew.className = "isley-ac-item isley-ac-new-item";
            addNew.id = `isley-ac-opt-${this._uid}-new`;
            addNew.setAttribute("role", "option");
            addNew.setAttribute("aria-selected", "false");
            const typed = showTextNew ? this._typedName(query) : "";
            const text = showTextNew ? this.labels.newText.replace("{name}", typed) : this._newLabel;
            let html = `<i class="fa-solid fa-plus me-1"></i> ${this._escapeHtml(text)}`;
            if (showTextNew) {
                const near = this._nearDuplicate(query);
                if (near) {
                    html += `<div class="small text-warning isley-ac-near">${this._escapeHtml(this.labels.didYouMean.replace("{name}", near.label))}</div>`;
                }
            }
            addNew.innerHTML = html;
            addNew.addEventListener("mousedown", (e) => {
                e.preventDefault();
                if (showTextNew) {
                    this._chooseNew(typed, false);
                } else {
                    this._selectFixedNew();
                }
            });
            this.dropdown.appendChild(addNew);
        }

        this.dropdown.style.display = "block";
        this.input.setAttribute("aria-expanded", "true");

        const total = this.dropdown.querySelectorAll(".isley-ac-item").length;
        this._announce(this.labels.results.replace("{count}", total));
    }

    /** An existing or suggested name that differs from text only by case, spacing, punctuation or noise words. */
    _nearDuplicate(text) {
        const norm = IsleyAutocomplete.normalizeName(text);
        if (!norm) return null;
        const lower = text.toLowerCase();
        return this._items.concat(this._refItems).find(i =>
            i.label.toLowerCase() !== lower && IsleyAutocomplete.normalizeName(i.label) === norm) || null;
    }

    _selectItem(item, silent) {
        this.input.value = item.label;
        this.select.value = item.value;
        this._setNewName("");
        this._hideDropdown();
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
        if (!silent && this.opts.onSelect) {
            this.opts.onSelect(item.value, item.label, false);
        }
    }

    _chooseNew(name, silent) {
        this.input.value = name;
        this.select.value = this.opts.newValue;
        this._setNewName(name);
        this._hideDropdown();
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
        if (!silent && this.opts.onSelect) {
            this.opts.onSelect(this.opts.newValue, name, true);
        }
    }

    _selectFixedNew() {
        this.input.value = this._newLabel;
        this.select.value = this.opts.newValue;
        this._hideDropdown();
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
        if (this.opts.onSelect) {
            this.opts.onSelect(this.opts.newValue, this._newLabel, true);
        }
    }

    _setNewName(name) {
        this._newName = name;
        if (this.opts.newNameInput) this.opts.newNameInput.value = name;
    }

    /** After blur/Enter, make the selection agree with the visible text. */
    _resolveInput() {
        const text = this.input.value.trim();
        if (!text) return;

        const currentVal = this.select.value;
        const currentItem = this._items.find(i => String(i.value) === String(currentVal));
        if (currentItem && currentItem.label === text) return;

        const exact = this.opts.newFromText ? this._uniqueExact(this._items, text)
            : (this._findExact(this._items, text) || this._uniqueName(text));
        if (exact) {
            this._selectItem(exact, false);
            return;
        }

        if (!this.opts.newFromText || !this.opts.allowNew) {
            // Typed over an earlier choice with text that matches nothing: drop
            // the old choice, so a form can't save something other than what
            // the field shows. ("Add New ..." picked from the list keeps its label.)
            if (currentVal === this.opts.newValue && text === this._newLabel) return;
            if (currentVal !== "") {
                this.select.value = "";
                this._setNewName("");
                this.select.dispatchEvent(new Event("change", { bubbles: true }));
            }
            return;
        }
        if (currentVal === this.opts.newValue && this._newName === text) return;

        const ref = this._findExact(this._refItems, text);
        this._chooseNew(ref ? ref.label : this._typedName(text), false);
    }

    _hideDropdown() {
        this.dropdown.style.display = "none";
        this.dropdown.innerHTML = "";
        this._activeIdx = -1;
        this.input.setAttribute("aria-expanded", "false");
        this.input.removeAttribute("aria-activedescendant");
    }

    _setActiveItem(items, idx) {
        items.forEach(i => {
            i.classList.remove("active");
            i.setAttribute("aria-selected", "false");
        });
        if (items[idx]) {
            items[idx].classList.add("active");
            items[idx].setAttribute("aria-selected", "true");
            items[idx].scrollIntoView({ block: "nearest" });
            this.input.setAttribute("aria-activedescendant", items[idx].id);
            this._activeIdx = idx;
        }
    }

    _announce(message) {
        this._liveRegion.textContent = "";
        requestAnimationFrame(() => {
            this._liveRegion.textContent = message;
        });
    }

    /** Bold every case-insensitive occurrence of query, escaping each piece separately. */
    _highlightMatch(label, query) {
        if (!query) return this._escapeHtml(label);
        const lowerLabel = label.toLowerCase();
        const lowerQuery = query.toLowerCase();
        let out = "";
        let pos = 0;
        let idx = lowerLabel.indexOf(lowerQuery);
        while (idx !== -1 && lowerQuery.length > 0) {
            out += this._escapeHtml(label.slice(pos, idx));
            out += "<strong>" + this._escapeHtml(label.slice(idx, idx + query.length)) + "</strong>";
            pos = idx + query.length;
            idx = lowerLabel.indexOf(lowerQuery, pos);
        }
        return out + this._escapeHtml(label.slice(pos));
    }

    _escapeHtml(str) {
        const div = document.createElement("div");
        div.textContent = str;
        return div.innerHTML;
    }
}

// A strain name typed into a field marked data-capitalize gets capitals when the
// user changes it ("hazy donkey" -> "Hazy Donkey"); opening a page never changes it.
document.addEventListener("change", (e) => {
    const el = e.target;
    if (!el || !el.matches || !el.matches("input[data-capitalize]")) return;
    const name = IsleyAutocomplete.capitalizeName(el.value);
    if (name && name !== el.value) {
        el.value = name;
        el.dispatchEvent(new Event("input", { bubbles: true }));
    }
});
