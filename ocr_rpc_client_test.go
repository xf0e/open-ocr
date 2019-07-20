package ocrworker

import (
	"github.com/rs/zerolog/log"
	"testing"

	"github.com/couchbaselabs/go.assert"
)

func init() {
}

func workerConfigForTests() WorkerConfig {
	workerConfig := DefaultWorkerConfig()
	return workerConfig
}

func rabbitConfigForTests() RabbitConfig {
	rabbitConfig := DefaultTestConfig()
	return rabbitConfig
}

// This test assumes that rabbit mq is running
func DisabledTestOcrRpcClientIntegration(t *testing.T) {

	// TODO: serve this up through a fake webserver
	// that reads from the filesystem
	testImageUrl := "http://localhost:8080/img"

	rabbitConfig := rabbitConfigForTests()
	workerConfig := workerConfigForTests()

	requestID := "426d0ef9-a0c9-48cb-4562-f9d6f29a6ba5"

	// kick off a worker
	// this would normally happen on a different machine ..
	ocrWorker, err := NewOcrRpcWorker(workerConfig)
	if err != nil {
		log.Error().Str("component", "TEST").Err(err)
	}
	ocrWorker.Run()

	ocrClient, err := NewOcrRpcClient(rabbitConfig)
	if err != nil {
		log.Error().Str("component", "TEST").Err(err)
	}
	assert.True(t, err == nil)

	for i := 0; i < 50; i++ {

		ocrRequest := OcrRequest{ImgUrl: testImageUrl, EngineType: EngineMock}
		decodeResult, err := ocrClient.DecodeImage(ocrRequest, requestID)
		if err != nil {
			log.Error().Str("component", "TEST").Err(err)
		}
		assert.True(t, err == nil)
		log.Info().Str("component", "TEST").Str("decodeResult", decodeResult.Text)
		assert.Equals(t, decodeResult.Text, MOCK_ENGINE_RESPONSE)

	}

}
