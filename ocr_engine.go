package ocrworker

import (
	"encoding/json"
	"strings"

	"github.com/couchbaselabs/logg"
)

type OcrEngineType int

const (
	EngineTesseract = OcrEngineType(iota)
	EngineGoTesseract
	EngineSandwichTesseract
	EngineMock
)

type OcrEngine interface {
	ProcessRequest(ocrRequest OcrRequest) (OcrResult, error)
}

func NewOcrEngine(engineType OcrEngineType) OcrEngine {
	switch engineType {
	case EngineMock:
		return &MockEngine{}
	case EngineTesseract:
		return &TesseractEngine{}
	case EngineSandwichTesseract:
		return &SandwichEngine{}
	}
	return nil
}

func (e OcrEngineType) String() string {
	switch e {
	case EngineMock:
		return "ENGINE_MOCK"
	case EngineTesseract:
		return "ENGINE_TESSERACT"
	case EngineGoTesseract:
		return "ENGINE_GO_TESSERACT"
	case EngineSandwichTesseract:
		return "ENGINE_SANDWICH_TESSERACT"

	}
	return ""
}

func (e *OcrEngineType) UnmarshalJSON(b []byte) (err error) {

	var engineTypeStr string

	if err := json.Unmarshal(b, &engineTypeStr); err == nil {
		engineString := strings.ToUpper(engineTypeStr)
		switch engineString {
		case "TESSERACT":
			*e = EngineTesseract
		case "GO_TESSERACT":
			*e = EngineGoTesseract
		case "SANDWICH":
			*e = EngineSandwichTesseract
		case "MOCK":
			*e = EngineMock
		default:
			logg.LogWarn("Unexpected OcrEngineType json: %v", engineString)
			*e = EngineMock
		}
		return nil
	}

	// not a string .. maybe it's an int

	var engineTypeInt int
	if err := json.Unmarshal(b, &engineTypeInt); err == nil {
		*e = OcrEngineType(engineTypeInt)
		return nil
	} else {
		return err
	}

}
