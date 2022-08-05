package ocrworker

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

type OcrHttpStatusHandler struct{}

func NewOcrHttpStatusHandler() *OcrHttpStatusHandler {
	return &OcrHttpStatusHandler{}
}

func (s *OcrHttpStatusHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log.Debug().Str("component", "OCR_STATUS").Msg("OcrHttpStatusHandler called")

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
		log.Info().Str("component", "OCR_STATUS").Str("RequestID", ocrRequest.ImgUrl).
			Str("RemoteAddr", req.RemoteAddr).
			Msg("no such ocr request, processing time limit was probably reached for this request")
	}

	w.Header().Set("Content-Type", "application/json")
	js, err := json.Marshal(ocrResult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error().Err(err).Str("component", "OCR_STATUS").Str("RequestID", ocrRequest.ImgUrl).Str("RemoteAddr", req.RemoteAddr)
		return
	}
	_, err = w.Write(js)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_STATUS").Str("RequestID", ocrRequest.ImgUrl).Str("RemoteAddr", req.RemoteAddr)
	}
	if ocrRequestExists && err == nil {
		log.Info().Str("component", "OCR_STATUS").
			Str("RequestID", ocrRequest.ImgUrl).
			Str("RemoteAddr", req.RemoteAddr).
			Msg("ocr request was claimed")
	}
	_ = req.Body.Close()
}
