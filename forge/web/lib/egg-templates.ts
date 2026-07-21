export type EggTemplateItem = {
  id: string
  name: string
  description: string
  game: string
  author: string
  image: string
  images: Record<string, string>
  startup: string
  installContainer: string
  installEntrypoint: string
  installScript: string
  config: Record<string, unknown>
  env: Array<{
    name: string
    envVariable: string
    description: string
    defaultValue: string
    userViewable: boolean
    userEditable: boolean
    rules: string
  }>
  features: string[]
  fileDenylist: string[]
}

export const EGG_TEMPLATES: EggTemplateItem[] = [
  {
    id: "minecraft-paper",
    name: "Minecraft (Paper)",
    description:
      "High-performance PaperMC server — the most widely used Minecraft server software, featuring optimized gameplay, extensive plugin API, and automatic latest build downloads.",
    game: "Minecraft",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:java_21",
    images: {
      "Java 21": "ghcr.io/pterodactyl/yolks:java_21",
      "Java 17": "ghcr.io/pterodactyl/yolks:java_17",
    },
    startup:
      "java -Xms128M -XX:MaxRAMPercentage=95.0 -Dterminal.jline=false -Dterminal.ansi=true -jar {{SERVER_JARFILE}}",
    installContainer: "ghcr.io/pterodactyl/installers:alpine",
    installEntrypoint: "ash",
    installScript: `#!/bin/ash
# PaperMC Installation Script
# Server Files: /mnt/server
PROJECT=paper
if [ -n "\${DL_PATH}" ]; then
  DOWNLOAD_URL=$(eval echo $(echo \${DL_PATH} | sed -e 's/{{/${/g' -e 's/}}/}/g'))
else
  VER_EXISTS=$(curl -s "https://api.papermc.io/v2/projects/\${PROJECT}" | jq -r --arg VERSION $MINECRAFT_VERSION '.versions[] | contains($VERSION)' | grep -m1 true)
  LATEST_VERSION=$(curl -s "https://api.papermc.io/v2/projects/\${PROJECT}" | jq -r '.versions | last')
  if [ "\${VER_EXISTS}" = "true" ]; then echo "Version is valid. Using version \${MINECRAFT_VERSION}"
  else MINECRAFT_VERSION=\${LATEST_VERSION}; fi
  BUILD_EXISTS=$(curl -s "https://api.papermc.io/v2/projects/\${PROJECT}/versions/\${MINECRAFT_VERSION}" | jq -r --arg BUILD \${BUILD_NUMBER} '.builds[] | tostring | contains($BUILD)' | grep -m1 true)
  LATEST_BUILD=$(curl -s "https://api.papermc.io/v2/projects/\${PROJECT}/versions/\${MINECRAFT_VERSION}" | jq -r '.builds | last')
  if [ "\${BUILD_EXISTS}" = "true" ]; then echo "Build is valid. Using build \${BUILD_NUMBER}"
  else BUILD_NUMBER=\${LATEST_BUILD}; fi
  DOWNLOAD_URL="https://api.papermc.io/v2/projects/\${PROJECT}/versions/\${MINECRAFT_VERSION}/builds/\${BUILD_NUMBER}/downloads/\${PROJECT}-\${MINECRAFT_VERSION}-\${BUILD_NUMBER}.jar"
fi
cd /mnt/server
curl -o \${SERVER_JARFILE} -L "\${DOWNLOAD_URL}"`,
    config: {
      files: {
        "server.properties": {
          parser: "properties",
          find: {
            "server-ip": "0.0.0.0",
            "server-port": "{{server.build.default.port}}",
            "query.port": "{{server.build.default.port}}",
          },
        },
      },
      startup: { done: ')! For help, type "' },
      stop: "stop",
      logs: {},
    },
    env: [
      { name: "Minecraft Version", envVariable: "MINECRAFT_VERSION", description: "Minecraft version to download. Set to 'latest' for the most recent stable release.", defaultValue: "latest", userViewable: true, userEditable: true, rules: "nullable|string|max:20" },
      { name: "Server Jar File", envVariable: "SERVER_JARFILE", description: "File name for the server jar. Must end with .jar.", defaultValue: "server.jar", userViewable: true, userEditable: true, rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/" },
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Display name shown in the Minecraft server list.", defaultValue: "A GamePanel Minecraft Server", userViewable: true, userEditable: true, rules: "required|string|max:60" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of concurrent players.", defaultValue: "20", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:100" },
      { name: "Difficulty", envVariable: "DIFFICULTY", description: "Game difficulty level.", defaultValue: "easy", userViewable: true, userEditable: true, rules: "required|string|in:easy,normal,hard,peaceful" },
      { name: "Gamemode", envVariable: "GAMEMODE", description: "Default game mode for new players.", defaultValue: "survival", userViewable: true, userEditable: true, rules: "required|string|in:survival,creative,adventure,spectator" },
      { name: "Build Number", envVariable: "BUILD_NUMBER", description: "Paper build number. Leave as 'latest' for the most recent build.", defaultValue: "latest", userViewable: true, userEditable: true, rules: "required|string|max:20" },
      { name: "Download Path", envVariable: "DL_PATH", description: "Override download URL for the server jar. Internal use only.", defaultValue: "", userViewable: false, userEditable: false, rules: "nullable|string" },
    ],
    features: ["eula", "java_version", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "minecraft-vanilla",
    name: "Minecraft (Vanilla)",
    description: "Official Mojang vanilla Minecraft server. The original sandbox survival game with pure, unmodified gameplay.",
    game: "Minecraft",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:java_21",
    images: {
      "Java 21": "ghcr.io/pterodactyl/yolks:java_21",
      "Java 17": "ghcr.io/pterodactyl/yolks:java_17",
    },
    startup: "java -Xms128M -XX:MaxRAMPercentage=95.0 -Dterminal.jline=false -Dterminal.ansi=true -jar {{SERVER_JARFILE}}",
    installContainer: "ghcr.io/pterodactyl/installers:alpine",
    installEntrypoint: "ash",
    installScript: `#!/bin/ash
# Vanilla Minecraft Installation Script
# Server Files: /mnt/server
if [ -n "\${DL_PATH}" ]; then
  DOWNLOAD_URL=$(eval echo $(echo \${DL_PATH} | sed -e 's/{{/${/g' -e 's/}}/}/g'))
else
  if [ "\${MINECRAFT_VERSION}" = "latest" ]; then
    MANIFEST=$(curl -s https://launchermeta.mojang.com/mc/game/version_manifest.json)
    MINECRAFT_VERSION=$(echo "$MANIFEST" | jq -r '.latest.release')
  fi
  MANIFEST_URL=$(curl -s https://launchermeta.mojang.com/mc/game/version_manifest.json | jq -r --arg VER "\${MINECRAFT_VERSION}" '.versions[] | select(.id == $VER) | .url')
  DOWNLOAD_URL=$(curl -s "\${MANIFEST_URL}" | jq -r '.downloads.server.url')
fi
cd /mnt/server
curl -o \${SERVER_JARFILE} -L "\${DOWNLOAD_URL}"`,
    config: {
      files: {
        "server.properties": {
          parser: "properties",
          find: {
            "server-ip": "0.0.0.0",
            "server-port": "{{server.build.default.port}}",
          },
        },
      },
      startup: { done: "For help, type " },
      stop: "stop",
      logs: {},
    },
    env: [
      { name: "Minecraft Version", envVariable: "MINECRAFT_VERSION", description: "Minecraft version to download. Set to 'latest' for the most recent stable release.", defaultValue: "latest", userViewable: true, userEditable: true, rules: "nullable|string|max:20" },
      { name: "Server Jar File", envVariable: "SERVER_JARFILE", description: "File name for the server jar.", defaultValue: "server.jar", userViewable: true, userEditable: true, rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of concurrent players.", defaultValue: "20", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:100" },
      { name: "Difficulty", envVariable: "DIFFICULTY", description: "Game difficulty level.", defaultValue: "easy", userViewable: true, userEditable: true, rules: "required|string|in:easy,normal,hard,peaceful" },
      { name: "Gamemode", envVariable: "GAMEMODE", description: "Default game mode for new players.", defaultValue: "survival", userViewable: true, userEditable: true, rules: "required|string|in:survival,creative,adventure,spectator" },
      { name: "Download Path", envVariable: "DL_PATH", description: "Override download URL. Internal use only.", defaultValue: "", userViewable: false, userEditable: false, rules: "nullable|string" },
    ],
    features: ["eula", "java_version", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "palworld",
    name: "Palworld",
    description: "Palworld dedicated server. Build a base, befriend and capture Pals, and explore a vast open world together.",
    game: "Palworld",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./PalServer.sh -port={{SERVER_PORT}} -players={{MAX_PLAYERS}} -Servername={{SERVER_NAME}} -ServerDescription='{{SERVER_DESC}}' -AdminPassword={{ADMIN_PASSWORD}} -useperfthreads -NoAsyncLoadingThread -UseMultithreadForDS",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Palworld Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 2394010 +quit`,
    config: {
      startup: { done: "*** Server Start ***" },
      stop: "^C",
      logs: { content: "Pal.log" },
    },
    env: [
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Name of the Palworld server.", defaultValue: "GamePanel Palworld Server", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Server Description", envVariable: "SERVER_DESC", description: "Server description displayed in the server browser.", defaultValue: "A Palworld server hosted on GamePanel", userViewable: true, userEditable: true, rules: "nullable|string|max:256" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of players.", defaultValue: "16", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:32" },
      { name: "Admin Password", envVariable: "ADMIN_PASSWORD", description: "Administrator password for in-game admin commands.", defaultValue: "changeme", userViewable: false, userEditable: true, rules: "required|string|min:8|max:32" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "valheim",
    name: "Valheim",
    description: "Valheim dedicated server. A brutal exploration and survival game for 1-10 players set in a procedurally-generated Viking purgatory.",
    game: "Valheim",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./valheim_server.x86_64 -name '{{SERVER_NAME}}' -port {{SERVER_PORT}} -world {{WORLD_NAME}} -password {{SERVER_PASSWORD}} -public 1",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Valheim Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 896660 +quit`,
    config: {
      startup: { done: "Game server connected" },
      stop: "^C",
      logs: {},
    },
    env: [
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Name of the Valheim server.", defaultValue: "Valheim Server", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Server Password", envVariable: "SERVER_PASSWORD", description: "Server password (minimum 5 characters).", defaultValue: "gamepanel", userViewable: false, userEditable: true, rules: "required|string|min:5|max:32" },
      { name: "World Name", envVariable: "WORLD_NAME", description: "World save name.", defaultValue: "Dedicated", userViewable: true, userEditable: true, rules: "required|string|max:64" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "terraria",
    name: "Terraria",
    description: "Terraria dedicated server. Dig, fight, explore, build — the action-adventure sandbox game with limitless possibilities.",
    game: "Terraria",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:ubuntu",
    images: { Ubuntu: "ghcr.io/pterodactyl/yolks:ubuntu" },
    startup: "./TerrariaServer -config serverconfig.txt -port {{SERVER_PORT}} -players {{MAX_PLAYERS}} -pass {{SERVER_PASSWORD}} -world {{WORLD_PATH}} -autocreate {{WORLD_SIZE}}",
    installContainer: "ghcr.io/pterodactyl/installers:debian",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Terraria Installation Script
# Server Files: /mnt/server
cd /mnt/server
DOWNLOAD_URL="https://terraria.org/api/download/pc-dedicated-server/terraria-server-1449.zip"
curl -L -o terraria-server.zip "$DOWNLOAD_URL"
unzip -o terraria-server.zip
cp -r */Linux/* ./
rm -rf terraria-server.zip */
chmod +x TerrariaServer`,
    config: {
      files: {
        "serverconfig.txt": {
          parser: "properties",
          find: {
            port: "{{server.build.default.port}}",
            maxplayers: "{{MAX_PLAYERS}}",
            password: "{{SERVER_PASSWORD}}",
            worldname: "{{WORLD_NAME}}",
            autocreate: "{{WORLD_SIZE}}",
          },
        },
      },
      startup: { done: "Server started" },
      stop: "exit",
      logs: {},
    },
    env: [
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of concurrent players.", defaultValue: "8", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:255" },
      { name: "Server Password", envVariable: "SERVER_PASSWORD", description: "Server password for joining.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:32" },
      { name: "World Name", envVariable: "WORLD_NAME", description: "Name of the world file.", defaultValue: "World", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "World Size", envVariable: "WORLD_SIZE", description: "World size: 1=small, 2=medium, 3=large.", defaultValue: "2", userViewable: true, userEditable: true, rules: "required|integer|in:1,2,3" },
      { name: "Difficulty", envVariable: "DIFFICULTY", description: "0=normal, 1=expert, 2=master, 3=journey.", defaultValue: "0", userViewable: true, userEditable: true, rules: "required|integer|in:0,1,2,3" },
      { name: "World Path", envVariable: "WORLD_PATH", description: "Path to the world file.", defaultValue: "/mnt/server/World.wld", userViewable: false, userEditable: false, rules: "required|string" },
    ],
    features: ["pid_limit"],
    fileDenylist: [],
  },
  {
    id: "enshrouded",
    name: "Enshrouded",
    description: "Enshrouded dedicated server. A survival action RPG in a vast, open world with base building, crafting, and co-op gameplay.",
    game: "Enshrouded",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./enshrouded_server --logFile '' --server-name '{{SERVER_NAME}}' --save-directory ./savegame --log-directory ./logs --ip 0.0.0.0 --port {{SERVER_PORT}} --query-port {{QUERY_PORT}}",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Enshrouded Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 2278520 +quit`,
    config: {
      logs: { content: "logs/*.log" },
      startup: { done: "Enshrouded Server started" },
      stop: "^C",
    },
    env: [
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Name of the Enshrouded server.", defaultValue: "GamePanel Enshrouded Server", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Query Port", envVariable: "QUERY_PORT", description: "Query port for server listing.", defaultValue: "15637", userViewable: true, userEditable: false, rules: "required|integer|min:1024|max:65535" },
      { name: "Server Password", envVariable: "SERVER_PASSWORD", description: "Server password.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:32" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "satisfactory",
    name: "Satisfactory",
    description: "Satisfactory dedicated server. Build massive factories, automate production, and explore an alien planet in this first-person open-world factory building game.",
    game: "Satisfactory",
    author: "GamePanel",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./FactoryServer.sh -ServerPort={{SERVER_PORT}} -BeaconPort={{BEACON_PORT}} -multihome=0.0.0.0 -log",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Satisfactory Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 1690800 +quit`,
    config: {
      startup: { done: "Engine is initialized" },
      stop: "^C",
      logs: {},
    },
    env: [
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of players.", defaultValue: "4", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:4" },
      { name: "Beacon Port", envVariable: "BEACON_PORT", description: "Beacon port for server listing.", defaultValue: "15000", userViewable: true, userEditable: false, rules: "required|integer|min:1024|max:65535" },
      { name: "Timeout", envVariable: "TIMEOUT", description: "Server timeout in seconds.", defaultValue: "300", userViewable: false, userEditable: false, rules: "required|integer|min:30" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "rust",
    name: "Rust",
    description: "Rust dedicated server. The ultimate survival game where you must build, craft, and fight to survive in a harsh open world.",
    game: "Rust",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./RustDedicated -batchmode +server.port {{SERVER_PORT}} +server.level '{{SERVER_LEVEL}}' +server.seed {{SERVER_SEED}} +server.name '{{SERVER_NAME}}' +server.description '{{SERVER_DESC}}' +server.maxplayers {{MAX_PLAYERS}} +server.identity '{{SERVER_IDENTITY}}' +rcon.port {{RCON_PORT}} +rcon.password {{RCON_PASSWORD}} +server.ip 0.0.0.0",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Rust Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 258550 +quit`,
    config: {
      startup: { done: "Server Startup Complete" },
      stop: "^C",
      logs: {},
    },
    env: [
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Name of the Rust server.", defaultValue: "Rust Public Server", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Server Description", envVariable: "SERVER_DESC", description: "Description shown in the server browser.", defaultValue: "A Rust server hosted on GamePanel", userViewable: true, userEditable: true, rules: "required|string|max:256" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of players.", defaultValue: "50", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:500" },
      { name: "Server Level", envVariable: "SERVER_LEVEL", description: "Map type.", defaultValue: "Procedural Map", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Server Seed", envVariable: "SERVER_SEED", description: "Seed value for procedural map generation.", defaultValue: "1234", userViewable: true, userEditable: true, rules: "required|integer" },
      { name: "Server Identity", envVariable: "SERVER_IDENTITY", description: "Server identity folder name.", defaultValue: "server1", userViewable: false, userEditable: false, rules: "required|string|max:32" },
      { name: "RCON Port", envVariable: "RCON_PORT", description: "Port for RCON remote administration.", defaultValue: "28016", userViewable: true, userEditable: false, rules: "required|integer|min:1024|max:65535" },
      { name: "RCON Password", envVariable: "RCON_PASSWORD", description: "Password for RCON access.", defaultValue: "changeme", userViewable: false, userEditable: true, rules: "required|string|min:8|max:64" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "csgo",
    name: "Counter-Strike: Global Offensive (CS2)",
    description: "Counter-Strike 2 dedicated server. The legendary tactical first-person shooter with competitive matchmaking and community servers.",
    game: "Counter-Strike 2",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./game/cs2/bin/linuxsteamrt64/cs2 -dedicated -ip 0.0.0.0 -port {{SERVER_PORT}} -console -usercon +game_type {{GAME_TYPE}} +game_mode {{GAME_MODE}} +mapgroup {{MAP_GROUP}} +map {{MAP}} +sv_setsteamaccount {{STEAM_ACCOUNT}} +sv_password '{{SERVER_PASSWORD}}' +sv_lan 0 +exec {{SERVER_CFG}}",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# CS2 Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 730 +quit`,
    config: {
      startup: { done: "Loading game" },
      stop: "quit",
      logs: {},
    },
    env: [
      { name: "Server Password", envVariable: "SERVER_PASSWORD", description: "Server password for joining.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:32" },
      { name: "Game Type", envVariable: "GAME_TYPE", description: "0=classic, 1=armsrace/demolition.", defaultValue: "0", userViewable: true, userEditable: true, rules: "required|integer|in:0,1" },
      { name: "Game Mode", envVariable: "GAME_MODE", description: "0=casual, 1=competitive, 2=scrimmage, 3=wingman, 4=armsrace, 5=demolition, 6=deathmatch.", defaultValue: "0", userViewable: true, userEditable: true, rules: "required|integer|in:0,1,2,3,4,5,6" },
      { name: "Map Group", envVariable: "MAP_GROUP", description: "Map group to use.", defaultValue: "mg_active", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Map", envVariable: "MAP", description: "Starting map.", defaultValue: "de_dust2", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Steam Account Token", envVariable: "STEAM_ACCOUNT", description: "Steam game server login token from https://steamcommunity.com/dev/managegameservers.", defaultValue: "", userViewable: false, userEditable: true, rules: "required|string|max:64" },
      { name: "Server Config", envVariable: "SERVER_CFG", description: "Server config file to execute on start.", defaultValue: "server.cfg", userViewable: true, userEditable: true, rules: "required|string|max:64" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "factorio",
    name: "Factorio",
    description: "Factorio dedicated server. Build and manage automated factories in this legendary optimization and resource-management simulation game.",
    game: "Factorio",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:ubuntu",
    images: { Ubuntu: "ghcr.io/pterodactyl/yolks:ubuntu" },
    startup: "./factorio/bin/x64/factorio --start-server {{SAVE_NAME}} --port {{SERVER_PORT}} --server-settings ./data/server-settings.json",
    installContainer: "ghcr.io/pterodactyl/installers:debian",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Factorio Installation Script
# Server Files: /mnt/server
cd /mnt/server
LATEST_VERSION=$(curl -s https://factorio.com/api/latest-releases | jq -r '.stable.headless')
DOWNLOAD_URL="https://factorio.com/get-download/\${LATEST_VERSION}/headless/linux64"
curl -L -o factorio.tar.xz "$DOWNLOAD_URL"
tar -xf factorio.tar.xz
rm factorio.tar.xz`,
    config: {
      startup: { done: "Server isUP" },
      stop: "/quit",
      logs: {},
    },
    env: [
      { name: "Save Name", envVariable: "SAVE_NAME", description: "Name of the save file to load or create.", defaultValue: "factorio", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of players.", defaultValue: "100", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:65535" },
      { name: "Game Password", envVariable: "GAME_PASSWORD", description: "Server password.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:32" },
    ],
    features: ["pid_limit"],
    fileDenylist: [],
  },
  {
    id: "7days2die",
    name: "7 Days to Die",
    description: "7 Days to Die dedicated server. The open-world zombie survival sandbox game with crafting, building, and horde nights.",
    game: "7 Days to Die",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./7DaysToDieServer.x86_64 -configfile=serverconfig.xml -logfile=logs/7DaysToDie.log -quit -batchmode -nographics -dedicated",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# 7 Days to Die Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 294420 +quit`,
    config: {
      startup: { done: "Server start completed" },
      stop: "shutdown",
      logs: { content: "logs/*.log" },
    },
    env: [
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Name of the server.", defaultValue: "7 Days to Die Server", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Server World", envVariable: "SERVER_WORLD", description: "World name.", defaultValue: "Navezgane", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum players.", defaultValue: "8", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:64" },
      { name: "Server Password", envVariable: "SERVER_PASSWORD", description: "Server password.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:32" },
      { name: "Game Difficulty", envVariable: "GAME_DIFFICULTY", description: "0-5 difficulty level.", defaultValue: "2", userViewable: true, userEditable: true, rules: "required|integer|in:0,1,2,3,4,5" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
  {
    id: "minecraft-bedrock",
    name: "Minecraft: Bedrock Edition",
    description: "Minecraft Bedrock Edition dedicated server. Cross-platform Minecraft server supporting Windows, mobile, and console players.",
    game: "Minecraft Bedrock",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:ubuntu",
    images: { Ubuntu: "ghcr.io/pterodactyl/yolks:ubuntu" },
    startup: "LD_LIBRARY_PATH=. ./bedrock_server",
    installContainer: "ghcr.io/pterodactyl/installers:debian",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Minecraft Bedrock Installation Script
# Server Files: /mnt/server
cd /mnt/server
curl -L -o bedrock-server.zip "https://minecraft.net/en-us/download/server/bedrock/"
unzip -o bedrock-server.zip
rm bedrock-server.zip`,
    config: {
      files: {
        "server.properties": {
          parser: "properties",
          find: {
            "server-port": "{{server.build.default.port}}",
            "server-portv6": "19133",
            "max-players": "{{MAX_PLAYERS}}",
          },
        },
      },
      startup: { done: "Server started." },
      stop: "stop",
      logs: {},
    },
    env: [
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum number of players.", defaultValue: "10", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:30" },
      { name: "Gamemode", envVariable: "GAMEMODE", description: "0=survival, 1=creative.", defaultValue: "0", userViewable: true, userEditable: true, rules: "required|integer|in:0,1" },
      { name: "Difficulty", envVariable: "DIFFICULTY", description: "0=peaceful, 1=easy, 2=normal, 3=hard.", defaultValue: "2", userViewable: true, userEditable: true, rules: "required|integer|in:0,1,2,3" },
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Server name.", defaultValue: "Bedrock Level", userViewable: true, userEditable: true, rules: "required|string|max:64" },
    ],
    features: ["pid_limit"],
    fileDenylist: [],
  },
  {
    id: "teamspeak3",
    name: "TeamSpeak 3",
    description: "TeamSpeak 3 voice server. The high-quality voice communication platform for online multiplayer gaming and communities.",
    game: "TeamSpeak 3",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:ubuntu",
    images: { Ubuntu: "ghcr.io/pterodactyl/yolks:ubuntu" },
    startup: "./ts3server default_voice_port={{SERVER_PORT}} query_port={{QUERY_PORT}} filetransfer_port={{FILE_PORT}} serveradmin_password='{{SERVERADMIN_PASSWORD}}'",
    installContainer: "ghcr.io/pterodactyl/installers:debian",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# TeamSpeak 3 Installation Script
# Server Files: /mnt/server
cd /mnt/server
DOWNLOAD_URL="https://files.teamspeak-services.com/releases/server/3.13.7/teamspeak3-server_linux_amd64-3.13.7.tar.bz2"
curl -L -o ts3.tar.bz2 "$DOWNLOAD_URL"
tar -xf ts3.tar.bz2 --strip-components=1
rm ts3.tar.bz2
touch .ts3server_license_accepted`,
    config: {
      startup: { done: "listening on" },
      stop: "exit",
      logs: {},
    },
    env: [
      { name: "Query Port", envVariable: "QUERY_PORT", description: "ServerQuery port.", defaultValue: "10011", userViewable: true, userEditable: false, rules: "required|integer|min:1024|max:65535" },
      { name: "File Transfer Port", envVariable: "FILE_PORT", description: "File transfer port.", defaultValue: "30033", userViewable: true, userEditable: false, rules: "required|integer|min:1024|max:65535" },
      { name: "Admin Password", envVariable: "SERVERADMIN_PASSWORD", description: "Server admin password. If left empty, a random one is generated on first start.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:64" },
    ],
    features: ["pid_limit"],
    fileDenylist: [],
  },
  {
    id: "zomboid",
    name: "Project Zomboid",
    description: "Project Zomboid dedicated server. The ultimate zombie survival simulation game with deep crafting, building, and permadeath mechanics.",
    game: "Project Zomboid",
    author: "GamePanel (from PufferPanel template)",
    image: "ghcr.io/pterodactyl/yolks:steamcmd",
    images: { SteamCMD: "ghcr.io/pterodactyl/yolks:steamcmd" },
    startup: "./start-server.sh -servername {{SERVER_NAME}} -adminpassword {{ADMIN_PASSWORD}} -port {{SERVER_PORT}}",
    installContainer: "ghcr.io/pterodactyl/installers:steamcmd",
    installEntrypoint: "bash",
    installScript: `#!/bin/bash
# Project Zomboid Installation Script
# Server Files: /mnt/server
mkdir -p /mnt/server/steamcmd
cd /mnt/server/steamcmd
curl -sL https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz | tar zxvf -
./steamcmd.sh +login anonymous +force_install_dir /mnt/server +app_update 380870 +quit`,
    config: {
      startup: { done: "SERVER STARTED" },
      stop: "quit",
      logs: {},
    },
    env: [
      { name: "Server Name", envVariable: "SERVER_NAME", description: "Name of the Zomboid server.", defaultValue: "Zomboid Server", userViewable: true, userEditable: true, rules: "required|string|max:64" },
      { name: "Admin Password", envVariable: "ADMIN_PASSWORD", description: "Admin password for server administration.", defaultValue: "changeme", userViewable: false, userEditable: true, rules: "required|string|min:4|max:32" },
      { name: "Max Players", envVariable: "MAX_PLAYERS", description: "Maximum players.", defaultValue: "16", userViewable: true, userEditable: true, rules: "required|integer|min:1|max:100" },
      { name: "Server Password", envVariable: "SERVER_PASSWORD", description: "Server password for joining.", defaultValue: "", userViewable: false, userEditable: true, rules: "nullable|string|max:32" },
    ],
    features: ["steamcmd", "pid_limit"],
    fileDenylist: [],
  },
]
