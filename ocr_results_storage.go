package ocrworker

import (
	"fmt"
	"sync"
	"time"
)

type ocrInFlight struct {
	ocrResInFlight      OcrResult
	ocrResInFlightTimer *time.Timer
	ocrResInFlightID    string
}

type ocrInFlightList []ocrInFlight

var (
	ocrQueueMu sync.RWMutex
	// OcrQueue is for holding and monitoring queued requests
	OcrQueue = NewInFlightList()
)

var (
	requestsAndTimersMu sync.RWMutex
	// Requests is for holding and monitoring queued requests
	Requests = make(map[string]chan OcrResult)
	timers   = make(map[string]*time.Timer)
)

func NewInFlightList() *ocrInFlightList {
	return &ocrInFlightList{}
}

func addNewOcrResult(resList ocrInFlightList, ocrResult *OcrResult, storageTime int, ocrID string) ocrInFlightList {
	tmpTimer := time.NewTimer(time.Duration(storageTime) * time.Second)
	var tempOcrInFlight = ocrInFlight{
		ocrResInFlight: *ocrResult,
		// ocrResInFlightDuration: time.Duration(storageTime) * time.Second,
		ocrResInFlightTimer: tmpTimer,
		ocrResInFlightID:    ocrID,
	}
	resList = append(resList, tempOcrInFlight)
	go func() {
		<-tmpTimer.C
		println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!time with req id %s fired", ocrID)
		remOcrResult(*OcrQueue, ocrID)
	}()

	return resList
}

func remOcrResult(resList ocrInFlightList, ocrID string) ocrInFlightList {

	for i, v := range resList {
		fmt.Printf("2**%d = %d\n", i, v)
		if v.ocrResInFlightID == ocrID {
			resList[i] = resList[len(resList)-1]
			resList[len(resList)-1] = ocrInFlight{}
			resList = resList[:len(resList)-1]
			break
		}
	}
	return resList
}
