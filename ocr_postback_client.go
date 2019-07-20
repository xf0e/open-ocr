package ocrworker

import (
	"bytes"
	"encoding/json"
	"github.com/rs/zerolog"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var postTimeout = time.Duration(15 * time.Second)

type ocrPostClient struct {
}

func newOcrPostClient() *ocrPostClient {
	return &ocrPostClient{}
}

func (c *ocrPostClient) postOcrRequest(ocrResult *OcrResult, replyToAddress string, numTry uint8) error {
	logger := zerolog.New(os.Stdout).With().Str("RequestID", ocrResult.ID).Timestamp().Logger()
	logger.Info().Str("component", "OCR_HTTP").Msg("sending ocr request back to requester")
	logger.Info().Str("component", "OCR_HTTP").
		Uint8("attempt", numTry).
		Str("replyToAddress", replyToAddress).
		Msg("sending the ocr back to requester")

	jsonReply, err := json.Marshal(ocrResult)
	if err != nil {
		ocrResult.Status = "error"
	}

	req, err := http.NewRequest("POST", replyToAddress, bytes.NewBuffer(jsonReply))
	if err != nil {
		logger.Error().Str("component", "OCR_HTTP").Err(err).Msg("forming POST reply error")
	}
	req.Close = true
	req.Header.Set("User-Agent", "open-ocr/1.1.8-beta")
	req.Header.Set("X-Custom-Header", "automated reply")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: postTimeout}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn().Err(err).Str("component", "OCR_HTTP").
			Str("replyToAddress", replyToAddress).
			Msg("ocr was not delivered. Target did not respond")
		return err
	}

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

	err = resp.Body.Close()
	if err != nil {
		logger.Warn().
			Str("component", "OCR_HTTP").
			Err(err)
	}
	return err
}
