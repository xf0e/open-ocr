package ocrworker

import (
	"encoding/json"
	"fmt"
	"github.com/couchbaselabs/logg"
)

type OcrQueueManager struct {
	NumMessages  uint `json:"messages"`
	NumConsumers uint `json:"consumers"`
	MessageBytes uint `json:"message_bytes"`
}

type OcrResManager struct {
	MemLimit uint64 `json:"mem_limit"`
	MemUsed  uint64 `json:"mem_used"`
}

const (
	factorForMessageAccept uint   = 2
	memoryThreshold        uint64 = 95
)

type AmqpAPIConfig struct {
	AmqpURI   string
	Port      string
	PathQueue string
	PathStats string
	QueueName string
}

func DefaultResManagerConfig() AmqpAPIConfig {

	AmqpApiConfig := AmqpAPIConfig{
		AmqpURI:   "http://guest:guest@localhost:",
		Port:      "15672",
		PathQueue: "/api/queues/%2f/",
		PathStats: "/api/nodes",
		QueueName: "decode-ocr",
	}
	return AmqpApiConfig

}

func AcceptRequest(config *AmqpAPIConfig) bool {
	isAvailable := false
	resManager := make([]OcrResManager, 0)
	queueManager := new(OcrQueueManager)
	var urlQueue, urlStat = "", ""
	urlQueue += config.AmqpURI + config.Port + config.PathQueue + config.QueueName
	urlStat += config.AmqpURI + config.Port + config.PathStats
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

	logg.LogTo("OCR_CLIENT", "Queue statistics: messages size %v, number consumers %v, number messages %v,	memory stats %v",
		queueManager.MessageBytes,
		queueManager.NumConsumers,
		queueManager.NumMessages,
		resManager)
	logg.LogTo("OCR_CLIENT", "API URL %s", urlQueue)
	logg.LogTo("OCR_CLIENT", "API URL %s", urlStat)

	flagForResources := schedulerByMemoryLoad(resManager)
	flagForQueue := schedulerByWorkerNumber(queueManager)
	if flagForQueue && flagForResources {
		isAvailable = true
		logg.LogTo("OCR_CLIENT", "resources for request are available")
	}

	return isAvailable
}

// computes the ratio of total available memory and used memory and returns a bool value if a threshold is reached
func schedulerByMemoryLoad(resManager []OcrResManager) bool {
	resFlag := false
	var memTotalAvailable uint64
	var memTotalInUse uint64
	for k := range resManager {
		memTotalInUse += resManager[k].MemUsed
		memTotalAvailable += resManager[k].MemLimit
	}

	logg.LogTo("OCR_CLIENT", "Memory in RabbitMQ cluster available %v, used %v", memTotalAvailable,
		memTotalInUse)

	if memTotalInUse < ((memTotalAvailable * memoryThreshold) / 100) {
		resFlag = true
	}

	return resFlag
}

// if the number of messages in the queue to high we should not accept the new messages
func schedulerByWorkerNumber(resManger *OcrQueueManager) bool {
	resFlag := false
	if (resManger.NumMessages) < (resManger.NumConsumers * factorForMessageAccept) {
		resFlag = true
	}
	return resFlag
}
