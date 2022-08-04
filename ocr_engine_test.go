package ocrworker

import (
	"encoding/json"
	"testing"

	"github.com/rs/zerolog/log"

	"github.com/couchbaselabs/go.assert"
)

func TestOcrEngineTypeJson(t *testing.T) {
	testJson := `{"img_url":"foo", "engine":"tesseract"}`
	ocrRequest := OcrRequest{}
	err := json.Unmarshal([]byte(testJson), &ocrRequest)
	if err != nil {
		log.Error().Str("component", "TEST").Err(err)
	}
	assert.True(t, err == nil)
	assert.Equals(t, ocrRequest.EngineType, EngineTesseract)
	log.Error().Str("component", "TEST").Interface("ocrRequest", ocrRequest)
}
