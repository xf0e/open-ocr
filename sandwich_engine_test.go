package ocrworker

import (
	"encoding/json"
	"testing"

	"io/ioutil"

	"github.com/couchbaselabs/go.assert"
	"github.com/couchbaselabs/logg"
)

func TestSandwichEngineWithRequest(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	engine := SandwichEngine{}
	bytes, err := ioutil.ReadFile("docs/ocrimage.pdf")
	//bytes, err := ioutil.ReadFile("docs/testimage.png")
	assert.True(t, err == nil)

	cFlags := make(map[string]interface{})
	cFlags["tessedit_char_whitelist"] = "0123456789"

	ocrRequest := OcrRequest{
		ImgBytes:   bytes,
		EngineType: EngineSandwichTesseract,
		EngineArgs: cFlags,
	}

	assert.True(t, err == nil)
	result, err := engine.ProcessRequest(ocrRequest)
	assert.True(t, err == nil)
	logg.LogTo("TEST", "result: %v", result)

}

func TestSandwichEngineWithJson(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	testJsons := []string{}
	/*testJsons = append(testJsons, `{"engine":"sandwich"}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":null}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"config_vars":{"tessedit_char_whitelist":"0123456789"}, "psm":"1"}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"config_vars":{"tessedit_create_hocr":"1", "tessedit_pageseg_mode":"1"}, "psm":"3"}}`)*/
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"lang":"deu", "ocr_type":"ocrlayeronly","result_optimize":true}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"lang":"deu", "ocr_type":"combinedpdf","result_optimize":true}}`)
	testJsons = append(testJsons, `{"engine":"sandwich", "engine_args":{"lang":"deu", "ocr_type":"combinedpdf","result_optimize":false}}`)

	for _, testJson := range testJsons {
		logg.LogTo("TEST", "testJson: %v", testJson)
		ocrRequest := OcrRequest{}
		err := json.Unmarshal([]byte(testJson), &ocrRequest)
		assert.True(t, err == nil)
		bytes, err := ioutil.ReadFile("docs/testimage.pdf")
		assert.True(t, err == nil)
		ocrRequest.ImgBytes = bytes
		engine := NewOcrEngine(ocrRequest.EngineType)
		result, err := engine.ProcessRequest(ocrRequest)
		logg.LogTo("TEST", "err: %v", err)
		assert.True(t, err == nil)
		logg.LogTo("TEST", "result: %v", result)

	}

}

func TestNewsandwichEngineArgs(t *testing.T) {
	testJson := `{"engine":"sandwich", "engine_args":{"config_vars":{"tessedit_char_whitelist":"0123456789"},"ocr_type":"combinedpdf", "psm":"0", "lang":"eng"}}`
	ocrRequest := OcrRequest{}
	err := json.Unmarshal([]byte(testJson), &ocrRequest)
	assert.True(t, err == nil)
	engineArgs, err := NewSandwichEngineArgs(ocrRequest)
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
	result, err := engine.processImageFile("docs/testimage.pdf", "PDF", engineArgs)
	logg.LogWarn("error %v", err)
	assert.True(t, err == nil)
	logg.LogTo("TEST", "result: %v", result)

}
