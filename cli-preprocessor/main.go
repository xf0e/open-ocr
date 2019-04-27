package main

import (
	"flag"
	"github.com/rs/zerolog/log"

	"github.com/couchbaselabs/logg"
	"github.com/xf0e/open-ocr"
)

// This assumes that there is a rabbit mq running
// To test it, fire up a webserver and send it a curl request

func init() {

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
		logg.LogTo("PREPROCESSOR_WORKER", "Creating new Preprocessor Worker")
		preprocessorWorker, err := ocrworker.NewPreprocessorRpcWorker(
			rabbitConfig,
			preprocessor,
		)
		if err != nil {
			logg.LogPanic("Could not create rpc worker: %v", err)
		}
		err = preprocessorWorker.Run()
		if err != nil {
			log.Error().Err(err).Str("compoent", "MAIN_PREPROSSOR").Msg("preprocessor worker failed")
		}

		// this happens when connection is closed
		err = <-preprocessorWorker.Done
		log.Error().Err(err).Str("compoent", "MAIN_PREPROSSOR").Msg("preprocessor worker failed")
	}

}
