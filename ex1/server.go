package main

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/sys/unix"
)

// runServer executes the TCP Daytime Server logic.
func runServer(port int) {
	// Step 1: Create low-level IPv4 TCP socket
	lfd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		log.Fatalf("socket error: %v", err)
	}
	defer unix.Close(lfd)

	// Step 2: Enable SO_REUSEADDR to avoid TIME_WAIT bind failure
	if err := unix.SetsockoptInt(lfd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		log.Fatalf("setsockopt error: %v", err)
	}

	// Step 3: Bind to 0.0.0.0:port
	addr := &unix.SockaddrInet4{
		Port: port,
		Addr: [4]byte{0, 0, 0, 0}, // INADDR_ANY
	}

	if err := unix.Bind(lfd, addr); err != nil {
		log.Fatalf("bind error: %v", err)
	}

	// Step 4: Listen for incoming connections
	if err := unix.Listen(lfd, 128); err != nil {
		log.Fatalf("listen error: %v", err)
	}

	fmt.Printf("[Server] Daytime server listening on 0.0.0.0:%d (Press Ctrl+C to stop)\n", port)

	// Step 5: Iterative Accept Loop
	for {
		connfd, from, err := unix.Accept(lfd)
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}

		if clientAddr, ok := from.(*unix.SockaddrInet4); ok {
			fmt.Printf("[Server] Accepted connection from %d.%d.%d.%d:%d\n",
				clientAddr.Addr[0], clientAddr.Addr[1], clientAddr.Addr[2], clientAddr.Addr[3], clientAddr.Port)
		}

		// Format current time string (Daytime protocol format)
		timeStr := time.Now().Format(time.ANSIC) + "\r\n"

		// Send time to client
		_, err = unix.Write(connfd, []byte(timeStr))
		if err != nil {
			log.Printf("write error: %v", err)
		}

		// Close connected descriptor
		unix.Close(connfd)
	}
}
