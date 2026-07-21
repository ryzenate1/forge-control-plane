# @forge/game-templates

Game server template catalog for **Forge** — Forge's answer to Pterodactyl/Pelican **eggs** and Coolify's 179+ service templates.

Provides one-click deploy templates for popular game servers:

| Template | Game | Port(s) | Image |
|---|---|---|---|
| `minecraft-paper` | Minecraft | 25565 TCP | ghcr.io/pterodactyl/yolks:java_21 |
| `minecraft-vanilla` | Minecraft | 25565 TCP | ghcr.io/pterodactyl/yolks:java_21 |
| `palworld` | Palworld | 8211 UDP, 27015 UDP | ghcr.io/parkervcp/steamcmd:debian |
| `valheim` | Valheim | 2456-2458 UDP | ghcr.io/parkervcp/steamcmd:debian |
| `terraria` | Terraria | 7777 TCP | ghcr.io/parkervcp/steamcmd:debian |
| `enshrouded` | Enshrouded | 15636 UDP | ghcr.io/parkervcp/steamcmd:debian |
| `satisfactory` | Satisfactory | 7777 UDP | ghcr.io/parkervcp/steamcmd:debian |

## Structure

```
packages/game-templates/
├── package.json              # NPM package
├── index.json                # Template registry (metadata index)
├── template-schema.json      # JSON Schema for template validation
├── README.md                 # This file
├── scripts/
│   └── validate-templates.mjs # Pre-build validation script
└── templates/
    ├── minecraft-paper.json
    ├── minecraft-vanilla.json
    ├── palworld.json
    ├── valheim.json
    ├── terraria.json
    ├── enshrouded.json
    └── satisfactory.json
```

## Template Format

Each template is a JSON file compatible with Forge's **Egg** model (see `forge/api/internal/store/store_nests.go`). Key fields:

| Field | Description |
|---|---|
| `id` | Unique kebab-case identifier |
| `name`, `description` | Display info |
| `image` / `images` | Docker image(s) for the server container |
| `startup` | Startup command with `{{VARIABLE}}` substitution |
| `config.files` | File parsing config for in-place config injection |
| `config.startup.done` | String to detect server startup completion |
| `config.stop` | Stop command |
| `ports` | Port definitions (port, protocol, public) |
| `env` | EggVariable definitions with validation rules |
| `resources` | Default CPU/memory/disk allocation |
| `install_script` | Installation container script (SteamCMD, curl, etc.) |
| `categories` | Classification tags |
| `supported_platforms` | `["docker", "podman"]` |

## Usage

```typescript
import { listTemplates, getTemplate } from '@forge/game-templates';

// List all available templates
const templates = await listTemplates();

// Get a specific template
const paper = await getTemplate('minecraft-paper');

// Create a server from a template (via API)
const server = await api.createServer({
  templateId: 'minecraft-paper',
  name: 'My Survival World',
  variables: {
    MINECRAFT_VERSION: '1.20.4',
    MAX_PLAYERS: '10',
    DIFFICULTY: 'hard'
  }
});
```

## Development

Add a new template:

1. Create `templates/<id>.json` following the schema
2. Add its entry to `index.json` under `registry`
3. Run `npm run prebuild` to validate

```bash
npm run prebuild
```
