# XMR WASP

XMR is a **W**ebsocket **A**nd **S**tratum **P**roxy for Monero (XMR) miners.  It accepts connections from miners over both websocket and TCP sockets using the monero stratum protocol.  Written in Go and inspired heavily by XMRig-Proxy.

## Features

* Easy to use - just configure and execute the binary.
* Configurable with either OS Environment or JSON file.
* Low system overhead per connection.
* Accepts any miner, including Web Miners!
* Works with Coinhive and that other one.
* Reduce Pool Load by using one connection for many miners.

## Installation

### Download

[Binary releases are available for most platforms](https://github.com/trey-jones/xmrwasp/releases)

Move to a directory in your system PATH if desired.

### Install with Go Toolchain

As long as you don't pass the `-d` flag, `go get` will install the package as well.

`go get -u github.com/trey-jones/xmrwasp`

## Usage

### Configuration

Configuration can be done via system environment variables, or by invoking `xmrwasp` with the `-c` flag, and passing a path to a JSON config file.

#### Examples (unix)
##### Globally Installed

```bash
source example/example.env
xmrwasp &
```

`xmrwasp -c example/example.config.json &`

##### Not Globally Installed

```bash
export XMRWASP_URL='xmrpool.eu:3333'
export XMRWASP_LOGIN=47sfBPDL9qbDuF5stdrE8C6gVQXf15SeTN4BNxBZ3Ahs6LTayo2Kia2YES7MeN5FU7MKUrWAYPhmeFUYQ3v4JBAvKSPjigi
export XMRWASP_PASSWORD=x
export XMRWASP_LOG=/tmp/xmrwasp.log
./xmrwasp &
```

## Support This Project

The project has a donation mechanism built in.  By default it donates (without breaking your pool connection) 1 minute and 48 seconds (donate=3) of every hour of mining to me.  This is configurable down to 36 seconds (donate=1) per hour.  Or higher if you feel generous. Saavy users of course may find that they can disable the donation mechanism.  If you do so, consider a one time donation if you would be so kind:

* Monero: `47sfBPDL9qbDuF5stdrE8C6gVQXf15SeTN4BNxBZ3Ahs6LTayo2Kia2YES7MeN5FU7MKUrWAYPhmeFUYQ3v4JBAvKSPjigi`
* BTC: `1NwemnZSLhJLnNUbzXvER6yNX55pv9tAcv`
