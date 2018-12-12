package main

import "log"

func main() {
	if !checkIfConfigFileExists() {
		log.Fatalln("Config file is not found. Consider creating one at " + configFileName)
	}

	loadConfigFile()
	loadAgentState()
	if isAgentRegisterRequired() {
		registerAgent()
	}
	initAgent()

	startTrackingCycle()
}
