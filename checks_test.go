package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "forwardings.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGetServersValid(t *testing.T) {
	path := writeConfig(t, `[
	  {"Host":"  router  ","Forward":[
	    {"Service":"  LuCi  ","ListenAddr":" 127.0.0.1:8080 ","RemoteAddr":"[::1]:80"}
	  ]},
	  {"Host":"user@host","Forward":[
	    {"Service":"printer","ListenAddr":"127.0.0.2:8080","RemoteAddr":"printer.lan:631"}
	  ]}
	]`)

	servers, err := getServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
	if servers[0].Host != "router" {
		t.Errorf("host not trimmed: %q", servers[0].Host)
	}
	if servers[0].Forward[0].Service != "LuCi" {
		t.Errorf("service not trimmed: %q", servers[0].Forward[0].Service)
	}
	if servers[0].Forward[0].ListenAddr != "127.0.0.1:8080" {
		t.Errorf("listen address not trimmed: %q", servers[0].Forward[0].ListenAddr)
	}
}

func TestGetServersErrors(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name:    "empty file",
			config:  `[]`,
			wantErr: "does not contain any server definitions",
		},
		{
			name:    "invalid json",
			config:  `[{`,
			wantErr: "cannot parse",
		},
		{
			name:    "option injection in host",
			config:  `[{"Host":"-oProxyCommand=evil","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "must not start with",
		},
		{
			name:    "empty host",
			config:  `[{"Host":"  ","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: `empty "Host"`,
		},
		{
			name:    "no forwardings",
			config:  `[{"Host":"router","Forward":[]}]`,
			wantErr: "has no forwardings",
		},
		{
			name:    "wildcard listen address",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":"0.0.0.0:8080","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "wildcard",
		},
		{
			name:    "wildcard ipv6 listen address",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":"[::]:8080","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "wildcard",
		},
		{
			name:    "empty host in listen address",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":":8080","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "missing host",
		},
		{
			name:    "port too large",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:65536","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "port must be",
		},
		{
			name:    "port zero",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:0","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "port must be",
		},
		{
			name:    "not host port",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":"/tmp/socket","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: "expected host:port",
		},
		{
			name:    "invalid remote address",
			config:  `[{"Host":"router","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1"}]}]`,
			wantErr: "expected host:port",
		},
		{
			name: "duplicate host",
			config: `[
			  {"Host":"router","Forward":[{"Service":"a","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"}]},
			  {"Host":"ROUTER","Forward":[{"Service":"b","ListenAddr":"127.0.0.2:8080","RemoteAddr":"127.0.0.1:80"}]}
			]`,
			wantErr: "declared multiple times",
		},
		{
			name: "duplicate service",
			config: `[{"Host":"router","Forward":[
			  {"Service":"LuCi","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"},
			  {"Service":"luci","ListenAddr":"127.0.0.2:8080","RemoteAddr":"127.0.0.1:80"}
			]}]`,
			wantErr: "declared multiple times",
		},
		{
			name: "duplicate listen address across hosts",
			config: `[
			  {"Host":"a","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"}]},
			  {"Host":"b","Forward":[{"Service":"s","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"}]}
			]`,
			wantErr: "already forwarded",
		},
		{
			name:    "missing service",
			config:  `[{"Host":"router","Forward":[{"Service":"  ","ListenAddr":"127.0.0.1:8080","RemoteAddr":"127.0.0.1:80"}]}]`,
			wantErr: `empty "Service"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := getServers(writeConfig(t, tc.config))
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestGetServersMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	_, err := getServers(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q does not name the file %q", err, path)
	}
}

func TestValidateAddr(t *testing.T) {
	tests := []struct {
		addr    string
		listen  bool
		want    string
		wantErr bool
	}{
		{"127.0.0.1:8080", true, "127.0.0.1:8080", false},
		{"LocalHost:80", true, "localhost:80", false},
		{"[::1]:80", true, "[::1]:80", false},
		{"printer.lan:631", false, "printer.lan:631", false},
		{"127.0.0.1:65536", true, "", true},
		{"127.0.0.1:0", true, "", true},
		{"127.0.0.1:http", true, "", true},
		{":8080", true, "", true},
		{"0.0.0.0:8080", true, "", true},
		{"[::]:8080", true, "", true},
		{"/tmp/socket", true, "", true},
		{"127.0.0.1:8080", false, "127.0.0.1:8080", false},
	}
	for _, tc := range tests {
		got, err := validateAddr(tc.addr, tc.listen)
		if tc.wantErr {
			if err == nil {
				t.Errorf("validateAddr(%q) = %q, want an error", tc.addr, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("validateAddr(%q): unexpected error %v", tc.addr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("validateAddr(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}
