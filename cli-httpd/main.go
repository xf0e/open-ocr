package main

import (
	"flag"
	"fmt"
	"net/http"
	// _ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	// "github.com/google/gops/agent"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xf0e/open-ocr"
)

// This assumes that there is a worker running
// To test it:
// curl -X POST -H "Content-Type: application/json" -d '{"img_url":"http://localhost:8081/img","engine":0}' http://localhost:8081/ocr

var (
	sha1ver   string
	buildTime string
	version   string
)

func init() {
	zerolog.TimeFieldFormat = time.StampMilli
	// Default level is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func main() {
	// defer profile.Start(profile.MemProfile).Stop()
	appStopLocal := false
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	/*	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal()
	}*/

	go func() {
		select {
		case sig := <-signals:
			log.Info().Str("component", "OCR_HTTP").Str("signal", sig.String()).
				Msg("Caught signal to terminate, will not serve any further requests. Once the ocr queue is empty," +
					" http daemon will terminate.")
			ocrworker.StopChan <- true
			for {
				// as soon number of queued requests reaches zero, http daemon will exit
				if len(ocrworker.Requests) == 0 {
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

	// any requests to root, just redirect to main page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ocrworker.ServiceCanAcceptMu.Lock()
		appStopLocal = ocrworker.AppStop
		ocrworker.ServiceCanAcceptMu.Unlock()
		text := ocrworker.GenerateLandingPage(appStopLocal, ocrworker.TechnicalErrorResManager)
		_, _ = fmt.Fprintf(w, text)
	})

	// http.Handle("/ocr", ocrworker.NewOcrHttpHandler(rabbitConfig))
	ocrChain := ocrworker.InstrumentHttpStatusHandler(ocrworker.NewOcrHttpHandler(rabbitConfig))

	http.Handle("/ocr", ocrChain)

	http.Handle("/ocr-file-upload", ocrworker.NewOcrHttpMultipartHandler(rabbitConfig))

	http.Handle("/ocr-status", ocrworker.NewOcrHttpStatusHandler())
	// expose metrics for prometheus
	http.Handle("/metrics", promhttp.Handler())

	listenAddr := fmt.Sprintf(":%d", httpPort)

	log.Info().Str("component", "OCR_HTTP").Str("listenAddr", listenAddr).Msg("Starting listener...")

	// start a goroutine which will run forever and decide if we have resources for incoming requests
	go func() {
		ocrworker.SetResManagerState(rabbitConfig)
	}()

	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal().Err(err).Str("component", "CLI_HTTP").Caller().Msg("cli_http has failed to start")
	}

}
