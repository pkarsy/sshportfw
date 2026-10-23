package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/syslog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
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

// global variables are bad but given how simple the program is, lets accept 2 of them
var routineCount = safeCounter.New()
var activeForwardings = safeCounter.New()

type forwarding struct {
	Service    string
	ListenAddr string
	RemoteAddr string
}

type serverInfo struct {
	Host    string
	Forward []forwarding
}

// lineLogger forwards every line written to it to the standard logger, so that
// the ssh client's output goes to the configured destination (-o, -s) and
// obeys its flags instead of escaping to the terminal.
type lineLogger struct {
	prefix string
}

func (l lineLogger) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			log.Printf("%s %s", l.prefix, line)
		}
	}
	return len(p), nil
}

// Stops the timer and drains the chan safely
// TODO delete
/* func stopTimer(t *time.Timer) {
	t.Stop()
	select {
	case <-t.C:
	default:
	}
} */

func flagParse() {
	var version bool
	var syslogOutput bool
	var output string
	var lines bool
	var printtime bool
	flag.BoolVar(&version, "version", false, "prints current sshportfw version")
	flag.BoolVar(&version, "v", false, "")
	flag.BoolVar(&syslogOutput, "syslog", false, "redirects output to syslog")
	flag.BoolVar(&syslogOutput, "s", false, "")
	flag.StringVar(&output, "output", "", "Redirect output to file, only messages from ssh client are displayed to console. Use -o /dev/null for quiet operation")
	flag.StringVar(&output, "o", "", "")
	flag.BoolVar(&lines, "lines", false, "Print source code line numbers for debugging")
	flag.BoolVar(&lines, "l", false, "")
	flag.BoolVar(&printtime, "time", false, "Print date and time for every line of output (ignored on syslog output)")
	flag.BoolVar(&printtime, "t", false, "")
	flag.Parse()
	currflags := 0
	if version {
		fmt.Println("sshportfw Version 0.6.3")
		os.Exit(0)
	}
	if output != "" && syslogOutput {
		log.Print("-s and -o flags cannot be enabled at the same time")
		os.Exit(1)
	}
	if output != "" {
		outfile, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(outfile)
	}
	if lines {
		currflags = currflags | log.Lshortfile
	}
	if printtime && !syslogOutput {
		// logger will print date and time
		currflags = currflags | log.Ldate | log.Ltime
	}
	if syslogOutput {
		log.SetFlags(0)
		// Configure logger to write to the syslog
		logwriter, err := syslog.New(syslog.LOG_NOTICE, Appname)
		if err == nil {
			log.SetOutput(logwriter)
		} else {
			log.Fatal("Cannot set syslog output")
		}
	}
	log.SetFlags(currflags)
}

// isPeerGone reports errors that only say the other side has already gone away.
// They follow the real reason (ssh's stderr, a tab closed by the user) and are
// not worth a log line of their own.
func isPeerGone(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EPIPE)
}

// called as a Goroutine, takes a net.Conn, spawns a ssh connection and bidirectionally transfers data
func sshInstance(localConn net.Conn, fw forwarding, host string) {
	if localConn == nil {
		return
	}
	id := routineCount.Inc()
	// "/data/data/com.termux/files/usr/bin/ssh"
	cmd := exec.Command("ssh", "-W", fw.RemoteAddr, host)
	// ssh reports host key, authentication and connection problems on stderr.
	// Without this, os/exec discards the child's stderr and a failed forward
	// looks like a silent connection reset in the browser.
	cmd.Stderr = lineLogger{prefix: fmt.Sprintf("#%d ssh stderr:", id)}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("#%d %q", id, err)
		localConn.Close()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("#%d %q", id, err)
		stdin.Close()
		localConn.Close()
		return
	}
	// Now we actually execute the ssh command
	err = cmd.Start()

	if err != nil {
		log.Printf("#%d %q", id, err)
		stdin.Close()
		localConn.Close()

		return
	}
	log.Printf("#%d Start forwarding : %s %s", id, host, fw.Service)
	// we use it to limit the exit message to just the first terminating goroutine
	once := sync.Once{}
	// We use it to wait for the goroutines
	wg := sync.WaitGroup{}
	// we will have 3 goroutines
	wg.Add(3)
	//active :=

	// Terminate ssh at most once, whichever direction finishes first. The
	// "signal: terminated" that Wait then reports is our own doing, so it is
	// not logged as an error.
	var killOnce sync.Once
	var killed atomic.Bool
	killSSH := func() {
		killOnce.Do(func() {
			pr := cmd.Process
			if pr != nil {
				killed.Store(true)
				log.Printf("#%d Sending term signal to ssh client", id)
				pr.Signal(syscall.SIGTERM)
			}
		})
	}
	// only the first unexpected error is reported
	copyError := func(direction string, err error) {
		if err != nil && !isPeerGone(err) {
			once.Do(func() {
				log.Printf("#%d %s : %q", id, direction, err)
			})
		}
	}

	go func() {
		_, err := io.Copy(stdin, localConn)
		localConn.Close() // we force the other io.Copy to terminate (reader)
		copyError("local --> remote", err)
		killSSH()
		wg.Done()
	}()
	go func() {
		_, err := io.Copy(localConn, stdout)
		localConn.Close() // we force the other io.Copy(goroutine) to exit (reader)
		copyError("remote --> local", err)
		killSSH()
		wg.Done()
	}()
	go func() {
		err := cmd.Wait()
		if err != nil && !killed.Load() {
			once.Do(func() {
				log.Printf("#%d : %q", id, err)
			})
		}
		localConn.Close()
		wg.Done()
	}()

	log.Printf("#%d Copy routine started (total active %d)", id, activeForwardings.Inc())
	// wait until all 3 goroutines are terminted
	wg.Wait()
	//once.Do(func() {
	log.Printf("#%d ssh forwarder ends (active remaining %d)", id, activeForwardings.Dec())
	//})
	//log.Printf("Active SSH forwardings remaining : %d")
}

// how long to wait before retrying a listen or an accept that failed
const listenRetryDelay = time.Minute

// listenWithRetry binds addr, retrying every listenRetryDelay until it succeeds.
// It returns false when cancel is closed first, so that a port which can never
// be bound cannot block a reload or a shutdown.
func listenWithRetry(tag, addr string, cancel <-chan struct{}) (net.Listener, bool) {
	for {
		localListener, err := net.Listen("tcp", addr)
		if err == nil {
			return localListener, true
		}
		log.Printf("%s listen failed at %s err=%q", tag, addr, err)
		select {
		case <-cancel:
			log.Printf("%s listen cancelled while retrying", tag)
			return nil, false
		case <-time.After(listenRetryDelay):
		}
	}
}

// listens to a local port and whan a local connection occurs
// connects to remote ssh if necessary
// establishes a socket for communication and starts a DataCopy goroutine for the copying of
// the data send and received
func localPortListen(fw forwarding, host string, cancel <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	tag := fmt.Sprintf("%s-%s", host, fw.Service)
	defer log.Printf("%s goroutine ends", tag)

	localListener, ok := listenWithRetry(tag, fw.ListenAddr, cancel)
	if !ok {
		return
	}
	defer localListener.Close()
	log.Printf("%s listening at %s", tag, fw.ListenAddr)

	// Closing the listener unblocks Accept. The goroutine ends either when the
	// listener is cancelled or when this function returns.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-cancel:
			localListener.Close()
		case <-stop:
		}
	}()

	for {
		localConn, err := localListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("%s listen.Accept failed: %v . We will try again in %s", tag, err, listenRetryDelay)
			select {
			case <-cancel:
				return
			case <-time.After(listenRetryDelay):
			}
			continue
		}
		go sshInstance(localConn, fw, host)
	}
}

func main() {
	log.SetOutput(os.Stdout)
	flagParse()
	// The location of the config dir on linux is ~/.config/sshportfw
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
			log.Printf("%q : %s", err, "Check if the program is already running in another console or in the background.")
			//log.Print("Already running")
			os.Exit(1)
		}

	}
	// we check for the necessary env vars and programs zenity and notify-send
	//if err := checkEnvironment(); err != nil {
	//	log.Print(err)
	//	os.Exit(1)
	//}
	// this loop can cycle only on a HUP sugnal. The first time we just start the listening servers
	//reloadSignal := false

	// must have capacity of 1 accordig to docs
	osSignal := make(chan os.Signal, 1)
	// HUP causes the server to reread the config file
	signal.Notify(osSignal, os.Interrupt, syscall.SIGHUP)
	// these signals (in addition to SIGKILL) terminate the server
	signal.Notify(osSignal, os.Interrupt, syscall.SIGTERM)
	signal.Notify(osSignal, os.Interrupt, syscall.SIGABRT)
	signal.Notify(osSignal, os.Interrupt, syscall.SIGINT)
	// We get the list of the services from the config file
	allServers, err := getServers(configPath)
	if err != nil {
		log.Print(err)
		// we can't do anything else
		os.Exit(1)
	}
	for {
		// the SSH servers as defined in the config file
		//allServers, err := getServers(configPath)
		/* if err != nil {
			if reloadSignal == false { // first run and the config file is broken
				log.Print(err)
				// we can't do anything else
				os.Exit(1)
			}
			// we got a HUP signal but we cant reload the config file either
			// we print a message but we dont terminate and still using the old config
			log.Print("Cannot reload the new config err=", err)
		} */

		// We use this channel to signal the listening servers to terminate
		cancel := make(chan struct{})
		// counts the listening servers. When 0 we know all servers are ended
		var wg sync.WaitGroup

		for _, info := range allServers {
			for _, fw := range info.Forward {
				wg.Add(1)
				go localPortListen(fw, info.Host, cancel, &wg)
			}
		}

		// waiting a signal from the os
		for {
			sig := <-osSignal // we wait here until an OS signal is caught
			if sig != syscall.SIGHUP {
				log.Printf("Signal %q, program ends", sig)
				log.Print("Closing listening servers")
				close(cancel) // not necesary as os will close all listening ports anyway
				wg.Wait()
				// We dont stop any active connections, we probably should TODO
				return // terminate
			}
			// At this point signal is HUP and we want to reload config file and restart
			// all listening servers, without stopping any existing connections
			allServers, err = getServers(configPath)
			if err == nil {
				close(cancel)
				wg.Wait()
				log.Print("All listening servers stopped but active connections are not affected, now reloading servers from the config file")
				break
			}
			log.Print("Config file has errors, leaving the previous configuration. Fix the errors in the config file and send a HUP signal again")
			log.Print(err)
		}
	}
}
