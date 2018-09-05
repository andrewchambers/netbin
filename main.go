package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

func main() {
	log.SetOutput(os.Stderr)

	ctx := context.Background()
	ctx, shutdown := context.WithCancel(ctx)

	domain := flag.String("domain", "tcp", "domain to listen on, valid domains are 'tcp' 'tcp4' 'tcp6' 'unix' ")
	address := flag.String("addr", "127.0.0.1:5877", "address to listen on")
	maxConcurrent := flag.Uint("max-concurrent", 20, "Maximum concurrent connections, 0 to disable")
	tcpKeepAlive := flag.Uint("tcp-keepalive", 120, "TCP keepalive in seconds, 0 to disable")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "netbin OPTIONS CMD CMDARGS\noptions:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
	}

	l, err := net.Listen(*domain, *address)
	if err != nil {
		log.Fatalf("unable to listen for connections: %s", err)
	}
	log.Printf("listening for connections on %s", l.Addr())

	wg := &sync.WaitGroup{}

	var concurrentConLimit *semaphore.Weighted

	hasConcurrencyControl := *maxConcurrent > 0
	if hasConcurrencyControl {
		concurrentConLimit = semaphore.NewWeighted(int64(*maxConcurrent))
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if hasConcurrencyControl {
				err := concurrentConLimit.Acquire(ctx, 1)
				if err != nil {
					// We must be shutting down
					return
				}
			}

			conn, err := l.Accept()
			if err != nil {
				log.Fatalf("unable to listen for new connections: %s", err)
				return
			}

			switch conn.(type) {
			case *net.TCPConn:
			case *net.UnixConn:
			default:
				log.Fatalf("unsupported connection type")
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				if hasConcurrencyControl {
					defer concurrentConLimit.Release(1)
				}

				switch conn := conn.(type) {
				case *net.TCPConn:
					if *tcpKeepAlive > 0 {
						conn.SetKeepAlive(true)
						conn.SetKeepAlivePeriod(time.Second * time.Duration(*tcpKeepAlive))
					}
				}

				var cmd *exec.Cmd

				if len(flag.Args()) == 1 {
					cmd = exec.CommandContext(ctx, flag.Args()[0])
				} else {
					cmd = exec.CommandContext(ctx, flag.Args()[0], flag.Args()[1:]...)
				}

				cmd.Stdin = conn
				cmd.Stdout = conn
				errormsgs, b := io.Pipe()
				cmd.Stderr = b

				err = cmd.Start()
				if err != nil {
					log.Printf("error starting subprocess for conn %s: %s", conn.RemoteAddr(), err)
				}

				pid := 0
				if cmd.Process != nil {
					pid = cmd.Process.Pid
				}

				if cmd.Process != nil {
					log.Printf("child pid=%d: serving conn from %s", pid, conn.RemoteAddr())
				}

				workerWg := &sync.WaitGroup{}
				workerWg.Add(1)
				go func() {
					defer workerWg.Done()
					const maxLineLen = 4096

					limitedLines := &io.LimitedReader{
						R: errormsgs,
						N: maxLineLen,
					}

					output := bufio.NewReader(limitedLines)
					for {
						limitedLines.N = maxLineLen

						ln, err := output.ReadString('\n')
						if len(ln) > 0 {
							log.Printf("child pid=%d: %s", pid, ln)
						}
						if err != nil {
							if limitedLines.N == 0 && err == io.EOF {
								continue
							}
							return
						}
					}
				}()

				if cmd.Process != nil {
					state, err := cmd.Process.Wait()
					if err == nil {
						log.Printf("child pid=%d: exited: success=%v", pid, state.Success())
					}
				}

				// Clearly if the process is dead, there is no one reading.
				// We don't close the connection outright in case
				// so that cmd.Wait can ensure the command output was
				// fully copied...
				//
				// TODO:
				// Not totally confident this is needed.
				// Need to look into some half duplex test cases?
				//
				switch conn := conn.(type) {
				case *net.TCPConn:
					_ = conn.CloseRead()
				case *net.UnixConn:
					_ = conn.CloseRead()
				default:
					panic("unreachable")
				}
				_ = cmd.Wait()
				_ = conn.Close()
				b.Close()
				workerWg.Wait()
			}()
		}
	}()

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)

	<-interrupted
	signal.Reset()
	log.Printf("shutting down...")
	_ = l.Close()
	shutdown()

	wg.Wait()
}
