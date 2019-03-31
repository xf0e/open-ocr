package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
	"time"
)

type ocrQueueManager struct {
	NumMessages  uint `json:"messages"`
	NumConsumers uint `json:"consumers"`
	MessageBytes uint `json:"message_bytes"`
}

type ocrResManager struct {
	MemLimit uint64 `json:"mem_limit"`
	MemUsed  uint64 `json:"mem_used"`
}

const (
	factorForMessageAccept uint   = 2
	memoryThreshold        uint64 = 95
)

// checks if resources for incoming request are available
func CheckForAcceptRequest(queueManager ocrQueueManager, resManager []ocrResManager, config RabbitConfig, statusChanged bool) bool {

	isAvailable := false
	var urlQueue, urlStat = "", ""
	urlQueue += config.AmqpAPIURI + config.APIPathQueue + config.APIQueueName
	urlStat += config.AmqpAPIURI + config.APIPathStats
	jsonQueueStat, err := url2bytes(urlQueue)
	if err != nil {
		logg.LogError(err)
		return false
	}
	jsonResStat, err := url2bytes(urlStat)
	if err != nil {
		logg.LogError(err)
		return false
	}

	err = json.Unmarshal(jsonQueueStat, &queueManager)
	if err != nil {
		msg := "Error unmarshaling json: %v"
		errMsg := fmt.Sprintf(msg, err)
		logg.LogError(fmt.Errorf(errMsg))
		return false
	}

	err = json.Unmarshal(jsonResStat, &resManager)
	if err != nil {
		msg := "Error unmarshaling json: %v"
		errMsg := fmt.Sprintf(msg, err)
		logg.LogError(fmt.Errorf(errMsg))
		return false
	}

	flagForResources := schedulerByMemoryLoad(resManager)
	flagForQueue := schedulerByWorkerNumber(&queueManager)
	if flagForQueue && flagForResources {
		isAvailable = true
	}

	if statusChanged {
		logg.LogTo("OCR_RESMAN", "Queue statistics: messages size %v, number consumers %v, number messages %v,	memory stats %v",
			queueManager.MessageBytes,
			queueManager.NumConsumers,
			queueManager.NumMessages,
			resManager)
		// logg.LogTo("OCR_RESMAN", "API URL %s", urlQueue)
		// logg.LogTo("OCR_RESMAN", "API URL %s", urlStat)
		if isAvailable {
			logg.LogTo("OCR_RESMAN", "open-ocr is operational with free resources. We are ready to serve.")
		} else {
			logg.LogTo("OCR_RESMAN", "open-ocr is alive but won't serve any requests. Workers are busy or not connected.")
		}

	}

	return isAvailable
}

// computes the ratio of total available memory and used memory and returns a bool value if a threshold is reached
func schedulerByMemoryLoad(resManager []ocrResManager) bool {
	resFlag := false
	var memTotalAvailable uint64
	var memTotalInUse uint64
	for k := range resManager {
		memTotalInUse += resManager[k].MemUsed
		memTotalAvailable += resManager[k].MemLimit
	}

	if memTotalInUse < ((memTotalAvailable * memoryThreshold) / 100) {
		resFlag = true
	}
	return resFlag
}

// if the number of messages in the queue too high we should not accept the new messages
func schedulerByWorkerNumber(resManger *ocrQueueManager) bool {
	resFlag := false
	if (resManger.NumMessages) < (resManger.NumConsumers * factorForMessageAccept) {
		resFlag = true
	}
	return resFlag
}

func SetResManagerState(ampqAPIConfig RabbitConfig) {
	resManager := make([]ocrResManager, 0)
	queueManager := new(ocrQueueManager)
	var boolValueChanged = false
	var boolNewValue = false
	var boolOldValue = false
	for {
		// only print the RESMAN output if the state has changed
		boolValueChanged = boolOldValue != boolNewValue
		if boolValueChanged {
			boolOldValue = boolNewValue
		}
		ServiceCanAcceptMu.Lock()
		ServiceCanAccept = CheckForAcceptRequest(*queueManager, resManager, ampqAPIConfig, boolValueChanged)
		boolNewValue = ServiceCanAccept
		ServiceCanAcceptMu.Unlock()
		time.Sleep(500 * time.Millisecond)
	}
}
