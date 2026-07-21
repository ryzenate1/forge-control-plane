// Package config provides type-safe, validated configuration for the beacon
// daemon.
//
// Configuration is assembled from multiple sources, applied in order of
// increasing precedence:
//
//  1. Typed defaults declared as [ConfigEntry] values and returned by [Default].
//  2. A YAML file read via [LoadWithOptions].
//  3. Environment variables with the DAEMON prefix (configurable via the
//     envPrefix parameter); the dotted key path is mapped to an env var by
//     replacing "." with "_" (e.g. system.api.host → DAEMON_SYSTEM_API_HOST).
//  4. Optional pflag flags supplied via [LoadWithOptions].
//
// [LoadWithOptions] merges sources and runs [Configuration.Validate] before
// publishing the result as the package-level global.
package config
