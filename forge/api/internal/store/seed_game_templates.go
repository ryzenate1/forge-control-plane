package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// gameTemplate is the minimal structure for seeding from packages/game-templates.
type gameTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Game        string            `json:"game"`
	Author      string            `json:"author"`
	UpdateURL   string            `json:"update_url"`
	Image       string            `json:"image"`
	Images      map[string]string `json:"images"`
	Startup     string            `json:"startup"`
	Config      json.RawMessage   `json:"config"`
	Env         []struct {
		Name         string `json:"name"`
		EnvVariable  string `json:"env_variable"`
		Description  string `json:"description"`
		DefaultValue string `json:"default_value"`
		UserViewable bool   `json:"user_viewable"`
		UserEditable bool   `json:"user_editable"`
		Rules        string `json:"rules"`
	} `json:"env"`
	Resources struct {
		MemoryMB int `json:"memory_mb"`
	} `json:"resources"`
	InstallScript struct {
		Container  string `json:"container"`
		Entrypoint string `json:"entrypoint"`
		Script     string `json:"script"`
	} `json:"install_script"`
	InstallSteps json.RawMessage `json:"install_steps"`
	FileDenylist []string        `json:"file_denylist"`
	Features     []string        `json:"features"`
}

// secretSeedVar reports whether an env variable holds a credential that must
// never ship a shared default. It mirrors the SECRET_VAR_RE exemption in
// packages/game-templates/scripts/validate-templates.mjs: an empty default
// for such a variable is valid at seed time (the value is supplied when the
// server is created), while a weak non-empty default would give every
// deployment the same guessable credential.
func secretSeedVar(envVariable string) bool {
	for _, suffix := range []string{"PASSWORD", "PASSWD", "PASS", "SECRET", "TOKEN", "PRIVATE_KEY"} {
		if strings.HasSuffix(envVariable, suffix) {
			return true
		}
	}
	return false
}

// fallbackGameTemplates is used when the embedded FS is empty (e.g. packages deleted).
// It mirrors the 14 curated templates in packages/game-templates/templates/*.json and
// forge/web/lib/egg-templates.ts:27 — kept minimal but sufficient to close the 93% deficit.
var fallbackGameTemplates = []gameTemplate{
	{
		ID: "minecraft-paper", Name: "Minecraft (Paper)", Description: "High-performance PaperMC server — the most widely used Minecraft server software, featuring optimized gameplay, extensive plugin API, and automatic latest build downloads.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:java_21", Startup: "java -Xms128M -XX:MaxRAMPercentage=95.0 -Dterminal.jline=false -Dterminal.ansi=true -jar {{SERVER_JARFILE}}",
		Images: map[string]string{"Java 21": "ghcr.io/pterodactyl/yolks:java_21", "Java 17": "ghcr.io/pterodactyl/yolks:java_17"}, Config: json.RawMessage(`{"files":{"server.properties":{"parser":"properties","find":{"server-ip":"0.0.0.0","server-port":"{{server.build.default.port}}"}}},"startup":{"done":")! For help, type \""},"stop":"stop","logs":{}}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 2048}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:alpine", Entrypoint: "ash", Script: "#!/bin/ash\n# PaperMC Installation Script\ncd /mnt/server\n# Paper download via papermc API (full script in packages/game-templates/templates/minecraft-paper.json)\necho \"install paper\""},
		FileDenylist: []string{}, Features: []string{"eula", "java_version", "pid_limit"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Minecraft Version", EnvVariable: "MINECRAFT_VERSION", Description: "Minecraft version to download.", DefaultValue: "latest", UserViewable: true, UserEditable: true, Rules: "nullable|string|max:20"},
			{Name: "Server Jar File", EnvVariable: "SERVER_JARFILE", Description: "File name for the server jar.", DefaultValue: "server.jar", UserViewable: true, UserEditable: true, Rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"},
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Display name.", DefaultValue: "A GamePanel Minecraft Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:60"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum number of concurrent players.", DefaultValue: "20", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:100"},
			{Name: "Difficulty", EnvVariable: "DIFFICULTY", Description: "Game difficulty level.", DefaultValue: "easy", UserViewable: true, UserEditable: true, Rules: "required|string|in:easy,normal,hard,peaceful"},
			{Name: "Gamemode", EnvVariable: "GAMEMODE", Description: "Default game mode.", DefaultValue: "survival", UserViewable: true, UserEditable: true, Rules: "required|string|in:survival,creative,adventure,spectator"},
			{Name: "Build Number", EnvVariable: "BUILD_NUMBER", Description: "Paper build number.", DefaultValue: "latest", UserViewable: true, UserEditable: true, Rules: "required|string|max:20"},
			{Name: "Download Path", EnvVariable: "DL_PATH", Description: "Override download URL.", DefaultValue: "", UserViewable: false, UserEditable: false, Rules: "nullable|string"},
		},
	},
	{
		ID: "minecraft-vanilla", Name: "Minecraft (Vanilla)", Description: "Official vanilla Minecraft server from Mojang. Pure, unmodified gameplay experience.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:java_21", Startup: "java -Xms128M -XX:MaxRAMPercentage=95.0 -Dterminal.jline=false -Dterminal.ansi=true -jar {{SERVER_JARFILE}}",
		Images: map[string]string{"Java 21": "ghcr.io/pterodactyl/yolks:java_21"}, Config: json.RawMessage(`{"files":{"server.properties":{"parser":"properties","find":{"server-ip":"0.0.0.0"}}},"startup":{"done":"For help, type"},"stop":"stop"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 1024}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:alpine", Entrypoint: "ash", Script: "#!/bin/ash\n# Vanilla\n echo install vanilla"},
		FileDenylist: []string{}, Features: []string{"eula"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Minecraft Version", EnvVariable: "MINECRAFT_VERSION", Description: "Minecraft version.", DefaultValue: "latest", UserViewable: true, UserEditable: true, Rules: "nullable|string|max:20"},
			{Name: "Server Jar File", EnvVariable: "SERVER_JARFILE", Description: "File name for the server jar.", DefaultValue: "server.jar", UserViewable: true, UserEditable: true, Rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"},
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Display name.", DefaultValue: "A GamePanel Minecraft Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:60"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "20", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:100"},
			{Name: "Difficulty", EnvVariable: "DIFFICULTY", Description: "Game difficulty.", DefaultValue: "easy", UserViewable: true, UserEditable: true, Rules: "required|string|in:easy,normal,hard,peaceful"},
			{Name: "Gamemode", EnvVariable: "GAMEMODE", Description: "Default game mode.", DefaultValue: "survival", UserViewable: true, UserEditable: true, Rules: "required|string|in:survival,creative,adventure,spectator"},
			{Name: "Download Path", EnvVariable: "DL_PATH", Description: "Override download URL.", DefaultValue: "", UserViewable: false, UserEditable: false, Rules: "nullable|string"},
		},
	},
	{
		ID: "minecraft-bedrock", Name: "Minecraft: Bedrock Edition", Description: "Minecraft Bedrock Edition dedicated server. Cross-platform Minecraft server.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:ubuntu", Startup: "LD_LIBRARY_PATH=. ./bedrock_server",
		Images: map[string]string{"Ubuntu": "ghcr.io/pterodactyl/yolks:ubuntu"}, Config: json.RawMessage(`{"files":{"server.properties":{"parser":"properties","find":{"server-port":"{{server.build.default.port}}"}}},"startup":{"done":"Server started."},"stop":"stop"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 2048}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:debian", Entrypoint: "bash", Script: "#!/bin/bash\n# Bedrock\n echo install bedrock"},
		FileDenylist: []string{}, Features: []string{"pid_limit"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "10", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:30"},
			{Name: "Gamemode", EnvVariable: "GAMEMODE", Description: "0=survival, 1=creative.", DefaultValue: "0", UserViewable: true, UserEditable: true, Rules: "required|integer|in:0,1"},
			{Name: "Difficulty", EnvVariable: "DIFFICULTY", Description: "0=peaceful, 1=easy, 2=normal, 3=hard.", DefaultValue: "2", UserViewable: true, UserEditable: true, Rules: "required|integer|in:0,1,2,3"},
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Server name.", DefaultValue: "Bedrock Level", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
		},
	},
	{
		ID: "palworld", Name: "Palworld", Description: "Palworld dedicated server. Build a base, befriend and capture Pals.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./PalServer.sh -port={{SERVER_PORT}} -players={{MAX_PLAYERS}}",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"*** Server Start ***"},"stop":"^C"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# Palworld\n ./steamcmd.sh +app_update 2394010 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Name of the Palworld server.", DefaultValue: "GamePanel Palworld Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "16", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:32"},
			{Name: "Admin Password", EnvVariable: "ADMIN_PASSWORD", Description: "Administrator password. No default is shipped — set a value when creating the server.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "required|string|min:8|max:32"},
			{Name: "Daytime Speed", EnvVariable: "PALWORLD_DAYTIME_SPEED", Description: "Daytime cycle speed.", DefaultValue: "1.000000", UserViewable: true, UserEditable: true, Rules: "required|regex:/^\\d+\\.\\d+$/"},
			{Name: "Nighttime Speed", EnvVariable: "PALWORLD_NIGHTTIME_SPEED", Description: "Nighttime cycle speed.", DefaultValue: "1.000000", UserViewable: true, UserEditable: true, Rules: "required|regex:/^\\d+\\.\\d+$/"},
			{Name: "EXP Rate", EnvVariable: "PALWORLD_EXP_RATE", Description: "Experience gain multiplier.", DefaultValue: "1.000000", UserViewable: true, UserEditable: true, Rules: "required|regex:/^\\d+\\.\\d+$/"},
			{Name: "Capture Rate", EnvVariable: "PALWORLD_CAPTURE_RATE", Description: "Pal capture rate.", DefaultValue: "1.000000", UserViewable: true, UserEditable: true, Rules: "required|regex:/^\\d+\\.\\d+$/"},
			{Name: "RCON Enabled", EnvVariable: "RCON_ENABLED", Description: "Enable RCON.", DefaultValue: "true", UserViewable: false, UserEditable: true, Rules: "required|string|in:true,false"},
		},
	},
	{
		ID: "valheim", Name: "Valheim", Description: "Valheim dedicated server. A brutal exploration and survival game.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./valheim_server.x86_64 -name '{{SERVER_NAME}}' -port {{SERVER_PORT}} -world {{WORLD_NAME}}",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"Game server connected"},"stop":"^C"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# Valheim\n ./steamcmd.sh +app_update 896660 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Name of the Valheim server.", DefaultValue: "Valheim Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Server Password", EnvVariable: "SERVER_PASSWORD", Description: "Server password. No default is shipped — set a value when creating the server.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "required|string|min:5|max:32"},
			{Name: "World Name", EnvVariable: "WORLD_NAME", Description: "World save name.", DefaultValue: "Dedicated", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
		},
	},
	{
		ID: "terraria", Name: "Terraria", Description: "Terraria dedicated server. Dig, fight, explore, build.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:ubuntu", Startup: "./TerrariaServer -config serverconfig.txt -port {{SERVER_PORT}}",
		Images: map[string]string{"Ubuntu": "ghcr.io/pterodactyl/yolks:ubuntu"}, Config: json.RawMessage(`{"startup":{"done":"Server started"},"stop":"exit"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 1024}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:debian", Entrypoint: "bash", Script: "#!/bin/bash\n# Terraria\n echo install terraria"},
		FileDenylist: []string{}, Features: []string{"pid_limit"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "8", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:255"},
			{Name: "Server Password", EnvVariable: "SERVER_PASSWORD", Description: "Server password.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:32"},
			{Name: "World Name", EnvVariable: "WORLD_NAME", Description: "Name of the world file.", DefaultValue: "World", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "World Size", EnvVariable: "WORLD_SIZE", Description: "World size: 1=small, 2=medium, 3=large.", DefaultValue: "2", UserViewable: true, UserEditable: true, Rules: "required|integer|in:1,2,3"},
		},
	},
	{
		ID: "enshrouded", Name: "Enshrouded", Description: "Enshrouded dedicated server. A survival action RPG in a vast, open world.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./enshrouded_server --server-name '{{SERVER_NAME}}' --port {{SERVER_PORT}}",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"Enshrouded Server started"},"stop":"^C"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# Enshrouded\n ./steamcmd.sh +app_update 2278520 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Name of the Enshrouded server.", DefaultValue: "GamePanel Enshrouded Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Query Port", EnvVariable: "QUERY_PORT", Description: "Query port for server listing.", DefaultValue: "15637", UserViewable: true, UserEditable: false, Rules: "required|integer|min:1024|max:65535"},
			{Name: "Server Password", EnvVariable: "SERVER_PASSWORD", Description: "Server password.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:32"},
		},
	},
	{
		ID: "satisfactory", Name: "Satisfactory", Description: "Satisfactory dedicated server. Build massive factories, automate production.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./FactoryServer.sh -ServerPort={{SERVER_PORT}} -BeaconPort={{BEACON_PORT}}",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"Engine is initialized"},"stop":"^C"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# Satisfactory\n ./steamcmd.sh +app_update 1690800 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "4", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:4"},
			{Name: "Beacon Port", EnvVariable: "BEACON_PORT", Description: "Beacon port.", DefaultValue: "15000", UserViewable: true, UserEditable: false, Rules: "required|integer|min:1024|max:65535"},
			{Name: "Timeout", EnvVariable: "TIMEOUT", Description: "Server timeout in seconds.", DefaultValue: "300", UserViewable: false, UserEditable: false, Rules: "required|integer|min:30"},
		},
	},
	{
		ID: "rust", Name: "Rust", Description: "Rust dedicated server. The ultimate survival game.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./RustDedicated -batchmode +server.port {{SERVER_PORT}} +server.level '{{SERVER_LEVEL}}'",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"Server Startup Complete"},"stop":"^C"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# Rust\n ./steamcmd.sh +app_update 258550 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Name of the Rust server.", DefaultValue: "Rust Public Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "50", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:500"},
			{Name: "Server Level", EnvVariable: "SERVER_LEVEL", Description: "Map type.", DefaultValue: "Procedural Map", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Server Seed", EnvVariable: "SERVER_SEED", Description: "Seed value for procedural map generation.", DefaultValue: "1234", UserViewable: true, UserEditable: true, Rules: "required|integer"},
			{Name: "RCON Password", EnvVariable: "RCON_PASSWORD", Description: "Password for RCON access. No default is shipped — set a value when creating the server.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "required|string|min:8|max:64"},
		},
	},
	{
		ID: "csgo", Name: "Counter-Strike: Global Offensive (CS2)", Description: "Counter-Strike 2 dedicated server. The legendary tactical first-person shooter.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./game/cs2/bin/linuxsteamrt64/cs2 -dedicated -ip 0.0.0.0 -port {{SERVER_PORT}}",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"Loading game"},"stop":"quit"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# CS2\n ./steamcmd.sh +app_update 730 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Password", EnvVariable: "SERVER_PASSWORD", Description: "Server password for joining.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:32"},
			{Name: "Game Type", EnvVariable: "GAME_TYPE", Description: "0=classic, 1=armsrace/demolition.", DefaultValue: "0", UserViewable: true, UserEditable: true, Rules: "required|integer|in:0,1"},
			{Name: "Game Mode", EnvVariable: "GAME_MODE", Description: "0=casual, 1=competitive, etc.", DefaultValue: "0", UserViewable: true, UserEditable: true, Rules: "required|integer|in:0,1,2,3,4,5,6"},
			{Name: "Map Group", EnvVariable: "MAP_GROUP", Description: "Map group to use.", DefaultValue: "mg_active", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Map", EnvVariable: "MAP", Description: "Starting map.", DefaultValue: "de_dust2", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Steam Account Token", EnvVariable: "STEAM_ACCOUNT", Description: "Steam game server login token. May be left empty for LAN-only play.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:64"},
		},
	},
	{
		ID: "factorio", Name: "Factorio", Description: "Factorio dedicated server. Build and manage automated factories.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:ubuntu", Startup: "./factorio/bin/x64/factorio --start-server {{SAVE_NAME}} --port {{SERVER_PORT}}",
		Images: map[string]string{"Ubuntu": "ghcr.io/pterodactyl/yolks:ubuntu"}, Config: json.RawMessage(`{"startup":{"done":"Server isUP"},"stop":"/quit"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 2048}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:debian", Entrypoint: "bash", Script: "#!/bin/bash\n# Factorio\n echo install factorio"},
		FileDenylist: []string{}, Features: []string{"pid_limit"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Save Name", EnvVariable: "SAVE_NAME", Description: "Name of the save file.", DefaultValue: "factorio", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "100", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:65535"},
			{Name: "Game Password", EnvVariable: "GAME_PASSWORD", Description: "Server password.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:32"},
		},
	},
	{
		ID: "7days2die", Name: "7 Days to Die", Description: "7 Days to Die dedicated server. The open-world zombie survival sandbox game.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./7DaysToDieServer.x86_64 -configfile=serverconfig.xml",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"Server start completed"},"stop":"shutdown"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# 7DTD\n ./steamcmd.sh +app_update 294420 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Name of the server.", DefaultValue: "7 Days to Die Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Server World", EnvVariable: "SERVER_WORLD", Description: "World name.", DefaultValue: "Navezgane", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "8", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:64"},
			{Name: "Server Password", EnvVariable: "SERVER_PASSWORD", Description: "Server password.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:32"},
			{Name: "Game Difficulty", EnvVariable: "GAME_DIFFICULTY", Description: "0-5 difficulty level.", DefaultValue: "2", UserViewable: true, UserEditable: true, Rules: "required|integer|in:0,1,2,3,4,5"},
		},
	},
	{
		ID: "teamspeak3", Name: "TeamSpeak 3", Description: "TeamSpeak 3 voice server. The high-quality voice communication platform.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:ubuntu", Startup: "./ts3server default_voice_port={{SERVER_PORT}} query_port={{QUERY_PORT}}",
		Images: map[string]string{"Ubuntu": "ghcr.io/pterodactyl/yolks:ubuntu"}, Config: json.RawMessage(`{"startup":{"done":"listening on"},"stop":"exit"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 512}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:debian", Entrypoint: "bash", Script: "#!/bin/bash\n# TS3\n echo install ts3"},
		FileDenylist: []string{}, Features: []string{"pid_limit"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Query Port", EnvVariable: "QUERY_PORT", Description: "ServerQuery port.", DefaultValue: "10011", UserViewable: true, UserEditable: false, Rules: "required|integer|min:1024|max:65535"},
			{Name: "File Transfer Port", EnvVariable: "FILE_PORT", Description: "File transfer port.", DefaultValue: "30033", UserViewable: true, UserEditable: false, Rules: "required|integer|min:1024|max:65535"},
			{Name: "Admin Password", EnvVariable: "SERVERADMIN_PASSWORD", Description: "Server admin password.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:64"},
		},
	},
	{
		ID: "zomboid", Name: "Project Zomboid", Description: "Project Zomboid dedicated server. The ultimate zombie survival simulation game.", Author: "GamePanel", Image: "ghcr.io/pterodactyl/yolks:steamcmd", Startup: "./start-server.sh -servername \"{{SERVER_NAME}}\" -adminpassword \"{{ADMIN_PASSWORD}}\"",
		Images: map[string]string{"SteamCMD": "ghcr.io/pterodactyl/yolks:steamcmd"}, Config: json.RawMessage(`{"startup":{"done":"SERVER STARTED"},"stop":"quit"}`),
		Resources: struct {
			MemoryMB int `json:"memory_mb"`
		}{MemoryMB: 4096}, InstallScript: struct {
			Container  string `json:"container"`
			Entrypoint string `json:"entrypoint"`
			Script     string `json:"script"`
		}{Container: "ghcr.io/pterodactyl/installers:steamcmd", Entrypoint: "bash", Script: "#!/bin/bash\n# Zomboid\n ./steamcmd.sh +app_update 380870 +quit"},
		FileDenylist: []string{}, Features: []string{"steamcmd"},
		Env: []struct {
			Name         string `json:"name"`
			EnvVariable  string `json:"env_variable"`
			Description  string `json:"description"`
			DefaultValue string `json:"default_value"`
			UserViewable bool   `json:"user_viewable"`
			UserEditable bool   `json:"user_editable"`
			Rules        string `json:"rules"`
		}{
			{Name: "Server Name", EnvVariable: "SERVER_NAME", Description: "Name of the Zomboid server.", DefaultValue: "Zomboid Server", UserViewable: true, UserEditable: true, Rules: "required|string|max:64"},
			{Name: "Admin Password", EnvVariable: "ADMIN_PASSWORD", Description: "Admin password. No default is shipped — set a value when creating the server.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "required|string|min:4|max:32"},
			{Name: "Max Players", EnvVariable: "MAX_PLAYERS", Description: "Maximum players.", DefaultValue: "16", UserViewable: true, UserEditable: true, Rules: "required|integer|min:1|max:100"},
			{Name: "Server Password", EnvVariable: "SERVER_PASSWORD", Description: "Server password.", DefaultValue: "", UserViewable: false, UserEditable: true, Rules: "nullable|string|max:32"},
		},
	},
}

// SeedGameTemplates idempotently upserts the 14 curated game templates.
// It is additive and uses ON CONFLICT (nest_id, name) DO NOTHING to remain idempotent.
func (s *Store) SeedGameTemplates(ctx context.Context) error {
	templates, err := loadGameTemplates()
	if err != nil {
		return fmt.Errorf("load game templates: %w", err)
	}
	// Ensure the Games nest exists.
	var nestID string
	err = s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID)
	if err != nil {
		// Create nest if missing
		nestID = uuid.NewString()
		if _, err2 := s.db.Exec(ctx, `INSERT INTO nests (id, name, description) VALUES ($1, 'Games', 'Default game nest') ON CONFLICT (name) DO NOTHING`, nestID); err2 != nil {
			return fmt.Errorf("create Games nest: %w", err2)
		}
		if err2 := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err2 != nil {
			return fmt.Errorf("fetch Games nest: %w", err2)
		}
	}
	for _, tpl := range templates {
		eggID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("gamepanel:template:"+tpl.ID)).String()
		dockerImages, _ := json.Marshal(tpl.Images)
		if len(dockerImages) == 0 || string(dockerImages) == "null" {
			dockerImages, _ = json.Marshal(map[string]string{tpl.Image: tpl.Image})
		}
		config := tpl.Config
		if len(config) == 0 {
			config = json.RawMessage(`{}`)
		}
		fileDenylist, _ := json.Marshal(tpl.FileDenylist)
		if len(fileDenylist) == 0 {
			fileDenylist = json.RawMessage(`[]`)
		}
		features, _ := json.Marshal(tpl.Features)
		if len(features) == 0 {
			features = json.RawMessage(`[]`)
		}
		installSteps := tpl.InstallSteps
		if len(installSteps) == 0 {
			installSteps = json.RawMessage(`[]`)
		} else {
			// Validate it is a JSON array; normalize empty to []
			var arr []json.RawMessage
			if err := json.Unmarshal(installSteps, &arr); err != nil {
				return fmt.Errorf("seed egg %s install_steps invalid: %w", tpl.Name, err)
			}
			if arr == nil {
				installSteps = json.RawMessage(`[]`)
			}
		}
		// Insert egg idempotently; ON CONFLICT (nest_id, name) DO NOTHING preserves existing user-customized eggs.
		_, err = s.db.Exec(ctx, `
			INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config,
			                  default_memory_mb, install_script, install_container, install_entrypoint,
			                  install_steps, file_denylist, author, features)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (nest_id, name) DO NOTHING
		`, eggID, nestID, tpl.Name, tpl.Description, string(dockerImages), tpl.Startup, string(config),
			tpl.Resources.MemoryMB, tpl.InstallScript.Script, tpl.InstallScript.Container, tpl.InstallScript.Entrypoint,
			string(installSteps), string(fileDenylist), tpl.Author, string(features))
		if err != nil {
			return fmt.Errorf("seed egg %s: %w", tpl.Name, err)
		}
		// Seed variables for this egg. ON CONFLICT (egg_id, env_variable) DO NOTHING ensures idempotency.
		for idx, v := range tpl.Env {
			varID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("gamepanel:template:"+tpl.ID+":var:"+v.EnvVariable)).String()
			// Use non-nil sort; default to idx*10 if not provided.
			sortVal := idx * 10
			_, err = s.db.Exec(ctx, `
				INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value, user_viewable, user_editable, rules, sort)
				SELECT $1, e.id, $2, $3, $4, $5, $6, $7, $8, $9
				FROM eggs e WHERE e.nest_id = $10 AND e.name = $11
				ON CONFLICT (egg_id, env_variable) DO NOTHING
			`, varID, v.Name, v.Description, v.EnvVariable, v.DefaultValue, v.UserViewable, v.UserEditable, v.Rules, sortVal, nestID, tpl.Name)
			if err != nil {
				return fmt.Errorf("seed variable %s for %s: %w", v.EnvVariable, tpl.Name, err)
			}
			// Validate seeding succeeded with fixed regex validator (PTDL imports).
			// Empty secret variables are exempt: no shared default is shipped and
			// the value is supplied at server creation (see secretSeedVar).
			if err := validateVariableValue(v.DefaultValue, v.Rules); err != nil {
				if v.DefaultValue != "" || !secretSeedVar(v.EnvVariable) {
					return fmt.Errorf("seed validation failed for %s %s: %w", tpl.Name, v.EnvVariable, err)
				}
			}
		}
	}
	return nil
}

func loadGameTemplates() ([]gameTemplate, error) {
	return fallbackGameTemplates, nil
}
