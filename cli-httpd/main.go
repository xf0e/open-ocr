// main package for the Open-OCR HTTP daemon.
// This package provides the main entry point for the HTTP server that handles OCR requests.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	ocrworker "github.com/xf0e/open-ocr"
)

var (
	sha1ver   string
	buildTime string
	version   string
)

// config holds the application's configuration.
type config struct {
	httpPort uint
	debug    bool
	useHttps bool
	keyFile  string
	certFile string
	rabbit   ocrworker.RabbitConfig
}

func init() {
	zerolog.TimeFieldFormat = time.StampMilli
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

// handleIndex serves the landing page.
func handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	// The 'ocrworker.AppStop' variable is used to check if the application is in the process of shutting down.
	// 'ocrworker.TechnicalErrorResManager' indicates if there are technical errors with the resource manager.
	text := ocrworker.GenerateLandingPage(ocrworker.AppStop, ocrworker.TechnicalErrorResManager, version)
	_, _ = fmt.Fprint(w, text)
}

// createHTTPServer creates and configures an HTTP server.
func createHTTPServer(cfg *config, ocrChain http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.Handle("/ocr", ocrChain)
	mux.Handle("/ocr-file-upload", ocrworker.NewOcrHttpMultipartHandler(&cfg.rabbit))
	mux.Handle("/ocr-status", ocrworker.NewOcrHttpStatusHandler())
	mux.Handle("/metrics", promhttp.Handler())

	// Add pprof endpoints for debugging performance issues.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.httpPort),
		ReadTimeout:       60 * time.Second,
		ReadHeaderTimeout: 60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler:           mux,
	}
}

// waitForShutdown handles graceful shutdown of the server.
func waitForShutdown(srv *http.Server) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	ssig := <-signals

	log.Info().Str("component", "OCR_HTTP").Str("signal", sig.String()).
		Msg("Caught signal to terminate, shutting down gracefully.")

	// Create a context with a timeout to allow for graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Signal other parts of the application to stop.
	ocrworker.StopChan <- true

	// Wait for in-flight requests to complete.
	for {
		if atomic.LoadUint32(&ocrworker.RequestTrackLength) == 0 {
			log.Info().Str("component", "OCR_HTTP").
				Msg("OCR queue is now empty. open-ocr http daemon will now exit.")
			break
		}
		log.Info().Str("component", "OCR_HTTP").Uint32("Length of Requests", atomic.LoadUint32(&ocrworker.RequestTrackLength)).
			Msg("In-flight requests queue is not empty. Waiting for requests to be processed.")
		time.Sleep(10 * time.Second)
	}

	// Shutdown the server.
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Error during server shutdown")
	}
}

func main() {
	var cfg config
	var flgVersion bool

	// Define and parse command-line flags.
	flag.UintVar(&cfg.httpPort, "http_port", 8080, "The http port to listen on")
	flag.BoolVar(&cfg.debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&flgVersion, "version", false, "Show version and exit")
	flag.BoolVar(&cfg.useHttps, "usehttps", false, "Use HTTPS")
	flag.StringVar(&cfg.keyFile, "keyfile", "", "Path to private key for HTTPS")
	flag.StringVar(&cfg.certFile, "certfile", "", "Path to certificate file for HTTPS")

	// Override default RabbitMQ config with command-line flags.
	cfg.rabbit = ocrworker.DefaultConfigFlagsOverride(flag.CommandLine.Add)

	flag.Parse()

	if flgVersion {
		fmt.Printf("version %s. Build on %s from git commit hash %s\n", version, buildTime, sha1ver)
		return
	}

	if cfg.debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// Log the configuration without sensitive information.
	logConfig := cfg
	urlTmp, _ := url.Parse(logConfig.rabbit.AmqpAPIURI)
	logConfig.rabbit.AmqpAPIURI = ocrworker.StripPasswordFromUrl(urlTmp)
	urlTmp, _ = url.Parse(logConfig.rabbit.AmqpURI)
	logConfig.rabbit.AmqpURI = ocrworker.StripPasswordFromUrl(urlTmp)
	log.Info().Interface("parameters", logConfig).Msg("starting with parameters")

	// Create the OCR processing chain.
	ocrcChain := ocrworker.InstrumentHttpStatusHandler(ocrworker.NewOcrHttpHandler(&cfg.rabbit))

	// Start the resource manager in a separate goroutine.
	go ocrworker.SetResManagerState(&cfg.rabbit)

	// Create and start the HTTP server.
	srv := createHTTPServer(&cfg, ocrChain)

	log.Info().Str("component", "OCR_HTTP").Str("listenAddr", srv.Addr).Msg("Starting listener...")

	go func() {
		var err error
		if cfg.useHttps {
			if cfg.certFile == "" || cfg.keyFile == "" {
				log.Fatal().Msg("HTTPS requires both a key and a certificate file.")
			}
			// Configure TLS settings for security.
			srv.TLSConfig = &tls.Config{
				MinVersion:       tls.VersionTLS12,
				CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
				CipherSuites: []uint16{
					ls.TLS_AES_256_GCM_SHA384,
					ls.TLS_CHACHA20_POLY1305_SHA256,
					ls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
					ls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					ls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				},
			}
			err = srv.ListenAndServeTLS(cfg.certFile, cfg.keyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Str("component", "CLI_HTTP").Msg("Server failed to start")
		}
	}()

	// Wait for a shutdown signal.
	waitForShutdown(srv)
}