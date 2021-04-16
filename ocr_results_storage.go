package ocrworker

import (
	//"github.com/sasha-s/go-deadlock"
	"sync"
	"time"
)

var (
	//requestsAndTimersMu sync.RWMutex
	requestsAndTimersMu sync.RWMutex
	// Requests is for holding and monitoring queued requests
	Requests           = make(map[string]chan OcrResult)
	ocrWasSentBackChan = make(chan string)
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

func getQueueLen() uint {
	requestsAndTimersMu.RLock()
	queueLength := uint(len(Requests))
	requestsAndTimersMu.RUnlock()
	return queueLength
}

func deleteRequestFromQueue(requestID string) {
	requestsAndTimersMu.Lock()
	inFlightGauge.Dec()
	/*		println("!!!!!!!!!!before deleting from Requests and requestChannels" + requestID)
			for key, element := range Requests {
				fmt.Println("Key:", key, "=>", "Element:", element)
			}*/
	_, ok := Requests[requestID]
	if ok {
		delete(Requests, requestID)
	}

	requestsAndTimersMu.Unlock()
}

func addNewOcrResultToQueue(storageTime int, requestID string, rpcResponseChan chan OcrResult) {

	inFlightGauge.Inc()
	requestsAndTimersMu.Lock()
	Requests[requestID] = rpcResponseChan
	requestsAndTimersMu.Unlock()

	// this go routine will cancel the request after global timeout or if request was sent back
	// if the requestID arrives on ocrWasSentBackChan - ocrResult was send back to requester an request deletion is triggered
	go func() {
		select {
		case <-ocrWasSentBackChan:
			requestsAndTimersMu.RLock()
			if _, ok := Requests[requestID]; ok {
				requestsAndTimersMu.RUnlock()
				deleteRequestFromQueue(requestID)
			}
		case <-time.After(time.Second * time.Duration(storageTime+10)):
			requestsAndTimersMu.RLock()
			if _, ok := Requests[requestID]; ok {
				requestsAndTimersMu.RUnlock()
				deleteRequestFromQueue(requestID)
			}
		}
	}()
}
