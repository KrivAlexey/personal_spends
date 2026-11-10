// Command server runs the REST API as a plain HTTP server on localhost:8080,
// for local development. Run it from the repository root: the category and
// bank-mapping files are read from their paths in the repo.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/KrivAlexey/personal_spends/internal/categories"
	"github.com/KrivAlexey/personal_spends/internal/categorizer"
	"github.com/KrivAlexey/personal_spends/internal/handler"
	"github.com/KrivAlexey/personal_spends/internal/ingest"
	"github.com/KrivAlexey/personal_spends/internal/logging"
	"github.com/KrivAlexey/personal_spends/internal/parser"
	"github.com/KrivAlexey/personal_spends/internal/storage"
)

const (
	addr           = "localhost:8080"
	categoriesFile = "internal/categories/categories.yaml"
	mappingsFile   = "internal/parser/bank_mappings.yaml"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	logger, err := logging.New(os.Stderr, os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"))
	if err != nil {
		return err
	}

	batchSize := 0 // ingest.DefaultBatchSize
	if v := os.Getenv("BATCH_SIZE"); v != "" {
		if batchSize, err = strconv.Atoi(v); err != nil || batchSize <= 0 {
			return fmt.Errorf("invalid BATCH_SIZE %q", v)
		}
	}

	cat, err := newCategorizer(ctx, logger)
	if err != nil {
		return err
	}

	mappings, err := parser.NewFileMappingProvider(ctx, mappingsFile)
	if err != nil {
		return err
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(envOr("AWS_REGION", "eu-central-1")))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	store := storage.NewStore(dynamodb.NewFromConfig(awsCfg), envOr("DYNAMODB_TABLE", "personal-spends-expenses"), logger)

	imp := ingest.NewImporter(parser.NewParser(mappings), cat, store, batchSize, logger)

	// No WriteTimeout: an upload waits on Claude for every batch.
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler.New(imp, logger).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", addr))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	return nil
}

// fakeRules match the merchants in samples/, for CATEGORIZER=fake.
var fakeRules = []categorizer.KeywordRule{
	{Keyword: "rewe", Category: "groceries"},
	{Keyword: "edeka", Category: "groceries"},
	{Keyword: "netflix", Category: "streaming_subscriptions"},
	{Keyword: "tankstelle", Category: "fuel"},
	{Keyword: "deutsche bahn", Category: "transportation_public"},
	{Keyword: "miete", Category: "housing_rent"},
}

// newCategorizer returns the Claude categorizer, or with CATEGORIZER=fake a
// Keyword one that calls no model and needs no API key.
func newCategorizer(ctx context.Context, logger *slog.Logger) (categorizer.Categorizer, error) {
	switch v := os.Getenv("CATEGORIZER"); v {
	case "fake":
		return categorizer.NewKeyword(fakeRules, logger), nil
	case "", "claude":
	default:
		return nil, fmt.Errorf("invalid CATEGORIZER %q", v)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required")
	}
	categoryProvider, err := categories.NewYamlCategoryProvider(categoriesFile)
	if err != nil {
		return nil, err
	}
	cats, err := categoryProvider.List(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	return categorizer.NewClaude(apiKey, names, logger), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
