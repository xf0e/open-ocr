package main

import (
	"flag"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xf0e/open-ocr"
	"net/http"
	_ "net/http/pprof"
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

	var httpPort int
	var debug bool
	flagFunc := func() {
		flag.IntVar(
			&httpPort,
			"http_port",
			8080,
			"The http port to listen on, eg, 8081",
		)
		flag.BoolVar(
			&debug,
			"debug",
			false,
			"sets debug flag will print more messages",
		)

	}
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(flagFunc)

	// any requests to root, just redirect to main page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		text := `<h1>OpenOCR is running!<h1> Need <a href="http://www.openocr.net">docs</a>?`
		fmt.Fprintf(w, text)
	})

	http.Handle("/ocr", ocrworker.NewOcrHttpHandler(rabbitConfig))

	http.Handle("/ocr-file-upload", ocrworker.NewOcrHttpMultipartHandler(rabbitConfig))

	http.Handle("/ocr-status", ocrworker.NewOcrHttpStatusHandler())
	// add a handler to serve up an image from the filesystem.
	// ignore this, was just something for testing ..
	http.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../refactoring.png")
	})

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
