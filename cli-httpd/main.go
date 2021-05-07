package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	ocrworker "github.com/xf0e/open-ocr"
)

// This assumes that there is a worker running
// To test it:
// curl -X POST -H "Content-Type: application/json" -d '{"img_url":"http://localhost:8081/img","engine":0}' http://localhost:8081/ocr

var (
	sha1ver      string
	buildTime    string
	version      string
	appStopLocal = false
)

func init() {
	zerolog.TimeFieldFormat = time.StampMilli
	// Default level is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func handleIndex(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	appStopLocal = ocrworker.AppStop
	text := ocrworker.GenerateLandingPage(appStopLocal, ocrworker.TechnicalErrorResManager)
	_, _ = fmt.Fprint(writer, text)
}

func makeServerFromMux(mux *http.ServeMux) *http.Server {
	return &http.Server{
		ReadTimeout:       60 * time.Second,
		ReadHeaderTimeout: 60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler:           mux,
	}
}

func makeHTTPServer(rabbitConfig *ocrworker.RabbitConfig, ocrChain http.Handler) *http.Server {
	mux := &http.ServeMux{}
	mux.HandleFunc("/", handleIndex)
	mux.Handle("/ocr", ocrChain)
	mux.Handle("/ocr-file-upload", ocrworker.NewOcrHttpMultipartHandler(rabbitConfig))
	// api end point for getting orc request status
	mux.Handle("/ocr-status", ocrworker.NewOcrHttpStatusHandler())
	// expose metrics for prometheus
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return makeServerFromMux(mux)

}

func main() {
	// defer profile.Start(profile.MemProfile).Stop()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGINT, syscall.SIGQUIT)

	/*	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal()
	}*/

	go func() {
		for sig := range signals {
			log.Info().Str("component", "OCR_HTTP").Str("signal", sig.String()).
				Msg("Caught signal to terminate, will not serve any further requests. Once the ocr queue is empty," +
					" http daemon will terminate.")

			ocrworker.StopChan <- true
			for {
				log.Info().Str("component", "OCR_HTTP").Uint32("Length of Requests", ocrworker.RequestTrackLength)
				// prepare "requests" map for displaying in flight requests upon service shutdown

				ocrworker.RequestsTrack.Range(func(key, value interface{}) bool {
					log.Info().Str("component", "OCR_HTTP").Msg("Inflight request " + fmt.Sprint(key))
					return true
				})

				// as soon number of queued requests reaches zero, http daemon will exit
				if ocrworker.RequestTrackLength == 0 {
					log.Info().Str("component", "OCR_HTTP").Str("signal", sig.String()).
						Msg("ocr queue is now empty. open-ocr http daemon will now exit. You may stop workers now")
					time.Sleep(20 * time.Second) // delay puffer for sending all requests back
					break
				}
				time.Sleep(1 * time.Second)
			}
			os.Exit(0)
		}
	}()

	var httpPort uint
	var debug bool
	var flgVersion bool
	var useHttps bool
	var keyFile string
	var certFile string
	flagFunc := func() {
		flag.UintVar(
			&httpPort,
			"http_port",
			8080,
			"The http port to listen on, eg, 8081",
		)
		flag.BoolVar(
			&debug,
			"debug",
			false,
			"sets debug flag, program will print more messages",
		)
		flag.BoolVar(
			&flgVersion,
			"version",
			false,
			"show version and exit",
		)
		flag.BoolVar(
			&useHttps,
			"usehttps",
			false,
			"set to use secure connection",
		)
		flag.StringVar(
			&keyFile,
			"keyfile",
			"",
			"path to private key",
		)
		flag.StringVar(
			&certFile,
			"certfile",
			"",
			"path to certificate file",
		)
	}

	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(flagFunc)
	if flgVersion {
		fmt.Printf("version %s. Build on %s from git commit hash %s\n", version, buildTime, sha1ver)
		os.Exit(0)
	}
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	rabbitConfigTemp := rabbitConfig
	urlTmp, _ := url.Parse(rabbitConfigTemp.AmqpAPIURI)
	rabbitConfigTemp.AmqpAPIURI = ocrworker.StripPasswordFromUrl(urlTmp)
	urlTmp, _ = url.Parse(rabbitConfigTemp.AmqpURI)
	rabbitConfigTemp.AmqpURI = ocrworker.StripPasswordFromUrl(urlTmp)
	log.Info().Interface("parameters", rabbitConfigTemp).Msg("trying to start with parameters")

	ocrChain := ocrworker.InstrumentHttpStatusHandler(ocrworker.NewOcrHttpHandler(&rabbitConfig))
	listenAddr := fmt.Sprintf(":%d", httpPort)

	// start a goroutine which will run forever and decide if we have resources for incoming requests
	go func() {
		ocrworker.SetResManagerState(&rabbitConfig)
	}()
	log.Info().Str("component", "OCR_HTTP").Str("listenAddr", listenAddr).Msg("Starting listener...")

	if useHttps {
		if certFile == "" || keyFile == "" {
			log.Fatal().Msg("usehttps flag only makes sense if both the private key and a certificate are available")
		}
		var httpsSrv = makeHTTPServer(&rabbitConfig, ocrChain)
		httpsSrv.Addr = listenAddr

		// crypto settings
		cryptSettings := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}
		httpsSrv.TLSConfig = cryptSettings

		if err := httpsSrv.ListenAndServeTLS(certFile, keyFile); err != nil {
			log.Fatal().Err(err).Str("component", "CLI_HTTP").Caller().Msg("cli_https has failed to start")
		}
	} else {
		var httpSrv = makeHTTPServer(&rabbitConfig, ocrChain)
		httpSrv.Addr = listenAddr
		if err := httpSrv.ListenAndServe(); err != nil {
			log.Fatal().Err(err).Str("component", "CLI_HTTP").Caller().Msg("cli_http has failed to start")
		}
	}
}
