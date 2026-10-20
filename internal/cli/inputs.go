package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/profile"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

// inputs is everything the commands read from disk. config is nil when the
// default config file does not exist, which list tolerates and sync will not.
type inputs struct {
	manifest *skill.Manifest
	config   *profile.Config
}

// defaultPath resolves a data or config file: an explicit environment
// variable wins, then the XDG base directory, then its conventional fallback.
func defaultPath(envVar, xdgVar, fallbackDir, rel string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	base := os.Getenv(xdgVar)
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, fallbackDir)
	}
	return filepath.Join(base, rel)
}

// defaultLibraryPath is the directory Nix lays sources out in, one
// subdirectory per source.
func defaultLibraryPath() string {
	return defaultPath("LOADOUT_LIBRARY", "XDG_DATA_HOME", ".local/share", "loadout/library")
}

func defaultConfigPath() string {
	return defaultPath("LOADOUT_CONFIG", "XDG_CONFIG_HOME", ".config", "loadout/loadout.hjson")
}

// defaultCachePath is where patched skill copies are built.
func defaultCachePath() string {
	return defaultPath("LOADOUT_CACHE", "XDG_CACHE_HOME", ".cache", "loadout/skills")
}

// defaultLaunchPath is where generated --settings and --mcp-config files go.
func defaultLaunchPath() string {
	return defaultPath("LOADOUT_LAUNCH", "XDG_CACHE_HOME", ".cache", "loadout/launch")
}

func addInputFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("library", defaultLibraryPath(), "directory of skill sources, one subdirectory per source")
	cmd.PersistentFlags().String("manifest", "", "read this manifest.json instead of scanning the library (also LOADOUT_MANIFEST)")
	cmd.PersistentFlags().String("config", defaultConfigPath(), "path to loadout.hjson")
	cmd.PersistentFlags().String("sources", "", "path to sources.json (default: next to the config file, also LOADOUT_SOURCES)")
}

// loadManifest scans the library, unless a manifest file is named: that
// override exists for tests and for pinning a catalogue by hand.
func loadManifest(cmd *cobra.Command) (*skill.Manifest, error) {
	return loadManifestTo(cmd, cmd.ErrOrStderr())
}

// loadManifestTo is loadManifest with scan warnings sent to warn, so shell
// completion can discard them instead of printing into the prompt.
func loadManifestTo(cmd *cobra.Command, warn io.Writer) (*skill.Manifest, error) {
	manifestPath, _ := cmd.Flags().GetString("manifest")
	if manifestPath == "" {
		manifestPath = os.Getenv("LOADOUT_MANIFEST")
	}
	if manifestPath != "" {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read manifest: %w", err)
		}
		manifest, err := skill.ParseManifest(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		return manifest, nil
	}

	libraryPath, _ := cmd.Flags().GetString("library")
	manifest, warnings, err := skill.ScanLibrary(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("%w (build it with `hms`, or point --library at a directory of sources)", err)
	}
	for _, w := range warnings {
		fmt.Fprintln(warn, "warning:", w)
	}
	return manifest, nil
}

func loadInputs(cmd *cobra.Command) (inputs, error) {
	configPath, _ := cmd.Flags().GetString("config")

	manifest, err := loadManifest(cmd)
	if err != nil {
		return inputs{}, err
	}

	data, err := os.ReadFile(configPath)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !cmd.Flag("config").Changed:
		return inputs{manifest: manifest}, nil
	case err != nil:
		return inputs{}, fmt.Errorf("read config: %w", err)
	}
	config, err := profile.Parse(data)
	if err != nil {
		return inputs{}, fmt.Errorf("%s: %w", configPath, err)
	}
	return inputs{manifest: manifest, config: config}, nil
}
