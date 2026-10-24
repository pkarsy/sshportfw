# sshportfw

Current version: **0.7.0**. `sshportfw -v` prints the version of a binary, and released executables are on the
[Releases page](https://github.com/pkarsy/sshportfw/releases).

A program that performs automatic SSH port forwarding whenever we need to access our SSH-secured network appliances and servers. The audience is users and administrators who rely heavily on ssh to access remote devices.
In this document we take the approach that we want to access all appliances via SSH, even if they are on the same network as our workstation.

The idea is to have sshportfw listening on local addresses such as 127.0.5.1:8080. When we point our browser at this address, the *sshportfw* daemon connects to our OpenWrt router using ssh. The same thing can be achieved with this command (which must be executed BEFORE we open the web page):
```sh
> ssh -L127.0.5.1:8080:127.0.0.1:80 applianceHostname  # (or the IP)
```

but with sshportfw the process becomes automatic. sshportfw calls *ssh* to do the actual forwarding, so some expertise in configuring ~/.ssh/config is necessary. Note, however, that you do NOT need to configure port forwardings inside ~/.ssh/config: the file ~/.config/sshportfw/forwardings.json is used instead, as we will see.

Typically port forwarding is used to access [OpenWrt](https://openwrt.org/) routers, [Syncthing](https://syncthing.net/) web interfaces, printer pages, and in general services that (for security reasons) can only serve the localhost interface. Not only that: the ssh config file is very powerful and allows bypassing firewalls, accessing remote print queues, etc., all with a level of security, maturity and flexibility that no other software offers.

## STEP 1: Installation of the executable
Note that the program is only tested on Linux.
A Linux amd64 executable is published on the Releases page; if the newest tag has no binary yet, build it from source below. It should run on any modern Linux PC. It can probably work on other platforms if compiled there, but it has not been tested. See **Other platforms** below.
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

It does not try to create it by itself. Another file can be used with `-config`, and `-check-config` verifies it without starting anything.

A sample config follows, so you can copy, paste and edit it. It assumes a router at 10.5.2.1 and the other LAN devices have 10.5.2.X addresses. It also assumes that local DNS works, for example that `router` or `router.lan` resolves to 10.5.2.1. You can ignore DNS entirely by using IP addresses: instead of "router", use "10.5.2.1".

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

Both addresses must be written as `host:port`; IPv6 addresses need brackets, for example `[::1]:80`.
A wildcard listen address (`:8080`, `0.0.0.0:8080`, `[::]:8080`) is refused because it would expose the
forwarded service on every interface, which is the opposite of what sshportfw is for. Every `ListenAddr`
must be unique, and a `Host` may not start with `-` (ssh would parse it as an option).

The "Host" can be a hostname (or an IP), a `user@host` string, or a **Host entry in ~/.ssh/config** (the preferred approach).

The program listens on "ListenAddr": "127.0.10.1:8080", etc., but does not connect to any SSH server until we point our browser at "http://127.0.10.1:8080". Then sshportfw uses the ssh client to connect to the router and forward the local data to 127.0.0.1:80 on the remote machine — the LuCI configuration page in this case.

The browser may complain about "insecure connections". This is harmless (I am not a security expert, so no guarantees), as all traffic is tunnelled over ssh and decrypted only at the remote host. To avoid genuinely insecure connections (connections that transfer cleartext data over the network and/or do not check the authenticity of the peer), the remote service must be blocked by the remote firewall and be reachable only through the remote "lo" interface.

The "forwardings.json" file is deliberately simple and has no other functionality. All other fields (for example Username, Hostname and of course Comment) are ignored. For all other purposes, the powerful "~/.ssh/config" file can be used by creating a new "Host" entry.

### STEP 3: Log in to every SSH server manually using the command line
```sh
> ssh router # The same host as the host inside forwardings.json
# Or, if the server inside forwardings.json is "10.5.2.1"
> ssh 10.5.2.1
```
Accept the unknown-host message (on the first connection, after you verify you are connected to the correct host) and then log out. If you skip this step, the connection fails and ssh's explanation (unknown host key, authentication failure, unreachable host) is written to sshportfw's output, or to syslog when running with `-s`.

### STEP 4: Running the program
When you have configured "forwardings.json", run the program manually to check its output.
If you need the port forwarding facility constantly — for example to use your printer — add the program to your startup applications. It no longer requires `DISPLAY`, but ssh may still need a terminal to ask for a passphrase or a security-key touch, so a plain cron job has nowhere to prompt: use your desktop's Startup Applications, or make sure an ssh agent and `SSH_ASKPASS` are available. Redirect the output to a file so you can see what happened if there are problems, or use `-s` to write to syslog.

To find out whether a forwarding works without opening a browser, run
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

A failure is also written to `/dev/tty` when one is available, at most once per service per minute, so it
is visible even when the log goes to syslog (`-s`) or to a file (`-o`). When there is no controlling
terminal (started by systemd or cron), that message is dropped and the log is the only place to look.
`-o /dev/null` therefore silences the log, but not those one-line messages on a desktop terminal.

### Signals
| signal | what it does |
|---|---|
| `SIGHUP` | Restarts the process with the executable on disk, keeping the same PID. Use it after replacing the binary. Open forwarded connections are closed. |
| `SIGUSR1` | Re-reads `forwardings.json` and restarts the listeners in place. Open forwarded connections are not affected. |
| `SIGTERM`, `SIGINT`, `SIGABRT` | Stops the program. |

Before restarting, sshportfw parses the new `forwardings.json` and runs the new executable with
`-check-config`. If either step fails, it logs the reason and keeps running the old binary, so a broken build
or a typo cannot take the service down. The restart re-executes the path this process was started from
(`os.Executable()`), so a rebuild in place at that path is picked up; repointing the symlink that leads to
it is not.

Upgrading while it runs:
```sh
> cd ~/Projects/sshportfw && go build -o sshportfw .
> kill -HUP $(pgrep -x sshportfw)
```

If you are upgrading from 0.6.x: `SIGHUP` used to reload the configuration in place. That is now `SIGUSR1`;
`SIGHUP` restarts the process and drops the open forwards. Version 0.7.0 also refuses configurations it
used to accept, such as a listen address with no host (`:8080`), a wildcard address, the same `ListenAddr`
twice, or a `Host` starting with `-`.

The next sections describe how to configure ssh to make sshportfw more useful.

## Adding functionality: Configuring ~/.ssh/config

### ~/.ssh/config: Using a control socket
At the **END** of ~/.ssh/config, you may want to add
```sh
match host * # it may be a list of hosts instead of *
    ControlMaster auto
    ControlPath ~/.ssh/ssh_mux_%h_%p_%r
    ControlPersist 300
```

The control socket makes subsequent connections very fast, but it has some drawbacks; see the manual.
Do not put such global options at the beginning of the file, because they cannot be overridden by subsequent entries.

### ~/.ssh/config: Accessing LAN devices from outside
Most LANs have a public IPv4 address and private (NAT) IPv4 addresses for all devices inside the LAN. Let's suppose we have a Raspberry Pi with the **static** private LAN address 10.5.2.2 (rpi.lan). We can set up port forwarding on our router and access our RPi from outside using mydynamicip.freemyip.com:2002 (this topic is not explained here; find instructions for your router).
We want ssh (and sshportfw) to connect to this device (RPi) even when using our laptop outside the home.
An entry like this in ~/.ssh/config will do the trick:

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
This rule can detect the 10.5.2.XX network and act accordingly. Of course, we can detect another unique element of our network. Be careful here, as many NATs use 192.168.0.x or 192.168.1.x and can be hard to tell apart. It is probably better to use less common IP ranges.
NOTE: If we have a range extender or a second router creating a different subnet, the ssh config needs additional rules.

### ~/.ssh/config: Accessing firewalled services using a jumphost
If we can't or don't want to open a lot of ports on our router (see the previous example), we can use a jumphost:
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
- We can edit /etc/hosts and add the line
  ```sh
  # This local port connects with the
  # LuCI interface on our router
  127.0.14.1      routerluci.fw
  ```
  Now the service can be accessed by pointing our browser at "routerluci.fw:8080".
  NOTE: It may be tempting to put the DNS entry for routerluci.fw on the OpenWrt router itself (hostnames section), but the hostname will *not* be available when we are connected to another network, or when using a local resolver such as dnscrypt-proxy, which (at least by default) ignores the router's DNS server.

### Eliminating the port specification in the browser URL (probably a bad idea)
```url
This means
http://routerluci.fw  (the port is in fact 80)
instead of
http://routerluci.fw:8080
```
There are plenty of tutorials on using ports below 1024 (setcap port redirection, etc.), but it may not be worth the effort. It offers a minor improvement but involves manipulating files and services as root, which adds complexity and creates security considerations.


## Security
First of all, use the program at your own risk! Anything related to SSH with the wrong configuration can expose your appliances or PCs to the world.
- On a system with multiple users, all users with a shell will have access to the remote services, at least to the login page. The program is designed to be used from your trusted PC or laptop, not from a shared computer at work or university. Using an SSH client on a machine that is not yours is a security risk anyway. Of course, this depends on how important the server is.
- The ssh command keeps the SSH connection open as long as there are active forwardings (this can be very long), but even after that the program keeps the connection open if you use the ControlSocket option. After that, the SSH connection is closed and you will need to log in again (i.e. touch your Yubikey again) to use the service.
- Password-based authentication must be avoided (it is easily stolen and guessed!). Also, file-based private ssh keys (those in ~/.ssh/) can be copied and used without you noticing. A hardware security key is the real solution.

### Security hardening: Yubikey, Solokey, GNUK
Security keys such as [Yubico](https://www.yubico.com/), [Solokey](https://solokeys.com/) or [GNUK](http://www.fsij.org/category/gnuk.html) can offer enhanced security without the need to type passphrases. The private key is stored on the hardware token, and the token is designed to perform specific cryptographic operations with it but never to allow the private key to leave the device. Note that the dropbear SSH server (used by OpenWrt) cannot handle FIDO private keys (those with the `-sk` suffix); you have to install and configure the OpenSSH server for that. GNUK uses normal ssh keys, but it is somewhat difficult to build the hardware and configure the system. Also, more expensive tokens such as Yubikey offer authentication methods compatible with dropbear. Do your research, and keep in mind that you need two of them: one is the backup if you lose the other.

## Bugs
Testing is very limited: the program is used on a Linux Mint laptop with Yubico FIDO keys. If you find bugs, please report them in the issues section.
The configuration parser has unit tests; run `go test ./...` after changing it.

## Other platforms
The program is pure Go. It builds for Linux and macOS. The Windows build fails because the syslog output uses `log/syslog`, which is not available there. It is only tested on Linux. If you can run the application successfully on a Mac, send me the instructions so I can include them in this document.

## Alternative solutions
There is no need to read all this; it is included for completeness. The (many) problems with these solutions are the reason sshportfw was created.


### Solution 1: plain ssh with forwarding rules in ~/.ssh/config or directly on the command line
This is the method used by most people. If there are a lot of rules, however, repeating this process becomes tiring and error-prone, and automating it is exactly sshportfw's job.

### Solution 2: VPN (not the anonymizing providers, but self-hosted mesh overlay networks)
I tried [Zerotier](https://www.zerotier.com/) and [Nebula](https://github.com/slackhq/nebula). For complex setups with multiple internal (NAT) network Docker instances or virtual machines, a VPN is probably the way to go.

There are many downsides, however:
- Many VPNs only work if an Internet connection is available — *even when we try to access a local node!* Yet we mostly need access to our OpenWrt router exactly when there are problems on our network. The VPN works when we don't need it and stops working the exact moment we do!
- Remote services are constantly exposed, which is not a good security practice. Port forwarding gives better control: we know if and when the service is used, for example because we have to press the security key.
- A VPN requires careful setup, especially firewall setup, *usually in a custom VPN firewall language.* This is _very_ time-consuming, and there is always the danger that something is wrong and lets unauthorized traffic into our network. The normal firewall (not the virtual one) also needs rules to accept traffic from the TUN/TAP interface.
- The servers on the remote machines (i.e. uhttpd) need config modifications. An OpenWrt router needs to have uhttpd accept connections from the virtual network (typically a tun device). This means modifications to the OpenWrt firewall *and* to the uhttpd config file.
- If the service you want to use is not hosted on the same server as the VPN node, the VPN alone cannot help you; you need custom port forwarding rules. For example, a network printer (192.168.6.25) at work is connected to the same network as the Raspberry Pi (192.168.6.2). At home, we can access the RPi either via VPN or via SSH.
To print from home with SSH, all we need to do is
ssh -L127.0.7.1:6000:192.168.6.25:661
and configure a printer setup pointing to 127.0.7.1:6000 IPP port TODO fix IP. A VPN, however, lets us reach only the RPi, not the printer, so the RPi needs additional port forwarding rules or a redirecting daemon such as rinetd — a very awkward and complex solution.
- Debugging VPN problems can become very difficult, because every node can effectively be reached in two different ways: through its normal IP or through the VPN one. It is not uncommon for traffic meant for a real interface to go through the VPN, or vice versa, or for the VPN not to work because of firewall rules (real or virtual!).
- It requires additional software on every node. The software may not be available for some platforms, and many OpenWrt routers do not have enough free space. Most routers have only enough flash for their proprietary firmware. One example I have is the Xiaomi Mi Gigabit edition: it is very fast, has enough RAM, easily sustains 300 Mbps of traffic and runs OpenWrt perfectly, but it has only 8 MB of flash.

### Solution 3: Securing the web interface with SSL/TLS
Many services, such as OpenWrt uhttpd or many network printers, allow secure connections over SSL/TLS. With this method we forget about both VPN and SSH port forwarding and connect directly to the server. Again, there are many severe problems.

- Many web interfaces of routers, smart switches and other appliances do not offer the option of SSL.

- Many services such as OpenWrt uhttpd *should not be exposed to the Internet* even when using an encrypted connection. The server is not hardened the way openssh-server is. To be fair to uhttpd, NOTHING is as battle-hardened as the OpenSSH server. The security status of the built-in HTTP server of a network printer is unknown and thus **way** more questionable than OpenWrt's uhttpd.

- Additional setup is required on the servers (TLS certificates and firewall ports).

- Every service needs valid certificates and/or configuration changes in the browser when it complains about invalid certificates. If we choose to ignore such errors, our network is open to man-in-the-middle attacks.

- In most cases the authentication mechanism is a mere password (LuCI, for example). This is hardly acceptable these days. Compare it with SSH protected by a FIDO token.

- Even worse, some network services are not password protected (a printer queue). Having a printer accept jobs from the whole Internet (even over TLS) is not an option at all, while SSH port forwarding offers a really elegant solution.
