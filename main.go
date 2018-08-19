package main

import (
	"log"
)

func main() {
	log.Println("[INFO] Start!")

	resec, err := setup()
	if err != nil {
		log.Fatal(err)
	}

	resec.waitForRedisToBeReady()
	go resec.watchRedisUptime()
	go resec.watchRedisReplicationStatus()
	go resec.watchConsulMasterService()

	resec.run()
}
