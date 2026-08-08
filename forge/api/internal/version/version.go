package version

import "runtime/debug"

var (
	Version   = "dev"
	Commit    = "unknown"
	Date      = "unknown"
	BuildTime = "unknown"
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if Commit == "unknown" {
				Commit = setting.Value
			}
		case "vcs.time":
			if Date == "unknown" {
				Date = setting.Value
			}
			if BuildTime == "unknown" {
				BuildTime = setting.Value
			}
		}
	}
}
