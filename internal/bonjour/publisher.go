package bonjour

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

type Publisher struct {
	mutex     sync.Mutex
	processes []*publisherProcess
}

type publisherProcess struct {
	command *exec.Cmd
	done    chan error
}

func (publisher *Publisher) Start(services []config.BonjourService, iface net.Interface, proxyIP net.IP) error {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	if len(publisher.processes) > 0 {
		return fmt.Errorf("Bonjour publisher is already running")
	}
	if proxyIP == nil || proxyIP.To4() == nil {
		return fmt.Errorf("proxy IPv4 address is invalid")
	}
	for _, service := range services {
		arguments := []string{
			"-i", iface.Name,
			"-P", service.Instance,
			service.Type,
			service.Domain,
			strconv.Itoa(service.Port),
			service.Hostname,
			proxyIP.String(),
		}
		arguments = append(arguments, service.TXT...)
		command := exec.Command("/usr/bin/dns-sd", arguments...)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Start(); err != nil {
			publisher.stopLocked()
			return fmt.Errorf("publish %s.%s: %w", service.Instance, service.Type, err)
		}
		process := &publisherProcess{command: command, done: make(chan error, 1)}
		publisher.processes = append(publisher.processes, process)
		go func() { process.done <- command.Wait() }()
		select {
		case err := <-process.done:
			publisher.stopLocked()
			if err == nil {
				err = fmt.Errorf("dns-sd exited unexpectedly")
			}
			return fmt.Errorf("publish %s.%s: %w", service.Instance, service.Type, err)
		case <-time.After(150 * time.Millisecond):
		}
	}
	return nil
}

func (publisher *Publisher) Stop() {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	publisher.stopLocked()
}

func (publisher *Publisher) stopLocked() {
	for _, process := range publisher.processes {
		if process.command.Process == nil {
			continue
		}
		_ = process.command.Process.Signal(os.Interrupt)
		select {
		case <-process.done:
		case <-time.After(time.Second):
			_ = process.command.Process.Kill()
			select {
			case <-process.done:
			case <-time.After(time.Second):
			}
		}
	}
	publisher.processes = nil
}

func (publisher *Publisher) Healthy() bool {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()
	if len(publisher.processes) == 0 {
		return false
	}
	for _, process := range publisher.processes {
		if process.command.Process == nil || process.command.Process.Signal(syscall.Signal(0)) != nil {
			return false
		}
	}
	return true
}
