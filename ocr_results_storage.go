package ocrworker

import (
	"sync"
	"sync/atomic"
)

var (
	RequestsTrack      = sync.Map{}
	RequestTrackLength = uint32(0)
	// ocrWasSentBackChan = make(chan string)
)

// CheckOcrStatusByID checks status of an ocr request based on origin of request
func CheckOcrStatusByID(requestID string) (OcrResult, bool) {
	if _, ok := RequestsTrack.Load(requestID); !ok {
		return OcrResult{}, false
	}

	ocrResult := OcrResult{}

	tempChannel := make(chan OcrResult)
	v, ok := RequestsTrack.Load(requestID)
	if ok {
		tempChannel = v.(chan OcrResult)
	} else {
		// log.Info().Str("component", "OCR_CLIENT").Str("RequestID", requestID).Msg("no such request found in the queue")
		return OcrResult{}, false
	}

	select {
	case ocrResult = <-tempChannel:
		// log.Debug().Str("component", "OCR_CLIENT").Msg("got ocrResult := <-Requests[requestID]")
		defer deleteRequestFromQueue(requestID)
	default:
		return OcrResult{Status: "processing", ID: requestID}, true
	}

	return ocrResult, true
}

func getQueueLen() uint {
	return uint(atomic.LoadUint32(&RequestTrackLength))
}

func deleteRequestFromQueue(requestID string) {
	atomic.AddUint32(&RequestTrackLength, ^uint32(0))
	inFlightGauge.Dec()
	RequestsTrack.Delete(requestID)
}

func addNewOcrResultToQueue(requestID string, rpcResponseChan chan OcrResult) {
	atomic.AddUint32(&RequestTrackLength, 1)
	inFlightGauge.Inc()
	RequestsTrack.Store(requestID, rpcResponseChan)

	// this go routine will cancel the request after global timeout or if request was sent back
	// if the requestID arrives on ocrWasSentBackChan - ocrResult was send back to requester an request deletion is triggered
	// go func() {
	// 	select {
	// 	case <-ocrWasSentBackChan:
	// 		if _, ok := RequestsTrack.Load(requestID); ok {
	// 			deleteRequestFromQueue(requestID)
	// 		}
	// 		// TODO: a bug leaking goroutines if the global timeout is set to a low value the routine in ocr_rpc_client:221 will leak
	// 		// TODO since there is no listener in this goroutine since this goroutine is dead
	// 	/* case <-time.After(time.Second * time.Duration(storageTime+10)):
	// 		if _, ok := RequestsTrack.Load(requestID); ok {
	// 			deleteRequestFromQueue(requestID)
	// 		}*/
	// //default:
	//
	// 	}
	// }()
}
