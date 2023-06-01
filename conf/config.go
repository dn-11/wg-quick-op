package conf

import (
	_ "embed"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"time"
)

//go:embed config-sample.yaml
var configSample []byte

var DDNS struct {
	Interval time.Duration
	Iface    []string
}

var Enabled []string

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
	update()
	if err != nil {
		logrus.WithError(err).Fatalf("read config from %s failed", file)
	}
}

func update() {
	DDNS.Interval = time.Duration(viper.GetInt("ddns.interval")) * time.Second
	DDNS.Iface = viper.GetStringSlice("ddns.iface")
	Enabled = viper.GetStringSlice("enabled")
}
