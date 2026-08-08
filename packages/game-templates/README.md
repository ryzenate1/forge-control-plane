# Game Templates

Pre-configured game server templates for easy deployment in GamePanel.

## 📁 Structure

- `src/` - Source TypeScript code for template management
- `dist/` - Compiled output
- `templates/` - Game template definitions
- `scripts/` - Build and utility scripts
- `index.json` - Index of all available templates
- `template-schema.json` - JSON schema for template validation

## 🎮 Supported Games

This package includes templates for popular game servers. See `index.json` for the complete list of supported games and their configurations.

## 🚀 Usage

### Adding a New Template

1. Create a new template file in `templates/`
2. Add the template metadata to `index.json`
3. Validate against `template-schema.json`

### Template Schema

Each template must conform to the schema defined in `template-schema.json`:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "description": {"type": "string"},
    "dockerImage": {"type": "string"},
    "ports": {"type": "array", "items": {"type": "number"}},
    "environment": {"type": "object"},
    "volumes": {"type": "array"},
    "startup": {"type": "string"}
  },
  "required": ["name", "dockerImage"]
}
```

### Variable Substitution Contract

Templates use `{{VARIABLE}}` placeholders in the `startup` command and `config.files`
sections. These are resolved by the Forge runtime (wings protocol) when a server is
started:

- **Built-in server variables** — always available, resolved by the daemon from the
  server's allocation:

  | Variable | Value |
  | --- | --- |
  | `{{SERVER_PORT}}` | The server's actual allocated port |
  | `{{SERVER_IP}}` | The server's allocated IP address |
  | `{{SERVER_MEMORY}}` | The server's memory limit in MB |
  | `{{server.build.default.port}}` | The default allocation port |
  | `{{server.build.default.ip}}` | The default allocation IP |

- **Template env variables** — any placeholder matching the `env_variable` of an
  entry in the template's `env[]` array is replaced with that variable's value
  (`default_value` unless overridden by the user). Each template's `env[]` list is
  the authoritative variable list for that template; every placeholder used in
  `startup`/`config.files` must be either a built-in server variable or defined in
  `env[]`.

`scripts/validate-templates.mjs` enforces this contract: it fails the build if a
template references a placeholder that is neither a built-in server variable nor a
defined env variable.

> ⚠️ Do not use `{{PORT}}` in templates — it is not a built-in and will be passed
> through to the game server literally. Use `{{SERVER_PORT}}`.

## 📦 Installation

```bash
npm install @gamepanel/game-templates
```
