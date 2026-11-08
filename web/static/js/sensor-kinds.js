// What kind of reading a sensor gives, shared by the Environment page
// (sensors-page.js) and Manage sensors (sensors-manage.js): kind, port or
// not, inside or outside, an icon, and temperatures shown in the
// measurement units chosen in Settings (stored values never change).
window.SensorKinds = (function () {
    "use strict";
    // Ports: AC Infinity's, and a Vivosun device's equipment (VS.light.lv, VS.usb1.lv, VS.cure.comp: two parts after VS., never the ".out" air ones).
    const isPort = s => /^ACIP\./.test(s.type) || /^VS\.(?!.*\.out$)[A-Za-z0-9]+\.[A-Za-z]+$/.test(s.type);
    // An equipment state that is only on or off (the VCure's compressor).
    const isOnOff = s => s.unit === "on/off";
    // A light's color setting: a number, never "Off".
    const isSpectrum = s => /^VS\.light\.spec$/.test(s.type);
    // Vivosun air outside the device (the VCure's room readings, controllers' outside ones) ends in ".out".
    const isOutside = s => /outside|outdoor/i.test(s.name) || /^ACI\.\d+\.[4-7]$/.test(s.type) || /^VS\..+\.out$/.test(s.type);
    function kind(s) {
        if (isPort(s)) return "port";
        const u = (s.unit || "").toLowerCase(), ty = s.type + " " + s.name;
        if (/battery/i.test(ty)) return "other";
        if (/^VS\.waterLv$/.test(s.type) || /water level/i.test(s.name)) return "level";
        if (u === "lx" || /illuminance|\blux\b/i.test(ty)) return "light";
        if (u.includes("°") || /^(celsius|fahrenheit|degc|degf|℃|℉)$/.test(u) || /temp/i.test(ty) && !u.includes("%")) return "temp";
        if (u === "kpa" || /vpd/i.test(ty)) return "vpd";
        if (u === "ppm" || /co2|co₂/i.test(ty)) return "co2";
        if (/ms\/cm|\.ec$/i.test(u + " " + s.type)) return "ec";
        if (/soil|moist/i.test(ty)) return "soil";
        if (u === "%" || /hum/i.test(ty)) return "hum";
        return "other";
    }
    const ICONS = { light: "fa-sun", temp: "fa-temperature-half", hum: "fa-droplet", vpd: "fa-cloud", co2: "fa-smog", level: "fa-glass-water", soil: "fa-water", ec: "fa-bolt", port: "fa-plug", other: "fa-gauge" };
    // Vivosun equipment that does one job gets its icon from the reading code,
    // so a renamed light keeps its lightbulb (and the Today page's Lights on/off).
    const VS_ICONS = { "light.lv": "fa-lightbulb", "light.spec": "fa-palette", "cFan.lv": "fa-fan", "dFan.lv": "fa-fan",
        "cure.fanCir": "fa-fan", "cure.fanVap": "fa-fan", "cure.comp": "fa-snowflake", "cure.hmdf": "fa-droplet", "cure.ptc": "fa-fire" };
    // Icon for a port or equipment: by code where it says what it is, else by name
    // (AC Infinity ports, and Vivosun outlets and USB ports, which can power anything).
    function portIcon(s) {
        const key = (s.type || "").replace(/^VS\./, "");
        if (/^VS\./.test(s.type || "") && VS_ICONS[key]) return VS_ICONS[key];
        const n = (s.name || "").toLowerCase();
        const byName = nameIcon(n);
        if (byName !== "fa-plug") return byName;
        return /^VS\.usb\d\.lv$/.test(s.type || "") ? "fa-bolt" : "fa-plug";
    }
    function nameIcon(n) {
        if (/light|led|lamp|sun|bloom|veg|fc\d/.test(n)) return "fa-lightbulb";
        if (/dehum/.test(n)) return "fa-wind";
        if (/humid|mist/.test(n)) return "fa-droplet";
        if (/heat/.test(n)) return "fa-fire";
        if (/fan|exhaust|inline|blower|clip/.test(n)) return "fa-fan";
        if (/pump|water/.test(n)) return "fa-faucet";
        return "fa-plug";
    }
    const icon = s => isPort(s) ? portIcon(s) : ICONS[kind(s)];
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
    const decimals = s => ({ temp: 1, hum: 1, vpd: 2, co2: 0, level: 0, soil: 1, ec: 2, light: 0, port: 0 })[kind(s)] ?? 1;
    // A port's or equipment's value as text ("40%", Off, On); words: { on, off }.
    function portText(s, v, words) {
        if (isOnOff(s)) return v > 0 ? words.on : words.off;
        return v > 0 || isSpectrum(s) ? Math.round(v) + "%" : words.off;
    }
    // How full a port's lane is drawn, 0-100 (on/off states are 0 or 1).
    const portLevel = (s, v) => isOnOff(s) ? (v > 0 ? 100 : 0) : Math.max(0, Math.min(100, v));
    return { isPort, isOnOff, isSpectrum, portText, portLevel, isOutside, kind, icon, portIcon, display, decimals };
})();
