package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// Support both flags (-mode) and simple subcommands (server / client)
	modeFlag := flag.String("mode", "", "Mode to run: 'server' or 'client'")
	portFlag := flag.Int("port", 9999, "Port to listen on or connect to")
	ipFlag := flag.String("ip", "127.0.0.1", "Server IPv4 address (for client mode)")

	flag.Parse()

	mode := *modeFlag
	port := *portFlag
	ip := *ipFlag

	// Also check positional arguments for easy shorthand (e.g., `go run . server 9999` or `go run . client 127.0.0.1 9999`)
	args := flag.Args()
	if mode == "" && len(args) > 0 {
		switch args[0] {
		case "server":
			mode = "server"
			if len(args) > 1 {
				fmt.Sscanf(args[1], "%d", &port)
			}
		case "client":
			mode = "client"
			if len(args) > 1 {
				ip = args[1]
			}
			if len(args) > 2 {
				fmt.Sscanf(args[2], "%d", &port)
			}
		}
	}

	switch mode {
	case "server":
		runServer(port)
	case "client":
		runClient(ip, port)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("==================================================")
	fmt.Println("UNIX Network Programming - Exercises 1.2 & 1.4")
	fmt.Println("==================================================")
	fmt.Println("Usage with flags:")
	fmt.Println("  go run . -mode=server [-port=9999]")
	fmt.Println("  go run . -mode=client [-ip=127.0.0.1] [-port=9999]")
	fmt.Println("\nOr usage with subcommands:")
	fmt.Println("  go run . server [port]")
	fmt.Println("  go run . client [IP] [port]")
	fmt.Println("\nExamples:")
	fmt.Println("  go run . server 9999")
	fmt.Println("  go run . client 127.0.0.1 9999")
	fmt.Println("==================================================")
}
