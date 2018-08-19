package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// runAsSlave sets the instance to be a slave for the master
func (rc *resec) runAsSlave(masterAddress string, masterPort int) error {
	log.Printf("[DEBUG] Enslaving redis %s to be slave of %s:%d", rc.redis.address, masterAddress, masterPort)

	if err := rc.redis.client.SlaveOf(masterAddress, strconv.Itoa(masterPort)).Err(); err != nil {
		return fmt.Errorf("[ERROR] Could not enslave redis %s to be slave of %s:%d (%v)", rc.redis.address, masterAddress, masterPort, err)
	}

	log.Printf("[INFO] Enslaved redis %s to be slave of %s:%d", rc.redis.address, masterAddress, masterPort)

	// change our internal state to being a slave
	rc.redis.replicationStatus = "slave"
	if err := rc.registerService(); err != nil {
		return fmt.Errorf("[ERROR] Consul Service registration failed - %s", err)
	}

	// if we are enslaved and our status is published in consul, lets go back to trying
	// to acquire leadership / master role as well
	go rc.acquireConsulLeadership()

	return nil
}

// runAsMaster sets the instance to be the master
func (rc *resec) runAsMaster() error {
	if err := rc.redis.client.SlaveOf("no", "one").Err(); err != nil {
		return err
	}

	log.Println("[INFO] Promoted redis to Master")
	return nil
}

// watchRedisReplicationStatus checks redis replication status
func (rc *resec) watchRedisReplicationStatus() {
	ticker := time.NewTicker(rc.healthCheckInterval)

	for ; true; <-ticker.C {
		log.Println("[DEBUG] Checking redis replication status")

		result, err := rc.redis.client.Info("replication").Result()
		if err != nil {
			log.Printf("[ERROR] Can't connect to redis running on %s", rc.redis.address)
			result = fmt.Sprintf("Can't connect to redis running on %s", rc.redis.address)
		}

		rc.redisReplicationCh <- &redisHealth{
			output:  result,
			healthy: err == nil,
		}
	}
}

// watchRedisUptime checks redis server uptime
func (rc *resec) watchRedisUptime() {
	lastUptime := 0
	connectionErrors := 0
	allowedConnectionErrors := 3

	ticker := time.NewTicker(rc.healthCheckInterval)
	for ; true; <-ticker.C {
		log.Println("[DEBUG] Checking redis server info")

		result, err := rc.redis.client.Info("server").Result()
		if err != nil {
			log.Printf("[WARN] Could not query for server info: %s", err)
			connectionErrors++

			if connectionErrors > allowedConnectionErrors {
				log.Printf("[ERROR] Too many connection errors, shutting down")
				rc.stopCh <- true
			}

			continue
		}
		connectionErrors = 0

		parsed := parseKeyValue(result)
		uptimeString, ok := parsed["uptime_in_seconds"]
		if !ok {
			log.Printf("[ERROR] Could not find 'uptime_in_seconds' in server info respone")
			continue
		}

		uptime, err := strconv.Atoi(uptimeString)
		if err != nil {
			log.Printf("[ERROR] Could not parse 'uptime_in_seconds' to integer")
			continue
		}

		if uptime < lastUptime {
			log.Printf("[ERROR] Current uptime (%d) is less than previous (%d) - Redis likely restarted - stopping resec", uptime, lastUptime)
			rc.stopCh <- true
			continue
		}

		lastUptime = uptime
	}
}

func (rc *resec) waitForRedisToBeReady() {
	t := time.NewTicker(time.Second)

	for {
		select {
		case <-t.C:
			persistenceString, err := rc.redis.client.Info("persistence").Result()
			if err != nil {
				log.Printf("[ERROR] could not query for redis persistence info: %s", err)
				continue
			}

			persistence := parseKeyValue(persistenceString)
			loading, ok := persistence["loading"]
			if !ok {
				log.Printf("[ERROR] could not find 'persistence.loading' key")
				continue
			}

			if loading != "0" {
				log.Printf("[INFO] Redis is not ready yet, currently loading data from disk")
				continue
			}

			log.Printf("[INFO] Redis is ready to serve traffic")
			return
		}
	}
}

func parseKeyValue(str string) map[string]string {
	res := make(map[string]string)

	lines := strings.Split(str, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			continue
		}

		pair := strings.Split(line, ":")
		if len(pair) != 2 {
			continue
		}

		res[pair[0]] = pair[1]
	}

	return res
}
