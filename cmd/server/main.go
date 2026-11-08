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

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return errors.New("ANTHROPIC_API_KEY is required")
	}
	batchSize := 0 // ingest.DefaultBatchSize
	if v := os.Getenv("BATCH_SIZE"); v != "" {
		if batchSize, err = strconv.Atoi(v); err != nil || batchSize <= 0 {
			return fmt.Errorf("invalid BATCH_SIZE %q", v)
		}
	}

	categoryProvider, err := categories.NewYamlCategoryProvider(categoriesFile)
	if err != nil {
		return err
	}
	cats, err := categoryProvider.List(ctx)
	if err != nil {
		return err
	}
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
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

	imp := ingest.NewImporter(parser.NewParser(mappings), categorizer.NewClaude(apiKey, names, logger), store, batchSize, logger)

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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
