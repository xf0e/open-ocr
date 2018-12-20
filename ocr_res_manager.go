package ocrworker

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

// {"consumer_details":[{"arguments":{},"channel_details":{"connection_name":"127.0.0.1:42242 -> 127.0.0.1:5672","name":"127.0.0.1:42242 -> 127.0.0.1:5672 (1)","node":"rabbit@wega","number":1,"peer_host":"127.0.0.1","peer_port":42242,"user":"guest"},"ack_required":true,"consumer_tag":"foo","exclusive":false,"prefetch_count":0,"queue":{"name":"decode-ocr","vhost":"/"}}],"arguments":{},"auto_delete":false,"backing_queue_status":{"avg_ack_egress_rate":0.13959283486782112,"avg_ack_ingress_rate":0.09377917479588953,"avg_egress_rate":0.09377917479588953,"avg_ingress_rate":0.07468769667840547,"delta":["delta",0,0,0,0],"len":0,"mode":"default","next_seq_id":17,"q1":0,"q2":0,"q3":0,"q4":0,"target_ram_count":"infinity"},"consumer_utilisation":null,"consumers":1,"deliveries":[],"durable":true,"effective_policy_definition":[],"exclusive":false,"exclusive_consumer_tag":null,"garbage_collection":{"fullsweep_after":65535,"max_heap_size":0,"min_bin_vheap_size":46422,"min_heap_size":233,"minor_gcs":6},"head_message_timestamp":null,"idle_since":"2018-12-20 0:53:28","incoming":[],"memory":17044,"message_bytes":0,"message_bytes_paged_out":0,"message_bytes_persistent":0,"message_bytes_ram":0,"message_bytes_ready":0,"message_bytes_unacknowledged":0,"message_stats":{"ack":17,"ack_details":{"rate":0.0},"deliver":20,"deliver_details":{"rate":0.0},"deliver_get":20,"deliver_get_details":{"rate":0.0},"deliver_no_ack":0,"deliver_no_ack_details":{"rate":0.0},"get":0,"get_details":{"rate":0.0},"get_no_ack":0,"get_no_ack_details":{"rate":0.0},"publish":17,"publish_details":{"rate":0.0},"redeliver":3,"redeliver_details":{"rate":0.0}},"messages":0,"messages_details":{"rate":0.0},"messages_paged_out":0,"messages_persistent":0,"messages_ram":0,"messages_ready":0,"messages_ready_details":{"rate":0.0},"messages_ready_ram":0,"messages_unacknowledged":0,"messages_unacknowledged_details":{"rate":0.0},"messages_unacknowledged_ram":0,"name":"decode-ocr","node":"rabbit@wega","operator_policy":null,"policy":null,"recoverable_slaves":null,"reductions":76046,"reductions_details":{"rate":0.0},"state":"running","vhost":"/"}

type OcrResManager struct {
	numMessages  string `json:"messages"`
	numConsumers string `json:"consumers"`
}

type AmqpApiConfig struct {
	AmqpURI   string
	Port      string
	Path      string
	QueueName string
}

func DefaultResManagerConfig() AmqpApiConfig {

	// http://localhost:15672/api/queues/%2f/decode-ocr

	AmqpApiConfig := AmqpApiConfig{
		AmqpURI:   "http://guest:guest@localhost:",
		Port:      "15672",
		Path:      "/api/queues/%2f/",
		QueueName: "decode-ocr",
	}
	return AmqpApiConfig

}

func serviceCanAccept(config *AmqpApiConfig) bool {
	isAvailable := false
	resManager := new(OcrResManager)
	var url = ""
	url += config.AmqpURI + config.Port + config.Path + config.QueueName
	getJson(url, &resManager)

	println(url)
	println(resManager.numMessages)
	println(resManager.numConsumers)

	return isAvailable
}

func getJson(url string, target interface{}) error {

	var myClient = &http.Client{Timeout: 10 * time.Second}
	r, err := myClient.Get(url)
	if err != nil {
		// return err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err.Error())
	}

	return json.Unmarshal(body, &target)
}
