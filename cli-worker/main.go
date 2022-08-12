package main

import (
	"fmt"
	"net/url"
	"os"

	// _ "net/http/pprof"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xf0e/open-ocr"
)

var (
	sha1ver   string
	buildTime string
	version   string
)

// This assumes that there is a rabbit mq running
// To test it, fire up a web server and send it a curl request

func init() {
	zerolog.TimeFieldFormat = time.StampMilli
	// Default level is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func main() {
	noOpFlagFuncEngine := ocrworker.NoOpFlagFunctionWorker()
	workerConfig, err := ocrworker.DefaultConfigFlagsWorkerOverride(noOpFlagFuncEngine)
	if err != nil {
		log.Panic().Str("component", "OCR_WORKER").
			Msgf("error getting arguments: %v ", err)
	}

	if workerConfig.FlgVersion {
		fmt.Printf("version %s. Build on %s from git commit hash %s\n", version, buildTime, sha1ver)
		os.Exit(0)
	}

	if workerConfig.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// copy configuration for logging purposes to prevent leaking passwords to logs
	workerConfigToLog := workerConfig
	urlToLog, err := url.Parse(workerConfigToLog.AmqpAPIURI)
	if err == nil {
		workerConfigToLog.AmqpAPIURI = ocrworker.StripPasswordFromUrl(urlToLog)
	}
	urlToLog, err = url.Parse(workerConfigToLog.AmqpURI)
	if err == nil {
		workerConfigToLog.AmqpURI = ocrworker.StripPasswordFromUrl(urlToLog)
	}

	log.Info().Interface("workerConfig", workerConfigToLog).Msg("worker started with this parameters")

	// infinite loop, since sometimes worker <-> rabbitmq connection
	// gets broken.  see https://github.com/tleyden/open-ocr/issues/4
	for {
		log.Info().
			Str("component", "OCR_WORKER").
			Msg("Creating new OCR Worker")

		ocrWorker, err := ocrworker.NewOcrRpcWorker(&workerConfig)
		if err != nil {
			log.Panic().Str("component", "OCR_WORKER").
				Msg("Could not create rpc worker")
		}

		if err := ocrWorker.Run(); err != nil {
			log.Panic().Str("component", "OCR_WORKER").
				Msgf("Error running worker: %v", err)
		}

		// this happens when connection is closed
		err = <-ocrWorker.Done
		log.Error().
			Str("component", "OCR_WORKER").Err(err).
			Msg("OCR Worker failed with error")
	}
}
