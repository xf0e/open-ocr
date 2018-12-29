package ocrworker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
	"io/ioutil"
	"net/http"
	"time"
)

type OcrPostClient struct {
}

func NewOcrPostClient() *OcrPostClient {
	return &OcrPostClient{}
}

func (c *OcrPostClient) Post(requestID string, replyToAddress string) error {
	logg.LogTo("OCR_HTTP", "Post response called")

	fmt.Println("URL:>", replyToAddress)
	fmt.Println(requestID)

	ocrResult, err := CheckOcrStatusByID(requestID)

	jsonReply, err := json.Marshal(ocrResult)
	println(ocrResult.Text)
	println(ocrResult.Status)
	println(ocrResult.Id)
	if err != nil {
		ocrResult.Text = requestID
		ocrResult.Status = "error"
	}
	// println(ocrResult.Status)
	// println(ocrResult.Text[0:64])
	req, err := http.NewRequest("POST", replyToAddress, bytes.NewBuffer(jsonReply))
	req.Close = true
	req.Header.Set("User-Agent", "open-ocr/1.5")
	req.Header.Set("X-Custom-Header", "automated reply")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(2 * time.Second)}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return err
}
