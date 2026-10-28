document.addEventListener("DOMContentLoaded", () => {
    const addPlantForm = document.getElementById("addPlantForm");
    const zoneSelect = document.getElementById("zoneSelect");
    const newZoneInput = document.getElementById("newZoneInput");
    const strainSelect = document.getElementById("strainSelect");
    const parentPlantDropdown = document.getElementById("parentPlantDropdown");
    const parentPlantSelect = document.getElementById("parentPlantSelect");
    const isClone = document.getElementById("isClone");

    // Start date: the day is chosen, the time of day is recorded silently.
    const pad = n => String(n).padStart(2, "0");
    const now = new Date();
    const startDate = document.getElementById("startDate");
    startDate.max = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
    if (!startDate.value) startDate.value = startDate.max;
    function withTimeNow(day) {
        const t = new Date();
        return `${day}T${pad(t.getHours())}:${pad(t.getMinutes())}:${pad(t.getSeconds())}`;
    }

    // Initialize autocomplete on strain and zone selects
    const zoneAC = new IsleyAutocomplete(zoneSelect, {
        placeholder: "Type to search zones...",
    });
    // Strains are added on the Strains page (lookups, seeds), not here: a name
    // that isn't there gets a note with a link, plus "Did you mean" for typos.
    const strainAC = StrainFirst.strainField(strainSelect);
    StrainFirst.wire(addPlantForm, "/plant/new");

    // Show/Hide New Zone Input
    zoneSelect.addEventListener("change", () => {
        newZoneInput.classList.toggle("d-none", zoneSelect.value !== "new");
    });

    // "Decrease seed count by 1" only means something for a strain with seeds
    // in the Seed Vault: hidden (and unticked) for any other.
    function syncSeedBox() {
        const opt = strainSelect.selectedOptions[0];
        const seeds = opt ? Number(opt.dataset.seeds || 0) : 0;
        const hasSeeds = seeds > 0;
        document.getElementById("seedBox").classList.toggle("d-none", !hasSeeds);
        if (!hasSeeds) document.getElementById("decrementSeedCount").checked = false;
        const left = document.getElementById("seedLeft");
        if (hasSeeds && left) {
            left.textContent = left.dataset.text
                .replace("{name}", opt.dataset.name || "")
                .replace("{n}", seeds)
                .replace("{m}", seeds - 1);
        }
    }

    // The summary card beside the form.
    function syncSummary() {
        const label = sel => {
            const o = sel.selectedOptions[0];
            return o && o.value && o.value !== "new" ? (o.dataset.name || o.textContent.trim()) : "–";
        };
        const name = document.getElementById("plantName").value.trim();
        const sumName = document.getElementById("sumName");
        if (sumName) sumName.textContent = name || sumName.dataset.empty;
        const zone = zoneSelect.value === "new" ? document.getElementById("newZoneName").value.trim() || "–" : label(zoneSelect);
        const set = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = v; };
        set("sumStrain", label(strainSelect));
        set("sumZone", zone);
        set("sumStage", label(document.getElementById("statusSelect")));
    }
    addPlantForm.addEventListener("input", syncSummary);
    addPlantForm.addEventListener("change", syncSummary);

    strainSelect.addEventListener("change", () => {
        syncSeedBox();
        formHelpers.clearFieldError(strainAC.input);
        // Load parent plants for selected strain
        if (strainSelect.value) {
            fetch(`/plants/by-strain/${strainSelect.value}`)
                .then(r => r.json())
                .then(plants => {
                    parentPlantSelect.innerHTML = '<option value="0">None</option>';
                    plants.forEach(p => {
                        const opt = document.createElement("option");
                        opt.value = p.id;
                        opt.textContent = p.name;
                        parentPlantSelect.appendChild(opt);
                    });
                })
                .catch(() => {});
        }
    });

    // Show/Hide Parent Plant Dropdown
    isClone.addEventListener("change", () => {
        parentPlantDropdown.classList.toggle("d-none", !isClone.checked);
    });

    // Form submission
    addPlantForm.addEventListener("submit", (e) => {
        e.preventDefault();

        formHelpers.clearFieldError(zoneAC.input);
        formHelpers.clearFieldError(strainAC.input);
        strainAC.commit();

        // The autocomplete only sets zoneSelect/strainSelect's value when an
        // existing item or "Add New" is explicitly chosen from its dropdown.
        // Typing a name and clicking away without picking anything leaves it
        // blank, which previously reached the server as neither an existing
        // ID nor new-item data and crashed the handler.
        let hasError = false;
        if (zoneSelect.value === "") {
            formHelpers.setFieldError(zoneAC.input, uiMessages.t('api_zone_required'));
            hasError = true;
        }
        if (strainSelect.value === "") {
            const typed = strainAC.input.value.trim();
            if (typed) {
                StrainFirst.showError(strainAC, typed);
            } else {
                formHelpers.setFieldError(strainAC.input, uiMessages.t('api_strain_required'));
            }
            hasError = true;
        }
        if (hasError) return;

        const submitBtn = addPlantForm.querySelector('button[type="submit"]');
        submitBtn.classList.add("is-loading");
        submitBtn.disabled = true;

        const payload = {
            name: document.getElementById("plantName").value,
            zone_id: zoneSelect.value === "new" ? null : parseInt(zoneSelect.value, 10),
            new_zone: zoneSelect.value === "new" ? document.getElementById("newZoneName").value : null,
            strain_id: parseInt(strainSelect.value, 10),
            status_id: parseInt(document.getElementById("statusSelect").value, 10),
            date: withTimeNow(document.getElementById("startDate").value),
            clone: isClone.checked ? 1 : 0,
            parent_id: parseInt(parentPlantSelect.value, 10),
            decrement_seed_count: document.getElementById("decrementSeedCount").checked,
        };

        fetch("/plants", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
        .then(response => {
            if (!response.ok) throw new Error("Failed to add plant");
            return response.json();
        })
        .then(data => {
            // Redirect to the new plant's detail page if possible
            if (data && data.id) {
                window.location.href = `/plant/${data.id}`;
            } else {
                window.location.href = "/plants";
            }
        })
        .catch(error => {
            submitBtn.classList.remove("is-loading");
            submitBtn.disabled = false;
            uiMessages.showToast(uiMessages.t('failed_to_add_plant') || 'Failed to add plant. Try again.', 'danger');
        });
    });

    // "Start a grow" from the Strains page: ?strain_id=&name=&use_seed=1
    // pre-fill the strain, the plant's name and "use one seed".
    // Back from the Strains page (?strain_id=&resume=1): what was filled in
    // before is put back too.
    const params = new URLSearchParams(location.search);
    const draft = params.get("resume") === "1" ? StrainFirst.take("/plant/new") : null;
    if (draft) {
        StrainFirst.restore(draft);
        zoneAC.setValue(zoneSelect.value);
    }
    if (params.get("strain_id")) {
        strainAC.setValue(params.get("strain_id"));
        // The strain is chosen already: the "add the strain first" note isn't needed.
        document.getElementById("strainFirstNote")?.classList.add("d-none");
    }
    if (params.get("name")) document.getElementById("plantName").value = params.get("name");
    if (params.get("use_seed") === "1") document.getElementById("decrementSeedCount").checked = true;
    syncSeedBox();
    syncSummary();
    if (draft && strainSelect.value) {
        const label = strainAC.input.value;
        uiMessages._ready().then(() => {
            document.getElementById("strainFirstBackText").innerHTML = StrainFirst.withName("sf_back_restored", label);
            document.getElementById("strainFirstBack").classList.remove("d-none");
        });
    }
});
