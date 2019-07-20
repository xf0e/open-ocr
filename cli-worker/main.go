package main

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xf0e/open-ocr"
	//_ "net/http/pprof"
	"time"
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
	if workerConfig.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Debug().Interface("workerConfig", workerConfig).Msg("parameter list of workerConfig")

	// infinite loop, since sometimes worker <-> rabbitmq connection
	// gets broken.  see https://github.com/tleyden/open-ocr/issues/4
	for {
		log.Info().
			Str("component", "OCR_WORKER").
			Msg("Creating new OCR Worker")

		ocrWorker, err := ocrworker.NewOcrRpcWorker(workerConfig)
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
