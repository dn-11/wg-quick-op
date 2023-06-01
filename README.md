# wg-quick-op

based on [wg-quick-op](https://github.com/nmiculinic/wg-quick-go), but with more futures for openwrt

why not use luci-app-wireguard? because it's not enough for dn11.

## What's new?

- [x] add `Table=off` option to disable auto create route table
- [x] use regexp to match config file name (use `wg-quick * up` to up all wg interfaces)
- [x] start with system (use /etc/init.d)
- [x] DDNS check and update (use sync)

## Other changes

* config in workdir is ignored, only `/etc/wireguard` is supported
* arg `iface` is removed

## How to use?

use just like using wg-quick is ok.

For additional feature, you may follow the steps below:

1. run `wg-quick-op service install` to install service to init.d
2. run `wg-quick-op service enable` to enable service
3. run `wg-quick-op service start` to start service
4. edit `/etc/wg-quick-op.yaml` to config the interface that you want to start with system or needs ddns resolve
5. run `wg-quick-op service restart` to restart service and apply config
