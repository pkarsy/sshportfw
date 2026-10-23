package main

import (
	"bytes"
	"context"
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
	"path/filepath"
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
// obeys its flags instead of escaping to the terminal. The first line is also
// reported on the console when one is available.
type lineLogger struct {
	prefix string
	notify func(string)
	once   sync.Once
}

func (l *lineLogger) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			log.Printf("%s %s", l.prefix, line)
			l.once.Do(func() {
				if l.notify != nil {
					l.notify(line)
				}
			})
		}
	}
	return len(p), nil
}

// consolef writes a message to /dev/tty when one is available, so that a
// failure is visible even when the log goes to syslog (-s) or to a file (-o).
// It is a no-op when there is no controlling terminal (systemd, cron).
func consolef(format string, args ...any) {
	ttyOnce.Do(func() {
		ttyFile, _ = os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	})
	if ttyFile != nil {
		fmt.Fprintf(ttyFile, format+"\n", args...)
	}
}

// failureNotifier reports a forwarding failure on the console at most once per
// notifyInterval, so retries of the same broken service do not flood it.
type failureNotifier struct {
	tag  string
	mu   sync.Mutex
	last time.Time
}

func (n *failureNotifier) notify(format string, args ...any) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if time.Since(n.last) < notifyInterval {
		return
	}
	n.last = time.Now()
	consolef("%s: %s: %s", Appname, n.tag, fmt.Sprintf(format, args...))
}

// how long to wait between attempts to bind a local port or to accept on it
const listenRetryDelay = time.Minute

// bounds the TCP connect time of the ssh client we spawn per connection.
const sshConnectTimeout = "10"

// at most one console message per forwarding and interval, so a service that is
// down does not flood the terminal each time the browser retries.
const notifyInterval = time.Minute

// set at build time with -ldflags "-X main.versionString=..."
var versionString = "0.7.0"

var (
	// set from the command line before any goroutine starts
	checkMode       bool
	checkConfigMode bool
	configFile      string
	sshBinary       = "ssh"

	// path of this executable, resolved once at startup and used by re-exec
	selfPath string

	// keeps the lock reachable for the lifetime of the process: a finalized
	// os.File closes its descriptor, which would release the lock.
	instanceLock *fslock.Lock

	// /dev/tty, opened lazily and shared by the console messages
	ttyOnce sync.Once
	ttyFile *os.File
)

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
	flag.StringVar(&output, "output", "", "Redirect the log to a file. Use -o /dev/null for a quiet log; failures still appear on /dev/tty")
	flag.StringVar(&output, "o", "", "")
	flag.BoolVar(&lines, "lines", false, "Print source code line numbers for debugging")
	flag.BoolVar(&lines, "l", false, "")
	flag.BoolVar(&printtime, "time", false, "Print date and time for every line of output (ignored on syslog output)")
	flag.BoolVar(&printtime, "t", false, "")
	flag.BoolVar(&checkMode, "check", false, "Check every forwarding once and exit. One ssh connection is opened per service, so a security key may ask to be touched")
	flag.BoolVar(&checkConfigMode, "check-config", false, "Parse the configuration file and exit; opens no listener and no ssh connection")
	flag.StringVar(&configFile, "config", "", "Path of forwardings.json (default $XDG_CONFIG_HOME/sshportfw/forwardings.json)")
	flag.StringVar(&sshBinary, "ssh", "ssh", "ssh client to execute")
	flag.Parse()
	currflags := 0
	if version {
		fmt.Println("sshportfw Version " + versionString)
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
func sshInstance(localConn net.Conn, fw forwarding, host string, notifier *failureNotifier) {
	if localConn == nil {
		return
	}
	if notifier == nil {
		notifier = &failureNotifier{tag: fmt.Sprintf("%s-%s", host, fw.Service)}
	}
	id := routineCount.Inc()
	cmd := exec.Command(sshBinary, "-o", "ConnectTimeout="+sshConnectTimeout, "-W", fw.RemoteAddr, host)
	// ssh reports host key, authentication and connection problems on stderr.
	// Without this, os/exec discards the child's stderr and a failed forward
	// looks like a silent connection reset in the browser. The first line is
	// also shown on the console, rate limited.
	cmd.Stderr = &lineLogger{
		prefix: fmt.Sprintf("#%d ssh stderr:", id),
		notify: func(line string) { notifier.notify("%s", line) },
	}
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

// listenWithRetry binds addr, retrying every listenRetryDelay until it succeeds.
// It returns false when cancel is closed first, so that a port which can never
// be bound cannot block a reload or a shutdown.
func listenWithRetry(tag, addr string, cancel <-chan struct{}, notifier *failureNotifier) (net.Listener, bool) {
	for {
		localListener, err := net.Listen("tcp", addr)
		if err == nil {
			return localListener, true
		}
		log.Printf("%s listen failed at %s err=%q", tag, addr, err)
		notifier.notify("cannot listen on %s: %v", addr, err)
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
	notifier := &failureNotifier{tag: tag}

	localListener, ok := listenWithRetry(tag, fw.ListenAddr, cancel, notifier)
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
		go sshInstance(localConn, fw, host, notifier)
	}
}

func main() {
	log.SetOutput(os.Stdout)
	flagParse()
	// The location of the config dir on linux is ~/.config/sshportfw
	configPath := configdir.LocalConfig(Appname)
	path := configFile
	if path == "" {
		path = filepath.Join(configPath, ForwardingsPath)
	}
	// The path travels to the re-exec'd process, so make it independent of the
	// working directory.
	absPath, err := filepath.Abs(path)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	path = absPath

	// The SSH servers as defined in the config file
	allServers, err := getServers(path)
	if err != nil {
		log.Print(err)
		// we can't do anything else
		os.Exit(1)
	}

	// -check-config and -check only report. They start no listener and take no
	// lock, so they can run while the daemon is running.
	if checkConfigMode {
		fmt.Printf("configuration %s OK: %d server(s), %d forwarding(s)\n", path, len(allServers), countForwardings(allServers))
		return
	}
	if checkMode {
		os.Exit(runChecks(path, allServers))
	}

	if err := configdir.MakePath(configPath); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	// Do not allow 2 instances of the program to run at the same time
	if err := acquireLock(configPath); err != nil {
		log.Printf("%q : %s", err, "Check if the program is already running in another console or in the background.")
		os.Exit(1)
	}
	// The binary to run on SIGHUP, resolved once: a rebuild in place at this
	// path is picked up, a change of the symlink that points here is not.
	selfPath, err = os.Executable()
	if err != nil {
		log.Printf("Cannot determine the path of this executable, SIGHUP will only reload the configuration: %v", err)
	}

	// must have capacity of 1 accordig to docs
	osSignal := make(chan os.Signal, 1)
	// HUP restarts the process with the binary on disk, USR1 reloads the
	// configuration in place (live connections survive)
	signal.Notify(osSignal, os.Interrupt, syscall.SIGHUP, syscall.SIGUSR1)
	// these signals (in addition to SIGKILL) terminate the server
	signal.Notify(osSignal, os.Interrupt, syscall.SIGTERM)
	signal.Notify(osSignal, os.Interrupt, syscall.SIGABRT)
	signal.Notify(osSignal, os.Interrupt, syscall.SIGINT)

	log.Printf("%s version %s starting, %d forwarding(s) from %s", Appname, versionString, countForwardings(allServers), path)
	for {
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
	signals:
		for {
			sig := <-osSignal // we wait here until an OS signal is caught
			switch sig {
			case syscall.SIGHUP:
				// Restart with the binary on disk, so an upgraded executable is
				// used. Everything that can fail happens before the listeners
				// and the lock are released.
				log.Printf("Signal %q, restarting with the binary at %s", sig, selfPath)
				updated, err := getServers(path)
				if err != nil {
					log.Print("Config file has errors, not restarting. Fix the errors and send SIGHUP again")
					log.Print(err)
					continue
				}
				if selfPath == "" {
					log.Print("The path of this executable is unknown, reloading the configuration instead of restarting")
					allServers = updated
					close(cancel)
					wg.Wait()
					break signals
				}
				if err := preflight(path); err != nil {
					log.Printf("The executable at %q did not accept the configuration, not restarting: %v", selfPath, err)
					continue
				}
				// exec closes the pipes of the running ssh clients, so active
				// forwardings cannot survive the restart.
				log.Printf("Closing listening servers, %d active forwarding(s) will be dropped", activeForwardings.Value())
				close(cancel)
				wg.Wait()
				// fslock opens the lock without O_CLOEXEC, so it has to be
				// released explicitly or the new image would find itself
				// locked out.
				if err := instanceLock.Unlock(); err != nil {
					log.Printf("Cannot release the lock: %v. Not restarting", err)
					if err := acquireLock(configPath); err != nil {
						log.Printf("Cannot take the lock back: %v. Exiting", err)
						os.Exit(1)
					}
					allServers = updated
					break signals
				}
				if err := syscall.Exec(selfPath, os.Args, os.Environ()); err != nil {
					// Exec returns only on failure
					log.Printf("Cannot exec %q: %v. Continuing with the old binary", selfPath, err)
					if err := acquireLock(configPath); err != nil {
						log.Printf("Cannot take the lock back: %v. Another instance may be running, exiting", err)
						os.Exit(1)
					}
					allServers = updated
					break signals
				}
			case syscall.SIGUSR1:
				// Reload the configuration in this process. The listeners are
				// restarted, active connections are not affected.
				log.Printf("Signal %q, reloading the configuration", sig)
				updated, err := getServers(path)
				if err == nil {
					allServers = updated
					close(cancel)
					wg.Wait()
					log.Print("All listening servers stopped but active connections are not affected, now reloading servers from the config file")
					break signals
				}
				log.Print("Config file has errors, leaving the previous configuration. Fix the errors in the config file and send a SIGUSR1 signal again")
				log.Print(err)
			default:
				log.Printf("Signal %q, program ends", sig)
				log.Print("Closing listening servers")
				close(cancel) // not necesary as os will close all listening ports anyway
				wg.Wait()
				// We dont stop any active connections, we probably should TODO
				return // terminate
			}
		}
	}
}

// acquireLock takes the single-instance lock in the config directory. It is
// called again when a re-exec fails and the daemon keeps running.
func acquireLock(configPath string) error {
	if instanceLock == nil {
		instanceLock = fslock.New(filepath.Join(configPath, "lock"))
	}
	return instanceLock.TryLock()
}

func countForwardings(allServers []serverInfo) int {
	n := 0
	for _, info := range allServers {
		n += len(info.Forward)
	}
	return n
}

// how long the -check-config child may take before we give up on a restart
const preflightTimeout = 5 * time.Second

// preflight runs this executable with -check-config, so a binary that cannot
// even parse the configuration never replaces the running daemon.
func preflight(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), preflightTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, selfPath, "-check-config", "-config", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("no answer within %s", preflightTimeout)
	}
	if err != nil {
		return fmt.Errorf("%v: %s", err, firstLine(out.String()))
	}
	return nil
}

// how long an ssh -W probe may stay alive before we consider the remote side
// reachable. A failing ssh exits within this window.
const checkProbeWait = 5 * time.Second

// runChecks probes every forwarding once, prints one line per service and
// returns the process exit code. Connections are closed again immediately; the
// lazy design of the daemon is unaffected because this is a separate command.
func runChecks(path string, allServers []serverInfo) int {
	total := 0
	for _, info := range allServers {
		total += len(info.Forward)
	}
	fmt.Printf("checking %d forwarding(s) from %s\n", total, path)

	failures := 0
	for _, info := range allServers {
		for _, fw := range info.Forward {
			tag := fmt.Sprintf("%s-%s", info.Host, fw.Service)
			ok, reason := probeForwarding(info.Host, fw)
			if ok {
				fmt.Printf("ok    %-32s %-22s -> %s\n", tag, fw.ListenAddr, fw.RemoteAddr)
				continue
			}
			failures++
			fmt.Printf("FAIL  %-32s %-22s -> %s : %s\n", tag, fw.ListenAddr, fw.RemoteAddr, reason)
		}
	}
	if failures > 0 {
		fmt.Printf("%d of %d forwarding(s) failed\n", failures, total)
		return 1
	}
	fmt.Printf("all %d forwarding(s) ok\n", total)
	return 0
}

// probeForwarding runs "ssh -W" the same way a local connection would and
// decides from its behaviour: ssh exits when the remote port cannot be reached
// or authentication fails, and stays alive when the channel is open.
func probeForwarding(host string, fw forwarding) (bool, string) {
	cmd := exec.Command(sshBinary, "-o", "ConnectTimeout="+sshConnectTimeout, "-W", fw.RemoteAddr, host)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return false, err.Error()
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		return false, err.Error()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if reason := firstLine(stderr.String()); reason != "" {
			return false, reason
		}
		if err != nil {
			return false, err.Error()
		}
		return false, "ssh exited"
	case <-time.After(checkProbeWait):
		// still running: the remote side accepted the channel. Do not leave
		// the connection open.
		cmd.Process.Kill()
		<-done
		return true, ""
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
