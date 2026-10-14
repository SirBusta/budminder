// What kind of reading a sensor gives, shared by the Environment page
// (sensors-page.js) and Manage sensors (sensors-manage.js): kind, port or
// not, inside or outside, an icon, and temperatures shown in the
// measurement units chosen in Settings (stored values never change).
window.SensorKinds = (function () {
    "use strict";
    const isPort = s => /^ACIP\./.test(s.type);
    const isOutside = s => /outside|outdoor/i.test(s.name) || /^ACI\.\d+\.[4-7]$/.test(s.type);
    function kind(s) {
        if (isPort(s)) return "port";
        const u = (s.unit || "").toLowerCase(), ty = s.type + " " + s.name;
        if (/battery/i.test(ty)) return "other";
        if (u === "lx" || /illuminance|\blux\b/i.test(ty)) return "light";
        if (u.includes("°") || /temp/i.test(ty) && !u.includes("%")) return "temp";
        if (u === "kpa" || /vpd/i.test(ty)) return "vpd";
        if (u === "ppm" || /co2|co₂/i.test(ty)) return "co2";
        if (/ms\/cm|\.ec$/i.test(u + " " + s.type)) return "ec";
        if (/soil|moist/i.test(ty)) return "soil";
        if (u === "%" || /hum/i.test(ty)) return "hum";
        return "other";
    }
    const ICONS = { light: "fa-sun", temp: "fa-temperature-half", hum: "fa-droplet", vpd: "fa-cloud", co2: "fa-smog", soil: "fa-water", ec: "fa-bolt", port: "fa-plug", other: "fa-gauge" };
    function portIcon(name) {
        const n = (name || "").toLowerCase();
        if (/light|led|lamp|sun|bloom|veg|fc\d/.test(n)) return "fa-lightbulb";
        if (/dehum/.test(n)) return "fa-wind";
        if (/humid|mist/.test(n)) return "fa-droplet";
        if (/heat/.test(n)) return "fa-fire";
        if (/fan|exhaust|inline|blower|clip/.test(n)) return "fa-fan";
        if (/pump|water/.test(n)) return "fa-faucet";
        return "fa-plug";
    }
    const icon = s => isPort(s) ? portIcon(s.name) : ICONS[kind(s)];
    // display returns the unit to show and a function converting a stored
    // value into it: °C <-> °F follows Settings → Measurement units.
    function display(s, units) {
        let unit = s.unit || "", conv = v => v;
        if (kind(s) === "temp") {
            const imperial = units !== "metric";
            if (imperial && /c/i.test(unit)) { conv = v => v * 9 / 5 + 32; unit = "°F"; }
            else if (!imperial && /f/i.test(unit)) { conv = v => (v - 32) * 5 / 9; unit = "°C"; }
        }
        return { unit, conv };
    }
    const decimals = s => ({ temp: 1, hum: 1, vpd: 2, co2: 0, soil: 1, ec: 2, light: 0, port: 0 })[kind(s)] ?? 1;
    return { isPort, isOutside, kind, icon, portIcon, display, decimals };
})();
