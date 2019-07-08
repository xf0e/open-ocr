package main

import (
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xf0e/open-ocr"
	"net/http"
	//_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// This assumes that there is a worker running
// To test it:
// curl -X POST -H "Content-Type: application/json" -d '{"img_url":"http://localhost:8081/img","engine":0}' http://localhost:8081/ocr

func init() {
	zerolog.TimeFieldFormat = time.StampMilli
	// Default level is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func main() {
	//defer profile.Start(profile.MemProfile).Stop()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

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
						Msg("The ocr queue is now empty. open-ocr http daemon will now exit. You may stop workers now")
					time.Sleep(5 * time.Second) // delay puffer for sending all requests back
					break
				}
				time.Sleep(1 * time.Second)
			}
			os.Exit(0)
		}
	}()

	var httpPort uint
	var debug bool
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
	}

	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(flagFunc)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	//phm := ocrworker.InitPrometheusHttpMetric("open-ocr", prometheus.LinearBuckets(0, 5, 20))

	// any requests to root, just redirect to main page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		text := `<!DOCTYPE html><html lang="en"> <head> <meta charset="utf-8"> <title>open-ocr</title><style>pre{ font-family: Fixedsys,Courier,monospace; padding: 0px; }</style> </head> <body>` +
			`<h1>Open-ocr  ></h1> Need <a href="https://godoc.org/github.com/xf0e/open-ocr">docs</a>?<div><p>Status: RUNNING</p><p>Nice day to put slinkies on an escalator!</p> ` +
			`<pre>  __   ; '.'  :
|  +. :  ;   :   __
.-+\   ':  ;   ,,-'  :
'.  '.  '\ ;  :'   ,'
,-+'+. '. \.;.:  _,' '-.
'.__  "-.::||::.:'___-+'
-"  '--..::::::_____.IIHb
'-:______.:; '::,,...,;:HB\
.+         \ ::,,...,;:HB \
'-.______.:+ \'+.,...,;:P'  \
.-'           \              \
'-.______.:+   \______________\
.-::::::::,     BBBBBBBBBBBBBBB
::,,...,;:HB    BBBBBBBBBBBBBBB
::,,...,;:HB    BBBBBBBBBBBBBBB
::,,...,;:HB\   BBBBBBBBBBBBBBB
::,,...,;:HB \  BBBBBBBBBBBBBBB
'+.,...,;:P'  \ BBBBBBBBBBBBBBB
               \BBBBBBBBBBBBBBB</pre></div><p>proudly made with BASIC:)</p></body> </html>`
		fmt.Fprintf(w, text)
	})

	//http.Handle("/ocr", ocrworker.NewOcrHttpHandler(rabbitConfig))
	http.Handle("/ocr", prometheus.InstrumentHandler("open-ocr-httpd", ocrworker.NewOcrHttpHandler(rabbitConfig)))

	//http.Handle("/ocr", phm.WrapHandler("open-ocr-httpd", ocrworker.NewOcrHttpHandler(rabbitConfig))

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
