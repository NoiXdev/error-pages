package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/NoiXdev/error-pages/web"
)

// The repo has no JS test runner, so this is a source-level check: language
// lookups in i18n.js must be own-property checks, or a stored "__proto__"
// would pass as an enabled language.
func TestI18nLanguageLookupsAreOwnPropertyChecks(t *testing.T) {
	b, err := web.FS.ReadFile("static/js/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	// texts[x] may only be read right after the has() guard; as an
	// existence check it would accept "__proto__".
	guarded := strings.ReplaceAll(src, "if (!has(lang)) return null;\n    var m = texts[lang];", "")
	bare := regexp.MustCompile(`\btexts\[[^\]]+\]`)
	for _, m := range bare.FindAllString(guarded, -1) {
		t.Errorf("unguarded lookup %s: use has() / Object.prototype.hasOwnProperty.call(texts, ...)", m)
	}
	if !strings.Contains(src, "Object.prototype.hasOwnProperty.call(texts,") {
		t.Error("no own-property guard on texts")
	}
	// A stored language that is not enabled is removed.
	if !strings.Contains(src, "removeItem(STORE)") {
		t.Error("an invalid stored language is not removed")
	}
}
