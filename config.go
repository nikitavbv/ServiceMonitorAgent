package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
)

const configFileName = "/sm/config.json"

var config map[string]interface{}

func checkIfConfigFileExists() bool {
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		return false
	}
	return true
}

func loadConfigFile() {
	configData, _ := ioutil.ReadFile(configFileName)
	err := json.Unmarshal(configData, &config)
	if err != nil {
		log.Panicln(err)
	}
}
