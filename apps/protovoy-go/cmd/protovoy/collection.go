package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scripting"
	"protovoy.dev/protovoy/internal/secrets"
)

func runCollectionCommand(args []string) {
	flags := flag.NewFlagSet("protovoy run", flag.ExitOnError)
	failFast := flags.Bool("fail-fast", false, "stop after the first failed request")
	data := flags.String("data", "", "JSON or CSV file with one variable row per iteration")
	concurrency := flags.Int("concurrency", 1, "number of data rows to run concurrently (1 to 16)")
	junit := flags.String("junit", "", "write a JUnit XML report to a new file")
	streamTimeout := flags.Duration("stream-timeout", 30*time.Second, "deadline per streaming request")
	preScript := flags.String("pre-script", "", "JavaScript file to run before each HTTP request")
	postScript := flags.String("post-script", "", "JavaScript file to run after each HTTP response")
	responsePlugin := flags.String("response-plugin", "", "trusted transformer package directory for HTTP response bodies")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: protovoy run [options] PROJECT.json")
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)
	if flags.NArg() != 1 || *streamTimeout <= 0 || *concurrency < 1 || *concurrency > 16 {
		flags.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	options := collectionRunOptions{failFast: *failFast, streamTimeout: *streamTimeout, concurrency: *concurrency, junitPath: *junit}
	var err error
	if *data != "" {
		options.rows, err = readCollectionData(*data)
	}
	if err == nil && *preScript != "" {
		var source string
		source, err = readHTTPScript(*preScript)
		options.preScripts = []string{source}
	}
	if err == nil && *postScript != "" {
		var source string
		source, err = readHTTPScript(*postScript)
		options.postScripts = []string{source}
	}
	if err == nil && *responsePlugin != "" {
		options.transformResponse, err = prepareResponsePlugin(*responsePlugin, os.Getenv("PROTOVOY_PLUGIN_TRUSTED_KEYS"))
	}
	if err == nil {
		err = runCollectionFile(ctx, flags.Arg(0), options, os.Stdout)
	}
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type collectionRunOptions struct {
	failFast          bool
	streamTimeout     time.Duration
	rows              []map[string]string
	concurrency       int
	junitPath         string
	preScripts        []string
	postScripts       []string
	transformResponse responseTransformer
}

func runCollectionFile(ctx context.Context, path string, options collectionRunOptions, output io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New("could not open the collection file")
	}
	defer file.Close()
	value, err := project.Read(file)
	if err != nil {
		return err
	}
	return runCollection(ctx, value, options, output)
}

type collectionResult struct {
	Name         string  `json:"name"`
	Protocol     string  `json:"protocol"`
	Status       int     `json:"status,omitempty"`
	Passed       bool    `json:"passed"`
	Error        string  `json:"error,omitempty"`
	Iteration    int     `json:"iteration,omitempty"`
	RequestIndex int     `json:"requestIndex"`
	DurationMS   float64 `json:"durationMs"`
}

func runCollection(ctx context.Context, value project.Project, options collectionRunOptions, output io.Writer) error {
	if options.concurrency < 0 || options.concurrency > 16 {
		return errors.New("collection concurrency must be from 1 to 16")
	}
	if options.concurrency > 1 && (options.failFast || options.rows == nil) {
		return errors.New("concurrent collection runs require -data and cannot use -fail-fast")
	}
	if len(value.Requests) == 0 {
		return errors.New("collection has no requests")
	}
	for i, request := range value.Requests {
		if options.transformResponse != nil && (collectionProtocol(request) != "http" ||
			request.HasScripts() || request.HTTPSecretReference != "" || len(options.preScripts) != 0 || len(options.postScripts) != 0) {
			return fmt.Errorf("collection request %d: -response-plugin requires HTTP without request scripts or credential bindings", i+1)
		}
		if err := request.ValidateScripts(); err != nil {
			return fmt.Errorf("collection request %d: %w", i+1, err)
		}
		if collectionProtocol(request) == "" {
			return fmt.Errorf("collection request %d uses a protocol unsupported by this runner", i+1)
		}
		if collectionProtocol(request) == "http" && request.HTTPSecretReference != "" &&
			(len(options.preScripts) != 0 || len(options.postScripts) != 0) {
			return fmt.Errorf("collection request %d uses an HTTP credential binding with scripts", i+1)
		}
	}
	encoder := json.NewEncoder(output)
	emit := func(item collectionResult) error { return encoder.Encode(item) }
	var junit *junitReport
	if options.junitPath != "" {
		var err error
		junit, err = openJUnitReport(options.junitPath)
		if err != nil {
			return err
		}
		defer junit.file.Close()
		emit = func(item collectionResult) error {
			if err := encoder.Encode(item); err != nil {
				return err
			}
			return junit.record(item)
		}
	}
	var failed int
	var err error
	if options.concurrency > 1 {
		failed, err = runConcurrentIterations(ctx, value, options, emit)
	} else {
		failed, err = runSequentialIterations(ctx, value, options, emit)
	}
	if junit != nil {
		skipped := 0
		if err == nil && options.failFast && failed != 0 {
			skipped = len(value.Requests)*max(1, len(options.rows)) - junit.recorded
		}
		err = errors.Join(err, junit.finish(err != nil, skipped))
	}
	if err != nil {
		return err
	}
	if failed != 0 {
		return fmt.Errorf("%d collection request(s) failed", failed)
	}
	return nil
}

func runSequentialIterations(ctx context.Context, value project.Project, options collectionRunOptions, emit func(collectionResult) error) (int, error) {
	rows := options.rows
	if rows == nil {
		rows = []map[string]string{nil}
	}
	failed := 0
	for index, row := range rows {
		iteration := 0
		variables := value.CollectionVariables
		if options.rows != nil {
			iteration = index + 1
			variables = iterationVariables(row, value.CollectionVariables)
		}
		count, err := runCollectionIteration(ctx, value.Requests, variables, iteration, options, emit)
		if err != nil {
			return failed, err
		}
		failed += count
		if options.failFast && failed != 0 {
			break
		}
	}
	return failed, nil
}

func iterationVariables(row, collection map[string]string) map[string]string {
	variables := make(map[string]string, len(row)+len(collection))
	maps.Copy(variables, row)
	maps.Copy(variables, collection)
	return variables
}

func runCollectionIteration(ctx context.Context, requests []project.Request, variables map[string]string,
	iteration int, options collectionRunOptions, emit func(collectionResult) error,
) (int, error) {
	client := execution.HTTPClient{}
	failed := 0
	for index, request := range requests {
		if ctx.Err() != nil {
			return failed, ctx.Err()
		}
		item := collectionResult{Name: request.Name, Protocol: collectionProtocol(request), Iteration: iteration, RequestIndex: index + 1}
		started := time.Now()
		var err error
		switch item.Protocol {
		case "sql":
			err = executeCollectionSQL(ctx, request, variables)
		case "redis":
			err = executeCollectionRedis(ctx, request, variables)
		case "grpc":
			item.Status, err = executeCollectionGRPC(ctx, request, variables)
		case "tcp", "udp":
			err = executeCollectionSocket(ctx, request, variables)
		case "mqtt":
			err = executeCollectionMQTT(ctx, request, variables)
		case "amqp":
			err = executeCollectionAMQP(ctx, request, variables)
		case "kafka":
			err = executeCollectionKafka(ctx, request, variables)
		case "sse":
			item.Status, err = executeCollectionSSE(ctx, request, variables, options.streamTimeout)
		case "graphql-subscription":
			err = executeCollectionSubscription(ctx, request, variables, options.streamTimeout)
		case "websocket":
			err = executeCollectionWebSocket(ctx, request, variables, options.streamTimeout)
		default:
			var input execution.HTTPRequest
			input, err = request.PrepareCollectionHTTP(variables)
			if err == nil && request.HTTPSecretReference != "" {
				input.Headers, err = secrets.HTTPHeadersContext(ctx, input.Headers, input.Target, request.HTTPSecretReference, request.HTTPSecretHeader, input.FollowRedirects)
			}
			if err == nil {
				expectedBody := request.ResponseBodyContains
				if expectedBody != "" {
					expectedBody, err = execution.ResolveTextVariables(expectedBody, variables, request.Variables)
					switch {
					case err != nil:
						err = fmt.Errorf("could not resolve response body assertion: %w", err)
					case expectedBody == "" || len(expectedBody) > project.MaxResponseBodyAssertionBytes:
						err = fmt.Errorf("resolved response body assertion must contain 1 to %d bytes", project.MaxResponseBodyAssertionBytes)
					}
				}
				if err == nil {
					if options.transformResponse != nil {
						item.Status, err = transformHTTPResponse(ctx, client, input, options.transformResponse, nil, func(body string) error {
							if !strings.Contains(body, expectedBody) {
								return errors.New("transformed response body does not contain the expected text")
							}
							return nil
						})
					} else if item.Protocol == "http" && (request.HasScripts() || len(options.preScripts) != 0 || len(options.postScripts) != 0) {
						item.Status, err = collectScriptedHTTP(ctx, client, input, expectedBody,
							iterationVariables(variables, request.Variables),
							slices.Concat(options.preScripts, request.PreScripts), slices.Concat(request.PostScripts, options.postScripts))
					} else {
						item.Status, err = collectHTTPStatus(ctx, client, input, item.Protocol, expectedBody)
					}
				}
			}
		}
		if ctx.Err() != nil {
			return failed, ctx.Err()
		}
		statusPassed := item.Status == 0 || item.Status >= 200 && item.Status < 300
		if err == nil {
			if expected := request.ExpectedHTTPStatus; expected != 0 && item.Status != expected {
				err = fmt.Errorf("expected HTTP status %d, received %d", expected, item.Status)
			} else if expected == 0 && !statusPassed {
				err = fmt.Errorf("unexpected response status %d", item.Status)
			}
		}
		item.Passed = err == nil
		if err != nil {
			item.Error = err.Error()
		}
		item.DurationMS = float64(time.Since(started)) / float64(time.Millisecond)
		if err := emit(item); err != nil {
			return failed, err
		}
		if !item.Passed {
			failed++
			if options.failFast {
				break
			}
		}
	}
	return failed, nil
}

func collectScriptedHTTP(ctx context.Context, client execution.HTTPClient, input execution.HTTPRequest,
	expectedBody string, variables map[string]string, preScripts, postScripts []string,
) (int, error) {
	input, data, variables, err := scripting.PrepareHTTP(ctx, input, preScripts, variables)
	if err != nil {
		return 0, err
	}
	if len(postScripts) == 0 {
		return collectHTTPStatus(ctx, client, input, "http", expectedBody)
	}
	status, body, err := scripting.ExecuteHTTPPost(ctx, client, input, postScripts, data, variables)
	if err == nil && expectedBody != "" && !bytes.Contains(body, []byte(expectedBody)) {
		err = errors.New("response body does not contain the expected text")
	}
	return status, err
}

func collectionProtocol(request project.Request) string {
	switch {
	case request.SQL != nil:
		return "sql"
	case request.Redis != nil:
		return "redis"
	case request.GRPC != nil:
		return "grpc"
	case request.Socket != nil:
		return request.Socket.Network
	case request.MQTT != nil:
		return "mqtt"
	case request.AMQP != nil:
		return "amqp"
	case request.Kafka != nil:
		return "kafka"
	case request.SSE != nil:
		return "sse"
	case request.IsGraphQLSubscription():
		return "graphql-subscription"
	case request.WebSocket != nil:
		return "websocket"
	default:
		return request.CollectionHTTPProtocol()
	}
}

func collectHTTPStatus(ctx context.Context, client execution.HTTPClient, input execution.HTTPRequest, protocol, bodyContains string) (int, error) {
	return execution.CollectHTTPStatus(ctx, client, input, protocol, bodyContains)
}
func streamContains(reader io.Reader, needle []byte) (bool, error) {
	return execution.StreamContains(reader, needle)
}
func checkCollectionGraphQLResult(body []byte) error {
	return execution.CheckCollectionGraphQLResult(body)
}
