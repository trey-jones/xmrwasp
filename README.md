# XMR WASP

XMR is a **W**ebsocket **A**nd **S**tratum **P**roxy for Monero (XMR) miners.  It accepts connections from miners over both websocket and TCP sockets using the monero stratum protocol.  Written in Go and inspired heavily by XMRig-Proxy.


## Features


* Easy to use - just configure and execute the binary.
* Configurable with either OS Environment or JSON file.
* Low system overhead per connection.
* Accepts any miner, including Web Miners!
* Works with Coinhive and that other one.
* Reduce Pool Load by using one connection for many miners.
