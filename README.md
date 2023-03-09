# sshportfw

A program doing automatic SSH port forwarding whenever we need to access our SSH-secured network appliances and servers. The audience is users/administrators which heavily rely on ssh to access remote devices.


Typically port forwarding is used to access [OpenWRT](https://openwrt.org/) routers, [Syncthing](https://syncthing.net/) web interfaces, and in general services that (due to security reasons) can only serve the localhost interface. Not only that, the ssh config file is very powerfull and allows to bypass firewalls, accessing remotelly and securelly print queues etc, and all that with a security, maturity and flexibility not comparable with any other software.

The idea is to have sshportfw listening to local addresses such as *127.0.5.1:8080*. When we point our browser to this address, openssh client is called automatically and connects to our OpenWRT router. The same thing can be achieved with the command (must be executed BEFORE we open the web page)
```sh
> ssh -L127.0.5.1:8080:127.0.0.1:80 router  # (or the IP)
```
but with sshportfw the process becomes automatic. ssportfw calls ssh to do the actual forwarding so some expertice of configuring the ~/.ssh/config is necessary. Note however that you do NOT need to configure port forwardings inside ~/.ssh/config. The file ~/.config/sshportfw/forwardings.json is used as we will see.

## STEP 1 : Installation of the binary
Note that the program is only tested on Linux.
A Linux amd64 executable is included. It should run on every modern Linux for PC. It can probably work on other platforms (after a compile), but it is not tested. See **Other platforms** below
```sh
> git clone https://github.com/pkarsy/sshportfw
> cd sshportfw
# Run  the precompiled binary
> ./sshportfw
# Copy the "sshportfw" binary somewere in the PATH
# such as ~/bin or /usr/local/bin/
# then simply run
# sshportfw
#
# or build the binary yourself
> go build
> ./sshportfw
# Or compile and run in 1 step
> go run *.go
```

### STEP 2 : Connect to every SSH server with command line BEFORE using this utility
for example
```
> ssh router
```
accept the unknown host message (if this is the first time) and then logout. sshportfw will not try to connect to any unknown host

### STEP3 : Configuring the forwardings.json file
The program is looking for the file  ~/.congig/sshportfw/forwardings.json. It does not try to create it by itself. You can add entries for your devices in this file.

A sample config looks like this: (You can copy-paste it and edit)

```json
[
  {
    "Host": "router",
    "Comment":"Gets internet(PPPOE) from the providers modem",
    "Comment2":"In fact we can have as meny comments as we like",
    "Forward": [
      {
        "Service": "LuCi",
        "ListenAddr": "127.0.10.1:8080",
        "RemoteAddr": "127.0.0.1:80"
      },
      {
        "Service": "Kyocera",
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
    "Forward": [
      {
        "Service": "LuCi",
        "ListenAddr": "127.0.14.1:8080",
        "RemoteAddr": "127.0.0.1:80"
      },
      {
        "Service": "",
        "ListenAddr": "127.0.14.2:8080",
        "RemoteAddr": "router-anatoliki.lan:8080"
      },
      {
        "Service": "RouterDytiki",
        "ListenAddr": "127.0.14.3:8080",
        "RemoteAddr": "router-dytiki.lan:8080"
      }
    ]
  }
]
```

The "Host" can be the hostname(or the IP) or a **Host entry inside ~/.ssh/config** This is much preffered as we can use many ssh options (user, port jumphost and others). By pointing our browser to "http://127.0.14.1:8080" we can access LuCi on our router. Also, note that by playing with 127.x.x.x addresses we can use the same listening port (8080 in this case) with multiple entries. Note also that the browser may complain about "insecure connections". This is harmless (I am not a security expert, so no guaranties), all traffic is tunnelled via ssh, and decrypted only at the remote host. To avoid true insecure connections (connections that transfer creartext data via the network), the remote service must be blocked using the (remote) firewall and only be accessible via the "lo" interface

The "forwardings.json" file is on purpose very simple and does not have any other options. All other entries (for example Username Hostname) ARE IGNORED. If you need more functionality it can be added in the powerfull "~/.ssh/config" file by creating a new "Host" entry


## Configuring the ~/.ssh/config

### Using a control socket
At the **END** of ~/.ssh/config you may want to add
```sh
match host * # or for specific hosts only
    # user root
    # CheckHostIP no
    # ForwardAgent no
    ControlMaster auto
    ControlPath ~/.ssh/ssh_mux_%h_%p_%r
    ControlPersist 300
```

The control socket makes subsequent connections very fast, but there are some considerations, see the manual.
Do not put such global options at start because they cannot be overriden by subsequent entries.

### Access your devices sometimes from inside of the LAN and sometimes from the outside world.
Most LANs have a public IPV4 address and private(NAT) IPV4 addresses for all devices inside the LAN. Let's suppose we have a Raspberry Pi with **static** private LAN address 10.6.3.2(rpi.lan) We set up port forwarding on our router and we can access our Rpi from outside using mydynamicip.freemyip.com:2002(WARNING this topic is not explained here find instructions for your router)
We want ssh (and sshportfw) to connect to this device(Rpi) even when using our laptop outside of our home.
An entry like this in ~/.ssh/config will do the trick **automatically**


```sh
match host rpi !exec "ip -4 a | grep -q 10.6.3."
  # the 10.6.3 must be adapted for every private LAN
  # works only if the router is configured to redirect incoming
  # TCP connections on port 2002 to 10.6.3.2:22
  # Also a dynamic DNS service must be configured on your router
  hostname mydynamicip.freemyip.com
  port 2002

host rpi
  user auser
  # some options but dont put forwarding rules here
```
This rule can distinguish the network by the first 3 numbers of an IPv4. Of course we can detect another unique element of our network. Be careful here as a lot of NATs tend to use the 192.168.0.x or 192.168.1.x, and can be hard to distinguish them. It is probably beneficial to use less comman IP ranges.
NOTE : If we have a range extender/second router giving a different subnet, the ssh config needs additional rules.

### Access internel services using a jumphost
If we can't or don't want to open a lot of ports to our router (see the previous example) we can use a jumphost
```sh
match host rpi !exec "ip -4 a | grep -q 10.6.3."
  # Works only if the host is accessible from the outside world, and usually this is the router.
  # Of course we must setup a DynamicDns service for this to work
  # Almost all routers and of course OpenWRT has good support on this
  ProxyJump mydynamicip.freemyip.com 
```


## command line
-v --version
-s --syslog

### Use hostnames instead of IP on browser.
- We can edit /etc/hosts and add the line
  ```sh
  # This local port connects with the
  # LuCi interface on our router
  127.0.14.1      routerluci.fw
  ```
  Now the service can be accessed by pointing our browser to "routerluci.fw:8080".
  NOTE: It may be tempting to put the DNS resolution(routerluci.fw in this case) to our OpenWRT router itself (hostnames section) but the hostnames will *not* be available when we are connected to another network, or if using a local resolver like dnscrypt-proxy which (at least by default) ignores the LAN resolver.

### Eliminate port specifiction in the browser URL (probably a bad idea)
```url
This means
http://routerluci.fw  (The port is in fact 80)
instead of
http://routerluci.fw:8080
```
There are plenty of tutorials on how to use ports<1024 (SETCAP port redir etc), but it may not worth the effort. It offers a minor improvement but involves manipulating files and services as root, adding to the complexity and creating security considerations.


### running the program
When you configure the "forwardings.json" you have to run it manually to check the output. Add one entry at a time.
If you are in constant need of the port forward facility, ie to use your printer then put the program in the list of the startup programs. If you put it in a cron startup script it wont run because it needs the DISPLAY environment variable. If you use ControlPanel->StartupApps it is ok. Redirect the output to a file to know what happens if you have problems.

## Security implications
First of all, use the program at your own risk! Anything related to SSH with the wrong configuration can expose your appliances/PCs to the world. As sshportfw uses the standard openssh client, all the problems presented here are in fact ssh misconfiguration problems.
- On a system with multiple users, all users with a shell will have access to the remote services, at least to the login page. The program is designed to be used from your own trusted PC/laptop, not from a shared computer at work/university. The use of a SSH client in a machine that is not yours is a security risk anyway. Of course this depends on how importand the server is.
- The ssh command keeps the SSH connection open as long as there are active forwardings (This can be very long) but ever after this, the program keeps the connection open if you use the conrolSocket option. After this, the SSH connection is closed and you will need to re-login (ie you need to touch again your youbikey) to use the service.
- Password-based authentication must be avoided (Easily stolen and guessed !). And file-based private ssh keys (those in ~/.ssh/) can be copied and used without you noticing. A hardware security key is the real solution.

## ssh tip : Security tokens, Yubikey, Solokey, GNUK
Tokens such as [YubikeyTODO](https://www.yubico.com/) or [GNUK](http://www.fsij.org/category/gnuk.html) SOLOKEY can offer levels of security and trust not conceivable with key files. The private key is stored on the hardware token and the token is designed to perform specific cryptographic operations with it, but never allow the key to escape out of the device. Note that dropbear SSH server (used by OpenWRT) cannot handle FIDO(YbikeyTODO Solokey) private keys (those with -sk suffix). You have to install and configure the OpenSSH server for this purpose. GNUK uses normal ssh keys but it is somewhat difficult to build the hardware and configure the system.

## Other platforms
The program is pure Go(golang) and is trivial to compile (and cross compile) for any supported platform. It is only tested on Linux however. If you can run the application successfully on mac or windows send me the instructions to include in this document.

## Alternative solutions to port forwarding
No need to read all this, just for completeness. The (many) problems with the alternative solutions are the reason sshportfw was created.


### Solution 1. plain ssh with forwarding rules in ~/.ssh/config or directly on command line
This is the method I used for a lot of time, however if there are a lot of rules, easily the manual process becomes very boring and sometimes time consuming and error prone.

### Alternative solution 1. VPN (Not the anonymizing providers but self hosted mesh VPNs)
I tried [Zerotier](https://www.zerotier.com/)](https://www.zerotier.com/) and [Nebula](https://github.com/slackhq/nebula). For complex setups with multiple internal (NAT) network docker instances or virtual machines,  VPN probably is the way to go.

There are many downsides, however :
- Many VPNs will only work if an Internet connection is available. **Even if we try to access a local node!** However, we mostly need access to our OpenWrt router **exactly** when there are problems on our network. The VPN works when we don't need it and stops working the exact moment we need it!
- Remote services are available all the time, not a good security practice. With port forwarding, there is better control. We know if and when the service is used, we need to press the security key for example.
- A VPN requires careful setup, especially firewall setup *usually in a custom VPN firewall language.* This is _very_ time-consuming and there is always the danger that something is wrong, allowing unauthorized access to our network. The firewall (the normal one, not the virtual) need also rules to accept traffic from the TUN/TAP interface.
- The servers(ie uhttpd) on the remote machines need config modifications. An OpenWRT router needs to have uhttpd accept connections from the virtual network (typically a tun device) This means modifications to the OpenWRT firewall *and* to uhttpd config file.
- If the service you want to use is not hosted on the same server as the VPN node, then the VPN alone cannot help you. You need custom port forwarding rules. For example, a network printer(192.168.6.25) at work is connected to the same network as the Raspberry Pi (192.168.6.2). At home, we can access the Rpi either via VPN or SSH.
To print from home with SSH all we need to do is
ssh -L127.0.7.1:6000:192.168.6.25:661
and configure a printer setup pointing to 127.0.7.1:6000 IPP port TODO fix IP. A VPN though allows us to reach only the RPi not the printer, and RPi need to have additional rules for port forwarding or run a redirecting daemon as rinetd, a very awkward and complex solution.
- debugging VPN problems can become very difficult, as every node can be accessed effectively in 2 different ways through the normal IP or the VPN one. It is not uncommon, for traffic designed to pass through a real interface, to go via VPN or vice versa, or for the VPN to not work due to firewall rules (real firewall or virtual!)
- requires additional software to be installed on every node. The software may not be available for some platforms. And for many OpenWRT routers, there is not enough free space. Most routers have only enough flash to store their own proprietary firmware. One example I have is the Xiaomi Mi gigabit edition. It is very fast, with enough RAM, easily sustains 300Mbps traffic, and can run OpenWRT perfectly, but has only 8GB flash.

### Alternative solution 2. Securing the web interface with SSL/TLS
Many services such as OpenWrt uhttpd or many network printers allow secure connections over SSL/TLS. With this method, we forget about both VPN and SSH port forwarding and we connect directly to the server. Again there are many(fatal im opinion) problems.

- Many web interfaces of routers, smart switches and other appliances, do not offer the option for SSL

- Many services such as OpenWrt uhttpd *should not be exposed to the Internet* even when using an encrypted connection. The server is not security hardened the way openssh-server is. To be fair with uhttpd, NOTHING is as battle-hardened as the openssh server. The security status of the built-in http server of a network printer is unknown and thus **way** more questionable than OpenWRT-uhttpd.

- Additional setup is required on the servers (TLS certificates and firewall ports)

- Every service needs valid certificates and/or configuration changes to the browser when is complaining about invalid certs. If we choose to ignore such errors, then our network is open to man-in-the-middle attacks.

- In most cases, the authentication mechanism is a mere password (LuCi for example). This is hardly an acceptable solution these days. Compare this with SSH protected with a FIDO token.

- Even worse, some network services are not password protected (a printer queue). Sending a print job from the Internet (even if using TLS) is not an option at all, but SSH port forwarding offers a really elegant solution.
