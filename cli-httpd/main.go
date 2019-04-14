package main

import (
	"flag"
	"fmt"
	"github.com/couchbaselabs/logg"
	"github.com/xf0e/open-ocr"
	"net/http"
	_ "net/http/pprof"
)

// This assumes that there is a worker running
// To test it:
// curl -X POST -H "Content-Type: application/json" -d '{"img_url":"http://localhost:8081/img","engine":0}' http://localhost:8081/ocr

func init() {
	logg.LogKeys["OCR"] = true
	logg.LogKeys["OCR_CLIENT"] = true
	logg.LogKeys["OCR_WORKER"] = true
	logg.LogKeys["OCR_HTTP"] = true
	logg.LogKeys["OCR_TESSERACT"] = true
	logg.LogKeys["OCR_SANDWICH"] = true
	logg.LogKeys["OCR_RESMAN"] = true
}

func main() {

	var httpPort int
	flagFunc := func() {
		flag.IntVar(
			&httpPort,
			"http_port",
			8080,
			"The http port to listen on, eg, 8081",
		)

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

	logg.LogTo("OCR_HTTP", "Starting listener on %v", listenAddr)

	// start a goroutine which will run forever and decide if we have resources for incoming requests
	go func() {
		ocrworker.SetResManagerState(rabbitConfig)
	}()

	logg.LogError(http.ListenAndServe(listenAddr, nil))

}
