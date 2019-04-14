package main

import (
	"github.com/couchbaselabs/logg"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xf0e/open-ocr"
	_ "net/http/pprof"
	"time"
)

// This assumes that there is a rabbit mq running
// To test it, fire up a web server and send it a curl request

func init() {
	logg.LogKeys["OCR"] = true
	logg.LogKeys["OCR_CLIENT"] = true
	logg.LogKeys["OCR_WORKER"] = true
	logg.LogKeys["OCR_HTTP"] = true
	logg.LogKeys["OCR_TESSERACT"] = true
	logg.LogKeys["OCR_SANDWICH"] = true

	zerolog.TimeFieldFormat = time.StampMilli

}

func main() {

	noOpFlagFunc := ocrworker.NoOpFlagFunction()
	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(noOpFlagFunc)

	// infinite loop, since sometimes worker <-> rabbitmq connection
	// gets broken.  see https://github.com/tleyden/open-ocr/issues/4
	for {
		log.Info().
			Str("component", "OCR_WORKER").
			Msg("Creating new OCR Worker")

		ocrWorker, err := ocrworker.NewOcrRpcWorker(rabbitConfig)
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
