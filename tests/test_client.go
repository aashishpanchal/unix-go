package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	serverAddr   = "127.0.0.1:8081"
	totalClients = 20
)

func main() {
	fmt.Printf("=== Starting %d Concurrent Clients to %s ===\n", totalClients, serverAddr)
	overallStart := time.Now()

	var wg sync.WaitGroup

	for i := 1; i <= totalClients; i++ {
		wg.Add(1)
		clientID := i

		// Client side uses goroutine to simulate real concurrent users
		go func(id int) {
			defer wg.Done()

			start := time.Now()

			conn, err := net.Dial("tcp", serverAddr)
			if err != nil {
				fmt.Printf("[Client %2d] Connection error: %v\n", id, err)
				return
			}
			defer conn.Close()

			// Send message
			msg := fmt.Sprintf("Ping from client %d\n", id)
			_, err = conn.Write([]byte(msg))
			if err != nil {
				fmt.Printf("[Client %2d] Write error: %v\n", id, err)
				return
			}

			// Read response (reading until we get the echo or DB response)
			reader := bufio.NewReader(conn)
			response, err := reader.ReadString('\n')
			if err != nil {
				fmt.Printf("[Client %2d] Read error: %v\n", id, err)
				return
			}

			elapsed := time.Since(start)
			finishOffset := time.Since(overallStart)

			fmt.Printf("[T+%.2fs] Client %2d received response: %q (took %.2fs)\n",
				finishOffset.Seconds(), id, response[:len(response)-1], elapsed.Seconds())
		}(clientID)
	}

	wg.Wait()
	totalElapsed := time.Since(overallStart)

	fmt.Println("==================================================")
	fmt.Printf("Total Time for %d requests: %.2f seconds\n", totalClients, totalElapsed.Seconds())
	if totalElapsed >= time.Duration(totalClients-2)*time.Second {
		fmt.Printf("ANALYSIS: Requests were processed SEQUENTIALLY (1-by-1) because the server event loop is blocked!\n")
	} else {
		fmt.Printf("ANALYSIS: Requests were processed CONCURRENTLY!\n")
	}
	fmt.Println("==================================================")
}
