package ocrworker

import (
	"sync"
	"time"
)

var (
	requestsAndTimersMu sync.RWMutex
	// Requests is for holding and monitoring queued requests
	Requests = make(map[string]chan OcrResult)
	timers   = make(map[string]*time.Timer)
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
	/* println("!!!!!!!!!!before deleting from Requests and timers")
	   for key, element := range Requests {
	       fmt.Println("Key:", key, "=>", "Element:", element)
	   }*/
	delete(Requests, requestID)
	timers[requestID].Stop()
	delete(timers, requestID)
	/*
	   println("!!!!!!!!!!after deleting from Requests and timers")

	   for key, element := range timers {
	       fmt.Println("Key:", key, "=>", "Element:", element)
	   }*/

	requestsAndTimersMu.RUnlock()
	/*log.Info().Str("component", "OCR_CLIENT").
	  Int("nOfPendingReqs", len(Requests)).
	  Int("nOfPendingTimers", len(timers)).
	  Msg("deleted request from the queue")
	*/
}

func addNewOcrResultToQueue(storageTime int, requestID string, rpcResponseChan chan OcrResult) {

	inFlightGauge.Inc()
	timer := time.NewTimer(time.Duration(storageTime) * time.Second)
	requestsAndTimersMu.RLock()
	Requests[requestID] = rpcResponseChan
	timers[requestID] = timer
	requestsAndTimersMu.RUnlock()

	// thi go routine will cancel the request after global timeout if client stopped polling
	go func() {
		<-timer.C
		// ocrResult, ocrExists := CheckOcrStatusByID(requestID)
		// if ocrExists {
		if _, ok := Requests[requestID]; ok { // && ocrResult.Status != "processing" {
			deleteRequestFromQueue(requestID)
		}
	}()

}
