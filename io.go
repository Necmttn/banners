package main

import (
	"encoding/json"
)

type JsonRawIpPort struct {
	Ip   string `json:"ip"`
	Port int    `json:"port"`
}

func decodeJson(addressInput string) (JsonRawIpPort, error) {
	var result JsonRawIpPort
	err := json.Unmarshal([]byte(addressInput), &result)
	return result, err
}
