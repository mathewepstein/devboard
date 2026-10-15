// Package agent manages the launchd LaunchAgent that keeps serve running.
package agent

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

const Label = "local.devboard"

type Runner interface {
	Run(args ...string) (string, error)
}

type Launchctl struct{}

func (Launchctl) Run(args ...string) (string, error) {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	return string(out), err
}

type Options struct {
	Executable     string
	Home           string
	ConfigOverride string
	UID            int
	Runner         Runner
}

type Status struct {
	Installed bool
	Loaded    bool
	State     string
	PID       int
}

func PlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

func LogPath(home string) string {
	return filepath.Join(home, "Library", "Logs", "devboard", "devboard.log")
}

func (o Options) target() string { return "gui/" + strconv.Itoa(o.UID) + "/" + Label }
func (o Options) domain() string { return "gui/" + strconv.Itoa(o.UID) }

var plistTmpl = template.Must(template.New("plist").Funcs(template.FuncMap{"x": xmlEscape}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{x .Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{x .Exe}}</string>
		<string>serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>WorkingDirectory</key>
	<string>{{x .Home}}</string>
	<key>StandardOutPath</key>
	<string>{{x .Log}}</string>
	<key>StandardErrorPath</key>
	<string>{{x .Log}}</string>
{{- if .Config}}
	<key>EnvironmentVariables</key>
	<dict>
		<key>DEVBOARD_CONFIG</key>
		<string>{{x .Config}}</string>
	</dict>
{{- end}}
</dict>
</plist>
`))

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// launchd cannot exec from ~/Documents (TCC).
func Render(o Options) (string, error) {
	docs := filepath.Join(o.Home, "Documents") + string(filepath.Separator)
	if strings.HasPrefix(o.Executable, docs) {
		return "", fmt.Errorf("binary %s is under ~/Documents, which launchd cannot run; run `make install` and use ~/go/bin/devboard", o.Executable)
	}
	var b bytes.Buffer
	err := plistTmpl.Execute(&b, map[string]string{
		"Label": Label, "Exe": o.Executable, "Home": o.Home, "Log": LogPath(o.Home), "Config": o.ConfigOverride,
	})
	return b.String(), err
}

func Install(o Options) error {
	plist, err := Render(o)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(LogPath(o.Home)), 0o700); err != nil {
		return err
	}
	path := PlistPath(o.Home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	if Inspect(o).Loaded {
		if out, err := o.Runner.Run("bootout", o.target()); err != nil {
			return fmt.Errorf("launchctl bootout: %w: %s", err, strings.TrimSpace(out))
		}
	}
	if out, err := o.Runner.Run("bootstrap", o.domain(), path); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func Uninstall(o Options) error {
	if Inspect(o).Loaded {
		if out, err := o.Runner.Run("bootout", o.target()); err != nil {
			return fmt.Errorf("launchctl bootout: %w: %s", err, strings.TrimSpace(out))
		}
	}
	if err := os.Remove(PlistPath(o.Home)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func Inspect(o Options) Status {
	var s Status
	if _, err := os.Stat(PlistPath(o.Home)); err == nil {
		s.Installed = true
	}
	out, err := o.Runner.Run("print", o.target())
	if err != nil {
		return s
	}
	s.Loaded = true
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "\t\t") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimSpace(line), " = ")
		if !ok {
			continue
		}
		switch k {
		case "state":
			s.State = v
		case "pid":
			s.PID, _ = strconv.Atoi(v)
		}
	}
	return s
}
