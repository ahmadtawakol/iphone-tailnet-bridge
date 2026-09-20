package launchagent

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

const Label = "com.github.ahmadtawakol.iphone-tailnet-bridge"

func Install(ctx context.Context, paths config.Paths, sourceExecutable string) error {
	if err := config.EnsurePaths(paths); err != nil {
		return err
	}
	if err := installExecutable(sourceExecutable, paths.Executable); err != nil {
		return err
	}
	plist := renderPlist(paths)
	if err := writePublicFile(paths.LaunchAgent, []byte(plist), 0o644); err != nil {
		return err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_ = runLaunchctl(ctx, "bootout", domain+"/"+Label)
	for attempt := 0; attempt < 20 && serviceLoaded(ctx, domain); attempt++ {
		time.Sleep(50 * time.Millisecond)
	}
	var bootstrapError error
	for attempt := 0; attempt < 3; attempt++ {
		bootstrapError = runLaunchctl(ctx, "bootstrap", domain, paths.LaunchAgent)
		if bootstrapError == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if bootstrapError != nil {
		return bootstrapError
	}
	return runLaunchctl(ctx, "kickstart", "-k", domain+"/"+Label)
}

func serviceLoaded(ctx context.Context, domain string) bool {
	command := exec.CommandContext(ctx, "/bin/launchctl", "print", domain+"/"+Label)
	return command.Run() == nil
}

func Remove(ctx context.Context, paths config.Paths) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_ = runLaunchctl(ctx, "bootout", domain+"/"+Label)
	if err := os.Remove(paths.LaunchAgent); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Restart(ctx context.Context) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	return runLaunchctl(ctx, "kickstart", "-k", domain+"/"+Label)
}

func installExecutable(source, destination string) error {
	sourcePath, err := filepath.EvalSymlinks(source)
	if err != nil {
		sourcePath = source
	}
	destinationPath, _ := filepath.EvalSymlinks(destination)
	if sourcePath == destinationPath {
		return os.Chmod(destination, 0o755)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".iphone-tailnet-bridge-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := io.Copy(temporary, input); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o755); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, destination)
}

func renderPlist(paths config.Paths) string {
	values := map[string]string{
		"executable": paths.Executable,
		"stdout":     filepath.Join(paths.Logs, "bridge.log"),
		"stderr":     filepath.Join(paths.Logs, "bridge-error.log"),
	}
	escape := func(value string) string {
		var buffer bytes.Buffer
		_ = xml.EscapeText(&buffer, []byte(value))
		return buffer.String()
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + Label + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + escape(values["executable"]) + `</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Interactive</string>
  <key>SoftResourceLimits</key><dict><key>NumberOfFiles</key><integer>65536</integer></dict>
  <key>HardResourceLimits</key><dict><key>NumberOfFiles</key><integer>65536</integer></dict>
  <key>StandardOutPath</key><string>` + escape(values["stdout"]) + `</string>
  <key>StandardErrorPath</key><string>` + escape(values["stderr"]) + `</string>
</dict>
</plist>
`
}

func runLaunchctl(ctx context.Context, arguments ...string) error {
	command := exec.CommandContext(ctx, "/bin/launchctl", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("launchctl %s: %s", arguments[0], message)
	}
	return nil
}

func writePublicFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".launchagent-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
