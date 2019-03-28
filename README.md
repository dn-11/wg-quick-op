# wg-quick-go

[![Build Status](https://gitlab.com/neven-miculinic/wg-quick-go/badges/master/pipeline.svg)](https://gitlab.com/neven-miculinic/wg-quick-go/pipelines) [![GoDoc](https://godoc.org/github.com/nmiculinic/wireguardctrl?status.svg)](https://godoc.org/github.com/nmiculinic/wg-quick-go) [![Go Report Card](https://goreportcard.com/badge/github.com/nmiculinic/wg-quick-go)](https://goreportcard.com/report/github.com/nmiculinic/wg-quick-go)

wg-quick like library in go for embedding

# Roadmap

* [x] full wg-quick feature parity
    * [x] PreUp
    * [x] PostUp
    * [x] PreDown
    * [x] PostDown
    * [x] DNS
    * [x] MTU
    * [x] Save --> Use MarshallText interface to save config
* [x] Sync
* [x] Up
* [x] Down
* [x] MarshallText
* [x] UnmarshallText
* [x] Minimal test
* [ ] Integration tests ((TODO; have some virtual machines/kvm and wreck havoc :) ))

# Caveats

* Endpoints DNS MarshallText is unsupported
* Pre/Post Up/Down doesn't support escaped `%i`, that is all `%i` are expanded to interface name.
* SaveConfig in config is only a placeholder (( since there's no reading/writing from files )). Use Unmarshall/Marshall Text to save/load config (( you're responsible for IO)).
