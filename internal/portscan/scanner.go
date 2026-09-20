package portscan

import (
	"context"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Options struct {
	First       int
	Last        int
	Timeout     time.Duration
	Concurrency int
	Passes      int
}

func OpenTCP(ctx context.Context, ip net.IP, options Options) []int {
	if ip == nil || ip.To4() == nil || options.First < 1 || options.Last < options.First || options.Last > 65535 {
		return nil
	}
	if options.Timeout <= 0 {
		options.Timeout = 300 * time.Millisecond
	}
	if options.Concurrency < 1 {
		options.Concurrency = 128
	}
	if options.Concurrency > 1024 {
		options.Concurrency = 1024
	}
	if options.Passes < 1 {
		options.Passes = 1
	}
	found := map[int]bool{}
	for pass := 0; pass < options.Passes && ctx.Err() == nil; pass++ {
		for _, port := range scanPass(ctx, ip, options) {
			found[port] = true
		}
	}
	ports := make([]int, 0, len(found))
	for port := range found {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func FirstOpenTCP(ctx context.Context, ip net.IP, options Options) (int, bool) {
	if ip == nil || ip.To4() == nil || options.First < 1 || options.Last < options.First || options.Last > 65535 {
		return 0, false
	}
	if options.Timeout <= 0 {
		options.Timeout = 300 * time.Millisecond
	}
	if options.Concurrency < 1 {
		options.Concurrency = 128
	}
	if options.Concurrency > 1024 {
		options.Concurrency = 1024
	}
	scanContext, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	result := make(chan int, 1)
	var waitGroup sync.WaitGroup
	for index := 0; index < options.Concurrency; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for port := range jobs {
				if ReachableTCP(scanContext, ip, port, options.Timeout) {
					select {
					case result <- port:
						cancel()
					case <-scanContext.Done():
					}
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for port := options.First; port <= options.Last; port++ {
			select {
			case jobs <- port:
			case <-scanContext.Done():
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(done)
	}()
	select {
	case port := <-result:
		return port, true
	case <-done:
		return 0, false
	case <-ctx.Done():
		return 0, false
	}
}

func ReachableTCP(ctx context.Context, ip net.IP, port int, timeout time.Duration) bool {
	if ip == nil || ip.To4() == nil || port < 1 || port > 65535 {
		return false
	}
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func scanPass(ctx context.Context, ip net.IP, options Options) []int {
	jobs := make(chan int)
	results := make(chan int, options.Last-options.First+1)
	var waitGroup sync.WaitGroup
	for index := 0; index < options.Concurrency; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for port := range jobs {
				if ReachableTCP(ctx, ip, port, options.Timeout) {
					select {
					case results <- port:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for port := options.First; port <= options.Last; port++ {
			select {
			case jobs <- port:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		waitGroup.Wait()
		close(results)
	}()
	var ports []int
	for port := range results {
		ports = append(ports, port)
	}
	return ports
}
