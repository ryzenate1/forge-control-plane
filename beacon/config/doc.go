// Package config provides type-safe, validated configuration for the beacon
// daemon.
//
// Configuration is assembled from multiple sources, applied in order of
// increasing precedence:
//
//  1. Typed defaults declared as [ConfigEntry] values and returned by [Default].
//  2. A YAML file read via [Load], [LoadFromSources], or [LoadWithOptions].
//  3. Environment variables with the DAEMON prefix (configurable via the
//     envPrefix parameter); the dotted key path is mapped to an env var by
//     replacing "." with "_" (e.g. system.api.host → DAEMON_SYSTEM_API_HOST).
//  4. Optional pflag flags supplied via [LoadWithOptions].
//
// [LoadFromSources] and [LoadWithOptions] merge sources and run
// [Configuration.Validate] before publishing the result as the package-level
// global (accessible via [Get]). [Load] is the backward-compatible YAML-only
// entry point that preserves historical behaviour: it does not consult
// environment variables or flags and does not run validation.
//
// Type-safe accessor methods (e.g. [Configuration.APIConfig],
// [Configuration.SFTPConfig], [Configuration.DataDir]) return copies of
// slice/map values and are safe for concurrent use.
package config
