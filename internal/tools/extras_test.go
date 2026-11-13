package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestWebClientSafetyAndArgs(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1", "169.254.1.1", "0.0.0.0"} {
		if !isPrivateIP(net.ParseIP(ip)) {
			t.Fatalf("%s should be private", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if isPrivateIP(net.ParseIP(ip)) {
			t.Fatalf("%s should be public", ip)
		}
	}
	if !isPrivateIP(nil) {
		t.Fatal("nil IP must be treated as private")
	}

	cfg := config.ToolsConfig{HTTPTimeout: config.Duration(2 * time.Second), MaxFetchBytes: 1024,
		UserAgent: "ua", BlockPrivateIPs: true}
	guarded := NewWebClient(cfg)
	if _, _, err := guarded.Get(context.Background(), "http://127.0.0.1:1/x", 0); err == nil {
		t.Fatal("private address not blocked")
	}
	if _, _, err := guarded.Get(context.Background(), "ftp://example.com/x", 0); err == nil {
		t.Fatal("non-http scheme accepted")
	}
	if _, _, err := guarded.Get(context.Background(), "http:///nohost", 0); err == nil {
		t.Fatal("missing host accepted")
	}

	if intArg(map[string]any{"n": json.Number("7")}, "n", 1) != 7 {
		t.Fatal("json.Number arg")
	}
	if intArg(map[string]any{"n": 3}, "n", 1) != 3 {
		t.Fatal("int arg")
	}
	if intArg(map[string]any{"n": 2.9}, "n", 1) != 2 {
		t.Fatal("float arg")
	}
	if intArg(map[string]any{}, "n", 9) != 9 {
		t.Fatal("fallback arg")
	}
	if stringArg(map[string]any{"s": " x "}, "s") != "x" || stringArg(map[string]any{}, "s") != "" {
		t.Fatal("string arg")
	}
	schema := Schema(map[string]any{"q": map[string]any{"type": "string"}}, "q")
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil || parsed["required"] == nil {
		t.Fatalf("schema: %s", schema)
	}
}

func TestWikiSearchAndBuildAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/w/api.php" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `["tea",["Tea"],["a drink"],["https://wiki/Tea"]]`)
	}))
	defer srv.Close()
	client := clientFor(srv)
	results, err := wikiSearch(context.Background(), client, "en", "tea", 3)
	if err != nil || len(results) != 1 || results[0].Title != "Tea" {
		t.Fatalf("wikiSearch: %v %+v", err, results)
	}

	reg := NewRegistry()
	full := config.Default()
	full.Tools = config.ToolsConfig{InternetEnabled: false, SearchProvider: "mock"}
	if err := BuildAll(reg, Deps{Config: full}); err != nil {
		t.Fatal(err)
	}
	names := reg.Names()
	for _, want := range []string{"web_search", "web_fetch", "knowledge_lookup", "navigate_route"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("BuildAll missing %s (have %v)", want, names)
		}
	}
	if _, ok := reg.Get("device_command"); ok {
		t.Fatal("device tools registered without a device API")
	}
}

func TestEmailMessageAndSend(t *testing.T) {
	msg := buildMessage("from@test.local", "to@test.local", "Hello\r\nBcc: evil", "body line")
	text := string(msg)
	if !strings.Contains(text, "From: from@test.local") || !strings.Contains(text, "Subject: Hello Bcc: evil") {
		t.Fatalf("message headers: %s", text)
	}
	if strings.Contains(text, "\r\nBcc:") {
		t.Fatal("header injection not sanitised")
	}
	if NewSMTP(SMTPConfig{}) != nil {
		t.Fatal("incomplete SMTP config accepted")
	}
	sender := NewSMTP(SMTPConfig{Host: "smtp.test", From: "a@b.c"})
	if sender == nil {
		t.Fatal("sender not built")
	}
	if err := sender.Send(context.Background(), "not-an-email", "s", "b"); err == nil {
		t.Fatal("invalid recipient accepted")
	}

	addr, received := startFakeSMTP(t)
	sender = NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: addr, From: "a@test.local"})
	if sender == nil {
		t.Fatal("sender not built")
	}
	if err := sender.Send(context.Background(), "b@test.local", "Subject", "Body text"); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case data := <-received:
		if !strings.Contains(data, "Subject: Subject") || !strings.Contains(data, "Body text") {
			t.Fatalf("server received: %q", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("smtp server did not receive the message")
	}
}

// startFakeSMTP accepts one message and returns the port plus captured DATA.
func startFakeSMTP(t *testing.T) (int, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 localhost ESMTP\r\n")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					received <- data.String()
					fmt.Fprint(conn, "250 OK\r\n")
					continue
				}
				data.WriteString(line)
				continue
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				fmt.Fprint(conn, "250-localhost\r\n250-AUTH PLAIN\r\n250 OK\r\n")
			case strings.HasPrefix(cmd, "AUTH"):
				fmt.Fprint(conn, "235 Authentication successful\r\n")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"), strings.HasPrefix(cmd, "RSET"):
				fmt.Fprint(conn, "250 OK\r\n")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				fmt.Fprint(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				fmt.Fprint(conn, "221 Bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 OK\r\n")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, received
}

var _ = ports.Tool{}
