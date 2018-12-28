package ocrworker

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/couchbaselabs/logg"
)

type OcrHttpStatusHandler struct {
}

func NewOcrHttpStatusHandler() *OcrHttpStatusHandler {
	return &OcrHttpStatusHandler{}
}

func (s *OcrHttpStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	logg.LogTo("OCR_HTTP", "serveHttp called")
	defer req.Body.Close()

	ocrRequest := OcrRequest{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&ocrRequest)
	if err != nil {
		logg.LogError(err)
		http.Error(w, "unable to unmarshal json", 500)
		return
	}

	ocrResult, err := CheckOcrStatusByID(ocrRequest.ImgUrl)
	if err != nil {
		msg := "unable to perform OCR status check.  Error: %v"
		errMsg := fmt.Sprintf(msg, err)
		logg.LogError(fmt.Errorf(errMsg))
		http.Error(w, errMsg, 500)
		return
	}

	// logg.LogTo("OCR_HTTP", "ocrResult: %v", ocrResult)
	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(ocrResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(js)
}
