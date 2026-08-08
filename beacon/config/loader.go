package config

// Load loads the Beacon configuration from the specified paths
// and returns a combined configuration object.
func Load(paths ...string) (*Configuration, error) {
	return LoadWithOptions(LoadOptions{Paths: paths})
}
