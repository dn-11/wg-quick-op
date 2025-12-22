# wg-quick-op

[![Go Report Card](https://goreportcard.com/badge/github.com/hdu-dn11/wg-quick-op)](https://goreportcard.com/report/github.com/hdu-dn11/wg-quick-op)

based on [wg-quick-go](https://github.com/nmiculinic/wg-quick-go), but with more feature for openwrt

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

1. run `wg-quick-op install` to install wg-quick-op
2. run `service wg-quick-op enable` to enable service
3. run `service wg-quick-op start` to start service
4. edit `/etc/wg-quick-op.yaml` to config the interface that you want to start with system or needs ddns resolve
5. run `service wg-quick-op restart` to restart service and apply config

## Update & Security Notice

This project provides an optional `update` command for convenience.

When executing `wg-quick-op update`, the tool may connect to the network to check for and download a newer release, and replace the local executable file. By default, the update process may attempt to fetch releases from a person-maintained mirror (maintainer: GitHub ID `macarons-s`, community ID `Macarons`, ASN `AS4211110722`), and fall back to the official GitHub Releases if the mirror is unavailable.

Users can explicitly control the update source:

- `--source github`  
  Only use the official GitHub Releases as the update source.
  (https://api.github.com/repos/dn-11/wg-quick-op/releases)
- `--source mirror`  
  Only use the mirror site maintained by the update function contributor.
  (https://mirror.jp.macaronss.top:8443/github/dn-11/wg-quick-op/releases)
- `--source auto` (default)  
  Try the mirror first, then fall back to GitHub.

Using the `update` command is entirely optional. Users may also choose to download and install new versions manually.

Please be aware that downloading and executing binaries from the network involves inherent supply-chain and network security risks. By choosing to use the `update` functionality, you acknowledge and accept these risks and are responsible for evaluating whether the update source and network environment are trusted.

If you have specific security requirements, it is recommended to avoid using the `update` command and rely on manual installation or a self-managed update workflow.

## Project DN11

这个项目是去中心化网络 DN11 的一部分。

This repo now included in Project DN11, a decentralized network.

![image](https://github.com/user-attachments/assets/9d1b46b3-41d3-4bdb-8f89-ddf911531f37)
