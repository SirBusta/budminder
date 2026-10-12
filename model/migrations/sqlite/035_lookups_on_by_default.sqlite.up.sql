-- StrainCompass and CannaDB lookups start ON. Until now a new install had
-- no saved setting, which counts as off, so the Strains page hid "Find on
-- StrainCompass" and new users only saw "Enter manually". Only fills in a
-- setting that was never saved: anyone who turned either off keeps that.
INSERT INTO settings (name, value)
SELECT 'straincompass.enabled', '1'
 WHERE NOT EXISTS (SELECT 1 FROM settings WHERE name = 'straincompass.enabled');
INSERT INTO settings (name, value)
SELECT 'cannadb.enabled', '1'
 WHERE NOT EXISTS (SELECT 1 FROM settings WHERE name = 'cannadb.enabled');
