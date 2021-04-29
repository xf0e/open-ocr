package ocrworker

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"
)

type OcrHttpStatusHandler struct {
}

func NewOcrHttpStatusHandler() *OcrHttpStatusHandler {
	return &OcrHttpStatusHandler{}
}

func (s *OcrHttpStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	log.Info().Str("component", "OCR_STATUS").Msg("OcrHttpStatusHandler called")

	ocrRequest := OcrRequest{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&ocrRequest)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_STATUS")
		http.Error(w, "unable to unmarshal json", 400)
		return
	}

	ocrResult, ocrRequestExists := CheckOcrStatusByID(ocrRequest.ImgUrl)
	if !ocrRequestExists {
		ocrResult.Text = ""
		ocrResult.ID = ocrRequest.ImgUrl
		ocrResult.Status = "not found"
		msg := errors.New("no such ocr request. probably the timeout was reached for this request ID")
		log.Error().Err(msg).Str("component", "OCR_STATUS")
	}

	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(ocrResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error().Err(err).Str("component", "OCR_STATUS")
		return
	}
	_, err = w.Write(js)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_STATUS")
	}

	_ = req.Body.Close()
}
