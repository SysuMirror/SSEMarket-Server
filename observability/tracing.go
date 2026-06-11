package observability

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// InitTracing 初始化 OpenTelemetry TracerProvider，导出至 Jaeger（OTLP gRPC）。
func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	if !viper.GetBool("observability.tracing.enabled") {
		log.Println("[observability] tracing disabled")
		return func(context.Context) error { return nil }, nil
	}

	endpoint := viper.GetString("observability.tracing.jaeger_endpoint")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	serviceName := viper.GetString("observability.tracing.service_name")
	if serviceName == "" {
		serviceName = "sse_market_server"
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Printf("[observability] tracing enabled, endpoint=%s service=%s", endpoint, serviceName)
	return tp.Shutdown, nil
}
