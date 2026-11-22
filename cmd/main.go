package main

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/sys/unix"
)

type Conn struct {
	fd  int
	out []byte
}

func main() {
	// Create non-blocking TCP socket
	lfd, err := unix.Socket(
		unix.AF_INET,
		unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC,
		0,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(lfd)

	unix.SetsockoptInt(lfd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)

	const port = 8081

	addr := &unix.SockaddrInet4{Port: port}
	addr.Addr = [4]byte{0, 0, 0, 0}

	if err := unix.Bind(lfd, addr); err != nil {
		log.Fatal(err)
	}

	if err := unix.Listen(lfd, 128); err != nil {
		log.Fatal(err)
	}

	// Create epoll instance
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(epfd)

	// Register listener
	err = unix.EpollCtl(
		epfd,
		unix.EPOLL_CTL_ADD,
		lfd,
		&unix.EpollEvent{
			Events: unix.EPOLLIN,
			Fd:     int32(lfd),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	conns := make(map[int]*Conn)
	events := make([]unix.EpollEvent, 128)

	log.Printf("Listening on :%d\n", port)

	for {
		n, err := unix.EpollWait(epfd, events, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			log.Fatal(err)
		}

		for i := 0; i < n; i++ {
			fd := int(events[i].Fd)
			ev := events[i].Events

			// Listener event
			if fd == lfd {
				acceptClients(epfd, lfd, conns)
				continue
			}

			c := conns[fd]
			if c == nil {
				continue
			}

			if ev&(unix.EPOLLERR|unix.EPOLLHUP) != 0 {
				closeConn(epfd, c, conns)
				continue
			}

			// Read event
			if ev&unix.EPOLLIN != 0 {
				if !readClient(epfd, c, conns) {
					continue
				}
			}

			// Write event
			if ev&unix.EPOLLOUT != 0 {
				flushClient(epfd, c, conns)
			}
		}
	}
}

func acceptClients(epfd, lfd int, conns map[int]*Conn) {
	for {
		fd, _, err := unix.Accept4(
			lfd,
			unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC,
		)

		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return
		}
		if err != nil {
			log.Println("accept:", err)
			return
		}

		c := &Conn{fd: fd}
		conns[fd] = c

		err = unix.EpollCtl(
			epfd,
			unix.EPOLL_CTL_ADD,
			fd,
			&unix.EpollEvent{
				Events: unix.EPOLLIN | unix.EPOLLRDHUP,
				Fd:     int32(fd),
			},
		)
		if err != nil {
			closeConn(epfd, c, conns)
			continue
		}

		log.Println("Client connected:", fd)
	}
}

// queryDatabaseDemo simulates a slow blocking database query without goroutines.
func queryDatabaseDemo() string {
	log.Println("[DB] Executing synchronous query (blocking event loop for 1s)...")
	time.Sleep(1 * time.Second)
	log.Println("[DB] Query completed.")
	return "DB response: record fetched\n"
}

func readClient(epfd int, c *Conn, conns map[int]*Conn) bool {
	buf := make([]byte, 4096)

	for {
		n, err := unix.Read(c.fd, buf)

		if n > 0 {
			// Synchronous DB query: blocks the entire thread and all other clients!
			dbResult := queryDatabaseDemo()
			c.out = append(c.out, []byte(dbResult)...)
			c.out = append(c.out, buf[:n]...)
		}

		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			break
		}

		if err != nil {
			closeConn(epfd, c, conns)
			return false
		}

		if n == 0 {
			closeConn(epfd, c, conns)
			return false
		}
	}

	if len(c.out) > 0 {
		flushClient(epfd, c, conns)
	}

	return conns[c.fd] != nil
}

func flushClient(epfd int, c *Conn, conns map[int]*Conn) {
	for len(c.out) > 0 {
		n, err := unix.Write(c.fd, c.out)

		if n > 0 {
			// Consume only successfully written bytes.
			c.out = c.out[n:]
		}

		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			break
		}

		if err != nil {
			closeConn(epfd, c, conns)
			return
		}

		if n == 0 {
			break
		}
	}

	// No pending data: listen only for read events.
	// Pending data: also listen for write readiness.
	mask := uint32(unix.EPOLLIN | unix.EPOLLRDHUP)
	if len(c.out) > 0 {
		mask |= unix.EPOLLOUT
	}

	err := unix.EpollCtl(
		epfd,
		unix.EPOLL_CTL_MOD,
		c.fd,
		&unix.EpollEvent{
			Events: mask,
			Fd:     int32(c.fd),
		},
	)
	if err != nil {
		closeConn(epfd, c, conns)
	}
}

func closeConn(epfd int, c *Conn, conns map[int]*Conn) {
	if _, ok := conns[c.fd]; !ok {
		return
	}

	unix.EpollCtl(epfd, unix.EPOLL_CTL_DEL, c.fd, nil)
	unix.Close(c.fd)
	delete(conns, c.fd)

	fmt.Println("Client disconnected:", c.fd)
}
