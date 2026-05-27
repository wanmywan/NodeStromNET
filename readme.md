
# NodeStorm-C2

![Go Version](https://img.shields.io/badge/golang-go1.25.5+-blue.svg)
[![Twitter](https://img.shields.io/twitter/follow/wanzroot?label=wanzroot&style=social)]([https://twitter.com/intent/follow?screen_name=al3x_n3ff](https://x.com/wanzroot))


```text
                    _.-'`)     (`'-._
                .' -' / __    \ '- '.
               / .-' ( '-,`|   ) '-. \
              / .-',-`'._/ \_.'`-,'-. \
             ; ; /.`'.-'    '-.'`.\ ; ;
             | .-'|\//'-/   \-'\\/|'-. |
             |` |; :|'._\   /_,'|: ;| `|
             || : |;    `Y-Y`    ;| : ||
             \:| :/======"="======\| |:/
              /_:-`                 `-;_\
            NodeStorm.Net Decentralized C2
            @HeloKity @Dr909 @Ashiraa

[*] Welcome to the NodeStrom.Net Decentralized C2, please type 'help' for options
[*] The system is patient. It always waits for its targets to reveal themselves.

```

NodeStorm-C2 is an advanced, decentralized Command and Control (C2) framework built entirely on Peer-to-Peer (P2P) architecture. Written in Go, it leverages the powerful `libp2p` networking stack to provide highly resilient, secure, and untraceable communication between the Controller and its Agents.

## How It Works

Unlike traditional C2 frameworks that rely on a central server (which acts as a single point of failure and is easily identifiable by blue teams), NodeStorm-C2 operates on a distributed P2P network using Kademlia Distributed Hash Tables (DHT).

1. **Discovery & Routing**: The framework connects to public bootstrap peers (such as IPFS bootstrap nodes) to form its network. Agents and the Controller discover each other through a shared "Room", which is securely generated via a salted cryptographic hash of a room name and password.
2. **Beaconing & PubSub**: Agents use GossipSub (a PubSub protocol in libp2p) to securely broadcast their heartbeat and system metadata (Hostname, OS, Username, etc.) to the Controller.
3. **NAT Traversal & Hole Punching**: Thanks to `libp2p`, NodeStorm-C2 automatically handles complex networking setups. It supports UPnP/NAT-PMP, TCP/UDP Hole Punching, and fallback Auto-Relays. This means Agents can directly talk to the Controller even if both are sitting behind strict firewalls or Carrier-Grade NATs, without needing exposed inbound ports.
4. **Encrypted Streams**: When the Controller interacts with an Agent, a direct multiplexed stream is opened. All traffic within this stream—including commands, files, and proxy data—is symmetrically encrypted using your unique shared password.

## Key Advantages

- **Zero Central Infrastructure**: No need to purchase, configure, or hide a central C2 server, domain name, or use redirectors. The global P2P network itself is the infrastructure. This makes infrastructure takedowns extremely difficult.
- **Unstoppable Connections**: Advanced NAT hole-punching and relaying mechanisms ensure you can reliably interact with agents regardless of inbound port restrictions or firewall configurations.
- **End-to-End Security**: On top of libp2p's native transport security, NodeStorm-C2 employs an additional layer of custom symmetrical payload encryption. Only nodes that possess the correct room password can decrypt commands and join the swarm.
- **Multi-Platform Support**: Built natively in Go, it supports Windows, Linux, and macOS out of the box.
- **Stealth & Resilience**: Agents run in background loops with jitter incorporated into their beacons to avoid predictable network patterns. They can ignore termination signals or run masked/as a service.

## Features

The Controller provides a beautiful, syntax-highlighted Command Line Interface (CLI) with tab-autocompletion. Supported features include:

* **`agents`**: List all active swarm agents with real-time status.
* **`cmd <id> <command>`**: Execute an arbitrary shell command on the target end-to-end encrypted.
* **`shell <id>`**: Spawn a fully interactive Remote Shell (supports PTY/Raw mode on Unix, and stealth PowerShell on Windows).
* **`upload` / `download`**: Fast, chunked, and encrypted file transfers to and from the agent.
* **`socks <id> <port>`**: Spin up a SOCKS5 proxy securely tunneled through the P2P connection, providing raw access to the target's internal network.
* **`fwd <id> <target> <local_port>`**: Port forward any internal service through the agent back to your local machine.
* **`memexec <id> <binary> [args]`**: *(Linux Only)* Fileless memory execution. Run ELF binaries directly from memory without dropping them on the disk.
* **`screenshot <id>`**: *(Windows Only)* Take stealthy screen captures of the target's desktop.
* **`keylog <id> [START|STOP|DUMP|CLEAR]`**: *(Windows Only)* Background asynchronous keylogger that tracks keyboard inputs to a hidden temporary log.

## Quick Start & Usage

Go Setup: [Wiki](https://github.com/wanmywan/NodeStromNET/wiki)
