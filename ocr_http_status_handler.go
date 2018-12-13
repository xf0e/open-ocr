package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
	"net/http"
)

type OcrHttpStatusHandler struct {
}

func NewOcrHttpStatusHandler() *OcrHttpStatusHandler {
	return &OcrHttpStatusHandler{}
}

func (s *OcrHttpStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logg.LogTo("OCR_HTTP", "serveHttp called")
	defer req.Body.Close()

	OcrRequest := OcrRequest{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&OcrRequest)
	if err != nil {
		logg.LogError(err)
		http.Error(w, "unable to unmalrshal json", 500)
		return
	}

	ocrResult, err := CheckOcrStatusById(OcrRequest.ImgUrl)

	if err != nil {
		msg := "unable to perform OCR status check. Error: %v"
		errMsg := fmt.Sprintf(msg, err)
		logg.LogError(fmt.Errorf(errMsg))
		http.Error(w, errMsg, 500)
		return
	}

	logg.LogTo("OCR_HTTP", "ocrResult: %v", ocrResult)
	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(ocrResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(js)
}
