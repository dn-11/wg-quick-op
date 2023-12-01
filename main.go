package main

import (
	"github.com/BaiMeow/wg-quick-op/cmd"
	"github.com/sirupsen/logrus"
	"os"
)

func main() {
	logrus.SetOutput(os.Stdout)
	cmd.Execute()
}
