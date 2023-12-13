package main

import (
	"github.com/hdu-dn11/wg-quick-op/cmd"
	"github.com/sirupsen/logrus"
	"os"
)

func main() {
	logrus.SetOutput(os.Stdout)
	cmd.Execute()
}
