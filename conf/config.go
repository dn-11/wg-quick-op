package conf

import (
	_ "embed"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"time"
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

var Log struct {
	Level logrus.Level
}

func Init(file string) {
	if _, err := os.Stat(file); err != nil {
		if !os.IsNotExist(err) {
			logrus.WithError(err).Fatalf("get stat of %s failed", file)
		}
		logrus.Infof("config not existed, creating at %s", file)
		created, err := os.Create(file)
		if err != nil {
			logrus.WithError(err).Fatalf("create config at %s failed", file)
		}
		if _, err := created.Write(configSample); err != nil {
			logrus.WithError(err).Fatalf("write config at %s failed", file)
		}
	}

	viper.SetConfigFile(file)
	err := viper.ReadInConfig()

	viper.SetDefault("ddns.interval", 60)
	viper.SetDefault("ddns.handshake_max", 150)

	update()
	if err != nil {
		logrus.WithError(err).Fatalf("read config from %s failed", file)
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

	if level, err := logrus.ParseLevel(viper.GetString("log.level")); err == nil {
		Log.Level = level
		logrus.SetLevel(level)
	}
}
