package main

import (
	"fmt"
	"github.com/couchbaselabs/logg"
	"github.com/xf0e/open-ocr"
	_ "net/http/pprof"
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
}

func main() {

	/*agent := stackimpact.Start(stackimpact.Options{
		AgentKey: "819507c0da027d68b0f6ee694dca6c3b389daeab",
		AppName: "Basic",
		AppVersion: "1.0.0",
		AppEnvironment: "dev",
	})*/

	noOpFlagFunc := ocrworker.NoOpFlagFunction()
	rabbitConfig := ocrworker.DefaultConfigFlagsOverride(noOpFlagFunc)

	// infinite loop, since sometimes worker <-> rabbitmq connection
	// gets broken.  see https://github.com/tleyden/open-ocr/issues/4
	for {
		logg.LogTo("OCR_WORKER", "Creating new OCR Worker")
		ocrWorker, err := ocrworker.NewOcrRpcWorker(rabbitConfig)
		if err != nil {
			logg.LogPanic("Could not create rpc worker")
		}

		// span := agent.Profile();
		if err := ocrWorker.Run(); err != nil {
			logg.LogPanic("Error running worker: %v", err)
		}
		// defer span.Stop()

		// this happens when connection is closed
		err = <-ocrWorker.Done
		logg.LogError(fmt.Errorf("OCR Worker failed with error: %v", err))
	}

}
