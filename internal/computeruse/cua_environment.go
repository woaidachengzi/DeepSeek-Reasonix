package computeruse

import "os"

func cuaProcessEnvironment(platform string) []string {
	env := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	if home, err := os.UserHomeDir(); err == nil {
		env = append(env, "HOME="+home)
	}
	if platform == "linux" {
		// These are transport addresses for the logged-in desktop, not model
		// credentials or Cua driver configuration. Never inherit the whole env.
		for _, name := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "XDG_SESSION_TYPE", "XDG_CURRENT_DESKTOP", "XDG_SESSION_DESKTOP", "DESKTOP_SESSION", "LANG", "LC_ALL", "LC_CTYPE"} {
			if value, ok := os.LookupEnv(name); ok {
				env = append(env, name+"="+value)
			}
		}
		if os.Getenv("CUA_DRIVER_RS_ENABLE_WAYLAND") == "1" {
			env = append(env, "CUA_DRIVER_RS_ENABLE_WAYLAND=1")
		}
	}
	// A private app-scoped engine has no need for upstream telemetry/updaters.
	return append(env, "CUA_DRIVER_RS_TELEMETRY_ENABLED=0", "CUA_TELEMETRY_ENABLED=0", "CUA_DRIVER_RS_UPDATE_CHECK=0")
}
