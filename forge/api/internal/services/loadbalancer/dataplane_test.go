package loadbalancer

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func TestTCPDataPlaneProxiesTraffic(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(conn)
		}
	}()
	port := freeTCPPort(t)
	svc := New(nil)
	svc.enabled = true
	svc.bindHost = "127.0.0.1"
	svc.portMin = port
	svc.portMax = port
	group := &TargetGroup{ID: "g1", Name: "tcp", Port: port, Protocol: "tcp"}
	if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	addr := backend.Addr().(*net.TCPAddr)
	if _, err := svc.AddTarget(context.Background(), "g1", "server", "node", "127.0.0.1", addr.Port, 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)
	var conn net.Conn
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		conn, err = net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmtInt(port)), 50*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("got %q", buf)
	}
}

func fmtInt(value int) string { return strconv.Itoa(value) }

func TestUDPDataPlaneProxiesTraffic(t *testing.T) {
	backend, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, client, err := backend.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			_, _ = backend.WriteToUDP(buffer[:n], client)
		}
	}()

	port := freeUDPPort(t)
	svc := New(nil)
	svc.enabled = true
	svc.bindHost = "127.0.0.1"
	svc.portMin = port
	svc.portMax = port
	group := &TargetGroup{ID: "g1", Name: "udp", Port: port, Protocol: "udp"}
	if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTarget(context.Background(), "g1", "server", "node", "127.0.0.1", backend.LocalAddr().(*net.UDPAddr).Port, 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)

	proxy, err := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", fmtInt(port)))
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.DialUDP("udp", nil, proxy)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	time.Sleep(50 * time.Millisecond)
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err = io.ReadFull(client, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "ping" {
		t.Fatalf("got %q", buffer)
	}
}
