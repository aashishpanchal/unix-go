package main

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// runClient executes the TCP Daytime Client logic.
func runClient(ipStr string, port int) {
	// Step 1: Validate and parse IPv4 address
	parsedIP := net.ParseIP(ipStr).To4()
	if parsedIP == nil {
		fmt.Fprintf(os.Stderr, "Invalid IPv4 address: %s\n", ipStr)
		os.Exit(1)
	}

	var ip4Bytes [4]byte
	copy(ip4Bytes[:], parsedIP)

	// Step 2: Create low-level socket (AF_INET, SOCK_STREAM)
	sockfd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "socket error: %v\n", err)
		os.Exit(1)
	}
	defer unix.Close(sockfd)

	// Step 3: Populate sockaddr_in
	addr := &unix.SockaddrInet4{
		Port: port,
		Addr: ip4Bytes,
	}

	// Step 4: Connect to server (TCP 3-Way Handshake)
	fmt.Printf("[Client] Connecting to %s:%d...\n", ipStr, port)
	if err := unix.Connect(sockfd, addr); err != nil {
		fmt.Fprintf(os.Stderr, "connect error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[Client] Connected! Waiting for daytime server response:\n\n")

	// Step 5: Read data in a loop until EOF (n == 0)
	buf := make([]byte, 4096)
	for {
		n, err := unix.Read(sockfd, buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if n == 0 {
			// EOF: Server closed the connection
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("\n[Client] Connection closed by server.")
}
