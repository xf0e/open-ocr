package ocrworker

import (
	"fmt"
	"sync"
	"time"
)

var (
	requestsAndTimersMu sync.RWMutex
	// Requests is for holding and monitoring queued requests
	Requests        = make(map[string]chan OcrResult)
	requestChannels = make(map[string]chan bool)
)

// CheckOcrStatusByID checks status of an ocr request based on origin of request
func CheckOcrStatusByID(requestID string) (OcrResult, bool) {
	requestsAndTimersMu.RLock()
	if _, ok := Requests[requestID]; !ok {
		requestsAndTimersMu.RUnlock()
		// log.Info().Str("component", "OCR_CLIENT").Str("requestID", requestID).Msg("no such request found in the queue")
		return OcrResult{}, false // fmt.Errorf("no such request %s", requestID)
	}

	// log.Debug().Str("component", "OCR_CLIENT").Msg("getting ocrResult := <-Requests[requestID]")
	ocrResult := OcrResult{}
	select {
	case ocrResult = <-Requests[requestID]:
		// log.Debug().Str("component", "OCR_CLIENT").Msg("got ocrResult := <-Requests[requestID]")
	default:
		_, ok := Requests[requestID]
		if ok {
			return OcrResult{Status: "processing", ID: requestID}, true
		}
	}
	requestsAndTimersMu.RUnlock()

	return ocrResult, true
}

func deleteRequestFromQueue(requestID string) {
	requestsAndTimersMu.RLock()

	inFlightGauge.Dec()
	println("!!!!!!!!!!before deleting from Requests and requestChannels")
	for key, element := range Requests {
		fmt.Println("Key:", key, "=>", "Element:", element)
	}
	delete(Requests, requestID)
	delete(requestChannels, requestID)

	println("!!!!!!!!!!after deleting from Requests and requestChannels")

	for key, element := range requestChannels {
		fmt.Println("Key:", key, "=>", "Element:", element)
	}

	requestsAndTimersMu.RUnlock()
	/*log.Info().Str("component", "OCR_CLIENT").
	  Int("nOfPendingReqs", len(Requests)).
	  Int("nOfPendingTimers", len(requestChannels)).
	  Msg("deleted request from the queue")
	*/
}

func addNewOcrResultToQueue(storageTime int, requestID string, rpcResponseChan chan OcrResult) {

	inFlightGauge.Inc()
	timerChan := make(chan bool, 1)
	requestsAndTimersMu.RLock()
	Requests[requestID] = rpcResponseChan
	requestChannels[requestID] = timerChan
	requestsAndTimersMu.RUnlock()

	// this go routine will cancel the request after global timeout or if request was sent back
	go func() {
		select {
		case timeOutOccurred := <-requestChannels[requestID]:
			fmt.Println(timeOutOccurred)
			if _, ok := Requests[requestID]; ok { // && ocrResult.Status != "processing" {
				deleteRequestFromQueue(requestID)
			}
		case <-time.After(time.Second * time.Duration(storageTime+200)):
			if _, ok := Requests[requestID]; ok { // && ocrResult.Status != "processing" {
				deleteRequestFromQueue(requestID)
			}
		}
	}()
}
