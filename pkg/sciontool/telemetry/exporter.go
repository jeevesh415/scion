/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"golang.org/x/oauth2/google"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
)

// loadGCPDialOptions loads GCP credentials from a service account key file
// and returns gRPC dial options for per-RPC authentication. Returns (nil, nil)
// if credFile is empty. The credentials are scoped for Cloud Trace, Logging,
// and Monitoring write access.
func loadGCPDialOptions(ctx context.Context, credFile string) ([]grpc.DialOption, error) {
	if credFile == "" {
		return nil, nil
	}
	keyBytes, err := os.ReadFile(credFile)
	if err != nil {
		return nil, fmt.Errorf("reading GCP credentials file: %w", err)
	}
	creds, err := google.CredentialsFromJSON(ctx, keyBytes,
		"https://www.googleapis.com/auth/trace.append",
		"https://www.googleapis.com/auth/logging.write",
		"https://www.googleapis.com/auth/monitoring.write",
	)
	if err != nil {
		return nil, fmt.Errorf("parsing GCP credentials: %w", err)
	}
	perRPC := oauth.TokenSource{TokenSource: creds.TokenSource}
	return []grpc.DialOption{grpc.WithPerRPCCredentials(perRPC)}, nil
}

// CloudExporter exports traces and metrics to a cloud OTLP endpoint.
type CloudExporter struct {
	traceExporter trace.SpanExporter
	grpcClient    coltracepb.TraceServiceClient
	metricClient  colmetricpb.MetricsServiceClient
	logClient     collogspb.LogsServiceClient
	grpcConn      *grpc.ClientConn
	protocol      string
	endpoint      string
}

// NewCloudExporter creates a new cloud trace exporter.
// Returns nil if cloud export is not configured.
func NewCloudExporter(config *Config) (*CloudExporter, error) {
	if !config.IsCloudConfigured() {
		return nil, nil
	}

	exporter := &CloudExporter{
		protocol: config.Protocol,
		endpoint: config.Endpoint,
	}

	if config.CloudProvider == "gcp" && config.Protocol == "http" {
		log.Println("[telemetry] WARNING: GCP OTLP export uses gRPC; HTTP protocol may not work with GCP credentials")
	}

	var err error
	switch config.Protocol {
	case "grpc":
		err = exporter.initGRPC(config)
	case "http":
		err = exporter.initHTTP(config)
	default:
		err = exporter.initGRPC(config) // default to gRPC
	}

	if err != nil {
		return nil, err
	}

	return exporter, nil
}

// initGRPC initializes the gRPC exporter.
func (e *CloudExporter) initGRPC(config *Config) error {
	ctx := context.Background()

	// Load GCP dial options if credentials are configured and TLS is enabled
	var gcpDialOpts []grpc.DialOption
	if config.GCPCredentialsFile != "" {
		if config.Insecure {
			log.Println("[telemetry] WARNING: GCP credentials require TLS; skipping credential injection with insecure mode")
		} else {
			var err error
			gcpDialOpts, err = loadGCPDialOptions(ctx, config.GCPCredentialsFile)
			if err != nil {
				return fmt.Errorf("failed to load GCP credentials: %w", err)
			}
		}
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.Endpoint),
	}

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	for _, do := range gcpDialOpts {
		opts = append(opts, otlptracegrpc.WithDialOption(do))
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create gRPC trace exporter: %w", err)
	}

	e.traceExporter = exporter

	// Also create a raw gRPC client for proto forwarding
	connOpts := []grpc.DialOption{}
	if config.Insecure {
		connOpts = append(connOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		connOpts = append(connOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}
	connOpts = append(connOpts, gcpDialOpts...)

	conn, err := grpc.NewClient(config.Endpoint, connOpts...)
	if err != nil {
		// Continue without raw client - we can still use SDK exporter
		return nil
	}

	e.grpcConn = conn
	e.grpcClient = coltracepb.NewTraceServiceClient(conn)
	e.metricClient = colmetricpb.NewMetricsServiceClient(conn)
	e.logClient = collogspb.NewLogsServiceClient(conn)

	return nil
}

// initHTTP initializes the HTTP exporter.
func (e *CloudExporter) initHTTP(config *Config) error {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.Endpoint),
	}

	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		return fmt.Errorf("failed to create HTTP trace exporter: %w", err)
	}

	e.traceExporter = exporter
	return nil
}

// ExportSpans exports a batch of SDK spans to the cloud endpoint.
func (e *CloudExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	if e == nil || e.traceExporter == nil {
		return nil
	}
	return e.traceExporter.ExportSpans(ctx, spans)
}

// ExportProtoSpans exports raw proto spans to the cloud endpoint.
// This is used for forwarding OTLP data received from agents.
func (e *CloudExporter) ExportProtoSpans(ctx context.Context, resourceSpans []*tracepb.ResourceSpans) error {
	if e == nil {
		return nil
	}

	// Use gRPC client if available
	if e.grpcClient != nil {
		req := &coltracepb.ExportTraceServiceRequest{
			ResourceSpans: resourceSpans,
		}
		_, err := e.grpcClient.Export(ctx, req)
		return err
	}

	// Otherwise we can't forward raw proto data
	// This is acceptable for M1 - cloud export may not work without proper setup
	return nil
}

// ExportProtoMetrics exports raw proto metrics to the cloud endpoint.
// This is used for forwarding OTLP data received from agents.
func (e *CloudExporter) ExportProtoMetrics(ctx context.Context, resourceMetrics []*metricpb.ResourceMetrics) error {
	if e == nil {
		return nil
	}

	if e.metricClient != nil {
		req := &colmetricpb.ExportMetricsServiceRequest{
			ResourceMetrics: resourceMetrics,
		}
		_, err := e.metricClient.Export(ctx, req)
		return err
	}

	return nil
}

// ExportProtoLogs exports raw proto logs to the cloud endpoint.
// This is used for forwarding OTLP data received from agents.
func (e *CloudExporter) ExportProtoLogs(ctx context.Context, resourceLogs []*logspb.ResourceLogs) error {
	if e == nil {
		return nil
	}

	if e.logClient != nil {
		req := &collogspb.ExportLogsServiceRequest{
			ResourceLogs: resourceLogs,
		}
		_, err := e.logClient.Export(ctx, req)
		return err
	}

	return nil
}

// Shutdown gracefully shuts down the exporter.
func (e *CloudExporter) Shutdown(ctx context.Context) error {
	if e == nil {
		return nil
	}

	var errs []error

	if e.traceExporter != nil {
		if err := e.traceExporter.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if e.grpcConn != nil {
		if err := e.grpcConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// SpanExporter returns the underlying trace.SpanExporter.
// This is useful for registering with a TracerProvider.
func (e *CloudExporter) SpanExporter() trace.SpanExporter {
	if e == nil {
		return nil
	}
	return e.traceExporter
}
