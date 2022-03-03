package ocrworker

import (
	"bytes"
	"encoding/json"
	"github.com/rs/zerolog/log"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var postTimeout = 50 * time.Second

type ocrPostClient struct {
}

func newOcrPostClient() *ocrPostClient {
	return &ocrPostClient{}
}

func (c *ocrPostClient) postOcrRequest(ocrResult *OcrResult, replyToAddress string, numTry uint) error {
	logger := zerolog.New(os.Stdout).With().Str("RequestID", ocrResult.ID).Timestamp().Logger()
	logger.Info().Str("component", "OCR_HTTP").
		Uint("attempt", numTry).
		Str("replyToAddress", replyToAddress).
		Msg("sending ocr back to requester")

	jsonReply, err := json.Marshal(ocrResult)
	if err != nil {
		ocrResult.Status = "error"
	}

	req, err := http.NewRequest("POST", replyToAddress, bytes.NewBuffer(jsonReply))
	if err != nil {
		logger.Error().Str("component", "OCR_HTTP").Err(err).Msg("forming POST reply error")
	}
	req.Close = true
	req.Header.Set("User-Agent", "open-ocr/"+version)
	req.Header.Set("X-open-ocr-reply-type", "automated reply")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: postTimeout}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn().Err(err).Str("component", "OCR_HTTP").
			Str("replyToAddress", replyToAddress).
			Msg("ocr was not delivered. Target did not respond")
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Warn().Err(err).Caller().Str("component", "OCR_HTTP").Msg(req.RequestURI + " response Body could not be removed")
		}
	}(resp.Body)

	body, err := ioutil.ReadAll(resp.Body)
	header := resp.StatusCode
	if err != nil {
		logger.Warn().Err(err).Str("component", "OCR_HTTP").
			Str("replyToAddress", replyToAddress).
			Msg("ocr was probably not delivered, response body is empty")
		return err
	}
	logger.Info().Str("component", "OCR_HTTP").
		Int("RESPONSE_CODE", header).
		Str("replyToAddress", replyToAddress).
		Interface("payload(first 32 bytes)", string(body[0:32])).
		Msg("target responded")

	return err
}
