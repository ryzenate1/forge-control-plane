package mail

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

func TestSMTPSenderDeliversWithAuthentication(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var mu sync.Mutex
	var message string
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		write := func(s string) { _, _ = fmt.Fprint(w, s); _ = w.Flush() }
		write("220 fake ESMTP\r\n")
		inData := false
		var data strings.Builder
		for {
			line, e := r.ReadString('\n')
			if e != nil {
				done <- e
				return
			}
			if inData {
				if line == ".\r\n" {
					mu.Lock()
					message = data.String()
					mu.Unlock()
					inData = false
					write("250 queued\r\n")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				write("250-fake\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(line, "AUTH PLAIN"):
				write("235 authenticated\r\n")
			case strings.HasPrefix(line, "MAIL FROM"), strings.HasPrefix(line, "RCPT TO"):
				write("250 ok\r\n")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				write("354 send\r\n")
			case strings.HasPrefix(line, "QUIT"):
				write("221 bye\r\n")
				done <- nil
				return
			default:
				write("250 ok\r\n")
			}
		}
	}()
	settings := store.PanelMailSettings{SMTPHost: "127.0.0.1", SMTPPort: listener.Addr().(*net.TCPAddr).Port, SMTPUsername: "user", SMTPPassword: "pass", MailFromAddress: "panel@example.com", MailFromName: "Forge"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := (SMTPSender{Timeout: time.Second}).Send(ctx, settings, "user@example.com", "Test", "hello", ""); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := message
	mu.Unlock()
	if !strings.Contains(got, "Subject: Test") || !strings.Contains(got, "hello") {
		t.Fatalf("unexpected message: %q", got)
	}
}
