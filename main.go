package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"sshportfw/safeCounter"

	"github.com/juju/fslock"
	"github.com/kirsle/configdir"
)

const (
	Appname         = "sshportfw"
	ForwardingsPath = "forwardings.json"
)

// global variables are bad but given how simple the program is, lets accept 2
var routineCount = safeCounter.New()
var activeRoutines = safeCounter.New()

type forwarding struct {
	Service    string
	ListenAddr string
	RemoteAddr string
}

type serverInfo struct {
	Host    string
	Forward []forwarding
}

// Stops the timer and drains the chan safely
func stopTimer(t *time.Timer) {
	t.Stop()
	select {
	case <-t.C:
	default:
	}
}

func flagParse() {
	var version bool
	var syslogOutput bool
	flag.BoolVar(&version, "version", false, "prints current sshportfw version")
	flag.BoolVar(&version, "v", false, "")
	flag.BoolVar(&syslogOutput, "syslog", false, "redirects output to syslog")
	flag.BoolVar(&syslogOutput, "s", false, "")
	flag.Parse()
	if version {
		fmt.Println("sshportfw Version 0.6.0")
		os.Exit(0)
	}
	if syslogOutput {
		log.Print("SYSLOG output")
		// Configure logger to write to the syslog
		logwriter, err := syslog.New(syslog.LOG_NOTICE, Appname)
		if err == nil {
			log.SetOutput(logwriter)
		} else {
			log.Fatal("Cannot set syslog output")
		}
	}
}

// Goroutine, takes a net.Conn spawns a ssh connection and bidirectionally transfers data
func sshInstance(localConn net.Conn, fw forwarding, host string) {
	cmd := exec.Command("ssh", "-W", fw.RemoteAddr, host)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Print(err)
		return
	}
	id := routineCount.Inc()
	// Now we actually execute the ssh command
	err = cmd.Start()

	if err != nil {
		log.Print(err)

		return
	}
	log.Printf("Start forwarding : %s %s", host, fw.Service)

	once := sync.Once{}
	wg := sync.WaitGroup{} // We use it to wait for the goroutines
	wg.Add(3)
	activeRoutines.Inc()
	go func() {
		_, err := io.Copy(stdin, localConn)
		localConn.Close() // we force the other io.Copy to terminate (reader)
		if err != nil {
			once.Do(func() {
				log.Printf("#%d local --> remote : %q", id, err)
			})
		}
		pr := cmd.Process
		if pr != nil {
			log.Print("Sending term signal")
			pr.Signal(syscall.SIGTERM)
		}
		wg.Done()
	}()
	go func() {
		_, err := io.Copy(localConn, stdout)
		localConn.Close() // we force the other io.Copy(goroutine) to exit (reader)
		if err != nil {
			once.Do(func() {
				log.Printf("#%d remote --> local : %q", id, err)
			})
		}
		pr := cmd.Process
		if pr != nil {
			log.Print("Sending term signal")
			pr.Signal(syscall.SIGTERM)
		}
		wg.Done()
	}()
	go func() {
		err = cmd.Wait()
		if err != nil {
			once.Do(func() {
				log.Printf("#%d : command exit error %q", id, err)
			})
		}
		localConn.Close()
		wg.Done()
	}()

	log.Printf("Copy Routine #%d started", id)

	wg.Wait()
	once.Do(func() {
		log.Printf("ssh forwarder #%d ends", id)
	})
	log.Printf("Active SSH forwardings remaining : %d", activeRoutines.Dec())
}

// listens to a local port and whan a local connection occurs
// connects to remote ssh if necessary
// establishes a socket for communication and starts a DataCopy goroutine for the copying of
// the data send and received
func localPortListen(fw forwarding, host string) {
	tag := fmt.Sprintf("%s-%s", host, fw.Service)
	defer log.Printf("%s goroutine ends", tag)

	var localListener net.Listener
	for {
		var err error
		localListener, err = net.Listen("tcp", fw.ListenAddr)
		if err == nil {
			break
		}
		log.Printf("%s listen failed at %s err=%q", tag, fw.ListenAddr, err)
		time.Sleep(time.Minute)
	}
	log.Printf("%s listening at %s", tag, fw.ListenAddr)

	for {
		localConn, err := localListener.Accept()
		if err != nil {
			log.Printf("%s listen.Accept failed: %v", tag, err)
			time.Sleep(time.Minute)
			continue
		}
		// Transfer data from local to remote fw port and vice versa
		go sshInstance(localConn, fw, host)
	}
}

func main() {
	log.SetFlags(log.Lshortfile)
	log.SetOutput(os.Stdout)
	flagParse()
	// The location of the config file
	configPath := configdir.LocalConfig(Appname)
	if err := configdir.MakePath(configPath); err != nil {
		log.Print(err)
		return
	}
	if err := os.Chdir(configPath); err != nil {
		log.Print(err)
		return
	}
	// Do not allow 2 insrances of the program to run at the same time
	{
		lock := fslock.New("lock")
		if err := lock.TryLock(); err != nil {
			log.Print(err)
			log.Print("Already running")
			os.Exit(1)
		}

	}
	// we check for the necessary env vars and programs zenity and notify-send
	if err := checkEnvironment(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	// the SSH servers as defined in the config file
	allServers, err := getServers(configPath)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	for _, info := range allServers {
		for _, fw := range info.Forward {
			go localPortListen(fw, info.Host)
		}
	}
	// must have capacity of 1 accordig to docs
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGTERM)
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGABRT)
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGHUP)
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGINT)
	// waiting for terminating signal from the os
	sig := <-sigChannel
	log.Printf("Signal %q, program ends", sig)
}
