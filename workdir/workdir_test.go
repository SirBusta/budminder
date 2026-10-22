package workdir

import (
	"path/filepath"
	"testing"
)

func TestChoose(t *testing.T) {
	t.Parallel()
	prog := filepath.Join("opt", "budminder")
	home := filepath.Join("home", "dave")
	none := func(string) bool { return false }
	dbIn := func(dir string) func(string) bool {
		return func(p string) bool { return p == filepath.Join(dir, "data", "isley.db") }
	}

	cases := []struct {
		name      string
		exeDir    string
		cwd       string
		dbFileSet bool
		exists    func(string) bool
		wantDir   string
		wantMoved bool
	}{
		{"started elsewhere: run next to the program", prog, home, false, none, prog, true},
		{"started in its own folder", prog, prog, false, none, prog, false},
		{"data folder chosen by the user", prog, home, true, none, home, false},
		{"an install that always ran from elsewhere keeps its data", prog, home, false, dbIn(home), home, false},
		{"data next to the program wins over data here", prog, home, false, func(p string) bool { return dbIn(home)(p) || dbIn(prog)(p) }, prog, true},
		{"go run build", filepath.Join("tmp", "go-build123", "b001", "exe"), home, false, none, home, false},
	}
	for _, c := range cases {
		got := Choose(c.exeDir, c.cwd, c.dbFileSet, c.exists)
		if got.Dir != c.wantDir || got.Moved != c.wantMoved {
			t.Errorf("%s: got %+v, want dir %q moved %v", c.name, got, c.wantDir, c.wantMoved)
		}
	}
}
