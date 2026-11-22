# Exercise 1.2: TCP Daytime Client & Server (Golang)

Yeh guide **W. Richard Stevens** ki book *UNIX Network Programming (Chapter 1)* ki **Exercise 1.2** ko Golang ke **Single-Package Split-File Architecture** mein implement karti hai:
* Daytime Client connect karna aur alag-alag IP addresses test karna.
* Server se connect hokar current date aur time print karna.

Is architecture mein:
* `main.go`: Sirf ek `main()` function jo flags (`-mode=server` / `-mode=client`) aur subcommands ko parse karta hai.
* `client.go`: Pure Client logic (`runClient()`).
* `server.go`: Pure Server logic (`runServer()`).
* **Zero `redeclared main` errors**: Ab aap direct `go run .` ya `go build` chala sakte hain!

---

## 📂 Architecture Structure

```text
ex1/
├── main.go      <-- Single entry point (Flags & Subcommand Router)
├── client.go    <-- Daytime Client implementation (runClient)
├── server.go    <-- Daytime Server implementation (runServer)
└── README.md    <-- Exercise & Experiments Guide
```

---

## 🚀 Kaise Run Karein (Both Flags and Subcommands Supported)

### Option A: Using Subcommands (Fastest & Easiest)

#### 1. Start Server (Terminal 1)
```bash
cd ex1
go run . server 9999
```

#### 2. Run Client (Terminal 2)
```bash
cd ex1
go run . client 127.0.0.1 9999
```

---

### Option B: Using Flags (`-mode`)

#### 1. Start Server (Terminal 1)
```bash
cd ex1
go run . -mode=server -port=9999
```

#### 2. Run Client (Terminal 2)
```bash
cd ex1
go run . -mode=client -ip=127.0.0.1 -port=9999
```

---

## 🔍 Code Walkthrough (System Calls)

### 1. Client Logic ([client.go](file:///home/aashish/Developer/golang/unix/ex1/client.go))

- **`unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)`**: Creates IPv4 TCP file descriptor.
- **`unix.Connect(sockfd, addr)`**: Initiates the TCP 3-Way Handshake (`SYN` $\rightarrow$ `SYN-ACK` $\rightarrow$ `ACK`).
- **`unix.Read()` in a Loop**: Reads bytes until `n == 0` (EOF). TCP is a byte-stream, so reading in a loop guarantees you receive the entire message even if fragmented.

### 2. Server Logic ([server.go](file:///home/aashish/Developer/golang/unix/ex1/server.go))

- **`unix.SetsockoptInt(lfd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)`**: Prevents `address already in use` error during rapid restarts.
- **`unix.Bind(lfd, addr)`**: Binds socket to `0.0.0.0:port` (`INADDR_ANY`).
- **`unix.Listen(lfd, 128)`**: Transitions socket to passive listening state.
- **`unix.Accept(lfd)`**: Waits for incoming connection and returns a new connected file descriptor (`connfd`).
- **`unix.Write()` & `unix.Close()`**: Sends formatted time and closes connection.

---

## 🧪 Exercise 1.2 Experiments: Testing Different IP Addresses

Stevens ne is exercise mein alag-alag IP addresses pass karke Linux TCP stack ka behavior dekhne ko kaha tha:

### 1. Valid Local IP (Server Chalu Hai)

```bash
go run . client 127.0.0.1 9999
```

- **Output:**
  ```text
  [Client] Connecting to 127.0.0.1:9999...
  [Client] Connected! Waiting for daytime server response:

  Sun Nov 22 17:53:31 2026

  [Client] Connection closed by server.
  ```

---

### 2. Valid IP, Lekin Port Band Hai (Port Closed)

```bash
go run . client 127.0.0.1 8888
```

- **Output:**
  ```text
  connect error: connection refused
  ```
- **Under the Hood (`TCP RST` Proof):**
  - Client ne `SYN` bheja.
  - Kernel ne dekha port 8888 par koi listener nahi hai.
  - Kernel ne turant **`TCP RST` (Reset)** packet bheja, jisse client ko `connection refused` mila.

---

### 3. Dead / Unreachable IP on Local Subnet

```bash
go run . client 192.168.1.249 9999
```

- **Output:** (Lagbhag **75 seconds** rukega):
  ```text
  connect error: connection timed out
  ```
- **Under the Hood (`ETIMEDOUT` Proof):**
  - Client ne `SYN` bheja.
  - Us IP par koi machine nahi hai, isliye na `ACK` aaya na `RST`.
  - Linux kernel SYN retransmit karta raha jab tak timeout nahi ho gaya.

---

### 4. Invalid IP Format

```bash
go run . client 999.999.999.999 9999
```

- **Output:**
  ```text
  Invalid IPv4 address: 999.999.999.999
  ```
- **Under the Hood:**
  - Network request se pehle hi `net.ParseIP()` ne format validation error pakad liya.
