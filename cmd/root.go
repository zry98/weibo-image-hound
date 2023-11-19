package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"weibo-image-hound/internal/probe/globalping"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	config      *Config
	cfgFilePath string
)

type Config struct {
	Providers struct {
		GlobalPing globalping.Config `yaml:"global_ping,omitempty"`
	} `yaml:"providers,omitempty"`
	Cache struct {
		Locations map[string][]string `yaml:"locations,omitempty,flow"`
		Resolves  []net.IP            `yaml:"resolves,omitempty,flow"`
	} `yaml:"cache,omitempty"`
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "weibo-image-hound",
	Short: "A tool to hunt for uncensored Weibo images.",
	Long: `A tool to hunt for uncensored Weibo images. 
It will try its best to find an uncensored version of the image by the given URL, 
by requesting to Weibo image CDNs from different locations across the world.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(loadConfig, saveConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFilePath, "config", "", "config file (default is $HOME/.weibo-image-hound.yaml)")
	if cfgFilePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(fmt.Errorf("failed to get user home directory: %w", err))
		}
		cfgFilePath = filepath.Join(home, ".weibo-image-hound.yaml")
	}
}

// loadConfig loads the configuration from the file at cfgFilePath.
func loadConfig() {
	f, err := os.ReadFile(cfgFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Config file not found at %s, creating one.\n", cfgFilePath)
			if _, err = os.Create(cfgFilePath); err != nil {
				panic(fmt.Errorf("failed to create config file: %w", err))
			}
		} else {
			panic(fmt.Errorf("failed to read config file: %w", err))
		}
	}

	if err = yaml.Unmarshal(f, &config); err != nil {
		panic(fmt.Errorf("failed to parse config file: %w", err))
	}
	if config == nil {
		config = &Config{}
	}
}

// saveConfig saves the current configuration to the file at cfgFilePath.
func saveConfig() {
	f, err := os.Create(cfgFilePath)
	if err != nil {
		panic(fmt.Errorf("failed to create config file: %w", err))
	}
	defer f.Close()

	b, err := yaml.Marshal(config)
	if err != nil {
		panic(fmt.Errorf("failed to marshal config: %w", err))
	}

	if _, err = f.Write(b); err != nil {
		panic(fmt.Errorf("failed to write config file: %w", err))
	}
}
