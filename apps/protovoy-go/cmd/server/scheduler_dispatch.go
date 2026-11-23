package main

import (
	"context"

	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

// scheduledExecutor snapshots the allowed HTTP targets and selects exactly one
// adapter after validating the complete portable request contract.
func scheduledExecutor(httpTarget, websocketTarget string, targets socketTargets) scheduler.Executor {
	http := scheduledHTTPExecutor(httpTarget, targets.AdditionalHTTP, targets.ScheduleHTTPKeychain)
	sse := scheduledSSEExecutor(httpTarget, targets.AdditionalHTTP)
	ws := scheduledWebSocketExecutor(websocketTarget)
	graphql := scheduledGraphQLExecutor(websocketTarget)
	socket := scheduledSocketExecutor(targets.TCP, targets.UDP)
	redis := scheduledRedisExecutor(targets.Redis, targets.RedisOptions, targets.RedisPasswordRef)
	grpc := scheduledGRPCExecutor(targets.GRPC)
	mqtt := scheduledMQTTExecutor(targets.MQTT)
	amqp := scheduledAMQPExecutor(targets.AMQP)
	sql := scheduledSQLExecutor(targets.SQL)
	kafka := scheduledKafkaExecutor(targets.Kafka)
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if _, err := project.CaptureHistoryRequest(request); err != nil {
			return scheduler.Outcome{}, err
		}
		execute := http
		switch {
		case request.SSE != nil:
			execute = sse
		case request.WebSocket != nil:
			execute = ws
		case request.IsGraphQLSubscription():
			execute = graphql
		case request.Socket != nil:
			execute = socket
		case request.Redis != nil:
			execute = redis
		case request.GRPC != nil:
			execute = grpc
		case request.MQTT != nil:
			execute = mqtt
		case request.AMQP != nil:
			execute = amqp
		case request.SQL != nil:
			execute = sql
		case request.Kafka != nil:
			execute = kafka
		}
		return execute(ctx, request)
	}
}
