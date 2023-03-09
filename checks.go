package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TODO return err with message
func isValidAddr(addr string) bool {
	if len(addr) == 0 {
		return false
	}
	if addr[0] == '/' {
		// We assume unix socket
		return true
	}
	addrPort := strings.Split(addr, ":")
	if len(addrPort) != 2 {
		return false
	}
	portStr := addrPort[1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return false
	}
	if port <= 0 || port > 65536 {
		return false
	}
	return true
}

func checkEnvironment() error {

	if _, ok := os.LookupEnv("DISPLAY"); !ok {
		return fmt.Errorf("Environment variable %s is not set. ssportfw is meant to run from a Desktop session.", "DISPLAY")
		//log.Printf("WARNING Environment variable %s is not set. ssh messages will printed in the console and you may not notic them", "DISPLAY")
	}
	return nil
}

// Extracts the servers from the JSON file and performs many checks to the data
func getServers(configPath string) ([]serverInfo, error) {
	var allServers []serverInfo
	{
		data, err := os.ReadFile(ForwardingsPath)
		if err != nil {
			return nil, err // TODO more verbose message
		}
		err = json.Unmarshal(data, &allServers)
		if err != nil {
			return nil, fmt.Errorf("Error parsing file %q err=%q", ForwardingsPath, err)
		}
	}

	if len(allServers) == 0 {
		return nil, fmt.Errorf("The configuration file %q does not contain any server definitions", "TODO")
	}

	// remove unwanted spaces and new lines
	for idx, server := range allServers {
		allServers[idx].Host = strings.TrimSpace(server.Host)
		//allServers[idx].Username = strings.TrimSpace(server.Username)
		for jdx, fw := range server.Forward {
			allServers[idx].Forward[jdx].Service = strings.TrimSpace(fw.Service)
		}
	}

	{
		names := make(map[string]struct{})
		for _, server := range allServers {
			if server.Host == "" {
				return nil, fmt.Errorf("Missing %q in server definition %#v", "Host", server)
			}
			//if server.Username == "" {
			//	return nil, fmt.Errorf("Missing %q in server definition %#v", "Username", server)
			//}
			sNameLower := strings.ToLower(server.Host)
			if _, ok := names[sNameLower]; ok {
				return nil, fmt.Errorf("%q is declared multiple times in %q", server.Host, ForwardingsPath)
				//os.Exit(1)
			}
			names[sNameLower] = struct{}{}
		}
	}

	for _, server := range allServers {
		//if !isValidAddr(server.Hostname) {
		//	return nil, fmt.Errorf("File %q server %q has no valid port in %q", ForwardingsPath, server.Name, "Hostname")
		//}
		//if server.HostnameExternal != "" && !isValidAddr(server.HostnameExternal) {
		//	return nil, fmt.Errorf("File %q server %q has no valid port in HostnameExternal", ForwardingsPath, server.Name)
		//}
		for _, fw := range server.Forward {
			if !isValidAddr(fw.ListenAddr) {
				return nil, fmt.Errorf("File %q server %q forwarding %q has no valid port in %q", ForwardingsPath, server.Host, fw.Service, "ListenAddr")
			}
			//
			if !isValidAddr(fw.RemoteAddr) {
				return nil, fmt.Errorf("File %q server %q forwarding %q has no valid port in %q", ForwardingsPath, server.Host, fw.Service, "RemoteAddr")
			}
		}
	}

	for _, server := range allServers {
		names := make(map[string]struct{})
		for _, fw := range server.Forward {
			if fw.Service == "" {
				return nil, fmt.Errorf("File %q, section %q, missing %q in forwarding definition", ForwardingsPath, server.Host, "Service")
			}
			fwNameLower := strings.ToLower(fw.Service)
			if _, ok := names[fwNameLower]; ok {
				return nil, fmt.Errorf("%q is declared multiple times in %q file %q", fw.Service, server.Host, ForwardingsPath)
			}
			names[fwNameLower] = struct{}{}
		}
	}

	return allServers, nil
}

///// END

//if _, err := exec.Command("notify-send", "--version").Output(); err != nil { // TODO var
//	return fmt.Errorf("Cannot run %q. Error=%q.", "notify-send", err)
//}
//if _, ok := os.LookupEnv(AuthSocketEnv); !ok {
//	// TODO notify-send message
//	return fmt.Errorf("Environment variable %s is not set. ssportfw needs an ssh agent for secret queries.", AuthSocketEnv)
//}
//if _, err := exec.Command("zenity", "--version").Output(); err != nil { // TODO var
//	// TODO notify-send message
//	return fmt.Errorf("Cannot run %q. Error=%q.", "zenity", err)
//}

/* if errors.Is(err, os.ErrNotExist) {
	_, err := os.Create(ForwardingsPath)
	if err == nil {
		//log.Printf("Created empty %q file. Go to\nhttps://github.com/pkarsy/sshportfw\nto to find out how to add SSH server entries", ForwardingsPath)
		return nil, fmt.Errorf("Created empty %q file. Go to\nhttps://github.com/pkarsy/sshportfw\nto to find out how to add SSH server entries", ForwardingsPath)
	}
	return nil, err
} else {
	log.Print(err)
	return
}
os.Exit(1) */

// The network match is case insensitive.
//allServers[idx].Network = strings.TrimSpace(strings.ToLower(server.Network))
//allServers[idx].Hostname = strings.TrimSpace(server.Hostname)
//allServers[idx].HostnameExternal = strings.TrimSpace(server.HostnameExternal)
//allServers[idx].JumpHostExternal = strings.TrimSpace(server.JumpHostExternal)

//if server.Hostname == "" {
//	return nil, fmt.Errorf("Missing %q in server definition %#v", "Hostname", server)
//	//os.Exit(1)
//}

/* if server.Network == "" {
	if server.HostnameExternal == "" && server.JumpHostExternal == "" {
		return nil, fmt.Errorf("%q is set but missing %q or %q in server definition %#v", "Network", "HostnameExternal", "JumpHostExternal", server)
		//os.Exit(1)
	}
} */

/* {
	var addrlast = 1
	for idx, server := range allServers {
		if server.JumpHostExternal != "" {
			if server.HostnameExternal != "" {
				//msg := fmt.Sprintf("ERROR : in file %q HostNameExternal and JumpHostExternal are both set", ForwardingsPath)
				return nil, fmt.Errorf("ERROR : in file %q HostNameExternal and JumpHostExternal are both set", ForwardingsPath)
				//os.Exit(1)
			}
			laddr := fmt.Sprintf("127.1.2.%d:%d", addrlast, JumpHostPort)
			allServers[idx].HostnameExternal = laddr // TODO dynamic and socket
			//log.Printf("Adding HostnameExternal=%q to %q", laddr, server.Name)
			addrlast++
			for idx, jh := range allServers {
				if jh.Name == server.JumpHostExternal {
					fw := forwarding{fmt.Sprintf("JumpHostFor-%s", server.Name), "JumpHost", laddr, server.Hostname}
					allServers[idx].Forward = append(jh.Forward, fw)
					//log.Printf("Adding forwarding %q to %q", fw, jh.Name)
					//log.Print(jh.Forward)
					break
				}
				if idx == len(allServers)-1 {
					return nil, fmt.Errorf("In %q file section %q cannot find %q jumphost. No such server is declared", ForwardingsPath, server.Name, server.JumpHostExternal)
					//os.Exit(1)
				}
			}
		}
	}
} */

/* for _, server := range allServers {
	if strings.EqualFold(server.Hostname, server.HostnameExternal) {
		return nil, fmt.Errorf("File %q, section %q, Hostname cannot be the same as HostnameExternal", ForwardingsPath, server.Name)
		//os.Exit(1)
	}
} */

// TODO na paei mesa sto getServers

/* for _, server := range allServers {
	if server.Network != "" {
		output, err := exec.Command(path.Join(configPath, NetworFinderPath)).Output()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// silently ignore the missing script, is not very useful anyway
				log.Printf("File %q server %q. HostnameExternel is set but executable file %q does not exist", ForwardingsPath, server.Name, "network")
			} else {
				log.Printf("File %q server %q. HostnameExternel is set but executable file %q gave error : %q", ForwardingsPath, server.Name, "network", err)
			}
			os.Exit(1)
		}
		if len(output) == 0 {
			log.Print("The network script does not produce any output, should print something like home, work etc")
			os.Exit(1)
		}
	}
} */
