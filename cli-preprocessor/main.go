package main

import (
	"flag"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/xf0e/open-ocr"
)

// This assumes that there is a rabbit mq running
// To test it, fire up a webserver and send it a curl request

func init() {
	zerolog.TimeFieldFormat = time.StampMilli
}

func main() {
	var preprocessor string
	flagFunc := func() {
		flag.StringVar(
			&preprocessor,
			"preprocessor",
			"identity",
			"The preprocessor to use, eg, stroke-width-transform",
		)
	}

	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(flagFunc)

	// inifinite loop, since sometimes worker <-> rabbitmq connection
	// gets broken.  see https://github.com/tleyden/open-ocr/issues/4
	for {
		log.Info().Str("component", "PREPROCESSOR_WORKER").Msg("creating new preprocessor worker")
		preprocessorWorker, err := ocrworker.NewPreprocessorRpcWorker(
			&rabbitConfig,
			preprocessor,
		)
		if err != nil {
			log.Panic().Err(err).Str("component", "MAIN_PREPROSSOR").Msg("could not create rpc worker")
		}
		err = preprocessorWorker.Run()
		if err != nil {
			log.Error().Err(err).Str("component", "MAIN_PREPROSSOR").Msg("preprocessor worker failed")
		}

		// this happens when connection is closed
		err = <-preprocessorWorker.Done
		log.Error().Err(err).Str("component", "MAIN_PREPROSSOR").Msg("preprocessor worker failed")
	}
}
