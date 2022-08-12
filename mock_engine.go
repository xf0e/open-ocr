package ocrworker

const MockEngineResponse = "mock engine decoder response"

type MockEngine struct{}

// ProcessRequest will process incoming OCR request by routing it through the whole process chain
func (MockEngine) ProcessRequest(ocrRequest *OcrRequest, workerConfig *WorkerConfig) (OcrResult, error) {
	return OcrResult{Text: MockEngineResponse}, nil
}
