# XMR WASP

XMR is a **W**ebsocket **A**nd **S**tratum **P**roxy for Monero (XMR) miners.  It accepts connections from miners over both websocket and plain TCP sockets using the monero stratum protocol.  Written in Go and inspired heavily by XMRig-Proxy.

## Goals

* Enable browser mining [in a responsible way](#a-word-about-responsibility)
* Educate site owners and users about browser mining through the [example site](https://www.xmrwasp.com) which uses this project as a proxy.

## Features

* Easy to use - just configure and execute the binary.
* No dependencies.
* Configurable with either OS Environment or JSON file.
* Low system overhead per connection.
* Accepts most miners, including Browser Miners!
* Reduce Pool Load by using one connection for many miners.

## Installation

### Download

[Binary releases are available for most platforms](https://github.com/trey-jones/xmrwasp/releases)

Move to a directory in your system PATH if desired.

### Install with Go Toolchain

As long as you don't pass the `-d` flag, `go get` will install the package as well.

`go get -u github.com/trey-jones/xmrwasp`

### Get the Docker Image

`docker pull treyjones/xmrwasp`

## Usage

### Binary Releases

To use XMR WASP just execute the binary as in the examples (unix):

`xmrwasp -c example/example.config.json &`

```bash
XMRWASP_URL='xmrpool.eu:3333' \
XMRWASP_LOGIN='47sfBPDL9qbDuF5stdrE8C6gVQXf15SeTN4BNxBZ3Ahs6LTayo2Kia2YES7MeN5FU7MKUrWAYPhmeFUYQ3v4JBAvKSPjigi.250+env_example' \
XMRWASP_PASSWORD='x' \
xmrwasp
```

To run in the background and write to a log file instead of stdout:

```bash
XMRWASP_URL='xmrpool.eu:3333' \
XMRWASP_LOGIN='47sfBPDL9qbDuF5stdrE8C6gVQXf15SeTN4BNxBZ3Ahs6LTayo2Kia2YES7MeN5FU7MKUrWAYPhmeFUYQ3v4JBAvKSPjigi.250+env_example' \
XMRWASP_PASSWORD='x' \
XMRWASP_LOG='xmrwasp.log'
xmrwasp &
tail -f xmrwasp.log
```

### Docker

#### Environment Config

```bash
docker run --rm -e 'XMRWASP_URL=xmrpool.eu:3333' \
    -e 'XMRWASP_LOGIN=47sfBPDL9qbDuF5stdrE8C6gVQXf15SeTN4BNxBZ3Ahs6LTayo2Kia2YES7MeN5FU7MKUrWAYPhmeFUYQ3v4JBAvKSPjigi.250+env_example' \
    -e 'XMRWASP_PASSWORD=x'
    treyjones/xmrwasp
```

#### File Config

```bash
docker run --rm -v $(pwd)/example/example.config.json:/config \
    treyjones/xmrwasp -c example.config.json
```

### Run the example with docker-compose

```bash
cd example
docker-compose up
```

### Configuration

Configuration can be done via system environment variables, or by invoking `xmrwasp` with the `-c` flag, and passing a path to a JSON config file.

#### Required Configuration Options

Environment | JSON |
----------- | ---- | ------------
XMRWASP_URL | url | Address of mining pool you will connect to. Without protocol.
XMRWASP_LOGIN | login | Login (often your monero address) used to connect to the mining pool.
XMRWASP_PASSWORD | password | Password used to connect to the mining pool.

#### All Configuration Options

Environment | JSON | Default |
----------- | ---- | ------- | ------------
XMRWASP_NOWEB | noweb | false | Don't serve websocket connections.
XMRWASP_NOTCP | notcp | true | Don't serve stratum+tcp connections.
XMRWASP_WSPORT | wsport | 8080 | Port to listen for websocket connections.
XMRWASP_STRPORT | strport | 1111 | Port to listen for stratum+tcp connections.
XMRWASP_WSS | wss | false | If true, try to serve websocket connections with TLS encryption.
XMRWASP_TLSCERT | tlscert | "" | Path to a TLS certificate file.  Required for `wss = true`
XMRWASP_TLSKEY | tlscert | "" | Path to private key used to create the above certificate. Required for `wss = true`
XMRWASP_STATS | stats | 60 | XMR WASP will print a report to the log at this interval of seconds.
XMRWASP_LOG | log | STDOUT | Path to your desired log file.  Will be created if necessary.  Takes precedence over `nolog`
XMRWASP_NOLOG | nolog | false | If true, no log will be generated and nothing will be written to STDOUT.
XMRWASP_DONATE | donate | 2 | Percentage of mining time to do jobs for the donation server.
XMRWASP_DEBUG | debug | false | Print debug messages to the log.

## A Word About Responsibility

One of the primary features of this piece of software has to do with enabling Monero mining in the browser.  The author of this project believes that browser mining **can** be a win-win.  Even a win-win-win (users, website owners, and the blockchain).  The users **can** win by not being subjected to ads, if browser mining proves to be lucrative enough for a site owner to stay afloat.  But they can also lose, and lose badly, if the site owner doesn't excercise some restraint over the opportunity.  Here are some things that you, as a site owner probably know, but might be tempted to *forget* for the sake of maybe mining a few extra Moneroj:

* Don't mine on mobile devices, lest you destroy their battery and their ability to browse your site.
* Don't mine without telling your users that you are doing it.
* Give your users a say in whether they participate in mining, and how much.
* Throttling the browser miner is a **good idea for everyone**.

## Roadmap

* Max Proxy Lifetime and Eventual Spindown
* Web Interface exposing current status and history
* Multiple Pool Configs for fallback
* Performance Improvements: Faster release of memory on broken connections
* User Feedback?
* TLS support for stratum listeners
* Just tons of little things that could be better
* Linux repositories?

## Support This Project

The project has a donation mechanism built in.  By default it donates (without breaking your pool connection) 1 minute and 12 seconds (donate=2) of every hour of mining.  This is configurable down to 36 seconds (donate=1) per hour.  Or higher if you feel generous. Saavy users of course may find that they can disable the donation mechanism.  If you do so, consider a one time donation if you would be so kind:

* Monero: `47sfBPDL9qbDuF5stdrE8C6gVQXf15SeTN4BNxBZ3Ahs6LTayo2Kia2YES7MeN5FU7MKUrWAYPhmeFUYQ3v4JBAvKSPjigi`
* Bitcoin: `1NwemnZSLhJLnNUbzXvER6yNX55pv9tAcv`

### Support Through Development

I will gladly take a look at any pull requests that come my way.  In particular if you wanted to take a stab at any of the following types of items that don't align very well with my skillset and interests:

* Making the [example app](https://www.xmrwasp.com) look nice
* Building (or mocking up) a nice looking dashboard for a future web interface feature
* Making a badass logo/emblem to represent the project.
* Enhancing the appearance of the logging output.
* Testing on Windows
