# Low-Level Linux Network Programming & Epoll in Go

A deep-dive reference guide into low-level Linux network programming, non-blocking sockets, and the `epoll` event-loop architecture implemented in Go (`golang.org/x/sys/unix`).

> 💡 **Deep-Dive Reading**: For an in-depth breakdown of TCP connections, packet headers, the 3-Way & 4-Way Handshakes, Ghost/Half-Open connections, Keep-Alives, and the Two Generals' Problem, see **[TCP_INTERNALS.md](file:///home/aashish/Developer/golang/unix/TCP_INTERNALS.md)**.

---

## 📌 Table of Contents
1. [Architecture Overview](#1-architecture-overview)
2. [Step-by-Step System Call Breakdown](#2-step-by-step-system-call-breakdown)
   - [Socket Creation (`unix.Socket`)](#socket-creation-unixsocket)
   - [Socket Options (`SO_REUSEADDR` vs `SO_REUSEPORT`)](#socket-options-so_reuseaddr-vs-so_reuseport)
   - [Binding & Listening (`Bind` & `Listen`)](#binding--listening-bind--listen)
   - [Epoll Instance (`EpollCreate1`)](#epoll-instance-epollcreate1)
   - [Epoll Control & Event Flags (`EpollCtl`)](#epoll-control--event-flags-epollctl)
   - [Accepting Connections (`Accept4`)](#accepting-connections-accept4)
   - [Non-Blocking Read/Write & `EAGAIN`](#non-blocking-readwrite--eagain)
3. [Epoll vs Select / Poll (Why Epoll is $O(1)$)](#3-epoll-vs-select--poll-why-epoll-is-o1)
4. [Capacity & Scale: 1 Core / 1 GB RAM](#4-capacity--scale-1-core--1-gb-ram)
5. [The Blocking Trap & Concurrency Proof](#5-the-blocking-trap--concurrency-proof)
6. [Why Redis Uses Epoll (And Doesn't Block)](#6-why-redis-uses-epoll-and-doesnt-block)
7. [Running & Benchmarking](#7-running--benchmarking)
8. [Interview Quick-Reference Sheet](#8-interview-quick-reference-sheet)

---

## 1. Architecture Overview

### Go Standard Library (`net/http`) vs Single-Threaded Epoll
| Feature | Standard Go (`net`) | This Project (`epoll`) |
| :--- | :--- | :--- |
| **Model** | 1 Goroutine per connection | 1 Single-Threaded Event Loop |
| **Memory per Conn** | ~2 KB – 4 KB (goroutine stack) | ~64 – 100 Bytes (`Conn` struct) |
| **Context Switching** | Go Runtime M:N scheduler switches | Zero thread context switches |
| **Synchronization** | Mutex / Channels needed | Lock-free (all runs on 1 thread) |
| **Blocking I/O** | Safe (other goroutines continue) | Dangerous (freezes entire server) |
| **Used by** | Standard Go web services | Nginx, Redis, Node.js, Envoy |

---

## 2. Step-by-Step System Call Breakdown

### Socket Creation (`unix.Socket`)
```go
lfd, err := unix.Socket(
    unix.AF_INET,
    unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC,
    0,
)
```
* **`AF_INET`**: Address Family for IPv4. (`AF_INET6` = IPv6, `AF_UNIX` = Linux Domain Sockets for ultra-fast local IPC).
* **`SOCK_STREAM`**: Sequenced, reliable, two-way, connection-based byte streams (**TCP**). (`SOCK_DGRAM` = UDP).
* **`SOCK_NONBLOCK`**: Sets the socket to non-blocking mode (`O_NONBLOCK`). System calls (`read`, `write`, `accept`) return immediately instead of putting the CPU thread to sleep.
* **`SOCK_CLOEXEC`** *(Close-on-Exec)*: Ensures this file descriptor is automatically closed if this process invokes `execve()` to spawn a child process. This prevents security leaks and dangling socket descriptors in child processes.

---

### Socket Options (`SO_REUSEADDR` vs `SO_REUSEPORT`)
```go
unix.SetsockoptInt(lfd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
```

#### Why is `SO_REUSEADDR` essential?
When a server shuts down or restarts, TCP sockets enter the **`TIME_WAIT`** state for **$2 \times \text{MSL}$** (Maximum Segment Lifetime, roughly 60 seconds).
* **Without `SO_REUSEADDR`**: Restarting the server fails immediately with `bind: address already in use`. You have to wait 60 seconds for the kernel to clean up.
* **With `SO_REUSEADDR`**: The kernel allows the new process to bind to the port immediately, even if previous sockets on that port are still in `TIME_WAIT`.

#### `SO_REUSEADDR` vs `SO_REUSEPORT`:
* **`SO_REUSEADDR`**: Allows reuse of a port in `TIME_WAIT`. Only **one** process can be actively listening on that IP:Port at any time.
* **`SO_REUSEPORT`**: Allows **multiple distinct processes or threads** to bind and listen to the exact same IP:Port simultaneously. The Linux kernel load-balances incoming connections across all listening sockets.

---

### Binding & Listening (`Bind` & `Listen`)
```go
addr := &unix.SockaddrInet4{Port: 8081}
addr.Addr = [4]byte{0, 0, 0, 0}
unix.Bind(lfd, addr)
unix.Listen(lfd, 128)
```
* **`Bind`**: Assigns the IP address (`0.0.0.0` = all network interfaces) and port to the socket file descriptor.
* **`Listen(lfd, backlog=128)`**: Transitions the socket from active to passive (ready to accept connections).
  * `128` is the **Backlog Queue** size (the maximum length of completely established TCP connections waiting to be accepted by `accept()`).
  * If the backlog is full, incoming client connections are rejected or dropped.

---

### Epoll Instance (`EpollCreate1`)
```go
epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
```
* Creates an `epoll` file descriptor in the Linux kernel.
* Uses `EPOLL_CLOEXEC` to prevent descriptor leakage to child processes.

---

### Epoll Control & Event Flags (`EpollCtl`)
```go
unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{
    Events: unix.EPOLLIN | unix.EPOLLRDHUP,
    Fd:     int32(fd),
})
```

#### Operations:
* `EPOLL_CTL_ADD`: Register a new file descriptor to monitor.
* `EPOLL_CTL_MOD`: Change monitored events for an existing FD (e.g. enable/disable `EPOLLOUT`).
* `EPOLL_CTL_DEL`: Stop monitoring and remove an FD from the epoll interest list.

#### Event Flags:
* **`unix.EPOLLIN`**: The file descriptor is ready for reading (`read()` will not block).
* **`unix.EPOLLOUT`**: The socket's kernel write buffer has space available (`write()` will not block).
* **`unix.EPOLLRDHUP`**: The remote peer closed the connection or shut down the writing half of the connection. This detects client disconnects early without needing an explicit `read()` error.
* **`unix.EPOLLERR`**: Error condition happened on the associated file descriptor.
* **`unix.EPOLLHUP`**: Hang up happened on the associated file descriptor.

---

### Accepting Connections (`Accept4`)
```go
fd, _, err := unix.Accept4(lfd, unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC)
```
* Traditional `accept()` returns a blocking socket without `CLOEXEC`. Setting non-blocking mode required two additional `fcntl` system calls (`F_SETFL` and `F_SETFD`).
* **`Accept4`** performs all three operations **atomically in a single system call**, eliminating syscall overhead and preventing race conditions.

---

### Non-Blocking Read/Write & `EAGAIN`
```go
n, err := unix.Read(c.fd, buf)
if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
    break // All available data currently in buffer has been consumed
}
```
* In non-blocking mode, when a socket buffer is empty, `Read` does not wait; it returns `EAGAIN` or `EWOULDBLOCK`.
* **This is not an error.** It signals that all incoming data currently delivered by the network card has been read.
* When `n == 0` and `err == nil`, it represents **EOF** (the client has cleanly closed the TCP connection).

---

## 3. Epoll vs Select / Poll (Why Epoll is $O(1)$)

| Mechanism | Kernel Data Structure | Complexity | Scalability Limit |
| :--- | :--- | :--- | :--- |
| **`select`** | Bit array (FD set) | $O(N)$ linear scan | Limited to 1024 FDs (`FD_SETSIZE`) |
| **`poll`** | Array of `pollfd` structs | $O(N)$ linear scan | No hard cap, but CPU degrades linearly |
| **`epoll`** | **Red-Black Tree** + **Ready List** | **$O(1)$ event-driven** | Millions of connections with 0% CPU waste |

### How Epoll works internally:
1. **Red-Black Tree**: Stores all monitored file descriptors. Adding, modifying, or removing an FD takes $O(\log N)$.
2. **Ready List (Doubly-Linked List)**: When network packets hit the network interface card (NIC), the hardware triggers an interrupt. The kernel driver appends only the affected sockets into the Ready List.
3. **`EpollWait`**: Returns only the sockets already sitting in the Ready List in $O(1)$ time, completely ignoring millions of idle sockets.

---

## 4. Capacity & Scale: 1 Core / 1 GB RAM

### A. Concurrent Idle Connections (C100K Problem)
* **Go struct overhead**: `Conn{fd: int, out: []byte}` is under **100 bytes** in user-space RAM.
* **Kernel TCP socket buffers**: Tunable to ~2 KB – 4 KB per connection via `sysctl`:
  ```bash
  sysctl -w net.ipv4.tcp_rmem="2048 4096 8192"
  sysctl -w net.ipv4.tcp_wmem="2048 4096 8192"
  ```
* **Result**: On 1 GB RAM, this architecture can hold **100,000 to 200,000+ idle connections**.

### B. Active Requests Per Second (Throughput)
* On 1 dedicated CPU core with purely non-blocking CPU operations (like Echo or Redis in-memory lookup), this server can deliver **50,000 to 100,000+ RPS** because there are zero thread switches and zero lock contentions.

---

## 5. The Blocking Trap & Concurrency Proof

### The Danger:
Because the entire server runs in a **single-threaded loop**, calling any synchronous/blocking function (e.g. database query, disk I/O, `time.Sleep`) **freezes the entire server** for all clients.

### Proof with `test_client.go`:
When a 1-second synchronous DB query was added to `readClient`:
```text
=== Starting 20 Concurrent Clients to 127.0.0.1:8081 ===
[T+1.00s]  Client  2 received response (took 1.00s)
[T+2.00s]  Client  3 received response (took 2.00s)
[T+3.00s]  Client  1 received response (took 3.00s)
...
[T+20.01s] Client  8 received response (took 20.01s)
==================================================
Total Time: 20.01 seconds (Sequential Execution!)
==================================================
```
* 20 concurrent clients took **20 full seconds** because each request blocked the single event loop.
* **Solution**: In real Go applications, network I/O remains non-blocking, but slow operations (DB, computation) are delegated to a **Worker Pool** or **Goroutines**.

---

## 6. Why Redis Uses Epoll (And Doesn't Block)

If blocking halts the event loop, why does Redis use single-threaded `epoll`?
1. **In-Memory Operations (Nanoseconds vs Milliseconds)**: Redis works entirely in RAM. Accessing RAM takes ~50-100 ns. Executing `GET` or `SET` is so fast that the event loop never feels stalled.
2. **Lock-Free Concurrency**: No mutexes, spinlocks, or race conditions. Every command is 100% atomic by default.
3. **CPU Cache Locality**: Running on one core maximizes L1/L2 CPU cache utilization without cache-line bouncing.
4. **Background Threads for Slow Tasks**: Redis is single-threaded for command execution, but uses background threads for disk persistence (`RDB`/`AOF`), asynchronous deletion (`UNLINK`), and threaded socket I/O (Redis 6.0+).

---

## 7. Running & Benchmarking

### 1. Increase OS Open File Limits
```bash
ulimit -n 1000000
```

### 2. Start the Server
```bash
go run cmd/main.go
```

### 3. Run the Concurrent Client Benchmark
```bash
go run tests/test_client.go
```

### 4. Connect Manually via Netcat
```bash
nc 127.0.0.1 8081
```

---

## 8. Interview Quick-Reference Sheet

| Question | Answer Summary |
| :--- | :--- |
| **Why `SO_REUSEADDR`?** | Allows instant rebinding to a port in the TCP `TIME_WAIT` state (~60s) after a restart. |
| **Why `SO_REUSEPORT`?** | Allows multiple distinct processes to bind to the same port for kernel-level load balancing. |
| **What does `Accept4` do?** | Atomically accepts a connection and applies `SOCK_NONBLOCK` + `SOCK_CLOEXEC` in a single syscall. |
| **What does `EAGAIN` mean?** | Non-blocking buffer has no more data right now; wait for the next `EPOLLIN` event. |
| **Why is `epoll` faster than `select`?** | `select` scans $O(N)$ descriptors; `epoll` uses a kernel ready-list to return active events in $O(1)$. |
| **Why is `EPOLLRDHUP` useful?** | Detects remote peer hangup/disconnect at the event loop without requiring a failing `read()`. |
| **Can single-threaded epoll scale?** | Yes, for I/O-bound in-memory tasks (Redis/Nginx), but blocking operations require worker threads/goroutines. |
