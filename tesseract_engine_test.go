package ocrworker

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"testing"

	"io/ioutil"

	"github.com/couchbaselabs/go.assert"
)

func TestTesseractEngineWithRequest(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	engine := TesseractEngine{}
	bytes, err := ioutil.ReadFile("docs/testimage.png")
	assert.True(t, err == nil)

	cFlags := make(map[string]interface{})
	cFlags["tessedit_char_whitelist"] = "0123456789"

	ocrRequest := OcrRequest{
		ImgBytes:   bytes,
		EngineType: EngineTesseract,
		EngineArgs: cFlags,
	}

	assert.True(t, err == nil)
	result, err := engine.ProcessRequest(ocrRequest)
	assert.True(t, err == nil)
	log.Info().Str("component", "TEST").Interface("result", result)

}

func TestTesseractEngineWithJson(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	testJsons := []string{}
	testJsons = append(testJsons, `{"engine":"tesseract"}`)
	testJsons = append(testJsons, `{"engine":"tesseract", "engine_args":{}}`)
	testJsons = append(testJsons, `{"engine":"tesseract", "engine_args":null}`)
	testJsons = append(testJsons, `{"engine":"tesseract", "engine_args":{"config_vars":{"tessedit_char_whitelist":"0123456789"}, "-psm":"1"}}`)
	testJsons = append(testJsons, `{"engine":"tesseract", "engine_args":{"config_vars":{"tessedit_create_hocr":"1", "tessedit_pageseg_mode":"1"}, "-psm":"3"}}`)

	for _, testJson := range testJsons {
		log.Info().Str("component", "TEST").Interface("testJson", testJson)
		ocrRequest := OcrRequest{}
		err := json.Unmarshal([]byte(testJson), &ocrRequest)
		assert.True(t, err == nil)
		bytes, err := ioutil.ReadFile("docs/testimage.png")
		assert.True(t, err == nil)
		ocrRequest.ImgBytes = bytes
		engine := NewOcrEngine(ocrRequest.EngineType)
		result, err := engine.ProcessRequest(ocrRequest)
		log.Error().Err(err).Str("component", "TEST")

		assert.True(t, err == nil)
		log.Info().Str("component", "TEST").Interface("result", result)
	}

}

func TestNewTesseractEngineArgs(t *testing.T) {
	testJson := `{"engine":"tesseract", "engine_args":{"config_vars":{"tessedit_char_whitelist":"0123456789"}, "psm":"0", "lang":"jpn"}}`
	ocrRequest := OcrRequest{}
	err := json.Unmarshal([]byte(testJson), &ocrRequest)
	assert.True(t, err == nil)
	engineArgs, err := NewTesseractEngineArgs(ocrRequest)
	assert.True(t, err == nil)
	assert.Equals(t, len(engineArgs.configVars), 1)
	assert.Equals(t, engineArgs.configVars["tessedit_char_whitelist"], "0123456789")
	assert.Equals(t, engineArgs.pageSegMode, "0")
	assert.Equals(t, engineArgs.lang, "jpn")

}

func TestTesseractEngineWithFile(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	engine := TesseractEngine{}
	engineArgs := TesseractEngineArgs{}
	result, err := engine.processImageFile("docs/testimage.png", engineArgs)
	assert.True(t, err == nil)
	log.Info().Str("component", "TEST").Interface("result", result)

}
