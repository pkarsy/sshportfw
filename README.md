# sshportfw

Current version: **0.7.0**. `sshportfw -v` prints the version of a binary, and released executables are on the
[Releases page](https://github.com/pkarsy/sshportfw/releases).

sshportfw performs automatic SSH port forwarding for SSH-secured appliances and servers. It is aimed at users and administrators who rely heavily on ssh, and it assumes that every appliance should be reachable over SSH even when it is on the same network as the workstation.

The daemon listens on local addresses such as 127.0.5.1:8080. When a browser connects to that address, sshportfw opens an ssh connection to the OpenWrt router and forwards the traffic. The equivalent manual command, which must run before the page is opened, is:
```sh
> ssh -L127.0.5.1:8080:127.0.0.1:80 applianceHostname  # (or the IP)
```

With sshportfw this process is automatic. Some familiarity with ~/.ssh/config is required, because sshportfw delegates the connection to the ssh client; port forwardings themselves do not belong there, they are defined in `~/.config/sshportfw/forwardings.json`.

Typical uses are [OpenWrt](https://openwrt.org/) routers, [Syncthing](https://syncthing.net/) web interfaces, printer pages, and in general services that, for security reasons, listen only on the localhost interface. The ssh client also makes it easy to bypass firewalls and reach remote print queues, with a level of security, maturity and flexibility that is hard to match.

## STEP 1: Installation of the executable
The program is tested on Linux only. A Linux amd64 executable is published on the Releases page; when the newest tag has no binary, build from source as shown below. It should run on any modern Linux PC. It can probably be compiled for other platforms, but those are untested; see **Other platforms** below.
```sh
# Download the executable from the latest release, then
> chmod +x sshportfw
# Copy/move the "sshportfw" binary somewhere in the PATH
# Or, if you have git and Go installed
> git clone https://github.com/pkarsy/sshportfw
> cd sshportfw
> go build
> ./sshportfw [options]
# Or compile and run in one step
> go run . [options]
```

### STEP 2: Editing the forwardings.json file
The program looks for this file:

```sh
~/.config/sshportfw/forwardings.json
```

It does not create the file. Another location can be selected with `-config`, and `-check-config` validates a file without starting anything.

A sample configuration follows. It assumes a router at 10.5.2.1 and the other LAN devices on 10.5.2.X addresses, and that local DNS resolves `router` or `router.lan` to 10.5.2.1. DNS can be ignored entirely by using IP addresses: write "10.5.2.1" instead of "router".

```json
[
  {
    "Host": "router",
    "Comment":"Gets internet (PPPoE) from the provider's modem",
    "Comment2":"We can have as many comments as we like",
    "Forward": [
      {
        "Service": "LuCi",
        "ListenAddr": "127.0.10.1:8080",
        "RemoteAddr": "127.0.0.1:80"
      },
      {
        "Service": "Kyocera",
        "Comment": "Print queue",
        "ListenAddr": "127.0.10.4:6310",
        "RemoteAddr": "10.5.2.7:631"
      }
    ]
  },
  {
    "Host": "ap1",
    "Comment":"AP 2.4+5GHz",
    "Forward": [
      {
        "Service": "LuCi",
        "ListenAddr": "127.0.15.1:8080",
        "RemoteAddr": "127.0.0.1:80"
      }
    ]
  },
  {
    "Host": "ap2",
    "Forward": [
      {
        "Service": "LuCi",
        "ListenAddr": "127.0.11.1:8080",
        "RemoteAddr": "127.0.0.1:80"
      }
    ]
  },
  {
    "Host": "rpi",
    "Forward": [
      {
        "Service": "TransmissionServer",
        "ListenAddr": "127.0.13.1:9091",
        "RemoteAddr": "127.0.0.1:9091"
      }
    ]
  },
  {
    "Host": "openwrt2",
    "Comment": "Printer1 and Printer2 are connected to, and only accessible via, openwrt2",
    "Forward": [
      {
        "Service": "LuCi",
        "ListenAddr": "127.0.14.1:8080",
        "RemoteAddr": "127.0.0.1:80"
      },
      {
        "Service": "Printer1 GUI",
        "ListenAddr": "127.0.14.2:8080",
        "RemoteAddr": "printer1.lan:80"
      },
      {
        "Service": "Printer2 GUI",
        "ListenAddr": "127.0.14.3:8080",
        "RemoteAddr": "printer2.lan:80"
      }
    ]
  }
]
```

Both addresses must use the `host:port` form; IPv6 addresses need brackets, for example `[::1]:80`.
A wildcard listen address (`:8080`, `0.0.0.0:8080`, `[::]:8080`) is refused, because it would expose the
forwarded service on every interface — the opposite of what sshportfw is for. Every `ListenAddr` must be
unique, and a `Host` may not start with `-` (ssh would parse it as an option).

The "Host" field can be a hostname or an IP, a `user@host` string, or a **Host entry in ~/.ssh/config** (the preferred form).

The daemon listens on "ListenAddr": "127.0.10.1:8080" and so on, but contacts no SSH server until a browser connects to "http://127.0.10.1:8080". sshportfw then uses the ssh client to connect to the router and forward the local data to 127.0.0.1:80 on the remote machine — in this case the LuCI configuration page.

The browser may warn about an insecure connection. This is harmless: all traffic is tunnelled over ssh and decrypted only at the remote host. No guarantee is implied, as I am not a security expert. To avoid genuinely insecure connections — those that carry cleartext over the network or do not authenticate the peer — block the service at the remote firewall and allow it only on the remote "lo" interface.

The `forwardings.json` file is deliberately minimal. Fields other than Host, Service, ListenAddr and RemoteAddr (for example Comment) are ignored. For anything else, the powerful ~/.ssh/config file can be used by creating a new "Host" entry.

### STEP 3: Log in to every SSH server manually using the command line
```sh
> ssh router # The same host as the host inside forwardings.json
# Or, if the server inside forwardings.json is "10.5.2.1"
> ssh 10.5.2.1
```
Accept the unknown-host message on the first connection, after verifying that the host is the correct one, and then log out. Skipping this step makes the connection fail; ssh's explanation (unknown host key, authentication failure, unreachable host) appears in sshportfw's output, or in syslog when running with `-s`.

### STEP 4: Running the program
After editing `forwardings.json`, run the program manually once and check its output.
If forwarding is needed permanently — for example for a printer — add the program to the desktop's startup applications. `DISPLAY` is no longer required, but ssh may still need a terminal for a passphrase or a security-key touch, so a cron job has nowhere to prompt: use the desktop's Startup Applications, or provide an ssh agent and `SSH_ASKPASS`. Redirect the output to a file to keep a record of problems, or use `-s` to write to syslog.

To check whether a forwarding works without opening a browser, run
```sh
> sshportfw -check
checking 11 forwarding(s) from /home/user/.config/sshportfw/forwardings.json
ok    router-LuCi                  127.0.10.1:8080        -> 127.0.0.1:80
FAIL  printer-GUI                 127.0.14.3:8080        -> printer2.lan:80 : ssh: Could not resolve hostname printer2.lan
1 of 11 forwarding(s) failed
```
`-check` opens one `ssh -W` connection per service, waits up to 5 seconds for each and closes it again.
It starts no listener and takes no lock, so it can run while sshportfw is running. With a security key
this means one touch per service, unless a control socket is reused.

Failures are also written to `/dev/tty` when one is available, at most once per service per minute, so they
remain visible when the log goes to syslog (`-s`) or to a file (`-o`). Without a controlling terminal
(systemd, cron) that message is dropped and the log is the only place to look. `-o /dev/null` therefore
silences the log, but not the one-line messages on a desktop terminal.

### Signals
| signal | what it does |
|---|---|
| `SIGHUP` | Restarts the process with the executable on disk, keeping the same PID. Use it after replacing the binary. Open forwarded connections are closed. |
| `SIGUSR1` | Re-reads `forwardings.json` and restarts the listeners in place. Open forwarded connections are not affected. |
| `SIGTERM`, `SIGINT`, `SIGABRT` | Stops the program. |

Before restarting, sshportfw parses the new `forwardings.json` and runs the new executable with
`-check-config`. If either step fails it logs the reason and keeps the old binary running, so a broken build
or a typo cannot take the service down. The restart re-executes the path this process was started from
(`os.Executable()`), so a rebuild in place at that path is picked up; repointing the symlink that leads to
it is not.

Upgrading while it runs:
```sh
> cd ~/Projects/sshportfw && go build -o sshportfw .
> kill -HUP $(pgrep -x sshportfw)
```

When upgrading from 0.6.x, note that `SIGHUP` used to reload the configuration in place. That is now `SIGUSR1`;
`SIGHUP` restarts the process and drops the open forwards. Version 0.7.0 also refuses configurations it
used to accept, such as a listen address with no host (`:8080`), a wildcard address, the same `ListenAddr`
twice, or a `Host` starting with `-`.

The next sections describe how to configure ssh so that sshportfw becomes more useful.

## Adding functionality: Configuring ~/.ssh/config

### ~/.ssh/config: Using a control socket
At the **END** of ~/.ssh/config, add
```sh
match host * # it may be a list of hosts instead of *
    ControlMaster auto
    ControlPath ~/.ssh/ssh_mux_%h_%p_%r
    ControlPersist 300
```

A control socket makes later connections much faster, at the cost of some drawbacks described in the manual.
Do not put such global options at the beginning of the file, because subsequent entries cannot override them.

### ~/.ssh/config: Accessing LAN devices from outside
Most LANs have one public IPv4 address and private (NAT) IPv4 addresses for the devices inside. Suppose a Raspberry Pi has the **static** private address 10.5.2.2 (rpi.lan), and the router forwards a WAN port to it. The RPi is then reachable from outside at mydynamicip.freemyip.com:2002 (this topic is not covered here; consult the documentation for your router).
ssh — and therefore sshportfw — should reach that device even from a laptop outside the home, which this ~/.ssh/config entry achieves:

```sh
match host rpi !exec "ip -4 a | grep -q 10.5.2."
  # the 10.5.2 must be adapted to your actual IP range
  # works only if the router is configured to redirect incoming
  # TCP connections on port 2002 to 10.5.2.2:22
  # Also a dynamic DNS service must be configured on your router
  hostname mydynamicip.freemyip.com
  port 2002

host rpi
  user auser
  # some options but don't put forwarding rules here
```
This rule detects the 10.5.2.XX network and acts accordingly. Any other unique element of the network can be used instead. Be careful: many NATs use 192.168.0.x or 192.168.1.x, which are hard to tell apart, so less common ranges are preferable.
NOTE: with a range extender or a second router that creates a different subnet, the ssh config needs additional rules.

### ~/.ssh/config: Accessing firewalled services using a jumphost
If opening many ports on the router is undesirable (see the previous example), use a jumphost:
```sh
match host router !exec "ip -4 a | grep -q 10.5.2."
  # Works only if the host is accessible from the outside world, and usually this is the router.
  # Of course we must setup a DynamicDns service for this to work
  # Almost all routers and of course OpenWrt has good support on this
  # also the SSH server must listen to port 20202 and the WAN port 20202 to be open
  ProxyJump mydynamicip.freemyip.com
  Port 20202
```


## Command line
```sh
> sshportfw -h

-check
    Check every forwarding once and exit. One ssh connection is opened per service, so a security key may ask to be touched
-check-config
    Parse the configuration file and exit; opens no listener and no ssh connection
-config string
    Path of forwardings.json (default $XDG_CONFIG_HOME/sshportfw/forwardings.json)
-l	
-lines
    Print source code line numbers for debugging
-o string
-output string
    Redirect the log to a file. Use -o /dev/null for a quiet log; failures still appear on /dev/tty
-s	
-syslog
    redirects output to syslog
-ssh string
    ssh client to execute (default "ssh")
-t	
-time
    Print date and time for every line of output (ignored on syslog output)
-v	
-version
    prints current sshportfw version
```

Every option has a short and a long form with the same meaning (`-o` / `-output`, `-s` / `-syslog`, and so on).
Both report modes return an exit code that a script can use: `-check-config` exits 0 only if the file parses,
and `-check` exits 0 only if every forwarding answered. `-ssh` selects a different ssh client when the one found
in `PATH` is not the one you want, for example `-ssh /data/data/com.termux/files/usr/bin/ssh` on Termux.

### Using hostnames instead of IPs in the browser
- Add the line to /etc/hosts:
  ```sh
  # This local port connects with the
  # LuCI interface on our router
  127.0.14.1      routerluci.fw
  ```
  The service is then reachable at "routerluci.fw:8080".
  NOTE: adding the name to the router's DNS (hostnames section) is tempting, but it will not resolve on another network, or through a local resolver such as dnscrypt-proxy, which by default ignores the router's DNS server.

### Eliminating the port specification in the browser URL (probably a bad idea)
```url
This means
http://routerluci.fw  (the port is in fact 80)
instead of
http://routerluci.fw:8080
```
Tutorials exist for binding ports below 1024 (setcap port redirection and similar), but the gain is small and the cost is running parts of the setup as root, which adds complexity and security risk.


## Security
Use the program at your own risk: a wrong SSH configuration can expose appliances or PCs to the internet.
- On a multi-user system, every user with a shell can reach the forwarded services, at least their login pages. The program is intended for a trusted PC or laptop, not a shared machine at work or university; using an SSH client on a machine that is not yours is a security risk in itself. How much this matters depends on the importance of the server.
- ssh keeps the connection open while forwardings are active, potentially for a long time. With the ControlSocket option it stays open even afterwards; once it closes, you must log in again (touching the Yubikey) to use the service.
- Avoid password authentication: it is easily stolen or guessed. Private key files in ~/.ssh can be copied and used without your knowledge. A hardware security key is the real solution.

### Security hardening: Yubikey, Solokey, GNUK
Hardware tokens such as [Yubico](https://www.yubico.com/), [Solokey](https://solokeys.com/) or [GNUK](http://www.fsij.org/category/gnuk.html) provide strong authentication without passphrases. The private key never leaves the token; the device performs the cryptographic operations itself. Note that dropbear, the SSH server commonly used on OpenWrt, cannot use FIDO keys (the `-sk` type), so OpenSSH must be installed and configured instead. GNUK uses ordinary ssh keys, but building the hardware and configuring it is involved. More expensive tokens such as the Yubikey also support methods that dropbear accepts. Research the options, and buy two: one is the backup for the other.

## Bugs
Testing is limited: the program is used on a Linux Mint laptop with Yubico FIDO keys. If you find bugs, please report them in the issues section.
The configuration parser has unit tests; run `go test ./...` after changing it.

## Other platforms
The program is pure Go. It builds for Linux and macOS. The Windows build fails because the syslog output uses `log/syslog`, which is not available there. It is tested on Linux only. If the application runs successfully on a Mac, send the instructions so they can be included in this document.

## Alternative solutions
This section is optional and included for completeness. The problems described here are the reason sshportfw exists.


### Solution 1: plain ssh with forwarding rules in ~/.ssh/config or directly on the command line
Most people use this method. With many rules, however, the repetition becomes tiring and error-prone; automating it is exactly sshportfw's purpose.

### Solution 2: VPN (not the anonymizing providers, but self-hosted mesh overlay networks)
I tried [Zerotier](https://www.zerotier.com/) and [Nebula](https://github.com/slackhq/nebula). For complex setups with several internal (NAT) networks, Docker instances or virtual machines, a VPN is probably the better answer.

There are many downsides, however:
- Many VPNs only work when an Internet connection is available, even to reach a local node. Yet the router is needed precisely when the network has problems: the VPN works when it is not needed and fails exactly when it is.
- Remote services are constantly exposed, which is poor security practice. Port forwarding gives better control: use is visible, and with a hardware key it requires a touch.
- A VPN needs careful setup, especially of its firewall, usually in a custom rule language. This is _very_ time-consuming and error-prone, and a mistake can expose the network. The normal firewall also needs rules to accept traffic from the TUN/TAP interface.
- The servers on the remote machines (uhttpd on OpenWrt) need configuration changes: uhttpd must accept connections from the virtual network (typically a tun device), which means changes to the OpenWrt firewall *and* to the uhttpd configuration.
- If the service is not hosted on the VPN node itself, the VPN alone is not enough and custom forwarding rules are still required. A network printer at 192.168.6.25, for example, can be reached through the Raspberry Pi at 192.168.6.2: from home the RPi is reachable by VPN or ssh, but the printer is not. With SSH, all that is needed is `ssh -L127.0.7.1:6000:192.168.6.25:661` and a printer configured to use 127.0.7.1:6000 as its IPP port (TODO fix IP). With a VPN, only the RPi is reachable; the printer would need additional rules on the RPi or a redirecting daemon such as rinetd.
- Debugging becomes difficult because every node is reachable two ways: through its normal address and through the VPN address. Traffic intended for a real interface may take the VPN path, or vice versa, and firewall rules (real or virtual) can silently break the VPN.
- Additional software is required on every node. It may be unavailable for some platforms, and many OpenWrt routers do not have room for it. Most routers have only enough flash for their firmware; the Xiaomi Mi Gigabit edition, for example, is fast, has enough RAM, sustains 300 Mbps and runs OpenWrt well, but has only 8 MB of flash.

### Solution 3: Securing the web interface with SSL/TLS
Many services, including OpenWrt's uhttpd and most network printers, can serve HTTPS directly. That removes the need for both VPN and SSH forwarding, but it has serious drawbacks.

- Many router, switch and appliance web interfaces offer no TLS at all.

- Services such as uhttpd should not be exposed to the internet even over TLS; they are not hardened like openssh-server. To be fair to uhttpd, NOTHING is as battle-tested as the OpenSSH server. The security status of a network printer's built-in HTTP server is unknown, and therefore **way** more questionable than OpenWrt's uhttpd.

- Additional setup is required on the servers (TLS certificates and firewall ports).

- Every service needs valid certificates and/or configuration changes in the browser when it rejects them. Ignoring certificate errors opens the network to man-in-the-middle attacks.

- Authentication is usually just a password (LuCI, for example), which is hardly acceptable today; compare it with SSH protected by a FIDO token.

- Even worse, some services have no authentication at all (a printer queue). Accepting print jobs from the entire internet is not an option, even over TLS, while SSH port forwarding solves this cleanly.
