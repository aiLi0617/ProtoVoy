package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const webUserEnv = "PROTOVOY_WEB_USER"
const webPasswordEnv = "PROTOVOY_WEB_PASSWORD"

func loadWebPassword(password, path string) (string, error) {
	if path == "" {
		return password, nil
	}
	if password != "" {
		return "", errors.New("set only one of " + webPasswordEnv + " and -web-password-file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("could not open Web password file")
	}
	defer file.Close()
	const limit = 4096
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return "", errors.New("could not read Web password file within 4 KiB")
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
}

type webAccess struct {
	host, origin string
	auth         *webAuth
}

type webAuth struct {
	user, password [sha256.Size]byte
}

func configureWebAccess(addr, publicOrigin, certFile, keyFile, user, password string) (webAccess, error) {
	host, port, err := net.SplitHostPort(addr)
	ip, ipErr := netip.ParseAddr(host)
	if err != nil || ipErr != nil || ip.Zone() != "" {
		return webAccess{}, errors.New("set -listen to an IPv4 or IPv6 address")
	}
	if publicOrigin == "" {
		if !ip.IsLoopback() || certFile != "" || keyFile != "" {
			return webAccess{}, errors.New("remote listening and TLS require -public-origin")
		}
		return loopbackAccess(net.JoinHostPort(ip.String(), port)), nil
	}
	publicHost, err := httpsOriginHost(publicOrigin)
	if err != nil {
		return webAccess{}, err
	}
	if certFile == "" || keyFile == "" {
		return webAccess{}, errors.New("hosted mode requires -tls-cert and -tls-key")
	}
	if !printableCredential(user) || strings.Contains(user, ":") ||
		!printableCredential(password) || len(password) < 24 {
		return webAccess{}, errors.New("hosted mode requires " + webUserEnv + " and a password of at least 24 printable characters in " + webPasswordEnv + " or -web-password-file")
	}
	return webAccess{
		host: publicHost, origin: "https://" + publicHost,
		auth: &webAuth{user: sha256.Sum256([]byte(user)), password: sha256.Sum256([]byte(password))},
	}, nil
}

func loopbackAccess(addr string) webAccess {
	return webAccess{host: addr, origin: "http://" + addr}
}

func printableCredential(value string) bool {
	return value != "" && !strings.ContainsFunc(value, func(c rune) bool { return c < 33 || c > 126 })
}

func sameOrigin(r *http.Request, origin string) bool {
	values := r.Header.Values("Origin")
	return len(values) == 1 && values[0] == origin
}

func httpsOriginHost(input string) (string, error) {
	invalid := errors.New("set -public-origin to an HTTPS origin such as https://workbench.example.com")
	origin, err := url.Parse(input)
	if err != nil || origin.Scheme != "https" || origin.User != nil || origin.Hostname() == "" ||
		(origin.EscapedPath() != "" && origin.EscapedPath() != "/") || origin.RawQuery != "" ||
		origin.ForceQuery || strings.Contains(input, "#") || strings.HasSuffix(origin.Host, ":") {
		return "", invalid
	}
	host := strings.ToLower(origin.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" {
			return "", invalid
		}
		host = ip.String()
	} else if strings.ContainsFunc(host, func(c rune) bool {
		return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.')
	}) {
		return "", invalid
	}
	if port := origin.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return "", invalid
		}
		if number != 443 {
			return net.JoinHostPort(host, strconv.FormatUint(number, 10)), nil
		}
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]", nil
	}
	return host, nil
}

func (access webAccess) authorize(w http.ResponseWriter, r *http.Request) bool {
	if r.Host != access.host {
		http.Error(w, "invalid host", http.StatusForbidden)
		return false
	}
	if access.auth == nil {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	if r.TLS == nil {
		http.Error(w, "HTTPS required", http.StatusForbidden)
		return false
	}
	user, password, ok := r.BasicAuth()
	userHash, passwordHash := sha256.Sum256([]byte(user)), sha256.Sum256([]byte(password))
	userOK := subtle.ConstantTimeCompare(userHash[:], access.auth.user[:])
	passwordOK := subtle.ConstantTimeCompare(passwordHash[:], access.auth.password[:])
	if !ok || userOK&passwordOK != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="ProtoVoy"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return false
	}
	return true
}
