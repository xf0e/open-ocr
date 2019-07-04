package ocrworker

const MOCK_ENGINE_RESPONSE = "mock engine decoder response"

type MockEngine struct {
}

func (m MockEngine) ProcessRequest(ocrRequest OcrRequest, engineConfig EngineConfig) (OcrResult, error) {
	return OcrResult{Text: MOCK_ENGINE_RESPONSE}, nil
}
