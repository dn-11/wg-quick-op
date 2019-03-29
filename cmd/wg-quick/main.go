package main

import (
	"flag"
	"fmt"
	"github.com/nmiculinic/wg-quick-go"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
)

func printHelp() {
	fmt.Print("wg-quick [flags] [ up | down | sync ] [ config_file | interface ]\n\n")
	flag.Usage()
	os.Exit(1)
}

func main() {
	flag.String("iface", "", "interface")
	verbose := flag.Bool("v", false, "verbose")
	protocol := flag.Int("route-protocol", 0, "route protocol to use for our routes")
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		printHelp()
	}

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	iface := flag.Lookup("iface").Value.String()
	log := logrus.WithField("iface", iface)

	cfg := args[1]

	_, err := os.Stat(cfg)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		if iface == "" {
			iface = cfg
			log = logrus.WithField("iface", iface)
		}
		cfg = "/etc/wireguard/" + cfg + ".conf"
		_, err = os.Stat(cfg)
		if err != nil {
			log.WithError(err).Errorln("cannot find config file")
			printHelp()
		}
	default:
		logrus.WithError(err).Errorln("error while reading config file")
		printHelp()
	}

	b, err := ioutil.ReadFile(cfg)
	if err != nil {
		logrus.WithError(err).Fatalln("cannot read file")
	}
	c := &wgquick.Config{}
	if err := c.UnmarshalText(b); err != nil {
		logrus.WithError(err).Fatalln("cannot parse config file")
	}

	c.RouteProtocol = *protocol

	switch args[0] {
	case "up":
		if err := wgquick.Up(c, iface, log); err != nil {
			logrus.WithError(err).Errorln("cannot up interface")
		}
	case "down":
		if err := wgquick.Down(c, iface, log); err != nil {
			logrus.WithError(err).Errorln("cannot down interface")
		}
	case "sync":
		if err := wgquick.Sync(c, iface, log); err != nil {
			logrus.WithError(err).Errorln("cannot sync interface")
		}
	default:
		printHelp()
	}
}
