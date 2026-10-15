package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls  []string
	print  string
	loaded bool
}

func (f *fakeRunner) Run(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "print":
		if !f.loaded {
			return "Could not find service", errors.New("exit status 113")
		}
		return f.print, nil
	case "bootout":
		if !f.loaded {
			return "", errors.New("exit status 3")
		}
		f.loaded = false
	case "bootstrap":
		f.loaded = true
	}
	return "", nil
}

func opts(t *testing.T, r Runner) Options {
	t.Helper()
	home := t.TempDir()
	return Options{Executable: filepath.Join(home, "go", "bin", "devboard"), Home: home, UID: 501, Runner: r}
}

func TestRenderPlist(t *testing.T) {
	o := opts(t, nil)
	o.ConfigOverride = "/x/a&b.toml"
	p, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>local.devboard</string>",
		"<string>" + o.Executable + "</string>",
		"<string>serve</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<true/>",
		"<key>WorkingDirectory</key>\n\t<string>" + o.Home + "</string>",
		filepath.Join(o.Home, "Library", "Logs", "devboard", "devboard.log"),
		"<key>DEVBOARD_CONFIG</key>\n\t\t<string>/x/a&amp;b.toml</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q:\n%s", want, p)
		}
	}
	if strings.Contains(strings.ToUpper(p), "TOKEN") {
		t.Errorf("plist must not carry tokens:\n%s", p)
	}
}

func TestRenderOmitsEnvWithoutOverride(t *testing.T) {
	p, err := Render(opts(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p, "EnvironmentVariables") {
		t.Errorf("unexpected env block:\n%s", p)
	}
}

func TestRenderRefusesDocuments(t *testing.T) {
	o := opts(t, nil)
	o.Executable = filepath.Join(o.Home, "Documents", "devboard", "devboard")
	if _, err := Render(o); err == nil || !strings.Contains(err.Error(), "make install") {
		t.Fatalf("want Documents refusal with make install hint, got %v", err)
	}
}

func TestInstallFresh(t *testing.T) {
	r := &fakeRunner{}
	o := opts(t, r)
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PlistPath(o.Home)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(o.Home, "Library", "Logs", "devboard")); err != nil {
		t.Fatal(err)
	}
	want := "bootstrap gui/501 " + PlistPath(o.Home)
	if got := r.calls[len(r.calls)-1]; got != want {
		t.Fatalf("last call %q, want %q (all %v)", got, want, r.calls)
	}
}

func TestInstallReloadsWhenLoaded(t *testing.T) {
	r := &fakeRunner{loaded: true}
	o := opts(t, r)
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.calls, "|")
	if !strings.Contains(joined, "bootout gui/501/local.devboard|bootstrap gui/501 ") {
		t.Fatalf("want bootout then bootstrap, got %v", r.calls)
	}
}

func TestUninstall(t *testing.T) {
	r := &fakeRunner{}
	o := opts(t, r)
	if err := Install(o); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(o); err != nil {
		t.Fatal(err)
	}
	if r.loaded {
		t.Fatal("still loaded")
	}
	if _, err := os.Stat(PlistPath(o.Home)); !os.IsNotExist(err) {
		t.Fatalf("plist not removed: %v", err)
	}
	if err := Uninstall(o); err != nil {
		t.Fatalf("second uninstall should be a no-op, got %v", err)
	}
}

func TestStatus(t *testing.T) {
	r := &fakeRunner{loaded: true, print: "gui/501/local.devboard = {\n\tpath = /p\n\tstate = running\n\tpid = 4242\n\tendpoints = {\n\t\tstate = active\n\t}\n}\n"}
	o := opts(t, r)
	s := Inspect(o)
	if s.Installed || !s.Loaded || s.State != "running" || s.PID != 4242 {
		t.Fatalf("%+v", s)
	}
	r.loaded = false
	if s := Inspect(o); s.Loaded || s.PID != 0 {
		t.Fatalf("%+v", s)
	}
}
