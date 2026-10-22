package launcher

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBox(t *testing.T) {
	t.Parallel()
	first := Box(Welcome{Port: "8080", LANAddress: "192.168.1.108", Username: "admin", Password: "budminder", FromConsole: true}, false)
	for _, want := range []string{
		"http://localhost:8080", "http://192.168.1.108:8080",
		"  user:      admin", "  password:  budminder", "To stop BudMinder",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("first-run box is missing %q:\n%s", want, first)
		}
	}
	// Every line of the box is the same width, so the right edge lines up.
	var width int
	for _, l := range strings.Split(strings.TrimSpace(first), "\n") {
		if n := utf8.RuneCountInString(l); width == 0 {
			width = n
		} else if n != width {
			t.Fatalf("ragged box: %q is %d wide, not %d", l, n, width)
		}
	}

	later := Box(Welcome{Port: "9000"}, false)
	if strings.Contains(later, "password") || strings.Contains(later, "phone") || strings.Contains(later, "Ctrl+C") {
		t.Errorf("after the password is changed (no LAN address, no terminal) the box shows too much:\n%s", later)
	}
	if !strings.Contains(later, "http://localhost:9000") {
		t.Errorf("box uses the wrong port:\n%s", later)
	}
}

func TestDesktopEntry(t *testing.T) {
	t.Parallel()
	e := DesktopEntry("/home/dave/My Apps/budminder-linux-amd64", "/home/dave/My Apps/data/budminder-icon.png")
	for _, want := range []string{
		`Exec="/home/dave/My Apps/budminder-linux-amd64" --open`,
		`Exec="/home/dave/My Apps/budminder-linux-amd64" --stop`,
		"Icon=/home/dave/My Apps/data/budminder-icon.png", "Terminal=false", "Name=Stop BudMinder",
	} {
		if !strings.Contains(e, want) {
			t.Errorf("desktop entry is missing %q:\n%s", want, e)
		}
	}
	if got := quoteExec(`/a "b"/$c`); got != `"/a \\"b\\"/\\$c"` {
		t.Errorf("quoteExec = %q", got)
	}
}
