package ocrworker

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/rs/zerolog/log"

	"github.com/couchbaselabs/go.assert"
)

func TestSandwichEngineWithRequest(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	engine := SandwichEngine{}
	bytes, err := ioutil.ReadFile("docs/testimage.pdf")
	// bytes, err := ioutil.ReadFile("docs/testimage.png")
	assert.True(t, err == nil)

	cFlags := make(map[string]interface{})
	cFlags["tessedit_char_whitelist"] = "0123456789"
	cFlags["ocr_type"] = "ocrlayeronly"

	ocrRequest := OcrRequest{
		ImgBytes:   bytes,
		EngineType: EngineSandwichTesseract,
		EngineArgs: cFlags,
		TimeOut:    30,
	}

	workerConfig := workerConfigForTests()

	assert.True(t, err == nil)
	result, err := engine.ProcessRequest(ocrRequest, workerConfig)
	assert.True(t, err == nil)
	log.Info().Str("component", "TEST").Interface("result", result)

}

func TestSandwichEngineWithJson(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	var testJsons []string
	/*testJsons = append(testJsons, `{"engine":"sandwich"}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":null}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"config_vars":{"tessedit_char_whitelist":"0123456789"}, "psm":"1"}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"config_vars":{"tessedit_create_hocr":"1", "tessedit_pageseg_mode":"1"}, "psm":"3"}}`)*/
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"lang":"deu", "ocr_type":"ocrlayeronly","result_optimize":true}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"lang":"deu", "ocr_type":"combinedpdf","result_optimize":true}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"lang":"deu", "ocr_type":"combinedpdf","result_optimize":false}}`)

	for _, testJson := range testJsons {
		log.Info().Str("component", "TEST").Interface("testJson", testJson)
		ocrRequest := OcrRequest{TimeOut: 60}
		workerConfig := workerConfigForTests()
		err := json.Unmarshal([]byte(testJson), &ocrRequest)
		assert.True(t, err == nil)
		bytes, err := ioutil.ReadFile("docs/testimage.pdf")
		assert.True(t, err == nil)
		ocrRequest.ImgBytes = bytes
		engine := NewOcrEngine(ocrRequest.EngineType)
		result, err := engine.ProcessRequest(ocrRequest, workerConfig)
		log.Error().Err(err).Str("component", "TEST")
		assert.True(t, err == nil)
		log.Info().Str("component", "TEST").Interface("result", result)

	}

}

func TestNewsandwichEngineArgs(t *testing.T) {
	testJSON := `{"engine":"sandwich", "engine_args":{"config_vars":{"tessedit_char_whitelist":"0123456789"},"ocr_type":"combinedpdf", "psm":"0", "lang":"eng"}}`
	ocrRequest := OcrRequest{}
	workerConfig := workerConfigForTests()
	err := json.Unmarshal([]byte(testJSON), &ocrRequest)
	assert.True(t, err == nil)
	engineArgs, err := NewSandwichEngineArgs(ocrRequest, workerConfig)
	assert.True(t, err == nil)
	assert.Equals(t, len(engineArgs.configVars), 1)
	assert.Equals(t, engineArgs.configVars["tessedit_char_whitelist"], "0123456789")
	// assert.Equals(t, engineArgs.pageSegMode, "0")
	assert.Equals(t, engineArgs.lang, "eng")

}

func TestSandwichEngineWithFile(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	engine := SandwichEngine{}
	engineArgs := SandwichEngineArgs{}
	engineArgs.ocrType = "combinedpdf"
	engineArgs.ocrOptimize = true
	engineArgs.lang = "deu"
	engineArgs.saveFiles = true
	result, err := engine.processImageFile("docs/testimage.pdf", "PDF", engineArgs, 20)
	log.Warn().Err(err).Str("component", "TEST")
	assert.True(t, err == nil)

	log.Info().Str("component", "TEST").Interface("result", result)

}
