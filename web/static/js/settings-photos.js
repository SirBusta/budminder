// Settings > Photos (settings.html, handlers/photo_settings_http.go): Keep
// originals or Compression Options and the quality (saved straight away),
// the estimate for photos already in BudMinder, and Compress existing photos
// with its confirm dialog, progress, pause/resume and the Done panel with
// the backup folder. The line under the top menu (photo-job.js) polls the
// run's status and sends it here as "bm-photo-job" events.
(() => {
    "use strict";
    const card = document.getElementById("pcCard");
    const data = document.getElementById("pcText");
    if (!card || !data) return;
    const T = JSON.parse(data.textContent || "{}");
    const $ = id => document.getElementById(id);
    const fill = (s, v) => Object.keys(v).reduce((t, k) => t.split("{" + k + "}").join(v[k]), String(s || ""));
    const lang = document.documentElement.lang || undefined;
    const num = (n, d) => n.toLocaleString(lang, { minimumFractionDigits: d, maximumFractionDigits: d });
    function size(n) {
        if (n >= 1e9) return num(n / 1e9, 2) + " GB";
        if (n >= 1e7) return num(n / 1e6, 0) + " MB";
        if (n >= 1e6) return num(n / 1e6, 1) + " MB";
        return num(Math.max(0, n) / 1e3, 0) + " KB";
    }
    // Share of the original size at each quality (Go's encoder on a 4.86 MB
    // phone photo), for the example under Quality and the picture.
    const PCT = { 90: 80, 85: 66, 80: 57, 75: 51, 70: 46, 65: 43, 60: 40, 55: 38, 50: 36 };
    const EXAMPLE = 4.86e6;

    const mode = $("pcMode"), q = $("pcQ"), more = $("pcMore");
    let status = { state: card.dataset.state || "" }, estimate = null, estFor = 0;

    async function post(url, body, method = "POST") {
        const r = await fetch(url, {
            method,
            headers: { "Content-Type": "application/json", Accept: "application/json" },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) {
            if (!d.demo) uiMessages.showToast(d.error || T.failed, "danger");
            throw new Error(d.error || r.status);
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

    function compressOn() { return mode.querySelector(".on")?.dataset.v === "compress"; }

    function applyMode() {
        const on = compressOn();
        more.classList.toggle("off", !on);
        more.inert = !on;
        const p = PCT[q.value] || 51;
        $("pcEst").textContent = fill(T.est, { p: Math.round((100 - p) / 5) * 5, a: size(EXAMPLE), b: size(EXAMPLE * p / 100) });
        $("pcAbOrig").textContent = fill(T.abOrig, { s: size(EXAMPLE) });
        $("pcAbComp").textContent = fill(T.abComp, { s: size(EXAMPLE * p / 100) });
    }

    mode.addEventListener("click", e => {
        const b = e.target.closest("button");
        if (!b || b.classList.contains("on") || locked()) return;
        const before = mode.querySelector(".on");
        mode.querySelectorAll("button").forEach(x => x.classList.toggle("on", x === b));
        applyMode();
        post("/settings/photos", { compress: b.dataset.v === "compress" }).then(flash).catch(() => {
            mode.querySelectorAll("button").forEach(x => x.classList.toggle("on", x === before));
            applyMode();
        });
    });
    let lastQ = q.value;
    q.addEventListener("change", () => {
        applyMode();
        post("/settings/photos", { quality: +q.value }).then(() => { lastQ = q.value; flash(); loadEstimate(); })
            .catch(() => { q.value = lastQ; applyMode(); });
    });

    function locked() { return status.state === "running" || status.state === "paused"; }

    // ---------- Existing photos ----------
    function loadEstimate() {
        const at = +q.value;
        estFor = at;
        ["pcCount", "pcBytes", "pcSave"].forEach(id => { $(id).textContent = "…"; });
        $("pcGo").disabled = true;
        fetch("/settings/photos/existing?quality=" + at, { cache: "no-store", headers: { Accept: "application/json" } })
            .then(r => r.ok ? r.json() : Promise.reject())
            .then(e => {
                if (estFor !== at) return;
                estimate = e;
                $("pcCount").textContent = num(e.count, 0);
                $("pcBytes").textContent = size(e.bytes);
                $("pcSave").textContent = "~" + size(e.estimated_saving);
                render();
            }).catch(() => {});
    }

    function show(id, on) { $(id).classList.toggle("d-none", !on); }

    function render() {
        const s = status, st = s.state || "";
        const run = st === "running", paused = st === "paused";
        const none = !!estimate && estimate.count === 0;
        show("pcStats", !run && !paused && !none);
        show("pcNone", !run && !paused && none);
        show("pcGo", !run && !paused);
        $("pcGo").disabled = !estimate || none;
        show("pcStop", run);
        show("pcProg", run);
        show("pcPause", paused);
        show("pcPauseBtns", paused);
        show("pcLocked", run || paused);
        q.disabled = run || paused;
        mode.querySelectorAll("button").forEach(b => { b.disabled = run || paused; });

        const bar = $("pcBar");
        if (run && s.phase === "backup") {
            const p = s.backup_total ? s.backup_done / s.backup_total * 100 : 0;
            bar.style.width = p + "%";
            bar.classList.add("bg-secondary");
            $("pcProgTxt").textContent = fill(T.progBackup, { p: Math.round(p) });
        } else if (run) {
            bar.style.width = (s.total ? s.done / s.total * 100 : 0) + "%";
            bar.classList.remove("bg-secondary");
            let t = fill(T.progCompress, { n: num(Math.min(s.done + 1, s.total), 0), total: num(s.total, 0), saved: size(s.saved_bytes) });
            if (s.seconds_left > 0) t += fill(T.progLeft, { m: Math.max(1, Math.ceil(s.seconds_left / 60)) });
            $("pcProgTxt").textContent = t;
        }
        if (paused) {
            $("pcPauseWhy").textContent = s.error ? fill(T.whyError, { error: s.error }) : (s.interrupted ? T.whyClosed : T.whyStopped);
            $("pcPauseTxt").textContent = fill(T.pauseTxt, { done: num(s.done, 0), total: num(s.total, 0), saved: size(s.saved_bytes), left: num(s.left, 0) });
            $("pcPauseBar").style.width = (s.total ? s.done / s.total * 100 : 0) + "%";
        }

        // Done panel: for a finished run, while its backup is kept (7 days).
        const recent = st === "finished" && s.finished_at && (Date.now() - new Date(s.finished_at)) < 7 * 86400e3;
        show("pcDone", recent);
        if (recent) {
            $("pcDoneTitle").textContent = fill(T.doneTitle, { c: num(s.compressed, 0), k: num(s.kept, 0) });
            $("pcDoneSaved").textContent = fill(T.doneSaved, { saved: size(s.saved_bytes), before: size(s.bytes_before), after: size(s.bytes_before - s.saved_bytes) });
            const hasBackup = !!s.backup_path && !s.backup_gone;
            show("pcDoneBackup", hasBackup);
            show("pcDoneGone", !hasBackup);
            $("pcDoneGone").textContent = s.backup_path ? T.gone : T.noBackup;
            if (hasBackup) {
                $("pcPath").textContent = s.backup_path;
                show("pcOpen", !!s.can_open);
                const at = new Date(s.deletes_at), left = at - Date.now();
                const days = Math.floor(left / 86400e3);
                const when = days >= 2 ? fill(T.days, { n: days }) : days === 1 ? T.dayOne : fill(T.hours, { n: Math.max(1, Math.ceil(left / 3600e3)) });
                $("pcTimer").textContent = fill(T.timer, { days: when, date: at.toLocaleDateString(lang, { month: "short", day: "numeric" }) });
            }
        }
    }

    function update(s) {
        const was = status.state;
        status = s || {};
        render();
        if (was && was !== status.state && status.state !== "running") loadEstimate();
    }
    document.addEventListener("bm-photo-job", e => update(e.detail));
    function watch(s) {
        update(s);
        if (window.bmPhotoJob) { window.bmPhotoJob.show(s); window.bmPhotoJob.watch(); }
    }

    // ---------- Confirm dialog ----------
    const dlg = $("pcDlg");
    document.body.appendChild(dlg); // out of the section, which may be hidden
    const bk = $("pcDlgBk");
    function dlgLabels() {
        const n = estimate ? estimate.count : 0;
        bk.closest(".dlg-bk").querySelector("small").style.opacity = bk.checked ? 1 : .5;
        $("pcDlgYes").querySelector("span").textContent = bk.checked ? fill(n === 1 ? T.dlgYesOne : T.dlgYes, { n: num(n, 0) }) : T.dlgNoBk;
    }
    $("pcGo").addEventListener("click", () => {
        if (!estimate || !estimate.count) return;
        const n = estimate.count;
        $("pcDlgT").textContent = fill(n === 1 ? T.dlgTitleOne : T.dlgTitle, { n: num(n, 0) });
        $("pcDlgQ").textContent = fill(T.dlgQ, { q: q.value, s: size(estimate.estimated_saving) });
        $("pcDlgBkTxt").textContent = fill(T.dlgBk, { s: size(estimate.bytes) });
        bk.checked = true;
        dlgLabels();
        bootstrap.Modal.getOrCreateInstance(dlg).show();
    });
    bk.addEventListener("change", dlgLabels);
    $("pcDlgYes").addEventListener("click", () => {
        $("pcDlgYes").disabled = true;
        post("/settings/photos/existing/start", { backup: bk.checked })
            .then(s => { bootstrap.Modal.getOrCreateInstance(dlg).hide(); watch(s); })
            .catch(() => bootstrap.Modal.getOrCreateInstance(dlg).hide())
            .finally(() => { $("pcDlgYes").disabled = false; });
    });

    // ---------- Run buttons ----------
    function act(btn, url, then) {
        btn.addEventListener("click", () => {
            btn.disabled = true;
            post(url).then(then).catch(() => {}).finally(() => { btn.disabled = false; });
        });
    }
    act($("pcStop"), "/settings/photos/existing/stop", watch);
    act($("pcResume"), "/settings/photos/existing/resume", watch);
    act($("pcFinish"), "/settings/photos/existing/finish", s => { watch(s); loadEstimate(); });
    $("pcDel").addEventListener("click", () => {
        uiMessages.showConfirm(T.confirmDel, { danger: true }).then(ok => {
            if (ok) post("/settings/photos/existing/backup", undefined, "DELETE").then(update).catch(() => {});
        });
    });
    $("pcOpen").addEventListener("click", () => {
        post("/settings/photos/existing/backup/open").catch(() => {});
    });
    $("pcCopy").addEventListener("click", () => {
        const text = $("pcPath").textContent;
        const done = () => uiMessages.showToast(T.copied, "success");
        if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text).then(done).catch(() => {});
            return;
        }
        // Plain http on the local network: no clipboard API.
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.setAttribute("readonly", "");
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        try { if (document.execCommand("copy")) done(); } catch (e) { /* not supported */ }
        ta.remove();
    });

    applyMode();
    fetch("/settings/photos/existing/status", { cache: "no-store", headers: { Accept: "application/json" } })
        .then(r => r.ok ? r.json() : Promise.reject()).then(s => { status = s; render(); }).catch(() => {});
    loadEstimate();
})();
