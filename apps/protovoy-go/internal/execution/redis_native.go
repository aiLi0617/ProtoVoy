//go:build !js

package execution

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func ExecuteRedis(ctx context.Context, target string, arguments []string, timeout time.Duration) (RedisValue, error) {
	values, err := ExecuteRedisPipeline(ctx, target, [][]string{arguments}, timeout)
	if err != nil {
		return RedisValue{}, err
	}
	return values[0], nil
}

// ExecuteRedisPipeline sends commands on one connection and returns replies in command order.
func ExecuteRedisPipeline(ctx context.Context, target string, commands [][]string, timeout time.Duration) ([]RedisValue, error) {
	return ExecuteRedisWithOptions(ctx, target, commands, timeout, RedisOptions{})
}

// ExecuteRedisWithOptions completes connection setup before sending the pipeline.
func ExecuteRedisWithOptions(ctx context.Context, target string, commands [][]string, timeout time.Duration, options RedisOptions) ([]RedisValue, error) {
	return ExecuteRedisWithPasswordResolver(ctx, target, commands, timeout, options, func(string) (string, error) { return os.Getenv(RedisPasswordEnv), nil })
}

func ExecuteRedisWithPasswordResolver(ctx context.Context, target string, commands [][]string, timeout time.Duration, options RedisOptions, resolve func(string) (string, error)) ([]RedisValue, error) {
	var err error
	options, err = ResolveRedisOptions(target, options, resolve)
	if err != nil {
		return nil, err
	}
	request, err := encodeRedisCommands(commands)
	if err != nil {
		return nil, err
	}
	setup, err := redisSetup(options, redisMessageLimit-len(request))
	if err != nil {
		return nil, err
	}
	if !options.TLS && (options.CAPEM != "" || options.ServerName != "") {
		return nil, errors.New("enable Redis TLS to configure its server name or CA")
	}
	var roots *x509.CertPool
	if options.CAPEM != "" {
		roots, err = socketRoots(options.CAPEM, nil)
		if err != nil {
			return nil, err
		}
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dial := (&net.Dialer{}).DialContext
	if options.TLS {
		dial = (&tls.Dialer{Config: &tls.Config{
			MinVersion: tls.VersionTLS12, ServerName: options.ServerName, RootCAs: roots,
		}}).DialContext
	}
	connection, err := dial(ctx, "tcp", target)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Redis connection failed")
	}
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(io.LimitReader(connection, redisMessageLimit))
	budget := redisMessageLimit
	for _, command := range setup {
		_, err := io.WriteString(connection, command.wire)
		if err == nil {
			var reply RedisValue
			reply, err = readRedisReply(reader, &budget)
			if err == nil && command.accepts(reply) {
				continue
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if isRedisTimeout(err) {
			return nil, context.DeadlineExceeded
		}
		// A setup error reply may contain credential material supplied by the peer.
		return nil, errors.New(command.failure)
	}

	// Read replies while sending so neither peer has to buffer the entire exchange.
	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(connection, request)
		writeDone <- err
		if err != nil {
			connection.Close()
		}
	}()
	values := make([]RedisValue, len(commands))
	for i := range values {
		values[i], err = readRedisReply(reader, &budget)
		if err != nil {
			connection.Close()
			break
		}
	}
	writeErr := <-writeDone
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if isRedisTimeout(err) || isRedisTimeout(writeErr) {
		return nil, context.DeadlineExceeded
	}
	if err != nil {
		return nil, errors.New("invalid, incomplete, or oversized Redis response")
	}
	if writeErr != nil {
		return nil, errors.New("could not send Redis commands")
	}
	return values, nil
}

type redisSetupCommand struct {
	wire    string
	failure string
	hello   bool
}

func (command redisSetupCommand) accepts(reply RedisValue) bool {
	if !command.hello {
		return reply.Kind == "string" && reply.Text == "OK"
	}
	if reply.Kind == "map" {
		for _, entry := range reply.Entries {
			if ((entry.Key.Kind == "bulk" && string(entry.Key.Data) == "proto") ||
				(entry.Key.Kind == "string" && entry.Key.Text == "proto")) &&
				entry.Value.Kind == "integer" && entry.Value.Integer == 3 {
				return true
			}
		}
	}
	return false
}

// ValidateRedisSetup checks connection options without opening a Redis connection.
func ValidateRedisSetup(options RedisOptions) error {
	_, err := redisSetup(options, redisMessageLimit)
	return err
}

func redisSetup(options RedisOptions, remaining int) ([]redisSetupCommand, error) {
	if options.Protocol != 0 && options.Protocol != 2 && options.Protocol != 3 {
		return nil, errors.New("select Redis RESP2 or RESP3")
	}
	if options.Username != "" && options.Password == "" {
		return nil, errors.New("Redis username requires a password")
	}
	var setup []redisSetupCommand
	add := func(failure string, hello bool, args ...string) error {
		wire, err := encodeRedisCommands([][]string{args})
		if err != nil || len(wire) > remaining {
			return errors.New("Redis request exceeds 1 MiB")
		}
		remaining -= len(wire)
		setup = append(setup, redisSetupCommand{wire: wire, failure: failure, hello: hello})
		return nil
	}
	if options.Password != "" {
		auth := []string{"AUTH"}
		if options.Username != "" {
			auth = append(auth, options.Username)
		}
		if err := add("Redis authentication failed", false, append(auth, options.Password)...); err != nil {
			return nil, err
		}
	}
	if options.Protocol == 3 {
		if err := add("Redis RESP3 negotiation failed", true, "HELLO", "3"); err != nil {
			return nil, err
		}
	}
	if options.Database != "" {
		database, err := strconv.ParseUint(options.Database, 10, 64)
		if err != nil {
			return nil, errors.New("Redis database must be a nonnegative integer")
		}
		if err := add("Redis database selection failed", false, "SELECT", strconv.FormatUint(database, 10)); err != nil {
			return nil, err
		}
	}
	return setup, nil
}

func isRedisTimeout(err error) bool {
	networkError, ok := err.(net.Error)
	return ok && networkError.Timeout()
}

func encodeRedisCommands(commands [][]string) (string, error) {
	if len(commands) == 0 || len(commands) > 1024 {
		return "", errors.New("Redis requires 1 to 1024 commands")
	}
	var command strings.Builder
	for _, arguments := range commands {
		if len(arguments) == 0 || len(arguments) > 1024 {
			return "", errors.New("Redis requires 1 to 1024 command arguments")
		}
		fmt.Fprintf(&command, "*%d\r\n", len(arguments))
		for _, arg := range arguments {
			overhead := len(strconv.Itoa(len(arg))) + 5
			if len(arg) > redisMessageLimit-command.Len()-overhead {
				return "", errors.New("Redis request exceeds 1 MiB")
			}
			fmt.Fprintf(&command, "$%d\r\n%s\r\n", len(arg), arg)
		}
	}
	return command.String(), nil
}

func readRedisReply(reader *bufio.Reader, budget *int) (RedisValue, error) {
	for {
		value, err := readRedisValue(reader, 0, budget)
		if err != nil || value.Kind != "push" {
			return value, err
		}
	}
}

func readRedisHeader(reader *bufio.Reader, budget *int) (byte, string, error) {
	if *budget <= 0 {
		return 0, "", errors.New("RESP limit")
	}
	prefix, err := reader.ReadByte()
	if err != nil {
		return 0, "", err
	}
	*budget -= 1
	var line strings.Builder
	for {
		if *budget <= 0 {
			return 0, "", errors.New("RESP limit")
		}
		char, err := reader.ReadByte()
		if err != nil {
			return 0, "", err
		}
		*budget -= 1
		if char == '\r' {
			if *budget <= 0 {
				return 0, "", errors.New("RESP limit")
			}
			next, err := reader.ReadByte()
			if err != nil {
				return 0, "", err
			}
			*budget -= 1
			if next != '\n' {
				return 0, "", errors.New("RESP line")
			}
			return prefix, line.String(), nil
		}
		if char == '\n' {
			return 0, "", errors.New("RESP line")
		}
		line.WriteByte(char)
	}
}

var redisDouble = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
var redisBigNumber = regexp.MustCompile(`^[+-]?[0-9]+$`)

func readRedisValue(reader *bufio.Reader, depth int, budget *int) (RedisValue, error) {
	if depth > 64 {
		return RedisValue{}, errors.New("RESP nesting limit")
	}
	prefix, text, err := readRedisHeader(reader, budget)
	if err != nil {
		return RedisValue{}, err
	}
	switch prefix {
	case '+':
		return RedisValue{Kind: "string", Text: text}, nil
	case '-':
		return RedisValue{Kind: "error", Text: text}, nil
	case ':':
		n, err := strconv.ParseInt(text, 10, 64)
		return RedisValue{Kind: "integer", Integer: n}, err
	case '_':
		if text == "" {
			return RedisValue{Kind: "null"}, nil
		}
	case '#':
		if text == "t" || text == "f" {
			value := text == "t"
			return RedisValue{Kind: "boolean", Boolean: &value}, nil
		}
	case ',':
		if text == "inf" || text == "-inf" || text == "nan" || redisDouble.MatchString(text) {
			return RedisValue{Kind: "double", Text: text}, nil
		}
	case '(':
		if redisBigNumber.MatchString(text) {
			return RedisValue{Kind: "big-number", Text: text}, nil
		}
	case '$', '!', '=':
		return readRedisBlob(reader, budget, prefix, text)
	case '*', '~', '>', '%', '|':
		return readRedisAggregate(reader, depth, budget, prefix, text)
	}
	return RedisValue{}, errors.New("invalid or unsupported RESP value")
}

func readRedisBlob(reader *bufio.Reader, budget *int, prefix byte, length string) (RedisValue, error) {
	if prefix == '$' && length == "-1" {
		return RedisValue{Kind: "null"}, nil
	}
	if prefix == '$' && length == "?" {
		var data []byte
		for {
			marker, length, err := readRedisHeader(reader, budget)
			if err != nil {
				return RedisValue{}, err
			}
			if marker != ';' {
				return RedisValue{}, errors.New("invalid RESP chunk")
			}
			if length == "0" {
				return RedisValue{Kind: "bulk", Data: data}, nil
			}
			chunk, err := readRedisBytes(reader, budget, length)
			if err != nil {
				return RedisValue{}, err
			}
			data = append(data, chunk...)
		}
	}
	data, err := readRedisBytes(reader, budget, length)
	if err != nil {
		return RedisValue{}, err
	}
	switch prefix {
	case '!':
		return RedisValue{Kind: "bulk-error", Data: data}, nil
	case '=':
		if len(data) < 4 || data[3] != ':' {
			return RedisValue{}, errors.New("invalid RESP verbatim string")
		}
		return RedisValue{Kind: "verbatim", Format: string(data[:3]), Data: data[4:]}, nil
	default:
		return RedisValue{Kind: "bulk", Data: data}, nil
	}
}

func readRedisBytes(reader *bufio.Reader, budget *int, length string) ([]byte, error) {
	size, err := strconv.ParseUint(length, 10, 32)
	if err != nil || *budget < 2 || size > uint64(*budget-2) {
		return nil, errors.New("invalid RESP length or byte limit exceeded")
	}
	n := int(size)
	*budget -= n + 2
	data := make([]byte, n+2)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	if string(data[n:]) != "\r\n" {
		return nil, errors.New("invalid RESP terminator")
	}
	return data[:n], nil
}

func readRedisAggregate(reader *bufio.Reader, depth int, budget *int, prefix byte, length string) (RedisValue, error) {
	if prefix == '*' && length == "-1" {
		return RedisValue{Kind: "null"}, nil
	}
	const maxEntries = 10000
	streamed := length == "?" && (prefix == '*' || prefix == '~' || prefix == '%')
	var count uint64
	if !streamed {
		var err error
		count, err = strconv.ParseUint(length, 10, 32)
		if err != nil || count > maxEntries {
			return RedisValue{}, errors.New("invalid RESP aggregate length")
		}
	}
	var aggregate RedisValue
	switch prefix {
	case '*':
		aggregate.Kind = "array"
	case '~':
		aggregate.Kind = "set"
	case '>':
		aggregate.Kind = "push"
	case '%', '|':
		aggregate.Kind = "map"
	}
	for i := uint64(0); streamed || i < count; i++ {
		if streamed {
			next, err := reader.Peek(1)
			if err != nil {
				return RedisValue{}, err
			}
			if next[0] == '.' {
				_, tail, err := readRedisHeader(reader, budget)
				if err != nil {
					return RedisValue{}, err
				}
				if tail != "" {
					return RedisValue{}, errors.New("invalid RESP aggregate terminator")
				}
				break
			}
			if i == maxEntries {
				return RedisValue{}, errors.New("RESP aggregate limit")
			}
		}
		item, err := readRedisValue(reader, depth+1, budget)
		if err != nil {
			return RedisValue{}, err
		}
		if aggregate.Kind == "map" {
			value, err := readRedisValue(reader, depth+1, budget)
			if err != nil {
				return RedisValue{}, err
			}
			aggregate.Entries = append(aggregate.Entries, RedisPair{Key: item, Value: value})
		} else {
			aggregate.Items = append(aggregate.Items, item)
		}
	}
	if prefix == '|' {
		value, err := readRedisValue(reader, depth+1, budget)
		value.Attributes = append(aggregate.Entries, value.Attributes...)
		return value, err
	}
	return aggregate, nil
}
