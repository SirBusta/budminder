package utils

// Guards against text that no locale defines. A missing key doesn't fail
// anywhere at runtime: templates render it as an empty label, and T() /
// uiMessages.t() / the pages' tr() hand back the key itself, so people see
// "api_backup_deleted" or a blank button instead of a sentence.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	// {{ .lcl.some_key }} in templates (and Go-templated inline scripts).
	lclKeyRe = regexp.MustCompile(`\.lcl\.([A-Za-z0-9_]+)`)
	// tr("some_key"), t('some_key'), uiMessages.t("some_key") in JavaScript.
	jsKeyRe = regexp.MustCompile(`\b(?:tr|t)\(\s*['"]([a-z][a-z0-9_]*)['"]`)
	// T(c, "some_key"), apiOK(c, "some_key"), apiError(c, status, "some_key").
	goKeyRe = regexp.MustCompile(`\b(?:T|api[A-Z][A-Za-z]*)\(\s*c\s*,\s*(?:[A-Za-z.]+\s*,\s*)?"([a-z][a-z0-9_]*)"`)
)

var localeLangs = []string{"en", "de", "es", "fr"}

func loadLocaleKeys(t *testing.T, lang string) map[string]struct{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("locales", lang+".yaml"))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, yaml.Unmarshal(data, &m))
	keys := make(map[string]struct{}, len(m))
	for k := range m {
		keys[k] = struct{}{}
	}
	return keys
}

// usedKeys walks root and collects the keys each matching file uses,
// mapped to the files that use them.
func usedKeys(t *testing.T, root string, want func(path string) bool, res ...*regexp.Regexp) map[string][]string {
	t.Helper()
	used := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", ".git", "node_modules", "data":
				return filepath.SkipDir
			}
			return nil
		}
		if !want(path) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, re := range res {
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				// "pl_none_" + kind: a prefix built into a key at runtime.
				if strings.HasSuffix(m[1], "_") {
					continue
				}
				used[m[1]] = append(used[m[1]], filepath.ToSlash(path))
			}
		}
		return nil
	})
	require.NoError(t, err)
	return used
}

func assertDefined(t *testing.T, en map[string]struct{}, used map[string][]string) {
	t.Helper()
	keys := make([]string, 0, len(used))
	for k := range used {
		if _, ok := en[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		assert.Failf(t, "missing translation key", "%q is used in %v but is not in locales/en.yaml", k, used[k][0])
	}
}

// Every key the pages use must exist in en.yaml.
func TestI18n_PageKeysExistInEnglish(t *testing.T) {
	t.Parallel()
	en := loadLocaleKeys(t, "en")
	assertDefined(t, en, usedKeys(t, filepath.Join("..", "web", "templates"),
		func(p string) bool { return strings.HasSuffix(p, ".html") }, lclKeyRe, jsKeyRe))
	assertDefined(t, en, usedKeys(t, filepath.Join("..", "web", "static", "js"),
		func(p string) bool { return strings.HasSuffix(p, ".js") }, lclKeyRe, jsKeyRe))
}

// Every message key the Go handlers send back must exist in en.yaml.
func TestI18n_HandlerKeysExistInEnglish(t *testing.T) {
	t.Parallel()
	en := loadLocaleKeys(t, "en")
	assertDefined(t, en, usedKeys(t, "..", func(p string) bool {
		return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")
	}, goKeyRe))
}

// The confirm dialog (ui-messages.js) builds its title and buttons from these.
func TestI18n_ConfirmDialogKeysExist(t *testing.T) {
	t.Parallel()
	for _, lang := range localeLangs {
		keys := loadLocaleKeys(t, lang)
		for _, k := range []string{"confirm", "cancel", "ok", "delete", "title_close"} {
			assert.Containsf(t, keys, k, "%s.yaml is missing %q", lang, k)
		}
	}
}

// Every language defines exactly the keys English does.
func TestI18n_LocalesHaveSameKeysAsEnglish(t *testing.T) {
	t.Parallel()
	en := loadLocaleKeys(t, "en")
	for _, lang := range localeLangs[1:] {
		other := loadLocaleKeys(t, lang)
		for k := range en {
			assert.Containsf(t, other, k, "%s.yaml is missing key %q", lang, k)
		}
		for k := range other {
			assert.Containsf(t, en, k, "%s.yaml has key %q that en.yaml lacks", lang, k)
		}
	}
}
