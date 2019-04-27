package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

type OcrHttpMultipartHandler struct {
	RabbitConfig RabbitConfig
}

func NewOcrHttpMultipartHandler(r RabbitConfig) *OcrHttpMultipartHandler {
	return &OcrHttpMultipartHandler{
		RabbitConfig: r,
	}
}

func (s *OcrHttpMultipartHandler) extractParts(req *http.Request) (OcrRequest, error) {

	log.Info().Str("component", "OCR_HTTP").Msg("request to ocr-file-upload")
	ocrReq := OcrRequest{}

	switch req.Method {
	case "POST":
		h := req.Header.Get("Content-Type")
		log.Info().Str("component", "OCR_HTTP").
			Str("content_type", h).
			Msg("content type")

		contentType, attrs, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		log.Info().Str("component", "OCR_HTTP").
			Str("content_type", contentType).
			Msg("content type")

		if !strings.HasPrefix(h, "multipart/related") {
			return ocrReq, fmt.Errorf("expected multipart related")
		}

		reader := multipart.NewReader(req.Body, attrs["boundary"])

		for {

			part, err := reader.NextPart()

			if err == io.EOF {
				break
			}
			contentTypeOuter := part.Header["Content-Type"][0]
			contentType, attrs, _ := mime.ParseMediaType(contentTypeOuter)

			log.Info().Str("component", "OCR_HTTP").Interface("attrs", attrs)

			switch contentType {
			case "application/json":
				decoder := json.NewDecoder(part)
				err := decoder.Decode(&ocrReq)
				if err != nil {
					return ocrReq, fmt.Errorf("unable to unmarshal json: %s", err)
				}
				part.Close()
			default:
				if !strings.HasPrefix(contentType, "image") {
					return ocrReq, fmt.Errorf("expected content-type: image/*")
				}

				partContents, err := ioutil.ReadAll(part)
				if err != nil {
					return ocrReq, fmt.Errorf("failed to read mime part: %v", err)
				}

				ocrReq.ImgBytes = partContents
				return ocrReq, nil

			}

		}

		return ocrReq, fmt.Errorf("didn't expect to get this far")

	default:
		return ocrReq, fmt.Errorf("this endpoint only accepts POST requests")
	}

}

func (s *OcrHttpMultipartHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	defer req.Body.Close()

	ocrRequest, err := s.extractParts(req)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_HTTP")
		errStr := fmt.Sprintf("Error extracting multipart/related parts: %v", err)
		http.Error(w, errStr, 500)
		return
	}

	ocrResult, err := HandleOcrRequest(ocrRequest, s.RabbitConfig)

	if err != nil {
		msg := "Unable to perform OCR decode."
		log.Error().Err(err).Str("component", "OCR_HTTP").Msg(msg)
		http.Error(w, msg, 500)
		return
	}

	fmt.Fprintf(w, ocrResult.Text)

}
