package computeruse

import (
	"strings"
	"testing"
)

func TestCuaLinuxEnvironmentKeepsDesktopTransportsWithoutSecretsOrOverrides(t *testing.T) {
	for key, value := range map[string]string{
		"DISPLAY": ":7", "WAYLAND_DISPLAY": "wayland-7", "XAUTHORITY": "/run/user/1000/fixture.auth", "DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus", "XDG_RUNTIME_DIR": "/run/user/1000", "XDG_SESSION_TYPE": "wayland", "CUA_DRIVER_RS_ENABLE_WAYLAND": "1",
		"OPENAI_API_KEY": "fake-secret", "REASONIX_DESKTOP_BRIDGE_TOKEN": "fake-secret", "REASONIX_HOME": "/private-profile", "CUA_DRIVER_RS_HOME": "/override", "LD_PRELOAD": "/untrusted.so", "CUA_TELEMETRY_ENABLED": "1", "PATH": "/untrusted",
	} {
		t.Setenv(key, value)
	}
	values := func(platform string) map[string]string {
		out := map[string]string{}
		for _, entry := range cuaProcessEnvironment(platform) {
			key, value, _ := strings.Cut(entry, "=")
			out[key] = value
		}
		return out
	}
	linux := values("linux")
	if linux["DISPLAY"] != ":7" || linux["WAYLAND_DISPLAY"] != "wayland-7" || linux["DBUS_SESSION_BUS_ADDRESS"] != "unix:path=/run/user/1000/bus" || linux["CUA_DRIVER_RS_ENABLE_WAYLAND"] != "1" {
		t.Fatalf("desktop environment lost: %v", linux)
	}
	for _, key := range []string{"OPENAI_API_KEY", "REASONIX_DESKTOP_BRIDGE_TOKEN", "REASONIX_HOME", "CUA_DRIVER_RS_HOME", "LD_PRELOAD"} {
		if _, ok := linux[key]; ok {
			t.Fatalf("private/override variable inherited: %s", key)
		}
	}
	if linux["PATH"] != "/usr/bin:/bin:/usr/sbin:/sbin" || linux["CUA_TELEMETRY_ENABLED"] != "0" {
		t.Fatal("safe defaults were overridden")
	}
	if _, ok := values("darwin")["DISPLAY"]; ok {
		t.Fatal("Linux desktop env leaked into macOS")
	}
	t.Setenv("CUA_DRIVER_RS_ENABLE_WAYLAND", "true")
	if _, ok := values("linux")["CUA_DRIVER_RS_ENABLE_WAYLAND"]; ok {
		t.Fatal("experimental Wayland enabled without exact opt-in")
	}
}
