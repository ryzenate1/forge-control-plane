package main

const defaultConfigPath = "/etc/beacon/config.yml"

var Version = "beacon-dev"

// UpdatePublicKey is the base64-encoded Ed25519 release signing public key.
// Release builds must inject it with -ldflags "-X main.UpdatePublicKey=...".
var UpdatePublicKey string
