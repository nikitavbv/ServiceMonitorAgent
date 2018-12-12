package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"time"
)

const stateFileName = "/sm/state.json"
const trackingPeriod = 60 * time.Second

var state map[string]interface{}

func loadAgentState() {
	if _, err := os.Stat(stateFileName); os.IsNotExist(err) {
		state = map[string]interface{}{}
		return
	}

	stateData, _ := ioutil.ReadFile(stateFileName)
	err := json.Unmarshal(stateData, &state)
	if err != nil {
		log.Panicln(err)
	}
}

func saveAgentState() {
	stateJSON, _ := json.Marshal(state)
	err := ioutil.WriteFile(stateFileName, stateJSON, 0644)
	if err != nil {
		log.Println(err)
	}
}

func isAgentRegisterRequired() bool {
	if _, err := os.Stat(stateFileName); os.IsNotExist(err) {
		return true
	}
	if _, containsToken := state["token"]; containsToken {
		return false
	}
	return true
}

func registerAgent() {
	agentName := generateAgentName()
	log.Println("Registering agent")
	log.Println("Agent name:", agentName)

	registerResult, err := makeAPIRequest("POST", "/api/v1/agent", map[string]interface{}{
		"token": config["projectToken"],
		"name":  agentName,
	})
	if err != nil {
		log.Panicln("Failed to register at backend")
	}
	state["token"] = registerResult["apiKey"].(string)
	saveAgentState()
	log.Println("Done registering")
}

func initAgent() {
	propertiesMap := map[string]interface{}{
		"os":   getOSNameAndVersion(),
		"ipv4": getIPV4(),
		"ipv6": getIPV6(),
	}

	_, err := makeAPIRequest("PUT", "/api/v1/agent", map[string]interface{}{
		"properties": propertiesMap,
	})
	if err != nil {
		log.Panicln("Failed to do init")
	}
}

func startTrackingCycle() {
	runTrackingIteration()

	ticker := time.NewTicker(trackingPeriod)
	quit := make(chan struct{})
	for {
		select {
		case <-ticker.C:
			runTrackingIteration()
		case <-quit:
			ticker.Stop()
			return
		}
	}
}
