document.addEventListener("DOMContentLoaded", () => {
    // Start date: only the day is shown; the stored time of day is kept.
    const startInput = document.getElementById("startDate");
    const storedStart = (startInput && startInput.dataset.stored) || "";
    if (startInput && !startInput.value) startInput.value = storedStart.slice(0, 10);
    function startDateValue() {
        const day = startInput.value;
        if (!day) return "";
        const time = storedStart.length >= 16 ? storedStart.slice(10) : "T00:00:00";
        return day + (time.length === 6 ? time + ":00" : time);
    }

    const editPlantForm = document.getElementById("editPlantForm");
    const zoneSelect = document.getElementById("zoneSelect");
    const newZoneInput = document.getElementById("newZoneInput");
    const strainSelect = document.getElementById("strainSelect");
    const deletePlantButton = document.getElementById("deletePlantButton");
    const plantId = document.getElementById("plantId").value;

    // Initialize autocomplete on strain and zone selects
    const zoneAC = new IsleyAutocomplete(zoneSelect, {
        placeholder: "Type to search zones...",
    });
    // Strains are added on the Strains page; see strain-first.js.
    const strainAC = StrainFirst.strainField(strainSelect);
    const editPath = `/plant/${plantId}/edit`;
    StrainFirst.wire(editPlantForm, editPath, () => document.getElementById("plantName").value);

    // Show/Hide New Zone Input
    zoneSelect.addEventListener("change", () => {
        newZoneInput.classList.toggle("d-none", zoneSelect.value !== "new");
    });

    // Form submission
    editPlantForm.addEventListener("submit", (e) => {
        e.preventDefault();

        strainAC.commit();
        if (!strainSelect.value) {
            const typed = strainAC.input.value.trim();
            if (typed) StrainFirst.showError(strainAC, typed);
            else formHelpers.setFieldError(strainAC.input, uiMessages.t('api_strain_required'));
            return;
        }

        const submitBtn = editPlantForm.querySelector('button[type="submit"]');
        submitBtn.classList.add("is-loading");
        submitBtn.disabled = true;

        const zoneVal = zoneSelect.value;
        const strainVal = strainSelect.value;
        const newZoneName = document.getElementById("newZoneName");

        const payload = {
            plant_id: parseInt(plantId, 10),
            plant_name: document.getElementById("plantName").value,
            plant_description: document.getElementById("plantDescription").value,
            zone_id: zoneVal && zoneVal !== "new" ? parseInt(zoneVal, 10) : null,
            new_zone: zoneVal === "new" ? newZoneName.value : "",
            strain_id: parseInt(strainVal, 10),
            clone: document.getElementById("cloneCheckbox").checked,
            start_date: startDateValue(),
            harvest_weight: parseFloat(document.getElementById("harvestWeight").value) || 0,
        };

        fetch("/plant", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
        .then(response => {
            if (!response.ok) throw new Error("Failed to update plant");
            return response.json();
        })
        .then(() => {
            if (plantId && /^\d+$/.test(String(plantId))) {
                window.location.href = `/plant/${plantId}`;
            }
        })
        .catch(error => {
            submitBtn.classList.remove("is-loading");
            submitBtn.disabled = false;
            console.error("Error:", error);
            uiMessages.showToast(uiMessages.t('failed_save_changes') || 'Failed to save changes', 'danger');
        });
    });

    // Back from the Strains page (?strain_id=&resume=1): the new strain is
    // chosen and any other changes made before leaving are put back.
    const params = new URLSearchParams(location.search);
    const draft = params.get("resume") === "1" ? StrainFirst.take(editPath) : null;
    if (draft) {
        StrainFirst.restore(draft);
        zoneAC.setValue(zoneSelect.value);
        if (params.get("strain_id")) strainAC.setValue(params.get("strain_id"));
        const label = strainAC.input.value;
        uiMessages._ready().then(() => {
            document.getElementById("strainFirstBackText").innerHTML = StrainFirst.withName("sf_back_restored_edit", label);
            document.getElementById("strainFirstBack").classList.remove("d-none");
        });
    }

    // Delete plant
    if (deletePlantButton) {
        deletePlantButton.addEventListener("click", () => {
            uiMessages.showConfirm(uiMessages.t('confirm_delete_plant'), { title: uiMessages.t('delete_plant'), danger: true })
                .then(ok => { if (ok) doDelete(); });
        });
    }

    function doDelete() {
        fetch(`/plant/delete/${plantId}`, { method: "DELETE" })
            .then(response => {
                if (!response.ok) throw new Error("Failed to delete plant");
                window.location.href = "/plants";
            })
            .catch(error => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_delete_plant') || 'Failed to delete plant', 'danger');
            });
    }
});