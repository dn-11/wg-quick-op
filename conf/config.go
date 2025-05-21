package conf

import (
	_ "embed"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

//go:embed config-sample.toml
var configSample []byte

var DDNS struct {
	Interval       time.Duration
	IfaceOnly      []string
	IfaceSkip      []string
	HandleShakeMax time.Duration
}

var StartOnBoot struct {
	Enabled   bool
	IfaceOnly []string
	IfaceSkip []string
}

var EnhancedDNS struct {
	DirectResolver struct {
		Enabled   bool
		ROAFinder string
	}
}

// Wireguard used to change default value of Wireguard
var Wireguard struct {
	MTU        int
	RandomPort bool
}

var Log struct {
	Level zerolog.Level
}

func Init(file string) {
	if _, err := os.Stat(file); err != nil {
		if !os.IsNotExist(err) {
			log.Fatal().Err(err).Msgf("get stat of %s failed", file)
		}
		log.Info().Msgf("config not existed, creating at %s", file)
		created, err := os.Create(file)
		if err != nil {
			log.Fatal().Err(err).Msgf("create config at %s failed", file)
		}
		if _, err := created.Write(configSample); err != nil {
			log.Fatal().Err(err).Msgf("write config at %s failed", file)
		}
	}

	viper.SetConfigFile(file)
	err := viper.ReadInConfig()

	viper.SetDefault("ddns.interval", 60)
	viper.SetDefault("ddns.handshake_max", 150)
	viper.SetDefault("wireguard.MTU", 1420)
	viper.SetDefault("wireguard.random_port", false)

	update()
	if err != nil {
		log.Fatal().Err(err).Msgf("read config from %s failed", file)
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		update()
	})
	viper.WatchConfig()
}

func update() {
	DDNS.Interval = time.Duration(viper.GetInt("ddns.interval")) * time.Second
	DDNS.HandleShakeMax = time.Duration(viper.GetInt("ddns.handshake_max")) * time.Second
	DDNS.IfaceOnly = viper.GetStringSlice("ddns.only_ifaces")
	DDNS.IfaceSkip = viper.GetStringSlice("ddns.skip_ifaces")

	StartOnBoot.Enabled = viper.GetBool("start_on_boot.enabled")
	StartOnBoot.IfaceOnly = viper.GetStringSlice("start_on_boot.only_ifaces")
	StartOnBoot.IfaceSkip = viper.GetStringSlice("start_on_boot.skip_ifaces")

	EnhancedDNS.DirectResolver.Enabled = viper.GetBool("enhanced_dns.direct_resolver.enabled")
	EnhancedDNS.DirectResolver.ROAFinder = viper.GetString("enhanced_dns.direct_resolver.roa_finder")

	if level, err := zerolog.ParseLevel(viper.GetString("log.level")); err == nil {
		Log.Level = level
		zerolog.SetGlobalLevel(level)
	}

	Wireguard.MTU = viper.GetInt("wireguard.MTU")
	Wireguard.RandomPort = viper.GetBool("wireguard.random_port")
}
