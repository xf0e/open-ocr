package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
)

type OcrHttpStatusHandler struct {
}

func NewOcrHttpStatusHandler() *OcrHttpStatusHandler {
	return &OcrHttpStatusHandler{}
}

func (s *OcrHttpStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	log.Info().Str("component", "OCR_STATUS").Msg("serveHttp called")

	ocrRequest := OcrRequest{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&ocrRequest)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_STATUS")
		http.Error(w, "unable to unmarshal json", 400)
		return
	}

	ocrResult, err := CheckOcrStatusByID(ocrRequest.ImgUrl)
	if err != nil {
		msg := "unable to perform OCR status check. %v"
		errMsg := fmt.Sprintf(msg, err)
		log.Error().Err(err).Str("component", "OCR_STATUS")
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(ocrResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(js)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_STATUS")
	}

	_ = req.Body.Close()
}
