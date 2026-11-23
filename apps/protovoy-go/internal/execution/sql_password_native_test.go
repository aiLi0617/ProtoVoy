//go:build !js

package execution

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestBoundPostgresPasswordNeverFallsBackToPlaintext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	checked := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			checked <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		var packet [8]byte
		if _, err := io.ReadFull(conn, packet[:]); err != nil {
			checked <- err
			return
		}
		if binary.BigEndian.Uint32(packet[:4]) != 8 || binary.BigEndian.Uint32(packet[4:]) != 80877103 {
			t.Error("expected PostgreSQL TLS negotiation as the first packet")
		}
		if _, err := conn.Write([]byte("N")); err != nil {
			checked <- err
			return
		}
		_, err = conn.Read(packet[:1])
		if err != io.EOF {
			t.Errorf("expected immediate disconnect after TLS refusal, got %v", err)
		}
		checked <- nil
	}()
	target := "postgres://" + listener.Addr().String() + "/jobs"
	binding, err := NewSQLPasswordBinding("jobs", target, "reader", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := binding.Resolve(target, SQLCredentials{PasswordRef: "jobs"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := endpoint.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(t.Context()); err == nil {
		t.Fatal("database accepted a connection without TLS")
	}
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
}

func TestSQLPasswordBindingRejectsInsecureOrConflictingUse(t *testing.T) {
	const target = "postgres://database.example/jobs"
	for _, endpoint := range []string{target + "?sslmode=disable", "sqlite::memory:", "mysql://database.example/jobs?ssl-mode=DISABLED"} {
		if _, err := NewSQLPasswordBinding("jobs", endpoint, "reader", "fixture-password"); err == nil {
			t.Error("accepted an insecure password binding")
		}
	}
	binding, err := NewSQLPasswordBinding("jobs", target, "reader", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, credentials := range []SQLCredentials{
		{PasswordRef: "other"}, {PasswordRef: "jobs", Password: "override"}, {PasswordRef: "jobs", Username: "other"},
	} {
		if _, err := binding.Resolve(target, credentials); err == nil {
			t.Error("accepted conflicting SQL credentials")
		}
	}
	if _, err := binding.Resolve(target, SQLCredentials{PasswordRef: "jobs"}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLPasswordLookupRequiresValidBinding(t *testing.T) {
	const target = "postgres://database.example/jobs"
	calls := 0
	resolve := func(reference string) (string, error) {
		calls++
		if reference != "jobs" {
			t.Errorf("unexpected lookup reference %q", reference)
		}
		return "fixture-password", nil
	}
	for _, config := range []struct{ reference, target, username string }{
		{"", target, "reader"},
		{"jobs", target, ""},
		{"jobs", target + "?sslmode=disable", "reader"},
		{"jobs", target + "?sslmode=verify-full&sslmode=disable", "reader"},
		{"jobs", "postgres://database.example:0/jobs", "reader"},
	} {
		if _, err := ResolveSQLPasswordBinding(config.reference, config.target, config.username, resolve); err == nil {
			t.Errorf("accepted invalid binding: %+v", config)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid bindings triggered %d password lookups", calls)
	}
	if _, err := ResolveSQLPasswordBinding("jobs", target, "reader", resolve); err != nil || calls != 1 {
		t.Fatalf("valid binding: %d lookups, error %v", calls, err)
	}
}
