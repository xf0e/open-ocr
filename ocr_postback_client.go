package ocrworker

import (
	"bytes"
	"encoding/json"
	"github.com/couchbaselabs/logg"
	"io/ioutil"
	"net/http"
	"time"
)

var postTimeout = time.Duration(5 * time.Second)

type OcrPostClient struct {
}

func NewOcrPostClient() *OcrPostClient {
	return &OcrPostClient{}
}

func (c *OcrPostClient) postOcrRequest(requestID string, replyToAddress string) error {
	logg.LogTo("OCR_HTTP", "Post response called")
	logg.LogTo("OCR_HTTP", "sending ocr to: %s ", replyToAddress)

	ocrResult, err := CheckOcrStatusByID(requestID)

	jsonReply, err := json.Marshal(ocrResult)
	if err != nil {
		ocrResult.Status = "error"
	}

	req, err := http.NewRequest("POST", replyToAddress, bytes.NewBuffer(jsonReply))
	req.Close = true
	req.Header.Set("User-Agent", "open-ocr/1.5")
	req.Header.Set("X-Custom-Header", "automated reply")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: postTimeout}
	resp, err := client.Do(req)
	if err != nil {
		logg.LogWarn("OCR_HTTP: ocr was not delivered. %s did not respond", replyToAddress)
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logg.LogWarn("OCR_HTTP: ocr was not delivered. %s did not respond", replyToAddress)
		return err
	}
	logg.LogTo("OCR_HTTP", "response from ocr delivery %s: ", string(body))
	return err
}
