# sshportfw

A program doing automatic SSH port forwarding whenever we need to access our SSH-secured network appliances and servers. The audience is users/administrators who heavily rely on ssh to access remote devices.
In this document, we take the approach that we want to access all appliances via SSH even if they are on the same network as our workstation.

The idea is to have sshportfw listening to local addresses such as 127.0.5.1:8080. When we point our browser to this address, openssh client is called automatically and connects to our OpenWRT router. The same thing can be achieved with the command (must be executed BEFORE we open the web page)
```sh
> ssh -L127.0.5.1:8080:127.0.0.1:80 router  # (or the IP)
```

but with sshportfw the process becomes automatic. ssportfw calls ssh to do the actual forwarding so some expertise in configuring the ~/.ssh/config is necessary. Note however that you do NOT need to configure port forwardings inside ~/.ssh/config. The file ~/.config/sshportfw/forwardings.json is used as we will see.


Typically port forwarding is used to access [OpenWRT](https://openwrt.org/) routers, [Syncthing](https://syncthing.net/) web interfaces, Printer pages, and in general services that (due to security reasons) can only serve the localhost interface. Not only that, the ssh config file is very powerful and allows bypassing firewalls, accessing remote print queues etc, and all that with security, maturity and flexibility not comparable with any other software.

## STEP 1: Installation of the executable
Note that the program is only tested on Linux.
A Linux amd64 executable is included on the Releases page. It should run on every modern Linux for PC. It can probably work on other platforms (after a compilation), but it is not tested. See **Other platforms** below
```sh
# Download the executable from the latest release and
> chmod +x ssportfw
# Copy/move the "sshportfw" binary somewere in the PATH
# Or if you have git and golang installed
> git clone https://github.com/pkarsy/sshportfw
> cd sshportfw
> go build
> ./sshportfw [options]
# Or compile and run in 1 step
> go run *.go [options]
```

### STEP 2: Editing the forwardings.json file
The program is looking for the file  

```sh
~/.config/sshportfw/forwardings.json
```

It does not try to create it by itself.

A sample config follows so you can copy-paste and edit it. This config assumes a router at 10.5.2.1
and the other LAN devices to have 10.5.2.X addresses. Also, we assume that local DNS is working, for example, router or router.lan resolve to 10.5.2.1. You can completely ignore DNS by using the IP addresses. So instead of "router" use "10.5.2.1"

```json
[
  {
    "Host": "router",
    "Comment":"Gets internet(PPPOE) from the providers modem",
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
    "Comment": "Printer1 and Printer2 are connected to and only accessibe via openwrt2",
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

The "Host" can be the hostname(or the IP) or "user@host" or a **Host entry inside ~/.ssh/config**  (this is the preferred approach)


The program listens to "ListenAddr": "127.0.10.1:8080" etc. but does not try to connect to any SSH server until we point our browser to  "http://127.0.10.1:8080". Then sshportfw uses the ssh client to connect to router and forward the local data to 127.0.0.1:80 on the remote machine, the LuCi configuration page in this case.

The browser may complain about "insecure connections". This is harmless (I am not a security expert, so no guarantees), as all traffic is tunneled via ssh and decrypted only at the remote host. To avoid true insecure connections (connections that transfer cleartext data via the network and/or do not check the authenticity of the peer), the remote service must be blocked using the remote firewall and only can be accessible via the remote "lo" interface.

The "forwardings.json" file is on purpose very simple and does not have any other functionality. All other options (for example Username Hostname and of course Comment) are ignored. For all other possibilities, the powerful "~/.ssh/config" file can be used by creating a new "Host" entry.


### STEP 3: login to every SSH server manually using the command line
```sh
> ssh router # The same host as the host inside forwardings.json
# Or if inside forwardings.json the server is "10.5.2.1"
> ssh 10.5.2.1
```
accept the unknown host message (if this is the first time and after you verify you are connected to the correct host) and then logout. If you skip this step, the connection will fail unless you notice the message and answer accordingly.


### STEP 4: running the program
When you configure the "forwardings.json" you have to run it manually to check the output.
If you are in constant need of the port forward facility, ie to use your printer then put the program in the list of the startup programs. If you put it in a cron startup script it won't run because it needs the DISPLAY environment variable. If you use ControlPanel->StartupApps it is ok. Redirect the output to a file to know what happens if you have problems, or use the --syslog flag.

The next sections are about configuring ssh to make sshportfw more useful.

## Adding functionality: Configuring the ~/.ssh/config

### ~/.ssh/config: Using a control socket
At the **END** of ~/.ssh/config, you may want to add
```sh
match host * # or for specific hosts only
    # user root
    # CheckHostIP no
    # ForwardAgent no
    ControlMaster auto
    ControlPath ~/.ssh/ssh_mux_%h_%p_%r
    ControlPersist 300
```

The control socket makes subsequent connections very fast, but there are some drawbacks, see the manual.
Do not put such global options at the beginning of the file, because they cannot be overridden by subsequent entries.

### ~/.ssh/config: Access LAN devices from outside.
Most LANs have a public IPV4 address and private(NAT) IPV4 addresses for all devices inside the LAN. Let's suppose we have a Raspberry Pi with **static** private LAN address 10.5.2.2(rpi.lan) We can set up port forwarding on our router and we can access our Rpi from outside using mydynamicip.freemyip.com:2002 (this topic is not explained, here find instructions for your router)
We want ssh (and sshportfw) to connect to this device(Rpi) even when using our laptop outside of our home.
An entry like this in ~/.ssh/config will do the trick :


```sh
match host rpi !exec "ip -4 a | grep -q 10.5.2."
  # the 10.5.2 must be adapted to our actual ip range
  # works only if the router is configured to redirect incoming
  # TCP connections on port 2002 to 10.5.2.2:22
  # Also a dynamic DNS service must be configured on your router
  hostname mydynamicip.freemyip.com
  port 2002

host rpi
  user auser
  # some options but dont put forwarding rules here
```
This rule can detect network 10.5.2.XX and act accordingly. Of course, we can detect another unique element of our network. Be careful here as a lot of NATs tend to use the 192.168.0.x or 192.168.1.x, and can be hard to distinguish them. It is probably beneficial to use less common IP ranges.
NOTE: If we have a range extender/second router giving a different subnet, the ssh config needs additional rules.

### ~/.ssh/config: Access firewalled services using a jumphost
If we can't or don't want to open a lot of ports to our router (see the previous example) we can use a jumphost
```sh
match host router !exec "ip -4 a | grep -q 10.5.2."
  # Works only if the host is accessible from the outside world, and usually this is the router.
  # Of course we must setup a DynamicDns service for this to work
  # Almost all routers and of course OpenWRT has good support on this
  # also the SSH server must listen to port 20202 and the WAN port 20202 to be open
  ProxyJump mydynamicip.freemyip.com
  Port 20202
```


## command line
```sh
> sshportfw -h

-l	
-lines
    Print source code line numbers for debugging
-o string
-output string
    Redirect output to file, only messages from ssh client are displayed to console. Use -o /dev/null for quiet operation
-s	
-syslog
    redirects output to syslog
-t	
-time
    Print date and time for every line of output (ignored on syslog output)
-v	
-version
    prints current sshportfw version
```

### Use hostnames instead of IPs on the browser.
- We can edit /etc/hosts and add the line
  ```sh
  # This local port connects with the
  # LuCi interface on our router
  127.0.14.1      routerluci.fw
  ```
  Now the service can be accessed by pointing our browser to "routerluci.fw:8080".
  NOTE: It may be tempting to put the DNS resolution(routerluci.fw in this case) to our OpenWRT router itself (hostnames section) but the hostnames will *not* be available when we are connected to another network, or if using a local resolver like dnscrypt-proxy which (at least by default) ignores the DNS server of the router.

### Eliminate port specification in the browser URL (probably a bad idea)
```url
This means
http://routerluci.fw  (The port is in fact 80)
instead of
http://routerluci.fw:8080
```
There are plenty of tutorials on how to use ports<1024 (SETCAP port redir etc), but it may not be worth the effort. It offers a minor improvement but involves manipulating files and services as root, adding to the complexity and creating security considerations.


## Security
First of all, use the program at your own risk! Anything related to SSH with the wrong configuration can expose your appliances/PCs to the world.
- On a system with multiple users, all users with a shell will have access to the remote services, at least to the login page. The program is designed to be used from your trusted PC/laptop, not from a shared computer at work/university. The use of an SSH client in a machine that is not yours is a security risk anyway. Of course, this depends on how important the server is.
- The ssh command keeps the SSH connection open as long as there are active forwardings (This can be very long) but ever after this, the program keeps the connection open if you use the ConrolSocket option. After this, the SSH connection is closed and you will need to re-login (ie you need to touch again your youbikey) to use the service.
- Password-based authentication must be avoided (Easily stolen and guessed !). And file-based private ssh keys (those in ~/.ssh/) can be copied and used without you noticing. A hardware security key is the real solution.

### Security hardening: Yubikey, Solokey, GNUK
Security keys such as [Yubico](https://www.yubico.com/) [Solokey](https://solokeys.com/) or [GNUK](http://www.fsij.org/category/gnuk.html) can offer enhanced security without the need to type passphrases. The private key is stored on the hardware token and the token is designed to perform specific cryptographic operations with it, but never allow (the private key) to escape from the device. Note that dropbear SSH server (used by OpenWRT) cannot handle FIDO private keys (those with -sk suffix). You have to install and configure the OpenSSH server for this purpose. GNUK uses normal ssh keys but it is somewhat difficult to build the hardware and configure the system. Also, the more expensive tokens like Ybikey offer authentication methods compatible with dropbear. Do your research and keep in mind that you need 2 of them, the one is the backup if you lose the other.

## Bugs
The testing is very limited, the program is used on a Linux Mint laptop using Ybico FIDO keys. If you find some bugs please report them in the issues section.

## Other platforms
The program is pure Golang and is trivial to compile and cross-compile for any supported platform. It is only tested on Linux however. If you can run the application successfully on a Mac or Windows send me the instructions to include in this document.

## Alternative solutions
No need to read all this, just for completeness. The (many) problems with these solutions are the reason sshportfw was created.


### Solution 1: plain ssh with forwarding rules in ~/.ssh/config or directly on the command line
This is the method used by most people. If there are a lot of rules, however, this repeating process becomes tiring and error-prone. And the job of sshportfw is to automate this process.

### Solution 2: VPN (Not the anonymizing providers, but self-hosted mesh overlay networks)
I tried [Zerotier](https://www.zerotier.com/) and [Nebula](https://github.com/slackhq/nebula). For complex setups with multiple internal (NAT) network docker instances or virtual machines,  VPN probably is the way to go.

There are many downsides, however :
- Many VPNs will only work if an Internet connection is available. *Even if we try to access a local node!* However, we mostly need access to our OpenWrt router exactly when there are problems on our network. The VPN works when we don't need it and stops working the exact moment we need it!
- Remote services are exposed constantly, not a good security practice. With port forwarding, there is better control. We know if and when the service is used, we need to press the security key for example.
- A VPN requires careful setup, especially firewall setup *usually in a custom VPN firewall language.* This is _very_ time-consuming and there is always the danger that something is wrong, allowing unauthorized access to our network. The firewall (the normal one, not the virtual) need also rules to accept traffic from the TUN/TAP interface.
- The servers (ie uhttpd) on the remote machines need config modifications. An OpenWRT router needs to have uhttpd accept connections from the virtual network (typically a tun device) This means modifications to the OpenWRT firewall *and* to uhttpd config file.
- If the service you want to use is not hosted on the same server as the VPN node, then the VPN alone cannot help you. You need custom port forwarding rules. For example, a network printer (192.168.6.25) at work is connected to the same network as the Raspberry Pi (192.168.6.2). At home, we can access the Rpi either via VPN or SSH.
To print from home with SSH all we need to do is
ssh -L127.0.7.1:6000:192.168.6.25:661
and configure a printer setup pointing to 127.0.7.1:6000 IPP port TODO fix IP. A VPN though allows us to reach only the RPi, not the printer, and RPi need to have additional rules for port forwarding or run a redirecting daemon as rinetd, a very awkward and complex solution.
- debugging VPN problems can become very difficult, as every node can be accessed effectively in 2 different ways through the normal IP or the VPN one. It is not uncommon, for traffic designed to pass through a real interface, to go via VPN or vice versa, or for the VPN to not work due to firewall rules (real firewall or virtual!)
- requires additional software to be installed on every node. The software may not be available for some platforms. And for many OpenWRT routers, there is not enough free space. Most routers have only enough flash to store their proprietary firmware. One example I have is the Xiaomi Mi gigabit edition. It is very fast, with enough RAM, easily sustains 300Mbps traffic, and can run OpenWRT perfectly, but has only 8GB flash.

### Solution 3: Securing the web interface with SSL/TLS
Many services such as OpenWrt uhttpd or many network printers allow secure connections over SSL/TLS. With this method, we forget about both VPN and SSH port forwarding and we connect directly to the server. Again there are many and severe problems.

- Many web interfaces of routers, smart switches and other appliances, do not offer the option for SSL

- Many services such as OpenWrt uhttpd *should not be exposed to the Internet* even when using an encrypted connection. The server is not security hardened the way openssh-server is. To be fair with uhttpd, NOTHING is as battle-hardened as the openssh server. The security status of the built-in http server of a network printer is unknown and thus **way** more questionable than OpenWRT-uhttpd.

- Additional setup is required on the servers (TLS certificates and firewall ports)

- Every service needs valid certificates and/or configuration changes to the browser when is complaining about invalid certs. If we choose to ignore such errors, then our network is open to man-in-the-middle attacks.

- In most cases, the authentication mechanism is a mere password (LuCi for example). This is hardly an acceptable solution these days. Compare this with SSH protected with a FIDO token.

- Even worse, some network services are not password protected (a printer queue). Having a printer accepting jobs from the whole Internet (even if using TLS) is not an option at all, but SSH port forwarding offers a really elegant solution.
