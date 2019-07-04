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

}

func main() {
	/*	SaveFile := false
		flagFunc := func() {
			flag.BoolVar(
				&SaveFile,
				"save_file",
				false,
				"The http port to listen on, eg, 8081",
			)
		}*/

	noOpFlagFunc := ocrworker.NoOpFlagFunction()
	noOpFlagFuncEngine := ocrworker.NoOpFlagFunctionEngine()
	engineConfig := ocrworker.DefaultConfigFlagsEngineOverride(noOpFlagFuncEngine)
	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(noOpFlagFunc)

	// infinite loop, since sometimes worker <-> rabbitmq connection
	// gets broken.  see https://github.com/tleyden/open-ocr/issues/4
	for {
		log.Info().
			Str("component", "OCR_WORKER").
			Msg("Creating new OCR Worker")

		ocrWorker, err := ocrworker.NewOcrRpcWorker(rabbitConfig, engineConfig)
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
