package ocrworker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/couchbaselabs/go.assert"
)

// This test assumes that rabbit mq is running
func DisabledTestOcrHttpHandlerIntegration(t *testing.T) {
	rabbitConfig := rabbitConfigForTests()
	workerConfig := workerConfigForTests()

	err := spawnOcrWorker(&workerConfig)
	if err != nil {
		log.Panic().Msg("Could not spawn ocr worker")
	}

	// add a handler to serve up an image from the filesystem.
	http.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "refactoring.png")
	})

	http.Handle("/ocr", NewOcrHttpHandler(&rabbitConfig))

	go http.ListenAndServe(":8081", nil)

	log.Info().Str("component", "TEST").Msg("test1")

	ocrRequest := OcrRequest{
		ImgUrl:     "http://localhost:8081/img",
		EngineType: EngineMock,
	}
	jsonBytes, err := json.Marshal(ocrRequest)
	if err != nil {
		log.Panic().Msg("Could not marshal OcrRequest")
	}

	reader := bytes.NewReader(jsonBytes)

	resp, err := http.Post("http://localhost:8081/ocr", "application/json", reader)
	assert.True(t, err == nil)
	log.Info().Str("component", "TEST").Interface("resp", resp)

	// connect to it via http client
	log.Info().Str("component", "TEST").Msg("Sleep for 60s")
	time.Sleep(time.Second * 60)

	// make sure get expected result

	assert.True(t, true)
}

func spawnOcrWorker(workerConfig *WorkerConfig) error {
	// kick off a worker
	// this would normally happen on a different machine ..
	ocrWorker, err := NewOcrRpcWorker(workerConfig)
	if err != nil {
		return err
	}
	ocrWorker.Run()
	return nil
}
